package codec_test

import (
	"testing"

	"github.com/jonaskahn/relo/internal/inference"
)

// TestFreeformToolEncodesAsStringArgument pins the chat shape a freeform
// tool goes out with: one string input argument and no required list, so a
// serving template that mishandles a wrapped object schema still emits a
// structured call. An ordinary function tool keeps its own schema.
func TestFreeformToolEncodesAsStringArgument(t *testing.T) {
	request := &inference.Request{
		Model: "mimo-v2.6-flash",
		Tools: []inference.Tool{
			{Name: "exec", Freeform: true, Parameters: []byte(`{"type":"object","properties":{"input":{"type":"string"}},"required":["input"]}`)},
			{Name: "get_weather", Parameters: []byte(`{"type":"object","properties":{"city":{"type":"string"}}}`)},
		},
	}
	body := encodeChatBody(t, "https://api.xiaomimimo.com/v1", request)
	tools := body["tools"].([]any)
	exec := tools[0].(map[string]any)["function"].(map[string]any)["parameters"].(map[string]any)
	if _, required := exec["required"]; required {
		t.Fatalf("exec parameters = %v, want no required list for a freeform tool", exec)
	}
	props := exec["properties"].(map[string]any)
	if props["input"].(map[string]any)["type"] != "string" {
		t.Fatalf("exec parameters = %v, want a string input", exec)
	}
	weather := tools[1].(map[string]any)["function"].(map[string]any)["parameters"].(map[string]any)
	if _, ok := weather["properties"].(map[string]any)["city"]; !ok {
		t.Fatalf("get_weather parameters = %v, want its own schema", weather)
	}
}
