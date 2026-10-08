// Credential pool: the candidates and strategy of one provider.
package account

import (
	"fmt"
	"sync"
	"time"

	"github.com/jonaskahn/relo/internal/clock"
)

// Options tunes one pool.
type Options struct {
	Strategy PoolStrategy
	Clock    clock.Clock
	Pins     *PinCache
	// Models answers which models one credential can serve, so a request is
	// never sent to an account whose own entitlement does not hold it. A nil
	// access keeps every credential eligible for every model.
	Models ModelAccess
	// FailoverBackoff is the escalating waits a refused credential walks
	// through, one step per consecutive failure. No steps keeps no denylist:
	// a refused credential is immediately reusable.
	FailoverBackoff []time.Duration
}

// Pool holds the credentials one provider can use.
type Pool struct {
	mu         sync.RWMutex
	providerID string
	entries    []PoolEntry
	strategy   PoolStrategy
	breakers   map[string]*Breaker
	pins       *PinCache
	models     ModelAccess
	clock      clock.Clock
	steps      []time.Duration
}

// NewPool creates an empty pool that picks credentials by quota headroom,
// the default strategy, and walks the shipped failover ladder.
func NewPool(providerID string) *Pool {
	return NewPoolWithOptions(providerID, Options{Strategy: &LeastLoadedStrategy{}, FailoverBackoff: DefaultFailoverBackoff()})
}

// NewPoolWithStrategy creates an empty pool with the given strategy, keeping
// the shipped failover ladder.
func NewPoolWithStrategy(providerID string, strategy PoolStrategy) *Pool {
	return NewPoolWithOptions(providerID, Options{Strategy: strategy, FailoverBackoff: DefaultFailoverBackoff()})
}

// NewPoolWithOptions creates an empty pool from explicit options.
func NewPoolWithOptions(providerID string, options Options) *Pool {
	settings := options
	if settings.Clock == nil {
		settings.Clock = clock.New()
	}
	if settings.Strategy == nil {
		settings.Strategy = &LeastLoadedStrategy{}
	}
	if settings.Pins == nil {
		settings.Pins = NewPinCache(0, 0, settings.Clock)
	}
	steps := settings.FailoverBackoff
	if len(steps) > 0 {
		steps = append([]time.Duration(nil), steps...)
	}
	return &Pool{
		providerID: providerID,
		strategy:   settings.Strategy,
		breakers:   map[string]*Breaker{},
		pins:       settings.Pins,
		models:     settings.Models,
		clock:      settings.Clock,
		steps:      steps,
	}
}

// ProviderID returns the provider this pool belongs to.
func (p *Pool) ProviderID() string {
	return p.providerID
}

// Pins returns the conversation pins this pool selects through.
func (p *Pool) Pins() *PinCache {
	return p.pins
}

// Models returns the account-level model access this pool filters with.
func (p *Pool) Models() ModelAccess {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.models
}

// Add inserts a credential, replacing any earlier entry with the same ID.
func (p *Pool) Add(entry PoolEntry) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for index := range p.entries {
		if p.entries[index].ID == entry.ID {
			p.entries[index] = entry
			return
		}
	}
	p.entries = append(p.entries, entry)
}

// Remove drops a credential from the pool and forgets everything the
// pool remembered about it.
func (p *Pool) Remove(id string) {
	p.mu.Lock()
	kept := p.entries[:0]
	for _, entry := range p.entries {
		if entry.ID != id {
			kept = append(kept, entry)
		}
	}
	p.entries = kept
	delete(p.breakers, id)
	p.mu.Unlock()
	p.pins.EvictByCredential(id)
}

// Entries returns a copy of the pool contents.
func (p *Pool) Entries() []PoolEntry {
	p.mu.RLock()
	defer p.mu.RUnlock()
	snapshot := make([]PoolEntry, len(p.entries))
	copy(snapshot, p.entries)
	return snapshot
}

// Entry returns one credential by identifier, whatever its status, which is
// how a caller reads a named account rather than the pool's own choice.
func (p *Pool) Entry(id string) (PoolEntry, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	for _, entry := range p.entries {
		if entry.ID == id {
			return entry, true
		}
	}
	return PoolEntry{}, false
}

// Select returns the credential that should serve a request: the pinned one
// when the conversation already has a healthy account, otherwise the one the
// strategy prefers. A credential the caller excluded — the account that just
// failed this request — is skipped while another account could serve it.
func (p *Pool) Select(ctx Selection) (PoolEntry, error) {
	usable := p.available()
	servable := p.servable(usable, ctx.Models)
	if len(servable) == 0 {
		if len(usable) > 0 {
			return PoolEntry{}, ErrNoModelAccount
		}
		return PoolEntry{}, ErrNoCredentials
	}
	candidates := withoutExcluded(servable, ctx.Exclude)
	if len(candidates) == 0 {
		// Every account that could serve the model is one this request has
		// already tried. Answering with one of them again is worse than a
		// fresh account and better than a local failure that would replace
		// the provider's own answer.
		candidates = servable
	}
	if pinned, found := p.pinned(ctx.ConversationID, candidates); found {
		return pinned, nil
	}
	selected, err := p.currentStrategy().Select(servable, ctx)
	if err != nil {
		return PoolEntry{}, err
	}
	p.pins.Pin(ctx.ConversationID, selected.ID)
	return selected, nil
}

func (p *Pool) servable(candidates []PoolEntry, modelIDs []string) []PoolEntry {
	access := p.Models()
	if access == nil || len(modelIDs) == 0 {
		return candidates
	}
	kept := make([]PoolEntry, 0, len(candidates))
	for _, candidate := range candidates {
		if access.CanServe(candidate.ID, modelIDs) {
			kept = append(kept, candidate)
		}
	}
	return kept
}

func withoutExcluded(entries []PoolEntry, exclude []string) []PoolEntry {
	if len(exclude) == 0 {
		return entries
	}
	kept := make([]PoolEntry, 0, len(entries))
	for _, entry := range entries {
		if !containsID(exclude, entry.ID) {
			kept = append(kept, entry)
		}
	}
	return kept
}

func containsID(ids []string, wanted string) bool {
	for _, id := range ids {
		if id == wanted {
			return true
		}
	}
	return false
}

// SetStrategy replaces the strategy the pool selects with.
func (p *Pool) SetStrategy(strategy PoolStrategy) {
	if strategy == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.strategy = strategy
}

// Pause takes a credential out of rotation and unpins its conversations.
func (p *Pool) Pause(id string) error {
	if err := p.setStatus(id, StatusPaused); err != nil {
		return err
	}
	p.pins.EvictByCredential(id)
	return nil
}

// Resume returns a paused credential to rotation.
func (p *Pool) Resume(id string) error {
	return p.setStatus(id, StatusActive)
}

// SetStatus records the exact state of one credential, which is what tells a
// paused account from one that has to be linked again. A credential that
// leaves rotation unpins the conversations that were using it.
func (p *Pool) SetStatus(id, status string) error {
	if err := p.setStatus(id, status); err != nil {
		return err
	}
	if status != StatusActive {
		p.pins.EvictByCredential(id)
	}
	return nil
}

// Breaker returns the breaker of one credential, creating a closed one on
// first use.
func (p *Pool) Breaker(id string) *Breaker {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.breakerLocked(id)
}

// Health reports the breaker state of one credential, which is closed until
// the credential has failed a request and trips it.
func (p *Pool) Health(id string) BreakerSnapshot {
	return p.Breaker(id).Snapshot()
}

// SetFailoverBackoff changes the escalating waits a refused credential
// walks through, for new and existing breakers. No steps keeps no denylist:
// open breakers close at once, so every credential is reusable again.
func (p *Pool) SetFailoverBackoff(steps []time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.steps = append([]time.Duration(nil), steps...)
	for _, breaker := range p.breakers {
		breaker.SetFailoverBackoff(steps)
	}
}

// RecordSuccess reports that a credential answered a request.
func (p *Pool) RecordSuccess(id string) {
	p.Breaker(id).RecordSuccess()
}

// RecordFailure reports an upstream answer and returns what it means for
// the credential.
func (p *Pool) RecordFailure(id string, status int, retryAfter time.Duration) CredentialVerdict {
	return p.Breaker(id).RecordFailure(status, retryAfter)
}

func (p *Pool) setStatus(id, status string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	for index := range p.entries {
		if p.entries[index].ID == id {
			p.entries[index].Status = status
			return nil
		}
	}
	return fmt.Errorf("%s: %w", id, ErrNotFound)
}

func (p *Pool) available() []PoolEntry {
	return p.activeWhere(func(breaker *Breaker) bool { return breaker.IsAvailable() })
}

func (p *Pool) ready() []PoolEntry {
	return p.activeWhere(func(breaker *Breaker) bool { return breaker.Allows() })
}

func (p *Pool) activeWhere(allows func(*Breaker) bool) []PoolEntry {
	p.mu.RLock()
	defer p.mu.RUnlock()
	active := make([]PoolEntry, 0, len(p.entries))
	for _, entry := range p.entries {
		if entry.Status != StatusActive {
			continue
		}
		if breaker, found := p.breakers[entry.ID]; found && !allows(breaker) {
			continue
		}
		active = append(active, entry)
	}
	return active
}

// CooldownUntil is when every active credential of this pool may take a
// request again. A zero time means at least one account can take a request
// now, or none is cooling down.
func (p *Pool) CooldownUntil() time.Time {
	if len(p.ready()) > 0 {
		return time.Time{}
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	var earliest time.Time
	for _, entry := range p.entries {
		if entry.Status != StatusActive {
			continue
		}
		breaker, found := p.breakers[entry.ID]
		if !found {
			continue
		}
		snapshot := breaker.Snapshot()
		if snapshot.State != BreakerOpen || snapshot.Until.IsZero() {
			continue
		}
		if earliest.IsZero() || snapshot.Until.Before(earliest) {
			earliest = snapshot.Until
		}
	}
	return earliest
}

func (p *Pool) pinned(conversationID string, candidates []PoolEntry) (PoolEntry, bool) {
	credentialID, found := p.pins.Get(conversationID)
	if !found {
		return PoolEntry{}, false
	}
	for _, candidate := range candidates {
		if candidate.ID == credentialID {
			return candidate, true
		}
	}
	p.pins.Evict(conversationID)
	return PoolEntry{}, false
}

func (p *Pool) currentStrategy() PoolStrategy {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.strategy
}

func (p *Pool) breakerLocked(id string) *Breaker {
	existing, found := p.breakers[id]
	if found {
		return existing
	}
	created := NewBreakerWithBackoff(p.clock, p.steps)
	p.breakers[id] = created
	return created
}

// StrategyName reports the name of the strategy this pool selects with.
func (p *Pool) StrategyName() string {
	return StrategyName(p.currentStrategy())
}
