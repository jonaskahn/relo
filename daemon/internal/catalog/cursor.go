// Cursor configuration selection: resolving one request to one tuple.
package catalog

import (
	"encoding/json"

	"github.com/jonaskahn/relo/internal/inference"
)

// RequestEffort reads the effort one canonical request asks for, reporting
// whether the request stated one at all.
func RequestEffort(reasoning *inference.ReasoningConfig) (string, bool) {
	if reasoning == nil {
		return "", false
	}
	return reasoning.Effort, reasoning.Effort != ""
}

// NormalizeCursorEffort maps a requested effort onto a Cursor selection:
// off, toggle, default, or a canonical effort. It reports false for an
// unknown value.
func NormalizeCursorEffort(effort string) (string, bool) {
	switch effort {
	case CursorSelectOff, CursorSelectToggle, CursorSelectDefault:
		return effort, true
	case "none":
		return CursorSelectOff, true
	case "extra-high":
		return inference.NormalizeEffort("xhigh")
	default:
		return inference.NormalizeEffort(effort)
	}
}

// SelectCursorVariant resolves one requested effort to one account's exact
// Cursor configuration. An empty effort means the group's default member.
// It reports false when the account serves the group but not the requested
// selection, which is an unavailable selection rather than a missing model.
func SelectCursorVariant(configs []CursorVariant, groupID, effort string) (CursorVariant, bool) {
	if effort == "" {
		for _, config := range configs {
			if config.GroupID == groupID && config.Default {
				return config, true
			}
		}
		for _, config := range configs {
			if config.GroupID == groupID {
				return config, true
			}
		}
		return CursorVariant{}, false
	}
	normalized, ok := NormalizeCursorEffort(effort)
	if !ok {
		return CursorVariant{}, false
	}
	for _, config := range configs {
		if config.GroupID == groupID && config.Selection == normalized {
			return config, true
		}
	}
	// A group whose members are all defaults states no reasoning choice at
	// all, so any requested effort resolves to that one member instead of
	// reporting a selection the account does not have.
	if !groupStatesReasoning(configs, groupID) {
		return SelectCursorVariant(configs, groupID, "")
	}
	return CursorVariant{}, false
}

// groupStatesReasoning reports whether one group holds any member that
// selects a reasoning choice: an effort, thinking off, or thinking on.
func groupStatesReasoning(configs []CursorVariant, groupID string) bool {
	for _, config := range configs {
		if config.GroupID != groupID {
			continue
		}
		if config.Selection != CursorSelectDefault && config.Selection != "" {
			return true
		}
	}
	return false
}

type cursorWireConfig struct {
	ModelID   string        `json:"model_id"`
	DetailsID string        `json:"details_id"`
	MaxMode   bool          `json:"max_mode"`
	Params    []CursorParam `json:"params"`
}

// EncodeCursorConfig stores one Cursor tuple as a request configuration. The
// wire payload keeps the exact triple the account's discovery published.
func EncodeCursorConfig(variant CursorVariant) ModelRequestConfig {
	raw, _ := json.Marshal(cursorWireConfig{
		ModelID: variant.ModelID, DetailsID: variant.DetailsID,
		MaxMode: variant.MaxMode, Params: variant.Params,
	})
	return ModelRequestConfig{
		ModelID: variant.GroupID, Selection: variant.Selection, Effort: variant.Effort,
		Default: variant.Default, APIFormat: FormatCursorAgent, WireConfig: string(raw),
		Context: variant.Context, Images: variant.Images, Reasoning: variant.Reasoning,
	}
}

// DecodeCursorConfig reads one stored Cursor tuple back. It reports false
// for another codec's configuration, which selection skips rather than
// sending.
func DecodeCursorConfig(config ModelRequestConfig) (CursorVariant, bool) {
	if config.APIFormat != FormatCursorAgent {
		return CursorVariant{}, false
	}
	var wire cursorWireConfig
	if err := json.Unmarshal([]byte(config.WireConfig), &wire); err != nil {
		return CursorVariant{}, false
	}
	if wire.ModelID == "" || wire.DetailsID == "" {
		return CursorVariant{}, false
	}
	return CursorVariant{
		GroupID: config.ModelID, Selection: config.Selection, Effort: config.Effort,
		Default: config.Default, ModelID: wire.ModelID, DetailsID: wire.DetailsID,
		MaxMode: wire.MaxMode, Params: wire.Params,
		Context: config.Context, Images: config.Images, Reasoning: config.Reasoning,
	}, true
}
