package testkit

import (
	"context"
	"sync"

	"github.com/jonaskahn/relo/internal/account"
	"github.com/jonaskahn/relo/internal/adapters/secrets"
)

// CredentialRepo returns an in-memory credential repository seeded with
// the given entries.
func CredentialRepo(entries ...account.PoolEntry) account.CredentialRepository {
	return &memoryRepo{entries: entries}
}

type memoryRepo struct {
	entries []account.PoolEntry
}

func (r *memoryRepo) List(context.Context) ([]account.PoolEntry, error) {
	return r.entries, nil
}

func (r *memoryRepo) Insert(_ context.Context, entry account.PoolEntry) error {
	r.entries = append(r.entries, entry)
	return nil
}

func (r *memoryRepo) SetStatus(_ context.Context, id string, status string) error {
	for index := range r.entries {
		if r.entries[index].ID == id {
			r.entries[index].Status = status
			return nil
		}
	}
	return nil
}

func (r *memoryRepo) SetPriority(_ context.Context, id string, priority int) error {
	for index := range r.entries {
		if r.entries[index].ID == id {
			r.entries[index].Priority = priority
			return nil
		}
	}
	return nil
}

func (r *memoryRepo) Remove(_ context.Context, id string) error {
	kept := r.entries[:0]
	for _, entry := range r.entries {
		if entry.ID != id {
			kept = append(kept, entry)
		}
	}
	r.entries = kept
	return nil
}

// SecretStore returns an in-memory secret store seeded with the given
// reference values.
func SecretStore(values map[string]string) secrets.SecretStore {
	return &memorySecrets{values: values}
}

type memorySecrets struct {
	mu     sync.Mutex
	values map[string]string
}

func (s *memorySecrets) Set(ref string, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[ref] = value
	return nil
}

func (s *memorySecrets) Get(ref string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, found := s.values[ref]
	if !found {
		return "", secrets.ErrNotFound
	}
	return value, nil
}

func (s *memorySecrets) Delete(ref string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, found := s.values[ref]; !found {
		return secrets.ErrNotFound
	}
	delete(s.values, ref)
	return nil
}

func (s *memorySecrets) Mode() secrets.SecretMode {
	return secrets.ModeNone
}
