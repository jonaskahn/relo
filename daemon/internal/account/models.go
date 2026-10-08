// Model index: the roster observations each credential reports.
package account

import (
	"sort"
	"sync"

	"github.com/jonaskahn/relo/internal/clock"
)

// The sources a model observation carries: the account's own listing, or
// what an upstream refusal or a served request taught Relo.
const (
	ModelSourceListing = "listing"
	ModelSourceLearned = "learned"
)

// ModelObservation is one stored pair of a credential and a model.
type ModelObservation struct {
	CredentialID string
	ModelID      string
	Source       string
	Available    bool
	ObservedAtMs int64
}

// RosterObservation marks one credential whose model roster is known, which
// is what makes a missing ModelObservation mean "not entitled" rather than
// "never observed".
type RosterObservation struct {
	CredentialID string
	Source       string
	ModelsCount  int
	ObservedAtMs int64
}

// ModelIndex remembers which models each credential can serve. A credential
// with no known roster is permissive: every model its connection lists is
// allowed, which is what keeps a provider that publishes one shared roster
// working unchanged.
type ModelIndex struct {
	mu      sync.RWMutex
	models  map[string]map[string]bool
	rosters map[string]RosterObservation
	clock   clock.Clock
}

// NewModelIndex returns an empty index.
func NewModelIndex(clk clock.Clock) *ModelIndex {
	if clk == nil {
		clk = clock.New()
	}
	return &ModelIndex{models: map[string]map[string]bool{}, rosters: map[string]RosterObservation{}, clock: clk}
}

// Load replaces the index with what the store holds.
func (i *ModelIndex) Load(models []ModelObservation, rosters []RosterObservation) {
	if i == nil {
		return
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	i.models = make(map[string]map[string]bool, len(models))
	i.rosters = make(map[string]RosterObservation, len(rosters))
	for _, roster := range rosters {
		i.rosters[roster.CredentialID] = roster
	}
	for _, model := range models {
		if i.models[model.CredentialID] == nil {
			i.models[model.CredentialID] = map[string]bool{}
		}
		i.models[model.CredentialID][model.ModelID] = model.Available
	}
}

// SetRoster records the models one account's own listing published, and the
// marker that says its roster is now known. Every earlier learned row is
// replaced, because a fresh listing is the authority on what the account
// holds: an entitlement that grew is picked up rather than remembered as a
// refusal forever.
func (i *ModelIndex) SetRoster(credentialID string, modelIDs []string, source string) {
	if i == nil || credentialID == "" {
		return
	}
	served := make(map[string]bool, len(modelIDs))
	for _, modelID := range modelIDs {
		if modelID != "" {
			served[modelID] = true
		}
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	i.models[credentialID] = served
	i.rosters[credentialID] = RosterObservation{
		CredentialID: credentialID, Source: source,
		ModelsCount: len(served), ObservedAtMs: i.clock.Now().UnixMilli(),
	}
}

// LearnUnavailable records that an upstream refused one model for one
// account, which is what keeps the next request away from it. The account
// stays in rotation: a model it may not use is not an account that failed.
func (i *ModelIndex) LearnUnavailable(credentialID, modelID string) {
	i.learn(credentialID, modelID, false)
}

// LearnAvailable records that an account served a model its known roster did
// not list, so a stale listing does not exclude an account the provider is
// happy to serve.
func (i *ModelIndex) LearnAvailable(credentialID, modelID string) {
	i.learn(credentialID, modelID, true)
}

func (i *ModelIndex) learn(credentialID, modelID string, available bool) {
	if i == nil || credentialID == "" || modelID == "" {
		return
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.models[credentialID] == nil {
		i.models[credentialID] = map[string]bool{}
	}
	if known, found := i.models[credentialID][modelID]; found && known == available {
		return
	}
	i.models[credentialID][modelID] = available
}

// CanServe reports whether one credential may serve a model, named by any of
// the identifiers a request may carry for it. An account whose roster is
// unknown serves anything.
func (i *ModelIndex) CanServe(credentialID string, modelIDs []string) bool {
	if i == nil {
		return true
	}
	i.mu.RLock()
	defer i.mu.RUnlock()
	served, known := i.models[credentialID]
	if !known {
		return true
	}
	for _, modelID := range modelIDs {
		if modelID == "" {
			continue
		}
		if available, found := served[modelID]; found && available {
			return true
		}
	}
	// Either a known roster does not list the model, or the only rows are
	// learned ones and none of them allows it.
	return false
}

// RosterKnown reports whether an account's own listing has been read.
func (i *ModelIndex) RosterKnown(credentialID string) bool {
	if i == nil {
		return false
	}
	i.mu.RLock()
	defer i.mu.RUnlock()
	_, found := i.rosters[credentialID]
	return found
}

// Roster returns the marker of one account's roster.
func (i *ModelIndex) Roster(credentialID string) (RosterObservation, bool) {
	if i == nil {
		return RosterObservation{}, false
	}
	i.mu.RLock()
	defer i.mu.RUnlock()
	observation, found := i.rosters[credentialID]
	return observation, found
}

// RosterModels returns every model one account is known to serve, in
// identifier order, with the learned exclusions left out.
func (i *ModelIndex) RosterModels(credentialID string) []string {
	if i == nil {
		return nil
	}
	i.mu.RLock()
	defer i.mu.RUnlock()
	modelIDs := make([]string, 0, len(i.models[credentialID]))
	for modelID, available := range i.models[credentialID] {
		if available {
			modelIDs = append(modelIDs, modelID)
		}
	}
	sort.Strings(modelIDs)
	return modelIDs
}

// Forget drops everything remembered about one credential, which is what
// removing it from a pool requires.
func (i *ModelIndex) Forget(credentialID string) {
	if i == nil {
		return
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	delete(i.models, credentialID)
	delete(i.rosters, credentialID)
}
