// Native body reuse: a routed Messages request keeps its wire shape.
package anthropic

import (
	"encoding/json"
)

// rebaseNativeBody retargets a client Messages body at the selected model
// and account instead of rebuilding it. The caller's system blocks, cache
// controls, tools, and native controls travel unchanged; only the model and
// the account attribution reflect the attempt Relo actually runs. A body
// that is not a Messages object falls back to the canonical encoder.
func rebaseNativeBody(raw []byte, model string, metadata *metadataDef) ([]byte, bool) {
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, false
	}
	messages, found := body["messages"].([]any)
	if !found || len(messages) == 0 {
		return nil, false
	}
	body["model"] = model
	if metadata != nil {
		body["metadata"] = map[string]any{"user_id": metadata.UserID}
	} else {
		delete(body, "metadata")
	}
	rebuilt, err := json.Marshal(body)
	if err != nil {
		return nil, false
	}
	return rebuilt, true
}
