// Native body reuse: a routed Messages request keeps its wire shape.
package anthropic

import (
	"bytes"
	"encoding/json"
)

// rebaseNativeBody retargets a client Messages body at the selected model
// and account instead of rebuilding it. The caller's system blocks, cache
// controls, tools, and native controls travel byte for byte; only the model,
// the account attribution, and a thinking display this API refuses reflect
// the attempt Relo actually runs. A body that is not a Messages object falls
// back to the canonical encoder.
//
// The body is edited as raw JSON members rather than decoded into `any`: a
// round trip through Go values escapes HTML, coerces numbers through a float,
// and reorders keys, and any of those rewrites the client's own prompt.
type nativeOverride struct {
	MaxTokens       int
	Thinking        *thinkingDef
	EnforceThinking bool
}

func rebaseNativeBody(raw []byte, model string, metadata *metadataDef, override nativeOverride) ([]byte, bool) {
	members, ok := topLevelMembers(raw)
	if !ok || !hasMessages(members) {
		return nil, false
	}
	rebuilt, written := rewriteNativeMembers(members, model, metadata, override)
	if !written {
		rebuilt = append(rebuilt, member{key: "model", value: modelValue(model)})
	}
	if override.EnforceThinking && override.Thinking != nil && !hasMember(rebuilt, "thinking") {
		rebuilt = append(rebuilt, member{key: "thinking", value: thinkingValue(override.Thinking)})
	}
	encoded, err := encodeMembers(rebuilt)
	if err != nil {
		return nil, false
	}
	return encoded, true
}

func rewriteNativeMembers(members []member, model string, metadata *metadataDef, override nativeOverride) ([]member, bool) {
	target := modelValue(model)
	rebuilt := make([]member, 0, len(members)+1)
	written := false
	for _, current := range members {
		switch current.key {
		case "model":
			current.value = target
			written = true
		case "max_tokens":
			if override.MaxTokens > 0 {
				current.value = intValue(override.MaxTokens)
			}
		case "metadata":
			if metadata == nil {
				continue
			}
			current.value = metadataValue(metadata)
		case "thinking":
			if override.EnforceThinking {
				if override.Thinking == nil {
					continue
				}
				current.value = thinkingValue(override.Thinking)
			} else {
				current.value = normalizeThinkingDisplay(current.value)
			}
		}
		rebuilt = append(rebuilt, current)
	}
	return rebuilt, written
}

func hasMember(members []member, name string) bool {
	for _, current := range members {
		if current.key == name {
			return true
		}
	}
	return false
}

func intValue(value int) json.RawMessage {
	encoded, _ := json.Marshal(value)
	return encoded
}

func thinkingValue(thinking *thinkingDef) json.RawMessage {
	encoded, _ := json.Marshal(thinking)
	return encoded
}

type member struct {
	key   string
	value json.RawMessage
}

// topLevelMembers reads one JSON object's members in the order it wrote them,
// keeping each value as the exact bytes it arrived as.
func topLevelMembers(raw []byte) ([]member, bool) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	opening, err := decoder.Token()
	if err != nil {
		return nil, false
	}
	if delimiter, ok := opening.(json.Delim); !ok || delimiter != '{' {
		return nil, false
	}
	members := make([]member, 0, 16)
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return nil, false
		}
		name, ok := key.(string)
		if !ok {
			return nil, false
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, false
		}
		members = append(members, member{key: name, value: value})
	}
	return members, true
}

func hasMessages(members []member) bool {
	for _, current := range members {
		if current.key != "messages" {
			continue
		}
		var messages []json.RawMessage
		return json.Unmarshal(current.value, &messages) == nil && len(messages) > 0
	}
	return false
}

func encodeMembers(members []member) ([]byte, error) {
	var buffer bytes.Buffer
	buffer.WriteByte('{')
	for index, current := range members {
		if index > 0 {
			buffer.WriteByte(',')
		}
		name, err := json.Marshal(current.key)
		if err != nil {
			return nil, err
		}
		buffer.Write(name)
		buffer.WriteByte(':')
		buffer.Write(current.value)
	}
	buffer.WriteByte('}')
	return buffer.Bytes(), nil
}

func modelValue(model string) json.RawMessage {
	encoded, _ := json.Marshal(model)
	return encoded
}

func metadataValue(metadata *metadataDef) json.RawMessage {
	encoded, _ := json.Marshal(map[string]string{"user_id": metadata.UserID})
	return encoded
}

// normalizeThinkingDisplay rewrites a thinking object so its display field
// holds a value the public Messages API accepts. The API rejects "updates",
// a value Claude Code speaks but only its own surface understands, and
// answers 400 for it. An unrecognized value becomes "omitted", the nearest
// public equivalent: the model still thinks, and the stream stays one this
// relay already knows how to read. A display this wire accepts is left as the
// client wrote it, and so is a thinking field that is not an object.
func normalizeThinkingDisplay(raw json.RawMessage) json.RawMessage {
	fields, ok := topLevelMembers(raw)
	if !ok {
		return raw
	}
	changed := false
	for index := range fields {
		if fields[index].key != "display" {
			continue
		}
		var display string
		if err := json.Unmarshal(fields[index].value, &display); err != nil {
			continue
		}
		switch display {
		case "summarized", "omitted":
			continue
		}
		fields[index].value = json.RawMessage(`"omitted"`)
		changed = true
	}
	if !changed {
		return raw
	}
	encoded, err := encodeMembers(fields)
	if err != nil {
		return raw
	}
	return encoded
}
