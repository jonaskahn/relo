// DeepSeek Harness default model: the patch item that names the model a fresh
// agent starts on.
//
// The headless profile has no model flag and reads this selection, which the
// harness's base bundle points at DeepSeek. Without an item of its own Relo's
// provider would be configured and never reached.
package codingclients

import (
	"strings"
)

// dshDefaultItem is the Cordis list item holding the default model selection.
const dshDefaultItem = "agent-default-model"

// MergeDSHHeadless returns the headless profile's patch with Relo's provider
// in it and the default model pointing at Relo, replacing any selection the
// patch already held.
func MergeDSHHeadless(existing, baseURL string, models []ModelRef) (string, error) {
	return mergeDSH(existing, baseURL, models, true)
}

// DSHHeadlessPointsAt reports whether the headless patch holds the provider
// and the default model Relo would write now.
func DSHHeadlessPointsAt(existing, baseURL string, models []ModelRef) bool {
	return DSHPointsAt(existing, baseURL, models) && dshDefaultPointsAt(existing, models)
}

// dshDefaultText renders the default-model item: the first model Relo lists,
// which is always one the provider entry carries. No model, no item.
func dshDefaultText(models []ModelRef) string {
	ordered := orderedModels(models)
	if len(ordered) == 0 {
		return ""
	}
	return "id: " + dshDefaultItem + "\n" +
		"config:\n" +
		"  provider: " + reloProviderID + "\n" +
		"  model: " + yamlQuote(ordered[0].ID) + "\n"
}

// appendDSHDefault adds Relo's default-model item to the end of the patch.
func appendDSHDefault(lines []string, models []ModelRef) []string {
	text := dshDefaultText(models)
	if text == "" {
		return lines
	}
	_, indent, _, _, _ := dshList(lines)
	return appendBlock(lines, strings.Join(dshRenderedItem(text, indent), "\n"))
}

// cutDSHDefault removes the default-model item from the patch. With onlyRelo
// set, a selection the operator pointed at another provider stays.
func cutDSHDefault(lines []string, onlyRelo bool) ([]string, error) {
	start, indent, end, found, err := dshList(lines)
	if err != nil || !found {
		return lines, err
	}
	kept := append([]string{}, lines[:start]...)
	for index := start; index < end; {
		line := lines[index]
		if indentOf(line) != indent || !strings.HasPrefix(strings.TrimSpace(line), "-") {
			kept = append(kept, line)
			index++
			continue
		}
		stop := dshItemEnd(lines, index, end, indent)
		if !dshIsDefaultItem(lines[index:stop], onlyRelo) {
			kept = append(kept, lines[index:stop]...)
		}
		index = stop
	}
	return append(kept, lines[end:]...), nil
}

func dshIsDefaultItem(item []string, onlyRelo bool) bool {
	named, relo := false, false
	for _, line := range item {
		if value, ok := yamlScalar(line, "id"); ok && value == dshDefaultItem {
			named = true
		}
		if value, ok := yamlScalar(line, "provider"); ok && value == reloProviderID {
			relo = true
		}
	}
	return named && (!onlyRelo || relo)
}

// dshDefaultPointsAt reports whether the patch's default-model item names
// Relo and the model Relo would pick now.
func dshDefaultPointsAt(existing string, models []ModelRef) bool {
	ordered := orderedModels(models)
	if len(ordered) == 0 {
		return true
	}
	document, err := dshDocument(existing)
	if err != nil {
		return false
	}
	for _, item := range document {
		row, _ := item.(map[string]any)
		if row == nil || row["id"] != dshDefaultItem {
			continue
		}
		config, _ := row[dshConfigPath].(map[string]any)
		return config["provider"] == reloProviderID && config["model"] == ordered[0].ID
	}
	return false
}
