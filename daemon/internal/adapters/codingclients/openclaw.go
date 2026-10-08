// OpenClaw config: merging Relo into its file and reading it back.
package codingclients

import (
	"fmt"
	"strconv"
	"strings"
)

func openClawKeyRef() string {
	return "${" + EnvKeyName(ClientOpenClaw) + "}"
}

// MergeOpenClaw returns an OpenClaw configuration with a relo provider under
// models.providers. An absent models.mode becomes merge, so OpenClaw keeps
// its own catalog beside Relo's. A mode the operator already set is left alone.
func MergeOpenClaw(existing, baseURL string, models []ModelRef) (string, error) {
	address, err := requireBase(baseURL)
	if err != nil {
		return "", fmt.Errorf("openclaw: %w", err)
	}
	merged := existing
	found, err := jsonHas(existing, []string{"models", "mode"})
	if err != nil {
		return "", fmt.Errorf("openclaw: %w", err)
	}
	if !found {
		merged, err = spliceKey(merged, []string{"models", "mode"}, `"merge"`)
		if err != nil {
			return "", fmt.Errorf("openclaw: %w", err)
		}
	}
	merged, err = spliceKey(merged, []string{"models", "providers", reloProviderID}, openClawProvider(address, models))
	if err != nil {
		return "", fmt.Errorf("openclaw: %w", err)
	}
	return ensureNewline(merged), nil
}

// StripOpenClaw removes the relo provider. A models.mode Relo may have added
// stays, because that key is shared with the rest of the catalog.
func StripOpenClaw(existing string) (string, error) {
	if strings.TrimSpace(existing) == "" {
		return "", nil
	}
	stripped, err := deleteKey(existing, []string{"models", "providers", reloProviderID})
	if err != nil {
		return "", fmt.Errorf("openclaw: %w", err)
	}
	if emptyDocument(stripped) {
		return "", nil
	}
	return ensureNewline(stripped), nil
}

func openClawPointsAt(existing, baseURL string, models []ModelRef) bool {
	address, err := requireBase(baseURL)
	if err != nil {
		return false
	}
	raw, ok, err := jsonRaw(existing, []string{"models", "providers", reloProviderID})
	if err != nil || !ok {
		return false
	}
	return jsonEqual(raw, openClawProvider(address, models))
}

func openClawHasRelo(existing string) (bool, error) {
	return jsonHas(existing, []string{"models", "providers", reloProviderID})
}

func openClawFragment(baseURL string, models []ModelRef) (string, error) {
	address, err := requireBase(baseURL)
	if err != nil {
		return "", fmt.Errorf("openclaw: %w", err)
	}
	return "{\n  \"models\": {\"providers\": {\"relo\": " + openClawProvider(address, models) + "}}\n}", nil
}

func openClawProvider(address string, models []ModelRef) string {
	return `{"baseUrl":` + jsonString(address) +
		`,"apiKey":` + jsonString(openClawKeyRef()) +
		`,"api":"openai-completions","models":` + openClawModels(models) + `}`
}

func openClawModels(models []ModelRef) string {
	ordered := orderedModels(models)
	if len(ordered) == 0 {
		return "[]"
	}
	parts := make([]string, 0, len(ordered))
	for _, model := range ordered {
		parts = append(parts, openClawModel(model))
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func openClawModel(model ModelRef) string {
	parts := []string{`"id":` + jsonString(model.ID)}
	if name := namedConfigLabel(model); name != "" {
		parts = append(parts, `"name":`+jsonString(name))
	}
	if model.ContextWindow != nil {
		parts = append(parts, `"contextWindow":`+strconv.FormatInt(*model.ContextWindow, 10))
	}
	if model.Vision != nil {
		parts = append(parts, `"input":`+openCodeInput(*model.Vision))
	}
	if model.Reasoning != nil && *model.Reasoning {
		parts = append(parts, `"reasoning":true`)
	}
	if compat := openClawCompat(model); compat != "" {
		parts = append(parts, `"compat":`+compat)
	}
	return "{" + strings.Join(parts, ",") + "}"
}

func openClawCompat(model ModelRef) string {
	fields := make([]string, 0, 4)
	if model.Tools != nil {
		fields = append(fields, `"supportsTools":`+strconv.FormatBool(*model.Tools))
	}
	if model.Reasoning != nil && *model.Reasoning {
		quoted := make([]string, 0, len(modelEfforts(model)))
		for _, effort := range modelEfforts(model) {
			quoted = append(quoted, jsonString(effort))
		}
		if len(quoted) > 0 {
			fields = append(fields,
				`"supportsReasoningEffort":true`,
				`"thinkingFormat":"openai"`,
				`"supportedReasoningEfforts":[`+strings.Join(quoted, ",")+`]`,
			)
		}
	}
	if len(fields) == 0 {
		return ""
	}
	return "{" + strings.Join(fields, ",") + "}"
}
