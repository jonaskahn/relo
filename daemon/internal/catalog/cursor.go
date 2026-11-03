// Cursor configuration selection: resolving one request to one tuple.
package catalog

import (
	"encoding/json"
	"strings"

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
	switch strings.ToLower(strings.TrimSpace(effort)) {
	case CursorSelectOff:
		return CursorSelectOff, true
	case CursorSelectToggle:
		return CursorSelectToggle, true
	case CursorSelectDefault:
		return CursorSelectDefault, true
	}
	selection, err := inference.ParseEffort(effort)
	if err != nil {
		return "", false
	}
	switch selection.Kind {
	case inference.EffortOff:
		return CursorSelectOff, true
	case inference.EffortOn:
		return CursorSelectToggle, true
	case inference.EffortAuto, inference.EffortDefault:
		return CursorSelectDefault, true
	default:
		return selection.Level, true
	}
}

// SelectCursorVariant resolves one requested effort to one account's exact
// Cursor configuration. An empty effort means the group's default member.
// It reports false when the account serves the group but not the requested
// selection, which is an unavailable selection rather than a missing model.
func SelectCursorVariant(configs []CursorVariant, groupID, effort string) (CursorVariant, bool) {
	selection, err := inference.ParseEffort(effort)
	if err != nil {
		return CursorVariant{}, false
	}
	if selection.Kind == inference.EffortAuto || selection.Kind == inference.EffortDefault {
		return defaultCursorVariant(configs, groupID)
	}
	if selection.Kind == inference.EffortOff {
		return cursorOffVariant(configs, groupID)
	}
	if selection.Kind == inference.EffortOn {
		return cursorOnVariant(configs, groupID)
	}
	if member, found := cursorSelection(configs, groupID, selection.Level); found {
		return member, true
	}
	if member, found := cursorCeiling(configs, groupID, selection.Level); found {
		return member, true
	}
	// A group whose members are all defaults states no reasoning choice at
	// all, so any requested effort resolves to that one member instead of
	// reporting a selection the account does not have.
	if !groupStatesReasoning(configs, groupID) {
		return defaultCursorVariant(configs, groupID)
	}
	if member, found := cursorHighest(configs, groupID); found {
		return member, true
	}
	return defaultCursorVariant(configs, groupID)
}

func cursorOffVariant(configs []CursorVariant, groupID string) (CursorVariant, bool) {
	if member, found := cursorSelection(configs, groupID, CursorSelectOff); found {
		return member, true
	}
	return defaultCursorVariant(configs, groupID)
}

func cursorOnVariant(configs []CursorVariant, groupID string) (CursorVariant, bool) {
	if member, found := cursorSelection(configs, groupID, CursorSelectToggle); found {
		return member, true
	}
	return defaultCursorVariant(configs, groupID)
}

func cursorSelection(configs []CursorVariant, groupID, selection string) (CursorVariant, bool) {
	for _, config := range configs {
		if config.GroupID == groupID && config.Selection == selection {
			return config, true
		}
	}
	return CursorVariant{}, false
}

func defaultCursorVariant(configs []CursorVariant, groupID string) (CursorVariant, bool) {
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

func cursorCeiling(configs []CursorVariant, groupID, level string) (CursorVariant, bool) {
	rank, ranked := cursorRank(level)
	if !ranked {
		return CursorVariant{}, false
	}
	bestRank := -1
	best := CursorVariant{}
	for _, config := range configs {
		if config.GroupID != groupID || config.Selection == CursorSelectOff || config.Selection == CursorSelectToggle {
			continue
		}
		valueRank, ok := cursorRank(config.Selection)
		if !ok || valueRank < rank {
			continue
		}
		if bestRank < 0 || valueRank < bestRank {
			bestRank, best = valueRank, config
		}
	}
	return best, bestRank >= 0
}

func cursorHighest(configs []CursorVariant, groupID string) (CursorVariant, bool) {
	bestRank := -1
	best := CursorVariant{}
	for _, config := range configs {
		if config.GroupID != groupID {
			continue
		}
		rank, ok := cursorRank(config.Selection)
		if !ok {
			continue
		}
		if rank > bestRank {
			bestRank, best = rank, config
		}
	}
	return best, bestRank >= 0
}

var cursorEffortOrder = []string{"none", "minimal", "low", "medium", "high", "xhigh", "max"}

func cursorRank(value string) (int, bool) {
	level, ok := inference.NormalizeEffort(value)
	if !ok {
		return 0, false
	}
	for rank, name := range cursorEffortOrder {
		if name == level {
			return rank, true
		}
	}
	return 0, false
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
