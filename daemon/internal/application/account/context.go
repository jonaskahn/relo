// Account context views: windows, overrides, and skips.
package account

import (
	"context"
	"fmt"
	"time"

	pool "github.com/jonaskahn/relo/internal/account"
	"github.com/jonaskahn/relo/internal/catalog"
)

const maxContextWindow = 100_000_000

// AccountContextModel is one model as one account reads it.
type AccountContextModel struct {
	ModelID   string
	Effective *int64
	Override  *int64
	Inherited *int64
	MaxInput  *int64
	Listed    bool
}

// AccountContext is one account's effective context windows.
type AccountContext struct {
	CredentialID string
	ProviderID   string
	Models       []AccountContextModel
}

// AccountContextSkip names a model a batch write left alone and why.
type AccountContextSkip struct {
	ModelID string
	Reason  string
}

// AccountContextWrite reports what a batch write changed.
type AccountContextWrite struct {
	Applied []string
	Skipped []AccountContextSkip
}

// AccountContext returns the effective context window of every model one account holds.
func (s *Service) AccountContext(ctx context.Context, credentialID string) (AccountContext, error) {
	entry, err := s.lookupExact(ctx, credentialID)
	if err != nil {
		return AccountContext{}, err
	}
	snapshot, err := s.snapshot()
	if err != nil {
		return AccountContext{}, err
	}
	overrides := map[string]*int64{}
	if s.facts != nil {
		facts, err := s.facts.List(ctx, entry.ID)
		if err != nil {
			return AccountContext{}, err
		}
		for _, fact := range facts {
			overrides[fact.ModelID] = fact.ContextWindow
		}
	}
	return s.accountContextOf(snapshot, entry.ID, entry.ProviderID, overrides), nil
}

func (s *Service) accountContextOf(snapshot *catalog.Snapshot, credentialID, providerID string, overrides map[string]*int64) AccountContext {
	described := AccountContext{CredentialID: credentialID, ProviderID: providerID, Models: []AccountContextModel{}}
	for _, model := range snapshot.Models(providerID) {
		override := overrides[model.ID]
		effective := model.ContextWindow
		if override != nil {
			effective = override
		}
		described.Models = append(described.Models, AccountContextModel{
			ModelID: model.ID, Effective: effective, Override: override,
			Inherited: model.ContextWindow, MaxInput: model.MaxInput,
			Listed: s.accountServes(credentialID, model),
		})
	}
	return described
}

// SetAccountModelsContext writes one context window on every named model of one account.
func (s *Service) SetAccountModelsContext(ctx context.Context, credentialID string, modelIDs []string, value *int64) (AccountContextWrite, error) {
	if value != nil && (*value < 1 || *value > maxContextWindow) {
		return AccountContextWrite{}, fmt.Errorf("context_window must be between 1 and %d: %w", maxContextWindow, ErrInvalidCatalogRow)
	}
	entry, err := s.lookupExact(ctx, credentialID)
	if err != nil {
		return AccountContextWrite{}, err
	}
	snapshot, err := s.snapshot()
	if err != nil {
		return AccountContextWrite{}, err
	}
	write := AccountContextWrite{Applied: []string{}, Skipped: []AccountContextSkip{}}
	now := time.Now().UnixMilli()
	for _, modelID := range modelIDs {
		_, found := snapshot.Model(entry.ProviderID, modelID)
		if !found {
			write.Skipped = append(write.Skipped, AccountContextSkip{ModelID: modelID, Reason: "the connection does not hold this model"})
			continue
		}
		if err := s.facts.Save(ctx, pool.ContextFact{
			CredentialID: entry.ID, ProviderID: entry.ProviderID, ModelID: modelID,
			ContextWindow: value, UpdatedAtMs: now,
		}); err != nil {
			return AccountContextWrite{}, err
		}
		write.Applied = append(write.Applied, modelID)
	}
	return write, nil
}

func (s *Service) accountServes(credentialID string, model catalog.Model) bool {
	if s.pools == nil {
		return true
	}
	index := s.pools.ModelIndex()
	if index == nil {
		return true
	}
	return index.CanServe(credentialID, modelIdentifiers(model))
}

// AdvertisedContexts returns, per provider model, the smallest effective
// context window among the accounts eligible to serve it.
func (s *Service) AdvertisedContexts(ctx context.Context) map[string]*int64 {
	advertised := map[string]*int64{}
	if s.pools == nil || s.facts == nil {
		return advertised
	}
	snapshot, err := s.snapshot()
	if err != nil {
		return advertised
	}
	for _, host := range snapshot.Providers {
		entries := activePoolEntries(s.pools, host.ID)
		if len(entries) == 0 {
			continue
		}
		facts, err := s.facts.ListProvider(ctx, host.ID)
		if err != nil {
			continue
		}
		advertiseHostContexts(advertised, s, snapshot, host, entries, providerOverrides(facts))
	}
	return advertised
}

func activePoolEntries(pools *pool.Manager, providerID string) []pool.PoolEntry {
	group := pools.GetPool(providerID)
	if group == nil {
		return nil
	}
	return group.Entries()
}

func providerOverrides(facts []pool.ContextFact) map[string]map[string]*int64 {
	overrides := map[string]map[string]*int64{}
	for _, fact := range facts {
		if overrides[fact.CredentialID] == nil {
			overrides[fact.CredentialID] = map[string]*int64{}
		}
		overrides[fact.CredentialID][fact.ModelID] = fact.ContextWindow
	}
	return overrides
}

func advertiseHostContexts(advertised map[string]*int64, s *Service, snapshot *catalog.Snapshot, host catalog.Provider, entries []pool.PoolEntry, overrides map[string]map[string]*int64) {
	for _, model := range snapshot.Models(host.ID) {
		if smallest, ok := smallestServedWindow(s, model, entries, overrides); ok {
			advertised[AdvertisedContextKey(host.ID, model.ID)] = smallest
		}
	}
}

func smallestServedWindow(s *Service, model catalog.Model, entries []pool.PoolEntry, overrides map[string]map[string]*int64) (*int64, bool) {
	smallest, any := model.ContextWindow, false
	for _, entry := range entries {
		if entry.Status != pool.StatusActive {
			continue
		}
		if !s.accountServes(entry.ID, model) {
			continue
		}
		window := model.ContextWindow
		if held, found := overrides[entry.ID][model.ID]; found {
			window = held
		}
		if window == nil {
			continue
		}
		if !any || *window < *smallest {
			smallest, any = window, true
		}
	}
	return smallest, any
}

// AdvertisedContextKey is the map key AdvertisedContexts uses.
func AdvertisedContextKey(providerID, modelID string) string {
	return providerID + "\x00" + modelID
}

func modelIdentifiers(model catalog.Model) []string {
	identifiers := make([]string, 0, 3)
	for _, id := range []string{model.ID, model.UpstreamID, model.ClonedFrom} {
		if id != "" {
			identifiers = append(identifiers, id)
		}
	}
	return identifiers
}
