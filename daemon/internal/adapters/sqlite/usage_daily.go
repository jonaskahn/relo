// Daily usage archive: day buckets and raw-data boundaries.
package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

const (
	dayLayout = "2006-01-02"
	dayMs     = 86_400_000
	// finalizeDaysPerPass bounds one catch-up, so a long history is closed
	// over several maintenance passes rather than in one long write loop.
	finalizeDaysPerPass = 400
)

const (
	selectFinalizedDaySQL  = `SELECT max(day) FROM usage_days WHERE finalized = 1`
	selectOldestEventSQL   = `SELECT min(timestamp) FROM usage_events`
	selectFinalizedDaysSQL = `SELECT day FROM usage_days WHERE finalized = 1`
	selectDayFinalizedSQL  = `SELECT finalized FROM usage_days WHERE day = ?`
	// claimDaySQL takes the write lock and makes sure the day has a row
	// before a pass decides anything, so two passes cannot both close it.
	claimDaySQL = `INSERT INTO usage_days (day, finalized, computed_at) VALUES (?, 0, 0)
	ON CONFLICT (day) DO NOTHING`
	markFinalizedSQL = `INSERT INTO usage_days (day, finalized, computed_at) VALUES (?, 1, ?)
	ON CONFLICT (day) DO UPDATE SET finalized = 1, computed_at = excluded.computed_at`
	upsertDailySQL = `INSERT INTO usage_daily (
	day, provider, model, account_id, account_label, client_id, client_name, origin, surface, status,
	requests, input_tokens, output_tokens, cache_read_tokens, cache_write_tokens,
	cost_micros, unpriced_requests, duration_ms, duration_max_ms, attempts, retried_requests
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (day, provider, model, account_id, account_label, client_id, client_name, origin, surface, status)
DO UPDATE SET
	requests = requests + excluded.requests,
	input_tokens = input_tokens + excluded.input_tokens,
	output_tokens = output_tokens + excluded.output_tokens,
	cache_read_tokens = cache_read_tokens + excluded.cache_read_tokens,
	cache_write_tokens = cache_write_tokens + excluded.cache_write_tokens,
	cost_micros = cost_micros + excluded.cost_micros,
	unpriced_requests = unpriced_requests + excluded.unpriced_requests,
	duration_ms = duration_ms + excluded.duration_ms,
	duration_max_ms = max(duration_max_ms, excluded.duration_max_ms),
	attempts = attempts + excluded.attempts,
	retried_requests = retried_requests + excluded.retried_requests`
	// mergeDaySQL fills one closed day from the request rows it still holds,
	// keeping whichever side is ahead on every counter: a day whose live
	// writes went missing is raised to what the rows prove, a day an older
	// version wrote is filled in, and a total is never lowered. Rebuilding a
	// total down from the rows is what loses data once a cleanup has
	// shortened the log, so a seal may only move a counter up.
	mergeDaySQL = `INSERT INTO usage_daily (
	day, provider, model, account_id, account_label, client_id, client_name, origin, surface, status,
	requests, input_tokens, output_tokens, cache_read_tokens, cache_write_tokens,
	cost_micros, unpriced_requests, duration_ms, duration_max_ms, attempts, retried_requests
)
SELECT strftime('%Y-%m-%d', timestamp / 1000, 'unixepoch'),
	provider, model, COALESCE(credential_id, ''), COALESCE(credential_label, ''),
	COALESCE(client_key_id, ''), COALESCE(client_key_name, ''),
	COALESCE(origin, ''), COALESCE(surface, ''), status,
	count(*), sum(input_tokens), sum(output_tokens), sum(cache_read_tokens), sum(cache_write_tokens),
	sum(COALESCE(estimated_cost_micros, 0)),
	sum(CASE WHEN estimated_cost_micros IS NULL THEN 1 ELSE 0 END),
	sum(duration_ms), max(duration_ms), sum(attempts), sum(retried)
FROM usage_events
WHERE timestamp >= ? AND timestamp < ?
GROUP BY 2, 3, 4, 5, 6, 7, 8, 9, 10
ON CONFLICT (day, provider, model, account_id, account_label, client_id, client_name, origin, surface, status)
DO UPDATE SET
	requests = max(requests, excluded.requests),
	input_tokens = max(input_tokens, excluded.input_tokens),
	output_tokens = max(output_tokens, excluded.output_tokens),
	cache_read_tokens = max(cache_read_tokens, excluded.cache_read_tokens),
	cache_write_tokens = max(cache_write_tokens, excluded.cache_write_tokens),
	cost_micros = max(cost_micros, excluded.cost_micros),
	unpriced_requests = max(unpriced_requests, excluded.unpriced_requests),
	duration_ms = max(duration_ms, excluded.duration_ms),
	duration_max_ms = max(duration_max_ms, excluded.duration_max_ms),
	attempts = max(attempts, excluded.attempts),
	retried_requests = max(retried_requests, excluded.retried_requests)`
)

// Archive holds the daily aggregates of the usage log: one row per closed
// UTC day and per combination of the dimensions the console filters by, so a
// total survives the deletion of the request rows it came from.
type Archive struct {
	db  *DB
	now func() time.Time
}

// NewArchive returns an archive over the given database.
func NewArchive(db *DB) *Archive {
	return &Archive{db: db, now: time.Now}
}

// SetClock fixes the time the archive reads, which is what a maintenance
// pass and a test about day boundaries both need.
func (a *Archive) SetClock(now func() time.Time) {
	if now != nil {
		a.now = now
	}
}

// DayOf returns the UTC day one timestamp belongs to.
func DayOf(timestampMs int64) string {
	return time.UnixMilli(timestampMs).UTC().Format(dayLayout)
}

// DayStartMs returns the first millisecond of one UTC day.
func DayStartMs(day string) (int64, error) {
	parsed, err := time.ParseInLocation(dayLayout, day, time.UTC)
	if err != nil {
		return 0, fmt.Errorf("parse day %s: %w", day, err)
	}
	return parsed.UnixMilli(), nil
}

// RawFromMs returns the first millisecond of the request rows that are still
// authoritative, and whether any closed day is stored. Every day before it is
// read from the archive.
func (a *Archive) RawFromMs(ctx context.Context) (int64, bool, error) {
	var day sql.NullString
	if err := a.db.sql.QueryRowContext(ctx, selectFinalizedDaySQL).Scan(&day); err != nil {
		return 0, false, fmt.Errorf("read the archived days: %w", err)
	}
	if !day.Valid || day.String == "" {
		return 0, false, nil
	}
	start, err := DayStartMs(day.String)
	if err != nil {
		return 0, false, err
	}
	return start + dayMs, true, nil
}

func addDaily(ctx context.Context, tx *sql.Tx, event UsageEvent, attempts int, retried bool) error {
	cost := int64(0)
	unpriced := 0
	if event.EstimatedCostMicros != nil {
		cost = *event.EstimatedCostMicros
	} else {
		unpriced = 1
	}
	_, err := tx.ExecContext(ctx, upsertDailySQL,
		DayOf(event.Timestamp), event.Provider, event.Model,
		event.CredentialID, event.CredentialLabel,
		event.ClientKeyID, event.ClientKeyName,
		originArg(event.Origin), event.Surface,
		event.Status, event.InputTokens, event.OutputTokens,
		event.CacheReadTokens, event.CacheWriteTokens, cost, unpriced,
		event.DurationMs, event.DurationMs, attempts, boolCount(retried),
	)
	if err != nil {
		return fmt.Errorf("add a day of usage: %w", err)
	}
	return nil
}

// Finalize closes every day up to yesterday that the archive does not hold
// yet, raising each one to the totals its request rows prove. It reports how
// many days it closed.
func (a *Archive) Finalize(ctx context.Context) (int, error) {
	oldest, err := a.oldestEvent(ctx)
	if err != nil {
		return 0, err
	}
	if oldest == 0 {
		return 0, nil
	}
	done, err := a.finalizedDays(ctx)
	if err != nil {
		return 0, err
	}
	today := DayOf(a.now().UnixMilli())
	closed := 0
	for cursor := DayOf(oldest); cursor < today && closed < finalizeDaysPerPass; cursor = nextDay(cursor) {
		if done[cursor] {
			continue
		}
		if err := a.finalizeDay(ctx, cursor); err != nil {
			return closed, err
		}
		closed++
	}
	return closed, nil
}

func (a *Archive) finalizeDay(ctx context.Context, day string) error {
	start, err := DayStartMs(day)
	if err != nil {
		return err
	}
	tx, err := a.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin finalize %s: %w", day, err)
	}
	if _, err := tx.ExecContext(ctx, claimDaySQL, day); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("claim day %s: %w", day, err)
	}
	finalized, err := claimedDayFinalized(tx, ctx, day)
	if err != nil {
		return err
	}
	if finalized {
		// Another pass already closed the day; its aggregate is the truth.
		_ = tx.Rollback()
		return nil
	}
	return sealFinalizedDay(tx, ctx, a, day, start)
}

func sealFinalizedDay(tx *sql.Tx, ctx context.Context, a *Archive, day string, start int64) error {
	if _, err := tx.ExecContext(ctx, mergeDaySQL, start, start+dayMs); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("seal day %s: %w", day, err)
	}
	if _, err := tx.ExecContext(ctx, markFinalizedSQL, day, a.now().UnixMilli()); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("mark day %s final: %w", day, err)
	}
	return commitFinalizedDay(tx, day)
}

func commitFinalizedDay(tx *sql.Tx, day string) error {
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit day %s: %w", day, err)
	}
	return nil
}

func claimedDayFinalized(tx *sql.Tx, ctx context.Context, day string) (bool, error) {
	var finalized int
	if err := tx.QueryRowContext(ctx, selectDayFinalizedSQL, day).Scan(&finalized); err != nil {
		_ = tx.Rollback()
		return false, fmt.Errorf("read day %s: %w", day, err)
	}
	return finalized == 1, nil
}

func (a *Archive) oldestEvent(ctx context.Context) (int64, error) {
	var oldest sql.NullInt64
	if err := a.db.sql.QueryRowContext(ctx, selectOldestEventSQL).Scan(&oldest); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("read the oldest usage event: %w", err)
	}
	if !oldest.Valid {
		return 0, nil
	}
	return oldest.Int64, nil
}

func (a *Archive) finalizedDays(ctx context.Context) (map[string]bool, error) {
	rows, err := a.db.sql.QueryContext(ctx, selectFinalizedDaysSQL)
	if err != nil {
		return nil, fmt.Errorf("read the finalized days: %w", err)
	}
	defer func() { _ = rows.Close() }()
	days := map[string]bool{}
	for rows.Next() {
		var day string
		if err := rows.Scan(&day); err != nil {
			return nil, fmt.Errorf("scan a finalized day: %w", err)
		}
		days[day] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate the finalized days: %w", err)
	}
	return days, nil
}

func nextDay(day string) string {
	start, err := DayStartMs(day)
	if err != nil {
		return day
	}
	return DayOf(start + dayMs)
}

func boolCount(value bool) int {
	if value {
		return 1
	}
	return 0
}
