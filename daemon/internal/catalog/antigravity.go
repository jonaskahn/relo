// Antigravity roster knowledge: ids the vendor retired and the model
// families a connection serves.
package catalog

import (
	"encoding/json"

	"github.com/jonaskahn/relo/internal/inference"
)

// RetiredAntigravityIDs are the backend SKUs the vendor superseded but may
// still list: a roster never serves them, however they are spelled.
var RetiredAntigravityIDs = map[string]bool{
	"gemini-3.5-flash-extra-low": true,
	"gemini-3.5-flash-low":       true,
	"gemini-3.5-flash-mid":       true,
	"gemini-3.5-flash-high":      true,
	"gemini-3-flash-agent":       true,
}

// RetiredAntigravity reports whether one Antigravity id is retired.
func RetiredAntigravity(id string) bool {
	return RetiredAntigravityIDs[id]
}

// AntigravityVariant is one exact Antigravity request configuration: the wire
// SKU one effort band of one logical model sends.
type AntigravityVariant struct {
	LogicalID string
	Selection string
	Effort    string
	// Default marks the member an empty effort request resolves to.
	Default   bool
	WireModel string
	ModelEnum string
	MaxOutput int
	Context   int64
	Reasoning bool
}

type antigravityWireConfig struct {
	WireModel string `json:"wire_model"`
	ModelEnum string `json:"model_enum,omitempty"`
	MaxOutput int    `json:"max_output,omitempty"`
}

// EncodeAntigravityConfig stores one Antigravity tuple as a request configuration.
func EncodeAntigravityConfig(variant AntigravityVariant) ModelRequestConfig {
	raw, _ := json.Marshal(antigravityWireConfig{
		WireModel: variant.WireModel, ModelEnum: variant.ModelEnum, MaxOutput: variant.MaxOutput,
	})
	return ModelRequestConfig{
		ModelID: variant.LogicalID, Selection: variant.Selection, Effort: variant.Effort,
		Default: variant.Default, APIFormat: FormatCloudCode, WireConfig: string(raw),
		Context: variant.Context, Reasoning: variant.Reasoning,
	}
}

// DecodeAntigravityConfig reads one stored Antigravity tuple back.
func DecodeAntigravityConfig(config ModelRequestConfig) (AntigravityVariant, bool) {
	if config.APIFormat != FormatCloudCode {
		return AntigravityVariant{}, false
	}
	var wire antigravityWireConfig
	if err := json.Unmarshal([]byte(config.WireConfig), &wire); err != nil {
		return AntigravityVariant{}, false
	}
	if wire.WireModel == "" {
		return AntigravityVariant{}, false
	}
	return AntigravityVariant{
		LogicalID: config.ModelID, Selection: config.Selection, Effort: config.Effort,
		Default: config.Default, WireModel: wire.WireModel, ModelEnum: wire.ModelEnum,
		MaxOutput: wire.MaxOutput, Context: config.Context, Reasoning: config.Reasoning,
	}, true
}

// SelectAntigravityVariant resolves one requested effort to one account's exact
// Antigravity configuration.
func SelectAntigravityVariant(configs []AntigravityVariant, logicalID, effort string) (AntigravityVariant, bool) {
	if effort == "" {
		for _, config := range configs {
			if config.LogicalID == logicalID && config.Default {
				return config, true
			}
		}
		for _, config := range configs {
			if config.LogicalID == logicalID {
				return config, true
			}
		}
		return AntigravityVariant{}, false
	}
	normalized, ok := inference.NormalizeEffort(effort)
	if !ok {
		return AntigravityVariant{}, false
	}
	for _, config := range configs {
		if config.LogicalID == logicalID && config.Effort == normalized {
			return config, true
		}
	}
	for _, config := range configs {
		if config.LogicalID == logicalID && config.Selection == normalized {
			return config, true
		}
	}
	return AntigravityVariant{}, false
}
