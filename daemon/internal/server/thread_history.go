// Server-side thread history: a client that continues a thread sends only
// the new turn, and Relo rebuilds the conversation the reply belongs to.
package server

import (
	"context"
	"errors"
	"net/http"

	"github.com/jonaskahn/relo/internal/adapters/wire"
	"github.com/jonaskahn/relo/internal/inference"
	"github.com/jonaskahn/relo/internal/threads"
)

// errThreadNotFound refuses a continuation Relo cannot rebuild. Forwarding it
// as a one-message request is what made a continued turn look like a new
// conversation, so the client is asked to resend instead.
var errThreadNotFound = errors.New("this conversation is no longer held; resend the message to start a new one")

// pendingTurn is what a finished reply is recorded under.
type pendingTurn struct {
	parent *threads.Resolved
	delta  []inference.Message
	system *inference.Message
	tools  []inference.Tool
}

// prepareThread resolves the thread a request continues and expands its
// history into the request. It reports whether the history came from the
// store, which decides whether the client's own native body can be reused.
func (s *Server) prepareThread(r *http.Request, request *inference.Request) (pendingTurn, bool, error) {
	system, messages := splitSystem(request.Messages)
	turn := pendingTurn{delta: messages, system: system, tools: request.Tools}
	if s.opts.Threads == nil || request.Thread == nil {
		return turn, false, nil
	}
	keyID, _, _ := describeClient(r.Context())
	resolved, found, err := s.opts.Threads.Resolve(r.Context(), keyID, request.Thread.PreviousID)
	if err != nil {
		return turn, false, err
	}
	if !found {
		return turn, false, errThreadNotFound
	}
	turn.parent = &resolved
	expandRequest(request, resolved, system, messages)
	return turn, true, nil
}

// expandRequest puts the stored history in front of the messages this request
// brought, and restores the tools and system prompt the thread opened with.
func expandRequest(request *inference.Request, resolved threads.Resolved, system *inference.Message, messages []inference.Message) {
	expandedSystem := resolved.System
	if system != nil && !wire.IsBillingStampOnly(textOf(system)) {
		expandedSystem = system
	}
	tools := resolved.Tools
	if len(request.Tools) > 0 {
		tools = request.Tools
	}
	combined := append(resolved.Messages, messages...)
	combined = inference.MergeToolResults(combined)
	if expandedSystem != nil {
		combined = append([]inference.Message{*expandedSystem}, combined...)
	}
	request.Messages = combined
	request.Tools = tools
}

// retainThread records the finished reply so the next turn can continue from
// it. The reply id is the key the client sends back.
func (s *Server) retainThread(r *http.Request, turn pendingTurn, reply wire.Turn) {
	if s.opts.Threads == nil {
		return
	}
	keyID, _, _ := describeClient(r.Context())
	stored := threads.Turn{
		ClientKey: keyID, ID: reply.MessageID,
		Delta: append(append([]inference.Message(nil), turn.delta...), reply.Assistant),
	}
	if turn.parent == nil {
		stored.System, stored.Tools = turn.system, turn.tools
	} else {
		stored.ParentID = turn.parent.NodeID
	}
	// The reply is already written, so a store that cannot keep it costs only
	// the next turn its history; logging it is the operator's warning.
	if err := s.opts.Threads.Record(context.WithoutCancel(r.Context()), stored); err != nil && s.opts.Logger != nil {
		s.opts.Logger.Warn("keep the finished thread turn", "error", err)
	}
}

// splitSystem separates the system prompt the decoder prepended from the
// conversation it belongs to. A client that continues a thread sends its
// billing stamp as a system prompt, which is not a prompt at all.
func splitSystem(messages []inference.Message) (*inference.Message, []inference.Message) {
	if len(messages) > 0 && messages[0].Role == inference.RoleSystem {
		system := messages[0]
		return &system, messages[1:]
	}
	return nil, messages
}

func textOf(message *inference.Message) string {
	if message == nil {
		return ""
	}
	text := ""
	for _, part := range message.Content {
		if part.Type == inference.ContentTypeText {
			text += part.Text
		}
	}
	return text
}

// writeThreadRefusal answers a continuation Relo cannot rebuild. It is a
// request error the client can act on, not an upstream failure.
func (s *Server) writeThreadRefusal(w http.ResponseWriter, err error) {
	s.writeAnthropicDocument(w, http.StatusBadRequest, "invalid_request_error", err.Error())
}

// installThreadRetainer makes the inbound codec report its finished reply to
// the thread store. It reports false when the codec cannot.
func installThreadRetainer(inbound wire.InboundCodec, retain func(wire.Turn)) bool {
	reporter, ok := inbound.(wire.TurnReporter)
	if !ok {
		return false
	}
	reporter.RetainTurn(retain)
	return true
}
