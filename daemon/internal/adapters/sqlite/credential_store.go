// Credential store: the account pool rows and their status.
package sqlite

import (
	"context"

	"github.com/jonaskahn/relo/internal/account"
)

// CredentialStore adapts the credential table to the pool repository
// contract, so a use case never touches a credential row.
type CredentialStore struct {
	repo *CredentialRepo
}

// NewCredentialStore returns a credential store over the given database.
func NewCredentialStore(db *DB) *CredentialStore {
	return &CredentialStore{repo: NewCredentialRepo(db)}
}

// List returns every live credential, ordered by insertion.
func (s *CredentialStore) List(ctx context.Context) ([]account.PoolEntry, error) {
	rows, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	entries := make([]account.PoolEntry, 0, len(rows))
	for _, row := range rows {
		entries = append(entries, EntryOf(row))
	}
	return entries, nil
}

// Insert stores one credential.
func (s *CredentialStore) Insert(ctx context.Context, entry account.PoolEntry) error {
	return s.repo.Insert(ctx, credentialRowOf(entry))
}

// SetStatus stores one credential's status.
func (s *CredentialStore) SetStatus(ctx context.Context, id, status string) error {
	return s.repo.SetStatus(ctx, id, status)
}

// SetPriority stores one credential's rank.
func (s *CredentialStore) SetPriority(ctx context.Context, id string, priority int) error {
	return s.repo.SetPriority(ctx, id, priority)
}

// Remove tombstones one credential.
func (s *CredentialStore) Remove(ctx context.Context, id string) error {
	return s.repo.SoftDelete(ctx, id)
}

// EntryOf renders one stored credential as the pool entry a use case reads.
func EntryOf(row CredentialRow) account.PoolEntry {
	return account.PoolEntry(row)
}

func credentialRowOf(entry account.PoolEntry) CredentialRow {
	return CredentialRow(entry)
}
