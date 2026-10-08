// Credential rows and the repository reading them.
package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/jonaskahn/relo/internal/account"
)

const (
	listCredentialsSQL = `SELECT id, provider_id, kind, COALESCE(label, ''), COALESCE(secret_ref, ''), status, priority
	FROM credentials WHERE deleted_at IS NULL ORDER BY provider_id, priority, id`
	insertCredentialSQL = `INSERT INTO credentials (
	id, provider_id, kind, label, secret_ref, status, priority, generation, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, 0, ?, ?)`
	updateCredentialPrioritySQL = `UPDATE credentials SET priority = ?, updated_at = ?
	WHERE id = ? AND deleted_at IS NULL`
	updateCredentialStatusSQL = `UPDATE credentials SET status = ?, updated_at = ?
	WHERE id = ? AND deleted_at IS NULL`
	deleteCredentialSQL = `UPDATE credentials SET deleted_at = ?, updated_at = ?
	WHERE id = ? AND deleted_at IS NULL`
	listSecretRefsSQL = `SELECT DISTINCT secret_ref FROM credentials
	WHERE secret_ref IS NOT NULL AND secret_ref != ''`
)

// CredentialRow is one live row of the credentials table.
type CredentialRow = account.PoolEntry

// CredentialRepo is the SQLite-backed credential repository.
type CredentialRepo struct {
	db *DB
}

// NewCredentialRepo returns a repository over the given database.
func NewCredentialRepo(db *DB) *CredentialRepo {
	return &CredentialRepo{db: db}
}

// List returns every live credential, ordered by provider and priority.
func (r *CredentialRepo) List(ctx context.Context) ([]CredentialRow, error) {
	rows, err := r.db.sql.QueryContext(ctx, listCredentialsSQL)
	if err != nil {
		return nil, fmt.Errorf("list credentials: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanCredentials(rows)
}

// Insert stores a new credential.
func (r *CredentialRepo) Insert(ctx context.Context, row CredentialRow) error {
	now := time.Now().Unix()
	_, err := r.db.sql.ExecContext(ctx, insertCredentialSQL,
		row.ID, row.ProviderID, row.Kind, row.Label, row.SecretRef, row.Status, row.Priority, now, now)
	if err != nil {
		return fmt.Errorf("insert credential %s: %w", row.ID, err)
	}
	return nil
}

// SetStatus updates the status of a live credential.
func (r *CredentialRepo) SetStatus(ctx context.Context, id string, status string) error {
	now := time.Now().Unix()
	return r.execOne(ctx, updateCredentialStatusSQL, "update credential status", status, now, id)
}

// SetPriority changes how the pool ranks a credential.
func (r *CredentialRepo) SetPriority(ctx context.Context, id string, priority int) error {
	now := time.Now().Unix()
	return r.execOne(ctx, updateCredentialPrioritySQL, "update credential priority", priority, now, id)
}

// SoftDelete tombstones a credential so its usage rows stay attributable.
// What the account was observed to serve goes with it: the rows describe a
// live account, and an id nothing answers for must not keep them.
func (r *CredentialRepo) SoftDelete(ctx context.Context, id string) error {
	now := time.Now().Unix()
	tx, err := r.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin credential delete: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, deleteCredentialSQL, now, now, id)
	if err != nil {
		return fmt.Errorf("delete credential: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete credential: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("delete credential: %w", ErrCredentialNotFound)
	}
	models := NewCredentialModelRepo(r.db)
	if err := models.forget(ctx, tx, id); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit credential delete: %w", err)
	}
	return nil
}

// SecretRefs returns every credential reference this state directory stored,
// tombstoned credentials included, so a wipe can name the secrets behind them
// before the home that holds the list goes.
func (r *CredentialRepo) SecretRefs(ctx context.Context) ([]string, error) {
	rows, err := r.db.sql.QueryContext(ctx, listSecretRefsSQL)
	if err != nil {
		return nil, fmt.Errorf("list secret refs: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var refs []string
	for rows.Next() {
		var ref string
		if err := rows.Scan(&ref); err != nil {
			return nil, fmt.Errorf("scan secret ref: %w", err)
		}
		refs = append(refs, ref)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list secret refs: %w", err)
	}
	return refs, nil
}

func (r *CredentialRepo) execOne(ctx context.Context, query, action string, args ...any) error {
	result, err := r.db.sql.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("%s: %w", action, err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("%s: %w", action, err)
	}
	if affected == 0 {
		return fmt.Errorf("%s: %w", action, ErrCredentialNotFound)
	}
	return nil
}

func scanCredentials(rows *sql.Rows) ([]CredentialRow, error) {
	var credentials []CredentialRow
	for rows.Next() {
		var row CredentialRow
		if err := rows.Scan(&row.ID, &row.ProviderID, &row.Kind, &row.Label, &row.SecretRef, &row.Status, &row.Priority); err != nil {
			return nil, fmt.Errorf("scan credential: %w", err)
		}
		credentials = append(credentials, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate credentials: %w", err)
	}
	return credentials, nil
}
