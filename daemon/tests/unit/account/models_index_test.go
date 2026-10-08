package pool_test

import (
	"context"
	"testing"

	"github.com/jonaskahn/relo/internal/account"
)

// TestModelIndexLoadsWhatTheStoreHolds covers the startup path: the rows a
// previous run wrote decide which accounts serve which models.
func TestModelIndexLoadsWhatTheStoreHolds(t *testing.T) {
	index := account.NewModelIndex(nil)
	index.Load(
		[]account.ModelObservation{
			{CredentialID: "free", ModelID: "gpt-5-mini", Source: account.ModelSourceListing, Available: true},
			{CredentialID: "pro", ModelID: "gpt-5-pro", Source: account.ModelSourceListing, Available: false},
		},
		[]account.RosterObservation{{CredentialID: "free", Source: account.ModelSourceListing, ModelsCount: 1}},
	)
	if !index.CanServe("free", []string{"gpt-5-mini"}) {
		t.Fatal("CanServe() = false for a stored model, want true")
	}
	if index.CanServe("free", []string{"gpt-5-pro"}) {
		t.Fatal("CanServe() = true for a model the roster does not hold")
	}
	if index.CanServe("pro", []string{"gpt-5-pro"}) {
		t.Fatal("CanServe() = true for a stored refusal")
	}
	if !index.CanServe("other", []string{"anything"}) {
		t.Fatal("CanServe() = false for an account with no stored roster, want true")
	}
}

// TestModelIndexLearnsWhatAnAccountServed covers the other half of learning: an
// account that answered a model its listing did not carry stops being excluded.
func TestModelIndexLearnsWhatAnAccountServed(t *testing.T) {
	index := account.NewModelIndex(nil)
	index.SetRoster("free", []string{"gpt-5-mini"}, account.ModelSourceListing)
	if index.CanServe("free", []string{"gpt-5.5-preview"}) {
		t.Fatal("CanServe() = true before the model was learned, want false")
	}
	index.LearnAvailable("free", "gpt-5.5-preview")
	if !index.CanServe("free", []string{"gpt-5.5-preview"}) {
		t.Fatal("CanServe() = false after the account served the model, want true")
	}
	index.LearnUnavailable("free", "gpt-5.5-preview")
	if index.CanServe("free", []string{"gpt-5.5-preview"}) {
		t.Fatal("CanServe() = true after a refusal, want false")
	}
	// An empty name is not a model, and remembering one would only make the
	// index grow.
	index.LearnUnavailable("free", "")
	index.LearnUnavailable("", "gpt-5-pro")
	index.Forget("free")
	if !index.CanServe("free", []string{"gpt-5-pro"}) {
		t.Fatal("CanServe() = false after Forget, want the account permissive again")
	}
	if index.RosterKnown("free") {
		t.Fatal("RosterKnown() = true after Forget, want the roster forgotten")
	}
}

// TestManagerLearnsThroughItsStore keeps the manager's learning and the store
// in step, which is what a restarted daemon reads back.
func TestManagerLearnsThroughItsStore(t *testing.T) {
	repo := &fakeModelRepo{}
	manager := account.NewManagerWithOptions(newFakeRepo(), fakeSecrets{},
		account.ManagerOptions{Models: account.NewModelIndex(nil), ModelRepo: repo})
	ctx := context.Background()
	if err := manager.LearnAvailable(ctx, "free", "gpt-5.5-preview"); err != nil {
		t.Fatalf("LearnAvailable() error = %v", err)
	}
	if err := manager.LearnUnavailable(ctx, "free", "gpt-5-pro"); err != nil {
		t.Fatalf("LearnUnavailable() error = %v", err)
	}
	if len(repo.models) != 2 {
		t.Fatalf("stored models = %+v, want both observations kept", repo.models)
	}
	if repo.models[0].Available != true || repo.models[1].Available != false {
		t.Fatalf("stored models = %+v, want what each attempt taught", repo.models)
	}
	if err := manager.LearnUnavailable(ctx, "", "gpt-5-pro"); err != nil {
		t.Fatalf("LearnUnavailable() with no account error = %v, want a no-op", err)
	}
}

// TestPoolEntryReturnsOneNamedAccount is what reading a single account's own
// roster depends on.
func TestPoolEntryReturnsOneNamedAccount(t *testing.T) {
	pooled := account.NewPool("openai")
	pooled.Add(entry("a", account.StatusActive))
	found, ok := pooled.Entry("a")
	if !ok || found.ID != "a" {
		t.Fatalf("Entry() = %+v %v, want the stored account", found, ok)
	}
	if _, ok := pooled.Entry("b"); ok {
		t.Fatal("Entry() found an account the pool does not hold")
	}
}
