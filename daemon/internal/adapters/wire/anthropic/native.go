// Native body reuse: a routed Messages request keeps its wire shape.
package anthropic

import (
	"bytes"
	"encoding/json"
)

// rebaseNativeBody retargets a client Messages body at the selected model
// and account instead of rebuilding it. The caller's system blocks, cache
// controls, tools, and native controls travel byte for byte; only the model
// and the account attribution reflect the attempt Relo actually runs. A body
// that is not a Messages object falls back to the canonical encoder.
//
// The body is edited as raw JSON members rather than decoded into `any`: a
// round trip through Go values escapes HTML, coerces numbers through a float,
// and reorders keys, and any of those rewrites the client's own prompt.
func rebaseNativeBody(raw []byte, model string, metadata *metadataDef) ([]byte, bool) {
	members, ok := topLevelMembers(raw)
	if !ok || !hasMessages(members) {
		return nil, false
	}
	target := modelValue(model)
	rebuilt := make([]member, 0, len(members)+1)
	written := false
	for _, current := range members {
		switch current.key {
		case "model":
			current.value = target
			written = true
		case "metadata":
			if metadata == nil {
				continue
			}
			current.value = metadataValue(metadata)
		}
		rebuilt = append(rebuilt, current)
	}
	if !written {
		rebuilt = append(rebuilt, member{key: "model", value: target})
	}
	encoded, err := encodeMembers(rebuilt)
	if err != nil {
		return nil, false
	}
	return encoded, true
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
