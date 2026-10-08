// Integration rows: files and operations per coding client.
package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

const (
	getIntegrationSQL  = "SELECT id, enabled, key_id, state, last_error, updated_at, models_digest, context_1m FROM integrations WHERE id = ?"
	saveIntegrationSQL = "INSERT INTO integrations (id, enabled, key_id, state, last_error, updated_at, models_digest, context_1m)" +
		" VALUES (?, ?, ?, ?, ?, ?, ?, ?)" +
		" ON CONFLICT (id) DO UPDATE SET enabled = excluded.enabled, key_id = excluded.key_id," +
		" state = excluded.state, last_error = excluded.last_error, updated_at = excluded.updated_at," +
		" models_digest = excluded.models_digest, context_1m = excluded.context_1m"
	listIntegrationFilesSQL = "SELECT integration_id, kind, path, digest, snapshot_path, updated_at" +
		" FROM integration_files WHERE integration_id = ? ORDER BY kind"
	saveIntegrationFileSQL = "INSERT INTO integration_files (integration_id, kind, path, digest, snapshot_path, updated_at)" +
		" VALUES (?, ?, ?, ?, ?, ?)" +
		" ON CONFLICT (integration_id, kind) DO UPDATE SET path = excluded.path, digest = excluded.digest," +
		" snapshot_path = excluded.snapshot_path, updated_at = excluded.updated_at"
	deleteIntegrationFilesSQL = "DELETE FROM integration_files WHERE integration_id = ?"
	appendIntegrationOpSQL    = "INSERT INTO integration_ops (id, integration_id, action, detail, created_at) VALUES (?, ?, ?, ?, ?)"
	listIntegrationOpsSQL     = "SELECT id, integration_id, action, detail, created_at" +
		" FROM integration_ops WHERE integration_id = ? ORDER BY created_at DESC, id DESC LIMIT ?"
)

// IntegrationRow is one stored integration: the operator's intent, what the
// last write achieved, and the key it owns.
type IntegrationRow struct {
	ID           string
	Enabled      bool
	KeyID        string
	State        string
	LastError    string
	UpdatedAtMs  int64
	ModelsDigest string
	Context1M    bool
}

// IntegrationFileRow is one file Relo wrote for an integration.
type IntegrationFileRow struct {
	IntegrationID string
	Kind          string
	Path          string
	Digest        string
	SnapshotPath  string
	UpdatedAtMs   int64
}

// IntegrationOpRow is one action Relo took, kept as the history of a machine.
type IntegrationOpRow struct {
	ID            string
	IntegrationID string
	Action        string
	Detail        string
	CreatedAtMs   int64
}

// IntegrationRepo is the SQLite-backed integration store.
type IntegrationRepo struct {
	db *DB
}

// NewIntegrationRepo returns a repository over the given database.
func NewIntegrationRepo(db *DB) *IntegrationRepo {
	return &IntegrationRepo{db: db}
}

// Get returns one integration, and whether it exists.
func (r *IntegrationRepo) Get(ctx context.Context, id string) (IntegrationRow, bool, error) {
	var row IntegrationRow
	err := r.db.sql.QueryRowContext(ctx, getIntegrationSQL, id).Scan(
		&row.ID, &row.Enabled, &row.KeyID, &row.State, &row.LastError, &row.UpdatedAtMs, &row.ModelsDigest, &row.Context1M)
	if errors.Is(err, sql.ErrNoRows) {
		return IntegrationRow{}, false, nil
	}
	if err != nil {
		return IntegrationRow{}, false, fmt.Errorf("read integration %s: %w", id, err)
	}
	return row, true, nil
}

// Save stores one integration.
func (r *IntegrationRepo) Save(ctx context.Context, row IntegrationRow) error {
	if _, err := r.db.sql.ExecContext(ctx, saveIntegrationSQL,
		row.ID, row.Enabled, row.KeyID, row.State, row.LastError, row.UpdatedAtMs, row.ModelsDigest, row.Context1M); err != nil {
		return fmt.Errorf("save integration %s: %w", row.ID, err)
	}
	return nil
}

// Files returns every file one integration owns.
func (r *IntegrationRepo) Files(ctx context.Context, id string) ([]IntegrationFileRow, error) {
	rows, err := r.db.sql.QueryContext(ctx, listIntegrationFilesSQL, id)
	if err != nil {
		return nil, fmt.Errorf("list the files of %s: %w", id, err)
	}
	defer func() { _ = rows.Close() }()
	files := make([]IntegrationFileRow, 0, 4)
	for rows.Next() {
		var row IntegrationFileRow
		if err := rows.Scan(&row.IntegrationID, &row.Kind, &row.Path, &row.Digest, &row.SnapshotPath, &row.UpdatedAtMs); err != nil {
			return nil, fmt.Errorf("scan integration file: %w", err)
		}
		files = append(files, row)
	}
	return files, rows.Err()
}

// SaveFile records one file Relo wrote.
func (r *IntegrationRepo) SaveFile(ctx context.Context, row IntegrationFileRow) error {
	if _, err := r.db.sql.ExecContext(ctx, saveIntegrationFileSQL,
		row.IntegrationID, row.Kind, row.Path, row.Digest, row.SnapshotPath, row.UpdatedAtMs); err != nil {
		return fmt.Errorf("save integration file %s: %w", row.Kind, err)
	}
	return nil
}

// DeleteFiles forgets every file one integration owned, which is what removing
// it requires.
func (r *IntegrationRepo) DeleteFiles(ctx context.Context, id string) error {
	if _, err := r.db.sql.ExecContext(ctx, deleteIntegrationFilesSQL, id); err != nil {
		return fmt.Errorf("delete the files of %s: %w", id, err)
	}
	return nil
}

// AppendOp records one action.
func (r *IntegrationRepo) AppendOp(ctx context.Context, row IntegrationOpRow) error {
	if _, err := r.db.sql.ExecContext(ctx, appendIntegrationOpSQL,
		row.ID, row.IntegrationID, row.Action, row.Detail, row.CreatedAtMs); err != nil {
		return fmt.Errorf("record the action %s: %w", row.Action, err)
	}
	return nil
}

// Ops returns the most recent actions of one integration, newest first.
func (r *IntegrationRepo) Ops(ctx context.Context, id string, limit int) ([]IntegrationOpRow, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := r.db.sql.QueryContext(ctx, listIntegrationOpsSQL, id, limit)
	if err != nil {
		return nil, fmt.Errorf("list the actions of %s: %w", id, err)
	}
	defer func() { _ = rows.Close() }()
	ops := make([]IntegrationOpRow, 0, limit)
	for rows.Next() {
		var row IntegrationOpRow
		if err := rows.Scan(&row.ID, &row.IntegrationID, &row.Action, &row.Detail, &row.CreatedAtMs); err != nil {
			return nil, fmt.Errorf("scan integration action: %w", err)
		}
		ops = append(ops, row)
	}
	return ops, rows.Err()
}
