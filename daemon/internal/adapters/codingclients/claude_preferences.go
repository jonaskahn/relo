// Personal model preferences in Claude Code's user settings.
package codingclients

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/jonaskahn/relo/internal/catalog"
)

const (
	claudeEffortKey  = "effortLevel"
	claudeCompactKey = "autoCompactWindow"
	claudeModelKey   = "model"
	claudeModelsKey  = "modelSettings"
	// claudeSubagentModelEnv is the default model for subagents that do not
	// name one. A definition or a per-spawn model still wins.
	claudeSubagentModelEnv = "CLAUDE_CODE_SUBAGENT_MODEL"
	// claudeAutoCompact is the documented value that returns a model to the
	// window Claude Code tunes for it.
	claudeAutoCompact = "auto"
	claudeCompactMin  = 100_000
	claudeCompactMax  = 1_000_000
)

// ClaudePreferences is the personal model choice stored in Claude Code's user
// settings. An empty model or effort, or an auto compact window, means Claude
// Code's own default.
type ClaudePreferences struct {
	Model         string
	SubagentModel string
	Effort        string
	AutoCompact   string
	Entries       map[string]ClaudeModelPreference
}

// ClaudeModelPreference is the effort and compact window saved for one model.
type ClaudeModelPreference struct {
	Effort      string
	AutoCompact string
}

// ClaudePreferenceUpdate replaces the default model and that model's saved
// effort and compact window.
type ClaudePreferenceUpdate struct {
	Model         string
	SubagentModel string
	Effort        string
	AutoCompact   string
}

// ReadClaudePreferences reads the default model and every per-model choice.
// Keys Relo does not manage stay unread.
func ReadClaudePreferences(content string) (ClaudePreferences, error) {
	document, err := openClaudeSettings(content)
	if err != nil {
		return ClaudePreferences{}, err
	}
	model, err := settingsString(document, claudeModelKey)
	if err != nil {
		return ClaudePreferences{}, err
	}
	entries, err := readModelEntries(document)
	if err != nil {
		return ClaudePreferences{}, err
	}
	subagent, err := readSubagentModel(document)
	if err != nil {
		return ClaudePreferences{}, err
	}
	preferences := ClaudePreferences{Model: model, SubagentModel: subagent, AutoCompact: claudeAutoCompact, Entries: entries}
	if saved, found := entries[model]; found && model != "" {
		preferences.Effort = saved.Effort
		preferences.AutoCompact = saved.AutoCompact
	}
	return preferences, nil
}

// UpdateClaudePreferences writes one personal choice. An empty model clears
// the default and does not change another model's saved settings. Empty effort
// and an auto compact window remove only those overrides.
func UpdateClaudePreferences(content string, update ClaudePreferenceUpdate) (string, error) {
	document, err := openClaudeSettings(content)
	if err != nil {
		return "", err
	}
	model := strings.TrimSpace(update.Model)
	effort := strings.TrimSpace(update.Effort)
	compact := strings.TrimSpace(update.AutoCompact)
	if compact == "" {
		compact = claudeAutoCompact
	}
	if model == "" {
		if err := clearDefaultModel(document, effort, compact); err != nil {
			return "", err
		}
	} else if err := writeModelChoice(document, model, effort, compact); err != nil {
		return "", err
	}
	if err := writeSubagentModel(document, strings.TrimSpace(update.SubagentModel)); err != nil {
		return "", err
	}
	return encodeSettings(document)
}

// ClearReloClaudePreferences removes a selected Relo gateway model and the
// per-model settings of every Relo gateway model. An alias or a native model
// stays, including settings Claude Code saved for it.
func ClearReloClaudePreferences(content string) (string, error) {
	document, err := openClaudeSettings(content)
	if err != nil {
		return "", err
	}
	model, err := settingsString(document, claudeModelKey)
	if err != nil {
		return "", err
	}
	if ReloGatewayModel(model) {
		delete(document, claudeModelKey)
	}
	settings, err := modelSettings(document)
	if err != nil {
		return "", err
	}
	for id := range settings {
		if ReloGatewayModel(id) {
			delete(settings, id)
		}
	}
	if len(settings) == 0 {
		delete(document, claudeModelsKey)
	} else {
		document[claudeModelsKey] = settings
	}
	if err := clearReloSubagentModel(document); err != nil {
		return "", err
	}
	return encodeSettings(document)
}

// ReloGatewayModel reports a model id Relo publishes for Claude Code.
func ReloGatewayModel(id string) bool {
	bare := strings.ToLower(catalog.StripContextSuffix(id))
	return strings.HasPrefix(bare, "claude-relo-") || strings.HasPrefix(bare, "claude-reloc-")
}

func openClaudeSettings(content string) (map[string]any, error) {
	document, err := decodeSettings(content)
	if err != nil {
		return nil, err
	}
	return document, nil
}

func readModelEntries(document map[string]any) (map[string]ClaudeModelPreference, error) {
	settings, err := modelSettings(document)
	if err != nil {
		return nil, err
	}
	entries := make(map[string]ClaudeModelPreference, len(settings))
	for id, raw := range settings {
		entry, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("claude-code: %w (modelSettings.%s is not an object)", ErrUnrecognisedFile, id)
		}
		read, err := readModelPreference(entry)
		if err != nil {
			return nil, err
		}
		entries[id] = read
	}
	return entries, nil
}

func clearDefaultModel(document map[string]any, effort, compact string) error {
	if effort != "" || compact != claudeAutoCompact {
		return fmt.Errorf("claude-code: %w (a model is required)", ErrUnrecognisedFile)
	}
	if _, err := settingsString(document, claudeModelKey); err != nil {
		return err
	}
	delete(document, claudeModelKey)
	return nil
}

func readSubagentModel(document map[string]any) (string, error) {
	env, err := settingsEnv(document)
	if err != nil {
		return "", err
	}
	value, _ := env[claudeSubagentModelEnv].(string)
	return value, nil
}

func writeSubagentModel(document map[string]any, model string) error {
	env, err := settingsEnv(document)
	if err != nil {
		return err
	}
	if model == "" {
		delete(env, claudeSubagentModelEnv)
	} else {
		env[claudeSubagentModelEnv] = model
	}
	return storeEnv(document, env)
}

func clearReloSubagentModel(document map[string]any) error {
	env, err := settingsEnv(document)
	if err != nil {
		return err
	}
	value, ok := env[claudeSubagentModelEnv].(string)
	if !ok || !ReloGatewayModel(value) {
		return nil
	}
	delete(env, claudeSubagentModelEnv)
	return storeEnv(document, env)
}

func storeEnv(document, env map[string]any) error {
	if len(env) == 0 {
		delete(document, "env")
		return nil
	}
	document["env"] = env
	return nil
}

func writeModelChoice(document map[string]any, model, effort, compact string) error {
	if !knownEffort(effort) {
		return fmt.Errorf("claude-code: %w (effort must be low, medium, high, or xhigh)", ErrUnrecognisedFile)
	}
	window, clearWindow, err := compactWindow(compact)
	if err != nil {
		return err
	}
	settings, err := modelSettings(document)
	if err != nil {
		return err
	}
	entry, err := oneModelSettings(settings, model)
	if err != nil {
		return err
	}
	storeModelChoice(document, settings, modelChoice{
		model: model, effort: effort, entry: entry, window: window, clearWindow: clearWindow,
	})
	return nil
}

type modelChoice struct {
	model       string
	effort      string
	entry       map[string]any
	window      int64
	clearWindow bool
}

func storeModelChoice(document, settings map[string]any, choice modelChoice) {
	if choice.effort == "" {
		delete(choice.entry, claudeEffortKey)
	} else {
		choice.entry[claudeEffortKey] = choice.effort
	}
	if choice.clearWindow {
		delete(choice.entry, claudeCompactKey)
	} else {
		choice.entry[claudeCompactKey] = choice.window
	}
	if len(choice.entry) == 0 {
		delete(settings, choice.model)
	} else {
		settings[choice.model] = choice.entry
	}
	if len(settings) == 0 {
		delete(document, claudeModelsKey)
	} else {
		document[claudeModelsKey] = settings
	}
	document[claudeModelKey] = choice.model
}

func readModelPreference(entry map[string]any) (ClaudeModelPreference, error) {
	preference := ClaudeModelPreference{AutoCompact: claudeAutoCompact}
	if raw, found := entry[claudeEffortKey]; found {
		effort, ok := raw.(string)
		if !ok {
			return ClaudeModelPreference{}, fmt.Errorf("claude-code: %w (effortLevel is not text)", ErrUnrecognisedFile)
		}
		preference.Effort = effort
	}
	if raw, found := entry[claudeCompactKey]; found {
		compact, err := compactText(raw)
		if err != nil {
			return ClaudeModelPreference{}, err
		}
		preference.AutoCompact = compact
	}
	return preference, nil
}

func compactText(raw any) (string, error) {
	switch value := raw.(type) {
	case string:
		if value == claudeAutoCompact {
			return claudeAutoCompact, nil
		}
	case float64:
		if value >= 0 && value == math.Trunc(value) {
			return strconv.FormatInt(int64(value), 10), nil
		}
	}
	return "", fmt.Errorf("claude-code: %w (autoCompactWindow is not a token count)", ErrUnrecognisedFile)
}

func compactWindow(value string) (int64, bool, error) {
	if value == claudeAutoCompact {
		return 0, true, nil
	}
	window, err := strconv.ParseInt(value, 10, 64)
	if err != nil || window < claudeCompactMin || window > claudeCompactMax {
		return 0, false, fmt.Errorf("claude-code: %w (auto compact must be auto or a token count from 100000 to 1000000)", ErrUnrecognisedFile)
	}
	return window, false, nil
}

func knownEffort(value string) bool {
	switch value {
	case "", "low", "medium", "high", "xhigh":
		return true
	default:
		return false
	}
}

func settingsString(document map[string]any, key string) (string, error) {
	raw, found := document[key]
	if !found {
		return "", nil
	}
	value, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("claude-code: %w (%s is not text)", ErrUnrecognisedFile, key)
	}
	return value, nil
}

func modelSettings(document map[string]any) (map[string]any, error) {
	raw, found := document[claudeModelsKey]
	if !found {
		return map[string]any{}, nil
	}
	settings, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("claude-code: %w (modelSettings is not an object)", ErrUnrecognisedFile)
	}
	return settings, nil
}

func oneModelSettings(settings map[string]any, model string) (map[string]any, error) {
	raw, found := settings[model]
	if !found {
		return map[string]any{}, nil
	}
	entry, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("claude-code: %w (modelSettings.%s is not an object)", ErrUnrecognisedFile, model)
	}
	return entry, nil
}
