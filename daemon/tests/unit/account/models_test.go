package pool_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jonaskahn/relo/internal/account"
)

// rosterIndex is a model index one account publishes a roster in, which is what
// keeps a request away from an account that cannot serve it.
func rosterIndex() *account.ModelIndex {
	index := account.NewModelIndex(nil)
	index.SetRoster("free", []string{"gpt-5-mini"}, account.ModelSourceListing)
	return index
}

// TestPoolKeepsRequestsOffAnAccountThatCannotServeThem is the whole point of an
// account-level roster: a free login is never asked for a model only the paid
// one holds.
func TestPoolKeepsRequestsOffAnAccountThatCannotServeThem(t *testing.T) {
	pooled := account.NewPoolWithOptions("openai", account.Options{Models: rosterIndex()})
	pooled.Add(entry("free", account.StatusActive))
	pooled.Add(entry("pro", account.StatusActive))
	for attempt := 0; attempt < 4; attempt++ {
		selected, err := pooled.Select(account.Selection{Models: []string{"gpt-5-pro"}})
		if err != nil {
			t.Fatalf("Select() error = %v", err)
		}
		if selected.ID != "pro" {
			t.Fatalf("Select() = %q, want pro: the free account does not serve the model", selected.ID)
		}
	}
}

// TestPoolServesEveryModelAnAccountWithoutARosterReports keeps a provider that
// publishes one shared list working: nothing is filtered for it.
func TestPoolServesEveryModelAnAccountWithoutARosterReports(t *testing.T) {
	pooled := account.NewPoolWithOptions("openrouter", account.Options{Models: rosterIndex()})
	pooled.Add(entry("key-a", account.StatusActive))
	selected, err := pooled.Select(account.Selection{Models: []string{"anything"}})
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	if selected.ID != "key-a" {
		t.Fatalf("Select() = %q, want key-a", selected.ID)
	}
}

// TestPoolMatchesACloneByTheModelItWasCopiedFrom keeps an operator's clone
// usable: the provider's listing names the source id, never the clone's.
func TestPoolMatchesACloneByTheModelItWasCopiedFrom(t *testing.T) {
	pooled := account.NewPoolWithOptions("openai", account.Options{Models: rosterIndex()})
	pooled.Add(entry("free", account.StatusActive))
	selected, err := pooled.Select(account.Selection{Models: []string{"my-clone", "gpt-5-mini"}})
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	if selected.ID != "free" {
		t.Fatalf("Select() = %q, want free", selected.ID)
	}
}

// TestPoolReportsAModelNoAccountCanServe names the reason rather than reporting
// a provider with no account at all.
func TestPoolReportsAModelNoAccountCanServe(t *testing.T) {
	pooled := account.NewPoolWithOptions("openai", account.Options{Models: rosterIndex()})
	pooled.Add(entry("free", account.StatusActive))
	if _, err := pooled.Select(account.Selection{Models: []string{"gpt-5-pro"}}); !errors.Is(err, account.ErrNoModelAccount) {
		t.Fatalf("Select() error = %v, want %v", err, account.ErrNoModelAccount)
	}
}

// TestPoolRetriesTheSameAccountWhenNoOtherCanServe keeps the provider's own
// answer: with one account left, a retry goes there rather than failing locally.
func TestPoolRetriesTheSameAccountWhenNoOtherCanServe(t *testing.T) {
	pooled := account.NewPoolWithOptions("openai", account.Options{Models: rosterIndex()})
	pooled.Add(entry("pro", account.StatusActive))
	selected, err := pooled.Select(account.Selection{Models: []string{"gpt-5-pro"}, Exclude: []string{"pro"}})
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	if selected.ID != "pro" {
		t.Fatalf("Select() = %q, want pro", selected.ID)
	}
}

// TestPoolEvictsAPinThatCannotServeTheNextModel keeps a conversation moving: a
// pinned account that does not hold the new model is not chosen.
func TestPoolEvictsAPinThatCannotServeTheNextModel(t *testing.T) {
	pooled := account.NewPoolWithOptions("openai", account.Options{Models: rosterIndex()})
	pooled.Add(entry("free", account.StatusActive))
	pooled.Add(entry("pro", account.StatusActive))
	first, err := pooled.Select(account.Selection{ConversationID: "chat-1", Models: []string{"gpt-5-mini"}})
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	if first.ID != "free" {
		t.Fatalf("first Select() = %q, want the account that serves the model", first.ID)
	}
	second, err := pooled.Select(account.Selection{ConversationID: "chat-1", Models: []string{"gpt-5-pro"}})
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	if second.ID != "pro" {
		t.Fatalf("second Select() = %q, want the pin to give way to an account that serves the model", second.ID)
	}
}

// TestManagerCoverageCountsWhatCouldTakeARequest is what the console reads to
// explain a model only some accounts reach.
func TestManagerCoverageCountsWhatCouldTakeARequest(t *testing.T) {
	index := rosterIndex()
	manager := account.NewManagerWithOptions(newFakeRepo(), fakeSecrets{},
		account.ManagerOptions{Models: index, Clock: nil})
	manager.GetPool("openai").Add(entry("free", account.StatusActive))
	manager.GetPool("openai").Add(entry("pro", account.StatusActive))
	serving, active := manager.Coverage("openai", []string{"gpt-5-pro"})
	if serving != 1 || active != 2 {
		t.Fatalf("Coverage() = %d/%d, want 1 of 2", serving, active)
	}
	if !manager.Serves("openai", []string{"gpt-5-pro"}) {
		t.Fatal("Serves() = false, want true while the pro account can serve it")
	}
	if err := manager.GetPool("openai").SetStatus("pro", account.StatusPaused); err != nil {
		t.Fatalf("SetStatus() error = %v", err)
	}
	if manager.Serves("openai", []string{"gpt-5-pro"}) {
		t.Fatal("Serves() = true, want false once the only capable account is paused")
	}
}

// TestManagerRecordsRostersAndForgetsThem covers the store round trip: a roster
// read is persisted, and removing the account drops it.
func TestManagerRecordsRostersAndForgetsThem(t *testing.T) {
	repo := &fakeModelRepo{}
	manager := account.NewManagerWithOptions(newFakeRepo(), fakeSecrets{},
		account.ManagerOptions{Models: account.NewModelIndex(nil), ModelRepo: repo})
	ctx := context.Background()
	if err := manager.SetRoster(ctx, "free", []string{"gpt-5-mini"}, account.ModelSourceListing); err != nil {
		t.Fatalf("SetRoster() error = %v", err)
	}
	if !manager.RosterKnown("free") {
		t.Fatal("RosterKnown() = false after a roster was stored")
	}
	observation, models := manager.Roster("free")
	if observation.ModelsCount != 1 || len(models) != 1 || models[0] != "gpt-5-mini" {
		t.Fatalf("Roster() = %+v %v, want the stored model", observation, models)
	}
	if len(repo.rosters) != 1 || len(repo.models) != 1 {
		t.Fatalf("store holds %d rosters and %d models, want one of each", len(repo.rosters), len(repo.models))
	}
	if err := manager.LearnUnavailable(ctx, "free", "gpt-5-pro"); err != nil {
		t.Fatalf("LearnUnavailable() error = %v", err)
	}
	if manager.ModelIndex().CanServe("free", []string{"gpt-5-pro"}) {
		t.Fatal("CanServe() = true after a learned refusal")
	}
	if err := manager.Remove(ctx, "openai", "free"); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	if manager.RosterKnown("free") {
		t.Fatal("RosterKnown() = true after the account was removed")
	}
	if len(repo.forgotten) != 1 || repo.forgotten[0] != "free" {
		t.Fatalf("forgotten = %v, want the removed account", repo.forgotten)
	}
}

// fakeModelRepo is an in-memory model repository.
type fakeModelRepo struct {
	models    []account.ModelObservation
	rosters   []account.RosterObservation
	forgotten []string
}

func (r *fakeModelRepo) ListModelAccess(context.Context) ([]account.ModelObservation, []account.RosterObservation, error) {
	return r.models, r.rosters, nil
}

func (r *fakeModelRepo) SetRoster(_ context.Context, credentialID, source string, modelIDs []string, observedAtMs int64) error {
	r.rosters = append(r.rosters, account.RosterObservation{
		CredentialID: credentialID, Source: source, ModelsCount: len(modelIDs), ObservedAtMs: observedAtMs,
	})
	for _, modelID := range modelIDs {
		r.models = append(r.models, account.ModelObservation{
			CredentialID: credentialID, ModelID: modelID, Source: source,
			Available: true, ObservedAtMs: observedAtMs,
		})
	}
	return nil
}

func (r *fakeModelRepo) RecordModel(_ context.Context, observation account.ModelObservation) error {
	r.models = append(r.models, observation)
	return nil
}

func (r *fakeModelRepo) ForgetCredential(_ context.Context, credentialID string) error {
	r.forgotten = append(r.forgotten, credentialID)
	return nil
}
