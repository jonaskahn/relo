// Usage recorder: appending request events and attempts.
package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jonaskahn/relo/internal/activity"
)

const defaultOrigin = "external"

const insertEventSQL = `INSERT INTO usage_events (
	request_id, timestamp, provider, model, requested_model, group_id, credential_label,
	surface, status, duration_ms, input_tokens, output_tokens,
	cache_read_tokens, cache_write_tokens, estimated_cost_micros,
	route_provider, route_reason, client_key_id, client_key_name, client_app, origin, credential_id, extra,
	attempts, retried
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

const insertAttemptSQL = `INSERT INTO usage_attempts (
	event_id, ordinal, provider, model, credential_label, credential_id, status, error_code,
	duration_ms, input_tokens, output_tokens
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

// UsageEvent is one row of usage_events. InputTokens is inclusive of the
// cache read and cache write counts, as providers report it; the cache
// counts are stored separately so cost estimation can apply their rates.
type UsageEvent = activity.RequestEvent

// UsageAttempt is one upstream attempt of a logical request.
type UsageAttempt = activity.RequestAttempt

// UsageRecorder writes usage rows for completed requests.
type UsageRecorder struct {
	db      *DB
	logger  *slog.Logger
	event   *sql.Stmt
	attempt *sql.Stmt
}

// NewUsageRecorder prepares the recorder statements on the given database.
func NewUsageRecorder(db *DB, logger *slog.Logger) (*UsageRecorder, error) {
	event, err := db.sql.Prepare(insertEventSQL)
	if err != nil {
		return nil, fmt.Errorf("prepare usage event insert: %w", err)
	}
	attempt, err := db.sql.Prepare(insertAttemptSQL)
	if err != nil {
		_ = event.Close()
		return nil, fmt.Errorf("prepare usage attempt insert: %w", err)
	}
	return &UsageRecorder{db: db, logger: logger, event: event, attempt: attempt}, nil
}

// Close releases the prepared statements.
func (r *UsageRecorder) Close() error {
	return errors.Join(r.event.Close(), r.attempt.Close())
}

// AppendEvent inserts one usage row together with the aggregate of its UTC
// day, in one transaction, and returns the row's identifier. The two land
// together so a finalize pass running while a request finishes either
// recomputes the day from the row or sees it arrive afterwards, never both.
func (r *UsageRecorder) AppendEvent(ctx context.Context, event UsageEvent) (int64, error) {
	tx, err := r.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin usage event: %w", err)
	}
	result, err := tx.StmtContext(ctx, r.event).ExecContext(ctx, eventArgs(event)...)
	if err != nil {
		_ = tx.Rollback()
		return 0, fmt.Errorf("append usage event: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		_ = tx.Rollback()
		return 0, fmt.Errorf("read usage event id: %w", err)
	}
	if err := addDaily(ctx, tx, event, event.Attempts, event.Retried); err != nil {
		_ = tx.Rollback()
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit usage event: %w", err)
	}
	if r.logger != nil {
		r.logger.Debug("usage event recorded", "request_id", event.RequestID, "status", event.Status)
	}
	return id, nil
}

// AppendAttempt inserts one attempt row linked to its parent event.
func (r *UsageRecorder) AppendAttempt(ctx context.Context, attempt UsageAttempt) error {
	_, err := r.attempt.ExecContext(ctx, attemptArgs(attempt)...)
	if err != nil {
		return fmt.Errorf("append usage attempt: %w", err)
	}
	return nil
}

func eventArgs(event UsageEvent) []any {
	return []any{
		event.RequestID, event.Timestamp, event.Provider, event.Model, event.RequestedModel,
		nullText(event.GroupID), event.CredentialLabel,
		event.Surface, event.Status, event.DurationMs, event.InputTokens, event.OutputTokens,
		event.CacheReadTokens, event.CacheWriteTokens, costArg(event.EstimatedCostMicros),
		event.RouteProvider, event.RouteReason, nullText(event.ClientKeyID), nullText(event.ClientKeyName),
		event.ClientApp, originArg(event.Origin), nullText(event.CredentialID), extraArg(event.Warnings),
		event.Attempts, boolCount(event.Retried),
	}
}

func extraArg(warnings []string) string {
	if len(warnings) == 0 {
		return "{}"
	}
	body, err := json.Marshal(map[string][]string{"warnings": warnings})
	if err != nil {
		return "{}"
	}
	return string(body)
}

func originArg(origin string) string {
	if origin == "" {
		return defaultOrigin
	}
	return origin
}

func attemptArgs(attempt UsageAttempt) []any {
	return []any{
		attempt.EventID, attempt.Ordinal, attempt.Provider, attempt.Model,
		attempt.CredentialLabel, nullText(attempt.CredentialID), attempt.Status, attempt.ErrorCode,
		attempt.DurationMs, attempt.InputTokens, attempt.OutputTokens,
	}
}

func costArg(micros *int64) sql.NullInt64 {
	if micros == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *micros, Valid: true}
}

func nullText(value string) any {
	if value == "" {
		return nil
	}
	return value
}
