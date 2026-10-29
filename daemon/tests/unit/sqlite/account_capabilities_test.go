package storage_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/tests/testkit"
)

func TestAccountCapabilitiesKeepPresenceAndSurviveRestart(t *testing.T) {
	ctx := context.Background()
	logger, _ := testkit.TestLogger(t)
	path := filepath.Join(t.TempDir(), "state.sqlite")
	db, err := sqlite.OpenDB(path, logger)
	if err != nil {
		t.Fatalf("OpenDB() error = %v", err)
	}
	if err := sqlite.Migrate(db); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	repo := sqlite.NewCatalogRepo(db)
	const providerID, credentialID = "openai", "cred-1"
	falseValue := false
	contextWindow, input, output := int64(262_144), int64(200_000), int64(65_536)
	facts := []catalog.AccountFactsRecord{{
		CredentialID: credentialID, ProviderID: providerID, ModelID: "gpt-5", Source: "listing",
		ContextWindow: &contextWindow, MaxInput: &input, MaxOutput: &output,
		SupportsTools: &falseValue, SupportsReasoning: &falseValue, SupportsVision: &falseValue,
		ReasoningEfforts: []string{}, ObservedAtMs: 1_700_000_000_000,
	}}
	if err := repo.SaveAccountCapabilities(ctx, providerID, credentialID, facts); err != nil {
		t.Fatalf("SaveAccountCapabilities() error = %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	db, err = sqlite.OpenDB(path, logger)
	if err != nil {
		t.Fatalf("reopen OpenDB() error = %v", err)
	}
	defer func() { _ = db.Close() }()
	if err := sqlite.Migrate(db); err != nil {
		t.Fatalf("reopen Migrate() error = %v", err)
	}
	stored, err := sqlite.NewCatalogRepo(db).ListAccountCapabilities(ctx)
	if err != nil {
		t.Fatalf("ListAccountCapabilities() error = %v", err)
	}
	if len(stored) != 1 {
		t.Fatalf("stored = %+v, want one account fact", stored)
	}
	got := stored[0]
	if got.ContextWindow == nil || *got.ContextWindow != contextWindow || got.MaxInput == nil || *got.MaxInput != input {
		t.Fatalf("limits = %+v, want exact numbers preserved", got)
	}
	if got.SupportsTools == nil || *got.SupportsTools || got.SupportsReasoning == nil || *got.SupportsReasoning {
		t.Fatalf("flags = %+v, want explicit false preserved", got)
	}
	if got.ReasoningEfforts == nil || len(got.ReasoningEfforts) != 0 {
		t.Fatalf("efforts = %v, want an explicit empty ladder, not unknown", got.ReasoningEfforts)
	}

	if err := sqlite.NewAccountFactStore(db).Delete(ctx, credentialID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	left, err := sqlite.NewCatalogRepo(db).ListAccountCapabilities(ctx)
	if err != nil {
		t.Fatalf("ListAccountCapabilities() after delete error = %v", err)
	}
	if len(left) != 0 {
		t.Fatalf("left = %+v, want account cleanup", left)
	}
}

func TestAccountCapabilitiesReplaceRatherThanAccumulate(t *testing.T) {
	ctx := context.Background()
	db := testkit.OpenTestDB(t)
	repo := sqlite.NewCatalogRepo(db)
	first, second := int64(100_000), int64(200_000)
	if err := repo.SaveAccountCapabilities(ctx, "p", "c", []catalog.AccountFactsRecord{{
		CredentialID: "c", ProviderID: "p", ModelID: "m", Source: "listing", ContextWindow: &first,
	}}); err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveAccountCapabilities(ctx, "p", "c", []catalog.AccountFactsRecord{{
		CredentialID: "c", ProviderID: "p", ModelID: "m", Source: "listing", ContextWindow: &second,
	}}); err != nil {
		t.Fatal(err)
	}
	stored, err := repo.ListAccountCapabilities(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 1 || stored[0].ContextWindow == nil || *stored[0].ContextWindow != second {
		t.Fatalf("stored = %+v, want the replacement only", stored)
	}
}
