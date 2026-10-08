// Quota snapshots: persisting vendor-reported windows.
package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

const (
	upsertQuotaSnapshotSQL = `INSERT INTO quota_snapshots (
	credential_id, window, used_percent, amount, currency, reset_at, window_seconds, source, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (credential_id, window) DO UPDATE SET
	used_percent = excluded.used_percent,
	amount       = excluded.amount,
	currency     = excluded.currency,
	reset_at     = excluded.reset_at,
	window_seconds = excluded.window_seconds,
	source       = excluded.source,
	updated_at   = excluded.updated_at`
	listQuotaSnapshotsSQL = `SELECT credential_id, window, COALESCE(used_percent, 0), amount, COALESCE(currency, ''), COALESCE(reset_at, 0), COALESCE(window_seconds, 0), COALESCE(source, ''), updated_at
	FROM quota_snapshots WHERE credential_id = ? ORDER BY window`
	insertQuotaHistorySQL = `INSERT OR REPLACE INTO quota_history (
	credential_id, window, observed_at, percent
) VALUES (?, ?, ?, ?)`
	trimQuotaHistoryAgeSQL         = `DELETE FROM quota_history WHERE observed_at < ?`
	trimQuotaHistoryCredentialsSQL = `DELETE FROM quota_history WHERE credential_id NOT IN (
	SELECT credential_id FROM quota_history GROUP BY credential_id ORDER BY MAX(observed_at) DESC LIMIT ?
)`
	trimQuotaHistoryTotalSQL = `DELETE FROM quota_history WHERE rowid NOT IN (
	SELECT rowid FROM quota_history ORDER BY observed_at DESC LIMIT ?
)`
	trimQuotaHistoryPerCredentialSQL = `DELETE FROM quota_history WHERE rowid IN (
	SELECT rowid FROM (
		SELECT rowid, ROW_NUMBER() OVER (PARTITION BY credential_id, window ORDER BY observed_at DESC) AS rank
		FROM quota_history
	) WHERE rank > ?
)`
)

// QuotaRow is one stored quota window.
type QuotaRow struct {
	CredentialID string
	Window       string
	UsedPercent  float64
	Amount       *float64
	Currency     string
	ResetAt      int64
	Seconds      int64
	Source       string
	ObservedAt   time.Time
}

// QuotaBounds are the fixed bounds the history ring is trimmed to.
type QuotaBounds struct {
	PerCredential int
	Credentials   int
	Total         int
	MaxAge        time.Duration
}

// QuotaRepo is the SQLite-backed quota store.
type QuotaRepo struct {
	db *DB
}

// NewQuotaRepo returns a quota repository over the given database.
func NewQuotaRepo(db *DB) *QuotaRepo {
	return &QuotaRepo{db: db}
}

// UpsertSnapshots stores the current windows of every credential.
func (r *QuotaRepo) UpsertSnapshots(ctx context.Context, rows []QuotaRow) error {
	for _, row := range rows {
		if _, err := r.db.sql.ExecContext(ctx, upsertQuotaSnapshotSQL,
			row.CredentialID, row.Window, row.UsedPercent, row.Amount, row.Currency, row.ResetAt, row.Seconds,
			row.Source, row.ObservedAt.Unix()); err != nil {
			return fmt.Errorf("store quota snapshot for %s: %w", row.CredentialID, err)
		}
	}
	return nil
}

// ListSnapshots returns the current windows of one credential.
func (r *QuotaRepo) ListSnapshots(ctx context.Context, credentialID string) ([]QuotaRow, error) {
	rows, err := r.db.sql.QueryContext(ctx, listQuotaSnapshotsSQL, credentialID)
	if err != nil {
		return nil, fmt.Errorf("list quota snapshots: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanQuotaRows(rows)
}

// AppendHistory records the observations the history ring keeps.
func (r *QuotaRepo) AppendHistory(ctx context.Context, rows []QuotaRow) error {
	for _, row := range rows {
		if _, err := r.db.sql.ExecContext(ctx, insertQuotaHistorySQL,
			row.CredentialID, row.Window, row.ObservedAt.Unix(), row.UsedPercent); err != nil {
			return fmt.Errorf("store quota history for %s: %w", row.CredentialID, err)
		}
	}
	return nil
}

// TrimHistory brings the history ring back inside its bounds.
func (r *QuotaRepo) TrimHistory(ctx context.Context, bounds QuotaBounds) error {
	age := time.Now().Add(-bounds.MaxAge).Unix()
	statements := []struct {
		query string
		arg   int64
	}{
		{trimQuotaHistoryAgeSQL, age},
		{trimQuotaHistoryCredentialsSQL, int64(bounds.Credentials)},
		{trimQuotaHistoryTotalSQL, int64(bounds.Total)},
		{trimQuotaHistoryPerCredentialSQL, int64(bounds.PerCredential)},
	}
	for _, statement := range statements {
		if _, err := r.db.sql.ExecContext(ctx, statement.query, statement.arg); err != nil {
			return fmt.Errorf("trim quota history: %w", err)
		}
	}
	return nil
}

func scanQuotaRows(rows *sql.Rows) ([]QuotaRow, error) {
	var stored []QuotaRow
	for rows.Next() {
		var row QuotaRow
		var amount sql.NullFloat64
		var updatedAt int64
		if err := rows.Scan(&row.CredentialID, &row.Window, &row.UsedPercent, &amount, &row.Currency, &row.ResetAt,
			&row.Seconds, &row.Source, &updatedAt); err != nil {
			return nil, fmt.Errorf("scan quota snapshot: %w", err)
		}
		if amount.Valid {
			row.Amount = new(amount.Float64)
		}
		row.ObservedAt = time.Unix(updatedAt, 0)
		stored = append(stored, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate quota snapshots: %w", err)
	}
	return stored, nil
}
