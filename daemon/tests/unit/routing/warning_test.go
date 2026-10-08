package routing_test

import (
	"context"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/internal/routing"
	"github.com/jonaskahn/relo/tests/testkit"
)

// TestCapabilityMismatchWarnsRatherThanBlocks is the advisory contract: a
// request that asks for a tool call the catalog says a model cannot make is
// still routed, and the doubt travels as a warning so the caller sees the
// upstream's own answer rather than a routing refusal.
func TestCapabilityMismatchWarnsRatherThanBlocks(t *testing.T) {
	db := testkit.OpenTestDB(t)
	upstream := testkit.MockUpstream(t, map[string]string{})
	testkit.CustomProvider(t, db, testkit.Upstream{
		ID: "openai", Label: "OpenAI", APIFormat: catalog.FormatOpenAIChat,
		BaseURL: upstream.URL, Auth: catalog.AuthAPIKey,
		Models: []string{"alpha"}, ModelsFormat: catalog.ModelsNone,
	})
	no := false
	window := int64(1_000)
	repo := sqlite.NewCatalogRepo(db)
	if err := repo.SaveModelFacts(context.Background(), sqlite.ModelFactsRow{
		ProviderID: "openai", ModelID: "alpha", Layer: "override",
		SupportsTools: &no, ContextWindow: &window,
	}); err != nil {
		t.Fatalf("SaveModelFacts() error = %v", err)
	}
	models := testkit.ReloadCatalog(t, db, accounts{})
	router := routing.New(routing.Options{Catalog: models})

	plan, err := router.Plan(context.Background(), routing.Request{
		Model: "openai/alpha",
		Needs: catalog.Requirements{Tools: true, PromptTokens: 5_000},
	})
	if err != nil {
		t.Fatalf("Plan() error = %v, want the request routed with warnings", err)
	}
	if len(plan.Candidates) != 1 {
		t.Fatalf("candidates = %d, want the model still routed", len(plan.Candidates))
	}
	if len(plan.Skipped) != 0 {
		t.Fatalf("skipped = %+v, want nothing blocked by a capability flag", plan.Skipped)
	}
	if len(plan.Candidates[0].Warnings) != 2 {
		t.Fatalf("candidate warnings = %v, want the tool and context doubts", plan.Candidates[0].Warnings)
	}
	if len(plan.Warnings) != 2 {
		t.Fatalf("plan warnings = %+v, want both doubts recorded", plan.Warnings)
	}
	for _, warning := range plan.Warnings {
		if warning.ProviderID != "openai" || warning.ModelID != "alpha" || warning.Reason == "" {
			t.Fatalf("warning = %+v, want it to name the candidate and its reason", warning)
		}
	}
}
