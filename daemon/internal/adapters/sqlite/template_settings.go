// Template choices: per-template context and refresh preferences.
package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// TemplateChoice is what one provider template stores beside the connection:
// whether it publishes a 1M context entry, and whether Relo renews its tokens.
type TemplateChoice struct {
	LongContext bool
	AutoRefresh bool
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
	err := s.db.sql.QueryRowContext(ctx,
		"SELECT long_context, auto_refresh FROM template_settings WHERE template_id = ?",
		templateID).Scan(&choice.LongContext, &choice.AutoRefresh)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return TemplateChoice{}, false, nil
		}
		return TemplateChoice{}, false, fmt.Errorf("read template settings for %s: %w", templateID, err)
	}
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
		current = TemplateChoice{LongContext: true, AutoRefresh: true}
	}
	if longContext != nil {
		current.LongContext = *longContext
	}
	if autoRefresh != nil {
		current.AutoRefresh = *autoRefresh
	}
	_, err = s.db.sql.ExecContext(ctx,
		`INSERT INTO template_settings (template_id, long_context, auto_refresh, updated_at)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(template_id) DO UPDATE SET
		   long_context = excluded.long_context,
		   auto_refresh = excluded.auto_refresh,
		   updated_at = excluded.updated_at`,
		templateID, current.LongContext, current.AutoRefresh, nowMs)
	if err != nil {
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
