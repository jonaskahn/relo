package cursor

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	adapterscursor "github.com/jonaskahn/relo/internal/adapters/cursor"
	"github.com/jonaskahn/relo/internal/adapters/wire"
	"github.com/jonaskahn/relo/internal/inference"
)

// fixtureServer speaks the Cursor agent protocol against one scripted turn.
type fixtureServer struct {
	t      *testing.T
	server *httptest.Server
}

func newFixtureServer(t *testing.T, script func(s *fixtureServer, w http.ResponseWriter, reader *adapterscursor.FrameReader)) *fixtureServer {
	t.Helper()
	s := &fixtureServer{t: t}
	// TLS with HTTP/2 so the turn duplexes: a stock HTTP/1.1 connection
	// writes the request body to EOF before reading the response, which
	// would deadlock the conversation.
	s.server = httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/agent.v1.AgentService/Run") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/connect+proto")
		w.WriteHeader(http.StatusOK)
		script(s, w, adapterscursor.NewFrameReader(r.Body))
	}))
	s.server.EnableHTTP2 = true
	s.server.StartTLS()
	t.Cleanup(s.server.Close)
	return s
}

func (s *fixtureServer) write(w http.ResponseWriter, payload []byte) {
	s.t.Helper()
	if _, err := w.Write(adapterscursor.Frame(payload)); err != nil {
		s.t.Fatalf("write frame: %v", err)
	}
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (s *fixtureServer) next(reader *adapterscursor.FrameReader) []byte {
	s.t.Helper()
	payload, ended, err := reader.Next()
	if err != nil {
		s.t.Fatalf("read frame: %v", err)
	}
	if ended {
		s.t.Fatal("stream ended early")
	}
	return payload
}

func serverExecMessage(numeric uint32, execID string, variant int) []byte {
	var message adapterscursor.Writer
	message.Uint32(1, numeric)
	message.String(15, execID)
	message.Empty(variant)
	var envelope adapterscursor.Writer
	envelope.Message(2, message.BytesOut())
	return envelope.BytesOut()
}

func serverUpdate(variant int, payload []byte) []byte {
	var update adapterscursor.Writer
	update.Message(variant, payload)
	var envelope adapterscursor.Writer
	envelope.Message(1, update.BytesOut())
	return envelope.BytesOut()
}

func textPayload(text string) []byte {
	var delta adapterscursor.Writer
	delta.String(1, text)
	return delta.BytesOut()
}

func turnEndedPayload() []byte {
	var ended adapterscursor.Writer
	ended.Int64(1, 12)
	ended.Int64(2, 34)
	return ended.BytesOut()
}

func mcpArgsPayload(name, toolCallID string) []byte {
	var args adapterscursor.Writer
	args.String(1, name)
	args.String(3, toolCallID)
	args.String(4, "relo")
	args.String(5, name)
	var message adapterscursor.Writer
	message.Uint32(1, 7)
	message.String(15, "exec-mcp")
	message.Message(11, args.BytesOut())
	var envelope adapterscursor.Writer
	envelope.Message(2, message.BytesOut())
	return envelope.BytesOut()
}

func testSelection() *wire.CursorRequest {
	return &wire.CursorRequest{
		ConversationID: "conversation-1",
		ModelID:        "sonnet",
		DetailsID:      "sonnet-low",
		Params:         []wire.CursorParam{{ID: "effort", Value: "low"}},
	}
}

func testRequest() *inference.Request {
	return &inference.Request{
		Model: "sonnet",
		Messages: []inference.Message{
			{Role: inference.RoleUser, Content: []inference.ContentPart{{Type: inference.ContentTypeText, Text: "hello"}}},
		},
		Stream: false,
	}
}

func parseClientMessage(t *testing.T, payload []byte) []adapterscursor.Field {
	t.Helper()
	fields, err := adapterscursor.Parse(payload)
	if err != nil {
		t.Fatalf("parse client message: %v", err)
	}
	return fields
}

// assertHandoff checks the reply carries an MCP success result, which is
// the external-handoff acknowledgement rather than a refusal.
func assertHandoff(t *testing.T, raw []byte) {
	t.Helper()
	fields, err := adapterscursor.Parse(raw)
	if err != nil {
		t.Fatalf("parse exec answer: %v", err)
	}
	results, err := adapterscursor.Nested(fields, resultMCP)
	if err != nil || len(results) != 1 {
		t.Fatalf("exec answer = %+v, want one MCP result", fields)
	}
	success, err := adapterscursor.Nested(results[0], mcpSuccessCase)
	if err != nil || len(success) != 1 {
		t.Fatalf("mcp result = %+v, want success", results[0])
	}
}

func h2Client(client *http.Client) *http.Client {
	transport, ok := client.Transport.(*http.Transport)
	if !ok || transport == nil {
		return client
	}
	clone := transport.Clone()
	clone.ForceAttemptHTTP2 = true
	duplicated := *client
	duplicated.Transport = clone
	return &duplicated
}

func TestRunTextTurn(t *testing.T) {
	server := newFixtureServer(t, func(s *fixtureServer, w http.ResponseWriter, reader *adapterscursor.FrameReader) {
		run := parseClientMessage(t, s.next(reader))
		if len(run) != 1 || run[0].Number != clientRunRequest {
			t.Fatalf("first message = %+v, want a run request", run)
		}
		s.write(w, serverExecMessage(1, "exec-1", 10))
		reply := parseClientMessage(t, s.next(reader))
		if len(reply) != 1 || reply[0].Number != clientExecMessage {
			t.Fatalf("reply = %+v, want an exec answer", reply)
		}
		s.write(w, serverUpdate(1, textPayload("hello there")))
		s.write(w, serverUpdate(14, turnEndedPayload()))
		for {
			payload, ended, err := reader.Next()
			if err != nil && err != io.EOF {
				t.Fatalf("drain: %v", err)
			}
			if ended || err != nil {
				return
			}
			_ = payload
		}
	})
	codec := NewCodec(server.server.URL)
	events, err := codec.RunBidirectional(context.Background(), wire.BidirectionalCall{
		Client: h2Client(server.server.Client()), Request: testRequest(), Opts: wire.CodecOpts{
			BaseURL: server.server.URL, CredentialRef: "token", Cursor: testSelection(),
		},
	})
	if err != nil {
		t.Fatalf("RunBidirectional() error = %v", err)
	}
	kinds := []inference.EventKind{}
	for _, event := range events {
		kinds = append(kinds, event.Kind)
	}
	want := []inference.EventKind{inference.EventTextDelta, inference.EventUsage, inference.EventTerminal}
	if len(kinds) != len(want) {
		t.Fatalf("kinds = %v, want %v", kinds, want)
	}
	for index := range want {
		if kinds[index] != want[index] {
			t.Fatalf("kinds = %v, want %v", kinds, want)
		}
	}
	if events[0].Text != "hello there" {
		t.Fatalf("text = %q", events[0].Text)
	}
	if events[1].Usage == nil || events[1].Usage.InputTokens != 12 || events[1].Usage.OutputTokens != 34 {
		t.Fatalf("usage = %+v", events[1].Usage)
	}
	if events[2].Terminal == nil || events[2].Terminal.Reason != inference.EventReasonStop {
		t.Fatalf("terminal = %+v", events[2].Terminal)
	}
}

func TestRunNativeRejection(t *testing.T) {
	server := newFixtureServer(t, func(s *fixtureServer, w http.ResponseWriter, reader *adapterscursor.FrameReader) {
		_ = parseClientMessage(t, s.next(reader))
		s.write(w, serverExecMessage(1, "exec-1", 10))
		_ = parseClientMessage(t, s.next(reader))
		var shell adapterscursor.Writer
		shell.String(1, "rm -rf /")
		shell.String(4, "call-shell")
		var message adapterscursor.Writer
		message.Uint32(1, 9)
		message.String(15, "exec-shell")
		message.Message(2, shell.BytesOut())
		var envelope adapterscursor.Writer
		envelope.Message(2, message.BytesOut())
		s.write(w, envelope.BytesOut())
		reply := parseClientMessage(t, s.next(reader))
		if len(reply) != 1 || reply[0].Number != clientExecMessage {
			t.Fatalf("reply = %+v, want the refusal", reply)
		}
		execMsg := mustParseForTest(t, reply[0].Raw)
		shellResults := 0
		for _, field := range execMsg {
			if field.Number != resultShell || field.Wire != 2 {
				continue
			}
			shellResults++
			rejected, err := adapterscursor.Nested(mustParseForTest(t, field.Raw), 4)
			if err != nil || len(rejected) != 1 {
				t.Fatalf("shell result = %v %v, want the rejection", rejected, err)
			}
		}
		if shellResults != 1 {
			t.Fatalf("exec answer = %+v, want one shell result", execMsg)
		}
		s.write(w, serverUpdate(14, turnEndedPayload()))
		for {
			_, ended, err := reader.Next()
			if ended || err != nil {
				return
			}
		}
	})
	codec := NewCodec(server.server.URL)
	events, err := codec.RunBidirectional(context.Background(), wire.BidirectionalCall{
		Client: h2Client(server.server.Client()), Request: testRequest(), Opts: wire.CodecOpts{
			BaseURL: server.server.URL, CredentialRef: "token", Cursor: testSelection(),
		},
	})
	if err != nil {
		t.Fatalf("RunBidirectional() error = %v", err)
	}
	terminal := ""
	for _, event := range events {
		if event.Kind == inference.EventTerminal && event.Terminal != nil {
			terminal = event.Terminal.Reason
		}
		if event.Kind == inference.EventToolCallStart {
			t.Fatalf("call = %+v, want no daemon-side tool call", event.ToolCall)
		}
	}
	if terminal != inference.EventReasonStop {
		t.Fatalf("terminal = %q, want stop", terminal)
	}
}

func mustParseForTest(t *testing.T, raw []byte) []adapterscursor.Field {
	t.Helper()
	fields, err := adapterscursor.Parse(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return fields
}

func TestRunToolHandoff(t *testing.T) {
	server := newFixtureServer(t, func(s *fixtureServer, w http.ResponseWriter, reader *adapterscursor.FrameReader) {
		_ = parseClientMessage(t, s.next(reader))
		s.write(w, serverExecMessage(1, "exec-1", 10))
		_ = parseClientMessage(t, s.next(reader))
		s.write(w, mcpArgsPayload("relo_read", "call-1"))
		reply := parseClientMessage(t, s.next(reader))
		if len(reply) != 1 || reply[0].Number != clientExecMessage {
			t.Fatalf("reply = %+v, want the handoff answer", reply)
		}
		assertHandoff(t, reply[0].Raw)
		s.write(w, serverUpdate(14, turnEndedPayload()))
		for {
			_, ended, err := reader.Next()
			if ended || err != nil {
				return
			}
		}
	})
	request := testRequest()
	request.Tools = []inference.Tool{{Name: "read", Description: "read a file", Parameters: []byte(`{"type":"object"}`)}}
	codec := NewCodec(server.server.URL)
	var streamed [][]inference.Event
	events, err := codec.RunBidirectional(context.Background(), wire.BidirectionalCall{
		Client: h2Client(server.server.Client()), Request: request, Opts: wire.CodecOpts{
			BaseURL: server.server.URL, CredentialRef: "token", Cursor: testSelection(),
		},
		Emit: func(batch []inference.Event) { streamed = append(streamed, batch) },
	})
	if err != nil {
		t.Fatalf("RunBidirectional() error = %v", err)
	}
	if len(streamed) == 0 {
		t.Fatal("no streaming batches emitted")
	}
	var call *inference.ToolCallDelta
	terminal := ""
	for _, event := range events {
		if event.Kind == inference.EventToolCallStart {
			call = event.ToolCall
		}
		if event.Kind == inference.EventTerminal && event.Terminal != nil {
			terminal = event.Terminal.Reason
		}
	}
	if call == nil || call.Name != "read" {
		t.Fatalf("call = %+v, want the canonical tool name", call)
	}
	if terminal != inference.EventReasonToolUse {
		t.Fatalf("terminal = %q, want tool_use", terminal)
	}
}
