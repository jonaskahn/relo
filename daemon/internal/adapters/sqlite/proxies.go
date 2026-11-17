// Outbound proxy rows: the non-secret half of the managed pool.
package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	appproxy "github.com/jonaskahn/relo/internal/proxy"
)

const proxyColumns = `id, label, scheme, host, port, username, secret_ref, enabled, rank, created_at, updated_at`

// ProxyStore is the SQLite-backed proxy repository.
type ProxyStore struct{ db *DB }

// NewProxyStore returns a proxy repository over the given database.
func NewProxyStore(db *DB) *ProxyStore { return &ProxyStore{db: db} }

// List returns every proxy row.
func (s *ProxyStore) List(ctx context.Context) ([]appproxy.Entry, error) {
	rows, err := s.db.sql.QueryContext(ctx, "SELECT "+proxyColumns+" FROM proxies ORDER BY rank, id")
	if err != nil {
		return nil, fmt.Errorf("list proxies: %w", err)
	}
	defer func() { _ = rows.Close() }()
	entries := make([]appproxy.Entry, 0)
	for rows.Next() {
		entry, err := scanProxy(rows)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

// Get returns one proxy by id.
func (s *ProxyStore) Get(ctx context.Context, id string) (appproxy.Entry, bool, error) {
	row := s.db.sql.QueryRowContext(ctx, "SELECT "+proxyColumns+" FROM proxies WHERE id = ?", id)
	entry, err := scanProxy(row)
	if errors.Is(err, sql.ErrNoRows) {
		return appproxy.Entry{}, false, nil
	}
	if err != nil {
		return appproxy.Entry{}, false, err
	}
	return entry, true, nil
}

// Create inserts one proxy row.
func (s *ProxyStore) Create(ctx context.Context, entry appproxy.Entry) (appproxy.Entry, error) {
	_, err := s.db.sql.ExecContext(ctx, `INSERT INTO proxies (`+proxyColumns+`)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		entry.ID, entry.Label, entry.Scheme, entry.Host, entry.Port, entry.Username, entry.SecretRef,
		bit(entry.Enabled), entry.Rank, entry.CreatedAt, entry.UpdatedAt)
	if err != nil {
		return appproxy.Entry{}, fmt.Errorf("create proxy: %w", err)
	}
	return entry, nil
}

// Update replaces the editable columns of one proxy.
func (s *ProxyStore) Update(ctx context.Context, entry appproxy.Entry) error {
	result, err := s.db.sql.ExecContext(ctx, `UPDATE proxies SET
		label = ?, scheme = ?, host = ?, port = ?, username = ?, secret_ref = ?,
		enabled = ?, rank = ?, updated_at = ? WHERE id = ?`,
		entry.Label, entry.Scheme, entry.Host, entry.Port, entry.Username, entry.SecretRef,
		bit(entry.Enabled), entry.Rank, entry.UpdatedAt, entry.ID)
	if err != nil {
		return fmt.Errorf("update proxy: %w", err)
	}
	return requireOneProxy(result, "update")
}

// Delete removes one proxy row.
func (s *ProxyStore) Delete(ctx context.Context, id string) error {
	result, err := s.db.sql.ExecContext(ctx, "DELETE FROM proxies WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete proxy: %w", err)
	}
	return requireOneProxy(result, "delete")
}

// Count reports how many proxy rows exist.
func (s *ProxyStore) Count(ctx context.Context) (int, error) {
	var count int
	if err := s.db.sql.QueryRowContext(ctx, "SELECT count(*) FROM proxies").Scan(&count); err != nil {
		return 0, fmt.Errorf("count proxies: %w", err)
	}
	return count, nil
}

type proxyScanner interface{ Scan(...any) error }

func scanProxy(row proxyScanner) (appproxy.Entry, error) {
	var entry appproxy.Entry
	var enabled int
	err := row.Scan(&entry.ID, &entry.Label, &entry.Scheme, &entry.Host, &entry.Port, &entry.Username,
		&entry.SecretRef, &enabled, &entry.Rank, &entry.CreatedAt, &entry.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return appproxy.Entry{}, err
		}
		return appproxy.Entry{}, fmt.Errorf("scan proxy: %w", err)
	}
	entry.Enabled = enabled != 0
	return entry, nil
}

func requireOneProxy(result sql.Result, action string) error {
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("%s proxy: %w", action, err)
	}
	if affected == 0 {
		return appproxy.ErrNotFound
	}
	return nil
}
