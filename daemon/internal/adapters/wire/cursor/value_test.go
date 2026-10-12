package cursor

import (
	"encoding/json"
	"reflect"
	"testing"

	adapterscursor "github.com/jonaskahn/relo/internal/adapters/cursor"
	"github.com/jonaskahn/relo/internal/inference"
)

// TestToolDefinitionCarriesTheSchemaAsAProtoValue pins the field a declared
// tool turn died on: inputSchema is a google.protobuf.Value, never the JSON
// text an upstream that reads a Value does not decode, which made every
// tool-bearing turn fail.
func TestToolDefinitionCarriesTheSchemaAsAProtoValue(t *testing.T) {
	const schema = `{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`
	tools, err := BuildToolMap([]inference.Tool{{Name: "read", Description: "read a file", Parameters: []byte(schema)}})
	if err != nil {
		t.Fatalf("BuildToolMap() error = %v", err)
	}
	defs := tools.Defs()
	if len(defs) != 1 {
		t.Fatalf("Defs() = %d definitions, want one", len(defs))
	}
	fields, err := adapterscursor.Parse(defs[0])
	if err != nil {
		t.Fatalf("parse tool definition: %v", err)
	}
	var raw []byte
	for _, field := range fields {
		if field.Number == defSchema && field.Wire == 2 {
			raw = field.Raw
		}
	}
	if raw == nil {
		t.Fatal("tool definition carries no input schema")
	}
	var want any
	if err := json.Unmarshal([]byte(schema), &want); err != nil {
		t.Fatalf("schema: %v", err)
	}
	got, ok := decodeJSONValue(raw)
	if !ok {
		t.Fatal("input schema is not a google.protobuf.Value")
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("input schema = %#v, want %#v", got, want)
	}
}

// TestSchemaValueIsAProtoValue pins the form McpToolDefinition.inputSchema
// carries. The upstream reads the field as google.protobuf.Value, so every
// JSON kind, including a zero number and a false that a skipping encoder
// would drop, has to survive the round trip.
func TestSchemaValueIsAProtoValue(t *testing.T) {
	schema := []byte(`{"type":"object","properties":{"q":{"type":"string","maxLength":10},"all":false,"none":null,"empty":""},"required":["q"],"minimum":0}`)
	var want any
	if err := json.Unmarshal(schema, &want); err != nil {
		t.Fatalf("schema: %v", err)
	}
	got, ok := decodeJSONValue(encodeSchemaValue(schema))
	if !ok {
		t.Fatal("encoded schema is not a google.protobuf.Value")
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("schema = %#v, want %#v", got, want)
	}
}

// TestDecodeArgValue covers both shapes a McpArgs entry arrives in: the proto
// Value the current builds send, which may wrap JSON text, and the JSON text
// itself from the older ones.
func TestDecodeArgValue(t *testing.T) {
	cases := []struct {
		name string
		raw  []byte
		want any
	}{
		{"proto value", encodeJSONValue(map[string]any{"path": "/tmp"}), map[string]any{"path": "/tmp"}},
		{"proto value wrapping json", encodeJSONValue(`{"path":"/tmp"}`), map[string]any{"path": "/tmp"}},
		{"proto number", encodeJSONValue(float64(2)), float64(2)},
		{"proto false", encodeJSONValue(false), false},
		{"json text", []byte(`{"path":"/tmp"}`), map[string]any{"path": "/tmp"}},
		{"bare text", []byte("tmp"), "tmp"},
	}
	for _, test := range cases {
		if got := decodeArgValue(test.raw); !reflect.DeepEqual(got, test.want) {
			t.Fatalf("%s: decodeArgValue() = %#v, want %#v", test.name, got, test.want)
		}
	}
}
