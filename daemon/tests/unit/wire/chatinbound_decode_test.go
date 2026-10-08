package codec_test

import (
	"errors"
	"github.com/jonaskahn/relo/internal/inference"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/wire"
	"github.com/jonaskahn/relo/internal/adapters/wire/openaichat"
)

func TestChatCompletionsDecodeRequest(t *testing.T) {
	t.Run("string content becomes a text part", func(t *testing.T) {
		body := `{"model":"gpt-4o","stream":true,"messages":[{"role":"user","content":"hi"}]}`
		request, err := openaichat.NewChatCompletionsCodec().DecodeRequest(newRequest(t, body))
		if err != nil {
			t.Fatalf("DecodeRequest() error = %v", err)
		}
		if request.Model != "gpt-4o" || !request.Stream {
			t.Fatalf("request = %+v, want the model and stream flag", request)
		}
		want := []inference.ContentPart{{Type: inference.ContentTypeText, Text: "hi"}}
		if len(request.Messages) != 1 || !equalParts(request.Messages[0].Content, want) {
			t.Fatalf("content = %+v, want %+v", request.Messages, want)
		}
	})

	t.Run("array content keeps text and image parts", func(t *testing.T) {
		body := `{"model":"gpt-4o","messages":[{"role":"user","content":[{"type":"text","text":"look"},{"type":"image_url","image_url":{"url":"https://example.invalid/a.png"}}]}]}`
		request, err := openaichat.NewChatCompletionsCodec().DecodeRequest(newRequest(t, body))
		if err != nil {
			t.Fatalf("DecodeRequest() error = %v", err)
		}
		parts := request.Messages[0].Content
		if len(parts) != 2 || parts[0].Text != "look" || parts[1].ImageURL != "https://example.invalid/a.png" {
			t.Fatalf("content = %+v, want a text part and an image part", parts)
		}
		if parts[1].Type != inference.ContentTypeImage {
			t.Fatalf("part type = %q, want %q", parts[1].Type, inference.ContentTypeImage)
		}
	})

	t.Run("tool calls and tools are decoded", func(t *testing.T) {
		body := `{"model":"gpt-4o","messages":[{"role":"assistant","content":"","tool_calls":[{"id":"call_1","type":"function","function":{"name":"lookup","arguments":"{}"}}]},{"role":"tool","content":"42","tool_call_id":"call_1"}],"tools":[{"type":"function","function":{"name":"lookup","description":"look it up","parameters":{"type":"object"}}}]}`
		request, err := openaichat.NewChatCompletionsCodec().DecodeRequest(newRequest(t, body))
		if err != nil {
			t.Fatalf("DecodeRequest() error = %v", err)
		}
		if len(request.Tools) != 1 || request.Tools[0].Name != "lookup" {
			t.Fatalf("tools = %+v, want the lookup tool", request.Tools)
		}
		call := request.Messages[0].ToolCalls[0]
		if call.ID != "call_1" || call.Name != "lookup" || call.Arguments != "{}" {
			t.Fatalf("tool call = %+v, want the decoded call", call)
		}
		if request.Messages[1].ToolCallID != "call_1" || request.Messages[1].Role != inference.RoleTool {
			t.Fatalf("tool message = %+v", request.Messages[1])
		}
	})

	t.Run("reasoning_effort is accepted alongside the nested object", func(t *testing.T) {
		body := `{"model":"gpt-4o","messages":[],"reasoning_effort":"high"}`
		request, err := openaichat.NewChatCompletionsCodec().DecodeRequest(newRequest(t, body))
		if err != nil {
			t.Fatalf("DecodeRequest() error = %v", err)
		}
		if request.Reasoning == nil || request.Reasoning.Effort != "high" {
			t.Fatalf("Reasoning = %+v, want the top-level effort", request.Reasoning)
		}
		withoutEffort := `{"model":"gpt-4o","messages":[]}`
		plain, err := openaichat.NewChatCompletionsCodec().DecodeRequest(newRequest(t, withoutEffort))
		if err != nil {
			t.Fatalf("DecodeRequest() error = %v", err)
		}
		if plain.Reasoning != nil {
			t.Fatalf("Reasoning = %+v, want none", plain.Reasoning)
		}
	})

	t.Run("max completion tokens wins over max tokens", func(t *testing.T) {
		body := `{"model":"gpt-4o","messages":[],"max_tokens":10,"max_completion_tokens":20,"temperature":0.5,"reasoning":{"effort":"low"}}`
		request, err := openaichat.NewChatCompletionsCodec().DecodeRequest(newRequest(t, body))
		if err != nil {
			t.Fatalf("DecodeRequest() error = %v", err)
		}
		if request.MaxTokens != 20 {
			t.Fatalf("MaxTokens = %d, want 20", request.MaxTokens)
		}
		if request.Temperature == nil || *request.Temperature != 0.5 {
			t.Fatalf("Temperature = %v, want 0.5", request.Temperature)
		}
		if request.Reasoning == nil || request.Reasoning.Effort != "low" {
			t.Fatalf("Reasoning = %+v, want effort low", request.Reasoning)
		}
	})

	t.Run("max tokens is used when the completion limit is absent", func(t *testing.T) {
		body := `{"model":"gpt-4o","messages":[],"max_tokens":10}`
		request, err := openaichat.NewChatCompletionsCodec().DecodeRequest(newRequest(t, body))
		if err != nil {
			t.Fatalf("DecodeRequest() error = %v", err)
		}
		if request.MaxTokens != 10 {
			t.Fatalf("MaxTokens = %d, want 10", request.MaxTokens)
		}
	})

	t.Run("missing model is rejected", func(t *testing.T) {
		_, err := openaichat.NewChatCompletionsCodec().DecodeRequest(newRequest(t, `{"messages":[]}`))
		if !errors.Is(err, wire.ErrInvalidRequest) {
			t.Fatalf("DecodeRequest() error = %v, want %v", err, wire.ErrInvalidRequest)
		}
	})

	t.Run("malformed json is rejected", func(t *testing.T) {
		_, err := openaichat.NewChatCompletionsCodec().DecodeRequest(newRequest(t, "{not json"))
		if !errors.Is(err, wire.ErrInvalidRequest) {
			t.Fatalf("DecodeRequest() error = %v, want %v", err, wire.ErrInvalidRequest)
		}
	})

	t.Run("unsupported content part is rejected", func(t *testing.T) {
		body := `{"model":"gpt-4o","messages":[{"role":"user","content":[{"type":"audio","text":"x"}]}]}`
		_, err := openaichat.NewChatCompletionsCodec().DecodeRequest(newRequest(t, body))
		if !errors.Is(err, wire.ErrInvalidRequest) {
			t.Fatalf("DecodeRequest() error = %v, want %v", err, wire.ErrInvalidRequest)
		}
	})

	t.Run("numeric content is rejected", func(t *testing.T) {
		body := `{"model":"gpt-4o","messages":[{"role":"user","content":42}]}`
		_, err := openaichat.NewChatCompletionsCodec().DecodeRequest(newRequest(t, body))
		if !errors.Is(err, wire.ErrInvalidRequest) {
			t.Fatalf("DecodeRequest() error = %v, want %v", err, wire.ErrInvalidRequest)
		}
	})

	t.Run("reasoning_content becomes a thinking part", func(t *testing.T) {
		body := `{"model":"deepseek-flash","messages":[{"role":"assistant","content":"answer","reasoning_content":"weighing options"}]}`
		request, err := openaichat.NewChatCompletionsCodec().DecodeRequest(newRequest(t, body))
		if err != nil {
			t.Fatalf("DecodeRequest() error = %v", err)
		}
		want := []inference.ContentPart{
			{Type: inference.ContentTypeThinking, Text: "weighing options"},
			{Type: inference.ContentTypeText, Text: "answer"},
		}
		if len(request.Messages) != 1 || !equalParts(request.Messages[0].Content, want) {
			t.Fatalf("content = %+v, want %+v", request.Messages[0].Content, want)
		}
	})

	t.Run("message without content is accepted", func(t *testing.T) {
		body := `{"model":"gpt-4o","messages":[{"role":"assistant"}]}`
		request, err := openaichat.NewChatCompletionsCodec().DecodeRequest(newRequest(t, body))
		if err != nil {
			t.Fatalf("DecodeRequest() error = %v", err)
		}
		if len(request.Messages[0].Content) != 0 {
			t.Fatalf("content = %+v, want none", request.Messages[0].Content)
		}
	})
}

func newRequest(t *testing.T, body string) *http.Request {
	t.Helper()
	return httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
}

func equalParts(got, want []inference.ContentPart) bool {
	if len(got) != len(want) {
		return false
	}
	for index := range got {
		if got[index] != want[index] {
			return false
		}
	}
	return true
}
