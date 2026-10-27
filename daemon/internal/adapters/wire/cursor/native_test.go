package cursor

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	adapterscursor "github.com/jonaskahn/relo/internal/adapters/cursor"
	"github.com/jonaskahn/relo/internal/adapters/wire"
	"github.com/jonaskahn/relo/internal/inference"
)

// nativeExec frames one ExecServerMessage carrying the given variant.
func nativeExec(numeric uint32, execID string, variant int, args []byte) []byte {
	var message adapterscursor.Writer
	message.Uint32(1, numeric)
	message.String(15, execID)
	message.Message(variant, args)
	var envelope adapterscursor.Writer
	envelope.Message(2, message.BytesOut())
	return envelope.BytesOut()
}

func shellArgsFrame(command, cwd, callID string) []byte {
	var args adapterscursor.Writer
	args.String(shellCommand, command)
	args.String(shellWorkingDir, cwd)
	args.String(shellCallID, callID)
	return args.BytesOut()
}

func readArgsFrame(path, callID string) []byte {
	var args adapterscursor.Writer
	args.String(readPath, path)
	args.String(readCallID, callID)
	return args.BytesOut()
}

func declaredTools(t *testing.T, names ...string) []inference.Tool {
	t.Helper()
	tools := make([]inference.Tool, 0, len(names))
	for _, name := range names {
		tools = append(tools, inference.Tool{Name: name, Description: name, Parameters: []byte(`{"type":"object"}`)})
	}
	return tools
}

type runResult struct {
	events  []inference.Event
	streams [][]inference.Event
	err     error
}

func runScripted(t *testing.T, request *inference.Request, script func(s *fixtureServer, w http.ResponseWriter, reader *adapterscursor.FrameReader)) runResult {
	t.Helper()
	done := make(chan struct{})
	server := newFixtureServer(t, func(s *fixtureServer, w http.ResponseWriter, reader *adapterscursor.FrameReader) {
		defer close(done)
		script(s, w, reader)
	})
	codec := NewCodec(server.server.URL)
	result := runResult{}
	result.events, result.err = codec.RunBidirectional(context.Background(), wire.BidirectionalCall{
		Client: h2Client(server.server.Client()), Request: request, Opts: wire.CodecOpts{
			BaseURL: server.server.URL, CredentialRef: "token", Cursor: testSelection(),
		},
		Emit: func(batch []inference.Event) { result.streams = append(result.streams, batch) },
	})
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the scripted turn did not finish")
	}
	return result
}

func terminalReason(events []inference.Event) string {
	reason := ""
	for _, event := range events {
		if event.Kind == inference.EventTerminal && event.Terminal != nil {
			reason = event.Terminal.Reason
		}
	}
	return reason
}

func toolCalls(events []inference.Event) []*inference.ToolCallDelta {
	var calls []*inference.ToolCallDelta
	for _, event := range events {
		if event.Kind == inference.EventToolCallStart && event.ToolCall != nil {
			calls = append(calls, event.ToolCall)
		}
	}
	return calls
}

// TestRunNativeShellBecomesBash pins the failure this bridge exists for: a
// native shell execution becomes the declared bash tool call, the daemon
// never runs it, and the turn ends with tool_use rather than an error.
func TestRunNativeShellBecomesBash(t *testing.T) {
	var reply []adapterscursor.Field
	result := runScripted(t, &inference.Request{
		Model: "sonnet", Tools: declaredTools(t, "bash"),
		Messages: []inference.Message{{Role: inference.RoleUser, Content: []inference.ContentPart{{Type: inference.ContentTypeText, Text: "list files"}}}},
	}, func(s *fixtureServer, w http.ResponseWriter, reader *adapterscursor.FrameReader) {
		_ = parseClientMessage(t, s.next(reader))
		s.write(w, serverExecMessage(1, "exec-context", execContext))
		_ = parseClientMessage(t, s.next(reader))
		s.write(w, nativeExec(2, "exec-shell", execShell, shellArgsFrame("ls -la", "/tmp", "call-1")))
		reply = parseClientMessage(t, s.next(reader))
	})
	if result.err != nil {
		t.Fatalf("RunBidirectional() error = %v, want a handoff, not a failure", result.err)
	}
	calls := toolCalls(result.events)
	if len(calls) != 1 || calls[0].Name != "bash" {
		t.Fatalf("calls = %+v, want one bash call", calls)
	}
	if calls[0].ID != "call-1" {
		t.Fatalf("call id = %q, want the native tool call id", calls[0].ID)
	}
	args := toolArguments(t, result.events, "call-1")
	if args["command"] != "ls -la" || args["cwd"] != "/tmp" {
		t.Fatalf("arguments = %v, want the command and cwd", args)
	}
	if reason := terminalReason(result.events); reason != inference.EventReasonToolUse {
		t.Fatalf("terminal = %q, want tool_use", reason)
	}
	if !replyCarriesShellHandoff(t, reply) {
		t.Fatalf("reply = %+v, want the handoff answer the server is waiting on", reply)
	}
}

// TestRunNativeReadBecomesRead covers the second high-traffic native frame.
func TestRunNativeReadBecomesRead(t *testing.T) {
	result := runScripted(t, &inference.Request{
		Model: "sonnet", Tools: declaredTools(t, "read"),
		Messages: []inference.Message{{Role: inference.RoleUser, Content: []inference.ContentPart{{Type: inference.ContentTypeText, Text: "read it"}}}},
	}, func(s *fixtureServer, w http.ResponseWriter, reader *adapterscursor.FrameReader) {
		_ = parseClientMessage(t, s.next(reader))
		s.write(w, nativeExec(2, "exec-read", execRead, readArgsFrame("/tmp/a.go", "call-read")))
		_ = parseClientMessage(t, s.next(reader))
	})
	if result.err != nil {
		t.Fatalf("RunBidirectional() error = %v", result.err)
	}
	calls := toolCalls(result.events)
	if len(calls) != 1 || calls[0].Name != "read" || calls[0].ID != "call-read" {
		t.Fatalf("calls = %+v, want one read call", calls)
	}
	args := toolArguments(t, result.events, "call-read")
	if args["path"] != "/tmp/a.go" {
		t.Fatalf("arguments = %v, want the path", args)
	}
	if reason := terminalReason(result.events); reason != inference.EventReasonToolUse {
		t.Fatalf("terminal = %q, want tool_use", reason)
	}
}

// TestRunUndeclaredNativeToolIsRejected keeps the old typed refusal for a
// native execution the caller declared no tool for.
func TestRunUndeclaredNativeToolIsRejected(t *testing.T) {
	var reply []adapterscursor.Field
	result := runScripted(t, testRequest(), func(s *fixtureServer, w http.ResponseWriter, reader *adapterscursor.FrameReader) {
		_ = parseClientMessage(t, s.next(reader))
		s.write(w, nativeExec(3, "exec-shell", execShell, shellArgsFrame("rm -rf /", "/", "call-x")))
		reply = parseClientMessage(t, s.next(reader))
		s.write(w, serverUpdate(14, turnEndedPayload()))
		for {
			_, ended, err := reader.Next()
			if ended || err != nil {
				return
			}
		}
	})
	if result.err != nil {
		t.Fatalf("RunBidirectional() error = %v", result.err)
	}
	if len(toolCalls(result.events)) != 0 {
		t.Fatalf("events = %+v, want no tool call for an undeclared native tool", result.events)
	}
	if reason := terminalReason(result.events); reason != inference.EventReasonStop {
		t.Fatalf("terminal = %q, want stop", reason)
	}
	if !replyCarriesShellRejection(t, reply) {
		t.Fatalf("reply = %+v, want the typed native rejection", reply)
	}
}

// TestRunNativeUpdateAndExecEmitOneCall covers Cursor announcing the call as
// an update and then sending the execution: the client sees one call.
func TestRunNativeUpdateAndExecEmitOneCall(t *testing.T) {
	result := runScripted(t, &inference.Request{
		Model: "sonnet", Tools: declaredTools(t, "bash"),
		Messages: []inference.Message{{Role: inference.RoleUser, Content: []inference.ContentPart{{Type: inference.ContentTypeText, Text: "run"}}}},
	}, func(s *fixtureServer, w http.ResponseWriter, reader *adapterscursor.FrameReader) {
		_ = parseClientMessage(t, s.next(reader))
		s.write(w, nativeToolStartedPayload("call-1", shellArgsFrame("echo hi", "", "call-1")))
		s.write(w, nativeExec(4, "exec-shell", execShell, shellArgsFrame("echo hi", "", "call-1")))
		_ = parseClientMessage(t, s.next(reader))
		for {
			_, ended, err := reader.Next()
			if ended || err != nil {
				return
			}
		}
	})
	if result.err != nil {
		t.Fatalf("RunBidirectional() error = %v", result.err)
	}
	if calls := toolCalls(result.events); len(calls) != 1 {
		t.Fatalf("calls = %+v, want exactly one call for the update and its exec", calls)
	}
}

// TestRunClosesStreamAfterHandoff pins the stream release: the request stream
// ends as soon as the handoff is answered, so the next request is not queued
// behind a turn Relo will never finish.
func TestRunClosesStreamAfterHandoff(t *testing.T) {
	released := make(chan struct{})
	server := startStreamServer(t, func(w http.ResponseWriter, r *http.Request) {
		reader := beginTurn(w, r)
		flushFrame(w, nativeExec(5, "exec-shell", execShell, shellArgsFrame("pwd", "/", "call-1")))
		if _, _, err := reader.Next(); err != nil {
			return
		}
		select {
		case <-r.Context().Done():
			close(released)
		case <-time.After(3 * time.Second):
		}
	})
	runner := newStreamRunner(server)
	runner.Request = &inference.Request{
		Model: "sonnet", Tools: declaredTools(t, "bash"),
		Messages: []inference.Message{{Role: inference.RoleUser, Content: []inference.ContentPart{{Type: inference.ContentTypeText, Text: "go"}}}},
	}
	events, err := runner.Run(context.Background())
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if reason := terminalReason(events); reason != inference.EventReasonToolUse {
		t.Fatalf("terminal = %q, want tool_use", reason)
	}
	select {
	case <-released:
	case <-time.After(3 * time.Second):
		t.Fatal("the stream stayed open after the handoff")
	}
}

// TestHandoffRotatesTheWireConversation covers the follow-up turn: the client
// conversation keeps its anchor while the handoff callback fires, which is
// what moves the next request onto a fresh wire conversation.
func TestHandoffRotatesTheWireConversation(t *testing.T) {
	server := newFixtureServer(t, func(s *fixtureServer, w http.ResponseWriter, reader *adapterscursor.FrameReader) {
		_ = parseClientMessage(t, s.next(reader))
		s.write(w, nativeExec(6, "exec-shell", execShell, shellArgsFrame("pwd", "/", "call-1")))
		_ = parseClientMessage(t, s.next(reader))
	})
	codec := NewCodec(server.server.URL)
	handoffs := 0
	_, err := codec.RunBidirectional(context.Background(), wire.BidirectionalCall{
		Client: h2Client(server.server.Client()), Request: &inference.Request{
			Model: "sonnet", Tools: declaredTools(t, "bash"),
			Messages: []inference.Message{{Role: inference.RoleUser, Content: []inference.ContentPart{{Type: inference.ContentTypeText, Text: "go"}}}},
		}, Opts: wire.CodecOpts{
			BaseURL: server.server.URL, CredentialRef: "token", Cursor: testSelection(),
		},
		Handoff: func() { handoffs++ },
	})
	if err != nil {
		t.Fatalf("RunBidirectional() error = %v", err)
	}
	if handoffs != 1 {
		t.Fatalf("handoffs = %d, want the rotation signalled once", handoffs)
	}
}

func toolArguments(t *testing.T, events []inference.Event, callID string) map[string]any {
	t.Helper()
	arguments := ""
	for _, event := range events {
		if event.Kind == inference.EventToolCallDelta && event.ToolCall != nil && event.ToolCall.ID == callID {
			arguments += event.ToolCall.Arguments
		}
	}
	decoded := map[string]any{}
	if err := json.Unmarshal([]byte(arguments), &decoded); err != nil {
		t.Fatalf("decode arguments %q: %v", arguments, err)
	}
	return decoded
}

func replyCarriesShellHandoff(t *testing.T, reply []adapterscursor.Field) bool {
	t.Helper()
	if len(reply) != 1 || reply[0].Number != clientExecMessage {
		return false
	}
	fields := mustParseForTest(t, reply[0].Raw)
	results, err := adapterscursor.Nested(fields, resultShell)
	if err != nil || len(results) != 1 {
		return false
	}
	rejected, err := adapterscursor.Nested(results[0], rejectedNoShell)
	if err != nil || len(rejected) != 1 {
		return false
	}
	return adapterscursor.FirstString(rejected[0], 3) == HandoffText
}

func replyCarriesShellRejection(t *testing.T, reply []adapterscursor.Field) bool {
	t.Helper()
	if len(reply) != 1 || reply[0].Number != clientExecMessage {
		return false
	}
	fields := mustParseForTest(t, reply[0].Raw)
	results, err := adapterscursor.Nested(fields, resultShell)
	if err != nil || len(results) != 1 {
		return false
	}
	rejected, err := adapterscursor.Nested(results[0], rejectedNoShell)
	if err != nil || len(rejected) != 1 {
		return false
	}
	return adapterscursor.FirstString(rejected[0], 3) == RejectionReason
}

// nativeToolStartedPayload frames a ToolCallStartedUpdate carrying a native
// shell call, the shape Cursor sends before the execution frame.
func nativeToolStartedPayload(callID string, args []byte) []byte {
	var call adapterscursor.Writer
	call.Message(callShell, args)
	call.String(callIDField, callID)
	var started adapterscursor.Writer
	started.String(toolStartedID, callID)
	started.Message(toolStartedCall, call.BytesOut())
	return serverUpdate(updateToolStarted, started.BytesOut())
}

// TestNativeTranslationFollowsTheDeclaredTool pins the argument spelling each
// declaration expects, since a shell tool rejects cwd and a bash tool rejects
// workdir.
func TestNativeTranslationFollowsTheDeclaredTool(t *testing.T) {
	fields, err := adapterscursor.Parse(shellArgsFrame("make test", "/src", "call-1"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	call, ok := translateNativeArgs(execShell, fields, "exec-1")
	if !ok {
		t.Fatal("translateNativeArgs() refused a shell frame")
	}
	if args := call.arguments("shell"); args["workdir"] != "/src" || args["cwd"] != nil {
		t.Fatalf("shell arguments = %v, want workdir", args)
	}
	if args := call.arguments("bash"); args["cwd"] != "/src" || args["workdir"] != nil {
		t.Fatalf("bash arguments = %v, want cwd", args)
	}
}

func TestNativeUnknownVariantIsNotTranslated(t *testing.T) {
	if _, ok := translateNativeArgs(99, nil, "exec-1"); ok {
		t.Fatal("translateNativeArgs() accepted an unknown variant")
	}
}

// TestHistoryResumesAHandedOffToolResult covers the request that follows a
// handoff: the assistant call and its result replay as one MCP-style pair and
// the turn resumes from the result instead of reissuing the native call.
func TestHistoryResumesAHandedOffToolResult(t *testing.T) {
	request := &inference.Request{
		Model: "sonnet", Tools: declaredTools(t, "bash"),
		Messages: []inference.Message{
			{Role: inference.RoleUser, Content: []inference.ContentPart{{Type: inference.ContentTypeText, Text: "list files"}}},
			{Role: inference.RoleAssistant, ToolCalls: []inference.ToolCall{{ID: "call-1", Name: "bash", Arguments: `{"command":"ls"}`}}},
			{Role: inference.RoleTool, ToolCallID: "call-1", Name: "bash", Content: []inference.ContentPart{{Type: inference.ContentTypeText, Text: "a.go"}}},
		},
	}
	tools, err := BuildToolMap(request.Tools)
	if err != nil {
		t.Fatalf("BuildToolMap() error = %v", err)
	}
	history, action, err := BuildHistory(request, tools)
	if err != nil {
		t.Fatalf("BuildHistory() error = %v", err)
	}
	if fields := mustParseForTest(t, action); len(fields) != 1 || fields[0].Number != actionResume {
		t.Fatalf("action = %+v, want a resume over the tool result", fields)
	}
	if !historyHasToolResult(t, history, mustParseForTest(t, history.State)) {
		t.Fatal("the resumed state carries no tool result")
	}
	if !historyHasToolCall(t, history, mustParseForTest(t, history.State)) {
		t.Fatal("the resumed state carries no tool call")
	}
}

// historyHasToolCall reports a root-prompt entry naming the handed-off call.
func historyHasToolCall(t *testing.T, history History, state []adapterscursor.Field) bool {
	t.Helper()
	for _, field := range state {
		if field.Number != stateRootPrompts || field.Wire != 2 {
			continue
		}
		blob, found := history.Store.Get(field.Raw)
		if !found {
			t.Fatalf("blob %x missing", field.Raw)
		}
		if strings.Contains(string(blob), `"toolCallId":"call-1"`) {
			return true
		}
	}
	return false
}
