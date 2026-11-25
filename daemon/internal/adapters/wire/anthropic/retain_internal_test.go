package anthropic

import (
	"net/http"
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/wire"
	"github.com/jonaskahn/relo/internal/inference"
)

// TestInboundRetainsTheFinishedReply covers what a client continues from: the
// reply is handed over once, under the identity the client will send back, with
// the text the assistant produced.
func TestInboundRetainsTheFinishedReply(t *testing.T) {
	inbound := NewInbound()
	var kept []wire.Turn
	inbound.RetainTurn(func(turn wire.Turn) { kept = append(kept, turn) })

	events := []inference.Event{
		{Kind: inference.EventMessageStart, MessageID: "msg_upstream"},
		{Kind: inference.EventTextDelta, Text: "the answer"},
		{Kind: inference.EventTerminal, Terminal: &inference.TerminalInfo{Reason: inference.EventReasonStop}},
	}
	for _, event := range events {
		if _, err := inbound.EncodeResponseEvent(event); err != nil {
			t.Fatalf("EncodeResponseEvent(%s) error = %v", event.Kind, err)
		}
	}

	if len(kept) != 1 {
		t.Fatalf("retained %d turns, want the finished reply once", len(kept))
	}
	if kept[0].MessageID != "msg_upstream" {
		t.Fatalf("message id = %q, want the identity the client continues", kept[0].MessageID)
	}
	if kept[0].Assistant.Role != inference.RoleAssistant {
		t.Fatalf("role = %q, want an assistant turn", kept[0].Assistant.Role)
	}
	if text := kept[0].Assistant.Content; len(text) != 1 || text[0].Text != "the answer" {
		t.Fatalf("content = %+v, want the streamed text", text)
	}
}

// TestInboundDoesNotRetainARefusedReply keeps a failed exchange out of the
// thread: a reply that ends in an error never opened a conversation.
func TestInboundDoesNotRetainARefusedReply(t *testing.T) {
	inbound := NewInbound()
	retained := false
	inbound.RetainTurn(func(wire.Turn) { retained = true })
	if _, err := inbound.EncodeResponseEvent(inference.Event{
		Kind: inference.EventError, Error: &inference.ErrorInfo{Code: "overloaded_error", Message: "busy"},
	}); err != nil {
		t.Fatalf("EncodeResponseEvent() error = %v", err)
	}
	if retained {
		t.Fatal("retained a reply that ended in an error")
	}
}

// TestInboundRetainsNothingWithoutAListener keeps a build that keeps no
// history working: the closing frames are built whether or not anyone listens.
func TestInboundRetainsNothingWithoutAListener(t *testing.T) {
	frames, err := NewInbound().EncodeResponseEvent(inference.Event{
		Kind: inference.EventTerminal, Terminal: &inference.TerminalInfo{Reason: inference.EventReasonStop},
	})
	if err != nil {
		t.Fatalf("EncodeResponseEvent() error = %v", err)
	}
	if frameNamed(frames, "message_stop") == "" {
		t.Fatalf("frames = %v, want the closing frame", frameNames(frames))
	}
}

// TestInboundDecodesTheThreadAClientContinues covers the request side: a
// continue with an id names the thread, and every other shape carries its own
// history so it names none.
func TestInboundDecodesTheThreadAClientContinues(t *testing.T) {
	cases := []struct {
		name string
		body string
		want *inference.ThreadRef
	}{
		{
			name: "a continue names the reply it answers",
			body: `{"model":"claude-sonnet-5","max_tokens":64,"thread":{"type":"continue","previous_message_id":"msg_01"},` +
				`"messages":[{"role":"user","content":"go on"}]}`,
			want: &inference.ThreadRef{Type: "continue", PreviousID: "msg_01"},
		},
		{
			name: "a create carries its own history",
			body: `{"model":"claude-sonnet-5","max_tokens":64,"thread":{"type":"create"},` +
				`"messages":[{"role":"user","content":"go"}]}`,
		},
		{
			name: "a continue without an id carries its own history",
			body: `{"model":"claude-sonnet-5","max_tokens":64,"thread":{"type":"continue"},` +
				`"messages":[{"role":"user","content":"go"}]}`,
		},
		{
			name: "a request that names no thread",
			body: `{"model":"claude-sonnet-5","max_tokens":64,"messages":[{"role":"user","content":"go"}]}`,
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			request, err := http.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(test.body))
			if err != nil {
				t.Fatalf("build the request: %v", err)
			}
			decoded, err := NewInbound().DecodeRequest(request)
			if err != nil {
				t.Fatalf("DecodeRequest() error = %v", err)
			}
			if test.want == nil {
				if decoded.Thread != nil {
					t.Fatalf("thread = %+v, want none", decoded.Thread)
				}
				return
			}
			if decoded.Thread == nil || *decoded.Thread != *test.want {
				t.Fatalf("thread = %+v, want %+v", decoded.Thread, test.want)
			}
		})
	}
}

// TestInboundRetainsTheReplyExactlyOnce guards a stream that closes twice: the
// client can read the id after the first closing frame, so a second one must
// not store the turn again.
func TestInboundRetainsTheReplyExactlyOnce(t *testing.T) {
	inbound := NewInbound()
	count := 0
	inbound.RetainTurn(func(wire.Turn) { count++ })
	for range 2 {
		if _, err := inbound.EncodeResponseEvent(inference.Event{
			Kind: inference.EventTerminal, Terminal: &inference.TerminalInfo{Reason: inference.EventReasonStop},
		}); err != nil {
			t.Fatalf("EncodeResponseEvent() error = %v", err)
		}
	}
	if count != 1 {
		t.Fatalf("retained %d times, want once", count)
	}
}

func frameNames(frames []wire.SSEEvent) []string {
	names := make([]string, 0, len(frames))
	for _, frame := range frames {
		names = append(names, frame.Name)
	}
	return names
}
