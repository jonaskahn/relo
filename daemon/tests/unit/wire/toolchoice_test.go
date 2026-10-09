package codec_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/wire"
	"github.com/jonaskahn/relo/internal/adapters/wire/anthropic"
	"github.com/jonaskahn/relo/internal/adapters/wire/formats"
	"github.com/jonaskahn/relo/internal/adapters/wire/openaichat"
	"github.com/jonaskahn/relo/internal/adapters/wire/openairesponses"
	"github.com/jonaskahn/relo/internal/inference"
)

// wireFormats is the registry the composition root injects, reached here
// through the same package the transport reads it from.
func wireFormats() *formats.Registry {
	return formats.New()
}

// toolChoiceSurface is one client protocol and one tool_choice as that protocol's
// clients write it.
type toolChoiceSurface struct {
	name string
	body string
}

// decodeSurface reads a client request the way its own surface does.
func decodeSurface(t *testing.T, surface toolChoiceSurface) *inference.Request {
	t.Helper()
	codec, found := wireFormats().Inbound(surfaceWire(surface.name))
	if !found {
		t.Fatalf("Relo does not implement the %s surface", surface.name)
	}
	decoded, err := codec.DecodeRequest(newRequest(t, surface.body))
	if err != nil {
		t.Fatalf("%s DecodeRequest() error = %v", surface.name, err)
	}
	return decoded
}

func surfaceWire(name string) string {
	switch name {
	case "responses":
		return inference.SurfaceResponses
	case "messages":
		return inference.SurfaceMessages
	default:
		return inference.SurfaceChatCompletions
	}
}

// encodeFor renders a canonical request the way one upstream family sends it.
func encodeFor(t *testing.T, codec wire.Codec, request *inference.Request) map[string]any {
	t.Helper()
	outbound, err := codec.EncodeRequest(request, wire.CodecOpts{CredentialRef: "sk-test"})
	if err != nil {
		t.Fatalf("EncodeRequest() error = %v", err)
	}
	raw, err := io.ReadAll(outbound.Body)
	if err != nil {
		t.Fatalf("read the upstream body: %v", err)
	}
	body := map[string]any{}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode the upstream body %q: %v", raw, err)
	}
	return body
}

// decodeBody reads an encoded request back through the same protocol's client
// codec, so a round trip is checked through the translation a retry makes.
func decodeBody(t *testing.T, outbound *http.Request, codec wire.CodecModule) *inference.Request {
	t.Helper()
	raw, err := io.ReadAll(outbound.Body)
	if err != nil {
		t.Fatalf("read the outbound body: %v", err)
	}
	inbound, found := wireFormats().Inbound(codecSurface(codec))
	if !found {
		t.Fatalf("Relo has no inbound surface for %T", codec)
	}
	replayed, err := inbound.DecodeRequest(newHTTPRequest(t, raw))
	if err != nil {
		t.Fatalf("DecodeRequest() error = %v", err)
	}
	return replayed
}

func codecSurface(codec wire.CodecModule) string {
	switch codec.(type) {
	case *openaichat.Codec:
		return inference.SurfaceChatCompletions
	case *openairesponses.Codec:
		return inference.SurfaceResponses
	default:
		return inference.SurfaceMessages
	}
}

func newHTTPRequest(t *testing.T, body []byte) *http.Request {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, "/v1", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build the request: %v", err)
	}
	return request
}

func assertChoice(t *testing.T, body map[string]any, want map[string]any) {
	t.Helper()
	for field, value := range want {
		encoded, err := json.Marshal(body[field])
		if err != nil {
			t.Fatalf("read %s: %v", field, err)
		}
		expected, err := json.Marshal(value)
		if err != nil {
			t.Fatalf("encode the wanted %s: %v", field, err)
		}
		if string(encoded) != string(expected) {
			t.Fatalf("%s = %s, want %s", field, encoded, expected)
		}
	}
}

// toolNames reads the declared tool names from any of the three bodies, since
// each names its tools in its own place.
func toolNames(t *testing.T, body map[string]any) []string {
	t.Helper()
	declared, _ := body["tools"].([]any)
	names := make([]string, 0, len(declared))
	for _, entry := range declared {
		tool, _ := entry.(map[string]any)
		if name, ok := tool["name"].(string); ok {
			names = append(names, name)
			continue
		}
		// The Chat wire nests the name one level down, under the function.
		if nested, ok := tool["function"].(map[string]any); ok {
			if name, ok := nested["name"].(string); ok {
				names = append(names, name)
			}
		}
	}
	return names
}

// TestToolChoiceSurvivesTranslation covers the client's constraint on which
// tool the model calls: it is a decision the caller made, so a family that
// cannot carry it must say so rather than answer with a different instruction.
func TestToolChoiceSurvivesTranslation(t *testing.T) {
	surfaces := []toolChoiceSurface{
		{name: "chat", body: `{"model":"m","stream":false,"messages":[{"role":"user","content":"go"}],` +
			`"tools":[{"type":"function","function":{"name":"read","description":"r",` +
			`"parameters":{"type":"object","properties":{"path":{"type":"string"}}}}}],"tool_choice":"none"}`},
		{name: "responses", body: `{"model":"m","stream":false,"input":` +
			`[{"role":"user","content":[{"type":"input_text","text":"go"}]}],` +
			`"tools":[{"type":"function","name":"read","description":"r",` +
			`"parameters":{"type":"object","properties":{"path":{"type":"string"}}}}],"tool_choice":"none"}`},
		{name: "messages", body: `{"model":"m","max_tokens":64,"stream":false,"messages":` +
			`[{"role":"user","content":"go"}],` +
			`"tools":[{"name":"read","description":"r","input_schema":{"type":"object"}}],` +
			`"tool_choice":{"type":"none"}}`},
	}

	families := []struct {
		name  string
		codec wire.Codec
		// want is the choice the upstream body must carry, which differs per
		// wire because the wires name the same decision differently.
		want map[string]any
	}{
		{
			name: "openai-chat", codec: openaichat.Module(),
			want: map[string]any{"tool_choice": "none"},
		},
		{
			name: "openai-responses", codec: openairesponses.Module(),
			want: map[string]any{"tool_choice": "none"},
		},
		{
			name: "anthropic-messages", codec: anthropic.Module(),
			want: map[string]any{"tool_choice": map[string]any{"type": "none"}},
		},
	}

	for _, surface := range surfaces {
		for _, family := range families {
			t.Run(surface.name+" -> "+family.name, func(t *testing.T) {
				decoded := decodeSurface(t, surface)
				if decoded.ToolChoice == nil || decoded.ToolChoice.Mode != inference.ToolChoiceNone {
					t.Fatalf("tool choice = %+v, want the client's own none", decoded.ToolChoice)
				}
				body := encodeFor(t, family.codec, decoded)
				assertChoice(t, body, family.want)
			})
		}
	}
}

// TestToolChoiceNamesTranslatePerWire covers a demand for one named tool, which
// is the shape a coding client uses to force a particular read.
func TestToolChoiceNamesTranslatePerWire(t *testing.T) {
	choice := &inference.ToolChoice{Mode: inference.ToolChoiceTool, Name: "read"}
	request := &inference.Request{
		Model: "m", ToolChoice: choice,
		Tools: []inference.Tool{{Name: "read", Parameters: json.RawMessage(`{"type":"object"}`)}},
	}

	for _, entry := range []struct {
		name  string
		codec wire.Codec
		want  map[string]any
	}{
		{
			name: "openai-chat", codec: openaichat.Module(),
			want: map[string]any{"tool_choice": map[string]any{
				"type": "function", "function": map[string]any{"name": "read"}}},
		},
		{
			name: "openai-responses", codec: openairesponses.Module(),
			want: map[string]any{"tool_choice": map[string]any{"type": "function", "name": "read"}},
		},
		{
			name: "anthropic-messages", codec: anthropic.Module(),
			want: map[string]any{"tool_choice": map[string]any{"type": "tool", "name": "read"}},
		},
	} {
		t.Run(entry.name, func(t *testing.T) {
			assertChoice(t, encodeFor(t, entry.codec, request), entry.want)
		})
	}
}

// TestToolChoiceIsNeverInvented covers the two defaults that matter to a
// gateway inspecting the request: Relo adds no tool the client did not send,
// and a request that named no choice keeps carrying none.
func TestToolChoiceIsNeverInvented(t *testing.T) {
	tools := []inference.Tool{{Name: "bash"}, {Name: "glob"}, {Name: "grep"}, {Name: "read"}}

	for _, entry := range []struct {
		name  string
		codec wire.Codec
	}{
		{"openai-chat", openaichat.Module()},
		{"openai-responses", openairesponses.Module()},
		{"anthropic-messages", anthropic.Module()},
	} {
		t.Run(entry.name+" keeps the client's own tools and no choice", func(t *testing.T) {
			body := encodeFor(t, entry.codec, &inference.Request{Model: "m", Tools: tools})
			declared := toolNames(t, body)
			if len(declared) != len(tools) {
				t.Fatalf("tools = %v, want exactly the four the client sent", declared)
			}
			for index, tool := range tools {
				if declared[index] != tool.Name {
					t.Fatalf("tools = %v, want the client's own names unchanged", declared)
				}
			}
			if _, present := body["tool_choice"]; present {
				t.Fatalf("body = %v, want no tool choice invented for a client that sent none", body)
			}
		})

		t.Run(entry.name+" keeps a partial tool set partial", func(t *testing.T) {
			partial := tools[:2]
			body := encodeFor(t, entry.codec, &inference.Request{Model: "m", Tools: partial})
			declared := toolNames(t, body)
			if len(declared) != len(partial) {
				t.Fatalf("tools = %v, want only what the client sent", declared)
			}
		})
	}
}

// TestToolChoiceRoundTripsThroughEverySurface covers the decision coming back
// out the way the client sent it, which is what a retry or a second hop needs.
func TestToolChoiceRoundTripsThroughEverySurface(t *testing.T) {
	for _, mode := range []string{inference.ToolChoiceAuto, inference.ToolChoiceNone, inference.ToolChoiceRequired} {
		t.Run(mode, func(t *testing.T) {
			for _, entry := range []struct {
				name  string
				codec wire.CodecModule
			}{
				{"chat", openaichat.NewCodec("")},
				{"responses", openairesponses.NewCodec("")},
				{"anthropic", anthropic.NewCodec("")},
			} {
				original := &inference.Request{
					Model: "m", Stream: false,
					Messages:   []inference.Message{{Role: inference.RoleUser, Content: []inference.ContentPart{{Type: inference.ContentTypeText, Text: "go"}}}},
					ToolChoice: &inference.ToolChoice{Mode: mode},
				}
				outbound, err := entry.codec.EncodeRequest(original, wire.CodecOpts{CredentialRef: "sk-test"})
				if err != nil {
					t.Fatalf("%s EncodeRequest() error = %v", entry.name, err)
				}
				replayed := decodeBody(t, outbound, entry.codec)
				if replayed.ToolChoice == nil || replayed.ToolChoice.Mode != mode {
					t.Fatalf("%s tool choice = %+v, want %q", entry.name, replayed.ToolChoice, mode)
				}
			}
		})
	}
}
