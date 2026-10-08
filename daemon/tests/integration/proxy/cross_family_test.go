package proxy_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"github.com/jonaskahn/relo/internal/inference"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/wire"
	"github.com/jonaskahn/relo/internal/adapters/wire/anthropic"
	"github.com/jonaskahn/relo/internal/adapters/wire/google"
	"github.com/jonaskahn/relo/internal/adapters/wire/openaichat"
	"github.com/jonaskahn/relo/internal/adapters/wire/openairesponses"
)

// The client request each inbound surface accepts. They describe the same
// conversation, so the whole matrix translates identical meaning.
const (
	chatSurfaceRequest = `{
  "model": "gpt-4o",
  "stream": true,
  "max_tokens": 128,
  "reasoning_effort": "low",
  "messages": [
    {"role": "system", "content": "be brief"},
    {"role": "user", "content": "hello"}
  ],
  "tools": [{"type": "function", "function": {"name": "get_weather", "description": "look it up", "parameters": {"type": "object", "properties": {"city": {"type": "string"}}}}}]
}`

	responsesSurfaceRequest = `{
  "model": "gpt-5",
  "stream": true,
  "max_output_tokens": 128,
  "instructions": "be brief",
  "reasoning": {"effort": "low", "summary": "auto"},
  "input": [{"role": "user", "content": [{"type": "input_text", "text": "hello"}]}],
  "tools": [{"type": "function", "name": "get_weather", "description": "look it up", "parameters": {"type": "object", "properties": {"city": {"type": "string"}}}}]
}`

	messagesSurfaceRequest = `{
  "model": "claude-sonnet-4",
  "max_tokens": 128,
  "stream": true,
  "system": "be brief",
  "thinking": {"type": "enabled", "budget_tokens": 4096},
  "messages": [{"role": "user", "content": "hello"}],
  "tools": [{"name": "get_weather", "description": "look it up", "input_schema": {"type": "object", "properties": {"city": {"type": "string"}}}}]
}`
)

// surface is one client-facing protocol and one request written the way its
// clients write it.
type surface struct {
	name string
	body string
}

// family is one upstream protocol and the way a request for it is read.
type family struct {
	name   string
	codec  wire.Codec
	assert func(t *testing.T, body map[string]any)
}

func surfaces() []surface {
	return []surface{
		{name: "chat", body: chatSurfaceRequest},
		{name: "responses", body: responsesSurfaceRequest},
		{name: "messages", body: messagesSurfaceRequest},
	}
}

func families() []family {
	return []family{
		{name: "openai-chat", codec: openaichat.Module(), assert: assertChatPayload},
		{name: "openai-responses", codec: openairesponses.Module(), assert: assertResponsesPayload},
		{name: "anthropic-messages", codec: anthropic.Module(), assert: assertMessagesPayload},
		{name: "google-gemini", codec: google.Module(), assert: assertGooglePayload},
	}
}

// TestCrossFamilyTranslation checks that a request arriving on any surface
// reaches a provider of any family as a valid request.
func TestCrossFamilyTranslation(t *testing.T) {
	t.Run("every inbound surface produces a valid request for every outbound family", func(t *testing.T) {
		for _, inbound := range surfaces() {
			for _, outbound := range families() {
				t.Run(inbound.name+" -> "+outbound.name, func(t *testing.T) {
					body := translate(t, inbound, outbound)
					outbound.assert(t, body)
				})
			}
		}
	})

	t.Run("tool schemas translate correctly", func(t *testing.T) {
		for _, outbound := range families() {
			t.Run(outbound.name, func(t *testing.T) {
				for _, inbound := range surfaces() {
					body := translate(t, inbound, outbound)
					if toolName(outbound.name, body) != "get_weather" {
						t.Fatalf("tools = %v, want the declared tool for %s", body["tools"], inbound.name)
					}
					if !toolSchemaAcceptsCity(outbound.name, body) {
						t.Fatalf("parameters = %v, want the schema properties carried", body["tools"])
					}
				}
			})
		}
	})

	t.Run("system messages handled", func(t *testing.T) {
		for _, outbound := range families() {
			for _, inbound := range surfaces() {
				body := translate(t, inbound, outbound)
				if !carriesText(body, "be brief") {
					t.Fatalf("body = %v, want the system prompt from the %s surface", body, inbound.name)
				}
				if !carriesText(body, "hello") {
					t.Fatalf("body = %v, want the user turn from the %s surface", body, inbound.name)
				}
			}
		}
	})

	t.Run("reasoning config translates where supported", func(t *testing.T) {
		request := canonicalConversation()
		request.Reasoning = &inference.ReasoningConfig{Effort: "high"}
		tests := []struct {
			family string
			value  func(t *testing.T, body map[string]any) any
		}{
			{"openai-chat", func(t *testing.T, body map[string]any) any { return body["reasoning_effort"] }},
			{"openai-responses", func(t *testing.T, body map[string]any) any {
				return body["reasoning"].(map[string]any)["effort"]
			}},
			{"anthropic-messages", func(t *testing.T, body map[string]any) any {
				return body["thinking"].(map[string]any)["budget_tokens"]
			}},
			{"google-gemini", func(t *testing.T, body map[string]any) any {
				return body["generationConfig"].(map[string]any)["thinkingConfig"].(map[string]any)["thinkingBudget"]
			}},
		}
		for _, test := range tests {
			t.Run(test.family, func(t *testing.T) {
				body := encodeFor(t, test.family, request)
				value := test.value(t, body)
				if value == nil || value == float64(0) {
					t.Fatalf("body = %v, want the reasoning config carried", body)
				}
			})
		}
	})

	t.Run("image input unsupported -> typed error", func(t *testing.T) {
		request := canonicalConversation()
		request.Messages = append(request.Messages, inference.Message{Role: inference.RoleUser, Content: []inference.ContentPart{
			{Type: inference.ContentTypeImage, ImageURL: "https://example.invalid/a.png"},
		}})
		if _, err := google.Module().EncodeRequest(request, wire.CodecOpts{CredentialRef: "token"}); !errors.Is(err, wire.ErrUnsupportedFeature) {
			t.Fatalf("EncodeRequest() error = %v, want the unsupported feature sentinel", err)
		}
		for _, outbound := range []string{"openai-chat", "openai-responses", "anthropic-messages"} {
			if _, err := codecFor(outbound).EncodeRequest(request, wire.CodecOpts{CredentialRef: "token"}); err != nil {
				t.Fatalf("%s EncodeRequest() error = %v, want the image to travel", outbound, err)
			}
		}
	})

	t.Run("a data URL image travels to every family", func(t *testing.T) {
		request := canonicalConversation()
		request.Messages = append(request.Messages, inference.Message{Role: inference.RoleUser, Content: []inference.ContentPart{
			{Type: inference.ContentTypeImage, ImageURL: "data:image/png;base64,aGVsbG8="},
		}})
		for _, outbound := range families() {
			body, err := outbound.codec.EncodeRequest(request, wire.CodecOpts{CredentialRef: "token"})
			if err != nil {
				t.Fatalf("%s EncodeRequest() error = %v, want the inline image to travel", outbound.name, err)
			}
			encoded, err := io.ReadAll(result(body))
			if err != nil {
				t.Fatalf("read body: %v", err)
			}
			if !bytes.Contains(encoded, []byte("aGVsbG8=")) {
				t.Fatalf("%s body = %s, want the inline bytes", outbound.name, encoded)
			}
		}
	})
}

// translate decodes a client request the way its surface does and encodes
// it the way the upstream family does.
func translate(t *testing.T, inbound surface, outbound family) map[string]any {
	t.Helper()
	return bodyOf(t, outbound.codec, requestFrom(t, inbound))
}

func requestFrom(t *testing.T, inbound surface) *inference.Request {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, "/", strings.NewReader(inbound.body))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	decoded, err := inboundCodec(inbound.name).DecodeRequest(request)
	if err != nil {
		t.Fatalf("DecodeRequest() error = %v", err)
	}
	return decoded
}

func inboundCodec(name string) wire.InboundCodec {
	if name == "chat" {
		return openaichat.NewChatCompletionsCodec()
	}
	if name == "responses" {
		return openairesponses.NewInbound()
	}
	return anthropic.NewInbound()
}

func codecFor(name string) wire.Codec {
	for _, outbound := range families() {
		if outbound.name == name {
			return outbound.codec
		}
	}
	return nil
}

func canonicalConversation() *inference.Request {
	return &inference.Request{
		Model:     "gpt-4o",
		Stream:    true,
		MaxTokens: 128,
		Messages: []inference.Message{
			{Role: inference.RoleSystem, Content: []inference.ContentPart{{Type: inference.ContentTypeText, Text: "be brief"}}},
			{Role: inference.RoleUser, Content: []inference.ContentPart{{Type: inference.ContentTypeText, Text: "hello"}}},
		},
		Tools: []inference.Tool{{
			Name:        "get_weather",
			Description: "look it up",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}},"required":["city"]}`),
		}},
	}
}

func encodeFor(t *testing.T, family string, request *inference.Request) map[string]any {
	t.Helper()
	module := codecFor(family)
	if module == nil {
		t.Fatalf("unknown family %q", family)
	}
	return bodyOf(t, module, request)
}

func bodyOf(t *testing.T, module wire.Codec, request *inference.Request) map[string]any {
	t.Helper()
	encoded, err := module.EncodeRequest(request, wire.CodecOpts{CredentialRef: "sk-test"})
	if err != nil {
		t.Fatalf("EncodeRequest() error = %v", err)
	}
	body, err := io.ReadAll(result(encoded))
	if err != nil {
		t.Fatalf("read request body: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode request body: %v", err)
	}
	return payload
}

func result(request *http.Request) io.Reader {
	return request.Body
}

func assertChatPayload(t *testing.T, body map[string]any) {
	t.Helper()
	system := messageWithRole(body["messages"], "system")
	if system == nil || !strings.Contains(textOfContent(system["content"]), "be brief") {
		t.Fatalf("messages = %v, want the system prompt as a message", body["messages"])
	}
	if toolName("openai-chat", body) != "get_weather" || body["reasoning_effort"] == nil {
		t.Fatalf("body = %v, want the tool and the reasoning effort", body)
	}
}

func assertResponsesPayload(t *testing.T, body map[string]any) {
	t.Helper()
	if !strings.Contains(toString(body["instructions"]), "be brief") {
		t.Fatalf("instructions = %v, want the system prompt", body["instructions"])
	}
	if toolName("openai-responses", body) != "get_weather" {
		t.Fatalf("tools = %v, want the declared tool", body["tools"])
	}
	if body["reasoning"] == nil || body["store"] != false {
		t.Fatalf("body = %v, want the reasoning parameters and store false", body)
	}
}

func assertMessagesPayload(t *testing.T, body map[string]any) {
	t.Helper()
	if !strings.Contains(toString(body["system"]), "be brief") {
		t.Fatalf("system = %v, want the top-level system prompt", body["system"])
	}
	if toolName("anthropic-messages", body) != "get_weather" {
		t.Fatalf("tools = %v, want the declared tool", body["tools"])
	}
	if body["thinking"] == nil || body["max_tokens"] == nil {
		t.Fatalf("body = %v, want the thinking config and the token limit", body)
	}
}

func assertGooglePayload(t *testing.T, body map[string]any) {
	t.Helper()
	instruction := body["systemInstruction"].(map[string]any)
	if !strings.Contains(textOfContent(instruction["parts"]), "be brief") {
		t.Fatalf("systemInstruction = %v, want the system prompt", body["systemInstruction"])
	}
	if toolName("google-gemini", body) != "get_weather" {
		t.Fatalf("tools = %v, want the declared tool", body["tools"])
	}
	config := body["generationConfig"].(map[string]any)
	if config["thinkingConfig"] == nil {
		t.Fatalf("generationConfig = %v, want the thinking config", config)
	}
}

func toolName(family string, body map[string]any) string {
	tools, _ := body["tools"].([]any)
	if len(tools) == 0 {
		return ""
	}
	tool, _ := tools[0].(map[string]any)
	switch family {
	case "openai-chat":
		function, _ := tool["function"].(map[string]any)
		return toString(function["name"])
	case "google-gemini":
		group, _ := tool["functionDeclarations"].([]any)
		if len(group) == 0 {
			return ""
		}
		declaration, _ := group[0].(map[string]any)
		return toString(declaration["name"])
	default:
		return toString(tool["name"])
	}
}

func toolSchemaAcceptsCity(family string, body map[string]any) bool {
	schema := toolSchema(family, body)
	if schema == nil {
		return false
	}
	properties, _ := schema["properties"].(map[string]any)
	return properties["city"] != nil
}

func toolSchema(family string, body map[string]any) map[string]any {
	tools, _ := body["tools"].([]any)
	if len(tools) == 0 {
		return nil
	}
	tool, _ := tools[0].(map[string]any)
	switch family {
	case "openai-chat":
		function, _ := tool["function"].(map[string]any)
		schema, _ := function["parameters"].(map[string]any)
		return schema
	case "google-gemini":
		group, _ := tool["functionDeclarations"].([]any)
		if len(group) == 0 {
			return nil
		}
		declaration, _ := group[0].(map[string]any)
		schema, _ := declaration["parameters"].(map[string]any)
		return schema
	case "anthropic-messages":
		schema, _ := tool["input_schema"].(map[string]any)
		return schema
	default:
		schema, _ := tool["parameters"].(map[string]any)
		return schema
	}
}

func messageWithRole(messages any, role string) map[string]any {
	entries, _ := messages.([]any)
	for _, entry := range entries {
		message, _ := entry.(map[string]any)
		if message["role"] == role {
			return message
		}
	}
	return nil
}

func textOfContent(content any) string {
	switch value := content.(type) {
	case string:
		return value
	case []any:
		var builder strings.Builder
		for _, entry := range value {
			part, _ := entry.(map[string]any)
			builder.WriteString(toString(part["text"]))
		}
		return builder.String()
	default:
		return ""
	}
}

// carriesText reports whether a JSON document anywhere in the payload holds
// the given text, which is how a family-agnostic assertion checks meaning.
func carriesText(value any, want string) bool {
	switch node := value.(type) {
	case string:
		return strings.Contains(node, want)
	case []any:
		return anyOf(node, want)
	case map[string]any:
		for _, entry := range node {
			if carriesText(entry, want) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func anyOf(values []any, want string) bool {
	for _, value := range values {
		if carriesText(value, want) {
			return true
		}
	}
	return false
}

func toString(value any) string {
	text, _ := value.(string)
	return text
}
