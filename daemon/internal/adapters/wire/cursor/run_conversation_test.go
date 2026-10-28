package cursor

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	adapterscursor "github.com/jonaskahn/relo/internal/adapters/cursor"
	"github.com/jonaskahn/relo/internal/adapters/wire"
	"github.com/jonaskahn/relo/internal/inference"
)

// TestRunRefusesBeforeOpeningAStream covers the request Cursor cannot carry:
// nothing is sent, so the failure is the codec's own verdict.
func TestRunRefusesBeforeOpeningAStream(t *testing.T) {
	runner := &Runner{Client: &http.Client{}, BaseURL: "https://cursor.test", Token: "token", Selection: testSelection()}
	temperature := 0.5
	runner.Request = testRequest()
	runner.Request.Temperature = &temperature
	if _, err := runner.Run(context.Background()); !errors.Is(err, wire.ErrUnsupportedFeature) {
		t.Fatalf("Run() error = %v, want the unsupported-feature refusal", err)
	}
}

func TestAwaitStreamEndWithoutAReaderReturns(t *testing.T) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		awaitStreamEnd(nil, time.Second)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("awaitStreamEnd(nil) did not return")
	}
}

// TestHeartbeatStopsAtTheFirstWriteFailure covers the ping loop against a
// stream that already closed: the loop ends instead of retrying a dead pipe.
func TestHeartbeatStopsAtTheFirstWriteFailure(t *testing.T) {
	runner := &Runner{HeartbeatEvery: time.Millisecond}
	var writes atomic.Int64
	stop := runner.heartbeat(func([]byte) error {
		writes.Add(1)
		return errors.New("the stream is closed")
	})
	defer stop()
	deadline := time.Now().Add(time.Second)
	for writes.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if got := writes.Load(); got != 1 {
		t.Fatalf("writes = %d, want the loop to stop at the first failure", got)
	}
}

// TestHeartbeatStopsWhenAsked covers a ping loop whose stream is still live:
// stopping it ends the loop, and no ping follows.
func TestHeartbeatStopsWhenAsked(t *testing.T) {
	// An interval this long never fires, so the only way the loop can end is
	// the stop it is given.
	runner := &Runner{HeartbeatEvery: time.Hour}
	var writes atomic.Int64
	runner.heartbeat(func([]byte) error {
		writes.Add(1)
		return nil
	})()
	time.Sleep(20 * time.Millisecond)
	if writes.Load() != 0 {
		t.Fatalf("writes = %d, want none without a tick", writes.Load())
	}
}

// nativeToolCallFrame frames one ToolCall carrying a native variant, the shape
// an update nests: the variant field's own payload is the variant's arguments.
func nativeToolCallFrame(callID string, variant int, args []byte) []byte {
	var call adapterscursor.Writer
	call.Bytes(variant, args)
	call.String(callIDField, callID)
	return call.BytesOut()
}

// mcpToolCallFrame frames one ToolCall carrying a declared MCP call.
func mcpToolCallFrame(callID, name string) []byte {
	var args adapterscursor.Writer
	args.String(mcpArgName, name)
	args.String(mcpArgToolName, name)
	args.String(mcpArgProvider, providerIdentifier)
	var mcp adapterscursor.Writer
	mcp.Bytes(mcpArgsField, args.BytesOut())
	var call adapterscursor.Writer
	call.Bytes(toolCallMCP, mcp.BytesOut())
	call.String(callIDField, callID)
	return call.BytesOut()
}

// TestNativeToolUpdateIsPublishedOnce covers the update Cursor sends before
// the execution frame: it becomes one canonical call, and a repeat of the same
// update adds nothing.
func TestNativeToolUpdateIsPublishedOnce(t *testing.T) {
	conversation := newConversation(mustBuildTools(t, "bash"), History{Store: NewStore()}, nil)
	payload := nativeToolCallFrame("call-1", callShell, shellArgsFrame("echo hi", "/src", "call-1"))
	for round := 0; round < 2; round++ {
		if err := conversation.handleNativeToolUpdate(payload, "call-1"); err != nil {
			t.Fatalf("handleNativeToolUpdate() error = %v", err)
		}
	}
	calls := toolCalls(conversation.events)
	if len(calls) != 1 || calls[0].Name != "bash" || calls[0].ID != "call-1" {
		t.Fatalf("calls = %+v, want one bash call", calls)
	}
	args := toolArguments(t, conversation.events, "call-1")
	if args["command"] != "echo hi" || args["cwd"] != "/src" {
		t.Fatalf("arguments = %v, want the command and cwd", args)
	}
	if !conversation.toolUse {
		t.Fatal("a native call did not mark the turn as tool use")
	}
}

// TestNativeToolUpdateWithoutADeclaredToolIsIgnored covers the update for a
// native call the caller declared no tool for: the execution frame that
// follows is what refuses it, so the announcement publishes nothing.
func TestNativeToolUpdateWithoutADeclaredToolIsIgnored(t *testing.T) {
	conversation := newConversation(mustBuildTools(t), History{}, nil)
	payload := nativeToolCallFrame("call-1", callShell, shellArgsFrame("rm -rf /", "/", "call-1"))
	if err := conversation.handleNativeToolUpdate(payload, "call-1"); err != nil {
		t.Fatalf("handleNativeToolUpdate() error = %v", err)
	}
	if len(conversation.events) != 0 {
		t.Fatalf("events = %+v, want nothing for an undeclared native tool", conversation.events)
	}
}

// TestNativeToolUpdateFallbacks covers the shape that carries no usable ids:
// an unreadable payload is an error, and a call with no id is named after its
// position in the turn.
func TestNativeToolUpdateFallbacks(t *testing.T) {
	conversation := newConversation(mustBuildTools(t, "bash"), History{}, nil)
	unreadable := nativeToolCallFrame("call-1", callShell, []byte{0x08})
	if err := conversation.handleNativeToolUpdate(unreadable, "call-1"); err == nil {
		t.Fatal("handleNativeToolUpdate() accepted an unreadable payload")
	}

	anonymous := nativeToolCallFrame("", callShell, shellArgsFrame("pwd", "", ""))
	if err := conversation.handleNativeToolUpdate(anonymous, ""); err != nil {
		t.Fatalf("handleNativeToolUpdate() error = %v", err)
	}
	calls := toolCalls(conversation.events)
	if len(calls) != 1 || calls[0].ID != "native-0" {
		t.Fatalf("calls = %+v, want the positional id", calls)
	}
}

// TestNativeToolUpdateRefusesAnUnreadableMCPCall covers the update whose MCP
// payload does not parse: the turn fails rather than guessing at the call.
func TestNativeToolUpdateRefusesAnUnreadableMCPCall(t *testing.T) {
	conversation := newConversation(mustBuildTools(t, "bash"), History{}, nil)
	var call adapterscursor.Writer
	call.String(toolCallIDField, "call-1")
	call.Bytes(toolCallMCP, []byte{0x08})
	var started adapterscursor.Writer
	started.String(toolStartedID, "call-1")
	started.Message(toolStartedCall, call.BytesOut())
	err := conversation.handleToolUpdate(ServerMessage{UpdateKind: updateToolStarted, Update: mustParseForTest(t, started.BytesOut())})
	if err == nil {
		t.Fatal("handleToolUpdate() accepted an unreadable MCP call")
	}
}

// TestNativeExecRefusesAVariantItCannotAnswer covers the guard behind the
// translation: a call the rejection table has no answer for fails the turn
// instead of leaving the server waiting.
func TestNativeExecRefusesAVariantItCannotAnswer(t *testing.T) {
	conversation := newConversation(mustBuildTools(t, "bash"), History{}, nil)
	err := conversation.handleNativeExec(nativeCall{variant: 99, callID: "call-1"}, 1, "exec-1")
	if !errors.Is(err, ErrUnsupportedExec) {
		t.Fatalf("handleNativeExec() error = %v, want ErrUnsupportedExec", err)
	}
}

// TestPartialToolCallPublishesItsArguments covers the partial update Cursor
// streams for a declared call: the arguments arrive as a delta and the turn
// counts as tool use.
func TestPartialToolCallPublishesItsArguments(t *testing.T) {
	conversation := newConversation(mustBuildTools(t, "bash"), History{}, nil)
	var partial adapterscursor.Writer
	partial.String(partialID, "call-1")
	partial.Bytes(partialCall, mcpToolCallFrame("call-1", WireName("bash")))
	partial.String(partialArgsDelta, `{"command":"ls"}`)
	message := ServerMessage{UpdateKind: updatePartialToolCall, Update: mustParseForTest(t, partial.BytesOut())}
	if err := conversation.handlePartialUpdate(message); err != nil {
		t.Fatalf("handlePartialUpdate() error = %v", err)
	}
	calls := toolCalls(conversation.events)
	if len(calls) != 1 || calls[0].Name != "bash" || calls[0].ID != "call-1" {
		t.Fatalf("calls = %+v, want the partial call", calls)
	}
	if args := toolArguments(t, conversation.events, "call-1"); args["command"] != "ls" {
		t.Fatalf("arguments = %v, want the streamed delta", args)
	}
}

// TestPartialToolCallWithoutANameIsIgnored covers the partial update that
// names no tool: there is nothing to publish yet.
func TestPartialToolCallWithoutANameIsIgnored(t *testing.T) {
	conversation := newConversation(mustBuildTools(t, "bash"), History{}, nil)
	var partial adapterscursor.Writer
	partial.String(partialID, "call-1")
	partial.String(partialArgsDelta, `{"command":"ls"}`)
	message := ServerMessage{UpdateKind: updatePartialToolCall, Update: mustParseForTest(t, partial.BytesOut())}
	if err := conversation.handlePartialUpdate(message); err != nil {
		t.Fatalf("handlePartialUpdate() error = %v", err)
	}
	if len(conversation.events) != 0 {
		t.Fatalf("events = %+v, want nothing for an unnamed call", conversation.events)
	}
}

// TestBlobStoreRoundTrip covers the blob channel: a stored blob is answered
// back byte for byte, and an unknown id answers empty.
func TestBlobStoreRoundTrip(t *testing.T) {
	conversation := newConversation(mustBuildTools(t), History{Store: NewStore()}, nil)
	var written [][]byte
	conversation.write = func(payload []byte) error {
		written = append(written, payload)
		return nil
	}
	var set adapterscursor.Writer
	set.Bytes(blobIDField, []byte("blob-1"))
	set.Bytes(2, []byte("state-bytes"))
	if err := conversation.handleKV(ServerMessage{KVKind: kvSetArgs, KVID: 3, KV: mustParseForTest(t, set.BytesOut())}); err != nil {
		t.Fatalf("handleKV(set) error = %v", err)
	}
	var get adapterscursor.Writer
	get.Bytes(blobIDField, []byte("blob-1"))
	if err := conversation.handleKV(ServerMessage{KVKind: kvGetArgs, KVID: 4, KV: mustParseForTest(t, get.BytesOut())}); err != nil {
		t.Fatalf("handleKV(get) error = %v", err)
	}
	var missing adapterscursor.Writer
	missing.Bytes(blobIDField, []byte("blob-2"))
	if err := conversation.handleKV(ServerMessage{KVKind: kvGetArgs, KVID: 5, KV: mustParseForTest(t, missing.BytesOut())}); err != nil {
		t.Fatalf("handleKV(missing) error = %v", err)
	}
	if len(written) != 3 {
		t.Fatalf("wrote %d answers, want the store, the blob, and the empty answer", len(written))
	}
	if blob := blobFor(mustParseForTest(t, get.BytesOut()), conversation.history.Store); string(blob) != "state-bytes" {
		t.Fatalf("blob = %q, want the stored bytes", blob)
	}
	if blob := blobFor(mustParseForTest(t, missing.BytesOut()), conversation.history.Store); blob != nil {
		t.Fatalf("blob = %q, want nothing for an unknown id", blob)
	}
	if id := blobIDOf(mustParseForTest(t, get.BytesOut())); string(id) != "blob-1" {
		t.Fatalf("id = %q, want the requested blob", id)
	}
	var empty adapterscursor.Writer
	if id := blobIDOf(mustParseForTest(t, empty.BytesOut())); id != nil {
		t.Fatalf("id = %q, want nothing without the field", id)
	}
}

// TestKVMessageWithoutAVariantIsIgnored covers the KV frame Relo has no answer
// for: the turn continues.
func TestKVMessageWithoutAVariantIsIgnored(t *testing.T) {
	conversation := newConversation(mustBuildTools(t), History{}, nil)
	if err := conversation.handleKV(ServerMessage{KVKind: 99}); err != nil {
		t.Fatalf("handleKV() error = %v", err)
	}
}

// execResult unwraps one client execution answer down to the named result
// case.
func execResult(t *testing.T, payload []byte, field int) ([][]byte, error) {
	t.Helper()
	client := mustParseForTest(t, payload)
	execs, err := adapterscursor.Nested(client, clientExecMessage)
	if err != nil || len(execs) != 1 {
		return nil, err
	}
	results := make([][]byte, 0, 1)
	for _, result := range execs[0] {
		if result.Number != field || result.Wire != 2 {
			continue
		}
		results = append(results, result.Raw)
	}
	return results, nil
}

// mustBuildTools declares the named tools, failing the test when the map
// cannot be built.
func mustBuildTools(t *testing.T, names ...string) *ToolMap {
	t.Helper()
	tools, err := BuildToolMap(declaredTools(t, names...))
	if err != nil {
		t.Fatalf("BuildToolMap() error = %v", err)
	}
	return tools
}

// TestConversationAnswersAnUnknownTool covers a declared-tool call the client
// never offered: the answer names the tools the attempt declared.
func TestConversationAnswersAnUnknownTool(t *testing.T) {
	conversation := newConversation(mustBuildTools(t, "bash"), History{Store: NewStore()}, nil)
	var written [][]byte
	conversation.write = func(payload []byte) error {
		written = append(written, payload)
		return nil
	}
	call := MCPCall{Name: "relo_deploy", ToolCallID: "call-1"}
	if err := conversation.emitMCPCall(call, 7, "exec-7", true); err != nil {
		t.Fatalf("emitMCPCall() error = %v", err)
	}
	if len(written) != 1 {
		t.Fatalf("wrote %d answers, want the unknown-tool refusal", len(written))
	}
	results, err := execResult(t, written[0], resultMCP)
	if err != nil || len(results) != 1 {
		t.Fatalf("answer = %v %v, want one MCP result", results, err)
	}
	notFound, err := adapterscursor.Nested(mustParseForTest(t, results[0]), 5)
	if err != nil || len(notFound) != 1 {
		t.Fatalf("result = %v %v, want the unknown-tool case", notFound, err)
	}
	if name := adapterscursor.FirstString(notFound[0], 1); name != "relo_deploy" {
		t.Fatalf("name = %q, want the refused tool", name)
	}
	if available := adapterscursor.Strings(notFound[0], 2); len(available) != 1 || available[0] != "relo_bash" {
		t.Fatalf("available = %v, want the declared wire name", available)
	}
}

// TestConversationAnswersAnUnknownToolWithoutReplying covers the same refusal
// on an update-observed call: nothing is written, because no execution is
// waiting on it.
func TestConversationAnswersAnUnknownToolWithoutReplying(t *testing.T) {
	conversation := newConversation(mustBuildTools(t, "bash"), History{}, nil)
	wrote := false
	conversation.write = func([]byte) error {
		wrote = true
		return nil
	}
	if err := conversation.emitMCPCall(MCPCall{Name: "relo_deploy", ToolCallID: "call-1"}, 7, "exec-7", false); err != nil {
		t.Fatalf("emitMCPCall() error = %v", err)
	}
	if wrote {
		t.Fatal("an update-observed call answered on the exec channel")
	}
}

// TestConversationAnswerFailureIsReported covers a reply the stream refused:
// the turn reports the write instead of assuming the server heard it.
func TestConversationAnswerFailureIsReported(t *testing.T) {
	conversation := newConversation(mustBuildTools(t, "bash"), History{}, nil)
	conversation.write = func([]byte) error { return errors.New("the stream is closed") }
	if err := conversation.reply(buildMCPHandoff()); err == nil {
		t.Fatal("reply() hid a failed write")
	}
	silent := newConversation(mustBuildTools(t, "bash"), History{}, nil)
	if err := silent.reply(buildMCPHandoff()); err != nil {
		t.Fatalf("reply() without a writer error = %v", err)
	}
}

// TestTurnHandlesAnAbortingControl covers the control frame Cursor sends to
// stop a turn: the run reports it rather than treating it as an empty answer.
func TestTurnHandlesAnAbortingControl(t *testing.T) {
	conversation := newConversation(mustBuildTools(t), History{}, nil)
	var envelope adapterscursor.Writer
	envelope.Empty(serverControl)
	if err := conversation.handle(envelope.BytesOut()); err == nil {
		t.Fatal("handle() accepted a control frame")
	}
}

// TestTurnIgnoresACheckpoint covers the checkpoint that carries no answer.
func TestTurnIgnoresACheckpoint(t *testing.T) {
	conversation := newConversation(mustBuildTools(t), History{}, nil)
	var envelope adapterscursor.Writer
	envelope.Empty(serverCheckpoint)
	if err := conversation.handle(envelope.BytesOut()); err != nil {
		t.Fatalf("handle() error = %v, want a checkpoint ignored", err)
	}
	if len(conversation.events) != 0 {
		t.Fatalf("events = %+v, want nothing for a checkpoint", conversation.events)
	}
}

// TestTurnRefusesAnInteraction covers the query Relo cannot answer.
func TestTurnRefusesAnInteraction(t *testing.T) {
	conversation := newConversation(mustBuildTools(t), History{}, nil)
	var envelope adapterscursor.Writer
	envelope.Empty(serverQuery)
	if err := conversation.handle(envelope.BytesOut()); !errors.Is(err, ErrInteraction) {
		t.Fatalf("handle() error = %v, want ErrInteraction", err)
	}
}

// TestTurnRefusesAMalformedFrame covers a server frame that does not parse.
func TestTurnRefusesAMalformedFrame(t *testing.T) {
	conversation := newConversation(mustBuildTools(t), History{}, nil)
	if err := conversation.handle([]byte{0x08}); err == nil {
		t.Fatal("handle() accepted a truncated frame")
	}
}

// TestExecAnswersTheContextAndStateChannels directly pins the two channels
// Cursor opens before any tool runs.
func TestExecAnswersTheContextAndStateChannels(t *testing.T) {
	conversation := newConversation(mustBuildTools(t, "read"), History{Rules: [][]byte{buildRule("/relo/rule.mdc", "be brief")}}, nil)
	var written [][]byte
	conversation.write = func(payload []byte) error {
		written = append(written, payload)
		return nil
	}
	for _, variant := range []int{execContext, execMCPState} {
		message := ServerMessage{ExecKind: variant, ExecNumericID: 3, ExecID: "exec-3"}
		if err := conversation.handleExec(message); err != nil {
			t.Fatalf("handleExec(%d) error = %v", variant, err)
		}
	}
	if len(written) != 2 {
		t.Fatalf("wrote %d answers, want the context and the state", len(written))
	}
	context, err := execResult(t, written[0], resultContext)
	if err != nil || len(context) != 1 {
		t.Fatalf("context answer = %v %v, want one result", context, err)
	}
	if defs := decodedContext(t, context[0]); len(defs) != 1 {
		t.Fatalf("context carries %d tools, want the declared one", len(defs))
	}
	state, err := execResult(t, written[1], resultMCPState)
	if err != nil || len(state) != 1 {
		t.Fatalf("state answer = %v %v, want one result", state, err)
	}
	if defs := decodedMCPState(t, state[0]); len(defs) != 1 {
		t.Fatalf("state carries %d tools, want the declared one", len(defs))
	}
}

// TestExecRefusesAVariantItCannotAnswer covers an execution variant outside
// both the context channels and the native table.
func TestExecRefusesAVariantItCannotAnswer(t *testing.T) {
	conversation := newConversation(mustBuildTools(t), History{}, nil)
	if err := conversation.handleExec(ServerMessage{ExecKind: 99, ExecNumericID: 1, ExecID: "exec-1"}); !errors.Is(err, ErrUnsupportedExec) {
		t.Fatalf("handleExec() error = %v, want ErrUnsupportedExec", err)
	}
}

// TestSkippedUpdateVariantsAreIgnored covers the updates Relo reads nothing
// from: the turn stays alive.
func TestSkippedUpdateVariantsAreIgnored(t *testing.T) {
	conversation := newConversation(mustBuildTools(t), History{}, nil)
	for _, kind := range []int{updateToolDelta, 99} {
		if err := conversation.handleUpdate(ServerMessage{UpdateKind: kind}); err != nil {
			t.Fatalf("handleUpdate(%d) error = %v", kind, err)
		}
	}
	if len(conversation.events) != 0 {
		t.Fatalf("events = %+v, want nothing from an ignored update", conversation.events)
	}
}

// TestThinkingDeltaIsPublished covers the reasoning channel: the model's
// thinking reaches the client apart from its answer.
func TestUsageAndThinkingDeltasArePublished(t *testing.T) {
	conversation := newConversation(mustBuildTools(t), History{}, nil)
	if err := conversation.handleUpdate(ServerMessage{UpdateKind: updateThinkingDelta, Update: mustParseForTest(t, thinkingPayload("thinking"))}); err != nil {
		t.Fatalf("handleUpdate() error = %v", err)
	}
	if len(conversation.events) != 1 || conversation.events[0].Kind != inference.EventReasoningDelta {
		t.Fatalf("events = %+v, want the reasoning delta", conversation.events)
	}
}

func thinkingPayload(text string) []byte {
	var delta adapterscursor.Writer
	delta.String(thinkingText, text)
	return delta.BytesOut()
}

// TestEmptyTextDeltaPublishesNothing covers the update that carries no text.
func TestEmptyTextDeltaPublishesNothing(t *testing.T) {
	conversation := newConversation(mustBuildTools(t), History{}, nil)
	for _, kind := range []int{updateTextDelta, updateThinkingDelta} {
		if err := conversation.handleUpdate(ServerMessage{UpdateKind: kind}); err != nil {
			t.Fatalf("handleUpdate(%d) error = %v", kind, err)
		}
	}
	if len(conversation.events) != 0 {
		t.Fatalf("events = %+v, want nothing", conversation.events)
	}
}

// TestPublishWithoutEventsDoesNothing covers the empty batch.
func TestPublishWithoutEventsDoesNothing(t *testing.T) {
	conversation := newConversation(mustBuildTools(t), History{}, nil)
	conversation.publish()
	if len(conversation.events) != 0 {
		t.Fatalf("events = %+v, want nothing", conversation.events)
	}
}
