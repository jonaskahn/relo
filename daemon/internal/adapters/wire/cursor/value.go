// Encoding a JSON schema as google.protobuf.Value.
package cursor

import (
	"encoding/binary"
	"encoding/json"
	"math"
	"strings"

	"github.com/jonaskahn/relo/internal/adapters/cursor"
)

// The google.protobuf.Value oneof: exactly one case carries the value.
const (
	valueNull   = 1
	valueNumber = 2
	valueString = 3
	valueBool   = 4
	valueStruct = 5
	valueList   = 6

	structFieldEntry = 1
	structEntryKey   = 1
	structEntryValue = 2
	listValueItem    = 1
)

// encodeSchemaValue encodes a declared tool's parameter schema the way Cursor
// reads McpToolDefinition.inputSchema: as google.protobuf.Value bytes. The
// field is not JSON text, so an object schema has to arrive as a Struct and a
// number as a double; raw JSON there does not decode and the declared tool is
// refused upstream. A schema that is not JSON becomes an open object.
func encodeSchemaValue(raw []byte) []byte {
	var parsed any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return encodeJSONValue(map[string]any{"type": "object"})
	}
	return encodeJSONValue(parsed)
}

// encodeJSONValue encodes one JSON value as google.protobuf.Value bytes.
func encodeJSONValue(value any) []byte {
	var out cursor.Writer
	writeJSONValue(&out, value)
	return out.BytesOut()
}

func writeJSONValue(out *cursor.Writer, value any) {
	switch typed := value.(type) {
	case nil:
		out.PresentVarint(valueNull, 0)
	case bool:
		out.PresentVarint(valueBool, boolVarint(typed))
	case float64:
		out.Double(valueNumber, typed)
	case string:
		out.Present(valueString, []byte(typed))
	case []any:
		var list cursor.Writer
		for _, item := range typed {
			list.Present(listValueItem, encodeJSONValue(item))
		}
		out.Present(valueList, list.BytesOut())
	case map[string]any:
		var fields cursor.Writer
		for key, item := range typed {
			var entry cursor.Writer
			entry.Present(structEntryKey, []byte(key))
			entry.Present(structEntryValue, encodeJSONValue(item))
			fields.Present(structFieldEntry, entry.BytesOut())
		}
		out.Present(valueStruct, fields.BytesOut())
	}
}

func boolVarint(value bool) uint64 {
	if value {
		return 1
	}
	return 0
}

func float64FromFixed(field cursor.Field) float64 {
	if len(field.Raw) < 8 {
		return 0
	}
	return math.Float64frombits(binary.LittleEndian.Uint64(field.Raw))
}

// decodeJSONValue decodes google.protobuf.Value bytes into a JSON value. It
// reports false when the bytes are not a Value, which is how the same field
// arrives from the older builds that wrote the value as JSON text.
func decodeJSONValue(raw []byte) (any, bool) {
	fields, err := cursor.Parse(raw)
	if err != nil || len(fields) != 1 {
		return nil, false
	}
	field := fields[0]
	switch field.Number {
	case valueNull:
		return nil, field.Wire == 0
	case valueNumber:
		return float64FromFixed(field), field.Wire == 1
	case valueString:
		return string(field.Raw), field.Wire == 2
	case valueBool:
		return field.Varint != 0, field.Wire == 0
	case valueStruct:
		if field.Wire != 2 {
			return nil, false
		}
		return decodeJSONStruct(field.Raw)
	case valueList:
		if field.Wire != 2 {
			return nil, false
		}
		return decodeJSONList(field.Raw)
	default:
		return nil, false
	}
}

func decodeJSONStruct(raw []byte) (any, bool) {
	entries, err := cursor.Parse(raw)
	if err != nil {
		return nil, false
	}
	object := map[string]any{}
	for _, entry := range entries {
		if entry.Number != structFieldEntry || entry.Wire != 2 {
			return nil, false
		}
		parts, err := cursor.Parse(entry.Raw)
		if err != nil {
			return nil, false
		}
		key := ""
		var value any
		for _, part := range parts {
			switch part.Number {
			case structEntryKey:
				key = string(part.Raw)
			case structEntryValue:
				decoded, ok := decodeJSONValue(part.Raw)
				if !ok {
					return nil, false
				}
				value = decoded
			default:
				return nil, false
			}
		}
		object[key] = value
	}
	return object, true
}

func decodeJSONList(raw []byte) (any, bool) {
	items, err := cursor.Parse(raw)
	if err != nil {
		return nil, false
	}
	list := make([]any, 0, len(items))
	for _, item := range items {
		if item.Number != listValueItem || item.Wire != 2 {
			return nil, false
		}
		decoded, ok := decodeJSONValue(item.Raw)
		if !ok {
			return nil, false
		}
		list = append(list, decoded)
	}
	return list, true
}

// decodeArgValue decodes one McpArgs map value. The current builds send a
// google.protobuf.Value, which may itself wrap JSON text; the older builds
// sent the JSON text itself, so that is the fallback.
func decodeArgValue(raw []byte) any {
	if value, ok := decodeJSONValue(raw); ok {
		if text, isString := value.(string); isString {
			if looksLikeJSON(text) {
				var parsed any
				if err := json.Unmarshal([]byte(text), &parsed); err == nil {
					return parsed
				}
			}
		}
		return value
	}
	var parsed any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return string(raw)
	}
	return parsed
}

func looksLikeJSON(text string) bool {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return false
	}
	return strings.ContainsRune("{[\"", rune(trimmed[0]))
}
