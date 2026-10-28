package service_test

import (
	"context"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	appaccount "github.com/jonaskahn/relo/internal/application/account"
	appcatalog "github.com/jonaskahn/relo/internal/application/catalog"
	"github.com/jonaskahn/relo/internal/catalog"
)

// TestAccountModelsContextSizesOneAccount covers the account-wide context
// action: one size is stored on every model the account serves, on that
// account alone, and a clear leaves the account on the connection's own
// figure.
func TestAccountModelsContextSizesOneAccount(t *testing.T) {
	h := newHarness(t)
	manager := pageService(t, h, catalogServer(t, catalogFixture()).URL)
	ctx := context.Background()
	first := h.addAccount("openai", "work", "sk-first")
	second := h.addAccount("openai", "second", "sk-second")

	// A second model of the connection, so a batch has one model that can take
	// the size and one that cannot.
	if _, err := manager.CloneModel(ctx, "openai", appcatalog.CloneRequest{
		SourceModelID: "model-1", ModelID: "model-2",
	}); err != nil {
		t.Fatalf("CloneModel() error = %v", err)
	}

	// model-1 states a maximum input of its own, which a chosen size may pass:
	// the provider is the authority, and the console marks the value.
	ceiling := int64(262_144)
	if _, err := manager.PatchModel(ctx, "openai", "model-1", appcatalog.ModelPatch{
		Override: &appcatalog.ModelOverride{MaxInput: &ceiling},
	}); err != nil {
		t.Fatalf("PatchModel() error = %v", err)
	}

	// A batch above model-1's ceiling is stored on both models.
	tooBig := int64(1_000_000)
	write, err := manager.accounts.SetAccountModelsContext(ctx, first.ID, []string{"model-1", "model-2"}, &tooBig)
	if err != nil {
		t.Fatalf("SetAccountModelsContext() error = %v", err)
	}
	if len(write.Applied) != 2 || len(write.Skipped) != 0 {
		t.Fatalf("write = %+v, want both models applied", write)
	}
	above, err := manager.accounts.AccountContext(ctx, first.ID)
	if err != nil {
		t.Fatalf("AccountContext(above) error = %v", err)
	}
	for _, model := range above.Models {
		if model.ModelID != "model-1" && model.ModelID != "model-2" {
			continue
		}
		if model.Override == nil || *model.Override != tooBig {
			t.Fatalf("%s override = %v, want %d", model.ModelID, model.Override, tooBig)
		}
	}

	// A model the connection does not hold is the one thing a batch skips.
	missing, err := manager.accounts.SetAccountModelsContext(ctx, first.ID, []string{"model-9"}, &tooBig)
	if err != nil {
		t.Fatalf("SetAccountModelsContext(missing) error = %v", err)
	}
	if len(missing.Applied) != 0 || len(missing.Skipped) != 1 || missing.Skipped[0].ModelID != "model-9" {
		t.Fatalf("write = %+v, want model-9 skipped", missing)
	}

	// A size within the ceiling applies to both.
	sized := int64(200_000)
	if _, err := manager.accounts.SetAccountModelsContext(ctx, first.ID, []string{"model-1", "model-2"}, &sized); err != nil {
		t.Fatalf("SetAccountModelsContext(within) error = %v", err)
	}

	read, err := manager.accounts.AccountContext(ctx, first.ID)
	if err != nil {
		t.Fatalf("AccountContext() error = %v", err)
	}
	if read.ProviderID != "openai" || len(read.Models) < 2 {
		t.Fatalf("account context = %+v, want openai's models", read)
	}
	for _, model := range read.Models {
		if model.ModelID != "model-1" && model.ModelID != "model-2" {
			continue
		}
		if model.Override == nil || *model.Override != sized {
			t.Fatalf("%s override = %v, want %d", model.ModelID, model.Override, sized)
		}
		if model.Effective == nil || *model.Effective != sized {
			t.Fatalf("%s effective = %v, want the account override", model.ModelID, model.Effective)
		}
	}

	// A different account keeps the connection's own figure.
	other, err := manager.accounts.AccountContext(ctx, second.ID)
	if err != nil {
		t.Fatalf("AccountContext(second) error = %v", err)
	}
	for _, model := range other.Models {
		if model.ModelID == "model-2" && model.Override != nil {
			t.Fatalf("second account override = %v, want it untouched", model.Override)
		}
	}

	// A second serving account has no account-scoped window, and the shared
	// model definition is unknown, so the public catalog must not claim an
	// exact value from the first account alone.
	advertised := manager.accounts.AdvertisedContexts(ctx)
	key := appaccount.AdvertisedContextKey("openai", "model-2")
	if value, found := advertised[key]; found {
		t.Fatalf("advertised model-2 = %v, want unknown while one serving account is unknown", *value)
	}

	// Clearing returns the account to the connection's figure.
	if _, err := manager.accounts.SetAccountModelsContext(ctx, first.ID, []string{"model-1", "model-2"}, nil); err != nil {
		t.Fatalf("SetAccountModelsContext(clear) error = %v", err)
	}
	cleared, err := manager.accounts.AccountContext(ctx, first.ID)
	if err != nil {
		t.Fatalf("AccountContext() error = %v", err)
	}
	for _, model := range cleared.Models {
		if model.ModelID == "model-2" && model.Override != nil {
			t.Fatalf("cleared override = %v, want it gone", model.Override)
		}
	}
}

// TestSingleModelContextAcceptsAboveMaxInput stores a single-model window
// above the maximum input the catalog states, which the row marks for the
// operator rather than the daemon refusing it.
func TestSingleModelContextAcceptsAboveMaxInput(t *testing.T) {
	h := newHarness(t)
	manager := pageService(t, h, catalogServer(t, catalogFixture()).URL)
	ctx := context.Background()
	ceiling := int64(262_144)
	if _, err := manager.PatchModel(ctx, "openai", "model-1", appcatalog.ModelPatch{
		Override: &appcatalog.ModelOverride{MaxInput: &ceiling},
	}); err != nil {
		t.Fatalf("PatchModel() error = %v", err)
	}
	tooBig := int64(400_000)
	updated, err := manager.PatchModel(ctx, "openai", "model-1", appcatalog.ModelPatch{
		ContextWindow: appcatalog.OptionalInt{Set: true, Value: &tooBig},
	})
	if err != nil {
		t.Fatalf("PatchModel(above maximum input) error = %v", err)
	}
	if updated.ContextWindow == nil || *updated.ContextWindow != tooBig {
		t.Fatalf("context window = %v, want %d", updated.ContextWindow, tooBig)
	}
	if updated.ContextLayers.Override == nil || *updated.ContextLayers.Override != tooBig {
		t.Fatalf("override layer = %v, want %d", updated.ContextLayers.Override, tooBig)
	}
	allowed := int64(200_000)
	if _, err := manager.PatchModel(ctx, "openai", "model-1", appcatalog.ModelPatch{
		ContextWindow: appcatalog.OptionalInt{Set: true, Value: &allowed},
	}); err != nil {
		t.Fatalf("PatchModel(within ceiling) error = %v", err)
	}
}

// TestEffectiveModelFollowsTheSelectedCredential pins the attempt-local view:
// one account's discovered facts replace the shared definition for that
// credential only, and the shared model is left untouched.
func TestEffectiveModelFollowsTheSelectedCredential(t *testing.T) {
	h := newHarness(t)
	manager := pageService(t, h, catalogServer(t, catalogFixture()).URL)
	ctx := context.Background()
	account := h.addAccount("openai", "work", "sk-first")

	window := int64(262_144)
	output := int64(65_536)
	no := false
	if err := sqlite.NewCatalogRepo(h.db).SaveAccountCapabilities(ctx, "openai", account.ID, []catalog.AccountFactsRecord{{
		CredentialID: account.ID, ProviderID: "openai", ModelID: "model-1", Source: "listing",
		ContextWindow: &window, MaxInput: &window, MaxOutput: &output, SupportsTools: &no,
	}}); err != nil {
		t.Fatalf("SaveAccountCapabilities() error = %v", err)
	}

	shared, err := manager.Model(ctx, "openai", "model-1")
	if err != nil {
		t.Fatalf("Model(shared) error = %v", err)
	}
	if shared.ContextWindow != nil {
		t.Fatalf("shared context = %v, want the shared layers unchanged", shared.ContextWindow)
	}

	effective, err := manager.accounts.EffectiveModel(ctx, "openai", "model-1", account.ID)
	if err != nil {
		t.Fatalf("EffectiveModel() error = %v", err)
	}
	if effective.ContextWindow == nil || *effective.ContextWindow != window ||
		effective.MaxOutput == nil || *effective.MaxOutput != output ||
		effective.SupportsTools == nil || *effective.SupportsTools {
		t.Fatalf("effective = %+v, want the selected account's facts", effective)
	}
	if effective.FactSources.ContextWindow != "account" || effective.FactSources.MaxOutput != "account" {
		t.Fatalf("sources = %+v, want the account layer named", effective.FactSources)
	}

	unknown, err := manager.accounts.EffectiveModel(ctx, "openai", "model-1", "another-account")
	if err != nil {
		t.Fatalf("EffectiveModel(other) error = %v", err)
	}
	if unknown.ContextWindow != nil {
		t.Fatalf("other account context = %v, want no borrowed account facts", unknown.ContextWindow)
	}

	if _, err := manager.accounts.EffectiveModel(ctx, "openai", "missing", account.ID); err == nil {
		t.Fatal("EffectiveModel(missing) error = nil, want an unknown-model refusal")
	}
}

// TestAccountContextSkipsAContextTheRosterAlreadyDefines keeps a manual account
// completion from superseding a discovered account fact.
func TestAccountContextSkipsAContextTheRosterAlreadyDefines(t *testing.T) {
	h := newHarness(t)
	manager := pageService(t, h, catalogServer(t, catalogFixture()).URL)
	ctx := context.Background()
	account := h.addAccount("openai", "work", "sk-first")
	window := int64(262_144)
	if err := sqlite.NewCatalogRepo(h.db).SaveAccountCapabilities(ctx, "openai", account.ID, []catalog.AccountFactsRecord{{
		CredentialID: account.ID, ProviderID: "openai", ModelID: "model-1", Source: "listing",
		ContextWindow: &window,
	}}); err != nil {
		t.Fatalf("SaveAccountCapabilities() error = %v", err)
	}
	override := int64(100_000)
	write, err := manager.accounts.SetAccountModelsContext(ctx, account.ID, []string{"model-1"}, &override)
	if err != nil {
		t.Fatalf("SetAccountModelsContext() error = %v", err)
	}
	if len(write.Applied) != 0 || len(write.Skipped) != 1 {
		t.Fatalf("write = %+v, want the discovered definition to win", write)
	}
}
