// Credential model observations: the rosters providers publish.
package sqlite

import (
	"context"

	"github.com/jonaskahn/relo/internal/account"
)

// CredentialModelStore adapts the account-model tables to the pool's model
// repository contract, so neither the pool nor a handler touches SQL.
type CredentialModelStore struct {
	repo *CredentialModelRepo
}

// NewCredentialModelStore returns a store over the given database.
func NewCredentialModelStore(db *DB) *CredentialModelStore {
	return &CredentialModelStore{repo: NewCredentialModelRepo(db)}
}

// ListModelAccess returns every stored observation and roster marker.
func (s *CredentialModelStore) ListModelAccess(ctx context.Context) ([]account.ModelObservation, []account.RosterObservation, error) {
	rows, rosters, err := s.repo.List(ctx)
	if err != nil {
		return nil, nil, err
	}
	observations := make([]account.ModelObservation, 0, len(rows))
	for _, row := range rows {
		observations = append(observations, account.ModelObservation{
			CredentialID: row.CredentialID, ModelID: row.ModelID, Source: row.Source,
			Available: row.Available, ObservedAtMs: row.ObservedAtMs,
		})
	}
	markers := make([]account.RosterObservation, 0, len(rosters))
	for _, row := range rosters {
		markers = append(markers, account.RosterObservation{
			CredentialID: row.CredentialID, Source: row.Source,
			ModelsCount: row.ModelsCount, ObservedAtMs: row.ObservedAtMs,
		})
	}
	return observations, markers, nil
}

// SetRoster replaces one account's listing.
func (s *CredentialModelStore) SetRoster(ctx context.Context, credentialID, source string, modelIDs []string, observedAtMs int64) error {
	return s.repo.SetRoster(ctx, credentialID, source, modelIDs, observedAtMs)
}

// RecordModel stores one learned observation.
func (s *CredentialModelStore) RecordModel(ctx context.Context, observation account.ModelObservation) error {
	return s.repo.Record(ctx, CredentialModelRow{
		CredentialID: observation.CredentialID, ModelID: observation.ModelID,
		Source: observation.Source, Available: observation.Available,
		ObservedAtMs: observation.ObservedAtMs,
	})
}

// ForgetCredential drops what one account was observed to serve.
func (s *CredentialModelStore) ForgetCredential(ctx context.Context, credentialID string) error {
	return s.repo.Forget(ctx, credentialID)
}
