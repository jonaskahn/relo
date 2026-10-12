// Cursor configuration selection: resolving one request to one tuple.
package catalog

import (
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
	return CursorVariant{}, false
}
