// Credential model rows and the repository reading them.
package sqlite

import (
	"context"
	"database/sql"
	"fmt"
)

const (
	listCredentialModelsSQL = `SELECT credential_id, model_id, source, available, observed_at
	FROM credential_models ORDER BY credential_id, model_id`
	listCredentialRostersSQL = `SELECT credential_id, source, models_count, observed_at
	FROM credential_rosters ORDER BY credential_id`
	upsertCredentialModelSQL = `INSERT INTO credential_models (credential_id, model_id, source, available, observed_at)
	VALUES (?, ?, ?, ?, ?)
	ON CONFLICT (credential_id, model_id) DO UPDATE SET
		source = excluded.source, available = excluded.available, observed_at = excluded.observed_at`
	upsertCredentialRosterSQL = `INSERT INTO credential_rosters (credential_id, source, models_count, observed_at)
	VALUES (?, ?, ?, ?)
	ON CONFLICT (credential_id) DO UPDATE SET
		source = excluded.source, models_count = excluded.models_count, observed_at = excluded.observed_at`
	deleteCredentialModelsSQL  = `DELETE FROM credential_models WHERE credential_id = ?`
	deleteCredentialRostersSQL = `DELETE FROM credential_rosters WHERE credential_id = ?`
)

// CredentialModelRow is one stored pair of a credential and a model.
type CredentialModelRow struct {
	CredentialID string
	ModelID      string
	Source       string
	Available    bool
	ObservedAtMs int64
}

// CredentialRosterRow marks one credential whose own model roster is known.
type CredentialRosterRow struct {
	CredentialID string
	Source       string
	ModelsCount  int
	ObservedAtMs int64
}

// CredentialModelRepo stores which models one account can serve.
type CredentialModelRepo struct {
	db *DB
}

// NewCredentialModelRepo returns a repository over the given database.
func NewCredentialModelRepo(db *DB) *CredentialModelRepo {
	return &CredentialModelRepo{db: db}
}

// List returns every stored observation and roster marker.
func (r *CredentialModelRepo) List(ctx context.Context) ([]CredentialModelRow, []CredentialRosterRow, error) {
	models, err := r.listModels(ctx)
	if err != nil {
		return nil, nil, err
	}
	rosters, err := r.listRosters(ctx)
	if err != nil {
		return nil, nil, err
	}
	return models, rosters, nil
}

func (r *CredentialModelRepo) listModels(ctx context.Context) ([]CredentialModelRow, error) {
	rows, err := r.db.sql.QueryContext(ctx, listCredentialModelsSQL)
	if err != nil {
		return nil, fmt.Errorf("list credential models: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var models []CredentialModelRow
	for rows.Next() {
		var row CredentialModelRow
		if err := rows.Scan(&row.CredentialID, &row.ModelID, &row.Source, &row.Available, &row.ObservedAtMs); err != nil {
			return nil, fmt.Errorf("scan credential model: %w", err)
		}
		models = append(models, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate credential models: %w", err)
	}
	return models, nil
}

func (r *CredentialModelRepo) listRosters(ctx context.Context) ([]CredentialRosterRow, error) {
	rows, err := r.db.sql.QueryContext(ctx, listCredentialRostersSQL)
	if err != nil {
		return nil, fmt.Errorf("list credential rosters: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var rosters []CredentialRosterRow
	for rows.Next() {
		var row CredentialRosterRow
		if err := rows.Scan(&row.CredentialID, &row.Source, &row.ModelsCount, &row.ObservedAtMs); err != nil {
			return nil, fmt.Errorf("scan credential roster: %w", err)
		}
		rosters = append(rosters, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate credential rosters: %w", err)
	}
	return rosters, nil
}

// SetRoster replaces one account's listing: every row it had is dropped, the
// ids the provider just published are written, and the marker that makes the
// roster known is upserted. A learned refusal for a model the listing still
// carries does not survive, because the listing is the fresher evidence.
func (r *CredentialModelRepo) SetRoster(ctx context.Context, credentialID, source string, modelIDs []string, observedAtMs int64) error {
	tx, err := r.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin roster write: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, deleteCredentialModelsSQL, credentialID); err != nil {
		return fmt.Errorf("clear credential models: %w", err)
	}
	seen := map[string]bool{}
	count := 0
	for _, modelID := range modelIDs {
		if modelID == "" || seen[modelID] {
			continue
		}
		seen[modelID] = true
		count++
		if _, err := tx.ExecContext(ctx, upsertCredentialModelSQL,
			credentialID, modelID, source, true, observedAtMs); err != nil {
			return fmt.Errorf("write credential model %s: %w", modelID, err)
		}
	}
	if _, err := tx.ExecContext(ctx, upsertCredentialRosterSQL, credentialID, source, count, observedAtMs); err != nil {
		return fmt.Errorf("write credential roster: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit roster write: %w", err)
	}
	return nil
}

// Record stores one learned observation about a pair.
func (r *CredentialModelRepo) Record(ctx context.Context, row CredentialModelRow) error {
	if _, err := r.db.sql.ExecContext(ctx, upsertCredentialModelSQL,
		row.CredentialID, row.ModelID, row.Source, row.Available, row.ObservedAtMs); err != nil {
		return fmt.Errorf("record credential model %s: %w", row.ModelID, err)
	}
	return nil
}

// Forget drops everything one credential was observed to serve, which is what
// removing the account requires.
func (r *CredentialModelRepo) Forget(ctx context.Context, credentialID string) error {
	return r.forget(ctx, r.db.sql, credentialID)
}

func (r *CredentialModelRepo) forget(ctx context.Context, handle execer, credentialID string) error {
	if _, err := handle.ExecContext(ctx, deleteCredentialModelsSQL, credentialID); err != nil {
		return fmt.Errorf("forget credential models: %w", err)
	}
	if _, err := handle.ExecContext(ctx, deleteCredentialRostersSQL, credentialID); err != nil {
		return fmt.Errorf("forget credential roster: %w", err)
	}
	return nil
}

type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}
