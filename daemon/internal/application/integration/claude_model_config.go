package integration

import (
	"context"
	"sort"
	"strconv"
	"strings"

	"github.com/jonaskahn/relo/internal/catalog"
)

const (
	claudeCompactMin = 100_000
	claudeCompactMax = 1_000_000
	claudeAuto       = "auto"
)

// ClaudeModelChoice is one model the Claude Code manage view can select.
type ClaudeModelChoice struct {
	ID            string
	Name          string
	ContextWindow *int64
	Efforts       []string
}

// ClaudeModelConfig is the personal choice together with the models the
// manage view can offer.
type ClaudeModelConfig struct {
	ClaudePreferences
	Models []ClaudeModelChoice
}

// preferenceError is a model choice the manage view can correct.
type preferenceError struct {
	message string
}

func (e preferenceError) Error() string { return e.message }

func (e preferenceError) Unwrap() error { return ErrInvalidQuery }

// ClaudeModelConfig reads the personal choice for an enabled Claude Code
// integration.
func (s *Service) ClaudeModelConfig(ctx context.Context, id string) (ClaudeModelConfig, error) {
	manager, err := s.claudeManager(ctx, id)
	if err != nil {
		return ClaudeModelConfig{}, err
	}
	preferences, err := manager.ReadClaudePreferences()
	if err != nil {
		return ClaudeModelConfig{}, err
	}
	return s.claudeConfig(preferences), nil
}

// SetClaudeModelConfig stores one personal choice and reads it back.
func (s *Service) SetClaudeModelConfig(ctx context.Context, id string, update ClaudePreferenceUpdate) (ClaudeModelConfig, error) {
	if id != "claude-code" {
		return ClaudeModelConfig{}, preferenceError{message: "model preferences are only available for claude code"}
	}
	if err := validateClaudeShape(update); err != nil {
		return ClaudeModelConfig{}, err
	}
	manager, err := s.claudeManager(ctx, id)
	if err != nil {
		return ClaudeModelConfig{}, err
	}
	current, err := manager.ReadClaudePreferences()
	if err != nil {
		return ClaudeModelConfig{}, err
	}
	if err := validateClaudeModel(update, current.Model, s.claudePickerModels()); err != nil {
		return ClaudeModelConfig{}, err
	}
	finish, err := s.begin("model-config", id)
	if err != nil {
		return ClaudeModelConfig{}, err
	}
	defer finish()
	saved, err := manager.WriteClaudePreferences(ctx, ClaudePreferenceUpdate{
		Model: strings.TrimSpace(update.Model), Effort: strings.TrimSpace(update.Effort),
		AutoCompact: compactOrAuto(update.AutoCompact),
	})
	if err != nil {
		return ClaudeModelConfig{}, err
	}
	return s.claudeConfig(saved), nil
}

func (s *Service) claudeManager(ctx context.Context, id string) (*Manager, error) {
	if id != "claude-code" {
		return nil, preferenceError{message: "model preferences are only available for claude code"}
	}
	manager, err := s.manager()
	if err != nil {
		return nil, err
	}
	view, err := manager.Inspect(ctx, id)
	if err != nil {
		return nil, err
	}
	if !view.Enabled {
		return nil, preferenceError{message: "claude code is not set up"}
	}
	return manager, nil
}

func (s *Service) claudeConfig(preferences ClaudePreferences) ClaudeModelConfig {
	if preferences.Entries == nil {
		preferences.Entries = map[string]ClaudeModelPreference{}
	}
	if preferences.AutoCompact == "" {
		preferences.AutoCompact = claudeAuto
	}
	choices := make([]ClaudeModelChoice, 0)
	for _, model := range s.claudePickerModels() {
		if strings.TrimSpace(model.ID) == "" {
			continue
		}
		choices = append(choices, ClaudeModelChoice{
			ID: model.ID, Name: model.Name, ContextWindow: model.ContextWindow,
			Efforts: claudeEffortLevels(model.ID + " " + model.Upstream),
		})
	}
	sort.Slice(choices, func(i, j int) bool { return choices[i].ID < choices[j].ID })
	return ClaudeModelConfig{ClaudePreferences: preferences, Models: choices}
}

func validateClaudeShape(update ClaudePreferenceUpdate) error {
	model := strings.TrimSpace(update.Model)
	effort := strings.TrimSpace(update.Effort)
	compact := compactOrAuto(update.AutoCompact)
	if model == "" && (effort != "" || compact != claudeAuto) {
		return preferenceError{message: "effort and auto compact need a selected model"}
	}
	if !claudeEffortAllowed(effort) {
		return preferenceError{message: "effort must be low, medium, high, or xhigh"}
	}
	if compact == claudeAuto {
		return nil
	}
	window, err := strconv.ParseInt(compact, 10, 64)
	if err != nil || window < claudeCompactMin || window > claudeCompactMax {
		return preferenceError{message: "auto compact must be auto or a token count from 100000 to 1000000"}
	}
	return nil
}

func validateClaudeModel(update ClaudePreferenceUpdate, current string, models []ModelRef) error {
	model := strings.TrimSpace(update.Model)
	if model == "" || claudeModelAllowed(model, current, models) {
		return nil
	}
	return preferenceError{message: "model is not available"}
}

func claudeModelAllowed(model, current string, models []ModelRef) bool {
	if model == current {
		return true
	}
	for _, candidate := range models {
		if candidate.ID == model {
			return true
		}
	}
	return false
}

func claudeEffortAllowed(effort string) bool {
	switch effort {
	case "", "low", "medium", "high", "xhigh":
		return true
	default:
		return false
	}
}

// claudeEffortLevels names the effort values Claude Code can save for a model.
// Opus 4.6 and Sonnet 4.6 have no extra-high level. max is session-only, so
// it is never offered here.
func claudeEffortLevels(model string) []string {
	id := strings.ToLower(catalog.StripContextSuffix(model))
	if strings.Contains(id, "opus-4-6") || strings.Contains(id, "sonnet-4-6") {
		return []string{"low", "medium", "high"}
	}
	return []string{"low", "medium", "high", "xhigh"}
}

func compactOrAuto(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return claudeAuto
	}
	return trimmed
}
