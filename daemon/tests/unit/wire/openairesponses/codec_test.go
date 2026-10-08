package openai_responses_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/jonaskahn/relo/internal/inference"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/codex"
	"github.com/jonaskahn/relo/internal/adapters/wire"
	"github.com/jonaskahn/relo/internal/adapters/wire/openairesponses"
	"github.com/jonaskahn/relo/internal/adapters/wire/sse"
	"github.com/jonaskahn/relo/tests/testkit"
)

const encryptedState = "gAAAA-encrypted-state-=="

func TestEncodeRequest(t *testing.T) {
	t.Run("input items, instructions, tools, and the store flag", func(t *testing.T) {
		body := encodeBody(t, canonicalRequest(true))
		if body["model"] != "gpt-5" || body["stream"] != true || body["store"] != false {
			t.Fatalf("body = %v, want the model, stream flag, and store false", body)
		}
		if body["instructions"] != "be brief" {
			t.Fatalf("instructions = %v, want the system prompt", body["instructions"])
		}
		if body["max_output_tokens"] != float64(64) {
			t.Fatalf("max_output_tokens = %v, want the canonical limit", body["max_output_tokens"])
		}
		items := body["input"].([]any)
		if items[0].(map[string]any)["content"].([]any)[0].(map[string]any)["type"] != "input_text" {
			t.Fatalf("input = %v, want a message item first", items)
		}
		tools := body["tools"].([]any)
		if tools[0].(map[string]any)["name"] != "get_weather" {
			t.Fatalf("tools = %v, want the Responses tool shape", tools)
		}
	})

	t.Run("tool calls and tool results become items", func(t *testing.T) {
		body := encodeBody(t, conversationRequest())
		items := body["input"].([]any)
		if types := itemTypes(items); !contains(types, "function_call") || !contains(types, "function_call_output") {
			t.Fatalf("input types = %v, want a call and its result", types)
		}
	})

	t.Run("reasoning config becomes reasoning parameters", func(t *testing.T) {
		request := canonicalRequest(false)
		request.Reasoning = &inference.ReasoningConfig{Effort: "high"}
		body := encodeBody(t, request)
		reasoning := body["reasoning"].(map[string]any)
		if reasoning["effort"] != "high" || reasoning["summary"] != "auto" {
			t.Fatalf("reasoning = %v, want the effort and summary", reasoning)
		}
		request.Reasoning.Effort = "max"
		if encodeBody(t, request)["reasoning"].(map[string]any)["effort"] != "max" {
			t.Fatal("max was rewritten")
		}
		include := body["include"].([]any)
		if len(include) != 1 || include[0] != "reasoning.encrypted_content" {
			t.Fatalf("include = %v, want the encrypted reasoning opt-in", include)
		}
	})

	t.Run("encrypted reasoning is replayed byte-for-byte", func(t *testing.T) {
		request := conversationRequest()
		request.Reasoning = &inference.ReasoningConfig{Effort: "low", ID: "rs_9", EncryptedContent: []byte(encryptedState)}
		items := encodeBody(t, request)["input"].([]any)
		replayed := findItem(items, "reasoning")
		if replayed == nil {
			t.Fatal("the replayed reasoning item is missing")
		}
		if replayed["id"] != "rs_9" || replayed["encrypted_content"] != encryptedState {
			t.Fatalf("reasoning item = %v, want the upstream state unchanged", replayed)
		}
		if summary, ok := replayed["summary"].([]any); !ok || len(summary) != 0 {
			t.Fatalf("summary = %v, want an empty array", replayed["summary"])
		}
		if indexOfItem(items, "reasoning") > indexOfItem(items, "function_call") {
			t.Fatalf("reasoning item = %v, want it before the calls it explains", items)
		}
	})

	t.Run("the credential is applied as a bearer header", func(t *testing.T) {
		request := newTestRequest(t, canonicalRequest(true), wire.CodecOpts{CredentialRef: "sk-test"})
		if got := request.Header.Get("Authorization"); got != "Bearer sk-test" {
			t.Fatalf("Authorization = %q", got)
		}
		if request.URL.String() != openairesponses.DefaultBaseURL+"/responses" {
			t.Fatalf("url = %s, want the Responses endpoint", request.URL)
		}
	})

	t.Run("a per-request base URL overrides the default", func(t *testing.T) {
		request := newTestRequest(t, canonicalRequest(true), wire.CodecOpts{BaseURL: "http://127.0.0.1:9000/", CredentialRef: "sk-test"})
		if request.URL.String() != "http://127.0.0.1:9000/responses" {
			t.Fatalf("url = %s, want the override", request.URL)
		}
	})

	t.Run("the public API path stays a plain Responses request", func(t *testing.T) {
		request := newTestRequest(t, canonicalRequest(true), wire.CodecOpts{
			CredentialRef: "sk-test",
			ExtraHeaders:  map[string]string{"User-Agent": "caller"},
		})
		if request.Header.Get("User-Agent") != "caller" || request.Header.Get("originator") != "" ||
			request.Header.Get("version") != "" || request.Header.Get("OpenAI-Beta") != "" {
			t.Fatalf("headers = %v, want no Codex fingerprint", request.Header)
		}
	})

	t.Run("missing credential is rejected", func(t *testing.T) {
		_, err := openairesponses.Module().EncodeRequest(canonicalRequest(true), wire.CodecOpts{})
		if !errors.Is(err, wire.ErrMissingCredential) {
			t.Fatalf("EncodeRequest() error = %v, want the missing credential sentinel", err)
		}
	})

	t.Run("the Codex backend's dialect leaves the ceiling out", func(t *testing.T) {
		encoded := newTestRequest(t, canonicalRequest(true), wire.CodecOpts{
			CredentialRef: "sk-test", RefusesMaxOutputTokens: true,
		})
		body := decodeBody(t, encoded)
		if _, present := body["max_output_tokens"]; present {
			t.Fatalf("body = %v, want no max_output_tokens for the Codex backend", body)
		}
		if body["model"] != "gpt-5" || body["stream"] != true || body["store"] != false {
			t.Fatalf("body = %v, want everything else intact", body)
		}
	})
}

func TestCodexHeaders(t *testing.T) {
	token := testJWT(t, map[string]any{
		"https://api.openai.com/auth": map[string]any{
			"chatgpt_data_residency": "eu",
		},
	})
	request := newTestRequest(t, canonicalRequest(true), wire.CodecOpts{
		BaseURL:       "https://chatgpt.com/backend-api/codex",
		CredentialRef: token,
		AuthMethod:    wire.AuthOAuth,
		SessionAnchor: "session-1",
		RequestID:     "request-1",
		ExtraHeaders: map[string]string{
			"chatgpt-account-id":       "account-1",
			"originator":               "old-client",
			"OpenAI-Beta":              "old-beta",
			"x-api-key":                "inbound-key",
			"x-codex-installation-id":  "install-1",
			"thread-id":                "thread-1",
			"x-codex-window-id":        "window-1",
			"x-codex-turn-metadata":    `{"turn":1}`,
			"x-codex-turn-state":       "state-1",
			"x-models-etag":            "etag-1",
			"x-openai-subagent":        "reviewer",
			"x-codex-parent-thread-id": "parent-1",
		},
	})
	if request.URL.String() != "https://chatgpt.com/backend-api/codex/responses" {
		t.Fatalf("url = %s", request.URL)
	}
	if request.Header.Get("Authorization") != "Bearer "+token ||
		request.Header.Get("chatgpt-account-id") != "account-1" {
		t.Fatalf("authorization = %q, account = %q",
			request.Header.Get("Authorization"), request.Header.Get("chatgpt-account-id"))
	}
	if request.Header.Get("originator") != codex.Originator ||
		request.Header.Get("version") != codex.Version ||
		request.Header.Get("User-Agent") != codex.UserAgent ||
		request.Header.Get("OpenAI-Beta") != codex.OpenAIBeta {
		t.Fatalf("client headers = %v", request.Header)
	}
	if request.Header.Get("Accept") != "text/event-stream" ||
		request.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("accept = %q, content-type = %q",
			request.Header.Get("Accept"), request.Header.Get("Content-Type"))
	}
	for _, name := range []string{"session_id", "conversation_id", "x-client-request-id", "session-id"} {
		if request.Header.Get(name) != "session-1" {
			t.Fatalf("%s = %q, want session-1", name, request.Header.Get(name))
		}
	}
	if request.Header.Get("x-codex-routing-hint") != "model=gpt-5" {
		t.Fatalf("routing hint = %q", request.Header.Get("x-codex-routing-hint"))
	}
	if request.Header.Get("x-api-key") != "" || request.Header.Get("x-codex-installation-id") != "" {
		t.Fatalf("api key = %q, installation = %q",
			request.Header.Get("x-api-key"), request.Header.Get("x-codex-installation-id"))
	}
	if request.Header.Get("x-openai-internal-codex-residency") != "eu" {
		t.Fatalf("residency = %q, want the token region", request.Header.Get("x-openai-internal-codex-residency"))
	}
	for name, want := range map[string]string{
		"thread-id":                "thread-1",
		"x-codex-window-id":        "window-1",
		"x-codex-turn-metadata":    `{"turn":1}`,
		"x-codex-turn-state":       "state-1",
		"x-models-etag":            "etag-1",
		"x-openai-subagent":        "reviewer",
		"x-codex-parent-thread-id": "parent-1",
	} {
		if request.Header.Get(name) != want {
			t.Fatalf("%s = %q, want %q", name, request.Header.Get(name), want)
		}
	}

	t.Run("caller residency wins and request id is the session fallback", func(t *testing.T) {
		overridden := newTestRequest(t, canonicalRequest(true), wire.CodecOpts{
			BaseURL:       "https://chatgpt.com/backend-api/codex",
			CredentialRef: token,
			AuthMethod:    wire.AuthOAuth,
			RequestID:     "request-2",
			ExtraHeaders: map[string]string{
				"x-openai-internal-codex-residency": "us",
			},
		})
		if overridden.Header.Get("x-openai-internal-codex-residency") != "us" {
			t.Fatalf("residency = %q, want caller override",
				overridden.Header.Get("x-openai-internal-codex-residency"))
		}
		if overridden.Header.Get("session_id") != "request-2" {
			t.Fatalf("session = %q, want request fallback", overridden.Header.Get("session_id"))
		}
	})
}

func testJWT(t *testing.T, claims map[string]any) string {
	t.Helper()
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal claims: %v", err)
	}
	return "e30." + base64.RawURLEncoding.EncodeToString(payload) + ".sig"
}

func TestStreamDecoding(t *testing.T) {
	t.Run("simple text streaming", func(t *testing.T) {
		events := pushStream(t, openairesponses.Module().NewStreamDecoder(), "openai/responses_streaming.txt")
		if got := textOf(events); got != "Hello world" {
			t.Fatalf("text = %q, want the concatenated deltas", got)
		}
		if countKind(events, inference.EventTerminal) != 1 {
			t.Fatalf("events = %+v, want exactly one terminal", events)
		}
		if reason := terminalReason(events); reason != inference.EventReasonStop {
			t.Fatalf("terminal = %q, want stop", reason)
		}
	})

	t.Run("usage from the terminal event", func(t *testing.T) {
		events := pushStream(t, openairesponses.Module().NewStreamDecoder(), "openai/responses_streaming.txt")
		usage := lastUsage(events)
		if usage == nil || usage.InputTokens != 10 || usage.OutputTokens != 2 || usage.CacheReadTokens != 4 {
			t.Fatalf("usage = %+v, want the reported counts", usage)
		}
		if countKind(events, inference.EventUsage) != 1 {
			t.Fatalf("usage events = %d, want one", countKind(events, inference.EventUsage))
		}
	})

	t.Run("encrypted reasoning byte-for-byte", func(t *testing.T) {
		events := pushStream(t, openairesponses.Module().NewStreamDecoder(), "openai/responses_reasoning.txt")
		if got := reasoningTextOf(events); got != "Checked the fixtures" {
			t.Fatalf("reasoning text = %q, want the summary", got)
		}
		state := reasoningState(events)
		if state == nil || state.ID != "rs_1" {
			t.Fatalf("reasoning state = %+v, want the upstream item", state)
		}
		if string(state.EncryptedContent) != encryptedState {
			t.Fatalf("encrypted content = %q, want %q", state.EncryptedContent, encryptedState)
		}
	})

	t.Run("tool call with argument deltas", func(t *testing.T) {
		events := pushStream(t, openairesponses.Module().NewStreamDecoder(), "openai/responses_tool_call.txt")
		starts, ends := countKind(events, inference.EventToolCallStart), countKind(events, inference.EventToolCallEnd)
		if starts != 2 || ends != 2 {
			t.Fatalf("tool call events = %d starts, %d ends, want two calls", starts, ends)
		}
		if reason := terminalReason(events); reason != inference.EventReasonToolUse {
			t.Fatalf("terminal = %q, want tool_use", reason)
		}
	})

	t.Run("parallel tool calls keep their own arguments", func(t *testing.T) {
		events := pushStream(t, openairesponses.Module().NewStreamDecoder(), "openai/responses_tool_call.txt")
		calls := assembledCalls(events)
		if len(calls) != 2 {
			t.Fatalf("calls = %+v, want two", calls)
		}
		if calls[0].ID != "call_1" || calls[0].Name != "get_weather" || calls[0].Arguments != `{"city":"Hanoi"}` {
			t.Fatalf("first call = %+v, want the weather call", calls[0])
		}
		if calls[1].ID != "call_2" || calls[1].Name != "get_time" || calls[1].Arguments != `{"zone":"UTC"}` {
			t.Fatalf("second call = %+v, want the time call", calls[1])
		}
	})

	t.Run("an interrupted stream is still closed with a terminal", func(t *testing.T) {
		decoder := openairesponses.Module().NewStreamDecoder()
		pushStreamData(t, decoder, `{"type":"response.output_text.delta","delta":"partial"}`)
		closing, err := decoder.Finish()
		if err != nil {
			t.Fatalf("Finish() error = %v", err)
		}
		if len(closing) != 1 || closing[0].Kind != inference.EventTerminal || closing[0].Terminal.Reason != inference.EventReasonStop {
			t.Fatalf("Finish() = %+v, want a synthesized terminal", closing)
		}
		again, err := decoder.Finish()
		if err != nil || len(again) != 0 {
			t.Fatalf("second Finish() = %+v, %v, want nothing", again, err)
		}
	})

	t.Run("an error mid-stream becomes an error event", func(t *testing.T) {
		decoder := openairesponses.Module().NewStreamDecoder()
		pushStreamData(t, decoder, `{"type":"response.output_text.delta","delta":"before"}`)
		events := pushStreamData(t, decoder, `{"type":"error","code":"server_error","message":"upstream gave up"}`)
		if len(events) != 1 || events[0].Kind != inference.EventError || events[0].Error.Message != "upstream gave up" {
			t.Fatalf("events = %+v, want the upstream failure", events)
		}
		closing, err := decoder.Finish()
		if err != nil {
			t.Fatalf("Finish() error = %v", err)
		}
		if closing[0].Kind != inference.EventTerminal || closing[0].Terminal.Reason != inference.EventReasonError {
			t.Fatalf("Finish() = %+v, want an error terminal", closing)
		}
	})

	t.Run("unknown events are skipped", func(t *testing.T) {
		decoder := openairesponses.Module().NewStreamDecoder()
		for _, data := range []string{"", "   ", `{"type":"response.in_progress"}`, `{"type":"response.queued","response":{}}`} {
			events, err := decoder.Push(wire.SSEEvent{Data: data})
			if err != nil {
				t.Fatalf("Push(%q) error = %v", data, err)
			}
			if len(events) != 0 {
				t.Fatalf("Push(%q) = %+v, want nothing", data, events)
			}
		}
	})

	t.Run("a malformed frame is rejected", func(t *testing.T) {
		_, err := openairesponses.Module().NewStreamDecoder().Push(wire.SSEEvent{Data: "{not json"})
		if !errors.Is(err, wire.ErrMalformedEvent) {
			t.Fatalf("Push() error = %v, want the malformed event sentinel", err)
		}
	})

	t.Run("the SSE event field names the frame", func(t *testing.T) {
		events, err := openairesponses.Module().DecodeResponseEvent(wire.SSEEvent{
			Name: "response.output_text.delta",
			Data: `{"delta":"named"}`,
		})
		if err != nil {
			t.Fatalf("DecodeResponseEvent() error = %v", err)
		}
		if len(events) != 1 || events[0].Text != "named" {
			t.Fatalf("events = %+v, want the delta", events)
		}
	})
}

func TestDecodeResponse(t *testing.T) {
	t.Run("simple text non-streaming", func(t *testing.T) {
		events, err := openairesponses.Module().DecodeResponse(readFixture(t, "openai/responses_nonstreaming.json"))
		if err != nil {
			t.Fatalf("DecodeResponse() error = %v", err)
		}
		if got := textOf(events); got != "Hi there" {
			t.Fatalf("text = %q, want the message text", got)
		}
		if got := reasoningTextOf(events); got != "Read the question" {
			t.Fatalf("reasoning = %q, want the summary", got)
		}
		if state := reasoningState(events); state == nil || string(state.EncryptedContent) != encryptedState {
			t.Fatalf("reasoning state = %+v, want the encrypted item", state)
		}
		if calls := assembledCalls(events); len(calls) != 1 || calls[0].Name != "get_weather" {
			t.Fatalf("calls = %+v, want the function call", calls)
		}
		if usage := lastUsage(events); usage == nil || usage.InputTokens != 12 || usage.OutputTokens != 5 || usage.CacheReadTokens != 2 {
			t.Fatalf("usage = %+v, want the reported counts", usage)
		}
		if reason := terminalReason(events); reason != inference.EventReasonToolUse {
			t.Fatalf("terminal = %q, want tool_use", reason)
		}
	})

	t.Run("an incomplete response finishes on a length terminal", func(t *testing.T) {
		body := []byte(`{"id":"resp_5","status":"incomplete","output":[],"incomplete_details":{"reason":"max_output_tokens"}}`)
		events, err := openairesponses.Module().DecodeResponse(body)
		if err != nil {
			t.Fatalf("DecodeResponse() error = %v", err)
		}
		if reason := terminalReason(events); reason != inference.EventReasonLength {
			t.Fatalf("terminal = %q, want length", reason)
		}
	})

	t.Run("a malformed completion is rejected", func(t *testing.T) {
		_, err := openairesponses.Module().DecodeResponse([]byte("{not json"))
		if !errors.Is(err, wire.ErrInvalidResponse) {
			t.Fatalf("DecodeResponse() error = %v, want the invalid response sentinel", err)
		}
	})

	t.Run("a completion carrying an error becomes an error event", func(t *testing.T) {
		body := []byte(`{"id":"resp_6","status":"failed","error":{"code":"server_error","message":"upstream is down"}}`)
		events, err := openairesponses.Module().DecodeResponse(body)
		if err != nil {
			t.Fatalf("DecodeResponse() error = %v", err)
		}
		if countKind(events, inference.EventError) != 1 {
			t.Fatalf("events = %+v, want one error event", events)
		}
		if reason := terminalReason(events); reason != inference.EventReasonError {
			t.Fatalf("terminal = %q, want error", reason)
		}
	})
}

func TestDecodeError(t *testing.T) {
	t.Run("an upstream error body is mapped", func(t *testing.T) {
		body := []byte(`{"error":{"code":"invalid_api_key","message":"Incorrect API key provided","type":"invalid_request_error"}}`)
		info := openairesponses.Module().DecodeError(401, body)
		if info.Status != 401 || info.Code != "invalid_api_key" || info.Message == "" {
			t.Fatalf("info = %+v, want the upstream failure", info)
		}
	})

	t.Run("a non-json error body falls back to the status text", func(t *testing.T) {
		info := openairesponses.Module().DecodeError(503, []byte("service unavailable"))
		if info.Code != "Service Unavailable" || info.Status != 503 {
			t.Fatalf("info = %+v, want the status text", info)
		}
	})
}

func TestInboundDecode(t *testing.T) {
	t.Run("inbound decode", func(t *testing.T) {
		request := inboundRequest(t, readFixture(t, "openai/responses_request.json"))
		if request.Model != "gpt-5" || !request.Stream || request.MaxTokens != 256 {
			t.Fatalf("request = %+v, want the client options", request)
		}
		if request.Messages[0].Role != inference.RoleSystem {
			t.Fatalf("messages = %+v, want instructions as a system message", request.Messages)
		}
		user := request.Messages[1]
		if user.Role != inference.RoleUser || user.Content[1].Type != inference.ContentTypeImage {
			t.Fatalf("user message = %+v, want text and image parts", user)
		}
		if calls := toolCalls(request.Messages); len(calls) != 1 || calls[0].ID != "call_1" {
			t.Fatalf("tool calls = %+v, want the client call", calls)
		}
		result := messageWithRole(request.Messages, inference.RoleTool)
		if result == nil || result.ToolCallID != "call_1" || result.Content[0].Text != `{"temp":31}` {
			t.Fatalf("tool message = %+v, want the result bound to its call", result)
		}
		if request.Reasoning == nil || request.Reasoning.Effort != "low" || string(request.Reasoning.EncryptedContent) != encryptedState {
			t.Fatalf("reasoning = %+v, want the effort and the replayed state", request.Reasoning)
		}
		if len(request.Tools) != 1 || request.Tools[0].Name != "get_weather" {
			t.Fatalf("tools = %+v, want the declared tool", request.Tools)
		}
	})

	t.Run("input may be a plain string", func(t *testing.T) {
		request := inboundRequest(t, []byte(`{"model":"gpt-5","input":"hello"}`))
		if len(request.Messages) != 1 || request.Messages[0].Content[0].Text != "hello" {
			t.Fatalf("messages = %+v, want one user message", request.Messages)
		}
	})

	t.Run("a request without a model is rejected", func(t *testing.T) {
		_, err := decodeInbound([]byte(`{"input":"hello"}`))
		if !errors.Is(err, wire.ErrInvalidRequest) {
			t.Fatalf("DecodeRequest() error = %v, want the invalid request sentinel", err)
		}
	})

	t.Run("a malformed body is rejected", func(t *testing.T) {
		if _, err := decodeInbound([]byte("{not json")); !errors.Is(err, wire.ErrInvalidRequest) {
			t.Fatalf("DecodeRequest() error = %v, want the invalid request sentinel", err)
		}
	})
}

func TestInboundEncode(t *testing.T) {
	t.Run("inbound encode", func(t *testing.T) {
		inbound := openairesponses.NewInbound()
		frames := encodeEvents(t, inbound, conversationEvents())
		names := frameNames(frames)
		if names[0] != "response.created" || names[1] != "response.in_progress" {
			t.Fatalf("frames = %v, want the response identity first", names)
		}
		last := frames[len(frames)-1]
		if last.Name != "response.completed" {
			t.Fatalf("last frame = %q, want the terminal response", last.Name)
		}
		if !strings.Contains(last.Data, `"input_tokens":7`) || !strings.Contains(last.Data, `"output_tokens":3`) {
			t.Fatalf("terminal = %s, want the token counts", last.Data)
		}
		if countName(frames, "response.output_text.delta") != 2 {
			t.Fatalf("frames = %v, want both text deltas", names)
		}
		if countName(frames, "response.function_call_arguments.done") != 1 {
			t.Fatalf("frames = %v, want the completed call", names)
		}
	})

	t.Run("encrypted reasoning is rendered unchanged", func(t *testing.T) {
		inbound := openairesponses.NewInbound()
		frames := encodeEvents(t, inbound, []inference.Event{{
			Kind:      inference.EventReasoningDelta,
			Reasoning: &inference.ReasoningConfig{ID: "rs_1", EncryptedContent: []byte(encryptedState)},
		}})
		item := frames[len(frames)-1]
		if item.Name != "response.output_item.done" || !strings.Contains(item.Data, encryptedState) {
			t.Fatalf("frame = %s, want the reasoning item unchanged", item.Data)
		}
	})

	t.Run("the non-streaming shape carries the assembled response", func(t *testing.T) {
		payload, err := openairesponses.NewInbound().EncodeResponse(conversationEvents())
		if err != nil {
			t.Fatalf("EncodeResponse() error = %v", err)
		}
		var object map[string]any
		if err := json.Unmarshal(payload, &object); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if object["object"] != "response" || object["status"] != "completed" {
			t.Fatalf("object = %v, want a completed response", object)
		}
		items := object["output"].([]any)
		if !contains(itemTypes(items), "function_call") {
			t.Fatalf("output = %v, want the function call item", items)
		}
		usage := object["usage"].(map[string]any)
		if usage["input_tokens"] != float64(7) || usage["total_tokens"] != float64(10) {
			t.Fatalf("usage = %v, want the token counts", usage)
		}
	})

	t.Run("a relay failure closes the stream with an error", func(t *testing.T) {
		frames := openairesponses.NewInbound().EncodeError(errors.New("upstream went away"))
		names := frameNames(frames)
		if len(names) != 4 || names[3] != "response.failed" {
			t.Fatalf("frames = %v, want the error and the failed response", names)
		}
		if !strings.Contains(frames[2].Data, "upstream went away") {
			t.Fatalf("frame = %s, want the failure message", frames[2].Data)
		}
	})

	t.Run("an unknown canonical event is rejected", func(t *testing.T) {
		_, err := openairesponses.NewInbound().EncodeResponseEvent(inference.Event{Kind: inference.EventKind(99)})
		if !errors.Is(err, wire.ErrUnknownEvent) {
			t.Fatalf("EncodeResponseEvent() error = %v, want the unknown event sentinel", err)
		}
	})
}

func TestModule(t *testing.T) {
	t.Run("the module satisfies the codec contract", func(t *testing.T) {
		var module wire.Codec = openairesponses.Module()
		events, err := module.DecodeResponse(readFixture(t, "openai/responses_nonstreaming.json"))
		if err != nil || len(events) == 0 {
			t.Fatalf("DecodeResponse() = %d events, %v, want the decoded response", len(events), err)
		}
		if streamer, ok := module.(wire.StreamCodec); !ok || streamer.NewStreamDecoder() == nil {
			t.Fatal("the module cannot decode a stream")
		}
		if _, ok := module.(wire.ErrorDecoder); !ok {
			t.Fatal("the module cannot decode an upstream error")
		}
	})
}

func canonicalRequest(stream bool) *inference.Request {
	return &inference.Request{
		Model:     "gpt-5",
		Stream:    stream,
		MaxTokens: 64,
		Messages: []inference.Message{
			{Role: inference.RoleSystem, Content: []inference.ContentPart{{Type: inference.ContentTypeText, Text: "be brief"}}},
			{Role: inference.RoleUser, Content: []inference.ContentPart{{Type: inference.ContentTypeText, Text: "hello"}}},
		},
		Tools: []inference.Tool{{Name: "get_weather", Description: "look it up", Parameters: json.RawMessage(`{"type":"object"}`)}},
	}
}

func conversationRequest() *inference.Request {
	request := canonicalRequest(false)
	request.Messages = append(request.Messages,
		inference.Message{Role: inference.RoleAssistant, ToolCalls: []inference.ToolCall{{ID: "call_1", Name: "get_weather", Arguments: `{"city":"Hanoi"}`}}},
		inference.Message{Role: inference.RoleTool, ToolCallID: "call_1", Content: []inference.ContentPart{{Type: inference.ContentTypeText, Text: `{"temp":31}`}}},
	)
	return request
}

func conversationEvents() []inference.Event {
	return []inference.Event{
		{Kind: inference.EventTextDelta, Text: "Hel"},
		{Kind: inference.EventTextDelta, Text: "lo"},
		{Kind: inference.EventToolCallStart, ToolCall: &inference.ToolCallDelta{Index: 0, ID: "call_1", Name: "get_weather"}},
		{Kind: inference.EventToolCallDelta, ToolCall: &inference.ToolCallDelta{Index: 0, Arguments: `{"city":"Hanoi"}`}},
		{Kind: inference.EventToolCallEnd, ToolCall: &inference.ToolCallDelta{Index: 0, ID: "call_1", Name: "get_weather", Arguments: `{"city":"Hanoi"}`}},
		{Kind: inference.EventUsage, Usage: &inference.UsageReport{InputTokens: 7, OutputTokens: 3, CacheReadTokens: 1}},
		{Kind: inference.EventTerminal, Terminal: &inference.TerminalInfo{Reason: inference.EventReasonToolUse}},
	}
}

func TestResponsesEffortCeilingsOntoTheLadder(t *testing.T) {
	request := &inference.Request{
		Model:     "gpt-5",
		Messages:  []inference.Message{{Role: inference.RoleUser, Content: []inference.ContentPart{{Type: inference.ContentTypeText, Text: "hi"}}}},
		Reasoning: &inference.ReasoningConfig{Effort: "max"},
	}
	opts := wire.CodecOpts{CredentialRef: "sk-test", ReasoningEfforts: []string{"none", "low", "medium", "high", "xhigh"}}
	body := decodeBody(t, newTestRequest(t, request, opts))
	reasoning := body["reasoning"].(map[string]any)
	if reasoning["effort"] != "xhigh" {
		t.Fatalf("reasoning = %v, want xhigh, the highest listed level", reasoning)
	}
	request.Reasoning.Effort = "none"
	body = decodeBody(t, newTestRequest(t, request, opts))
	if body["reasoning"].(map[string]any)["effort"] != "none" {
		t.Fatalf("reasoning = %v, want none", body["reasoning"])
	}
}

func encodeBody(t *testing.T, request *inference.Request) map[string]any {
	t.Helper()
	return decodeBody(t, newTestRequest(t, request, wire.CodecOpts{CredentialRef: "sk-test"}))
}

func newTestRequest(t *testing.T, request *inference.Request, opts wire.CodecOpts) *http.Request {
	t.Helper()
	encoded, err := openairesponses.Module().EncodeRequest(request, opts)
	if err != nil {
		t.Fatalf("EncodeRequest() error = %v", err)
	}
	return encoded
}

func decodeBody(t *testing.T, request *http.Request) map[string]any {
	t.Helper()
	body, err := io.ReadAll(request.Body)
	if err != nil {
		t.Fatalf("read request body: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode request body: %v", err)
	}
	return payload
}

func decodeInbound(body []byte) (*inference.Request, error) {
	request, err := http.NewRequest(http.MethodPost, "/responses", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	return openairesponses.NewInbound().DecodeRequest(request)
}

func inboundRequest(t *testing.T, body []byte) *inference.Request {
	t.Helper()
	request, err := decodeInbound(body)
	if err != nil {
		t.Fatalf("DecodeRequest() error = %v", err)
	}
	return request
}

func encodeEvents(t *testing.T, inbound wire.InboundCodec, events []inference.Event) []wire.SSEEvent {
	t.Helper()
	var frames []wire.SSEEvent
	for _, event := range events {
		encoded, err := inbound.EncodeResponseEvent(event)
		if err != nil {
			t.Fatalf("EncodeResponseEvent(%s) error = %v", event.Kind, err)
		}
		frames = append(frames, encoded...)
	}
	return frames
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	body, err := os.ReadFile(testkit.FixturePath(name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return body
}

// pushStream feeds every frame of an SSE fixture through a decoder and
// adds the events Finish() produces.
func pushStream(t *testing.T, decoder wire.StreamDecoder, fixture string) []inference.Event {
	t.Helper()
	var events []inference.Event
	reader := sse.NewReader(bytes.NewReader(readFixture(t, fixture)))
	for {
		frame, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("read fixture %s: %v", fixture, err)
		}
		events = append(events, pushStreamData(t, decoder, frame.Data)...)
	}
	closing, err := decoder.Finish()
	if err != nil {
		t.Fatalf("Finish() error = %v", err)
	}
	return append(events, closing...)
}

func pushStreamData(t *testing.T, decoder wire.StreamDecoder, data string) []inference.Event {
	t.Helper()
	events, err := decoder.Push(wire.SSEEvent{Data: data})
	if err != nil {
		t.Fatalf("Push() error = %v", err)
	}
	return events
}

func textOf(events []inference.Event) string {
	var builder strings.Builder
	for _, event := range events {
		if event.Kind == inference.EventTextDelta {
			builder.WriteString(event.Text)
		}
	}
	return builder.String()
}

func reasoningTextOf(events []inference.Event) string {
	var builder strings.Builder
	for _, event := range events {
		if event.Kind == inference.EventReasoningDelta {
			builder.WriteString(event.Text)
		}
	}
	return builder.String()
}

func reasoningState(events []inference.Event) *inference.ReasoningConfig {
	for _, event := range events {
		if event.Reasoning != nil {
			return event.Reasoning
		}
	}
	return nil
}

// assembledCalls folds tool call fragments into complete calls, the way a
// consumer of the canonical events has to.
func assembledCalls(events []inference.Event) []*inference.ToolCallDelta {
	calls := map[int]*inference.ToolCallDelta{}
	var order []int
	for _, event := range events {
		switch event.Kind {
		case inference.EventToolCallStart:
			calls[event.ToolCall.Index] = &inference.ToolCallDelta{Index: event.ToolCall.Index, ID: event.ToolCall.ID, Name: event.ToolCall.Name}
			order = append(order, event.ToolCall.Index)
		case inference.EventToolCallDelta:
			calls[event.ToolCall.Index].Arguments += event.ToolCall.Arguments
		case inference.EventToolCallEnd:
			calls[event.ToolCall.Index].Arguments = event.ToolCall.Arguments
		}
	}
	assembled := make([]*inference.ToolCallDelta, 0, len(order))
	for _, index := range order {
		assembled = append(assembled, calls[index])
	}
	return assembled
}

func terminalReason(events []inference.Event) string {
	for _, event := range events {
		if event.Kind == inference.EventTerminal {
			return event.Terminal.Reason
		}
	}
	return ""
}

func lastUsage(events []inference.Event) *inference.UsageReport {
	for index := len(events) - 1; index >= 0; index-- {
		if events[index].Usage != nil {
			return events[index].Usage
		}
	}
	return nil
}

func countKind(events []inference.Event, kind inference.EventKind) int {
	count := 0
	for _, event := range events {
		if event.Kind == kind {
			count++
		}
	}
	return count
}

func countName(frames []wire.SSEEvent, name string) int {
	count := 0
	for _, frame := range frames {
		if frame.Name == name {
			count++
		}
	}
	return count
}

func frameNames(frames []wire.SSEEvent) []string {
	names := make([]string, 0, len(frames))
	for _, frame := range frames {
		names = append(names, frame.Name)
	}
	return names
}

func frameIndex(frames []wire.SSEEvent, name string) int {
	for i, frame := range frames {
		if frame.Name == name {
			return i
		}
	}
	return -1
}

func itemTypes(items []any) []string {
	types := make([]string, 0, len(items))
	for _, item := range items {
		if kind, found := item.(map[string]any)["type"].(string); found {
			types = append(types, kind)
		}
	}
	return types
}

func findItem(items []any, kind string) map[string]any {
	for _, item := range items {
		entry := item.(map[string]any)
		if entry["type"] == kind {
			return entry
		}
	}
	return nil
}

func indexOfItem(items []any, kind string) int {
	for index, item := range items {
		if item.(map[string]any)["type"] == kind {
			return index
		}
	}
	return -1
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func messageWithRole(messages []inference.Message, role string) *inference.Message {
	for index := range messages {
		if messages[index].Role == role {
			return &messages[index]
		}
	}
	return nil
}

func toolCalls(messages []inference.Message) []inference.ToolCall {
	var calls []inference.ToolCall
	for _, message := range messages {
		calls = append(calls, message.ToolCalls...)
	}
	return calls
}

func TestEncodeRequestBranches(t *testing.T) {
	t.Run("extra headers are applied", func(t *testing.T) {
		request := newTestRequest(t, canonicalRequest(true), wire.CodecOpts{
			CredentialRef: "sk-test",
			ExtraHeaders:  map[string]string{"X-Trace": "abc"},
		})
		if got := request.Header.Get("X-Trace"); got != "abc" {
			t.Fatalf("X-Trace = %q, want the extra header", got)
		}
	})

	t.Run("a reasoning item with no calls is appended", func(t *testing.T) {
		request := canonicalRequest(false)
		request.Tools = nil
		request.Reasoning = &inference.ReasoningConfig{ID: "rs_1", EncryptedContent: []byte(encryptedState)}
		items := encodeBody(t, request)["input"].([]any)
		if indexOfItem(items, "reasoning") != len(items)-1 {
			t.Fatalf("input = %v, want the reasoning item last", items)
		}
	})

	t.Run("several system prompts join into one instruction", func(t *testing.T) {
		request := canonicalRequest(false)
		first := inference.Message{Role: inference.RoleSystem, Content: []inference.ContentPart{{Type: inference.ContentTypeText, Text: "first"}}}
		request.Messages = append([]inference.Message{first}, request.Messages...)
		request.Messages = append(request.Messages, inference.Message{Role: inference.RoleSystem})
		if got := encodeBody(t, request)["instructions"]; got != "first\n\nbe brief" {
			t.Fatalf("instructions = %q, want both prompts", got)
		}
	})

	t.Run("an empty text part is left out", func(t *testing.T) {
		request := canonicalRequest(false)
		request.Messages = []inference.Message{{Role: inference.RoleAssistant, Content: []inference.ContentPart{
			{Type: inference.ContentTypeText},
			{Type: inference.ContentTypeText, Text: "visible"},
		}}}
		content := encodeBody(t, request)["input"].([]any)[0].(map[string]any)["content"].([]any)
		if len(content) != 1 || content[0].(map[string]any)["text"] != "visible" {
			t.Fatalf("content = %v, want only the visible sentence", content)
		}
	})

	t.Run("thinking parts are not sent as text", func(t *testing.T) {
		request := canonicalRequest(false)
		request.Messages = []inference.Message{{Role: inference.RoleAssistant, Content: []inference.ContentPart{
			{Type: inference.ContentTypeThinking, Text: "hidden", Signature: "sig"},
			{Type: inference.ContentTypeText, Text: "visible"},
		}}}
		items := encodeBody(t, request)["input"].([]any)
		content := items[0].(map[string]any)["content"].([]any)
		if len(content) != 1 || content[0].(map[string]any)["text"] != "visible" {
			t.Fatalf("content = %v, want only the text part", content)
		}
	})

	t.Run("a message with no content produces no item", func(t *testing.T) {
		request := canonicalRequest(false)
		request.Messages = []inference.Message{{Role: inference.RoleAssistant, ToolCalls: []inference.ToolCall{{ID: "call_1", Name: "lookup", Arguments: "{}"}}}}
		items := encodeBody(t, request)["input"].([]any)
		if len(items) != 1 || items[0].(map[string]any)["type"] != "function_call" {
			t.Fatalf("input = %v, want only the call", items)
		}
	})
}

func TestStreamDecodingBranches(t *testing.T) {
	t.Run("a failed response carries the upstream error", func(t *testing.T) {
		decoder := openairesponses.Module().NewStreamDecoder()
		data := `{"type":"response.failed","response":{"id":"resp_7","status":"failed","error":{"code":"server_error","message":"upstream is down"},"usage":{"input_tokens":3,"output_tokens":1}}}`
		events := pushStreamData(t, decoder, data)
		if countKind(events, inference.EventUsage) != 1 || countKind(events, inference.EventError) != 1 {
			t.Fatalf("events = %+v, want usage and the failure", events)
		}
		if events[1].Error.Code != "server_error" {
			t.Fatalf("error = %+v, want the upstream code", events[1].Error)
		}
	})

	t.Run("an error event without detail is ignored", func(t *testing.T) {
		events := pushStreamData(t, openairesponses.Module().NewStreamDecoder(), `{"type":"error"}`)
		if len(events) != 0 {
			t.Fatalf("events = %+v, want nothing", events)
		}
	})

	t.Run("an incomplete completion ends on a length terminal", func(t *testing.T) {
		decoder := openairesponses.Module().NewStreamDecoder()
		data := `{"type":"response.incomplete","response":{"id":"resp_8","status":"incomplete","incomplete_details":{"reason":"max_output_tokens"},"usage":{"input_tokens":4,"output_tokens":9}}}`
		events := pushStreamData(t, decoder, data)
		closing, err := decoder.Finish()
		if err != nil {
			t.Fatalf("Finish() error = %v", err)
		}
		if lastUsage(events) == nil || closing[0].Terminal.Reason != inference.EventReasonLength {
			t.Fatalf("closing = %+v, want a length terminal", closing)
		}
	})

	t.Run("empty deltas are ignored", func(t *testing.T) {
		decoder := openairesponses.Module().NewStreamDecoder()
		frames := []string{
			`{"type":"response.output_text.delta","delta":""}`,
			`{"type":"response.reasoning_summary_text.delta","delta":""}`,
		}
		for _, data := range frames {
			if events := pushStreamData(t, decoder, data); len(events) != 0 {
				t.Fatalf("push(%s) = %+v, want nothing", data, events)
			}
		}
	})

	t.Run("fragments for unknown calls are ignored", func(t *testing.T) {
		decoder := openairesponses.Module().NewStreamDecoder()
		frames := []string{
			`{"type":"response.function_call_arguments.delta","item_id":"fc_9","delta":"{}"}`,
			`{"type":"response.function_call_arguments.done","item_id":"fc_9","arguments":"{}"}`,
		}
		for _, data := range frames {
			if events := pushStreamData(t, decoder, data); len(events) != 0 {
				t.Fatalf("push(%s) = %+v, want nothing", data, events)
			}
		}
	})

	t.Run("a call identified by call id alone still assembles", func(t *testing.T) {
		decoder := openairesponses.Module().NewStreamDecoder()
		start := pushStreamData(t, decoder, `{"type":"response.output_item.added","item":{"type":"function_call","call_id":"call_7","name":"lookup"}}`)
		if len(start) != 1 || start[0].ToolCall.ID != "call_7" || start[0].ToolCall.Name != "lookup" {
			t.Fatalf("start = %+v, want the call identity", start)
		}
		pushStreamData(t, decoder, `{"type":"response.function_call_arguments.delta","item_id":"call_7","delta":"{}"}`)
		done := pushStreamData(t, decoder, `{"type":"response.output_item.done","item":{"type":"function_call","call_id":"call_7","name":"lookup","arguments":"{\"q\":1}"}}`)
		if len(done) != 1 || done[0].Kind != inference.EventToolCallEnd || done[0].ToolCall.Arguments != `{"q":1}` {
			t.Fatalf("done = %+v, want the completed call", done)
		}
	})

	t.Run("a call identified by item id alone still assembles", func(t *testing.T) {
		start := pushStreamData(t, openairesponses.Module().NewStreamDecoder(), `{"type":"response.output_item.added","item":{"type":"function_call","id":"fc_9","name":"lookup"}}`)
		if len(start) != 1 || start[0].ToolCall.ID != "fc_9" {
			t.Fatalf("start = %+v, want the item id as the call id", start)
		}
	})

	t.Run("reasoning items without encrypted content are ignored", func(t *testing.T) {
		events := pushStreamData(t, openairesponses.Module().NewStreamDecoder(), `{"type":"response.output_item.done","item":{"type":"reasoning","id":"rs_1","summary":[]}}`)
		if len(events) != 0 {
			t.Fatalf("events = %+v, want nothing", events)
		}
	})

	t.Run("a completed event without a response body is ignored", func(t *testing.T) {
		events := pushStreamData(t, openairesponses.Module().NewStreamDecoder(), `{"type":"response.completed"}`)
		if len(events) != 0 {
			t.Fatalf("events = %+v, want nothing", events)
		}
	})

	t.Run("unknown output item types are skipped", func(t *testing.T) {
		body := []byte(`{"id":"resp_9","status":"completed","output":[{"type":"web_search_call","id":"ws_1"}]}`)
		events, err := openairesponses.Module().DecodeResponse(body)
		if err != nil {
			t.Fatalf("DecodeResponse() error = %v", err)
		}
		if countKind(events, inference.EventTextDelta)+countKind(events, inference.EventToolCallStart) != 0 {
			t.Fatalf("events = %+v, want only the terminal", events)
		}
	})
}

func TestDecodeErrorBranches(t *testing.T) {
	t.Run("an error body without a code uses the type", func(t *testing.T) {
		info := openairesponses.Module().DecodeError(400, []byte(`{"error":{"message":"bad","type":"invalid_request_error"}}`))
		if info.Code != "invalid_request_error" {
			t.Fatalf("info = %+v, want the error type as the code", info)
		}
	})

	t.Run("an error body with neither code nor type uses the status text", func(t *testing.T) {
		info := openairesponses.Module().DecodeError(400, []byte(`{"error":{"message":"bad"}}`))
		if info.Code != "Bad Request" {
			t.Fatalf("info = %+v, want the status text as the code", info)
		}
	})
}

func TestInboundBranches(t *testing.T) {
	t.Run("reasoning summary text is rendered with the upstream item id", func(t *testing.T) {
		frames := encodeEvents(t, openairesponses.NewInbound(), []inference.Event{
			{Kind: inference.EventReasoningDelta, Reasoning: &inference.ReasoningConfig{ID: "rs_9"}},
			{Kind: inference.EventReasoningDelta, Text: "thinking"},
		})
		partAt := frameIndex(frames, "response.reasoning_summary_part.added")
		deltaAt := frameIndex(frames, "response.reasoning_summary_text.delta")
		if partAt < 0 || deltaAt < partAt || !strings.Contains(frames[deltaAt].Data, "rs_9") {
			t.Fatalf("frames = %v, want the summary part before the delta", frameNames(frames))
		}
	})

	t.Run("reasoning summary text is stored on the response output", func(t *testing.T) {
		events := []inference.Event{
			{Kind: inference.EventReasoningDelta, Text: "think"},
			{Kind: inference.EventReasoningDelta, Text: "ing"},
			{Kind: inference.EventTerminal, Terminal: &inference.TerminalInfo{Reason: inference.EventReasonStop}},
		}
		frames := encodeEvents(t, openairesponses.NewInbound(), events)
		if countName(frames, "response.output_item.added") != 1 || countName(frames, "response.output_item.done") != 1 {
			t.Fatalf("frames = %v, want the reasoning item opened and closed", frameNames(frames))
		}
		textDone := frameIndex(frames, "response.reasoning_summary_text.done")
		partDone := frameIndex(frames, "response.reasoning_summary_part.done")
		itemDone := frameIndex(frames, "response.output_item.done")
		if textDone < 0 || partDone < textDone || itemDone < partDone {
			t.Fatalf("frames = %v, want the summary text and part closed before the item", frameNames(frames))
		}
		payload, err := openairesponses.NewInbound().EncodeResponse(events)
		if err != nil {
			t.Fatalf("EncodeResponse() error = %v", err)
		}
		var object map[string]any
		if err := json.Unmarshal(payload, &object); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		items, _ := object["output"].([]any)
		if len(items) != 1 {
			t.Fatalf("output = %v, want the reasoning item", items)
		}
		item, _ := items[0].(map[string]any)
		summary, _ := item["summary"].([]any)
		if item["type"] != "reasoning" || len(summary) != 1 {
			t.Fatalf("item = %v, want a reasoning summary", item)
		}
		part, _ := summary[0].(map[string]any)
		if part["text"] != "thinking" {
			t.Fatalf("summary = %v, want the joined text", summary)
		}
	})

	t.Run("a reasoning summary attaches to the following assistant message", func(t *testing.T) {
		body := []byte(`{"model":"gpt-5","input":[{"type":"reasoning","id":"rs_1","summary":[{"type":"summary_text","text":"look it up"}]},{"type":"function_call","call_id":"c1","name":"lookup","arguments":"{}"}]}`)
		request := inboundRequest(t, body)
		if len(request.Messages) != 1 || len(request.Messages[0].ToolCalls) != 1 {
			t.Fatalf("messages = %+v, want the assistant call", request.Messages)
		}
		part := request.Messages[0].Content
		if len(part) != 1 || part[0].Type != inference.ContentTypeThinking || part[0].Text != "look it up" {
			t.Fatalf("content = %+v, want the reasoning summary", part)
		}
	})

	t.Run("reasoning text without an item id uses a generated one", func(t *testing.T) {
		frames := encodeEvents(t, openairesponses.NewInbound(), []inference.Event{{Kind: inference.EventReasoningDelta, Text: "thinking"}})
		last := frames[len(frames)-1]
		if last.Name != "response.reasoning_summary_text.delta" || !strings.Contains(last.Data, "rs_") {
			t.Fatalf("frame = %s, want a generated item id", last.Data)
		}
	})

	t.Run("later reasoning items reuse the rendered index", func(t *testing.T) {
		frames := encodeEvents(t, openairesponses.NewInbound(), []inference.Event{
			{Kind: inference.EventReasoningDelta, Reasoning: &inference.ReasoningConfig{ID: "rs_1", EncryptedContent: []byte(encryptedState)}},
			{Kind: inference.EventReasoningDelta, Reasoning: &inference.ReasoningConfig{ID: "rs_2", EncryptedContent: []byte("second")}},
		})
		if countName(frames, "response.output_item.done") != 2 {
			t.Fatalf("frames = %v, want both items", frameNames(frames))
		}
	})

	t.Run("a repeated call start reuses the call", func(t *testing.T) {
		start := inference.Event{Kind: inference.EventToolCallStart, ToolCall: &inference.ToolCallDelta{Index: 0, ID: "call_1", Name: "lookup"}}
		frames := encodeEvents(t, openairesponses.NewInbound(), []inference.Event{start, start})
		if countName(frames, "response.output_item.added") != 2 {
			t.Fatalf("frames = %v, want both starts", frameNames(frames))
		}
	})

	t.Run("a developer message becomes the system prompt", func(t *testing.T) {
		body := []byte(`{"model":"gpt-5","input":[{"role":"developer","content":[{"type":"input_text","text":"rules"}]}]}`)
		request := inboundRequest(t, body)
		if request.Messages[0].Role != inference.RoleSystem || request.Messages[0].Content[0].Text != "rules" {
			t.Fatalf("messages = %+v, want a system message", request.Messages)
		}
	})

	t.Run("an item without a role is a user message", func(t *testing.T) {
		request := inboundRequest(t, []byte(`{"model":"gpt-5","input":[{"content":[{"type":"input_text","text":"hi"}]}]}`))
		if request.Messages[0].Role != inference.RoleUser {
			t.Fatalf("messages = %+v, want a user message", request.Messages)
		}
	})

	t.Run("a second call extends the assistant turn", func(t *testing.T) {
		body := []byte(`{"model":"gpt-5","input":[{"type":"function_call","call_id":"c1","name":"a","arguments":"{}"},{"type":"function_call","call_id":"c2","name":"b","arguments":"{}"}]}`)
		request := inboundRequest(t, body)
		if len(request.Messages) != 1 || len(request.Messages[0].ToolCalls) != 2 {
			t.Fatalf("messages = %+v, want one assistant turn with two calls", request.Messages)
		}
	})

	t.Run("reasoning items without encrypted content are skipped", func(t *testing.T) {
		body := []byte(`{"model":"gpt-5","input":[{"type":"reasoning","id":"rs_1","summary":[]},{"role":"user","content":[{"type":"input_text","text":"hi"}]}]}`)
		if request := inboundRequest(t, body); request.Reasoning != nil {
			t.Fatalf("reasoning = %+v, want none", request.Reasoning)
		}
	})

	t.Run("reasoning effort without state keeps the effort", func(t *testing.T) {
		request := inboundRequest(t, []byte(`{"model":"gpt-5","reasoning":{"effort":"high"},"input":"hi"}`))
		if request.Reasoning == nil || request.Reasoning.Effort != "high" || len(request.Reasoning.EncryptedContent) != 0 {
			t.Fatalf("reasoning = %+v, want the effort alone", request.Reasoning)
		}
	})

	t.Run("an input that is neither text nor a list is rejected", func(t *testing.T) {
		_, err := decodeInbound([]byte(`{"model":"gpt-5","input":5}`))
		if !errors.Is(err, wire.ErrInvalidRequest) {
			t.Fatalf("DecodeRequest() error = %v, want the invalid request sentinel", err)
		}
	})

	t.Run("a terminal without content still closes the response", func(t *testing.T) {
		events := []inference.Event{{Kind: inference.EventTerminal, Terminal: &inference.TerminalInfo{Reason: inference.EventReasonStop}}}
		frames := encodeEvents(t, openairesponses.NewInbound(), events)
		if names := frameNames(frames); len(names) != 3 || names[2] != "response.completed" {
			t.Fatalf("frames = %v, want created, in_progress, completed", names)
		}
	})

	t.Run("a length terminal carries the incomplete details", func(t *testing.T) {
		events := []inference.Event{{Kind: inference.EventTerminal, Terminal: &inference.TerminalInfo{Reason: inference.EventReasonLength}}}
		frames := encodeEvents(t, openairesponses.NewInbound(), events)
		last := frames[len(frames)-1]
		if last.Name != "response.completed" {
			t.Fatalf("last frame = %q, want the terminal response", last.Name)
		}
		var frame map[string]any
		if err := json.Unmarshal([]byte(last.Data), &frame); err != nil {
			t.Fatalf("decode frame: %v", err)
		}
		response := frame["response"].(map[string]any)
		details := response["incomplete_details"].(map[string]any)
		if response["status"] != "incomplete" || details["reason"] != "max_output_tokens" {
			t.Fatalf("response = %v, want the incomplete details", response)
		}
	})

	t.Run("an error terminal reports a failed response", func(t *testing.T) {
		events := []inference.Event{
			{Kind: inference.EventError, Error: &inference.ErrorInfo{Code: "stream_error", Message: "gone", Status: 502}},
			{Kind: inference.EventTerminal, Terminal: &inference.TerminalInfo{Reason: inference.EventReasonError}},
		}
		frames := encodeEvents(t, openairesponses.NewInbound(), events)
		last := frames[len(frames)-1]
		if last.Name != "response.failed" || !strings.Contains(last.Data, "failed") {
			t.Fatalf("frame = %s, want a failed response", last.Data)
		}
	})

	t.Run("a failure event without detail still opens the stream", func(t *testing.T) {
		frames, err := openairesponses.NewInbound().EncodeResponseEvent(inference.Event{Kind: inference.EventError})
		if err != nil {
			t.Fatalf("EncodeResponseEvent() error = %v", err)
		}
		if names := frameNames(frames); len(names) != 2 {
			t.Fatalf("frames = %v, want the opening frames only", names)
		}
	})

	t.Run("tool call deltas without an index are ignored", func(t *testing.T) {
		frames, err := openairesponses.NewInbound().EncodeResponseEvent(inference.Event{Kind: inference.EventToolCallDelta})
		if err != nil {
			t.Fatalf("EncodeResponseEvent() error = %v", err)
		}
		if len(frames) != 0 {
			t.Fatalf("frames = %v, want nothing", frameNames(frames))
		}
	})

	t.Run("the non-streaming shape rejects an unknown event", func(t *testing.T) {
		_, err := openairesponses.NewInbound().EncodeResponse([]inference.Event{{Kind: inference.EventKind(99)}})
		if !errors.Is(err, wire.ErrUnknownEvent) {
			t.Fatalf("EncodeResponse() error = %v, want the unknown event sentinel", err)
		}
	})
}
