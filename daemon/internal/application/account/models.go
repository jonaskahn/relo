// Account models: the roster reads and refreshes.
package account

import (
	"context"
	"fmt"

	pool "github.com/jonaskahn/relo/internal/account"
)

// AccountModels is what one account is known to serve.
type AccountModels struct {
	CredentialID string
	ProviderID   string
	Known        bool
	Source       string
	ObservedAtMs int64
	Models       []string
}

// AccountModels returns what one account is known to serve.
func (s *Service) AccountModels(ctx context.Context, credentialID string) (AccountModels, error) {
	entry, err := s.lookupExact(ctx, credentialID)
	if err != nil {
		return AccountModels{}, err
	}
	return s.describeAccountModels(entry.ID, entry.ProviderID), nil
}

// RefreshAccountModels reads one account's own model list again and stores what it publishes.
func (s *Service) RefreshAccountModels(ctx context.Context, credentialID string) (AccountModels, error) {
	entry, err := s.lookupExact(ctx, credentialID)
	if err != nil {
		return AccountModels{}, err
	}
	snapshot, err := s.snapshot()
	if err != nil {
		return AccountModels{}, err
	}
	host, found := snapshot.Provider(entry.ProviderID)
	if !found {
		return AccountModels{}, fmt.Errorf("%s: %w", entry.ProviderID, ErrProviderNotFound)
	}
	if s.discover == nil || !s.discover.PerAccount(host.ModelsFormat) {
		return AccountModels{}, fmt.Errorf("%s: %w", host.ID, ErrSharedRoster)
	}
	modelIDs, err := s.discover.List(ctx, host, entry.ID)
	if err != nil {
		return AccountModels{}, err
	}
	if err := s.pools.SetRoster(ctx, entry.ID, modelIDs, pool.ModelSourceListing); err != nil {
		return AccountModels{}, err
	}
	if err := s.discover.Refresh(ctx, entry.ProviderID); err != nil {
		s.logger.Warn("read the connection roster after an account roster", "provider", entry.ProviderID, "error", err)
	}
	return s.describeAccountModels(entry.ID, entry.ProviderID), nil
}

func (s *Service) describeAccountModels(credentialID, providerID string) AccountModels {
	described := AccountModels{CredentialID: credentialID, ProviderID: providerID, Models: []string{}}
	if s.pools == nil {
		return described
	}
	observation, models := s.pools.Roster(credentialID)
	if models != nil {
		described.Models = models
	}
	described.Known = observation.CredentialID != ""
	described.Source = observation.Source
	described.ObservedAtMs = observation.ObservedAtMs
	return described
}
