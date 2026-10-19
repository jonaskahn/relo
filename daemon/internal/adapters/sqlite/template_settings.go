// Template choices: per-template context and refresh preferences.
package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// TemplateChoice is what one provider template stores beside the connection:
// whether it publishes a 1M context entry, whether Relo renews its tokens,
// whether it trims tools the gateway refuses, and the model families its
// roster keeps.
type TemplateChoice struct {
	LongContext   bool
	AutoRefresh   bool
	ToolCutoff    bool
	ModelPrefixes []string
}

// TemplateSettings stores those choices by template identifier.
type TemplateSettings struct {
	db *DB
}

// NewTemplateSettings returns template settings over the database.
func NewTemplateSettings(db *DB) *TemplateSettings {
	return &TemplateSettings{db: db}
}

// Get returns the stored choice. A missing row is not an error: the caller
// keeps the defaults, which are both enabled.
func (s *TemplateSettings) Get(ctx context.Context, templateID string) (TemplateChoice, bool, error) {
	var choice TemplateChoice
	var prefixes string
	err := s.db.sql.QueryRowContext(ctx,
		"SELECT long_context, auto_refresh, tool_cutoff, model_prefixes FROM template_settings WHERE template_id = ?",
		templateID).Scan(&choice.LongContext, &choice.AutoRefresh, &choice.ToolCutoff, &prefixes)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return TemplateChoice{}, false, nil
		}
		choiceWithoutPrefixes := TemplateChoice{ToolCutoff: true}
		fallback := s.db.sql.QueryRowContext(ctx,
			"SELECT long_context, auto_refresh FROM template_settings WHERE template_id = ?",
			templateID).Scan(&choiceWithoutPrefixes.LongContext, &choiceWithoutPrefixes.AutoRefresh)
		if fallback != nil {
			if errors.Is(fallback, sql.ErrNoRows) {
				return TemplateChoice{}, false, nil
			}
			return TemplateChoice{}, false, fmt.Errorf("read template settings for %s: %w", templateID, err)
		}
		return choiceWithoutPrefixes, true, nil
	}
	choice.ModelPrefixes = splitPrefixes(prefixes)
	return choice, true, nil
}

// ContextVariants returns the stored long-context choices the catalog overlays
// on its defaults.
func (s *TemplateSettings) ContextVariants(ctx context.Context) (map[string]bool, error) {
	rows, err := s.db.sql.QueryContext(ctx, "SELECT template_id, long_context FROM template_settings")
	if err != nil {
		return nil, fmt.Errorf("read context variants: %w", err)
	}
	defer func() { _ = rows.Close() }()
	stored := map[string]bool{}
	for rows.Next() {
		var templateID string
		var enabled bool
		if err := rows.Scan(&templateID, &enabled); err != nil {
			return nil, fmt.Errorf("scan context variant: %w", err)
		}
		stored[templateID] = enabled
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate context variants: %w", err)
	}
	return stored, nil
}

// Save writes the fields the patch names and leaves the others as stored,
// or as enabled when this template has no row yet.
func (s *TemplateSettings) Save(ctx context.Context, templateID string, longContext, autoRefresh *bool, nowMs int64) error {
	current, found, err := s.Get(ctx, templateID)
	if err != nil {
		return err
	}
	if !found {
		current = TemplateChoice{LongContext: true, AutoRefresh: true, ToolCutoff: true}
	}
	if longContext != nil {
		current.LongContext = *longContext
	}
	if autoRefresh != nil {
		current.AutoRefresh = *autoRefresh
	}
	return s.saveChoice(ctx, templateID, current, nowMs)
}

// SavePrefixes writes the model families one template's roster keeps.
func (s *TemplateSettings) SavePrefixes(ctx context.Context, templateID string, prefixes []string, nowMs int64) error {
	current, found, err := s.Get(ctx, templateID)
	if err != nil {
		return err
	}
	if !found {
		current = TemplateChoice{LongContext: true, AutoRefresh: true, ToolCutoff: true}
	}
	current.ModelPrefixes = prefixes
	return s.saveChoice(ctx, templateID, current, nowMs)
}

// SaveToolCutoff stores whether one template trims a tool list the OpenCode
// Free gateway would refuse.
func (s *TemplateSettings) SaveToolCutoff(ctx context.Context, templateID string, toolCutoff bool, nowMs int64) error {
	current, found, err := s.Get(ctx, templateID)
	if err != nil {
		return err
	}
	if !found {
		current = TemplateChoice{LongContext: true, AutoRefresh: true, ToolCutoff: true}
	}
	current.ToolCutoff = toolCutoff
	return s.saveChoice(ctx, templateID, current, nowMs)
}

func splitPrefixes(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	prefixes := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.ToLower(strings.TrimSpace(part))
		if trimmed == "" {
			continue
		}
		prefixes = append(prefixes, trimmed)
	}
	return prefixes
}

func joinPrefixes(prefixes []string) string {
	trimmed := make([]string, 0, len(prefixes))
	for _, prefix := range prefixes {
		cleaned := strings.ToLower(strings.TrimSpace(prefix))
		if cleaned == "" {
			continue
		}
		trimmed = append(trimmed, cleaned)
	}
	return strings.Join(trimmed, ",")
}

func (s *TemplateSettings) saveChoice(ctx context.Context, templateID string, choice TemplateChoice, nowMs int64) error {
	_, err := s.db.sql.ExecContext(ctx,
		`INSERT INTO template_settings (template_id, long_context, auto_refresh, tool_cutoff, model_prefixes, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(template_id) DO UPDATE SET
		   long_context = excluded.long_context,
		   auto_refresh = excluded.auto_refresh,
		   tool_cutoff = excluded.tool_cutoff,
		   model_prefixes = excluded.model_prefixes,
		   updated_at = excluded.updated_at`,
		templateID, choice.LongContext, choice.AutoRefresh, choice.ToolCutoff, joinPrefixes(choice.ModelPrefixes), nowMs)
	if err != nil {
		if strings.Contains(err.Error(), "no such column: model_prefixes") {
			_, fallback := s.db.sql.ExecContext(ctx,
				`INSERT INTO template_settings (template_id, long_context, auto_refresh, updated_at)
				 VALUES (?, ?, ?, ?)
				 ON CONFLICT(template_id) DO UPDATE SET
				   long_context = excluded.long_context,
				   auto_refresh = excluded.auto_refresh,
				   updated_at = excluded.updated_at`,
				templateID, choice.LongContext, choice.AutoRefresh, nowMs)
			if fallback != nil {
				return fmt.Errorf("store template settings for %s: %w", templateID, fallback)
			}
			return nil
		}
		return fmt.Errorf("store template settings for %s: %w", templateID, err)
	}
	return nil
}

// AutoRefresh reports whether one connection may renew its own tokens. The
// stored switch decides for a connection whose row names it, and a missing
// row stays on.
func (s *TemplateSettings) AutoRefresh(ctx context.Context, providerID string) bool {
	choice, found, err := s.Get(ctx, providerID)
	if err != nil || !found {
		return true
	}
	return choice.AutoRefresh
}

// Prefixes returns the model families one template's roster keeps. A missing
// row or an empty list means the template default applies.
func (s *TemplateSettings) Prefixes(ctx context.Context, templateID string) ([]string, bool, error) {
	choice, found, err := s.Get(ctx, templateID)
	if err != nil || !found {
		return nil, found, err
	}
	if len(choice.ModelPrefixes) == 0 {
		return nil, true, nil
	}
	return choice.ModelPrefixes, true, nil
}
