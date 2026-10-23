package storage_test

import (
	"context"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	"github.com/jonaskahn/relo/tests/testkit"
)

// TestRefreshDeletesSupersededRows covers the duplicate a collapsed model
// leaves behind: a row an earlier refresh stored for one backend SKU is removed
// once the model publishes once, while a row a group still routes to stays.
func TestRefreshDeletesSupersededRows(t *testing.T) {
	ctx := context.Background()
	repo := sqlite.NewCatalogRepo(testkit.OpenTestDB(t))
	if err := repo.SaveProvider(ctx, sqlite.ProviderRow{
		ID: "google-antigravity", Origin: "template", Label: "Google Antigravity", Auth: "oauth", Enabled: true,
	}); err != nil {
		t.Fatalf("SaveProvider() error = %v", err)
	}
	available := false
	for _, modelID := range []string{"claude-sonnet-5-5-low", "claude-sonnet-5-5-medium", "claude-opus-5-5-low"} {
		if err := repo.SaveModel(ctx, sqlite.ModelRow{
			ProviderID: "google-antigravity", ModelID: modelID, Source: "listing", Available: &available,
		}); err != nil {
			t.Fatalf("SaveModel(%s) error = %v", modelID, err)
		}
	}
	if err := repo.SaveGroup(ctx, sqlite.GroupRow{
		ID: "coding", Label: "Coding", Strategy: "priority", Enabled: true,
		Members: []sqlite.GroupMemberRow{{
			Position: 0, ProviderID: "google-antigravity", ModelID: "claude-opus-5-5-low", Kind: "model", Weight: 1, Enabled: true,
		}},
	}); err != nil {
		t.Fatalf("SaveGroup() error = %v", err)
	}

	listed := true
	superseded := []string{"claude-sonnet-5-5-low", "claude-sonnet-5-5-medium", "claude-opus-5-5-low"}
	if err := repo.RefreshProviderModels(ctx, "google-antigravity", []sqlite.ModelRow{
		{ProviderID: "google-antigravity", ModelID: "claude-sonnet-5-5", Source: "listing", Match: "none", Enabled: true, Available: &listed},
	}, nil, superseded, superseded, 1_700_000_000_000, ""); err != nil {
		t.Fatalf("RefreshProviderModels() error = %v", err)
	}

	stored, err := repo.ListModels(ctx)
	if err != nil {
		t.Fatalf("ListModels() error = %v", err)
	}
	kept := make(map[string]bool, len(stored))
	for _, model := range stored {
		kept[model.ModelID] = true
	}
	if !kept["claude-sonnet-5-5"] {
		t.Fatalf("models = %+v, want the collapsed model kept", stored)
	}
	for _, modelID := range []string{"claude-sonnet-5-5-low", "claude-sonnet-5-5-medium"} {
		if kept[modelID] {
			t.Fatalf("models = %+v, want the superseded %s removed", stored, modelID)
		}
	}
	if !kept["claude-opus-5-5-low"] {
		t.Fatalf("models = %+v, want the row a group routes to kept", stored)
	}
}

// TestRefreshReportsASupersededDeleteFailure keeps the refresh from half
// applying: a delete the database refuses fails the whole call.
func TestRefreshReportsASupersededDeleteFailure(t *testing.T) {
	ctx := context.Background()
	db := testkit.OpenTestDB(t)
	repo := sqlite.NewCatalogRepo(db)
	if err := repo.SaveProvider(ctx, sqlite.ProviderRow{
		ID: "google-antigravity", Origin: "template", Label: "Google Antigravity", Auth: "oauth", Enabled: true,
	}); err != nil {
		t.Fatalf("SaveProvider() error = %v", err)
	}
	if err := repo.SaveModel(ctx, sqlite.ModelRow{
		ProviderID: "google-antigravity", ModelID: "claude-sonnet-5-5-low", Source: "listing",
	}); err != nil {
		t.Fatalf("SaveModel() error = %v", err)
	}
	if _, err := db.SQL().ExecContext(ctx,
		"CREATE TRIGGER refuse_delete BEFORE DELETE ON models BEGIN SELECT RAISE(ABORT, 'refused'); END"); err != nil {
		t.Fatalf("create trigger: %v", err)
	}
	err := repo.RefreshProviderModels(ctx, "google-antigravity", nil, nil, nil, []string{"claude-sonnet-5-5-low"}, 1, "")
	if err == nil {
		t.Fatal("RefreshProviderModels() error = nil, want the refused delete reported")
	}
}
