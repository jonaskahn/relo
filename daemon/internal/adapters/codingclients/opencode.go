// OpenCode config: merging Relo into its file and reading it back.
package codingclients

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	openCodeNPM     = "@ai-sdk/openai-compatible"
	openCodePackage = "@opencode-ai/ai/providers/openai-compatible"
)

func openCodeKeyRef() string {
	return "{env:" + EnvKeyName(ClientOpenCode) + "}"
}

// MergeOpenCode returns an OpenCode configuration with both provider
// generations pointing at Relo. Sibling keys are left as they were.
func MergeOpenCode(existing, baseURL string, models []ModelRef) (string, error) {
	address, err := requireBase(baseURL)
	if err != nil {
		return "", fmt.Errorf("opencode: %w", err)
	}
	merged, err := spliceKey(existing, []string{"provider", reloProviderID}, openCodeV1(address, models))
	if err != nil {
		return "", fmt.Errorf("opencode: %w", err)
	}
	merged, err = spliceKey(merged, []string{"providers", reloProviderID}, openCodeV2(address, models))
	if err != nil {
		return "", fmt.Errorf("opencode: %w", err)
	}
	return ensureNewline(merged), nil
}

// StripOpenCode removes both Relo provider generations.
func StripOpenCode(existing string) (string, error) {
	if strings.TrimSpace(existing) == "" {
		return "", nil
	}
	stripped, err := deleteKey(existing, []string{"provider", reloProviderID})
	if err != nil {
		return "", fmt.Errorf("opencode: %w", err)
	}
	stripped, err = deleteKey(stripped, []string{"providers", reloProviderID})
	if err != nil {
		return "", fmt.Errorf("opencode: %w", err)
	}
	if emptyDocument(stripped) {
		return "", nil
	}
	return ensureNewline(stripped), nil
}

func openCodePointsAt(existing, baseURL string, models []ModelRef) bool {
	address, err := requireBase(baseURL)
	if err != nil {
		return false
	}
	legacy, ok, err := jsonRaw(existing, []string{"provider", reloProviderID})
	if err != nil || !ok || !jsonEqual(legacy, openCodeV1(address, models)) {
		return false
	}
	current, ok, err := jsonRaw(existing, []string{"providers", reloProviderID})
	if err != nil || !ok {
		return false
	}
	return jsonEqual(current, openCodeV2(address, models))
}

func openCodeHasRelo(existing string) (bool, error) {
	legacy, err := jsonHas(existing, []string{"provider", reloProviderID})
	if err != nil || legacy {
		return legacy, err
	}
	return jsonHas(existing, []string{"providers", reloProviderID})
}

func openCodeFragment(baseURL string, models []ModelRef) (string, error) {
	address, err := requireBase(baseURL)
	if err != nil {
		return "", fmt.Errorf("opencode: %w", err)
	}
	return "{\n  \"provider\": {\"relo\": " + openCodeV1(address, models) +
		"},\n  \"providers\": {\"relo\": " + openCodeV2(address, models) + "}\n}", nil
}

func openCodeV1(address string, models []ModelRef) string {
	return `{"npm":` + jsonString(openCodeNPM) +
		`,"name":"Relo","options":{"baseURL":` + jsonString(address) +
		`,"apiKey":` + jsonString(openCodeKeyRef()) +
		`},"models":` + openCodeModels(models, false) + `}`
}

func openCodeV2(address string, models []ModelRef) string {
	return `{"package":` + jsonString(openCodePackage) +
		`,"name":"Relo","settings":{"baseURL":` + jsonString(address) +
		`,"apiKey":` + jsonString(openCodeKeyRef()) +
		`},"models":` + openCodeModels(models, true) + `}`
}

func openCodeModels(models []ModelRef, v2 bool) string {
	ordered := orderedModels(models)
	if len(ordered) == 0 {
		return "{}"
	}
	parts := make([]string, 0, len(ordered))
	for _, model := range ordered {
		parts = append(parts, jsonString(model.ID)+":"+openCodeModel(model, v2))
	}
	return "{" + strings.Join(parts, ",") + "}"
}

func openCodeModel(model ModelRef, v2 bool) string {
	fields := make([]string, 0, 4)
	if name := namedConfigLabel(model); name != "" {
		fields = append(fields, `"name":`+jsonString(name))
	}
	if limit := openCodeLimit(model); limit != "" {
		fields = append(fields, `"limit":`+limit)
	}
	if v2 {
		if capabilities := openCodeCapabilities(model); capabilities != "" {
			fields = append(fields, `"capabilities":`+capabilities)
		}
		if model.Reasoning != nil && *model.Reasoning {
			fields = append(fields, `"variants":`+openCodeVariants(modelEfforts(model)))
		}
	} else if modalities := openCodeV1Modalities(model); modalities != "" {
		fields = append(fields, modalities)
	}
	if len(fields) == 0 {
		return "{}"
	}
	return "{" + strings.Join(fields, ",") + "}"
}

func openCodeCapabilities(model ModelRef) string {
	if model.Tools == nil && model.Vision == nil {
		return ""
	}
	fields := make([]string, 0, 3)
	if model.Tools != nil {
		fields = append(fields, `"tools":`+strconv.FormatBool(*model.Tools))
	}
	if model.Vision != nil {
		fields = append(fields, `"input":`+openCodeInput(*model.Vision))
	}
	fields = append(fields, `"output":["text"]`)
	return "{" + strings.Join(fields, ",") + "}"
}

func openCodeV1Modalities(model ModelRef) string {
	if model.Vision == nil {
		return ""
	}
	return `"attachment":` + strconv.FormatBool(*model.Vision) +
		`,"modalities":{"input":` + openCodeInput(*model.Vision) + `,"output":["text"]}`
}

func openCodeInput(vision bool) string {
	if vision {
		return `["text","image"]`
	}
	return `["text"]`
}

func openCodeVariants(efforts []string) string {
	parts := make([]string, 0, len(efforts))
	for _, effort := range efforts {
		parts = append(parts, `{"id":`+jsonString(effort)+`,"settings":{"reasoningEffort":`+jsonString(effort)+`}}`)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func openCodeLimit(model ModelRef) string {
	parts := make([]string, 0, 2)
	if model.ContextWindow != nil {
		parts = append(parts, `"context":`+strconv.FormatInt(*model.ContextWindow, 10))
	}
	if model.MaxOutput != nil {
		parts = append(parts, `"output":`+strconv.FormatInt(*model.MaxOutput, 10))
	}
	if len(parts) == 0 {
		return ""
	}
	return "{" + strings.Join(parts, ",") + "}"
}
