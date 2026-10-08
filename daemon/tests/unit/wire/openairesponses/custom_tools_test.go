package openai_responses_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/wire"
	"github.com/jonaskahn/relo/internal/adapters/wire/openaichat"
	"github.com/jonaskahn/relo/internal/adapters/wire/openairesponses"
	"github.com/jonaskahn/relo/internal/inference"
)

// TestInboundDecodesCodexTools replays a request Codex really sent: a freeform
// `exec` tool, two function tools, and the hosted web_search tool. Upstreams
// rejected it because the hosted tool had no name and `exec` had no schema.
func TestInboundDecodesCodexTools(t *testing.T) {
	request := inboundRequest(t, readFixture(t, "openai/codex_request.json"))

	names := make([]string, 0, len(request.Tools))
	for _, tool := range request.Tools {
		if tool.Name == "" {
			t.Fatalf("tools = %+v, want no nameless tool", request.Tools)
		}
		names = append(names, tool.Name)
		var schema map[string]any
		if err := json.Unmarshal(tool.Parameters, &schema); err != nil || schema["type"] != "object" {
			t.Fatalf("%s parameters = %s, want an object schema", tool.Name, tool.Parameters)
		}
	}
	if strings.Join(names, ",") != "exec,wait,request_user_input" {
		t.Fatalf("tool names = %v, want the hosted tool dropped", names)
	}

	var exec struct {
		Properties map[string]map[string]any `json:"properties"`
	}
	if err := json.Unmarshal(request.Tools[0].Parameters, &exec); err != nil {
		t.Fatalf("decode exec schema: %v", err)
	}
	if exec.Properties["input"]["type"] != "string" {
		t.Fatalf("exec schema = %s, want a string input", request.Tools[0].Parameters)
	}
}

func TestInboundGivesAToollessSchemaAnEmptyObject(t *testing.T) {
	request := inboundRequest(t, []byte(`{"model":"gpt-5","input":"hi","tools":[
		{"type":"function","name":"ping","parameters":null},
		{"type":"function","name":"pong"}]}`))
	for _, tool := range request.Tools {
		if string(tool.Parameters) != `{"type":"object","properties":{}}` {
			t.Fatalf("%s parameters = %s, want the empty object schema", tool.Name, tool.Parameters)
		}
	}
}

// TestInboundCustomToolRoundTrip follows one freeform call through a turn: the
// history is decoded as a function call, and the reply comes back as the
// custom_tool_call the client can execute.
func TestInboundCustomToolRoundTrip(t *testing.T) {
	body := `{"model":"gpt-5","stream":true,
		"tools":[{"type":"custom","name":"exec","description":"run"}],
		"input":[
			{"type":"message","role":"user","content":[{"type":"input_text","text":"go"}]},
			{"type":"custom_tool_call","call_id":"call_0","name":"exec","input":"text(1)"},
			{"type":"custom_tool_call_output","call_id":"call_0","output":"1"}]}`
	inbound := openairesponses.NewInbound()
	request, err := decodeInboundWith(inbound, []byte(body))
	if err != nil {
		t.Fatalf("DecodeRequest() error = %v", err)
	}

	calls := toolCalls(request.Messages)
	if len(calls) != 1 || calls[0].Name != "exec" || calls[0].Arguments != `{"input":"text(1)"}` {
		t.Fatalf("tool calls = %+v, want the freeform input wrapped as arguments", calls)
	}
	result := messageWithRole(request.Messages, inference.RoleTool)
	if result == nil || result.ToolCallID != "call_0" || result.Content[0].Text != "1" {
		t.Fatalf("tool message = %+v, want the output bound to its call", result)
	}

	frames := encodeEvents(t, inbound, []inference.Event{
		{Kind: inference.EventToolCallStart, ToolCall: &inference.ToolCallDelta{ID: "call_1", Name: "exec"}},
		{Kind: inference.EventToolCallDelta, ToolCall: &inference.ToolCallDelta{Arguments: `{"input":"te`}},
		{Kind: inference.EventToolCallDelta, ToolCall: &inference.ToolCallDelta{Arguments: `xt(2)"}`}},
		{Kind: inference.EventToolCallEnd, ToolCall: &inference.ToolCallDelta{}},
		{Kind: inference.EventTerminal, Terminal: &inference.TerminalInfo{Reason: inference.EventReasonStop}},
	})
	if countName(frames, "response.function_call_arguments.delta") != 0 ||
		countName(frames, "response.function_call_arguments.done") != 0 {
		t.Fatalf("frames = %v, want no function call frames for a freeform tool", frameNames(frames))
	}
	if countName(frames, "response.custom_tool_call_input.delta") != 1 ||
		countName(frames, "response.custom_tool_call_input.done") != 1 {
		t.Fatalf("frames = %v, want the input sent once, whole", frameNames(frames))
	}
	done := frameNamed(frames, "response.output_item.done")
	for _, want := range []string{`"type":"custom_tool_call"`, `"input":"text(2)"`, `"call_id":"call_1"`, `"name":"exec"`} {
		if !strings.Contains(done, want) {
			t.Fatalf("item = %s, want %s", done, want)
		}
	}
	if last := frames[len(frames)-1]; !strings.Contains(last.Data, `"type":"custom_tool_call"`) {
		t.Fatalf("terminal = %s, want the custom call in the output", last.Data)
	}
}

func frameNamed(frames []wire.SSEEvent, name string) string {
	for _, frame := range frames {
		if frame.Name == name {
			return frame.Data
		}
	}
	return ""
}

func decodeInboundWith(inbound *openairesponses.Inbound, body []byte) (*inference.Request, error) {
	request, err := http.NewRequest(http.MethodPost, "/responses", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	return inbound.DecodeRequest(request)
}

// TestInboundKeepsAResultRightAfterItsCall replays a turn Codex recorded as
// call, text, result. The text belongs to the call's turn, and chat upstreams
// reject a result that does not follow the assistant turn holding the call.
func TestInboundKeepsAResultRightAfterItsCall(t *testing.T) {
	request := inboundRequest(t, []byte(`{"model":"gpt-5","input":[
		{"type":"message","role":"user","content":[{"type":"input_text","text":"go"}]},
		{"type":"custom_tool_call","call_id":"call_0","name":"exec","input":"1"},
		{"type":"message","role":"assistant","content":[{"type":"output_text","text":"done"}]},
		{"type":"custom_tool_call_output","call_id":"call_0","output":"ok"}]}`))

	roles := make([]string, 0, len(request.Messages))
	for _, message := range request.Messages {
		roles = append(roles, message.Role)
	}
	if got := strings.Join(roles, ","); got != "user,assistant,tool" {
		t.Fatalf("roles = %s, want the text and the call on one turn", got)
	}
	assistant := request.Messages[1]
	if len(assistant.ToolCalls) != 1 || len(assistant.Content) != 1 || assistant.Content[0].Text != "done" {
		t.Fatalf("assistant = %+v, want the text and the call together", assistant)
	}
	if request.Messages[2].ToolCallID != "call_0" {
		t.Fatalf("messages = %+v, want the result bound to its call", request.Messages)
	}
}

// TestInboundReadsAResultMadeOfContentItems covers a result with several
// parts, which Codex sends as a list rather than a string.
func TestInboundReadsAResultMadeOfContentItems(t *testing.T) {
	request := inboundRequest(t, []byte(`{"model":"gpt-5","input":[
		{"type":"custom_tool_call","call_id":"call_0","name":"exec","input":"1"},
		{"type":"custom_tool_call_output","call_id":"call_0","output":[
			{"type":"input_text","text":"Script completed\nOutput:\n"},
			{"type":"input_text","text":"25 lines"}]}]}`))

	result := messageWithRole(request.Messages, inference.RoleTool)
	if result == nil || result.Content[0].Text != "Script completed\nOutput:\n25 lines" {
		t.Fatalf("tool message = %+v, want the parts joined", result)
	}
}

// TestInboundCoalescesTheCodexTurn replays the item order Codex records for a
// DeepSeek turn: the tool call, then the reasoning and text of the same turn,
// then the result. Splitting them replays the call without its thinking, which
// ends the DeepSeek tool loop.
func TestInboundCoalescesTheCodexTurn(t *testing.T) {
	t.Run("text and reasoning stay on the call", func(t *testing.T) {
		request := inboundRequest(t, []byte(`{"model":"deepseek-flash",
			"tools":[{"type":"custom","name":"exec","description":"run"}],
			"input":[
				{"type":"message","role":"user","content":[{"type":"input_text","text":"go"}]},
				{"type":"custom_tool_call","call_id":"call_0","name":"exec","input":"1"},
				{"type":"reasoning","id":"rs_1","summary":[{"type":"summary_text","text":"weighing"}]},
				{"type":"message","role":"assistant","content":[{"type":"output_text","text":"answer"}]},
				{"type":"custom_tool_call_output","call_id":"call_0","output":"ok"}]}`))

		if len(request.Messages) != 3 {
			t.Fatalf("messages = %+v, want one assistant turn between the user and the result", request.Messages)
		}
		assistant := request.Messages[1]
		parts := assistant.Content
		if len(parts) != 2 ||
			parts[0].Type != inference.ContentTypeThinking || parts[0].Text != "weighing" ||
			parts[1].Type != inference.ContentTypeText || parts[1].Text != "answer" {
			t.Fatalf("content = %+v, want the reasoning and the text with the call", parts)
		}
		if len(assistant.ToolCalls) != 1 || assistant.ToolCalls[0].Name != "exec" {
			t.Fatalf("tool calls = %+v, want the exec call", assistant.ToolCalls)
		}
		if request.Messages[2].Role != inference.RoleTool || request.Messages[2].ToolCallID != "call_0" {
			t.Fatalf("messages = %+v, want the result right after the turn", request.Messages)
		}

		body := encodeDeepSeekBody(t, request)
		messages := body["messages"].([]any)
		if len(messages) != 3 {
			t.Fatalf("encoded messages = %v, want no second assistant message", messages)
		}
		encoded := messages[1].(map[string]any)
		if encoded["reasoning_content"] != "weighing" || encoded["content"] != "answer" {
			t.Fatalf("assistant = %v, want the reasoning and the text replayed on the call", encoded)
		}
		if calls, _ := encoded["tool_calls"].([]any); len(calls) != 1 {
			t.Fatalf("assistant = %v, want the tool call kept", encoded)
		}
	})

	t.Run("a call without text keeps its reasoning", func(t *testing.T) {
		request := inboundRequest(t, []byte(`{"model":"deepseek-flash",
			"tools":[{"type":"custom","name":"exec","description":"run"}],
			"input":[
				{"type":"message","role":"user","content":[{"type":"input_text","text":"go"}]},
				{"type":"custom_tool_call","call_id":"call_0","name":"exec","input":"1"},
				{"type":"reasoning","id":"rs_1","summary":[{"type":"summary_text","text":"weighing"}]},
				{"type":"custom_tool_call_output","call_id":"call_0","output":"ok"}]}`))

		assistant := request.Messages[1]
		if len(assistant.Content) != 1 ||
			assistant.Content[0].Type != inference.ContentTypeThinking || assistant.Content[0].Text != "weighing" {
			t.Fatalf("content = %+v, want the reasoning on the call", assistant.Content)
		}
		encoded := encodeDeepSeekBody(t, request)["messages"].([]any)[1].(map[string]any)
		if encoded["reasoning_content"] != "weighing" {
			t.Fatalf("assistant = %v, want its own reasoning, not the space placeholder", encoded)
		}
	})

	t.Run("a turn recorded text before the call also coalesces", func(t *testing.T) {
		request := inboundRequest(t, []byte(`{"model":"deepseek-flash",
			"tools":[{"type":"custom","name":"exec","description":"run"}],
			"input":[
				{"type":"message","role":"user","content":[{"type":"input_text","text":"go"}]},
				{"type":"reasoning","id":"rs_1","summary":[{"type":"summary_text","text":"weighing"}]},
				{"type":"message","role":"assistant","content":[{"type":"output_text","text":"answer"}]},
				{"type":"custom_tool_call","call_id":"call_0","name":"exec","input":"1"},
				{"type":"custom_tool_call_output","call_id":"call_0","output":"ok"}]}`))

		if len(request.Messages) != 3 {
			t.Fatalf("messages = %+v, want one assistant turn between the user and the result", request.Messages)
		}
		assistant := request.Messages[1]
		if len(assistant.ToolCalls) != 1 || len(assistant.Content) != 2 ||
			assistant.Content[0].Text != "weighing" || assistant.Content[1].Text != "answer" {
			t.Fatalf("assistant = %+v, want the reasoning, text, and call together", assistant)
		}
		encoded := encodeDeepSeekBody(t, request)["messages"].([]any)[1].(map[string]any)
		if encoded["reasoning_content"] != "weighing" {
			t.Fatalf("assistant = %v, want the reasoning replayed on the call", encoded)
		}
	})
}

// encodeDeepSeekBody sends a decoded request through the DeepSeek chat
// encoder, the way the relay does for the deepseek connection.
func encodeDeepSeekBody(t *testing.T, request *inference.Request) map[string]any {
	t.Helper()
	codec := openaichat.NewCodec("https://api.deepseek.com/v1")
	httpRequest, err := codec.EncodeRequest(request, wire.CodecOpts{
		CredentialRef: "sk-test", AuthMethod: wire.AuthAPIKey,
		BaseURL: "https://api.deepseek.com/v1",
	})
	if err != nil {
		t.Fatalf("EncodeRequest() error = %v", err)
	}
	body, err := io.ReadAll(httpRequest.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	return payload
}
