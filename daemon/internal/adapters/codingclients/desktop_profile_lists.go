// Claude Desktop profile comparison: what the gateway profile Relo manages
// holds right now.
package codingclients

import (
	"encoding/json"
	"strings"
)

// DesktopProfileLists reports whether a profile already carries the gateway
// address, static model discovery, and model rows Relo publishes now. It
// compares parsed values, so JSON formatting and unrelated settings do not
// count as a difference. Credentials are deliberately ignored.
func DesktopProfileLists(content, baseURL string, models []ModelRef) bool {
	var profile struct {
		BaseURL   string         `json:"inferenceGatewayBaseUrl"`
		Discovery *bool          `json:"modelDiscoveryEnabled"`
		Models    []desktopModel `json:"inferenceModels"`
	}
	if err := json.Unmarshal([]byte(content), &profile); err != nil {
		return false
	}
	if strings.TrimSpace(baseURL) == "" || strings.TrimSpace(profile.BaseURL) == "" {
		return false
	}
	if strings.TrimSuffix(strings.TrimSpace(profile.BaseURL), "/") !=
		strings.TrimSuffix(strings.TrimSpace(baseURL), "/") {
		return false
	}
	if profile.Discovery == nil || *profile.Discovery {
		return false
	}
	expected := desktopModels(models)
	if len(profile.Models) != len(expected) {
		return false
	}
	for index := range expected {
		if profile.Models[index] != expected[index] {
			return false
		}
	}
	return true
}
