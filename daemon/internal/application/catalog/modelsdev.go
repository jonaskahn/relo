// models.dev use cases: search and priced state.
package catalog

import (
	"context"
	"strings"

	"github.com/jonaskahn/relo/internal/catalog"
)

// PricesReport is what a price update did, which an operator reads to know
// whether anything changed at all.
type PricesReport struct {
	FetchedAtMs   int64
	Providers     int
	ModelsMatched int
	PricesChanged int
	// ModelsUnmatched is how many models the downloaded catalog does not
	// describe. They keep the facts already saved, so the count is what tells
	// an operator that some of what they read has an older date than the fetch.
	ModelsUnmatched int
	ModelsDev       catalog.ModelsDevState
}

// ModelsDevSearchHit is one model a search over the saved models.dev copy
// found, in the shape the console shows in a Priced as field.
type ModelsDevSearchHit struct {
	Ref           string
	ProviderID    string
	ProviderName  string
	ModelID       string
	Name          string
	ContextWindow *int64
	Prices        Prices
}

// SearchModelsDev finds models in the copy Relo already holds. It never asks
// the network, so a search stays fast and works while the network is down.
func (s *Service) SearchModelsDev(ctx context.Context, query string, limit int) ([]ModelsDevSearchHit, catalog.ModelsDevState) {
	idx, _ := s.modelsDev.Cached()
	state, _ := s.ModelsDevState(ctx)
	if idx == nil {
		return []ModelsDevSearchHit{}, state
	}
	hits := idx.Search(query, limit)
	rows := make([]ModelsDevSearchHit, 0, len(hits))
	for _, hit := range hits {
		rows = append(rows, ModelsDevSearchHit{
			Ref: hit.Ref, ProviderID: hit.ProviderID, ProviderName: hit.ProviderName,
			ModelID: hit.ModelID, Name: hit.Name, ContextWindow: hit.ContextWindow,
			Prices: toPrices(hit.Prices),
		})
	}
	return rows, state
}

func (s *Service) priceModel(ref string) (catalog.ModelsDevModel, string, string, bool) {
	idx, _ := s.modelsDev.Cached()
	if idx == nil {
		return catalog.ModelsDevModel{}, "", "", false
	}
	providerID, modelID, found := splitRef(ref)
	if !found {
		return catalog.ModelsDevModel{}, "", "", false
	}
	p, ok := idx.Providers[providerID]
	if !ok {
		return catalog.ModelsDevModel{}, "", "", false
	}
	m, ok := p.Models[modelID]
	if !ok {
		return catalog.ModelsDevModel{}, "", "", false
	}
	return m, providerID, modelID, true
}

func splitRef(ref string) (string, string, bool) {
	trimmed := strings.TrimSpace(ref)
	idx := strings.Index(trimmed, "/")
	if idx <= 0 || idx == len(trimmed)-1 {
		return "", "", false
	}
	return trimmed[:idx], trimmed[idx+1:], true
}

func modelsDevLayer(providerID, modelsDevProviderID, modelID string, idx *catalog.ModelsDevIndex, manual catalog.ModelsDevModel, manualRef string) (catalog.FactsRecord, string, bool) {
	if manualRef != "" {
		return factsFor(providerID, modelID, manual), string(catalog.ModelsDevMatchManual), true
	}
	if idx == nil {
		return catalog.FactsRecord{}, "", false
	}
	if modelsDevProviderID == "" {
		modelsDevProviderID = providerID
	}
	matched, matchType, found := idx.Match(modelsDevProviderID, modelID)
	if !found {
		return catalog.FactsRecord{}, "", false
	}
	return factsFor(providerID, modelID, matched), string(matchType), true
}

func modelsDevProviderIDOf(declared, providerID string) string {
	if declared != "" {
		return declared
	}
	return providerID
}

func factsFor(providerID, modelID string, m catalog.ModelsDevModel) catalog.FactsRecord {
	return catalog.FactsRecord{
		ProviderID: providerID, ModelID: modelID, Layer: "modelsdev",
		Name: stringPtr(m.Name), Description: stringPtr(m.Description),
		Family: stringPtr(m.Family), Category: stringPtr(m.Category),
		ContextWindow: m.ContextWindow, MaxInput: m.MaxInput, MaxOutput: m.MaxOutput,
		Status: stringPtr(m.Status), ReleaseDate: stringPtr(m.ReleaseDate),
		SupportsTools: m.SupportsTools, SupportsReasoning: m.SupportsReasoning,
		SupportsVision: m.SupportsVision, ReasoningEfforts: m.ReasoningEfforts,
		ReasoningToggle: statedBool(m.ReasoningToggle), ReasoningBudget: statedBool(m.ReasoningBudget),
		ReasoningBudgetMin: m.ReasoningBudgetMin, ReasoningBudgetMax: m.ReasoningBudgetMax,
		Prices: toPrices(m.Prices).row(),
	}
}

func statedBool(on bool) *bool {
	if !on {
		return nil
	}
	value := true
	return &value
}

// ModelsDevState reports the freshness of the copy Relo holds. It reads what
// is already saved and never fetches, so a page can show an age without
// waiting on a network.
func (s *Service) ModelsDevState(ctx context.Context) (catalog.ModelsDevState, error) {
	idx, err := s.modelsDev.Cached()
	state := catalog.ModelsDevState{SourceURL: catalog.ModelsDevDefaultURL}
	if idx != nil {
		state = idx.State
		state.Available = true
	}
	if s.store != nil {
		if row, found, readErr := s.store.GetModelsDevState(ctx); readErr == nil && found {
			state = mergeModelsDevRow(state, row)
		}
	}
	if !state.Available {
		state.Stale = false
	}
	return state, err
}

func mergeModelsDevRow(state catalog.ModelsDevState, row catalog.ModelsDevStateRecord) catalog.ModelsDevState {
	if row.FetchedAtMs > state.FetchedAtMs {
		state.FetchedAtMs = row.FetchedAtMs
	}
	if state.SourceURL == "" {
		state.SourceURL = row.SourceURL
	}
	if state.LastAttemptAtMs < row.LastAttemptAtMs {
		state.LastAttemptAtMs = row.LastAttemptAtMs
	}
	if state.LastError == "" {
		state.LastError = row.LastError
	}
	if !state.Available {
		state.ProvidersCount = row.ProvidersCount
		state.ModelsCount = row.ModelsCount
	}
	state.Stale = row.LastError != ""
	return state
}

// RefreshModelsDev downloads the newest models.dev catalog and rewrites every
// model's models.dev layer from it. An override is never touched, a model an
// operator priced by hand keeps the reference they chose, and a model the new
// catalog does not describe keeps the facts it already had.
func (s *Service) RefreshModelsDev(ctx context.Context) (PricesReport, error) {
	idx, err := s.modelsDev.Get(ctx, catalog.FetchForce)
	state, _ := s.ModelsDevState(ctx)
	if err != nil && (idx == nil || len(idx.Providers) == 0) {
		return PricesReport{ModelsDev: state}, err
	}
	matched, changed, unmatched, err := s.rewriteModelsDevLayer(ctx, idx)
	if err != nil {
		return PricesReport{ModelsDev: state}, err
	}
	return PricesReport{
		FetchedAtMs: idx.State.FetchedAtMs, Providers: len(idx.Providers),
		ModelsMatched: matched, PricesChanged: changed, ModelsUnmatched: unmatched,
		ModelsDev: state,
	}, nil
}

func (s *Service) rewriteModelsDevLayer(ctx context.Context, idx *catalog.ModelsDevIndex) (int, int, int, error) {
	snap, err := s.snapshot()
	if err != nil {
		return 0, 0, 0, err
	}
	rewritten := rewriteProviderLayers(s, snap, idx)
	if err := s.store.SaveModelsDevFacts(ctx, rewritten.facts, modelsDevStateRow(s, ctx, idx)); err != nil {
		return rewritten.matched, rewritten.changed, rewritten.unmatched, err
	}
	_ = s.reload(ctx)
	return rewritten.matched, rewritten.changed, rewritten.unmatched, nil
}

type rewrittenLayer struct {
	facts     []catalog.FactsRecord
	matched   int
	changed   int
	unmatched int
}

func rewriteProviderLayers(s *Service, snap *catalog.Snapshot, idx *catalog.ModelsDevIndex) rewrittenLayer {
	out := rewrittenLayer{facts: make([]catalog.FactsRecord, 0, 256)}
	for _, host := range snap.Providers {
		for _, model := range snap.Models(host.ID) {
			out.rewriteModel(s, host, model, idx)
		}
	}
	return out
}

func (r *rewrittenLayer) rewriteModel(s *Service, host catalog.Provider, model catalog.Model, idx *catalog.ModelsDevIndex) {
	fact, _, found := s.layerFor(host, model, idx)
	if !found {
		// The catalog describes no such model, so whatever is already
		// saved stays where it is and keeps its own date rather than
		// being replaced by nothing.
		if carriesModelsDevFacts(model) {
			r.unmatched++
		}
		return
	}
	r.matched++
	if !samePrices(toPrices(model.PricesModelsDev).row(), fact.Prices) {
		r.changed++
	}
	r.facts = append(r.facts, fact)
}

func modelsDevStateRow(s *Service, ctx context.Context, idx *catalog.ModelsDevIndex) catalog.ModelsDevStateRecord {
	state, _ := s.ModelsDevState(ctx)
	return catalog.ModelsDevStateRecord{
		SourceURL: state.SourceURL, FetchedAtMs: idx.State.FetchedAtMs,
		ETag: idx.State.ETag, LastModified: idx.State.LastModified,
		ProvidersCount: len(idx.Providers), ModelsCount: idx.TotalModels(),
		LastAttemptAtMs: idx.State.LastAttemptAtMs, LastError: idx.State.LastError,
	}
}

func carriesModelsDevFacts(model catalog.Model) bool {
	if model.FactsModelsDev != (catalog.Facts{}) {
		return true
	}
	return model.PricesModelsDev.Known()
}

func (s *Service) layerFor(host catalog.Provider, model catalog.Model, idx *catalog.ModelsDevIndex) (catalog.FactsRecord, string, bool) {
	return s.layerForRef(host.ID, modelsDevProviderIDOf(host.ModelsDevProviderID, host.ID), model.ID, model.Match, model.ModelsDevRef, idx)
}

func (s *Service) layerForRef(providerID, modelsDevProviderID, modelID, match, ref string, idx *catalog.ModelsDevIndex) (catalog.FactsRecord, string, bool) {
	if match == string(catalog.ModelsDevMatchManual) && ref != "" {
		declared, _, _, found := s.priceModel(ref)
		if !found {
			return catalog.FactsRecord{}, "", false
		}
		return modelsDevLayer(providerID, modelsDevProviderID, modelID, idx, declared, ref)
	}
	return modelsDevLayer(providerID, modelsDevProviderID, modelID, idx, catalog.ModelsDevModel{}, "")
}

func samePrices(before catalog.PriceRecord, after catalog.PriceRecord) bool {
	return sameInt(before.Input, after.Input) && sameInt(before.Output, after.Output) &&
		sameInt(before.CacheRead, after.CacheRead) && sameInt(before.CacheWrite, after.CacheWrite) &&
		sameInt(before.ExtThreshold, after.ExtThreshold) && sameInt(before.ExtInput, after.ExtInput) &&
		sameInt(before.ExtOutput, after.ExtOutput) && sameInt(before.ExtCacheRead, after.ExtCacheRead) &&
		sameInt(before.ExtCacheWrite, after.ExtCacheWrite)
}

func sameInt(left, right *int64) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}
