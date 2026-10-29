// OpenCode config: merging Relo into its file and reading it back.
package codingclients

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const openCodeNPM = "@ai-sdk/openai-compatible"

const openCodePackage = "@opencode/ai/providers/openai-compatible"

// OpenCodeV1, OpenCodeV2 and OpenCodeBoth name the configuration generations
// Relo can write: the legacy singular provider block, the current plural
// providers block, or both side by side in one file.
const (
	OpenCodeV1   = "v1"
	OpenCodeV2   = "v2"
	OpenCodeBoth = "v1+v2"
)

// ErrUnknownOpenCodeVersions reports a versions value outside the vocabulary
// above. The management API rejects it before a merge ever runs.
var ErrUnknownOpenCodeVersions = errors.New("unknown OpenCode config versions")

func openCodeKeyRef() string {
	return "{env:" + EnvKeyName(ClientOpenCode) + "}"
}

// MergeOpenCode returns an OpenCode configuration with the Relo provider in
// the current generation. Sibling keys are left as they were.
func MergeOpenCode(existing, baseURL string, models []ModelRef) (string, error) {
	return MergeOpenCodeVersions(existing, baseURL, models, OpenCodeV2)
}

// MergeOpenCodeVersions returns an OpenCode configuration with the Relo
// provider in the requested generations. A generation Relo does not own is
// left alone; a Relo block in a generation the request drops is removed, so
// switching from both generations to one cleans up after itself. An empty
// versions value writes the current generation.
func MergeOpenCodeVersions(existing, baseURL string, models []ModelRef, versions string) (string, error) {
	address, err := requireBase(baseURL)
	if err != nil {
		return "", fmt.Errorf("opencode: %w", err)
	}
	writeV1, writeV2, err := parseOpenCodeVersions(versions)
	if err != nil {
		return "", fmt.Errorf("opencode: %w", err)
	}
	merged := existing
	if writeV1 {
		merged, err = spliceKey(merged, []string{"provider", reloProviderID}, openCodeProvider(address, models))
	} else {
		merged, err = deleteKey(merged, []string{"provider", reloProviderID})
	}
	if err != nil {
		return "", fmt.Errorf("opencode: %w", err)
	}
	if writeV2 {
		merged, err = spliceKey(merged, []string{"providers", reloProviderID}, openCodeProviderV2(address, models))
	} else {
		merged, err = deleteKey(merged, []string{"providers", reloProviderID})
	}
	if err != nil {
		return "", fmt.Errorf("opencode: %w", err)
	}
	return ensureNewline(merged), nil
}

func parseOpenCodeVersions(versions string) (bool, bool, error) {
	switch versions {
	case "", OpenCodeV2:
		return false, true, nil
	case OpenCodeV1:
		return true, false, nil
	case OpenCodeBoth:
		return true, true, nil
	default:
		return false, false, fmt.Errorf("%q: %w", versions, ErrUnknownOpenCodeVersions)
	}
}

// OpenCodeVersionsPresent reports which generations hold a Relo provider: v1,
// v2, v1+v2, or empty when none does. An unreadable file reads as none.
func OpenCodeVersionsPresent(existing string) string {
	singular, _ := jsonHas(existing, []string{"provider", reloProviderID})
	plural, _ := jsonHas(existing, []string{"providers", reloProviderID})
	switch {
	case singular && plural:
		return OpenCodeBoth
	case singular:
		return OpenCodeV1
	case plural:
		return OpenCodeV2
	default:
		return ""
	}
}

// StripOpenCode removes the Relo provider in every generation.
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
	present := OpenCodeVersionsPresent(existing)
	if present == "" {
		return false
	}
	if strings.Contains(present, OpenCodeV1) {
		raw, ok, err := jsonRaw(existing, []string{"provider", reloProviderID})
		if err != nil || !ok || !jsonEqual(raw, openCodeProvider(address, models)) {
			return false
		}
	}
	if strings.Contains(present, OpenCodeV2) {
		raw, ok, err := jsonRaw(existing, []string{"providers", reloProviderID})
		if err != nil || !ok || !jsonEqual(raw, openCodeProviderV2(address, models)) {
			return false
		}
	}
	return true
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
	return "{\n  \"providers\": {\"relo\": " + openCodeProviderV2(address, models) + "}\n}", nil
}

func openCodeProvider(address string, models []ModelRef) string {
	return `{"npm":` + jsonString(openCodeNPM) +
		`,"name":"Relo","options":{"baseURL":` + jsonString(address) +
		`,"apiKey":` + jsonString(openCodeKeyRef()) +
		`},"models":` + openCodeModels(models) + `}`
}

func openCodeProviderV2(address string, models []ModelRef) string {
	return `{"package":` + jsonString(openCodePackage) +
		`,"name":"Relo","settings":{"baseURL":` + jsonString(address) +
		`,"apiKey":` + jsonString(openCodeKeyRef()) +
		`},"models":` + openCodeModelsV2(models) + `}`
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

func openCodeModelsV2(models []ModelRef) string {
	ordered := orderedModels(models)
	if len(ordered) == 0 {
		return "{}"
	}
	parts := make([]string, 0, len(ordered))
	for _, model := range ordered {
		parts = append(parts, jsonString(model.ID)+":"+openCodeModelV2(model))
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

func openCodeModelV2(model ModelRef) string {
	fields := make([]string, 0, 3)
	if name := namedConfigLabel(model); name != "" {
		fields = append(fields, `"name":`+jsonString(name))
	}
	if limit := openCodeLimit(model); limit != "" {
		fields = append(fields, `"limit":`+limit)
	}
	if capabilities := openCodeCapabilities(model); capabilities != "" {
		fields = append(fields, capabilities)
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

func openCodeCapabilities(model ModelRef) string {
	parts := make([]string, 0, 3)
	if model.Vision != nil {
		parts = append(parts, `"input":`+openCodeInput(*model.Vision))
		parts = append(parts, `"output":["text"]`)
	}
	if reasoning := openCodeReasoning(model); reasoning != "" {
		parts = append(parts, reasoning)
	}
	if len(parts) == 0 {
		return ""
	}
	return `"capabilities":{` + strings.Join(parts, ",") + `}`
}

func openCodeReasoning(model ModelRef) string {
	if model.Reasoning == nil || !*model.Reasoning {
		return ""
	}
	if model.ReasoningToggle != nil && *model.ReasoningToggle && model.ReasoningEfforts != nil && len(model.ReasoningEfforts) == 0 {
		return `"reasoning":{"supported":true,"toggle":true}`
	}
	if model.ReasoningEfforts != nil {
		quoted := make([]string, 0, len(model.ReasoningEfforts))
		for _, effort := range model.ReasoningEfforts {
			quoted = append(quoted, jsonString(effort))
		}
		return `"reasoning":{"supported":true,"effort":[` + strings.Join(quoted, ",") + `]}`
	}
	return `"reasoning":{"supported":true}`
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
