// Account fact store: context overrides per credential.
package sqlite

import (
	"context"

	"github.com/jonaskahn/relo/internal/account"
	"github.com/jonaskahn/relo/internal/catalog"
)

// AccountFactStore adapts account context overrides to the account feature.
type AccountFactStore struct {
	repo *CatalogRepo
}

// NewAccountFactStore returns an account-context store over the given database.
func NewAccountFactStore(db *DB) *AccountFactStore {
	return &AccountFactStore{repo: NewCatalogRepo(db)}
}

// List returns one account's stored overrides.
func (s *AccountFactStore) List(ctx context.Context, credentialID string) ([]account.ContextFact, error) {
	rows, err := s.repo.ListAccountFacts(ctx, credentialID)
	if err != nil {
		return nil, err
	}
	return factsOf(rows), nil
}

// ListProvider returns every account override of one connection.
func (s *AccountFactStore) ListProvider(ctx context.Context, providerID string) ([]account.ContextFact, error) {
	rows, err := s.repo.ListProviderAccountFacts(ctx, providerID)
	if err != nil {
		return nil, err
	}
	return factsOf(rows), nil
}

// Save writes one account's context override for one model.
func (s *AccountFactStore) Save(ctx context.Context, fact account.ContextFact) error {
	return s.repo.SaveAccountModelFact(ctx, AccountFactRow{
		CredentialID: fact.CredentialID, ProviderID: fact.ProviderID, ModelID: fact.ModelID,
		ContextWindow: fact.ContextWindow, UpdatedAtMs: fact.UpdatedAtMs,
	})
}

// Delete removes one account's overrides.
func (s *AccountFactStore) Delete(ctx context.Context, credentialID string) error {
	if err := s.repo.DeleteAccountFacts(ctx, credentialID); err != nil {
		return err
	}
	return s.repo.DeleteAccountCapabilities(ctx, credentialID)
}

// ListCapabilities returns one account's discovered facts and manual context
// completions in the shared capability vocabulary.
func (s *AccountFactStore) ListCapabilities(ctx context.Context, credentialID string) ([]catalog.AccountFactsRecord, error) {
	all, err := s.repo.ListAccountCapabilities(ctx)
	if err != nil {
		return nil, err
	}
	facts := make([]catalog.AccountFactsRecord, 0, len(all))
	for _, fact := range all {
		if fact.CredentialID == credentialID {
			facts = append(facts, fact)
		}
	}
	return facts, nil
}

// ListProviderCapabilities returns every account capability fact of one
// connection.
func (s *AccountFactStore) ListProviderCapabilities(ctx context.Context, providerID string) ([]catalog.AccountFactsRecord, error) {
	all, err := s.repo.ListAccountCapabilities(ctx)
	if err != nil {
		return nil, err
	}
	facts := make([]catalog.AccountFactsRecord, 0, len(all))
	for _, fact := range all {
		if fact.ProviderID == providerID {
			facts = append(facts, fact)
		}
	}
	return facts, nil
}

func factsOf(rows []AccountFactRow) []account.ContextFact {
	facts := make([]account.ContextFact, 0, len(rows))
	for _, row := range rows {
		facts = append(facts, account.ContextFact{
			CredentialID: row.CredentialID, ProviderID: row.ProviderID, ModelID: row.ModelID,
			ContextWindow: row.ContextWindow, UpdatedAtMs: row.UpdatedAtMs,
		})
	}
	return facts
}
