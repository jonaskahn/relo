// Thinking metadata: the effort ladder a client catalog advertises.
package codingclients

import (
	"strings"

	"github.com/jonaskahn/relo/internal/inference"
)

// ompThinkingEfforts are the effort names Oh My Pi accepts in a
// `thinking.efforts` list. One name outside this set fails the whole
// models.yml, which takes every Relo route down with it.
var ompThinkingEfforts = map[string]bool{
	"minimal": true,
	"low":     true,
	"medium":  true,
	"high":    true,
	"xhigh":   true,
	"max":     true,
}

// piThinkingLevels are the level names Pi accepts in a `thinkingLevelMap`,
// least to most intensive.
var piThinkingLevels = []string{"minimal", "low", "medium", "high", "xhigh"}

// clientEfforts is the ladder one model offers a client, least intensive
// first. A model that stated no ladder offers the shared list. The names are
// the ones the client may use as effort values, so a name the client cannot
// take is dropped rather than written: one unknown name fails the whole
// client catalog and takes every Relo route with it.
func clientEfforts(model ModelRef, accepted map[string]bool) []string {
	stated := modelEfforts(model)
	kept := make([]string, 0, len(stated))
	seen := make(map[string]bool, len(stated))
	for _, effort := range inference.OrderEfforts(stated) {
		name, known := inference.NormalizeEffort(effort)
		if !known || !accepted[name] || seen[name] {
			continue
		}
		seen[name] = true
		kept = append(kept, name)
	}
	return kept
}

// modelReasons reports whether one model offers thinking at all. A model
// whose capability the catalog does not know says nothing, so a client
// config omits the field rather than inventing it.
func modelReasons(model ModelRef) bool {
	return model.Reasoning != nil && *model.Reasoning
}

// modelHasToggle reports whether one model can turn thinking off without an
// effort name.
func modelHasToggle(model ModelRef) bool {
	return model.ReasoningToggle != nil && *model.ReasoningToggle
}

// ompThinkingFields renders the thinking metadata one Oh My Pi model entry
// carries. A model that does not reason carries none at all, so the client
// offers no thinking levels for it. A model that reasons but stated no ladder
// falls back to the client's own default rather than an empty list, which the
// client's schema refuses.
func ompThinkingFields(model ModelRef) string {
	if !modelReasons(model) {
		return ""
	}
	var body strings.Builder
	body.WriteString("      reasoning: true\n")
	efforts := clientEfforts(model, ompThinkingEfforts)
	if len(efforts) == 0 {
		return body.String()
	}
	body.WriteString("      thinking:\n")
	body.WriteString("        mode: effort\n")
	body.WriteString("        efforts: [" + strings.Join(efforts, ", ") + "]\n")
	return body.String()
}

// piThinkingFields renders the thinking metadata one Pi model entry carries.
// Pi names the levels it offers in a map, where a level the model does not
// take is null and disappears from the picker.
func piThinkingFields(model ModelRef) []string {
	if !modelReasons(model) {
		return nil
	}
	fields := []string{`"reasoning":true`}
	mapFields := piLevelFields(model)
	if len(mapFields) == 0 {
		return fields
	}
	return append(fields, `"thinkingLevelMap":{`+strings.Join(mapFields, ",")+"}")
}

// piLevelFields renders the level map one Pi model entry carries. A level the
// model takes names the effort sent to reach it; a level it does not take is
// null, which is how Pi hides it. The off level is named only for a model
// that can turn thinking off without an effort name, so the default off path
// of every other model stays a request without an effort.
func piLevelFields(model ModelRef) []string {
	efforts := clientEfforts(model, ompThinkingEfforts)
	fields := make([]string, 0, len(piThinkingLevels)+1)
	if modelHasToggle(model) {
		fields = append(fields, `"off":"none"`)
	}
	for _, level := range piThinkingLevels {
		value := "null"
		for _, effort := range efforts {
			if effort == level {
				value = jsonString(effort)
				break
			}
		}
		fields = append(fields, jsonString(level)+":"+value)
	}
	return fields
}
