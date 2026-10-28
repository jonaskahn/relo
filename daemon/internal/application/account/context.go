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
	ModelID        string
	Effective      *int64
	Override       *int64
	Inherited      *int64
	MaxInput       *int64
	MaxOutput      *int64
	ContextSource  string
	MaxInputSource string
	Listed         bool
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
	factsByModel := map[string]catalog.AccountFactsRecord{}
	if s.facts != nil {
		facts, err := s.facts.List(ctx, entry.ID)
		if err != nil {
			return AccountContext{}, err
		}
		for _, fact := range facts {
			overrides[fact.ModelID] = fact.ContextWindow
		}
		capabilities, err := s.facts.ListCapabilities(ctx, entry.ID)
		if err != nil {
			return AccountContext{}, err
		}
		for _, fact := range capabilities {
			if fact.Source != "manual" {
				factsByModel[fact.ModelID] = fact
			}
		}
	}
	return s.accountContextOf(snapshot, entry.ID, entry.ProviderID, overrides, factsByModel), nil
}

func (s *Service) accountContextOf(snapshot *catalog.Snapshot, credentialID, providerID string, overrides map[string]*int64, discovered map[string]catalog.AccountFactsRecord) AccountContext {
	described := AccountContext{CredentialID: credentialID, ProviderID: providerID, Models: []AccountContextModel{}}
	for _, model := range snapshot.Models(providerID) {
		override := overrides[model.ID]
		effective := effectiveAccountModel(model, override, discovered[model.ID])
		described.Models = append(described.Models, AccountContextModel{
			ModelID: model.ID, Effective: effective.ContextWindow, Override: override,
			Inherited: model.ContextWindow, MaxInput: effective.MaxInput,
			MaxOutput: effective.MaxOutput, ContextSource: effective.FactSources.ContextWindow,
			MaxInputSource: effective.FactSources.MaxInput,
			Listed:         s.accountServes(credentialID, model),
		})
	}
	return described
}

func effectiveAccountModel(model catalog.Model, manual *int64, discovered catalog.AccountFactsRecord) catalog.Model {
	rows := make([]catalog.AccountFactsRecord, 0, 2)
	if discovered.ModelID != "" {
		rows = append(rows, discovered)
	}
	if manual != nil {
		rows = append(rows, catalog.AccountFactsRecord{
			ProviderID: model.ProviderID, ModelID: model.ID, Source: "manual", ContextWindow: manual,
		})
	}
	if len(rows) == 0 {
		return model
	}
	if combined := catalog.CombineAccountFacts(rows, model); combined != nil {
		return catalog.ApplyAccountCapabilities(model, *combined)
	}
	return model
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
		model, found := snapshot.Model(entry.ProviderID, modelID)
		if !found {
			write.Skipped = append(write.Skipped, AccountContextSkip{ModelID: modelID, Reason: "the connection does not hold this model"})
			continue
		}
		if value != nil && model.ContextWindow != nil && model.FactSources.ContextWindow != "override" {
			write.Skipped = append(write.Skipped, AccountContextSkip{ModelID: modelID, Reason: "context_window is defined by " + model.FactSources.ContextWindow})
			continue
		}
		if value != nil && s.facts != nil {
			capabilities, err := s.facts.ListCapabilities(ctx, entry.ID)
			if err != nil {
				return AccountContextWrite{}, err
			}
			known := false
			for _, fact := range capabilities {
				if fact.ModelID == modelID && fact.Source != "manual" && fact.ContextWindow != nil {
					known = true
					break
				}
			}
			if known {
				write.Skipped = append(write.Skipped, AccountContextSkip{ModelID: modelID, Reason: "context_window is defined by the account roster"})
				continue
			}
		}
		if err := s.facts.Save(ctx, pool.ContextFact{
			CredentialID: entry.ID, ProviderID: entry.ProviderID, ModelID: modelID,
			ContextWindow: value, UpdatedAtMs: now,
		}); err != nil {
			return AccountContextWrite{}, err
		}
		write.Applied = append(write.Applied, modelID)
	}
	if s.catalog != nil {
		if err := s.catalog.Reload(ctx); err != nil {
			return AccountContextWrite{}, err
		}
	}
	if s.afterChange != nil {
		s.afterChange(ctx)
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
		for _, model := range snapshot.Models(host.ID) {
			view := snapshot.AdvertisedModel(model)
			if view.ContextWindow != nil {
				advertised[AdvertisedContextKey(host.ID, model.ID)] = view.ContextWindow
			}
		}
	}
	return advertised
}

// EffectiveModel resolves one credential's attempt-local model capabilities.
// The returned model is a copy; the shared catalog snapshot is never mutated.
func (s *Service) EffectiveModel(ctx context.Context, providerID, modelID, credentialID string) (catalog.Model, error) {
	snapshot, err := s.snapshot()
	if err != nil {
		return catalog.Model{}, err
	}
	model, found := snapshot.Model(providerID, modelID)
	if !found {
		return catalog.Model{}, ErrInvalidCatalogRow
	}
	if s.facts == nil || credentialID == "" {
		return model, nil
	}
	facts, err := s.facts.ListCapabilities(ctx, credentialID)
	if err != nil {
		return catalog.Model{}, err
	}
	rows := make([]catalog.AccountFactsRecord, 0, 2)
	for _, fact := range facts {
		if fact.ProviderID == providerID && fact.ModelID == modelID {
			rows = append(rows, fact)
		}
	}
	if len(rows) == 0 {
		return model, nil
	}
	combined := catalog.CombineAccountFacts(rows, model)
	if combined == nil {
		return model, nil
	}
	return catalog.ApplyAccountCapabilities(model, *combined), nil
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
