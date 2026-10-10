package google_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"github.com/jonaskahn/relo/internal/inference"
	"io"
	"net/http"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/antigravity"
	"github.com/jonaskahn/relo/internal/adapters/wire"
	"github.com/jonaskahn/relo/internal/adapters/wire/google"
	"github.com/jonaskahn/relo/internal/adapters/wire/sse"
	"github.com/jonaskahn/relo/tests/testkit"
)

const schemaWithRejectedKeywords = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "WeatherArgs",
  "type": "object",
  "additionalProperties": false,
  "required": ["city"],
  "properties": {
    "city": {"type": "string", "description": "the city", "title": "City"},
    "title": {"type": "string"},
    "nested": {"type": "array", "items": {"type": "object", "additionalProperties": false, "properties": {
      "width": {"type": "integer", "exclusiveMinimum": 0, "exclusiveMaximum": 100, "minimum": 1, "maximum": 99}
    }}}
  }
}`

func TestEncodeRequest(t *testing.T) {
	t.Run("AI Studio sends the key in its own header", func(t *testing.T) {
		request := newTestRequest(t, canonicalRequest(true), wire.CodecOpts{CredentialRef: "api-key-1"})
		if request.URL.Path != "/v1beta/models/gemini-2.0-flash:streamGenerateContent" {
			t.Fatalf("path = %s, want the AI Studio path", request.URL.Path)
		}
		if got := request.Header.Get(google.APIKeyHeader); got != "api-key-1" {
			t.Fatalf("key header = %q, want the credential", got)
		}
		if request.URL.Query().Get("key") != "" {
			t.Fatalf("key in query = %q, want the credential kept out of the URL", request.URL.Query().Get("key"))
		}
		if got := request.URL.Query().Get("alt"); got != "sse" {
			t.Fatalf("alt = %q, want the streaming flag", got)
		}
		if got := request.Header.Get("Authorization"); got != "" {
			t.Fatalf("Authorization = %q, want none for AI Studio", got)
		}
	})

	t.Run("Vertex uses the express path with a key", func(t *testing.T) {
		vertex := google.NewCodec(google.Config{Mode: google.ModeVertex})
		request := encode(t, vertex, canonicalRequest(true), wire.CodecOpts{CredentialRef: "api-key-1"})
		want := "https://aiplatform.googleapis.com/v1/publishers/google/models/gemini-2.0-flash:streamGenerateContent"
		if request.URL.Scheme+"://"+request.URL.Host+request.URL.Path != want {
			t.Fatalf("url = %s, want %s", request.URL, want)
		}
		if got := request.Header.Get(google.APIKeyHeader); got != "api-key-1" {
			t.Fatalf("key header = %q, want the credential", got)
		}
	})

	t.Run("Cloud Code Assist wraps the payload in its envelope", func(t *testing.T) {
		body := encodeBody(t, canonicalRequest(false), wire.CodecOpts{
			CredentialRef: "access-token",
			AuthMethod:    wire.AuthOAuth,
			Mode:          string(google.ModeCloudCodeAssist),
			Project:       "antigravity-project",
		})
		if body["project"] != "antigravity-project" || body["model"] != "gemini-2.0-flash" {
			t.Fatalf("body = %v, want the envelope identity", body)
		}
		inner, ok := body["request"].(map[string]any)
		if !ok || inner["contents"] == nil {
			t.Fatalf("body = %v, want the request inside the envelope", body)
		}
	})

	t.Run("Antigravity projectId injection from the request options", func(t *testing.T) {
		request := newTestRequest(t, canonicalRequest(false), wire.CodecOpts{
			CredentialRef: "access-token",
			AuthMethod:    wire.AuthOAuth,
			Mode:          string(google.ModeCloudCodeAssist),
			Project:       "antigravity-project",
		})
		if request.URL.Path != "/v1internal:generateContent" {
			t.Fatalf("path = %s, want the internal endpoint", request.URL.Path)
		}
		if got := request.Header.Get("Authorization"); got != "Bearer access-token" {
			t.Fatalf("Authorization = %q, want the bearer token", got)
		}
	})

	t.Run("the mode can be overridden per request", func(t *testing.T) {
		request := encode(t, google.Module(), canonicalRequest(false), wire.CodecOpts{
			CredentialRef: "access-token",
			Mode:          string(google.ModeVertex),
			Project:       "proj-1",
			BaseURL:       "http://127.0.0.1:9999",
		})
		want := "/publishers/google/models/gemini-2.0-flash:generateContent"
		if request.URL.Path != want {
			t.Fatalf("path = %s, want the Vertex path from the request settings", request.URL.Path)
		}
	})

	t.Run("an unknown mode is rejected", func(t *testing.T) {
		opts := wire.CodecOpts{CredentialRef: "api-key-1", Mode: "gemini-web"}
		_, err := google.Module().EncodeRequest(canonicalRequest(false), opts)
		if !errors.Is(err, google.ErrInvalidGoogleMode) {
			t.Fatalf("EncodeRequest() error = %v, want the invalid mode sentinel", err)
		}
	})

	t.Run("a keyless endpoint needs no credential", func(t *testing.T) {
		keyless := google.NewCodec(google.Config{Mode: google.ModeAIStudio})
		if _, err := keyless.EncodeRequest(canonicalRequest(false), wire.CodecOpts{AuthMethod: wire.AuthNone}); err != nil {
			t.Fatalf("EncodeRequest() error = %v, want a keyless request to encode", err)
		}
		if _, err := keyless.EncodeRequest(canonicalRequest(false), wire.CodecOpts{}); !errors.Is(err, wire.ErrMissingCredential) {
			t.Fatalf("EncodeRequest() error = %v, want the missing credential sentinel", err)
		}
	})

	t.Run("Cloud Code Assist without a project is rejected", func(t *testing.T) {
		code := google.NewCodec(google.Config{Mode: google.ModeCloudCodeAssist})
		if _, err := code.EncodeRequest(canonicalRequest(false), wire.CodecOpts{CredentialRef: "token"}); !errors.Is(err, google.ErrMissingProject) {
			t.Fatalf("EncodeRequest() error = %v, want the missing project sentinel", err)
		}
	})

	t.Run("missing credential is rejected", func(t *testing.T) {
		if _, err := google.Module().EncodeRequest(canonicalRequest(false), wire.CodecOpts{}); !errors.Is(err, wire.ErrMissingCredential) {
			t.Fatalf("EncodeRequest() error = %v, want the missing credential sentinel", err)
		}
	})
}

func TestEncodePayload(t *testing.T) {
	t.Run("messages become user and model contents", func(t *testing.T) {
		body := encodeBody(t, conversationRequest(), wire.CodecOpts{CredentialRef: "api-key-1"})
		system := body["systemInstruction"].(map[string]any)
		if system["parts"].([]any)[0].(map[string]any)["text"] != "be brief" {
			t.Fatalf("systemInstruction = %v, want the system prompt", system)
		}
		contents := body["contents"].([]any)
		if len(contents) != 3 {
			t.Fatalf("contents = %v, want one entry per turn", contents)
		}
		if contents[0].(map[string]any)["role"] != "user" || contents[1].(map[string]any)["role"] != "model" {
			t.Fatalf("contents = %v, want user and model roles", contents)
		}
		call := contents[1].(map[string]any)["parts"].([]any)[0].(map[string]any)["functionCall"].(map[string]any)
		if call["name"] != "get_weather" {
			t.Fatalf("functionCall = %v, want the call", call)
		}
		result := contents[2].(map[string]any)["parts"].([]any)[0].(map[string]any)["functionResponse"].(map[string]any)
		if result["name"] != "get_weather" {
			t.Fatalf("functionResponse = %v, want the function name from the call", result)
		}
		if result["response"].(map[string]any)["temp"] != float64(31) {
			t.Fatalf("functionResponse = %v, want the JSON result passed through", result)
		}
	})

	t.Run("a tool result that is not JSON is wrapped", func(t *testing.T) {
		request := canonicalRequest(false)
		request.Messages = append(request.Messages,
			inference.Message{Role: inference.RoleAssistant, ToolCalls: []inference.ToolCall{{ID: "call_1", Name: "get_weather"}}},
			inference.Message{Role: inference.RoleTool, ToolCallID: "call_1", Content: []inference.ContentPart{{Type: inference.ContentTypeText, Text: "31C"}}},
		)
		body := encodeBody(t, request, wire.CodecOpts{CredentialRef: "api-key-1"})
		contents := body["contents"].([]any)
		response := contents[len(contents)-1].(map[string]any)["parts"].([]any)[0].(map[string]any)["functionResponse"].(map[string]any)
		if response["name"] != "get_weather" || response["response"].(map[string]any)["result"] != "31C" {
			t.Fatalf("functionResponse = %v, want the wrapped result", response)
		}
	})

	t.Run("an unnamed tool result falls back to its call id", func(t *testing.T) {
		request := canonicalRequest(false)
		request.Messages = append(request.Messages, inference.Message{
			Role:       inference.RoleTool,
			ToolCallID: "call_9",
			Content:    []inference.ContentPart{{Type: inference.ContentTypeText, Text: "done"}},
		})
		body := encodeBody(t, request, wire.CodecOpts{CredentialRef: "api-key-1"})
		contents := body["contents"].([]any)
		response := contents[len(contents)-1].(map[string]any)["parts"].([]any)[0].(map[string]any)["functionResponse"].(map[string]any)
		if response["name"] != "call_9" {
			t.Fatalf("functionResponse = %v, want the call id as the name", response)
		}
	})

	t.Run("a base64 data URL becomes inline data", func(t *testing.T) {
		request := canonicalRequest(false)
		request.Messages = append(request.Messages, inference.Message{Role: inference.RoleUser, Content: []inference.ContentPart{
			{Type: inference.ContentTypeImage, ImageURL: "data:image/png;base64,aGVsbG8="},
		}})
		inline := lastParts(t, request)[0].(map[string]any)["inlineData"].(map[string]any)
		if inline["mimeType"] != "image/png" || inline["data"] != "aGVsbG8=" {
			t.Fatalf("inlineData = %v, want the decoded data URL", inline)
		}
	})

	t.Run("image input unsupported -> typed error", func(t *testing.T) {
		request := canonicalRequest(false)
		request.Messages = append(request.Messages, inference.Message{Role: inference.RoleUser, Content: []inference.ContentPart{
			{Type: inference.ContentTypeImage, ImageURL: "https://example.invalid/a.png"},
		}})
		if _, err := google.Module().EncodeRequest(request, wire.CodecOpts{CredentialRef: "api-key-1"}); !errors.Is(err, wire.ErrUnsupportedFeature) {
			t.Fatalf("EncodeRequest() error = %v, want the unsupported feature sentinel", err)
		}
	})

	t.Run("a data URL without base64 is unsupported too", func(t *testing.T) {
		request := canonicalRequest(false)
		request.Messages = append(request.Messages, inference.Message{Role: inference.RoleUser, Content: []inference.ContentPart{
			{Type: inference.ContentTypeImage, ImageURL: "data:image/png,plain"},
		}})
		if _, err := google.Module().EncodeRequest(request, wire.CodecOpts{CredentialRef: "api-key-1"}); !errors.Is(err, wire.ErrUnsupportedFeature) {
			t.Fatalf("EncodeRequest() error = %v, want the unsupported feature sentinel", err)
		}
	})

	t.Run("reasoning config becomes a thinking budget", func(t *testing.T) {
		request := canonicalRequest(false)
		request.Reasoning = &inference.ReasoningConfig{Effort: "high"}
		config := encodeBody(t, request, wire.CodecOpts{CredentialRef: "api-key-1"})["generationConfig"].(map[string]any)
		thinking := config["thinkingConfig"].(map[string]any)
		if thinking["thinkingBudget"] != float64(16384) || thinking["includeThoughts"] != true {
			t.Fatalf("thinkingConfig = %v, want the high budget with summaries", thinking)
		}
	})

	t.Run("an effort list becomes a thinking level", func(t *testing.T) {
		request := canonicalRequest(false)
		request.Model = "gemini-3.1-pro"
		request.Reasoning = &inference.ReasoningConfig{Effort: "xhigh"}
		config := encodeBody(t, request, wire.CodecOpts{
			CredentialRef: "api-key-1", ReasoningEfforts: []string{"low", "medium", "high"},
		})["generationConfig"].(map[string]any)
		thinking := config["thinkingConfig"].(map[string]any)
		if thinking["thinkingLevel"] != "high" || thinking["thinkingBudget"] != nil {
			t.Fatalf("thinkingConfig = %v, want level high and no budget", thinking)
		}
	})

	t.Run("an unknown effort is omitted and max keeps its budget", func(t *testing.T) {
		request := canonicalRequest(false)
		request.Reasoning = &inference.ReasoningConfig{Effort: "turbo"}
		config := encodeBody(t, request, wire.CodecOpts{CredentialRef: "api-key-1"})["generationConfig"].(map[string]any)
		if config["thinkingConfig"] != nil {
			t.Fatalf("thinkingConfig = %v, want the field omitted", config["thinkingConfig"])
		}
		request.Reasoning.Effort = "max"
		config = encodeBody(t, request, wire.CodecOpts{CredentialRef: "api-key-1"})["generationConfig"].(map[string]any)
		if config["thinkingConfig"].(map[string]any)["thinkingBudget"] != float64(32000) {
			t.Fatalf("thinkingConfig = %v, want the max budget", config["thinkingConfig"])
		}
	})

	t.Run("a none effort turns thinking off through the toggle", func(t *testing.T) {
		request := canonicalRequest(false)
		request.Reasoning = &inference.ReasoningConfig{Effort: "none"}
		config := encodeBody(t, request, wire.CodecOpts{
			CredentialRef: "api-key-1", ReasoningEfforts: []string{"low", "high"}, ReasoningToggle: true,
		})["generationConfig"].(map[string]any)
		thinking := config["thinkingConfig"].(map[string]any)
		if thinking["thinkingLevel"] != "minimal" || thinking["thinkingBudget"] != nil {
			t.Fatalf("thinkingConfig = %v, want the minimal level", thinking)
		}
	})

	t.Run("generation options are omitted when the request sets none", func(t *testing.T) {
		request := canonicalRequest(false)
		request.MaxTokens = 0
		if body := encodeBody(t, request, wire.CodecOpts{CredentialRef: "api-key-1"}); body["generationConfig"] != nil {
			t.Fatalf("generationConfig = %v, want none", body["generationConfig"])
		}
	})

	t.Run("temperature and the token limit travel", func(t *testing.T) {
		temperature := 0.25
		request := canonicalRequest(false)
		request.Temperature = &temperature
		config := encodeBody(t, request, wire.CodecOpts{CredentialRef: "api-key-1"})["generationConfig"].(map[string]any)
		if config["maxOutputTokens"] != float64(64) || config["temperature"] != 0.25 {
			t.Fatalf("generationConfig = %v, want the request options", config)
		}
	})

	t.Run("unparseable tool arguments become an empty object", func(t *testing.T) {
		request := canonicalRequest(false)
		request.Messages = append(request.Messages, inference.Message{
			Role:      inference.RoleAssistant,
			ToolCalls: []inference.ToolCall{{ID: "call_1", Name: "get_weather", Arguments: "{not json"}},
		})
		contents := encodeBody(t, request, wire.CodecOpts{CredentialRef: "api-key-1"})["contents"].([]any)
		call := contents[len(contents)-1].(map[string]any)["parts"].([]any)[0].(map[string]any)["functionCall"].(map[string]any)
		if len(call["args"].(map[string]any)) != 0 {
			t.Fatalf("args = %v, want an empty object", call["args"])
		}
	})

	t.Run("a call without arguments sends an empty object", func(t *testing.T) {
		request := canonicalRequest(false)
		request.Messages = append(request.Messages, inference.Message{
			Role:      inference.RoleAssistant,
			ToolCalls: []inference.ToolCall{{ID: "call_1", Name: "get_weather"}},
		})
		contents := encodeBody(t, request, wire.CodecOpts{CredentialRef: "api-key-1"})["contents"].([]any)
		call := contents[len(contents)-1].(map[string]any)["parts"].([]any)[0].(map[string]any)["functionCall"].(map[string]any)
		if len(call["args"].(map[string]any)) != 0 {
			t.Fatalf("args = %v, want an empty object", call["args"])
		}
	})

	t.Run("an empty text part is left out", func(t *testing.T) {
		request := canonicalRequest(false)
		request.Messages = append(request.Messages, inference.Message{Role: inference.RoleAssistant, Content: []inference.ContentPart{
			{Type: inference.ContentTypeText},
			{Type: inference.ContentTypeText, Text: "visible"},
		}})
		parts := lastParts(t, request)
		if len(parts) != 1 || parts[0].(map[string]any)["text"] != "visible" {
			t.Fatalf("parts = %v, want only the visible sentence", parts)
		}
	})

	t.Run("thinking parts are not sent as text", func(t *testing.T) {
		request := canonicalRequest(false)
		request.Messages = append(request.Messages, inference.Message{Role: inference.RoleAssistant, Content: []inference.ContentPart{
			{Type: inference.ContentTypeThinking, Text: "hidden", Signature: "sig"},
			{Type: inference.ContentTypeText, Text: "visible"},
		}})
		parts := lastParts(t, request)
		if len(parts) != 1 || parts[0].(map[string]any)["text"] != "visible" {
			t.Fatalf("parts = %v, want only the text part", parts)
		}
	})

	t.Run("a turn with nothing to send is dropped", func(t *testing.T) {
		request := canonicalRequest(false)
		request.Messages = append(request.Messages, inference.Message{Role: inference.RoleAssistant, Content: []inference.ContentPart{
			{Type: inference.ContentTypeThinking, Signature: "sig"},
		}})
		contents := encodeBody(t, request, wire.CodecOpts{CredentialRef: "api-key-1"})["contents"].([]any)
		if len(contents) != 1 {
			t.Fatalf("contents = %v, want only the user turn", contents)
		}
	})
}

func TestToolSchemas(t *testing.T) {
	t.Run("tool declarations carry sanitized schemas", func(t *testing.T) {
		request := canonicalRequest(false)
		request.Tools = []inference.Tool{{Name: "lookup", Description: "look it up", Parameters: json.RawMessage(schemaWithRejectedKeywords)}}
		body := encodeBody(t, request, wire.CodecOpts{CredentialRef: "api-key-1"})
		tools := body["tools"].([]any)
		declaration := tools[0].(map[string]any)["functionDeclarations"].([]any)[0].(map[string]any)
		if declaration["name"] != "lookup" {
			t.Fatalf("declaration = %v, want the function name", declaration)
		}
		parameters := declaration["parameters"].(map[string]any)
		if _, found := parameters["additionalProperties"]; found {
			t.Fatalf("parameters = %v, want the rejected keyword gone", parameters)
		}
	})

	t.Run("tool schema sanitization", func(t *testing.T) {
		cleaned := google.SanitizeSchema(json.RawMessage(schemaWithRejectedKeywords))
		var schema map[string]any
		if err := json.Unmarshal(cleaned, &schema); err != nil {
			t.Fatalf("decode sanitized schema: %v", err)
		}
		if schema["type"] != "object" || schema["required"].([]any)[0] != "city" {
			t.Fatalf("schema = %v, want the accepted keywords kept", schema)
		}
		properties := schema["properties"].(map[string]any)
		city := properties["city"].(map[string]any)
		if _, found := city["title"]; found {
			t.Fatalf("city = %v, want its title stripped", city)
		}
		if city["description"] != "the city" || city["type"] != "string" {
			t.Fatalf("city = %v, want the accepted keywords kept", city)
		}
		if _, found := properties["title"]; !found {
			t.Fatalf("properties = %v, want a property named title kept", properties)
		}
		nested := properties["nested"].(map[string]any)["items"].(map[string]any)
		if _, found := nested["additionalProperties"]; found {
			t.Fatalf("nested = %v, want the keyword stripped at every level", nested)
		}
		width := nested["properties"].(map[string]any)["width"].(map[string]any)
		if width["type"] != "integer" {
			t.Fatalf("width = %v, want the type kept", width)
		}
		for _, keyword := range []string{"exclusiveMinimum", "exclusiveMaximum", "minimum", "maximum"} {
			if _, found := width[keyword]; found {
				t.Fatalf("width = %v, want %s stripped", width, keyword)
			}
		}
	})

	t.Run("a schema that is not an object is returned unchanged", func(t *testing.T) {
		for _, schema := range []string{"", "{not json", "true"} {
			if got := string(google.SanitizeSchema(json.RawMessage(schema))); got != schema {
				t.Fatalf("SanitizeSchema(%q) = %q, want it unchanged", schema, got)
			}
		}
	})

	t.Run("a tool without parameters omits them", func(t *testing.T) {
		request := canonicalRequest(false)
		request.Tools = []inference.Tool{{Name: "ping"}}
		body := encodeBody(t, request, wire.CodecOpts{CredentialRef: "api-key-1"})
		tools := body["tools"].([]any)
		declaration := tools[0].(map[string]any)["functionDeclarations"].([]any)[0].(map[string]any)
		if declaration["parameters"] != nil {
			t.Fatalf("declaration = %v, want no parameters", declaration)
		}
	})
}

func TestStreamDecoding(t *testing.T) {
	t.Run("text stream per mode (AI Studio, Vertex, Cloud Code Assist)", func(t *testing.T) {
		modes := []struct {
			name string
			opts wire.CodecOpts
		}{
			{"ai studio", wire.CodecOpts{CredentialRef: "api-key-1"}},
			{"vertex", wire.CodecOpts{CredentialRef: "token", Mode: string(google.ModeVertex), Project: "p"}},
			{"cloud code assist", wire.CodecOpts{CredentialRef: "token", Mode: string(google.ModeCloudCodeAssist), Project: "p"}},
		}
		for _, mode := range modes {
			t.Run(mode.name, func(t *testing.T) {
				request := encode(t, google.Module(), canonicalRequest(true), mode.opts)
				if request.URL.Query().Get("alt") != "sse" {
					t.Fatalf("url = %s, want the streaming flag", request.URL)
				}
				events := pushStream(t, google.Module().NewStreamDecoder(), "google/gemini_streaming.txt")
				if got := textOf(events); got != "Hello world" {
					t.Fatalf("text = %q, want the concatenated parts", got)
				}
				if reason := terminalReason(events); reason != inference.EventReasonStop {
					t.Fatalf("terminal = %q, want stop", reason)
				}
			})
		}
	})

	t.Run("usage counts thought tokens as output", func(t *testing.T) {
		events := pushStream(t, google.Module().NewStreamDecoder(), "google/gemini_thinking.txt")
		usage := lastUsage(events)
		if usage == nil || usage.InputTokens != 8 || usage.OutputTokens != 6 {
			t.Fatalf("usage = %+v, want candidates and thoughts counted", usage)
		}
	})

	t.Run("thinking summary -> ReasoningDelta", func(t *testing.T) {
		events := pushStream(t, google.Module().NewStreamDecoder(), "google/gemini_thinking.txt")
		if got := reasoningTextOf(events); got != "Let me think" {
			t.Fatalf("reasoning = %q, want the thought summary", got)
		}
		if got := textOf(events); got != "Answer" {
			t.Fatalf("text = %q, want only the answer", got)
		}
	})

	t.Run("function call part -> ToolCallStart/End", func(t *testing.T) {
		events := pushStream(t, google.Module().NewStreamDecoder(), "google/gemini_tool_call.txt")
		if countKind(events, inference.EventToolCallStart) != 1 || countKind(events, inference.EventToolCallEnd) != 1 {
			t.Fatalf("events = %+v, want one whole call", events)
		}
		calls := assembledCalls(events)
		if calls[0].Name != "get_weather" || calls[0].ID != "call_0" || !sameJSON(calls[0].Arguments, `{"city":"Hanoi"}`) {
			t.Fatalf("call = %+v, want the function call with its arguments", calls[0])
		}
		if reason := terminalReason(events); reason != inference.EventReasonToolUse {
			t.Fatalf("terminal = %q, want tool_use", reason)
		}
	})

	t.Run("finishReason SAFETY -> error terminal", func(t *testing.T) {
		events := pushStream(t, google.Module().NewStreamDecoder(), "google/gemini_safety.txt")
		if countKind(events, inference.EventError) != 1 {
			t.Fatalf("events = %+v, want the blocked response reported", events)
		}
		failure := failureOf(events)
		if failure.Code != "safety" || !strings.Contains(failure.Message, "HARM_CATEGORY_DANGEROUS_CONTENT") {
			t.Fatalf("failure = %+v, want the safety rating detail", failure)
		}
		if reason := terminalReason(events); reason != inference.EventReasonError {
			t.Fatalf("terminal = %q, want error", reason)
		}
	})

	t.Run("a blocked finish without ratings still reports the reason", func(t *testing.T) {
		events := pushStreamData(t, google.Module().NewStreamDecoder(), `{"candidates":[{"finishReason":"RECITATION"}]}`)
		if countKind(events, inference.EventError) != 1 || !strings.Contains(failureOf(events).Message, "RECITATION") {
			t.Fatalf("events = %+v, want the finish reason reported", events)
		}
	})

	t.Run("a max token finish becomes a length terminal", func(t *testing.T) {
		decoder := google.Module().NewStreamDecoder()
		pushStreamData(t, decoder, `{"candidates":[{"finishReason":"MAX_TOKENS"}]}`)
		closing, err := decoder.Finish()
		if err != nil {
			t.Fatalf("Finish() error = %v", err)
		}
		if closing[0].Terminal.Reason != inference.EventReasonLength {
			t.Fatalf("terminal = %+v, want length", closing[0])
		}
	})

	t.Run("an interrupted stream is closed with a terminal", func(t *testing.T) {
		decoder := google.Module().NewStreamDecoder()
		pushStreamData(t, decoder, `{"candidates":[{"content":{"parts":[{"text":"partial"}]}}]}`)
		closing, err := decoder.Finish()
		if err != nil {
			t.Fatalf("Finish() error = %v", err)
		}
		if len(closing) != 1 || closing[0].Terminal.Reason != inference.EventReasonStop {
			t.Fatalf("Finish() = %+v, want a synthesized terminal", closing)
		}
		if again, err := decoder.Finish(); err != nil || len(again) != 0 {
			t.Fatalf("second Finish() = %+v, %v, want nothing", again, err)
		}
	})

	t.Run("an error that arrives after content is reported", func(t *testing.T) {
		decoder := google.Module().NewStreamDecoder()
		pushStreamData(t, decoder, `{"candidates":[{"content":{"parts":[{"text":"before"}]}}]}`)
		events := pushStreamData(t, decoder, `{"error":{"code":429,"message":"quota exceeded","status":"RESOURCE_EXHAUSTED"}}`)
		if countKind(events, inference.EventError) != 1 || failureOf(events).Code != "RESOURCE_EXHAUSTED" {
			t.Fatalf("events = %+v, want the upstream failure", events)
		}
		closing, err := decoder.Finish()
		if err != nil {
			t.Fatalf("Finish() error = %v", err)
		}
		if closing[0].Terminal.Reason != inference.EventReasonError {
			t.Fatalf("terminal = %+v, want error", closing[0])
		}
	})

	t.Run("empty and unknown chunks are skipped", func(t *testing.T) {
		decoder := google.Module().NewStreamDecoder()
		frames := []string{"", "   ", `{}`, `{"candidates":[]}`, `{"candidates":[{"content":{"parts":[]}}]}`}
		for _, data := range frames {
			events, err := decoder.Push(wire.SSEEvent{Data: data})
			if err != nil {
				t.Fatalf("Push(%q) error = %v", data, err)
			}
			if len(events) != 0 {
				t.Fatalf("Push(%q) = %+v, want nothing", data, events)
			}
		}
	})

	t.Run("parts the canonical format does not carry are ignored", func(t *testing.T) {
		data := `{"candidates":[{"content":{"parts":[{"inlineData":{"mimeType":"image/png","data":"aGk="}},{"text":""}]}}]}`
		if events := pushStreamData(t, google.Module().NewStreamDecoder(), data); len(events) != 0 {
			t.Fatalf("events = %+v, want nothing", events)
		}
	})

	t.Run("a malformed chunk is rejected", func(t *testing.T) {
		_, err := google.Module().NewStreamDecoder().Push(wire.SSEEvent{Data: "{not json"})
		if !errors.Is(err, wire.ErrMalformedEvent) {
			t.Fatalf("Push() error = %v, want the malformed event sentinel", err)
		}
	})
}

func TestDecodeResponse(t *testing.T) {
	t.Run("non-streaming response", func(t *testing.T) {
		events, err := google.Module().DecodeResponse(readFixture(t, "google/gemini_nonstreaming.json"))
		if err != nil {
			t.Fatalf("DecodeResponse() error = %v", err)
		}
		if got := textOf(events); got != "Hi there" {
			t.Fatalf("text = %q, want the answer", got)
		}
		calls := assembledCalls(events)
		if len(calls) != 1 || calls[0].Name != "get_weather" {
			t.Fatalf("calls = %+v, want the function call", calls)
		}
		usage := lastUsage(events)
		if usage == nil || usage.InputTokens != 9 || usage.OutputTokens != 6 || usage.CacheReadTokens != 3 {
			t.Fatalf("usage = %+v, want the reported counts", usage)
		}
		if reason := terminalReason(events); reason != inference.EventReasonToolUse {
			t.Fatalf("terminal = %q, want tool_use", reason)
		}
	})

	t.Run("a response carrying an error becomes an error event", func(t *testing.T) {
		body := []byte(`{"error":{"code":400,"message":"API key not valid","status":"INVALID_ARGUMENT"}}`)
		events, err := google.Module().DecodeResponse(body)
		if err != nil {
			t.Fatalf("DecodeResponse() error = %v", err)
		}
		if countKind(events, inference.EventError) != 1 || terminalReason(events) != inference.EventReasonError {
			t.Fatalf("events = %+v, want the failure and an error terminal", events)
		}
	})

	t.Run("a malformed response is rejected", func(t *testing.T) {
		_, err := google.Module().DecodeResponse([]byte("{not json"))
		if !errors.Is(err, wire.ErrInvalidResponse) {
			t.Fatalf("DecodeResponse() error = %v, want the invalid response sentinel", err)
		}
	})
}

func TestDecodeError(t *testing.T) {
	t.Run("an upstream error body is mapped", func(t *testing.T) {
		body := []byte(`{"error":{"code":403,"message":"caller does not have permission","status":"PERMISSION_DENIED"}}`)
		info := google.Module().DecodeError(403, body)
		if info.Status != 403 || info.Code != "PERMISSION_DENIED" || info.Message == "" {
			t.Fatalf("info = %+v, want the upstream failure", info)
		}
	})

	t.Run("an error without a status uses the HTTP status text", func(t *testing.T) {
		info := google.Module().DecodeError(500, []byte(`{"error":{"code":500,"message":"boom"}}`))
		if info.Code != "Internal Server Error" {
			t.Fatalf("info = %+v, want the status text", info)
		}
	})

	t.Run("a non-json error body falls back to the status text", func(t *testing.T) {
		info := google.Module().DecodeError(503, []byte("service unavailable"))
		if info.Code != "Service Unavailable" || info.Status != 503 {
			t.Fatalf("info = %+v, want the status text", info)
		}
	})
}

// cloudCodeAssistCodec is the codec the relay resolves for the Cloud Code
// Assist format, which is the one that has to read the wrapped answer.
func cloudCodeAssistCodec() *google.Codec {
	return google.NewCodec(google.Config{Mode: google.ModeCloudCodeAssist})
}

// TestCloudCodeAssistEnvelope is the defect the Antigravity endpoint exposed:
// every payload it answers with nests the GenerateContent response under
// `response`, so a decoder that reads the frame at the top level sees no
// candidates at all and a chat returns no text.
func TestCloudCodeAssistEnvelope(t *testing.T) {
	t.Run("a streamed frame is read through the wrapper", func(t *testing.T) {
		decoder := cloudCodeAssistCodec().NewStreamDecoder()
		first := pushStreamData(t, decoder,
			`{"response":{"candidates":[{"content":{"role":"model","parts":[{"text":"Hello"}]}}],"usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":2}}}`)
		second := pushStreamData(t, decoder,
			`{"response":{"candidates":[{"content":{"parts":[{"text":" world"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":3,"cachedContentTokenCount":1}}}`)
		closing, err := decoder.Finish()
		if err != nil {
			t.Fatalf("Finish() error = %v", err)
		}
		events := append(append(first, second...), closing...)
		if got := textOf(events); got != "Hello world" {
			t.Fatalf("text = %q, want the wrapped parts", got)
		}
		usage := lastUsage(events)
		if usage == nil || usage.InputTokens != 5 || usage.OutputTokens != 3 || usage.CacheReadTokens != 1 {
			t.Fatalf("usage = %+v, want the counts the wrapper carried", usage)
		}
		if reason := terminalReason(events); reason != inference.EventReasonStop {
			t.Fatalf("terminal = %q, want stop", reason)
		}
	})

	t.Run("one complete body is read through the wrapper", func(t *testing.T) {
		body := []byte(`{"response":{"candidates":[{"content":{"role":"model","parts":[{"text":"hello"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":9,"candidatesTokenCount":2,"cachedContentTokenCount":7}}}`)
		events, err := cloudCodeAssistCodec().DecodeResponse(body)
		if err != nil {
			t.Fatalf("DecodeResponse() error = %v", err)
		}
		if got := textOf(events); got != "hello" {
			t.Fatalf("text = %q, want the wrapped answer", got)
		}
		usage := lastUsage(events)
		if usage == nil || usage.InputTokens != 9 || usage.OutputTokens != 2 || usage.CacheReadTokens != 7 {
			t.Fatalf("usage = %+v, want the counts the wrapper carried", usage)
		}
		if reason := terminalReason(events); reason != inference.EventReasonStop {
			t.Fatalf("terminal = %q, want stop", reason)
		}
	})

	t.Run("an unwrapped frame still decodes", func(t *testing.T) {
		decoder := cloudCodeAssistCodec().NewStreamDecoder()
		events := pushStreamData(t, decoder,
			`{"candidates":[{"content":{"parts":[{"text":"bare"}]},"finishReason":"STOP"}]}`)
		if got := textOf(events); got != "bare" {
			t.Fatalf("text = %q, want the frame read as it arrived", got)
		}
	})

	t.Run("an error inside the wrapper fails the answer", func(t *testing.T) {
		body := []byte(`{"response":{"error":{"code":429,"message":"Individual quota reached","status":"RESOURCE_EXHAUSTED"}}}`)
		events, err := cloudCodeAssistCodec().DecodeResponse(body)
		if err != nil {
			t.Fatalf("DecodeResponse() error = %v", err)
		}
		if countKind(events, inference.EventError) != 1 || terminalReason(events) != inference.EventReasonError {
			t.Fatalf("events = %+v, want the wrapped failure", events)
		}
	})

	t.Run("a refusal arrives inside the same wrapper", func(t *testing.T) {
		body := []byte(`{"response":{"error":{"code":403,"message":"caller does not have permission","status":"PERMISSION_DENIED"}}}`)
		info := cloudCodeAssistCodec().DecodeError(403, body)
		if info.Code != "PERMISSION_DENIED" || info.Message == "" {
			t.Fatalf("info = %+v, want the wrapped refusal", info)
		}
	})
}

// TestCloudCodeAssistEnvelopeCarriesTheClientIdentity asserts the envelope the
// first-party Antigravity client sends, so a Cloud Code Assist request names
// the same protocol constants the endpoint gates its models by.
func TestCloudCodeAssistEnvelopeCarriesTheClientIdentity(t *testing.T) {
	request := encode(t, cloudCodeAssistCodec(), canonicalRequest(false), wire.CodecOpts{
		CredentialRef: "token", Project: "project-1", RequestID: "req-1",
	})
	body, err := io.ReadAll(request.Body)
	if err != nil {
		t.Fatalf("read request body: %v", err)
	}
	var envelope map[string]any
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("decode request body: %v", err)
	}
	for field, want := range map[string]string{
		"project":     "project-1",
		"model":       "gemini-2.0-flash",
		"userAgent":   "antigravity",
		"requestType": "agent",
	} {
		if got, _ := envelope[field].(string); got != want {
			t.Fatalf("envelope[%q] = %q, want %q", field, got, want)
		}
	}
	requestID, _ := envelope["requestId"].(string)
	parts := strings.Split(requestID, "/")
	if len(parts) != 5 || parts[0] != "agent" {
		t.Fatalf("requestId = %q, want agent/<agent>/<ms>/<trajectory>/<step>", requestID)
	}
	if parts[1] == parts[3] {
		t.Fatalf("requestId = %q, want the agent and the trajectory to be separate identities", requestID)
	}
	if parts[4] != "2" {
		t.Fatalf("requestId = %q, want the first turn to sit at step 2", requestID)
	}
	inner, found := envelope["request"].(map[string]any)
	if !found {
		t.Fatalf("envelope = %v, want the GenerateContent request inside", envelope)
	}
	instruction := inner["systemInstruction"].(map[string]any)
	if instruction["role"] != "user" {
		t.Fatalf("systemInstruction = %v, want role user", instruction)
	}
	if inner["sessionId"] == "" {
		t.Fatalf("request = %v, want a session id", inner)
	}
	labels := inner["labels"].(map[string]any)
	if labels["trajectory_id"] != parts[3] || labels["last_step_index"] != "1" {
		t.Fatalf("labels = %v, want the trajectory from the envelope at step 1", labels)
	}
	if labels["used_claude"] != "false" {
		t.Fatalf("labels = %v, want a Gemini turn", labels)
	}
	// The vendor reads exactly these labels; one it never sends is a guess
	// about what the endpoint watches, and a label it does read and Relo drops
	// is telemetry that goes missing. A first turn has no execution to name
	// back, so that one label joins the set rather than being filled in.
	want := []string{"last_step_index", "trajectory_id", "used_claude", "used_claude_conservative"}
	for _, name := range want {
		if _, found := labels[name]; !found {
			t.Fatalf("labels = %v, want it to carry %q", labels, name)
		}
	}
	if len(labels) != len(want) {
		t.Fatalf("labels = %v, want exactly %v", labels, want)
	}
	config := inner["generationConfig"].(map[string]any)
	thinking := config["thinkingConfig"].(map[string]any)
	if thinking["thinkingBudget"] != float64(-1) || thinking["includeThoughts"] != true {
		t.Fatalf("thinkingConfig = %v, want an open budget and thought summaries", thinking)
	}
	requested := canonicalRequest(false)
	requested.Reasoning = &inference.ReasoningConfig{Effort: "low"}
	kept := encode(t, cloudCodeAssistCodec(), requested, wire.CodecOpts{
		CredentialRef: "token", Project: "project-1", RequestID: "req-1",
		ReasoningEfforts: []string{"minimal", "low", "medium", "high"},
	})
	keptBody, err := io.ReadAll(kept.Body)
	if err != nil {
		t.Fatalf("read request body: %v", err)
	}
	if err := json.Unmarshal(keptBody, &envelope); err != nil {
		t.Fatalf("decode request body: %v", err)
	}
	keptInner := envelope["request"].(map[string]any)
	keptThinking := keptInner["generationConfig"].(map[string]any)["thinkingConfig"].(map[string]any)
	if keptThinking["thinkingLevel"] != "low" || keptThinking["thinkingBudget"] != nil {
		t.Fatalf("thinkingConfig = %v, want the requested level kept", keptThinking)
	}
	if config["maxOutputTokens"] != float64(64) {
		t.Fatalf("maxOutputTokens = %v, want the caller's limit", config["maxOutputTokens"])
	}
}

// TestCloudCodeAssistVariantRouting asserts the envelope names the SKU the
// vendor serves the requested effort on, under the label it reads the SKU's
// enum from, within the ceiling that SKU accepts. The request itself keeps the
// logical model, so usage and the replay cache stay filed under it.
// TestCloudCodeAssistSessionIdentity asserts the two conversation ids stay
// put across the turns of one conversation while the step advances with it:
// the endpoint gates its models on that continuity, so a turn that re-derives
// its trajectory or repeats a step reads as a different session.
// TestCloudCodeAssistExecutionID covers the answer the next turn names back:
// the endpoint issues an id with every response and expects the following turn
// to carry it, so a fabricated one is worse than none.
// TestCloudCodeAssistClaudeBeta covers the beta a reasoning Claude turn claims
// by name: the vendor gates interleaved thinking behind it and serves the turn
// without the reasoning the client asked for when it is missing.
func TestCloudCodeAssistClaudeBeta(t *testing.T) {
	beta := func(model, effort string) string {
		t.Helper()
		request := canonicalRequest(false)
		request.Model = model
		if effort != "" {
			request.Reasoning = &inference.ReasoningConfig{Effort: effort}
		}
		encoded := encode(t, cloudCodeAssistCodec(), request, wire.CodecOpts{
			CredentialRef: "token", AuthMethod: wire.AuthOAuth, Project: "project-1",
		})
		return encoded.Header.Get("anthropic-beta")
	}

	t.Run("a reasoning claude turn claims it", func(t *testing.T) {
		for _, model := range []string{"claude-sonnet-4-6", "claude-opus-4-6"} {
			for _, effort := range []string{"low", "medium", "high"} {
				if got := beta(model, effort); got != "interleaved-thinking-2025-05-14" {
					t.Fatalf("anthropic-beta = %q for %s at %s, want the interleaved-thinking beta", got, model, effort)
				}
			}
		}
	})

	t.Run("a turn with thinking off does not claim it", func(t *testing.T) {
		if got := beta("claude-sonnet-4-6", "none"); got != "" {
			t.Fatalf("anthropic-beta = %q, want none for a turn with thinking off", got)
		}
	})

	t.Run("a gemini turn does not claim it", func(t *testing.T) {
		if got := beta("gemini-3.5-flash", "high"); got != "" {
			t.Fatalf("anthropic-beta = %q, want none for a model that does not reason on this beta", got)
		}
	})
}

func TestCloudCodeAssistExecutionID(t *testing.T) {
	session := "codex-thread:execution"
	decode := func(body string) {
		t.Helper()
		request := canonicalRequest(false)
		bound := cloudCodeAssistCodec().Bind(request, wire.CodecOpts{SessionAnchor: session})
		decoder := bound.NewStreamDecoder()
		pushStreamData(t, decoder, body)
		if _, err := decoder.Finish(); err != nil {
			t.Fatalf("Finish() error = %v", err)
		}
	}
	execution := func() (string, bool) {
		t.Helper()
		body := assistEnvelope(t, canonicalRequest(false), wire.CodecOpts{
			CredentialRef: "token", Project: "project-1", SessionAnchor: session,
		})
		labels := body["request"].(map[string]any)["labels"].(map[string]any)
		value, found := labels["last_execution_id"]
		text, _ := value.(string)
		return text, found
	}

	if _, found := execution(); found {
		t.Fatal("the first turn names an execution the endpoint has not issued")
	}

	decode(`{"response":{"responseId":"exec-1","candidates":[{"content":{"parts":[{"text":"hi"}]},"finishReason":"STOP"}]}}`)
	id, found := execution()
	if !found || id != "exec-1" {
		t.Fatalf("last_execution_id = %q, want the id the endpoint issued", id)
	}

	t.Run("a later execution replaces it", func(t *testing.T) {
		decode(`{"response":{"responseId":"exec-2","candidates":[{"content":{"parts":[{"text":"again"}]},"finishReason":"STOP"}]}}`)
		if id, _ := execution(); id != "exec-2" {
			t.Fatalf("last_execution_id = %q, want the newest execution", id)
		}
	})

	t.Run("a refused answer leaves the execution standing", func(t *testing.T) {
		decode(`{"response":{"responseId":"exec-3","error":{"code":429,"message":"Individual quota reached","status":"RESOURCE_EXHAUSTED"}}}`)
		if id, _ := execution(); id != "exec-2" {
			t.Fatalf("last_execution_id = %q, want the last execution that answered", id)
		}
	})

	t.Run("another conversation names its own", func(t *testing.T) {
		body := assistEnvelope(t, canonicalRequest(false), wire.CodecOpts{
			CredentialRef: "token", Project: "project-1", SessionAnchor: "codex-thread:other",
		})
		labels := body["request"].(map[string]any)["labels"].(map[string]any)
		if _, found := labels["last_execution_id"]; found {
			t.Fatalf("labels = %v, want no execution from another conversation", labels)
		}
	})
}

func TestCloudCodeAssistSessionIdentity(t *testing.T) {
	turn := func(text string, extra ...inference.Message) *inference.Request {
		request := canonicalRequest(false)
		request.Messages[len(request.Messages)-1].Content = []inference.ContentPart{{Type: inference.ContentTypeText, Text: text}}
		return &inference.Request{Model: request.Model, Stream: false, Messages: append(request.Messages, extra...)}
	}
	read := func(request *inference.Request) (requestID, trajectory string, step string) {
		body := assistEnvelope(t, request, wire.CodecOpts{
			CredentialRef: "token", Project: "project-1", SessionAnchor: "codex-thread:one",
		})
		requestID, _ = body["requestId"].(string)
		labels := body["request"].(map[string]any)["labels"].(map[string]any)
		trajectory, _ = labels["trajectory_id"].(string)
		step, _ = labels["last_step_index"].(string)
		return requestID, trajectory, step
	}

	_, firstTrajectory, firstStep := read(turn("hello"))
	secondID, secondTrajectory, secondStep := read(turn("hello",
		inference.Message{Role: inference.RoleAssistant, Content: []inference.ContentPart{{Type: inference.ContentTypeText, Text: "hi"}}},
		inference.Message{Role: inference.RoleUser, Content: []inference.ContentPart{{Type: inference.ContentTypeText, Text: "again"}}},
	))

	if firstTrajectory != secondTrajectory {
		t.Fatalf("trajectory = %q then %q, want one identity for the conversation", firstTrajectory, secondTrajectory)
	}
	if firstStep != "1" || secondStep != "2" {
		t.Fatalf("last_step_index = %q then %q, want the turn count to advance", firstStep, secondStep)
	}
	if step := strings.Split(secondID, "/")[4]; step != "3" {
		t.Fatalf("requestId = %q, want the second turn at step 3", secondID)
	}

	t.Run("another conversation gets its own identities", func(t *testing.T) {
		body := assistEnvelope(t, turn("hello"), wire.CodecOpts{
			CredentialRef: "token", Project: "project-1", SessionAnchor: "codex-thread:two",
		})
		labels := body["request"].(map[string]any)["labels"].(map[string]any)
		if labels["trajectory_id"] == firstTrajectory {
			t.Fatalf("trajectory = %v, want a separate conversation its own identity", labels["trajectory_id"])
		}
	})

	t.Run("a conversation ending on a model turn does not skip a step", func(t *testing.T) {
		_, _, before := read(turn("hello"))
		trailing := turn("hello",
			inference.Message{Role: inference.RoleAssistant, Content: []inference.ContentPart{{Type: inference.ContentTypeText, Text: "hi"}}},
		)
		_, _, after := read(trailing)
		if before != after {
			t.Fatalf("last_step_index = %q then %q, want the continue turn left out of the count", before, after)
		}
	})
}

func TestCloudCodeAssistVariantRouting(t *testing.T) {
	cases := []struct {
		logical string
		effort  string
		wire    string
		enum    string
		ceiling float64
	}{
		{"gemini-3.5-flash", "minimal", "gemini-3.5-flash-extra-low", "MODEL_PLACEHOLDER_M187", 65536},
		{"gemini-3.5-flash", "medium", "gemini-3.5-flash-low", "MODEL_PLACEHOLDER_M20", 65536},
		{"gemini-3.5-flash", "high", "gemini-3-flash-agent", "MODEL_PLACEHOLDER_M132", 65536},
		{"gemini-3.7-flash", "high", "gemini-3.7-flash-high", "", 65536},
		{"gemini-3.1-pro", "low", "gemini-3.1-pro-low", "MODEL_PLACEHOLDER_M36", 65535},
		{"gemini-3.1-pro", "high", "gemini-pro-agent", "MODEL_PLACEHOLDER_M16", 65535},
		{"claude-sonnet-4-6", "high", "claude-sonnet-4-6", "", 64000},
		{"claude-opus-4-6", "high", "claude-opus-4-6-thinking", "", 64000},
		{"gemini-2.0-flash", "high", "gemini-2.0-flash", "", 65536},
	}
	for _, test := range cases {
		t.Run(test.logical+" at "+test.effort, func(t *testing.T) {
			request := canonicalRequest(false)
			request.Model = test.logical
			request.MaxTokens = 0
			if test.effort != "" {
				request.Reasoning = &inference.ReasoningConfig{Effort: test.effort}
			}
			body := assistEnvelope(t, request, wire.CodecOpts{
				CredentialRef: "token", AuthMethod: wire.AuthOAuth, Project: "project-1",
			})
			if got, _ := body["model"].(string); got != test.wire {
				t.Fatalf("model = %q, want the SKU %q", got, test.wire)
			}
			labels := body["request"].(map[string]any)["labels"].(map[string]any)
			enum, present := labels["model_enum"]
			if test.enum == "" && present {
				t.Fatalf("model_enum = %v, want none for a SKU the vendor gives no enum", enum)
			}
			if test.enum != "" && enum != test.enum {
				t.Fatalf("model_enum = %v, want %q", enum, test.enum)
			}
			config := body["request"].(map[string]any)["generationConfig"].(map[string]any)
			if got, _ := config["maxOutputTokens"].(float64); got != test.ceiling {
				t.Fatalf("maxOutputTokens = %v, want %v", got, test.ceiling)
			}
		})
	}

	t.Run("a ceiling the endpoint refuses is narrowed to what it serves", func(t *testing.T) {
		request := canonicalRequest(false)
		request.Model = "claude-sonnet-4-6"
		request.MaxTokens = 100000
		body := assistEnvelope(t, request, wire.CodecOpts{
			CredentialRef: "token", AuthMethod: wire.AuthOAuth, Project: "project-1",
		})
		config := body["request"].(map[string]any)["generationConfig"].(map[string]any)
		if got, _ := config["maxOutputTokens"].(float64); got != 64000 {
			t.Fatalf("maxOutputTokens = %v, want the Claude ceiling", got)
		}
	})

	t.Run("a caller below the ceiling keeps its own limit", func(t *testing.T) {
		request := canonicalRequest(false)
		request.Model = "gemini-3.5-flash"
		body := assistEnvelope(t, request, wire.CodecOpts{
			CredentialRef: "token", AuthMethod: wire.AuthOAuth, Project: "project-1",
		})
		config := body["request"].(map[string]any)["generationConfig"].(map[string]any)
		if got, _ := config["maxOutputTokens"].(float64); got != 64 {
			t.Fatalf("maxOutputTokens = %v, want the caller's limit", got)
		}
	})
}

// assistBody encodes one Cloud Code Assist request and reads the whole
// envelope, since the model SKU and the labels both sit outside the inner
// request.
func assistEnvelope(t *testing.T, request *inference.Request, opts wire.CodecOpts) map[string]any {
	t.Helper()
	opts.Mode = string(google.ModeCloudCodeAssist)
	encoded := encode(t, cloudCodeAssistCodec(), request, opts)
	body, err := io.ReadAll(encoded.Body)
	if err != nil {
		t.Fatalf("read request body: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode request body: %v", err)
	}
	return payload
}

func TestCloudCodeAssistSendMatchesTheClient(t *testing.T) {
	const realSignature = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcd"

	t.Run("the cli user agent replaces the ide fingerprint", func(t *testing.T) {
		request := encode(t, cloudCodeAssistCodec(), canonicalRequest(false), wire.CodecOpts{
			CredentialRef: "token", AuthMethod: wire.AuthOAuth, Project: "project-1",
			ExtraHeaders: map[string]string{"User-Agent": antigravity.UserAgent()},
		})
		if got := request.Header.Get("User-Agent"); got != antigravity.CLIUserAgent() {
			t.Fatalf("User-Agent = %q, want the CLI fingerprint", got)
		}
		if request.Header.Get("Accept-Encoding") != "gzip" || request.ContentLength != -1 {
			t.Fatalf("header length = %d, accept-encoding = %q, want a chunked gzip request", request.ContentLength, request.Header.Get("Accept-Encoding"))
		}
		if request.Header.Get("Authorization") != "Bearer token" || request.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("headers = %v, want the bearer token and json", request.Header)
		}
	})

	t.Run("an unset token limit defaults to the client budget", func(t *testing.T) {
		request := canonicalRequest(false)
		request.MaxTokens = 0
		body := assistBody(t, request, wire.CodecOpts{CredentialRef: "token", Project: "project-1"})
		config := body["generationConfig"].(map[string]any)
		if config["maxOutputTokens"] != float64(65536) {
			t.Fatalf("maxOutputTokens = %v, want the default budget", config["maxOutputTokens"])
		}
	})

	t.Run("the same client identity keeps the same session", func(t *testing.T) {
		codex := "codex-thread:parent\x00child"
		first := assistBody(t, canonicalRequest(false), wire.CodecOpts{CredentialRef: "token", Project: "p", SessionAnchor: codex})
		second := assistBody(t, canonicalRequest(false), wire.CodecOpts{CredentialRef: "token", Project: "p", SessionAnchor: codex})
		if first["sessionId"] == "" || first["sessionId"] != second["sessionId"] {
			t.Fatalf("session = %v and %v, want one stable id", first["sessionId"], second["sessionId"])
		}
		openCode := assistBody(t, canonicalRequest(false), wire.CodecOpts{CredentialRef: "token", Project: "p", SessionAnchor: "oc-session-1"})
		again := assistBody(t, canonicalRequest(false), wire.CodecOpts{CredentialRef: "token", Project: "p", SessionAnchor: "oc-session-1"})
		if openCode["sessionId"] != again["sessionId"] || openCode["sessionId"] == first["sessionId"] {
			t.Fatalf("opencode session = %v, codex session = %v, want each client stable and distinct", openCode["sessionId"], first["sessionId"])
		}
		fromText := assistBody(t, canonicalRequest(false), wire.CodecOpts{CredentialRef: "token", Project: "p"})
		fromTextAgain := assistBody(t, canonicalRequest(false), wire.CodecOpts{CredentialRef: "token", Project: "p"})
		if fromText["sessionId"] != fromTextAgain["sessionId"] {
			t.Fatalf("session = %v and %v, want the first user text to keep the session", fromText["sessionId"], fromTextAgain["sessionId"])
		}
		other := canonicalRequest(false)
		other.Messages[1].Content[0].Text = "something else"
		moved := assistBody(t, other, wire.CodecOpts{CredentialRef: "token", Project: "p"})
		if moved["sessionId"] == fromText["sessionId"] {
			t.Fatalf("session = %v, want a different first message to move it", moved["sessionId"])
		}
	})

	t.Run("a real signature on model text is forwarded", func(t *testing.T) {
		request := canonicalRequest(false)
		request.Messages = append(request.Messages, inference.Message{
			Role: inference.RoleAssistant,
			Content: []inference.ContentPart{{
				Type: inference.ContentTypeText, Text: "hello back", Signature: realSignature,
			}},
		})
		contents := assistBody(t, request, wire.CodecOpts{CredentialRef: "token", Project: "p"})["contents"].([]any)
		var signed string
		for _, raw := range contents {
			turn := raw.(map[string]any)
			if turn["role"] != "model" {
				continue
			}
			signed, _ = turn["parts"].([]any)[0].(map[string]any)["thoughtSignature"].(string)
		}
		if signed != realSignature {
			t.Fatalf("thoughtSignature = %q, want the model text signature", signed)
		}
	})

	t.Run("a foreign call id is not sent as a thought signature", func(t *testing.T) {
		request := canonicalRequest(false)
		request.Messages = append(request.Messages, inference.Message{
			Role: inference.RoleAssistant,
			Content: []inference.ContentPart{
				{Type: inference.ContentTypeThinking, Signature: "call_abcdefghijklmnop"},
			},
			ToolCalls: []inference.ToolCall{{ID: "1", Name: "lookup_foreign", Arguments: `{"q":"a"}`}},
		})
		call := assistCall(t, request, "lookup_foreign")
		if call["thoughtSignature"] == "call_abcdefghijklmnop" {
			t.Fatalf("part = %v, want the foreign id dropped", call)
		}
		if call["thoughtSignature"] != "skip_thought_signature_validator" {
			t.Fatalf("part = %v, want the validator bypass on the unsigned call", call)
		}
	})

	t.Run("a recorded signature is replayed and a claude model is not", func(t *testing.T) {
		anchor := "codex-thread:parent\x00replay"
		opts := wire.CodecOpts{CredentialRef: "token", Project: "p", SessionAnchor: anchor}
		bound := cloudCodeAssistCodec().Bind(canonicalRequest(false), opts)
		if _, err := bound.DecodeResponse([]byte(`{"candidates":[{"content":{"parts":[{"thoughtSignature":"` + realSignature + `","functionCall":{"name":"get_weather","args":{"city":"Hanoi"}}}]}}]}`)); err != nil {
			t.Fatalf("DecodeResponse() error = %v", err)
		}
		follow := canonicalRequest(false)
		follow.Messages = append(follow.Messages,
			inference.Message{Role: inference.RoleAssistant, ToolCalls: []inference.ToolCall{{ID: "1", Name: "get_weather", Arguments: `{"city":"Hanoi"}`}}},
			inference.Message{Role: inference.RoleTool, ToolCallID: "1", Content: []inference.ContentPart{{Type: inference.ContentTypeText, Text: `{"temp":31}`}}},
		)
		call := assistCall(t, follow, "get_weather", opts)
		if call["thoughtSignature"] != realSignature {
			t.Fatalf("part = %v, want the recorded signature", call)
		}

		claude := canonicalRequest(false)
		claude.Model = "claude-sonnet-4-6"
		claude.MaxTokens = 0
		claude.Tools = []inference.Tool{{Name: "get_weather", Parameters: json.RawMessage(`{"type":"object"}`)}}
		claude.Messages = append(claude.Messages, inference.Message{
			Role: inference.RoleAssistant,
			Content: []inference.ContentPart{
				{Type: inference.ContentTypeThinking, Text: "hmm", Signature: realSignature},
				{Type: inference.ContentTypeThinking, Signature: "short"},
			},
			ToolCalls: []inference.ToolCall{{ID: "call/1", Name: "get_weather", Arguments: `{"city":"Hanoi"}`}},
		})
		body := assistBody(t, claude, wire.CodecOpts{CredentialRef: "token", Project: "p", SessionAnchor: anchor})
		config := body["generationConfig"].(map[string]any)
		if _, found := config["thinkingConfig"]; found {
			t.Fatalf("generationConfig = %v, want no Gemini thinking config", config)
		}
		mode := body["toolConfig"].(map[string]any)["functionCallingConfig"].(map[string]any)["mode"]
		if mode != "VALIDATED" {
			t.Fatalf("toolConfig = %v, want validated calling", body["toolConfig"])
		}
		part := assistCall(t, claude, "get_weather", wire.CodecOpts{CredentialRef: "token", Project: "p", SessionAnchor: anchor})
		if part["thoughtSignature"] != nil {
			t.Fatalf("part = %v, want no sentinel on a claude call", part)
		}
		thought := assistBody(t, claude, wire.CodecOpts{CredentialRef: "token", Project: "p", SessionAnchor: anchor})
		var kept bool
		for _, raw := range thought["contents"].([]any) {
			turn := raw.(map[string]any)
			if turn["role"] != "model" {
				continue
			}
			for _, rawPart := range turn["parts"].([]any) {
				item := rawPart.(map[string]any)
				if item["thought"] == true && item["thoughtSignature"] == realSignature {
					kept = true
				}
				if item["thought"] == true && item["text"] == nil && item["thoughtSignature"] == nil {
					t.Fatalf("parts = %v, want an unsigned thinking part dropped", turn["parts"])
				}
			}
		}
		if !kept {
			t.Fatalf("contents = %v, want the signed thinking part kept", thought["contents"])
		}
		function := part["functionCall"].(map[string]any)
		if function["id"] == "" || function["id"] == "call/1" {
			t.Fatalf("functionCall = %v, want a rewritten id", function)
		}
	})

	t.Run("a model tail is continued and a call stays next to its response", func(t *testing.T) {
		tail := canonicalRequest(false)
		tail.Messages = append(tail.Messages, inference.Message{
			Role: inference.RoleAssistant, Content: []inference.ContentPart{{Type: inference.ContentTypeText, Text: "done"}},
		})
		contents := assistBody(t, tail, wire.CodecOpts{CredentialRef: "token", Project: "p"})["contents"].([]any)
		last := contents[len(contents)-1].(map[string]any)
		if last["role"] != "user" || last["parts"].([]any)[0].(map[string]any)["text"] != "(continue)" {
			t.Fatalf("contents = %v, want a continue turn", contents)
		}

		missing := canonicalRequest(false)
		missing.Messages = append(missing.Messages, inference.Message{
			Role: inference.RoleAssistant, ToolCalls: []inference.ToolCall{{ID: "1", Name: "lookup_missing", Arguments: `{}`}},
		})
		turns := assistBody(t, missing, wire.CodecOpts{CredentialRef: "token", Project: "p"})["contents"].([]any)
		responseTurn := turns[len(turns)-1].(map[string]any)
		if responseTurn["role"] != "user" {
			t.Fatalf("contents = %v, want the response beside the call", turns)
		}
		response := responseTurn["parts"].([]any)[0].(map[string]any)["functionResponse"].(map[string]any)
		if response["name"] != "lookup_missing" {
			t.Fatalf("functionResponse = %v, want the missing call answered", response)
		}
	})

	t.Run("a thought signature on a text part is returned to the client", func(t *testing.T) {
		events, err := cloudCodeAssistCodec().DecodeResponse([]byte(`{"candidates":[{"content":{"parts":[{"text":"Hello","thoughtSignature":"` + realSignature + `"}]}}]}`))
		if err != nil {
			t.Fatalf("DecodeResponse() error = %v", err)
		}
		if textOf(events) != "Hello" {
			t.Fatalf("text = %q, want the answer", textOf(events))
		}
		var signature string
		for _, event := range events {
			if event.Reasoning != nil {
				signature = event.Reasoning.Signature
			}
		}
		if signature != realSignature {
			t.Fatalf("signature = %q, want the part signature", signature)
		}
	})
}

func assistBody(t *testing.T, request *inference.Request, opts wire.CodecOpts) map[string]any {
	t.Helper()
	encoded := encode(t, cloudCodeAssistCodec(), request, opts)
	body, err := io.ReadAll(encoded.Body)
	if err != nil {
		t.Fatalf("read request body: %v", err)
	}
	var envelope map[string]any
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("decode request body: %v", err)
	}
	inner, ok := envelope["request"].(map[string]any)
	if !ok {
		t.Fatalf("envelope = %v, want the inner request", envelope)
	}
	return inner
}

func assistCall(t *testing.T, request *inference.Request, name string, opts ...wire.CodecOpts) map[string]any {
	t.Helper()
	options := wire.CodecOpts{CredentialRef: "token", Project: "p"}
	if len(opts) > 0 {
		options = opts[0]
	}
	contents := assistBody(t, request, options)["contents"].([]any)
	for _, raw := range contents {
		turn := raw.(map[string]any)
		if turn["role"] != "model" {
			continue
		}
		for _, rawPart := range turn["parts"].([]any) {
			part := rawPart.(map[string]any)
			call, _ := part["functionCall"].(map[string]any)
			if call["name"] == name {
				return part
			}
		}
	}
	t.Fatalf("contents = %v, want a call named %s", contents, name)
	return nil
}

func TestModule(t *testing.T) {
	t.Run("the module satisfies the codec contract", func(t *testing.T) {
		var module wire.Codec = google.Module()
		events, err := module.DecodeResponse(readFixture(t, "google/gemini_nonstreaming.json"))
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
		Model:     "gemini-2.0-flash",
		Stream:    stream,
		MaxTokens: 64,
		Messages: []inference.Message{
			{Role: inference.RoleSystem, Content: []inference.ContentPart{{Type: inference.ContentTypeText, Text: "be brief"}}},
			{Role: inference.RoleUser, Content: []inference.ContentPart{{Type: inference.ContentTypeText, Text: "hello"}}},
		},
	}
}

func conversationRequest() *inference.Request {
	temperature := 0.5
	request := canonicalRequest(false)
	request.Temperature = &temperature
	request.Messages = append(request.Messages,
		inference.Message{Role: inference.RoleAssistant, ToolCalls: []inference.ToolCall{{ID: "call_1", Name: "get_weather", Arguments: `{"city":"Hanoi"}`}}},
		inference.Message{Role: inference.RoleTool, ToolCallID: "call_1", Content: []inference.ContentPart{{Type: inference.ContentTypeText, Text: `{"temp":31}`}}},
	)
	return request
}

func encode(t *testing.T, module wire.Codec, request *inference.Request, opts wire.CodecOpts) *http.Request {
	t.Helper()
	encoded, err := module.EncodeRequest(request, opts)
	if err != nil {
		t.Fatalf("EncodeRequest() error = %v", err)
	}
	return encoded
}

func newTestRequest(t *testing.T, request *inference.Request, opts wire.CodecOpts) *http.Request {
	t.Helper()
	return encode(t, google.Module(), request, opts)
}

func encodeBody(t *testing.T, request *inference.Request, opts wire.CodecOpts) map[string]any {
	t.Helper()
	body, err := io.ReadAll(newTestRequest(t, request, opts).Body)
	if err != nil {
		t.Fatalf("read request body: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode request body: %v", err)
	}
	return payload
}

func lastParts(t *testing.T, request *inference.Request) []any {
	t.Helper()
	contents := encodeBody(t, request, wire.CodecOpts{CredentialRef: "api-key-1"})["contents"].([]any)
	last := contents[len(contents)-1].(map[string]any)
	return last["parts"].([]any)
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	body, err := os.ReadFile(testkit.FixturePath(name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return body
}

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

func failureOf(events []inference.Event) *inference.ErrorInfo {
	for _, event := range events {
		if event.Kind == inference.EventError {
			return event.Error
		}
	}
	return nil
}

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

func sameJSON(left, right string) bool {
	var decoded, expected any
	if json.Unmarshal([]byte(left), &decoded) != nil || json.Unmarshal([]byte(right), &expected) != nil {
		return false
	}
	return reflect.DeepEqual(decoded, expected)
}

func TestEncodeRequestDetails(t *testing.T) {
	t.Run("a codec without a configured mode defaults to AI Studio", func(t *testing.T) {
		request := encode(t, google.NewCodec(google.Config{}), canonicalRequest(false), wire.CodecOpts{CredentialRef: "api-key-1"})
		if request.URL.Path != "/v1beta/models/gemini-2.0-flash:generateContent" {
			t.Fatalf("path = %s, want the AI Studio path", request.URL.Path)
		}
		if request.Header.Get(google.APIKeyHeader) != "api-key-1" || request.URL.RawQuery != "" {
			t.Fatalf("query = %q, want the key in its header and no streaming flag", request.URL.RawQuery)
		}
	})

	t.Run("a request without a system prompt sends no instruction", func(t *testing.T) {
		request := canonicalRequest(false)
		request.Messages = request.Messages[1:]
		body := encodeBody(t, request, wire.CodecOpts{CredentialRef: "api-key-1"})
		if body["systemInstruction"] != nil {
			t.Fatalf("systemInstruction = %v, want none", body["systemInstruction"])
		}
	})

	t.Run("several system prompts join into one instruction", func(t *testing.T) {
		request := canonicalRequest(false)
		request.Messages = append(request.Messages, inference.Message{
			Role:    inference.RoleSystem,
			Content: []inference.ContentPart{{Type: inference.ContentTypeText, Text: "and now"}},
		})
		body := encodeBody(t, request, wire.CodecOpts{CredentialRef: "api-key-1"})
		instruction := body["systemInstruction"].(map[string]any)["parts"].([]any)[0].(map[string]any)
		if instruction["text"] != "be brief\n\nand now" {
			t.Fatalf("systemInstruction = %v, want both prompts", instruction)
		}
	})

	t.Run("a named tool result keeps the name the client sent", func(t *testing.T) {
		request := canonicalRequest(false)
		request.Messages = append(request.Messages, inference.Message{
			Role:       inference.RoleTool,
			ToolCallID: "call_1",
			Name:       "lookup",
			Content:    []inference.ContentPart{{Type: inference.ContentTypeText, Text: "done"}},
		})
		body := encodeBody(t, request, wire.CodecOpts{CredentialRef: "api-key-1"})
		contents := body["contents"].([]any)
		response := contents[len(contents)-1].(map[string]any)["parts"].([]any)[0].(map[string]any)["functionResponse"].(map[string]any)
		if response["name"] != "lookup" {
			t.Fatalf("functionResponse = %v, want the name from the request", response)
		}
	})

	t.Run("an empty result still sends an object", func(t *testing.T) {
		request := canonicalRequest(false)
		request.Messages = append(request.Messages, inference.Message{Role: inference.RoleTool, Name: "lookup", ToolCallID: "call_1"})
		body := encodeBody(t, request, wire.CodecOpts{CredentialRef: "api-key-1"})
		contents := body["contents"].([]any)
		response := contents[len(contents)-1].(map[string]any)["parts"].([]any)[0].(map[string]any)["functionResponse"].(map[string]any)
		if response["response"].(map[string]any)["result"] != "" {
			t.Fatalf("functionResponse = %v, want an empty result", response)
		}
	})
}

func TestStreamDecodingDetails(t *testing.T) {
	t.Run("a single frame decodes in isolation", func(t *testing.T) {
		events, err := google.Module().DecodeResponseEvent(wire.SSEEvent{Data: `{"candidates":[{"content":{"parts":[{"text":"solo"}]}}]}`})
		if err != nil {
			t.Fatalf("DecodeResponseEvent() error = %v", err)
		}
		if len(events) != 1 || events[0].Text != "solo" {
			t.Fatalf("events = %+v, want the text delta", events)
		}
	})

	t.Run("a thought part without text is ignored", func(t *testing.T) {
		data := `{"candidates":[{"content":{"parts":[{"thought":true,"text":""}]}}]}`
		if events := pushStreamData(t, google.Module().NewStreamDecoder(), data); len(events) != 0 {
			t.Fatalf("events = %+v, want nothing", events)
		}
	})

	t.Run("a plain stop finish carries no error", func(t *testing.T) {
		events := pushStreamData(t, google.Module().NewStreamDecoder(), `{"candidates":[{"finishReason":"STOP","safetyRatings":[{"category":"HARM_CATEGORY_HARASSMENT","probability":"NEGLIGIBLE"}]}]}`)
		if len(events) != 0 {
			t.Fatalf("events = %+v, want nothing", events)
		}
	})
}

func TestSanitizeSchemaDetails(t *testing.T) {
	t.Run("properties that are not an object are dropped", func(t *testing.T) {
		cleaned := string(google.SanitizeSchema(json.RawMessage(`{"type":"object","properties":5}`)))
		if !sameJSON(cleaned, `{"type":"object"}`) {
			t.Fatalf("schema = %s, want a non-object properties bag dropped", cleaned)
		}
	})

	t.Run("an incompatible union is dropped", func(t *testing.T) {
		cleaned := string(google.SanitizeSchema(json.RawMessage(`{"anyOf":[{"type":"string","title":"a"},{"type":"number"}]}`)))
		if !sameJSON(cleaned, `{}`) {
			t.Fatalf("schema = %s, want the union dropped", cleaned)
		}
	})

	t.Run("a type union keeps the first allowed type", func(t *testing.T) {
		cleaned := string(google.SanitizeSchema(json.RawMessage(`{"type":["string","boolean","number"]}`)))
		if !sameJSON(cleaned, `{"type":"string"}`) {
			t.Fatalf("schema = %s, want the first allowed type", cleaned)
		}
	})

	t.Run("null in a type union marks the schema nullable", func(t *testing.T) {
		cleaned := string(google.SanitizeSchema(json.RawMessage(`{"type":["string","null"]}`)))
		if !sameJSON(cleaned, `{"nullable":true,"type":"string"}`) {
			t.Fatalf("schema = %s, want a nullable string", cleaned)
		}
	})

	t.Run("a string or null union becomes a nullable string", func(t *testing.T) {
		cleaned := string(google.SanitizeSchema(json.RawMessage(`{"anyOf":[{"type":"string"},{"type":"null"}]}`)))
		if !sameJSON(cleaned, `{"nullable":true,"type":"string"}`) {
			t.Fatalf("schema = %s, want a nullable string", cleaned)
		}
	})

	t.Run("an enum union becomes one string enum", func(t *testing.T) {
		cleaned := string(google.SanitizeSchema(json.RawMessage(`{"anyOf":[{"type":"string","enum":["alpha","beta"]},{"type":"string","enum":["beta","gamma",5]}]}`)))
		if !sameJSON(cleaned, `{"enum":["alpha","beta","gamma"],"type":"string"}`) {
			t.Fatalf("schema = %s, want one string enum", cleaned)
		}
	})

	t.Run("a typeless enum union loses its type", func(t *testing.T) {
		cleaned := string(google.SanitizeSchema(json.RawMessage(`{"anyOf":[{"enum":["x"]},{"enum":["x"]}]}`)))
		if !sameJSON(cleaned, `{"enum":["x"]}`) {
			t.Fatalf("schema = %s, want one typeless enum", cleaned)
		}
	})

	t.Run("a non-string enum union is dropped", func(t *testing.T) {
		cleaned := string(google.SanitizeSchema(json.RawMessage(`{"anyOf":[{"type":"number","enum":[1,2]},{"type":"number","enum":[2]}]}`)))
		if !sameJSON(cleaned, `{}`) {
			t.Fatalf("schema = %s, want the union dropped", cleaned)
		}
	})

	t.Run("an enum branch with extra keys is dropped", func(t *testing.T) {
		cleaned := string(google.SanitizeSchema(json.RawMessage(`{"anyOf":[{"type":"string","enum":["a"],"description":"d"},{"type":"string","enum":["b"]}]}`)))
		if !sameJSON(cleaned, `{}`) {
			t.Fatalf("schema = %s, want the union dropped", cleaned)
		}
	})
	t.Run("tuple prefixes are dropped", func(t *testing.T) {
		cleaned := string(google.SanitizeSchema(json.RawMessage(`{"type":"array","items":{"type":"string"},"prefixItems":[{"type":"number"}]}`)))
		if !sameJSON(cleaned, `{"items":{"type":"string"},"type":"array"}`) {
			t.Fatalf("schema = %s, want prefixItems gone", cleaned)
		}
	})
}
