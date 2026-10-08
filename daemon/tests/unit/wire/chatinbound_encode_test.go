package codec_test

import (
	"encoding/json"
	"errors"
	"github.com/jonaskahn/relo/internal/inference"
	"io"
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/wire"
	"github.com/jonaskahn/relo/internal/adapters/wire/openaichat"
)

func TestChatCompletionsEncodeTextFrames(t *testing.T) {
	t.Run("text delta", func(t *testing.T) {
		frames, err := openaichat.NewChatCompletionsCodec().EncodeResponseEvent(inference.Event{Kind: inference.EventTextDelta, Text: "hello"})
		if err != nil {
			t.Fatalf("EncodeResponseEvent() error = %v", err)
		}
		if len(frames) != 1 || frames[0].Name != "" {
			t.Fatalf("frames = %+v, want one unnamed data frame", frames)
		}
		if deltaField(t, decodeChunk(t, frames[0]), "content") != "hello" {
			t.Fatalf("frame = %q, want the text delta", frames[0].Data)
		}
	})

	t.Run("reasoning delta", func(t *testing.T) {
		frames, err := openaichat.NewChatCompletionsCodec().EncodeResponseEvent(inference.Event{Kind: inference.EventReasoningDelta, Text: "why"})
		if err != nil {
			t.Fatalf("EncodeResponseEvent() error = %v", err)
		}
		if deltaField(t, decodeChunk(t, frames[0]), "reasoning_content") != "why" {
			t.Fatalf("frame = %q, want the reasoning delta", frames[0].Data)
		}
	})

	t.Run("tool call start", func(t *testing.T) {
		event := inference.Event{
			Kind:     inference.EventToolCallStart,
			ToolCall: &inference.ToolCallDelta{Index: 2, ID: "call_9", Name: "lookup"},
		}
		frames, err := openaichat.NewChatCompletionsCodec().EncodeResponseEvent(event)
		if err != nil {
			t.Fatalf("EncodeResponseEvent() error = %v", err)
		}
		call := firstToolCall(t, decodeChunk(t, frames[0]))
		if call["id"] != "call_9" || call["index"].(float64) != 2 || call["type"] != "function" {
			t.Fatalf("tool call = %v, want the call identity", call)
		}
		if call["function"].(map[string]any)["name"] != "lookup" {
			t.Fatalf("tool call = %v, want the function name", call)
		}
	})

	t.Run("tool call delta", func(t *testing.T) {
		event := inference.Event{
			Kind:     inference.EventToolCallDelta,
			ToolCall: &inference.ToolCallDelta{Index: 0, Arguments: "{}"},
		}
		frames, err := openaichat.NewChatCompletionsCodec().EncodeResponseEvent(event)
		if err != nil {
			t.Fatalf("EncodeResponseEvent() error = %v", err)
		}
		function := firstToolCall(t, decodeChunk(t, frames[0]))["function"].(map[string]any)
		if function["arguments"] != "{}" {
			t.Fatalf("function = %v, want the argument fragment", function)
		}
	})

	t.Run("tool call end emits nothing", func(t *testing.T) {
		frames, err := openaichat.NewChatCompletionsCodec().EncodeResponseEvent(inference.Event{
			Kind: inference.EventToolCallEnd, ToolCall: &inference.ToolCallDelta{Index: 0},
		})
		if err != nil {
			t.Fatalf("EncodeResponseEvent() error = %v", err)
		}
		if len(frames) != 0 {
			t.Fatalf("frames = %+v, want none", frames)
		}
	})

	t.Run("tool call frames tolerate a missing delta", func(t *testing.T) {
		canonical := openaichat.NewChatCompletionsCodec()
		for _, kind := range []inference.EventKind{inference.EventToolCallStart, inference.EventToolCallDelta} {
			frames, err := canonical.EncodeResponseEvent(inference.Event{Kind: kind})
			if err != nil || len(frames) != 0 {
				t.Fatalf("EncodeResponseEvent(%s) = %+v, %v, want no frames", kind, frames, err)
			}
		}
	})
}

func TestChatCompletionsEncodeTerminalFrames(t *testing.T) {
	t.Run("usage chunk carries token counts", func(t *testing.T) {
		event := inference.Event{
			Kind:  inference.EventUsage,
			Usage: &inference.UsageReport{InputTokens: 9, OutputTokens: 2, CacheReadTokens: 3},
		}
		frames, err := openaichat.NewChatCompletionsCodec().EncodeResponseEvent(event)
		if err != nil {
			t.Fatalf("EncodeResponseEvent() error = %v", err)
		}
		chunk := decodeChunk(t, frames[0])
		usage := chunk["usage"].(map[string]any)
		if usage["prompt_tokens"].(float64) != 9 || usage["completion_tokens"].(float64) != 2 || usage["total_tokens"].(float64) != 11 {
			t.Fatalf("usage = %v, want the reported counts", usage)
		}
		if len(chunk["choices"].([]any)) != 0 {
			t.Fatalf("choices = %v, want none in a usage chunk", chunk["choices"])
		}
	})

	t.Run("usage without a report emits nothing", func(t *testing.T) {
		frames, err := openaichat.NewChatCompletionsCodec().EncodeResponseEvent(inference.Event{Kind: inference.EventUsage})
		if err != nil || len(frames) != 0 {
			t.Fatalf("EncodeResponseEvent() = %+v, %v, want no frames", frames, err)
		}
	})

	t.Run("terminal chunk ends the stream", func(t *testing.T) {
		event := inference.Event{Kind: inference.EventTerminal, Terminal: &inference.TerminalInfo{Reason: inference.EventReasonToolUse}}
		frames, err := openaichat.NewChatCompletionsCodec().EncodeResponseEvent(event)
		if err != nil {
			t.Fatalf("EncodeResponseEvent() error = %v", err)
		}
		if len(frames) != 2 || frames[1].Data != "[DONE]" {
			t.Fatalf("frames = %+v, want a finish chunk and the sentinel", frames)
		}
		if finishReason(t, frames[0]) != "tool_calls" {
			t.Fatalf("finish_reason = %q, want tool_calls", finishReason(t, frames[0]))
		}
	})

	t.Run("terminal chunk without info stops", func(t *testing.T) {
		frames, err := openaichat.NewChatCompletionsCodec().EncodeResponseEvent(inference.Event{Kind: inference.EventTerminal})
		if err != nil {
			t.Fatalf("EncodeResponseEvent() error = %v", err)
		}
		if finishReason(t, frames[0]) != "stop" {
			t.Fatalf("finish_reason = %q, want stop", finishReason(t, frames[0]))
		}
	})

	t.Run("length terminal maps to length", func(t *testing.T) {
		event := inference.Event{Kind: inference.EventTerminal, Terminal: &inference.TerminalInfo{Reason: inference.EventReasonLength}}
		frames, err := openaichat.NewChatCompletionsCodec().EncodeResponseEvent(event)
		if err != nil {
			t.Fatalf("EncodeResponseEvent() error = %v", err)
		}
		if finishReason(t, frames[0]) != "length" {
			t.Fatalf("finish_reason = %q, want length", finishReason(t, frames[0]))
		}
	})

	t.Run("error event carries the failure", func(t *testing.T) {
		event := inference.Event{
			Kind:  inference.EventError,
			Error: &inference.ErrorInfo{Code: "invalid_api_key", Message: "bad key", Status: 401},
		}
		frames, err := openaichat.NewChatCompletionsCodec().EncodeResponseEvent(event)
		if err != nil {
			t.Fatalf("EncodeResponseEvent() error = %v", err)
		}
		if len(frames) != 2 || frames[1].Data != "[DONE]" {
			t.Fatalf("frames = %+v, want an error frame and the sentinel", frames)
		}
		if !strings.Contains(frames[0].Data, "invalid_api_key") || !strings.Contains(frames[0].Data, "bad key") {
			t.Fatalf("frame = %q, want the upstream failure", frames[0].Data)
		}
	})

	t.Run("error event without info uses a generic code", func(t *testing.T) {
		frames, err := openaichat.NewChatCompletionsCodec().EncodeResponseEvent(inference.Event{Kind: inference.EventError})
		if err != nil {
			t.Fatalf("EncodeResponseEvent() error = %v", err)
		}
		if !strings.Contains(frames[0].Data, "internal_error") {
			t.Fatalf("frame = %q, want a generic error code", frames[0].Data)
		}
	})

	t.Run("a terminal after an error is not rendered", func(t *testing.T) {
		inbound := openaichat.NewChatCompletionsCodec()
		failure := inference.Event{Kind: inference.EventError, Error: &inference.ErrorInfo{Code: "stream_error", Message: "gone"}}
		if _, err := inbound.EncodeResponseEvent(failure); err != nil {
			t.Fatalf("EncodeResponseEvent() error = %v", err)
		}
		terminal := inference.Event{Kind: inference.EventTerminal, Terminal: &inference.TerminalInfo{Reason: inference.EventReasonError}}
		frames, err := inbound.EncodeResponseEvent(terminal)
		if err != nil {
			t.Fatalf("EncodeResponseEvent() error = %v", err)
		}
		if len(frames) != 0 {
			t.Fatalf("frames = %+v, want nothing after the failure closed the stream", frames)
		}
	})

	t.Run("unknown event kind is rejected", func(t *testing.T) {
		_, err := openaichat.NewChatCompletionsCodec().EncodeResponseEvent(inference.Event{Kind: inference.EventKind(99)})
		if !errors.Is(err, wire.ErrUnknownEvent) {
			t.Fatalf("EncodeResponseEvent() error = %v, want %v", err, wire.ErrUnknownEvent)
		}
	})

	t.Run("relay failures are rendered for the client", func(t *testing.T) {
		frames := openaichat.NewChatCompletionsCodec().EncodeError(errors.New("upstream vanished"))
		if len(frames) != 2 || frames[1].Data != "[DONE]" {
			t.Fatalf("frames = %+v, want an error frame and the sentinel", frames)
		}
		if !strings.Contains(frames[0].Data, "upstream vanished") {
			t.Fatalf("frame = %q, want the relay failure", frames[0].Data)
		}
	})
}

func TestChatCompletionsEncodeResponse(t *testing.T) {
	t.Run("text, tool calls, and usage are assembled", func(t *testing.T) {
		events := []inference.Event{
			{Kind: inference.EventTextDelta, Text: "The weather is "},
			{Kind: inference.EventTextDelta, Text: "22C."},
			{Kind: inference.EventToolCallStart, ToolCall: &inference.ToolCallDelta{Index: 0, ID: "call_1", Name: "get_weather"}},
			{Kind: inference.EventToolCallDelta, ToolCall: &inference.ToolCallDelta{Index: 0, Arguments: "{\"city\":"}},
			{Kind: inference.EventToolCallDelta, ToolCall: &inference.ToolCallDelta{Index: 0, Arguments: "\"Hanoi\"}"}},
			{Kind: inference.EventToolCallEnd, ToolCall: &inference.ToolCallDelta{Index: 0, ID: "call_1"}},
			{Kind: inference.EventUsage, Usage: &inference.UsageReport{InputTokens: 12, OutputTokens: 7}},
			{Kind: inference.EventTerminal, Terminal: &inference.TerminalInfo{Reason: inference.EventReasonToolUse}},
		}
		payload, err := openaichat.NewChatCompletionsCodec().EncodeResponse(events)
		if err != nil {
			t.Fatalf("EncodeResponse() error = %v", err)
		}
		completion := decodeJSON(t, payload)
		if completion["object"] != "chat.completion" {
			t.Fatalf("object = %v, want chat.completion", completion["object"])
		}
		choice := completion["choices"].([]any)[0].(map[string]any)
		if choice["finish_reason"] != "tool_calls" {
			t.Fatalf("finish_reason = %v, want tool_calls", choice["finish_reason"])
		}
		message := choice["message"].(map[string]any)
		if message["content"] != "The weather is 22C." {
			t.Fatalf("content = %v, want the assembled text", message["content"])
		}
		calls := message["tool_calls"].([]any)
		if len(calls) != 1 {
			t.Fatalf("tool_calls = %v, want the reassembled call", calls)
		}
		call := calls[0].(map[string]any)
		if call["id"] != "call_1" || call["type"] != "function" {
			t.Fatalf("tool call = %v", call)
		}
		function := call["function"].(map[string]any)
		if function["name"] != "get_weather" || function["arguments"] != "{\"city\":\"Hanoi\"}" {
			t.Fatalf("function = %v, want the fully reassembled arguments", function)
		}
	})

	t.Run("reasoning content is passed through", func(t *testing.T) {
		events := []inference.Event{
			{Kind: inference.EventReasoningDelta, Text: "weighing options"},
			{Kind: inference.EventTextDelta, Text: "done"},
		}
		payload, err := openaichat.NewChatCompletionsCodec().EncodeResponse(events)
		if err != nil {
			t.Fatalf("EncodeResponse() error = %v", err)
		}
		choice := decodeJSON(t, payload)["choices"].([]any)[0].(map[string]any)
		message := choice["message"].(map[string]any)
		if message["reasoning_content"] != "weighing options" {
			t.Fatalf("message = %v, want the reasoning text", message)
		}
	})

	t.Run("an error event becomes an error object", func(t *testing.T) {
		events := []inference.Event{
			{Kind: inference.EventError, Error: &inference.ErrorInfo{Code: "bad_gateway", Message: "upstream is down", Status: 502}},
		}
		payload, err := openaichat.NewChatCompletionsCodec().EncodeResponse(events)
		if err != nil {
			t.Fatalf("EncodeResponse() error = %v", err)
		}
		if !strings.Contains(string(payload), "upstream is down") {
			t.Fatalf("payload = %s, want the error object", payload)
		}
	})

	t.Run("missing usage is reported as zero", func(t *testing.T) {
		payload, err := openaichat.NewChatCompletionsCodec().EncodeResponse([]inference.Event{{Kind: inference.EventTextDelta, Text: "hi"}})
		if err != nil {
			t.Fatalf("EncodeResponse() error = %v", err)
		}
		completion := decodeJSON(t, payload)
		usage := completion["usage"].(map[string]any)
		if usage["total_tokens"].(float64) != 0 {
			t.Fatalf("usage = %v, want zeros", usage)
		}
		choice := completion["choices"].([]any)[0].(map[string]any)
		if choice["finish_reason"] != "stop" {
			t.Fatalf("finish_reason = %v, want stop", choice["finish_reason"])
		}
	})

	t.Run("arguments without a start are ignored", func(t *testing.T) {
		events := []inference.Event{
			{Kind: inference.EventToolCallDelta, ToolCall: &inference.ToolCallDelta{Index: 0, Arguments: "{}"}},
			{Kind: inference.EventTextDelta, Text: "hi"},
		}
		if _, err := openaichat.NewChatCompletionsCodec().EncodeResponse(events); err != nil {
			t.Fatalf("EncodeResponse() error = %v", err)
		}
	})

	t.Run("a terminal after an error is not rendered", func(t *testing.T) {
		inbound := openaichat.NewChatCompletionsCodec()
		failure := inference.Event{Kind: inference.EventError, Error: &inference.ErrorInfo{Code: "stream_error", Message: "gone"}}
		if _, err := inbound.EncodeResponseEvent(failure); err != nil {
			t.Fatalf("EncodeResponseEvent() error = %v", err)
		}
		terminal := inference.Event{Kind: inference.EventTerminal, Terminal: &inference.TerminalInfo{Reason: inference.EventReasonError}}
		frames, err := inbound.EncodeResponseEvent(terminal)
		if err != nil {
			t.Fatalf("EncodeResponseEvent() error = %v", err)
		}
		if len(frames) != 0 {
			t.Fatalf("frames = %+v, want nothing after the failure closed the stream", frames)
		}
	})

	t.Run("unknown event kind is rejected", func(t *testing.T) {
		_, err := openaichat.NewChatCompletionsCodec().EncodeResponse([]inference.Event{{Kind: inference.EventKind(99)}})
		if !errors.Is(err, wire.ErrUnknownEvent) {
			t.Fatalf("EncodeResponse() error = %v, want %v", err, wire.ErrUnknownEvent)
		}
	})
}

func decodeChunk(t *testing.T, frame wire.SSEEvent) map[string]any {
	t.Helper()
	return decodeJSON(t, []byte(frame.Data))
}

func decodeJSON(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode payload %q: %v", body, err)
	}
	return payload
}

func deltaField(t *testing.T, chunk map[string]any, field string) any {
	t.Helper()
	choice := chunk["choices"].([]any)[0].(map[string]any)
	return choice["delta"].(map[string]any)[field]
}

func firstToolCall(t *testing.T, chunk map[string]any) map[string]any {
	t.Helper()
	choice := chunk["choices"].([]any)[0].(map[string]any)
	delta := choice["delta"].(map[string]any)
	return delta["tool_calls"].([]any)[0].(map[string]any)
}

func finishReason(t *testing.T, frame wire.SSEEvent) string {
	t.Helper()
	choice := decodeChunk(t, frame)["choices"].([]any)[0].(map[string]any)
	return choice["finish_reason"].(string)
}

func TestChatEffortKeepsTheRequestedLevel(t *testing.T) {
	request := &inference.Request{
		Model:     "gpt-5",
		Messages:  []inference.Message{{Role: inference.RoleUser, Content: []inference.ContentPart{{Type: inference.ContentTypeText, Text: "hi"}}}},
		Reasoning: &inference.ReasoningConfig{Effort: "xhigh"},
	}
	payload, err := openaichat.EncodePayload(request)
	if err != nil {
		t.Fatalf("EncodePayload() error = %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(payload, &body); err != nil {
		t.Fatalf("payload = %s: %v", payload, err)
	}
	if body["reasoning_effort"] != "xhigh" {
		t.Fatalf("body = %v, want xhigh passed through", body)
	}
}

func TestChatEffortFollowsTheModelLadder(t *testing.T) {
	request := &inference.Request{
		Model:     "mimo-v2.6-pro",
		Messages:  []inference.Message{{Role: inference.RoleUser, Content: []inference.ContentPart{{Type: inference.ContentTypeText, Text: "hi"}}}},
		Reasoning: &inference.ReasoningConfig{Effort: "xhigh"},
	}
	t.Run("a model with a ladder keeps the effort it asked for", func(t *testing.T) {
		body := encodeBodyWithEfforts(t, request, []string{"none", "low", "medium", "high", "xhigh"})
		if body["reasoning_effort"] != "xhigh" {
			t.Fatalf("body = %v, want xhigh kept", body)
		}
	})

	t.Run("a toggle-only model sends no effort", func(t *testing.T) {
		body := encodeBodyWithEfforts(t, request, []string{})
		if _, present := body["reasoning_effort"]; present {
			t.Fatalf("body = %v, want the field omitted", body)
		}
	})

	t.Run("a model with no stated ladder passes the effort through", func(t *testing.T) {
		body := encodeBodyWithEfforts(t, request, nil)
		if body["reasoning_effort"] != "xhigh" {
			t.Fatalf("body = %v, want xhigh passed through", body)
		}
	})

	t.Run("an effort above a stated ladder ceilings to the highest", func(t *testing.T) {
		body := encodeBodyWithEfforts(t, request, []string{"low", "medium"})
		if body["reasoning_effort"] != "medium" {
			t.Fatalf("body = %v, want medium, the highest listed level", body)
		}
	})

	t.Run("a toggle turns none off without an effort", func(t *testing.T) {
		request.Reasoning.Effort = "none"
		body := encodeChatOpts(t, request, wire.CodecOpts{
			CredentialRef: "sk-test", AuthMethod: wire.AuthAPIKey,
			ReasoningEfforts: []string{"low", "high", "max"}, ReasoningToggle: true,
		})
		thinking, _ := body["thinking"].(map[string]any)
		if _, present := body["reasoning_effort"]; present || thinking["type"] != "disabled" {
			t.Fatalf("body = %v, want thinking disabled and no effort", body)
		}
	})

	t.Run("a qwen template uses enable_thinking", func(t *testing.T) {
		request.Reasoning.Effort = "high"
		maxBudget := int64(8000)
		body := encodeChatOpts(t, request, wire.CodecOpts{
			CredentialRef: "sk-test", AuthMethod: wire.AuthAPIKey, TemplateID: "alibaba",
			ReasoningToggle: true, ReasoningBudget: true, ReasoningBudgetMax: &maxBudget,
		})
		if body["enable_thinking"] != true || body["thinking"] != nil {
			t.Fatalf("body = %v, want enable_thinking and no thinking object", body)
		}
		if body["thinking_budget"] != float64(8000) {
			t.Fatalf("thinking_budget = %v, want the declared max", body["thinking_budget"])
		}
	})
}

func TestDeepSeekReasoningReplay(t *testing.T) {
	tool := inference.Tool{Name: "lookup", Parameters: json.RawMessage(`{"type":"object"}`)}
	withTools := func(messages ...inference.Message) *inference.Request {
		return &inference.Request{Model: "deepseek-flash", Messages: messages, Tools: []inference.Tool{tool}}
	}
	answer := inference.Message{
		Role: inference.RoleAssistant,
		Content: []inference.ContentPart{
			{Type: inference.ContentTypeThinking, Text: "weighing"},
			{Type: inference.ContentTypeText, Text: "answer"},
		},
	}
	plain := inference.Message{
		Role:    inference.RoleAssistant,
		Content: []inference.ContentPart{{Type: inference.ContentTypeText, Text: "answer"}},
	}
	user := inference.Message{
		Role:    inference.RoleUser,
		Content: []inference.ContentPart{{Type: inference.ContentTypeText, Text: "hi"}},
	}

	t.Run("stored thinking is replayed as reasoning_content", func(t *testing.T) {
		body := encodeChatBody(t, "https://api.deepseek.com/v1", withTools(user, answer))
		if reasoningContent(t, body, 1) != "weighing" {
			t.Fatalf("messages = %v, want the thinking text", body["messages"])
		}
		if messageContent(t, body, 1) != "answer" {
			t.Fatalf("content = %v, want the answer without the thinking text", body["messages"])
		}
		if _, present := messageMap(t, body, 0)["reasoning_content"]; present {
			t.Fatalf("messages = %v, want reasoning_content only on assistant turns", body["messages"])
		}
	})

	t.Run("a missing chain is replayed as one space", func(t *testing.T) {
		body := encodeChatBody(t, "https://api.deepseek.com/v1", withTools(plain))
		if reasoningContent(t, body, 0) != " " {
			t.Fatalf("messages = %v, want a single space", body["messages"])
		}
	})

	t.Run("another host omits reasoning_content", func(t *testing.T) {
		body := encodeChatBody(t, "https://api.openai.com/v1", withTools(answer))
		if _, present := messageMap(t, body, 0)["reasoning_content"]; present {
			t.Fatalf("messages = %v, want no reasoning_content", body["messages"])
		}
	})

	t.Run("a request without tools omits reasoning_content", func(t *testing.T) {
		request := withTools(plain)
		request.Tools = nil
		body := encodeChatBody(t, "https://api.deepseek.com/v1", request)
		if _, present := messageMap(t, body, 0)["reasoning_content"]; present {
			t.Fatalf("messages = %v, want no reasoning_content", body["messages"])
		}
	})

	t.Run("thinking off omits reasoning_content", func(t *testing.T) {
		request := withTools(plain)
		request.Reasoning = &inference.ReasoningConfig{Effort: "none"}
		body := encodeChatOpts(t, request, wire.CodecOpts{
			BaseURL: "https://api.deepseek.com/v1", ReasoningToggle: true,
			ReasoningEfforts: []string{"low", "high", "max"},
		})
		if _, present := messageMap(t, body, 0)["reasoning_content"]; present {
			t.Fatalf("messages = %v, want no reasoning_content", body["messages"])
		}
	})
}

func TestChatDropsEmptyThinkingFromAssistantContent(t *testing.T) {
	request := &inference.Request{
		Model: "glm-5.3-flash",
		Messages: []inference.Message{{
			Role: inference.RoleAssistant,
			Content: []inference.ContentPart{
				{Type: inference.ContentTypeThinking, Signature: "opaque"},
				{Type: inference.ContentTypeText, Text: "The backend gate is already 90% per package"},
			},
			ToolCalls: []inference.ToolCall{{ID: "call_1", Name: "lookup", Arguments: "{}"}},
		}},
	}
	message := messageMap(t, encodeChatBody(t, "https://api.z.ai/api/paas/v4", request), 0)
	if message["content"] != "The backend gate is already 90% per package" {
		t.Fatalf("content = %v, want the visible sentence as a string", message["content"])
	}
	encoded, err := json.Marshal(message["content"])
	if err != nil {
		t.Fatalf("content = %v: %v", message["content"], err)
	}
	if strings.Contains(string(encoded), `"type"`) {
		t.Fatalf("content = %s, want no text part", encoded)
	}
	calls, _ := message["tool_calls"].([]any)
	if len(calls) != 1 {
		t.Fatalf("tool_calls = %v, want the call kept", message["tool_calls"])
	}
}

func messageMap(t *testing.T, body map[string]any, index int) map[string]any {
	t.Helper()
	messages, _ := body["messages"].([]any)
	if index >= len(messages) {
		t.Fatalf("messages = %v, want index %d", messages, index)
	}
	message, _ := messages[index].(map[string]any)
	return message
}

func reasoningContent(t *testing.T, body map[string]any, index int) string {
	t.Helper()
	text, _ := messageMap(t, body, index)["reasoning_content"].(string)
	return text
}

func messageContent(t *testing.T, body map[string]any, index int) string {
	t.Helper()
	text, _ := messageMap(t, body, index)["content"].(string)
	return text
}

func TestChatDropsNullBytePatternsForDeepSeek(t *testing.T) {
	request := &inference.Request{
		Model: "deepseek-v4-pro",
		Messages: []inference.Message{{
			Role:    inference.RoleUser,
			Content: []inference.ContentPart{{Type: inference.ContentTypeText, Text: "hi"}},
		}},
		Tools: []inference.Tool{{
			Name: "Artifact",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"content": {"type": "string", "pattern": "^[^\\0]*$"},
					"slug": {"type": "string", "pattern": "^[a-z]+$"}
				}
			}`),
		}},
	}

	t.Run("deepseek drops the null-byte pattern", func(t *testing.T) {
		body := encodeChatBody(t, "https://api.deepseek.com/v1", request)
		if pattern, ok := toolPropertyPattern(t, body, "content"); ok {
			t.Fatalf("content pattern = %q, want it omitted", pattern)
		}
		if pattern, ok := toolPropertyPattern(t, body, "slug"); !ok || pattern != "^[a-z]+$" {
			t.Fatalf("slug pattern = %q, present = %v, want ^[a-z]+$", pattern, ok)
		}
	})

	t.Run("other hosts keep the null-byte pattern", func(t *testing.T) {
		body := encodeChatBody(t, "https://api.openai.com/v1", request)
		if pattern, ok := toolPropertyPattern(t, body, "content"); !ok || pattern != `^[^\0]*$` {
			t.Fatalf("content pattern = %q, present = %v, want the null-byte guard", pattern, ok)
		}
		if pattern, ok := toolPropertyPattern(t, body, "slug"); !ok || pattern != "^[a-z]+$" {
			t.Fatalf("slug pattern = %q, present = %v, want ^[a-z]+$", pattern, ok)
		}
	})
}

func toolPropertyPattern(t *testing.T, body map[string]any, property string) (string, bool) {
	t.Helper()
	tools, _ := body["tools"].([]any)
	if len(tools) == 0 {
		t.Fatal("body has no tools")
	}
	function, _ := tools[0].(map[string]any)["function"].(map[string]any)
	parameters, _ := function["parameters"].(map[string]any)
	properties, _ := parameters["properties"].(map[string]any)
	schema, _ := properties[property].(map[string]any)
	pattern, ok := schema["pattern"].(string)
	return pattern, ok
}

func encodeChatBody(t *testing.T, baseURL string, request *inference.Request) map[string]any {
	t.Helper()
	codec := openaichat.NewCodec(baseURL)
	httpRequest, err := codec.EncodeRequest(request, wire.CodecOpts{CredentialRef: "sk-test", AuthMethod: wire.AuthAPIKey, BaseURL: baseURL})
	if err != nil {
		t.Fatalf("EncodeRequest() error = %v", err)
	}
	body, err := io.ReadAll(httpRequest.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return decodeJSON(t, body)
}

func encodeChatOpts(t *testing.T, request *inference.Request, opts wire.CodecOpts) map[string]any {
	t.Helper()
	if opts.CredentialRef == "" {
		opts.CredentialRef = "sk-test"
	}
	if opts.AuthMethod == "" {
		opts.AuthMethod = wire.AuthAPIKey
	}
	codec := openaichat.NewCodec("https://api.xiaomimimo.com/v1")
	httpRequest, err := codec.EncodeRequest(request, opts)
	if err != nil {
		t.Fatalf("EncodeRequest() error = %v", err)
	}
	body, err := io.ReadAll(httpRequest.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("payload = %s: %v", body, err)
	}
	return payload
}

func encodeBodyWithEfforts(t *testing.T, request *inference.Request, efforts []string) map[string]any {
	t.Helper()
	codec := openaichat.NewCodec("https://api.xiaomimimo.com/v1")
	httpRequest, err := codec.EncodeRequest(request, wire.CodecOpts{CredentialRef: "sk-test", AuthMethod: wire.AuthAPIKey, ReasoningEfforts: efforts})
	if err != nil {
		t.Fatalf("EncodeRequest() error = %v", err)
	}
	body, err := io.ReadAll(httpRequest.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("payload = %s: %v", body, err)
	}
	return payload
}
