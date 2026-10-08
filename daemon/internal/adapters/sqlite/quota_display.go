// Quota display setting: how remaining quota reads.
package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// QuotaDisplaySettings stores how the console draws a quota chart.
type QuotaDisplaySettings struct {
	db *DB
}

// NewQuotaDisplaySettings returns quota-display settings over the database.
func NewQuotaDisplaySettings(db *DB) *QuotaDisplaySettings {
	return &QuotaDisplaySettings{db: db}
}

// QuotaDisplay returns the stored reading, and whether an operator ever
// picked one.
func (s *QuotaDisplaySettings) QuotaDisplay(ctx context.Context) (string, bool, error) {
	var display string
	err := s.db.sql.QueryRowContext(ctx, "SELECT quota_display FROM ui_settings WHERE id = 1").Scan(&display)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("read quota display: %w", err)
	}
	return display, true, nil
}

// SaveQuotaDisplay stores the reading an operator picked.
func (s *QuotaDisplaySettings) SaveQuotaDisplay(ctx context.Context, display string, nowMs int64) error {
	if _, err := s.db.sql.ExecContext(ctx,
		"INSERT INTO ui_settings (id, quota_display, updated_at) VALUES (1, ?, ?)"+
			" ON CONFLICT (id) DO UPDATE SET quota_display = excluded.quota_display, updated_at = excluded.updated_at",
		display, nowMs); err != nil {
		return fmt.Errorf("store quota display: %w", err)
	}
	return nil
}
