package storage_test

import (
	"context"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	"github.com/jonaskahn/relo/tests/testkit"
)

func TestModelFactsReasoningEffortsRoundTrip(t *testing.T) {
	ctx := context.Background()
	db := testkit.OpenTestDB(t)
	repo := sqlite.NewCatalogRepo(db)
	if err := repo.SaveProvider(ctx, sqlite.ProviderRow{ID: "xiaomi", Origin: "template", Label: "Xiaomi", Auth: "api_key"}); err != nil {
		t.Fatalf("SaveProvider() error = %v", err)
	}
	if err := repo.SaveModel(ctx, sqlite.ModelRow{ProviderID: "xiaomi", ModelID: "mimo-v2.6-pro", Source: "listing", UpdatedAtMs: 1}); err != nil {
		t.Fatalf("SaveModel() error = %v", err)
	}

	layers := []sqlite.ModelFactsRow{
		{ProviderID: "xiaomi", ModelID: "mimo-v2.6-pro", Layer: "modelsdev", ReasoningEfforts: []string{}},
		{ProviderID: "xiaomi", ModelID: "mimo-v2.6-pro", Layer: "provider", ReasoningEfforts: []string{"low", "xhigh"}},
		{ProviderID: "xiaomi", ModelID: "mimo-v2.6-pro", Layer: "override"},
	}
	for _, layer := range layers {
		if err := repo.SaveModelFacts(ctx, layer); err != nil {
			t.Fatalf("SaveModelFacts() error = %v", err)
		}
	}

	stored, err := repo.ListModelFacts(ctx, "xiaomi", "mimo-v2.6-pro")
	if err != nil {
		t.Fatalf("ListModelFacts() error = %v", err)
	}
	got := map[string][]string{}
	for _, layer := range stored {
		got[layer.Layer] = layer.ReasoningEfforts
	}

	if ladder := got["modelsdev"]; ladder == nil || len(ladder) != 0 {
		t.Fatalf("modelsdev ladder = %v, want an empty ladder kept", ladder)
	}
	if ladder := got["provider"]; len(ladder) != 2 || ladder[0] != "low" || ladder[1] != "xhigh" {
		t.Fatalf("provider ladder = %v, want the declared values", ladder)
	}
	if ladder := got["override"]; ladder != nil {
		t.Fatalf("override ladder = %v, want nil", ladder)
	}
}

func TestModelFactsReasoningControlsRoundTrip(t *testing.T) {
	ctx := context.Background()
	db := testkit.OpenTestDB(t)
	repo := sqlite.NewCatalogRepo(db)
	if err := repo.SaveProvider(ctx, sqlite.ProviderRow{ID: "deepseek", Origin: "template", Label: "DeepSeek", Auth: "api_key"}); err != nil {
		t.Fatalf("SaveProvider() error = %v", err)
	}
	if err := repo.SaveModel(ctx, sqlite.ModelRow{ProviderID: "deepseek", ModelID: "deepseek-v4-pro", Source: "listing", UpdatedAtMs: 1}); err != nil {
		t.Fatalf("SaveModel() error = %v", err)
	}
	toggle, budget := true, true
	min, max := int64(1024), int64(8192)
	if err := repo.SaveModelFacts(ctx, sqlite.ModelFactsRow{
		ProviderID: "deepseek", ModelID: "deepseek-v4-pro", Layer: "modelsdev",
		ReasoningEfforts: []string{"low", "high", "max"}, ReasoningToggle: &toggle, ReasoningBudget: &budget,
		ReasoningBudgetMin: &min, ReasoningBudgetMax: &max,
	}); err != nil {
		t.Fatalf("SaveModelFacts() error = %v", err)
	}
	stored, err := repo.ListModelFacts(ctx, "deepseek", "deepseek-v4-pro")
	if err != nil {
		t.Fatalf("ListModelFacts() error = %v", err)
	}
	if len(stored) != 1 || stored[0].ReasoningToggle == nil || !*stored[0].ReasoningToggle || stored[0].ReasoningBudget == nil || !*stored[0].ReasoningBudget {
		t.Fatalf("controls = %+v, want the toggle and the budget", stored)
	}
	if stored[0].ReasoningBudgetMin == nil || *stored[0].ReasoningBudgetMin != 1024 || stored[0].ReasoningBudgetMax == nil || *stored[0].ReasoningBudgetMax != 8192 {
		t.Fatalf("bounds = %v %v, want 1024 and 8192", stored[0].ReasoningBudgetMin, stored[0].ReasoningBudgetMax)
	}
}
