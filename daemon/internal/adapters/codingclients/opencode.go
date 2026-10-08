// OpenCode config: merging Relo into its file and reading it back.
package codingclients

import (
	"fmt"
	"strconv"
	"strings"
)

const openCodeNPM = "@ai-sdk/openai-compatible"

func openCodeKeyRef() string {
	return "{env:" + EnvKeyName(ClientOpenCode) + "}"
}

// MergeOpenCode returns an OpenCode configuration with the Relo provider.
// Sibling keys are left as they were. A stale plural providers entry is
// removed, since current OpenCode only reads the singular provider key.
func MergeOpenCode(existing, baseURL string, models []ModelRef) (string, error) {
	address, err := requireBase(baseURL)
	if err != nil {
		return "", fmt.Errorf("opencode: %w", err)
	}
	merged, err := spliceKey(existing, []string{"provider", reloProviderID}, openCodeProvider(address, models))
	if err != nil {
		return "", fmt.Errorf("opencode: %w", err)
	}
	merged, err = deleteKey(merged, []string{"providers", reloProviderID})
	if err != nil {
		return "", fmt.Errorf("opencode: %w", err)
	}
	return ensureNewline(merged), nil
}

// StripOpenCode removes the Relo provider, including a stale plural entry.
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
	// A stale plural providers entry means the file still needs a merge.
	if has, err := jsonHas(existing, []string{"providers", reloProviderID}); err != nil || has {
		return false
	}
	raw, ok, err := jsonRaw(existing, []string{"provider", reloProviderID})
	if err != nil || !ok {
		return false
	}
	return jsonEqual(raw, openCodeProvider(address, models))
}

func openCodeHasRelo(existing string) (bool, error) {
	singular, err := jsonHas(existing, []string{"provider", reloProviderID})
	if err != nil || singular {
		return singular, err
	}
	return jsonHas(existing, []string{"providers", reloProviderID})
}

func openCodeFragment(baseURL string, models []ModelRef) (string, error) {
	address, err := requireBase(baseURL)
	if err != nil {
		return "", fmt.Errorf("opencode: %w", err)
	}
	return "{\n  \"provider\": {\"relo\": " + openCodeProvider(address, models) + "}\n}", nil
}

func openCodeProvider(address string, models []ModelRef) string {
	return `{"npm":` + jsonString(openCodeNPM) +
		`,"name":"Relo","options":{"baseURL":` + jsonString(address) +
		`,"apiKey":` + jsonString(openCodeKeyRef()) +
		`},"models":` + openCodeModels(models) + `}`
}

func openCodeModels(models []ModelRef) string {
	ordered := orderedModels(models)
	if len(ordered) == 0 {
		return "{}"
	}
	parts := make([]string, 0, len(ordered))
	for _, model := range ordered {
		parts = append(parts, jsonString(model.ID)+":"+openCodeModel(model))
	}
	return "{" + strings.Join(parts, ",") + "}"
}

func openCodeModel(model ModelRef) string {
	fields := make([]string, 0, 3)
	if name := namedConfigLabel(model); name != "" {
		fields = append(fields, `"name":`+jsonString(name))
	}
	if limit := openCodeLimit(model); limit != "" {
		fields = append(fields, `"limit":`+limit)
	}
	if modalities := openCodeModalities(model); modalities != "" {
		fields = append(fields, modalities)
	}
	if len(fields) == 0 {
		return "{}"
	}
	return "{" + strings.Join(fields, ",") + "}"
}
func openCodeModalities(model ModelRef) string {
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
