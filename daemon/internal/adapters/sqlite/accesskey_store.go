// Client-key store: persisting the keys the data plane issues.
package sqlite

import (
	"context"
	"errors"

	"github.com/jonaskahn/relo/internal/access"
)

// AccessKeyStore adapts the access_keys table to the client-key lifecycle's
// store contract, so a use case never touches SQL.
type AccessKeyStore struct {
	repo *AccessKeyRepo
}

// NewAccessKeyStore returns a client-key store over the given database.
func NewAccessKeyStore(db *DB) *AccessKeyStore {
	return &AccessKeyStore{repo: NewAccessKeyRepo(db)}
}

// List returns every key, newest first.
func (s *AccessKeyStore) List(ctx context.Context) ([]access.Key, error) {
	rows, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	keys := make([]access.Key, 0, len(rows))
	for _, row := range rows {
		keys = append(keys, keyOf(row))
	}
	return keys, nil
}

// Get returns one key, and whether a live row carries the identifier.
func (s *AccessKeyStore) Get(ctx context.Context, id string) (access.Key, bool, error) {
	row, found, err := s.repo.Get(ctx, id)
	if err != nil || !found {
		return access.Key{}, false, err
	}
	return keyOf(row), true, nil
}

// Insert stores one key.
func (s *AccessKeyStore) Insert(ctx context.Context, key access.Key) error {
	return translateKeyError(s.repo.Insert(ctx, rowOf(key)))
}

// Update stores one key's name and expiry.
func (s *AccessKeyStore) Update(ctx context.Context, update access.KeyUpdate) error {
	return translateKeyError(s.repo.Update(ctx, update))
}

// Rotate replaces one key's digest and hint.
func (s *AccessKeyStore) Rotate(ctx context.Context, rotation access.KeyRotation) error {
	return translateKeyError(s.repo.Rotate(ctx, rotation))
}

// Revoke retires one key.
func (s *AccessKeyStore) Revoke(ctx context.Context, id string, nowMs int64) error {
	return translateKeyError(s.repo.Revoke(ctx, id, nowMs))
}

// Touch records that one key authenticated a request.
func (s *AccessKeyStore) Touch(ctx context.Context, id string, nowMs int64) error {
	return s.repo.Touch(ctx, id, nowMs)
}

// DeleteExpired removes expired operator keys.
func (s *AccessKeyStore) DeleteExpired(ctx context.Context, nowMs int64, ids []string) (int, error) {
	return s.repo.DeleteExpired(ctx, nowMs, ids)
}

func rowOf(key access.Key) AccessKeyRow {
	return AccessKeyRow{
		ID: key.ID, Name: key.Name, Kind: key.Kind, Client: key.Client,
		TokenDigest: key.TokenDigest, TokenHint: key.TokenHint, Generation: key.Generation,
		ExpiresAtMs: key.ExpiresAtMs, RevokedAtMs: key.RevokedAtMs,
		CreatedAtMs: key.CreatedAtMs, UpdatedAtMs: key.UpdatedAtMs,
		LastUsedAtMs: key.LastUsedAtMs, Owner: key.Owner,
	}
}

func keyOf(row AccessKeyRow) access.Key {
	return access.Key{
		ID: row.ID, Name: row.Name, Kind: row.Kind, Client: row.Client, Owner: row.Owner,
		TokenDigest: row.TokenDigest, TokenHint: row.TokenHint, Generation: row.Generation,
		ExpiresAtMs: row.ExpiresAtMs, RevokedAtMs: row.RevokedAtMs,
		CreatedAtMs: row.CreatedAtMs, UpdatedAtMs: row.UpdatedAtMs, LastUsedAtMs: row.LastUsedAtMs,
	}
}

func translateKeyError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrAccessKeyNotFound):
		return access.ErrKeyNotFound
	case errors.Is(err, ErrAccessKeyNameTaken):
		return access.ErrKeyNameTaken
	default:
		return err
	}
}
