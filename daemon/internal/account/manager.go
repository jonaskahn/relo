// Credential pools: loading accounts and selecting one per request.
package account

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/jonaskahn/relo/internal/clock"
)

// CredentialRepository persists pool entries. The composition root passes
// the storage implementation, so this package never opens the database.
type CredentialRepository interface {
	List(ctx context.Context) ([]PoolEntry, error)
	Insert(ctx context.Context, entry PoolEntry) error
	SetStatus(ctx context.Context, id string, status string) error
	SetLabel(ctx context.Context, id string, label string) error
	SetPriority(ctx context.Context, id string, priority int) error
	Remove(ctx context.Context, id string) error
}

// ModelRepository persists what Relo has observed about which models one
// credential can serve. It is optional: a manager built without one keeps
// every account permissive.
type ModelRepository interface {
	ListModelAccess(ctx context.Context) ([]ModelObservation, []RosterObservation, error)
	SetRoster(ctx context.Context, credentialID, source string, modelIDs []string, observedAtMs int64) error
	RecordModel(ctx context.Context, observation ModelObservation) error
	ForgetCredential(ctx context.Context, credentialID string) error
}

// ManagerOptions tunes the pools one manager creates.
type ManagerOptions struct {
	Clock    clock.Clock
	Pins     *PinCache
	Strategy PoolStrategy
	// Models is the account-level model index every pool filters through. A
	// nil index is created, so a manager always has one.
	Models *ModelIndex
	// ModelRepo persists the index, and is optional.
	ModelRepo ModelRepository
	// FailoverBackoff is the escalating waits a refused credential walks
	// through, one step per consecutive failure. No steps keeps no denylist:
	// a refused credential is immediately reusable.
	FailoverBackoff []time.Duration
}

// Manager keeps one pool per provider and owns every mutation of them.
type Manager struct {
	mu      sync.RWMutex
	pools   map[string]*Pool
	repo    CredentialRepository
	secrets SecretReader
	index   *ModelIndex
	models  ModelRepository
	options Options
}

// NewManager returns a manager backed by the given repository and store,
// walking the shipped failover ladder for refused credentials.
func NewManager(repo CredentialRepository, secrets SecretReader) *Manager {
	return NewManagerWithOptions(repo, secrets, ManagerOptions{FailoverBackoff: DefaultFailoverBackoff()})
}

// NewManagerWithOptions returns a manager whose pools share the given
// clock, pin cache, and strategy.
func NewManagerWithOptions(repo CredentialRepository, secrets SecretReader, options ManagerOptions) *Manager {
	index := options.Models
	if index == nil {
		index = NewModelIndex(options.Clock)
	}
	settings := Options{
		Strategy: options.Strategy, Clock: options.Clock, Pins: options.Pins,
		Models: index, FailoverBackoff: append([]time.Duration(nil), options.FailoverBackoff...),
	}
	if settings.Clock == nil {
		settings.Clock = clock.New()
	}
	if settings.Pins == nil {
		settings.Pins = NewPinCache(0, 0, settings.Clock)
	}
	return &Manager{
		pools: map[string]*Pool{}, repo: repo, secrets: secrets,
		index: index, models: options.ModelRepo, options: settings,
	}
}

// ModelIndex returns the account-level model index this manager keeps.
func (m *Manager) ModelIndex() *ModelIndex {
	return m.index
}

// SetRoster records one account's own model roster, in the store first so a
// failure leaves the in-memory index unchanged.
func (m *Manager) SetRoster(ctx context.Context, credentialID string, modelIDs []string, source string) error {
	if m.models != nil {
		if err := m.models.SetRoster(ctx, credentialID, source, modelIDs, m.options.Clock.Now().UnixMilli()); err != nil {
			return err
		}
	}
	m.index.SetRoster(credentialID, modelIDs, source)
	return nil
}

// LearnUnavailable records that an upstream refused one model for one
// account, which keeps the next request away from it without taking the
// account out of rotation.
func (m *Manager) LearnUnavailable(ctx context.Context, credentialID, modelID string) error {
	return m.recordModel(ctx, credentialID, modelID, false)
}

// LearnAvailable records that an account served a model its stored roster did
// not list, so a stale listing stops excluding it.
func (m *Manager) LearnAvailable(ctx context.Context, credentialID, modelID string) error {
	return m.recordModel(ctx, credentialID, modelID, true)
}

func (m *Manager) recordModel(ctx context.Context, credentialID, modelID string, available bool) error {
	if credentialID == "" || modelID == "" {
		return nil
	}
	if m.models != nil {
		observation := ModelObservation{
			CredentialID: credentialID, ModelID: modelID, Source: ModelSourceLearned,
			Available: available, ObservedAtMs: m.options.Clock.Now().UnixMilli(),
		}
		if err := m.models.RecordModel(ctx, observation); err != nil {
			return err
		}
	}
	if available {
		m.index.LearnAvailable(credentialID, modelID)
		return nil
	}
	m.index.LearnUnavailable(credentialID, modelID)
	return nil
}

// Serves reports whether an account that could take a request right now can
// serve a model named by any of its identifiers.
func (m *Manager) Serves(providerID string, modelIDs []string) bool {
	serving, _ := m.Coverage(providerID, modelIDs)
	return serving > 0
}

// Coverage counts the credentials of one provider that could take a request
// now, and how many of them can serve the model.
func (m *Manager) Coverage(providerID string, modelIDs []string) (serving, active int) {
	for _, entry := range m.GetPool(providerID).ready() {
		active++
		if m.index == nil || m.index.CanServe(entry.ID, modelIDs) {
			serving++
		}
	}
	return serving, active
}

// RosterKnown reports whether one account's own model list has been read.
func (m *Manager) RosterKnown(credentialID string) bool {
	return m.index.RosterKnown(credentialID)
}

// Roster returns one account's roster marker and the models it serves.
func (m *Manager) Roster(credentialID string) (RosterObservation, []string) {
	observation, _ := m.index.Roster(credentialID)
	return observation, m.index.RosterModels(credentialID)
}

// GetPool returns the pool of a provider, creating an empty one when the
// provider has no credentials yet.
func (m *Manager) GetPool(providerID string) *Pool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.poolLocked(providerID)
}

// Active reports how many credentials of one provider are in rotation,
// whatever their breakers say, which is what tells a configured provider
// from one an operator never set up.
func (m *Manager) Active(providerID string) int {
	count := 0
	for _, entry := range m.GetPool(providerID).Entries() {
		if entry.Status == StatusActive {
			count++
		}
	}
	return count
}

// Available reports whether a provider has at least one credential that can
// take a request right now, which is what tells a cooling-down provider from
// an unconfigured one.
func (m *Manager) Available(providerID string) bool {
	return len(m.GetPool(providerID).ready()) > 0
}

// CooldownUntil is when every active account of a provider may take a
// request again. A zero time means the provider is not cooling down.
func (m *Manager) CooldownUntil(providerID string) time.Time {
	return m.GetPool(providerID).CooldownUntil()
}

// SetFailoverBackoff changes the escalating waits a refused credential
// walks through, for every pool this manager owns. No steps keeps no
// denylist: open breakers close at once, so every credential is reusable
// again.
func (m *Manager) SetFailoverBackoff(steps []time.Duration) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	m.options.FailoverBackoff = append([]time.Duration(nil), steps...)
	for _, pool := range m.pools {
		pool.SetFailoverBackoff(steps)
	}
}

// CoolingDown names a provider whose accounts are all waiting out a
// rate-limit backoff.
func CoolingDown(providerID string, until time.Time) string {
	return fmt.Sprintf("%s: every account is cooling down after upstream rate limiting until %s",
		providerID, until.UTC().Format("15:04 MST"))
}

// Health reports the breaker state of one credential inside one provider's
// pool. A provider without a pool has nothing in rotation, which is a
// closed breaker with no failures.
func (m *Manager) Health(providerID, id string) BreakerSnapshot {
	m.mu.RLock()
	pool, ok := m.pools[providerID]
	m.mu.RUnlock()
	if !ok {
		return BreakerSnapshot{State: BreakerClosed}
	}
	return pool.Health(id)
}

// MarkNeedsReauth takes a credential out of rotation until its owner logs in
// again, which is what an upstream refusal of the token means.
func (m *Manager) MarkNeedsReauth(ctx context.Context, providerID, id string) error {
	return m.changeStatus(ctx, providerID, id, StatusNeedsReauth)
}

// LoadFromDB replaces every pool with the credentials currently stored.
func (m *Manager) LoadFromDB(ctx context.Context) error {
	entries, err := m.repo.List(ctx)
	if err != nil {
		return fmt.Errorf("load credentials: %w", err)
	}
	if m.models != nil {
		observations, rosters, err := m.models.ListModelAccess(ctx)
		if err != nil {
			return fmt.Errorf("load account models: %w", err)
		}
		m.index.Load(observations, rosters)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pools = map[string]*Pool{}
	for _, entry := range entries {
		m.poolLocked(entry.ProviderID).Add(entry)
	}
	return nil
}

// AddCredential stores a credential and adds it to its provider's pool.
func (m *Manager) AddCredential(ctx context.Context, entry PoolEntry) error {
	if err := m.repo.Insert(ctx, entry); err != nil {
		return err
	}
	m.GetPool(entry.ProviderID).Add(entry)
	return nil
}

// Adopt puts a credential that is already stored into rotation. Storing and
// rotating are two halves of one account, so a caller that wrote the row as
// part of a larger transaction adopts it here rather than inserting it twice.
func (m *Manager) Adopt(entry PoolEntry) {
	m.GetPool(entry.ProviderID).Add(entry)
}

// Pause stores a paused credential, takes it out of rotation, and unpins
// the conversations that ran on it.
func (m *Manager) Pause(ctx context.Context, providerID, id string) error {
	return m.changeStatus(ctx, providerID, id, StatusPaused)
}

// Resume stores an active credential and returns it to rotation.
func (m *Manager) Resume(ctx context.Context, providerID, id string) error {
	return m.changeStatus(ctx, providerID, id, StatusActive)
}

// SetLabel renames a credential and keeps the pool's copy of it in step,
// which is what a request reads when it names the account it ran on.
func (m *Manager) SetLabel(ctx context.Context, providerID, id, label string) error {
	if err := m.repo.SetLabel(ctx, id, label); err != nil {
		return err
	}
	pooled := m.GetPool(providerID)
	for _, entry := range pooled.Entries() {
		if entry.ID != id {
			continue
		}
		entry.Label = label
		pooled.Add(entry)
	}
	return nil
}

// SetPriority changes how the strategy ranks a credential.
func (m *Manager) SetPriority(ctx context.Context, providerID, id string, priority int) error {
	if err := m.repo.SetPriority(ctx, id, priority); err != nil {
		return err
	}
	pooled := m.GetPool(providerID)
	for _, entry := range pooled.Entries() {
		if entry.ID != id {
			continue
		}
		entry.Priority = priority
		pooled.Add(entry)
	}
	return nil
}

// Remove tombstones a credential and drops it from its pool.
func (m *Manager) Remove(ctx context.Context, providerID, id string) error {
	if err := m.repo.Remove(ctx, id); err != nil {
		return err
	}
	m.GetPool(providerID).Remove(id)
	if m.models != nil {
		if err := m.models.ForgetCredential(ctx, id); err != nil {
			return fmt.Errorf("forget account models: %w", err)
		}
	}
	m.index.Forget(id)
	return nil
}

// ResolveSecret returns the secret a pool entry points at.
func (m *Manager) ResolveSecret(entry PoolEntry) (string, error) {
	if m.secrets == nil {
		return "", ErrNoSecretStore
	}
	if entry.SecretRef == "" {
		return "", fmt.Errorf("%s: %w", entry.ID, ErrNoSecretRef)
	}
	value, err := m.secrets.Get(entry.SecretRef)
	if err != nil {
		return "", fmt.Errorf("resolve secret for credential %s: %w", entry.ID, err)
	}
	return value, nil
}

func (m *Manager) changeStatus(ctx context.Context, providerID, id, status string) error {
	if err := m.repo.SetStatus(ctx, id, status); err != nil {
		return err
	}
	pooled := m.GetPool(providerID)
	// The status is kept as it was asked for: an account that needs a new
	// sign-in is not the same thing as one an operator paused, and a console
	// reads the difference.
	return pooled.SetStatus(id, status)
}

func (m *Manager) poolLocked(providerID string) *Pool {
	existing, found := m.pools[providerID]
	if found {
		return existing
	}
	created := NewPoolWithOptions(providerID, m.options)
	m.pools[providerID] = created
	return created
}

// SecretReader resolves the secret one credential points at, so a pool reads
// a secret through the contract it declares rather than through a store of
// its own.
type SecretReader interface {
	Get(ref string) (string, error)
}
