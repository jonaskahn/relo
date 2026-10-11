// Cursor configuration selection: resolving one request to one tuple.
package cursor

import (
	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/internal/inference"
)

// SelectConfig resolves one requested effort to one account's exact Cursor
// configuration. An empty effort means the group's default. It reports false
// when the account serves the group but not the requested selection, which
// is an unavailable selection rather than a missing model.
func SelectConfig(configs []catalog.CursorVariant, groupID, effort string) (catalog.CursorVariant, bool) {
	want := effort
	if want == "" {
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
		return catalog.CursorVariant{}, false
	}
	normalized, ok := NormalizeSelection(want)
	if !ok {
		return catalog.CursorVariant{}, false
	}
	for _, config := range configs {
		if config.GroupID == groupID && config.Selection == normalized {
			return config, true
		}
	}
	return catalog.CursorVariant{}, false
}

// NormalizeSelection maps a requested effort onto a Cursor selection: off,
// toggle, or a canonical effort. It reports false for an unknown value.
func NormalizeSelection(effort string) (string, bool) {
	switch effort {
	case catalog.CursorSelectOff, catalog.CursorSelectToggle, catalog.CursorSelectDefault:
		return effort, true
	default:
		return normalizeEffort(effort), normalizeEffort(effort) != ""
	}
}

// RequestEffort reads the effort one canonical request asks for, reporting
// whether the request stated one at all.
func RequestEffort(reasoning *inference.ReasoningConfig) (string, bool) {
	if reasoning == nil {
		return "", false
	}
	return reasoning.Effort, reasoning.Effort != ""
}
