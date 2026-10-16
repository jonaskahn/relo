// Antigravity roster knowledge: ids the vendor retired and the model
// families a connection serves.
package catalog

import (
	"encoding/json"
	"strings"

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

// ClaudeAntigravity reports whether one Antigravity id names a Claude model.
// Claude passes through under its own id with no thinking, so it never
// resolves an effort onto a wire SKU.
func ClaudeAntigravity(id string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(id)), "claude")
}

// RetiredAntigravity reports whether one Antigravity id is retired.
func RetiredAntigravity(id string) bool {
	return RetiredAntigravityIDs[id]
}

// The thinking bands the vendor names. Every logical model collapses onto one
// band per request, and a client chooses the band by asking for an effort.
const (
	AntigravityBandLow    = "low"
	AntigravityBandMedium = "medium"
	AntigravityBandHigh   = "high"
)

// AntigravityBands are the bands in the order a picker offers them, lowest
// first, so a model's thinking levels read in the order a client shows them.
func AntigravityBands() []string {
	return []string{AntigravityBandLow, AntigravityBandMedium, AntigravityBandHigh}
}

// AntigravityBand maps one canonical thinking effort onto the vendor's band.
// The canonical ladder is longer than the vendor's three tiers: the steps
// below low share the low band and the steps above high share the high band.
// An effort outside the ladder takes the middle band, which the vendor lists
// for every model it collapses.
func AntigravityBand(effort string) string {
	switch effort {
	case "none", "minimal", "low":
		return AntigravityBandLow
	case "high", "xhigh", "max":
		return AntigravityBandHigh
	default:
		return AntigravityBandMedium
	}
}

// antigravityEffortSuffixes are the thinking-effort labels the vendor appends
// to a Gemini model's display name.
var antigravityEffortSuffixes = []string{" (Low)", " (Medium)", " (High)", " (Extra Low)"}

// CleanAntigravityGeminiName drops the thinking-effort suffix the vendor
// appends to a Gemini display name. Relo publishes the model once, its effort
// chosen per request, so the suffix is not part of the name. A model of another
// family keeps its name, because the vendor spells those parentheses the same
// way without meaning an effort.
func CleanAntigravityGeminiName(modelID, name string) string {
	cleaned := strings.TrimSpace(name)
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(modelID)), "gemini") {
		return cleaned
	}
	for _, suffix := range antigravityEffortSuffixes {
		if strings.HasSuffix(cleaned, suffix) {
			return strings.TrimSpace(strings.TrimSuffix(cleaned, suffix))
		}
	}
	return cleaned
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
	// ThinkingBudget is the budget the vendor publishes for this SKU. Nil
	// leaves the request open-ended rather than guessing at a budget the
	// endpoint may not serve.
	ThinkingBudget *int
}

type antigravityWireConfig struct {
	WireModel      string `json:"wire_model"`
	ModelEnum      string `json:"model_enum,omitempty"`
	MaxOutput      int    `json:"max_output,omitempty"`
	ThinkingBudget *int   `json:"thinking_budget,omitempty"`
}

// EncodeAntigravityConfig stores one Antigravity tuple as a request configuration.
func EncodeAntigravityConfig(variant AntigravityVariant) ModelRequestConfig {
	raw, _ := json.Marshal(antigravityWireConfig{
		WireModel: variant.WireModel, ModelEnum: variant.ModelEnum, MaxOutput: variant.MaxOutput,
		ThinkingBudget: variant.ThinkingBudget,
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
		ThinkingBudget: wire.ThinkingBudget,
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
	band := AntigravityBand(normalized)
	for _, config := range configs {
		if config.LogicalID == logicalID && config.Effort == band {
			return config, true
		}
	}
	for _, config := range configs {
		if config.LogicalID == logicalID && config.Selection == band {
			return config, true
		}
	}
	return AntigravityVariant{}, false
}
