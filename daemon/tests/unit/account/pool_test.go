package pool_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/account"
	"github.com/jonaskahn/relo/internal/adapters/secrets"
)

func TestPoolSelection(t *testing.T) {
	t.Run("single credential always selected", func(t *testing.T) {
		pooled := account.NewPool("openai")
		pooled.Add(entry("a", account.StatusActive))
		for attempt := 0; attempt < 3; attempt++ {
			selected, err := pooled.Select(account.Selection{})
			if err != nil {
				t.Fatalf("Select() error = %v", err)
			}
			if selected.ID != "a" {
				t.Fatalf("Select() = %q, want a", selected.ID)
			}
		}
	})

	t.Run("two credentials alternate round-robin", func(t *testing.T) {
		pooled := account.NewPool("openai")
		pooled.Add(entry("a", account.StatusActive))
		pooled.Add(entry("b", account.StatusActive))
		order := make([]string, 0, 4)
		for attempt := 0; attempt < 4; attempt++ {
			selected, err := pooled.Select(account.Selection{})
			if err != nil {
				t.Fatalf("Select() error = %v", err)
			}
			order = append(order, selected.ID)
		}
		want := []string{"a", "b", "a", "b"}
		for index := range want {
			if order[index] != want[index] {
				t.Fatalf("selection order = %v, want %v", order, want)
			}
		}
	})

	t.Run("paused credential skipped", func(t *testing.T) {
		pooled := account.NewPool("openai")
		pooled.Add(entry("a", account.StatusActive))
		pooled.Add(entry("b", account.StatusPaused))
		for attempt := 0; attempt < 3; attempt++ {
			selected, err := pooled.Select(account.Selection{})
			if err != nil {
				t.Fatalf("Select() error = %v", err)
			}
			if selected.ID != "a" {
				t.Fatalf("Select() = %q, want the active credential", selected.ID)
			}
		}
	})

	t.Run("needs_reauth credential skipped", func(t *testing.T) {
		pooled := account.NewPool("openai")
		pooled.Add(entry("a", account.StatusNeedsReauth))
		pooled.Add(entry("b", account.StatusNeedsReauth))
		if _, err := pooled.Select(account.Selection{}); !errors.Is(err, account.ErrNoCredentials) {
			t.Fatalf("Select() error = %v, want %v", err, account.ErrNoCredentials)
		}
	})

	t.Run("all paused returns ErrNoCredentials", func(t *testing.T) {
		pooled := account.NewPool("openai")
		pooled.Add(entry("a", account.StatusActive))
		if err := pooled.Pause("a"); err != nil {
			t.Fatalf("Pause() error = %v", err)
		}
		if _, err := pooled.Select(account.Selection{}); !errors.Is(err, account.ErrNoCredentials) {
			t.Fatalf("Select() error = %v, want %v", err, account.ErrNoCredentials)
		}
		if err := pooled.Resume("a"); err != nil {
			t.Fatalf("Resume() error = %v", err)
		}
		if _, err := pooled.Select(account.Selection{}); err != nil {
			t.Fatalf("Select() after Resume error = %v", err)
		}
	})

	t.Run("empty pool returns ErrNoCredentials", func(t *testing.T) {
		if _, err := account.NewPool("openai").Select(account.Selection{}); !errors.Is(err, account.ErrNoCredentials) {
			t.Fatalf("Select() error = %v, want %v", err, account.ErrNoCredentials)
		}
	})

	t.Run("status changes report unknown credentials", func(t *testing.T) {
		pooled := account.NewPool("openai")
		if err := pooled.Pause("absent"); !errors.Is(err, account.ErrNotFound) {
			t.Fatalf("Pause() error = %v, want %v", err, account.ErrNotFound)
		}
		if err := pooled.Resume("absent"); !errors.Is(err, account.ErrNotFound) {
			t.Fatalf("Resume() error = %v, want %v", err, account.ErrNotFound)
		}
	})
}

func TestPoolContents(t *testing.T) {
	t.Run("add updates pool state", func(t *testing.T) {
		pooled := account.NewPool("openai")
		pooled.Add(entry("a", account.StatusActive))
		pooled.Add(entry("b", account.StatusActive))
		if got := len(pooled.Entries()); got != 2 {
			t.Fatalf("entries = %d, want 2", got)
		}
		replacement := entry("a", account.StatusPaused)
		replacement.Label = "renamed"
		pooled.Add(replacement)
		entries := pooled.Entries()
		if len(entries) != 2 {
			t.Fatalf("entries = %d after replacing, want 2", len(entries))
		}
		if entries[0].Label != "renamed" || entries[0].Status != account.StatusPaused {
			t.Fatalf("entry = %+v, want the replacement", entries[0])
		}
	})

	t.Run("remove updates pool state", func(t *testing.T) {
		pooled := account.NewPool("openai")
		pooled.Add(entry("a", account.StatusActive))
		pooled.Add(entry("b", account.StatusActive))
		pooled.Remove("a")
		entries := pooled.Entries()
		if len(entries) != 1 || entries[0].ID != "b" {
			t.Fatalf("entries = %+v, want only b", entries)
		}
		pooled.Remove("absent")
	})

	t.Run("entries returns a copy", func(t *testing.T) {
		pooled := account.NewPool("openai")
		pooled.Add(entry("a", account.StatusActive))
		snapshot := pooled.Entries()
		snapshot[0].Status = account.StatusPaused
		if got := pooled.Entries()[0].Status; got != account.StatusActive {
			t.Fatalf("pool status = %q, want the snapshot to be a copy", got)
		}
	})

	t.Run("provider id is reported", func(t *testing.T) {
		if got := account.NewPool("openai").ProviderID(); got != "openai" {
			t.Fatalf("ProviderID() = %q, want openai", got)
		}
	})

	t.Run("a custom strategy is used", func(t *testing.T) {
		pooled := account.NewPoolWithStrategy("openai", lastStrategy{})
		pooled.Add(entry("a", account.StatusActive))
		pooled.Add(entry("b", account.StatusActive))
		selected, err := pooled.Select(account.Selection{})
		if err != nil {
			t.Fatalf("Select() error = %v", err)
		}
		if selected.ID != "b" {
			t.Fatalf("Select() = %q, want the last candidate", selected.ID)
		}
	})

	t.Run("the strategy rejects an empty candidate list", func(t *testing.T) {
		if _, err := (&account.RoundRobinStrategy{}).Select(nil, account.Selection{}); !errors.Is(err, account.ErrNoCredentials) {
			t.Fatalf("Select() error = %v, want %v", err, account.ErrNoCredentials)
		}
	})

	t.Run("concurrent Select is safe", func(t *testing.T) {
		pooled := account.NewPool("openai")
		for index := 0; index < 4; index++ {
			pooled.Add(entry(string(rune('a'+index)), account.StatusActive))
		}
		var wg sync.WaitGroup
		for worker := 0; worker < 100; worker++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if _, err := pooled.Select(account.Selection{}); err != nil {
					t.Errorf("Select() error = %v", err)
				}
			}()
		}
		wg.Wait()
	})
}

func TestPoolFailover(t *testing.T) {
	t.Run("a repeated refusal skips the account on the next request", func(t *testing.T) {
		pooled := account.NewPool("openai")
		pooled.Add(entry("a", account.StatusActive))
		pooled.Add(entry("b", account.StatusActive))
		pooled.RecordFailure("a", 429, 0)
		pooled.RecordFailure("a", 429, 0)
		for attempt := 0; attempt < 3; attempt++ {
			selected, err := pooled.Select(account.Selection{})
			if err != nil {
				t.Fatalf("Select() error = %v", err)
			}
			if selected.ID != "b" {
				t.Fatalf("Select() = %q, want the healthy credential", selected.ID)
			}
		}
	})

	t.Run("a success returns the account to rotation", func(t *testing.T) {
		pooled := account.NewPool("openai")
		pooled.Add(entry("a", account.StatusActive))
		pooled.Add(entry("b", account.StatusActive))
		pooled.RecordFailure("a", 500, 0)
		pooled.RecordSuccess("a")
		seen := map[string]bool{}
		for attempt := 0; attempt < 4; attempt++ {
			selected, err := pooled.Select(account.Selection{})
			if err != nil {
				t.Fatalf("Select() error = %v", err)
			}
			seen[selected.ID] = true
		}
		if !seen["a"] {
			t.Fatal("a recovered credential never returned to rotation")
		}
	})
}

func TestManager(t *testing.T) {
	t.Run("LoadFromDB populates pools", func(t *testing.T) {
		repo := newFakeRepo()
		repo.entries = []account.PoolEntry{
			entry("a", account.StatusActive),
			{ID: "b", ProviderID: "anthropic", Kind: "api_key", Status: account.StatusActive},
		}
		manager := account.NewManager(repo, fakeSecrets{})
		if err := manager.LoadFromDB(context.Background()); err != nil {
			t.Fatalf("LoadFromDB() error = %v", err)
		}
		if got := len(manager.GetPool("openai").Entries()); got != 1 {
			t.Fatalf("openai pool = %d entries, want 1", got)
		}
		if got := len(manager.GetPool("anthropic").Entries()); got != 1 {
			t.Fatalf("anthropic pool = %d entries, want 1", got)
		}
	})

	t.Run("LoadFromDB reports repository failures", func(t *testing.T) {
		repo := newFakeRepo()
		repo.err = errors.New("database is gone")
		if err := account.NewManager(repo, fakeSecrets{}).LoadFromDB(context.Background()); err == nil {
			t.Fatal("LoadFromDB() error = nil, want the repository failure")
		}
	})

	t.Run("GetPool creates a pool on demand and reuses it", func(t *testing.T) {
		manager := account.NewManager(newFakeRepo(), fakeSecrets{})
		first := manager.GetPool("openai")
		if first != manager.GetPool("openai") {
			t.Fatal("GetPool() returned a different pool for the same provider")
		}
	})

	t.Run("AddCredential inserts and adds", func(t *testing.T) {
		repo := newFakeRepo()
		manager := account.NewManager(repo, fakeSecrets{})
		if err := manager.AddCredential(context.Background(), entry("a", account.StatusActive)); err != nil {
			t.Fatalf("AddCredential() error = %v", err)
		}
		if len(repo.entries) != 1 {
			t.Fatalf("repository holds %d entries, want 1", len(repo.entries))
		}
		if len(manager.GetPool("openai").Entries()) != 1 {
			t.Fatal("AddCredential() did not update the pool")
		}
	})

	t.Run("AddCredential reports repository failures", func(t *testing.T) {
		repo := newFakeRepo()
		repo.err = errors.New("insert failed")
		manager := account.NewManager(repo, fakeSecrets{})
		if err := manager.AddCredential(context.Background(), entry("a", account.StatusActive)); err == nil {
			t.Fatal("AddCredential() error = nil, want the repository failure")
		}
	})

	t.Run("Pause and Resume reach the pool and the repository", func(t *testing.T) {
		repo := newFakeRepo()
		manager := account.NewManager(repo, fakeSecrets{})
		if err := manager.AddCredential(context.Background(), entry("a", account.StatusActive)); err != nil {
			t.Fatalf("AddCredential() error = %v", err)
		}
		if err := manager.Pause(context.Background(), "openai", "a"); err != nil {
			t.Fatalf("Pause() error = %v", err)
		}
		if repo.statuses["a"] != account.StatusPaused {
			t.Fatalf("repository status = %q, want paused", repo.statuses["a"])
		}
		if got := manager.GetPool("openai").Entries()[0].Status; got != account.StatusPaused {
			t.Fatalf("pool status = %q, want paused", got)
		}
		if err := manager.Resume(context.Background(), "openai", "a"); err != nil {
			t.Fatalf("Resume() error = %v", err)
		}
		if got := manager.GetPool("openai").Entries()[0].Status; got != account.StatusActive {
			t.Fatalf("pool status = %q, want active", got)
		}
	})

	t.Run("Pause reports repository failures", func(t *testing.T) {
		repo := newFakeRepo()
		repo.err = errors.New("update failed")
		if err := account.NewManager(repo, fakeSecrets{}).Pause(context.Background(), "openai", "a"); err == nil {
			t.Fatal("Pause() error = nil, want the repository failure")
		}
	})

	t.Run("Remove deletes and drops", func(t *testing.T) {
		repo := newFakeRepo()
		manager := account.NewManager(repo, fakeSecrets{})
		if err := manager.AddCredential(context.Background(), entry("a", account.StatusActive)); err != nil {
			t.Fatalf("AddCredential() error = %v", err)
		}
		if err := manager.Remove(context.Background(), "openai", "a"); err != nil {
			t.Fatalf("Remove() error = %v", err)
		}
		if len(manager.GetPool("openai").Entries()) != 0 {
			t.Fatal("Remove() left the credential in the pool")
		}
		repo.err = errors.New("delete failed")
		if err := manager.Remove(context.Background(), "openai", "a"); err == nil {
			t.Fatal("Remove() error = nil, want the repository failure")
		}
	})
}

func TestManagerRotationReads(t *testing.T) {
	ctx := context.Background()
	manager := account.NewManager(newFakeRepo(), fakeSecrets{})
	if err := manager.AddCredential(ctx, entry("a", account.StatusActive)); err != nil {
		t.Fatalf("AddCredential() error = %v", err)
	}
	paused := entry("b", account.StatusActive)
	paused.Status = account.StatusPaused
	if err := manager.AddCredential(ctx, paused); err != nil {
		t.Fatalf("AddCredential() error = %v", err)
	}

	t.Run("Active counts the active credentials whatever their breakers say", func(t *testing.T) {
		if got := manager.Active("openai"); got != 1 {
			t.Fatalf("Active() = %d, want the one active credential", got)
		}
		if got := manager.Active("unknown"); got != 0 {
			t.Fatalf("Active() = %d, want no credentials on an unknown provider", got)
		}
	})

	t.Run("Health names one credential's breaker", func(t *testing.T) {
		if health := manager.Health("openai", "a"); health.State != account.BreakerClosed || health.Failures != 0 {
			t.Fatalf("Health() = %+v, want a closed breaker with no failures", health)
		}
		if health := manager.Health("unknown", "a"); health.State != account.BreakerClosed {
			t.Fatalf("Health() = %+v, want a closed breaker where nothing rotates", health)
		}
		if health := manager.GetPool("openai").Health("a"); health.State != account.BreakerClosed {
			t.Fatalf("pool Health() = %+v, want the same closed breaker", health)
		}
	})

	t.Run("a fresh provider cools down at no time", func(t *testing.T) {
		if until := manager.CooldownUntil("openai"); !until.IsZero() {
			t.Fatalf("CooldownUntil() = %v, want no wait on a fresh provider", until)
		}
		stamp := time.Date(2026, 10, 7, 8, 31, 0, 0, time.UTC)
		message := account.CoolingDown("openai", stamp)
		if !strings.Contains(message, "openai") || !strings.Contains(message, "08:31 UTC") {
			t.Fatalf("CoolingDown() = %q, want the provider and the wait named", message)
		}
	})

	t.Run("Adopt puts a stored credential into rotation", func(t *testing.T) {
		manager.Adopt(entry("c", account.StatusActive))
		if got := len(manager.GetPool("openai").Entries()); got != 3 {
			t.Fatalf("pool entries = %d, want the adopted credential beside the other two", got)
		}
		if got := manager.Active("openai"); got != 2 {
			t.Fatalf("Active() = %d, want both active credentials", got)
		}
	})

	t.Run("SetPriority reaches the pool entry", func(t *testing.T) {
		if err := manager.SetPriority(ctx, "openai", "a", 5); err != nil {
			t.Fatalf("SetPriority() error = %v", err)
		}
		for _, candidate := range manager.GetPool("openai").Entries() {
			if candidate.ID == "a" && candidate.Priority != 5 {
				t.Fatalf("priority = %d, want the stored rank", candidate.Priority)
			}
		}
	})

	t.Run("MarkNeedsReauth takes the credential out of rotation", func(t *testing.T) {
		if err := manager.MarkNeedsReauth(ctx, "openai", "a"); err != nil {
			t.Fatalf("MarkNeedsReauth() error = %v", err)
		}
		if _, err := manager.GetPool("openai").Select(account.Selection{}); err != nil {
			t.Fatalf("Select() error = %v, want the remaining credential to serve", err)
		}
	})

	t.Run("SetFailoverBackoff clears an open breaker", func(t *testing.T) {
		pooled := manager.GetPool("openai")
		pooled.RecordFailure("c", http.StatusTooManyRequests, 0)
		pooled.RecordFailure("c", http.StatusTooManyRequests, 0)
		if health := manager.Health("openai", "c"); health.State == account.BreakerClosed {
			t.Fatalf("Health() = %+v, want the repeated refusal to open the breaker", health)
		}
		manager.SetFailoverBackoff(nil)
		if health := manager.Health("openai", "c"); health.State != account.BreakerClosed {
			t.Fatalf("Health() = %+v, want no denylist to close the breaker at once", health)
		}
	})
}

func TestResolveSecret(t *testing.T) {
	t.Run("ResolveSecret returns decrypted value", func(t *testing.T) {
		manager := account.NewManager(newFakeRepo(), fakeSecrets{values: map[string]string{"apikey/openai/a": "sk-live"}})
		value, err := manager.ResolveSecret(entry("a", account.StatusActive))
		if err != nil {
			t.Fatalf("ResolveSecret() error = %v", err)
		}
		if value != "sk-live" {
			t.Fatalf("ResolveSecret() = %q, want the stored secret", value)
		}
	})

	t.Run("ResolveSecret reports missing stores and references", func(t *testing.T) {
		bare := account.NewManager(newFakeRepo(), nil)
		if _, err := bare.ResolveSecret(entry("a", account.StatusActive)); !errors.Is(err, account.ErrNoSecretStore) {
			t.Fatalf("ResolveSecret() error = %v, want %v", err, account.ErrNoSecretStore)
		}
		manager := account.NewManager(newFakeRepo(), fakeSecrets{})
		withoutRef := entry("a", account.StatusActive)
		withoutRef.SecretRef = ""
		if _, err := manager.ResolveSecret(withoutRef); !errors.Is(err, account.ErrNoSecretRef) {
			t.Fatalf("ResolveSecret() error = %v, want %v", err, account.ErrNoSecretRef)
		}
	})

	t.Run("ResolveSecret reports store failures", func(t *testing.T) {
		manager := account.NewManager(newFakeRepo(), fakeSecrets{err: errors.New("keychain is locked")})
		if _, err := manager.ResolveSecret(entry("a", account.StatusActive)); err == nil {
			t.Fatal("ResolveSecret() error = nil, want the store failure")
		}
	})
}

func entry(id, status string) account.PoolEntry {
	return account.PoolEntry{
		ID:         id,
		ProviderID: "openai",
		Kind:       "api_key",
		Label:      id,
		SecretRef:  "apikey/openai/" + id,
		Status:     status,
	}
}

// lastStrategy always picks the final candidate.
type lastStrategy struct{}

func (lastStrategy) Select(candidates []account.PoolEntry, _ account.Selection) (account.PoolEntry, error) {
	if len(candidates) == 0 {
		return account.PoolEntry{}, account.ErrNoCredentials
	}
	return candidates[len(candidates)-1], nil
}

// fakeRepo is an in-memory credential repository.
type fakeRepo struct {
	entries  []account.PoolEntry
	statuses map[string]string
	err      error
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{statuses: map[string]string{}}
}

func (r *fakeRepo) List(context.Context) ([]account.PoolEntry, error) {
	if r.err != nil {
		return nil, r.err
	}
	return r.entries, nil
}

func (r *fakeRepo) Insert(_ context.Context, entry account.PoolEntry) error {
	if r.err != nil {
		return r.err
	}
	r.entries = append(r.entries, entry)
	return nil
}

func (r *fakeRepo) SetStatus(_ context.Context, id string, status string) error {
	if r.err != nil {
		return r.err
	}
	r.statuses[id] = status
	return nil
}

func (r *fakeRepo) SetPriority(_ context.Context, id string, priority int) error {
	if r.err != nil {
		return r.err
	}
	for index := range r.entries {
		if r.entries[index].ID == id {
			r.entries[index].Priority = priority
			return nil
		}
	}
	return nil
}

func (r *fakeRepo) Remove(_ context.Context, id string) error {
	if r.err != nil {
		return r.err
	}
	kept := r.entries[:0]
	for _, entry := range r.entries {
		if entry.ID != id {
			kept = append(kept, entry)
		}
	}
	r.entries = kept
	return nil
}

// fakeSecrets stands in for the secret store.
type fakeSecrets struct {
	values map[string]string
	err    error
}

func (f fakeSecrets) Set(string, string) error {
	return nil
}

func (f fakeSecrets) Get(ref string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.values[ref], nil
}

func (f fakeSecrets) Delete(string) error {
	return nil
}

func (f fakeSecrets) Mode() secrets.SecretMode {
	return secrets.ModeNone
}

func TestStrategyNames(t *testing.T) {
	t.Run("every stored name builds a strategy", func(t *testing.T) {
		for _, name := range []string{account.StrategyLeastLoaded, account.StrategyRoundRobin, account.StrategyRandom} {
			strategy, err := account.StrategyNamed(name)
			if err != nil {
				t.Fatalf("StrategyNamed(%s) error = %v", name, err)
			}
			if got := account.StrategyName(strategy); got != name {
				t.Fatalf("StrategyName() = %q, want %q", got, name)
			}
		}
	})

	t.Run("an empty name is the default strategy", func(t *testing.T) {
		strategy, err := account.StrategyNamed("")
		if err != nil {
			t.Fatalf("StrategyNamed() error = %v", err)
		}
		if got := account.StrategyName(strategy); got != account.StrategyLeastLoaded {
			t.Fatalf("StrategyName() = %q, want %q", got, account.StrategyLeastLoaded)
		}
	})

	t.Run("an unknown name is refused", func(t *testing.T) {
		if _, err := account.StrategyNamed("nonsense"); !errors.Is(err, account.ErrUnknownStrategy) {
			t.Fatalf("StrategyNamed() error = %v, want ErrUnknownStrategy", err)
		}
	})

	t.Run("a strategy from outside the package has no name", func(t *testing.T) {
		if got := account.StrategyName(lastStrategy{}); got != "" {
			t.Fatalf("StrategyName() = %q, want none", got)
		}
	})
}

func TestApplyStrategy(t *testing.T) {
	t.Run("a provider setting replaces the pool strategy", func(t *testing.T) {
		manager := account.NewManager(newFakeRepo(), nil)
		if err := manager.ApplyStrategy("openai", account.StrategyRoundRobin); err != nil {
			t.Fatalf("ApplyStrategy() error = %v", err)
		}
		if got := manager.GetPool("openai").StrategyName(); got != account.StrategyRoundRobin {
			t.Fatalf("pool strategy = %q, want the provider setting", got)
		}
	})

	t.Run("an empty name keeps the manager strategy", func(t *testing.T) {
		manager := account.NewManagerWithOptions(newFakeRepo(), nil, account.ManagerOptions{Strategy: &account.RandomStrategy{}})
		if err := manager.ApplyStrategy("openai", ""); err != nil {
			t.Fatalf("ApplyStrategy() error = %v", err)
		}
		if got := manager.GetPool("openai").StrategyName(); got != account.StrategyRandom {
			t.Fatalf("pool strategy = %q, want the manager default", got)
		}
	})

	t.Run("a manager without a strategy falls back to least loaded", func(t *testing.T) {
		manager := account.NewManagerWithOptions(newFakeRepo(), nil, account.ManagerOptions{})
		if err := manager.ApplyStrategy("openai", ""); err != nil {
			t.Fatalf("ApplyStrategy() error = %v", err)
		}
		if got := manager.GetPool("openai").StrategyName(); got != account.StrategyLeastLoaded {
			t.Fatalf("pool strategy = %q, want the default", got)
		}
	})

	t.Run("an unknown setting is refused", func(t *testing.T) {
		manager := account.NewManager(newFakeRepo(), nil)
		if err := manager.ApplyStrategy("openai", "nonsense"); !errors.Is(err, account.ErrUnknownStrategy) {
			t.Fatalf("ApplyStrategy() error = %v, want ErrUnknownStrategy", err)
		}
	})
}
