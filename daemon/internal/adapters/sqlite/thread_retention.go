// Thread retention: how long a continued conversation stays recoverable.
package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jonaskahn/relo/internal/clock"
	"github.com/jonaskahn/relo/internal/threads"
)

const (
	selectThreadRetentionSQL = `SELECT days, updated_at FROM thread_retention_config WHERE id = 1`
	upsertThreadRetentionSQL = `INSERT INTO thread_retention_config (id, days, updated_at)
VALUES (1, ?, ?)
ON CONFLICT (id) DO UPDATE SET days = excluded.days, updated_at = excluded.updated_at`
)

// ThreadRetention stores the thread window and applies it to the turns table.
// A store built without one keeps the default window.
type ThreadRetention struct {
	db  *DB
	clk clock.Clock
}

// NewThreadRetention returns the thread retention over the database.
func NewThreadRetention(db *DB, clk clock.Clock) *ThreadRetention {
	if clk == nil {
		clk = clock.New()
	}
	return &ThreadRetention{db: db, clk: clk}
}

// Days returns the stored window, and whether an operator ever picked one.
func (s *ThreadRetention) Days(ctx context.Context) (int, bool, error) {
	var days int
	var updatedAt int64
	err := s.db.sql.QueryRowContext(ctx, selectThreadRetentionSQL).Scan(&days, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return threads.DefaultRetentionDays, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("read thread retention: %w", err)
	}
	return days, true, nil
}

// SaveDays stores the window, refusing one the store cannot honour.
func (s *ThreadRetention) SaveDays(ctx context.Context, days int) error {
	if days < 0 {
		return threads.ErrInvalidRetention
	}
	if _, err := s.db.sql.ExecContext(ctx, upsertThreadRetentionSQL, days, s.clk.Now().UnixMilli()); err != nil {
		return fmt.Errorf("store thread retention: %w", err)
	}
	return nil
}

// Sweep deletes every turn last written before the stored window.
func (s *ThreadRetention) Sweep(ctx context.Context) (int64, error) {
	days, _, err := s.Days(ctx)
	if err != nil {
		return 0, err
	}
	return s.deleteBefore(ctx, days)
}

// Preview reports what the stored window would delete, without deleting.
func (s *ThreadRetention) Preview(ctx context.Context, days int) (int64, error) {
	cutoff := threads.CutoffMs(days, s.clk.Now())
	if cutoff == 0 {
		return 0, nil
	}
	return NewThreadTurns(s.db).CountBefore(ctx, cutoff)
}

func (s *ThreadRetention) deleteBefore(ctx context.Context, days int) (int64, error) {
	cutoff := threads.CutoffMs(days, s.clk.Now())
	if cutoff == 0 {
		return 0, nil
	}
	return NewThreadTurns(s.db).DeleteBefore(ctx, cutoff)
}

// Now is the clock the window is measured against, so a preview and a sweep
// read the same day.
func (s *ThreadRetention) Now() time.Time {
	return s.clk.Now()
}
