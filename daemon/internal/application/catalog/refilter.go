// Stored roster re-filter: applying new prefix rules without a listing.
package catalog

import (
	"context"

	"github.com/jonaskahn/relo/internal/account"
	"github.com/jonaskahn/relo/internal/catalog"
)

// RefilterProviderModels re-applies the connection's stored roster rules to
// the models it already listed, so a prefix change takes effect without
// asking the provider again.
func (s *Service) RefilterProviderModels(ctx context.Context, providerID string) (RefreshResult, error) {
	defer func() { _ = s.reload(ctx) }()
	host, t, mdIdx, mdState, existing, err := s.openRefreshHost(ctx, providerID)
	if err != nil {
		return RefreshResult{}, err
	}
	if host.ModelsFormat == catalog.ModelsNone {
		return RefreshResult{Status: RefreshSkipped, ModelsDev: mdState}, nil
	}
	listed := storedListing(existing)
	roster := buildRoster(t, rosterModeFor(t), listed, nil)
	assembly := assembleRefreshModels(roster, mdIdx, providerID, modelsDevIDOf(host), indexExistingModels(existing))
	var atMs int64
	if host.LastRefreshedAtMs != nil {
		atMs = *host.LastRefreshedAtMs
	} else {
		atMs = catalog.NowMs()
	}
	s.narrowAccountRosters(ctx, host, t)
	finished, err := s.finishRefresh(ctx, providerID, refreshFinish{
		listed: listed, assembly: assembly, existing: existing,
		atMs: atMs, mdState: mdState,
	})
	if err != nil {
		return RefreshResult{}, err
	}
	finished.Status = RefreshUpdated
	return finished, nil
}

func modelsDevIDOf(host catalog.Provider) string {
	if host.ModelsDevProviderID != "" {
		return host.ModelsDevProviderID
	}
	return host.ID
}

func storedListing(existing []catalog.Model) []catalog.Listed {
	listed := make([]catalog.Listed, 0, len(existing))
	for _, model := range existing {
		if model.ClonedFrom != "" {
			continue
		}
		entry := catalog.Listed{
			ID: model.ID, Name: model.Name,
			ContextWindow: model.ContextWindow, MaxOutput: model.MaxOutput,
			SupportsTools: model.SupportsTools, SupportsReasoning: model.SupportsReasoning,
			SupportsVision: model.SupportsVision, ReasoningEfforts: model.ReasoningEfforts,
		}
		if model.ReasoningToggle {
			stated := true
			entry.ReasoningToggle = &stated
		}
		listed = append(listed, entry)
	}
	return listed
}

func (s *Service) narrowAccountRosters(ctx context.Context, host catalog.Provider, t catalog.Template) {
	if s.pools == nil {
		return
	}
	for _, entry := range s.activeAccounts(host.ID) {
		s.narrowAccountRoster(ctx, host, t, entry)
	}
}

func (s *Service) narrowAccountRoster(ctx context.Context, host catalog.Provider, t catalog.Template, entry account.PoolEntry) {
	_, roster := s.pools.Roster(entry.ID)
	if len(roster) == 0 {
		return
	}
	kept := make([]string, 0, len(roster))
	for _, id := range roster {
		if id == "" {
			continue
		}
		if t.FilterModels != nil && !t.FilterModels(catalog.Listed{ID: id}) {
			continue
		}
		if !catalog.MatchesPrefix(id, t.Prefixes) {
			continue
		}
		kept = append(kept, id)
	}
	if err := s.pools.SetRoster(ctx, entry.ID, kept, account.ModelSourceListing); err != nil {
		s.logger.Warn("narrow an account roster", "credential", entry.ID, "error", err)
	}
	s.narrowRequestConfigs(ctx, host, t, entry.ID)
}

func (s *Service) narrowRequestConfigs(ctx context.Context, host catalog.Provider, t catalog.Template, credentialID string) {
	configs, err := s.store.ModelRequestConfigs(ctx, host.ID, credentialID)
	if err != nil || len(configs) == 0 {
		return
	}
	kept := make([]catalog.ModelRequestConfig, 0, len(configs))
	for _, config := range configs {
		if t.FilterModels != nil && !t.FilterModels(catalog.Listed{ID: config.ModelID}) {
			continue
		}
		if !catalog.MatchesPrefix(config.ModelID, t.Prefixes) {
			continue
		}
		kept = append(kept, config)
	}
	if err := s.store.SaveModelRequestConfigs(ctx, host.ID, credentialID, kept); err != nil {
		s.logger.Warn("narrow an account's request configs", "credential", credentialID, "error", err)
	}
}
