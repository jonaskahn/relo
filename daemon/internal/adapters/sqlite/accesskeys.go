// Client-key rows and the repository reading them.
package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/jonaskahn/relo/internal/access"
)

var (
	// ErrAccessKeyNotFound reports an identifier no live key carries.
	ErrAccessKeyNotFound = errors.New("access key not found")
	// ErrAccessKeyNameTaken reports a live key that already has a name.
	ErrAccessKeyNameTaken = errors.New("an active access key already has that name")
)

const accessKeyColumns = `id, name, kind, client, token_digest, token_hint, generation,
	COALESCE(expires_at, 0), COALESCE(revoked_at, 0), created_at, updated_at, COALESCE(last_used_at, 0), owner`

const (
	listAccessKeysSQL = `SELECT ` + accessKeyColumns + ` FROM access_keys
		ORDER BY created_at DESC, id`
	getAccessKeySQL    = `SELECT ` + accessKeyColumns + ` FROM access_keys WHERE id = ?`
	insertAccessKeySQL = `INSERT INTO access_keys (
		id, name, kind, client, token_digest, token_hint, generation, expires_at, created_at, updated_at, owner
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	updateAccessKeySQL = `UPDATE access_keys SET name = ?, expires_at = ?, updated_at = ?
		WHERE id = ? AND revoked_at IS NULL`
	rotateAccessKeySQL = `UPDATE access_keys SET token_digest = ?, token_hint = ?,
		generation = generation + 1, expires_at = ?, updated_at = ?
		WHERE id = ? AND revoked_at IS NULL`
	revokeAccessKeySQL = `UPDATE access_keys SET revoked_at = ?, updated_at = ?
		WHERE id = ? AND revoked_at IS NULL`
	touchAccessKeySQL          = `UPDATE access_keys SET last_used_at = ? WHERE id = ?`
	deleteExpiredAccessKeysSQL = `DELETE FROM access_keys
		WHERE revoked_at IS NULL AND owner = '' AND expires_at IS NOT NULL AND expires_at <= ?`
)

// AccessKeyRow is one row of the access_keys table. The digest and hint are
// the only shapes of the key's secret that exist outside the operator's
// hands.
type AccessKeyRow struct {
	ID           string
	Name         string
	Kind         string
	Client       string
	TokenDigest  string
	TokenHint    string
	Generation   int
	ExpiresAtMs  int64
	RevokedAtMs  int64
	CreatedAtMs  int64
	UpdatedAtMs  int64
	LastUsedAtMs int64
	// Owner names the integration this key belongs to, and is empty on a key
	// an operator minted for themselves.
	Owner string
}

// AccessKeyRepo is the SQLite-backed client key repository.
type AccessKeyRepo struct {
	db *DB
}

// NewAccessKeyRepo returns a repository over the given database.
func NewAccessKeyRepo(db *DB) *AccessKeyRepo {
	return &AccessKeyRepo{db: db}
}

// List returns every key, newest first, retired ones included so their usage
// rows stay explainable.
func (r *AccessKeyRepo) List(ctx context.Context) ([]AccessKeyRow, error) {
	rows, err := r.db.sql.QueryContext(ctx, listAccessKeysSQL)
	if err != nil {
		return nil, fmt.Errorf("list access keys: %w", err)
	}
	defer func() { _ = rows.Close() }()
	keys, err := scanAccessKeys(rows)
	if err != nil {
		return nil, err
	}
	return keys, nil
}

// Get returns one key by identifier, and whether it exists.
func (r *AccessKeyRepo) Get(ctx context.Context, id string) (AccessKeyRow, bool, error) {
	row := r.db.sql.QueryRowContext(ctx, getAccessKeySQL, id)
	key, err := scanAccessKey(row)
	if errors.Is(err, sql.ErrNoRows) {
		return AccessKeyRow{}, false, nil
	}
	if err != nil {
		return AccessKeyRow{}, false, fmt.Errorf("read access key %s: %w", id, err)
	}
	return key, true, nil
}

// Insert stores one key. A name a live key already holds is refused.
func (r *AccessKeyRepo) Insert(ctx context.Context, key AccessKeyRow) error {
	_, err := r.db.sql.ExecContext(ctx, insertAccessKeySQL,
		key.ID, key.Name, key.Kind, key.Client, key.TokenDigest, key.TokenHint,
		key.Generation, nullableMillis(key.ExpiresAtMs), key.CreatedAtMs, key.UpdatedAtMs, key.Owner)
	if err != nil {
		return fmt.Errorf("insert access key %s: %w", key.ID, uniqueNameError(err))
	}
	return nil
}

// Update replaces one live key's name and optional expiry. The caller passes
// the time so a test drives it by hand.
func (r *AccessKeyRepo) Update(ctx context.Context, update access.KeyUpdate) error {
	return r.execOne(ctx, updateAccessKeySQL, "update access key",
		update.Name, nullableMillis(update.ExpiresAtMs), update.AtMs, update.ID)
}

// Rotate replaces one live key's secret, bumping its generation so a stale
// secret is recognisable as stale rather than merely wrong.
func (r *AccessKeyRepo) Rotate(ctx context.Context, rotation access.KeyRotation) error {
	return r.execOne(ctx, rotateAccessKeySQL, "rotate access key",
		rotation.Digest, rotation.Hint, nullableMillis(rotation.ExpiresAtMs), rotation.AtMs, rotation.ID)
}

// Revoke retires one live key permanently. Revoking twice is harmless.
func (r *AccessKeyRepo) Revoke(ctx context.Context, id string, at int64) error {
	return r.execOne(ctx, revokeAccessKeySQL, "revoke access key", at, at, id)
}

// Touch records when a key last authenticated a request.
func (r *AccessKeyRepo) Touch(ctx context.Context, id string, at int64) error {
	_, err := r.db.sql.ExecContext(ctx, touchAccessKeySQL, at, id)
	if err != nil {
		return fmt.Errorf("touch access key %s: %w", id, err)
	}
	return nil
}

// DeleteExpired removes expired operator keys. An empty id list removes every
// eligible row; a named list removes that subset of them. A revoked key, a
// live key, and a key an integration owns stay.
func (r *AccessKeyRepo) DeleteExpired(ctx context.Context, nowMs int64, ids []string) (int, error) {
	query, args := expiredDeleteQuery(nowMs, ids)
	result, err := r.db.sql.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("delete expired access keys: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("delete expired access keys: %w", err)
	}
	return int(affected), nil
}

func expiredDeleteQuery(nowMs int64, ids []string) (string, []any) {
	args := make([]any, 0, 1+len(ids))
	args = append(args, nowMs)
	if len(ids) == 0 {
		return deleteExpiredAccessKeysSQL, args
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	for _, id := range ids {
		args = append(args, id)
	}
	return deleteExpiredAccessKeysSQL + " AND id IN (" + placeholders + ")", args
}

func (r *AccessKeyRepo) execOne(ctx context.Context, query, action string, args ...any) error {
	result, err := r.db.sql.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("%s: %w", action, uniqueNameError(err))
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("%s: %w", action, err)
	}
	if affected == 0 {
		return fmt.Errorf("%s: %w", action, ErrAccessKeyNotFound)
	}
	return nil
}

func uniqueNameError(err error) error {
	if err != nil && strings.Contains(err.Error(), "idx_access_keys_name") {
		return ErrAccessKeyNameTaken
	}
	return err
}

func scanAccessKeys(rows *sql.Rows) ([]AccessKeyRow, error) {
	keys := make([]AccessKeyRow, 0, 8)
	for rows.Next() {
		key, err := scanAccessKey(rows)
		if err != nil {
			return nil, fmt.Errorf("scan access key: %w", err)
		}
		keys = append(keys, key)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate access keys: %w", err)
	}
	return keys, nil
}

type accessKeyScanner interface {
	Scan(dest ...any) error
}

func scanAccessKey(row accessKeyScanner) (AccessKeyRow, error) {
	var key AccessKeyRow
	err := row.Scan(&key.ID, &key.Name, &key.Kind, &key.Client, &key.TokenDigest,
		&key.TokenHint, &key.Generation, &key.ExpiresAtMs, &key.RevokedAtMs,
		&key.CreatedAtMs, &key.UpdatedAtMs, &key.LastUsedAtMs, &key.Owner)
	return key, err
}

func nullableMillis(value int64) sql.NullInt64 {
	if value == 0 {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: value, Valid: true}
}
