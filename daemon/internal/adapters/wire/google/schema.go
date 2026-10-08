// Gemini schema sanitizing for strict tool definitions.
package google

import (
	"encoding/json"
	"strings"
)

var allowedTypes = map[string]bool{
	"string":  true,
	"integer": true,
	"number":  true,
	"boolean": true,
	"array":   true,
	"object":  true,
}

// SanitizeSchema returns a tool schema Google accepts. Each object keeps only
// the fields Cloud Code Assist can parse: one type, nullable, description,
// format, a string enum, properties, one items schema, and required names that
// exist. A type union keeps its first allowed type, and anyOf becomes a
// nullable schema, a merged enum, or nothing. A schema that is not a JSON
// object is returned as it arrived.
func SanitizeSchema(schema json.RawMessage) json.RawMessage {
	if len(schema) == 0 {
		return nil
	}
	var decoded any
	if err := json.Unmarshal(schema, &decoded); err != nil {
		return schema
	}
	object, ok := decoded.(map[string]any)
	if !ok {
		return schema
	}
	cleaned, err := json.Marshal(sanitizeObject(object, false))
	if err != nil {
		return schema
	}
	return cleaned
}

func sanitizeObject(node map[string]any, preserveNull bool) map[string]any {
	cleaned := map[string]any{}
	applyType(node["type"], cleaned, preserveNull)
	if nullable, ok := node["nullable"].(bool); ok {
		cleaned["nullable"] = nullable
	}
	if description, ok := node["description"].(string); ok {
		cleaned["description"] = description
	}
	if format, ok := node["format"].(string); ok {
		cleaned["format"] = format
	}
	applyEnum(node, cleaned)
	applyProperties(node, cleaned)
	if items, ok := node["items"].(map[string]any); ok {
		cleaned["items"] = sanitizeObject(items, false)
	}
	if _, found := node["anyOf"]; found {
		for key, value := range normalizeAnyOf(node["anyOf"]) {
			cleaned[key] = value
		}
	}
	return cleaned
}

func applyEnum(node, cleaned map[string]any) {
	if _, found := node["enum"]; found {
		if enum := stringEnum(node["enum"]); enum != nil {
			cleaned["enum"] = enum
		}
	} else if constant, ok := node["const"].(string); ok {
		cleaned["enum"] = []any{constant}
	}
}

func applyProperties(node, cleaned map[string]any) {
	if properties, ok := sanitizeProperties(node["properties"]); ok {
		cleaned["properties"] = properties
		if required := keptRequired(node["required"], properties); len(required) > 0 {
			cleaned["required"] = required
		}
	}
}

func sanitizeProperties(value any) (map[string]any, bool) {
	properties, ok := value.(map[string]any)
	if !ok {
		return nil, false
	}
	cleaned := make(map[string]any, len(properties))
	for name, schema := range properties {
		cleaned[name] = sanitizeValue(schema, false)
	}
	return cleaned, true
}

func sanitizeValue(value any, preserveNull bool) map[string]any {
	object, ok := value.(map[string]any)
	if !ok {
		return map[string]any{}
	}
	return sanitizeObject(object, preserveNull)
}

func applyType(value any, cleaned map[string]any, preserveNull bool) {
	candidates, ok := value.([]any)
	if !ok {
		if value == nil {
			return
		}
		candidates = []any{value}
	}
	chosen, sawNull := pickTypeKind(candidates)
	if chosen != "" {
		cleaned["type"] = chosen
	}
	if !sawNull {
		return
	}
	if chosen != "" || !preserveNull {
		cleaned["nullable"] = true
		return
	}
	cleaned["type"] = "null"
}

func pickTypeKind(candidates []any) (string, bool) {
	chosen := ""
	sawNull := false
	for _, candidate := range candidates {
		text, ok := candidate.(string)
		if !ok {
			continue
		}
		kind := strings.ToLower(text)
		switch {
		case kind == "null":
			sawNull = true
		case chosen == "" && allowedTypes[kind]:
			chosen = kind
		}
	}
	return chosen, sawNull
}

func stringEnum(value any) []any {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	seen := map[string]bool{}
	var values []any
	for _, item := range items {
		text, ok := item.(string)
		if !ok || seen[text] {
			continue
		}
		seen[text] = true
		values = append(values, text)
	}
	if len(values) == 0 {
		return nil
	}
	return values
}

func keptRequired(value any, properties map[string]any) []any {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	seen := map[string]bool{}
	var required []any
	for _, item := range items {
		name, ok := item.(string)
		if !ok || seen[name] {
			continue
		}
		if _, found := properties[name]; !found {
			continue
		}
		seen[name] = true
		required = append(required, name)
	}
	return required
}

func normalizeAnyOf(value any) map[string]any {
	branches, ok := value.([]any)
	if !ok || len(branches) == 0 {
		return map[string]any{}
	}
	schemas := make([]map[string]any, 0, len(branches))
	for _, branch := range branches {
		schemas = append(schemas, sanitizeValue(branch, true))
	}
	var nonNull, nulls []map[string]any
	for _, schema := range schemas {
		if schema["type"] == "null" {
			nulls = append(nulls, schema)
			continue
		}
		nonNull = append(nonNull, schema)
	}
	if len(nonNull) == 1 && len(nulls) > 0 && everyTypeNull(nulls) {
		chosen := nonNull[0]
		chosen["nullable"] = true
		return chosen
	}
	if merged, ok := mergeEnumBranches(schemas); ok {
		return merged
	}
	return map[string]any{}
}

func everyTypeNull(schemas []map[string]any) bool {
	for _, schema := range schemas {
		if len(schema) != 1 || schema["type"] != "null" {
			return false
		}
	}
	return true
}

func mergeEnumBranches(schemas []map[string]any) (map[string]any, bool) {
	if len(schemas) == 0 {
		return nil, false
	}
	kind, hasType := schemas[0]["type"].(string)
	if hasType && kind == "null" {
		return nil, false
	}
	var values []any
	for _, schema := range schemas {
		schemaType, schemaHasType := schema["type"].(string)
		if schemaHasType != hasType || (hasType && schemaType != kind) || !enumOnly(schema, hasType) {
			return nil, false
		}
		items, _ := schema["enum"].([]any)
		values = append(values, items...)
	}
	enum := stringEnum(values)
	if enum == nil {
		return nil, false
	}
	merged := map[string]any{"enum": enum}
	if hasType {
		merged["type"] = kind
	}
	return merged, true
}

func enumOnly(schema map[string]any, hasType bool) bool {
	if _, ok := schema["enum"].([]any); !ok {
		return false
	}
	for key := range schema {
		if key == "enum" || (hasType && key == "type") {
			continue
		}
		return false
	}
	return true
}
