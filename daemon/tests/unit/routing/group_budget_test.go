package routing_test

import (
	"context"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/tests/testkit"
)

// newBudgetCatalog builds a catalog holding one connection with two models,
// both of which state nothing about themselves yet.
func newBudgetCatalog(t *testing.T) (*catalog.Catalog, *sqlite.CatalogRepo) {
	t.Helper()
	db := testkit.OpenTestDB(t)
	repo := sqlite.NewCatalogRepo(db)
	if err := repo.SaveProvider(context.Background(), sqlite.ProviderRow{
		ID: "openai", Origin: string(catalog.OriginCustom), Label: "OpenAI",
		Auth: string(catalog.AuthAPIKey), APIFormat: string(catalog.FormatOpenAIChat),
		BaseURL: "https://openai.example/v1", ModelsFormat: string(catalog.ModelsNone),
		Headers: map[string]string{}, Variables: map[string]string{},
		Enabled: true, Rank: 100, PoolStrategy: sqlite.StrategyLeastLoaded,
	}); err != nil {
		t.Fatalf("SaveProvider() error = %v", err)
	}
	for _, id := range []string{"alpha", "beta"} {
		if err := repo.SaveModel(context.Background(), sqlite.ModelRow{
			ProviderID: "openai", ModelID: id, Source: "manual", Enabled: true,
		}); err != nil {
			t.Fatalf("SaveModel(%s) error = %v", id, err)
		}
		category := string(catalog.CategoryChat)
		if err := repo.SaveModelFacts(context.Background(), sqlite.ModelFactsRow{
			ProviderID: "openai", ModelID: id, Layer: "override", Category: &category,
		}); err != nil {
			t.Fatalf("SaveModelFacts(%s) error = %v", id, err)
		}
	}
	if err := repo.SaveGroup(context.Background(), sqlite.GroupRow{
		ID: "g", Label: "G", Strategy: string(catalog.StrategyPriority),
		Enabled: true, Listed: true,
		Members: []sqlite.GroupMemberRow{
			{Position: 0, ProviderID: "openai", ModelID: "alpha", Weight: 1, Enabled: true},
			{Position: 1, ProviderID: "openai", ModelID: "beta", Weight: 1, Enabled: true},
		},
	}); err != nil {
		t.Fatalf("SaveGroup() error = %v", err)
	}
	return testkit.ReloadCatalog(t, db, accounts{}), repo
}

func sizeModel(t *testing.T, catalogBuilt *catalog.Catalog, repo *sqlite.CatalogRepo, modelID string, window, output int64) {
	t.Helper()
	if err := repo.SaveModelFacts(context.Background(), sqlite.ModelFactsRow{
		ProviderID: "openai", ModelID: modelID, Layer: "override",
		ContextWindow: &window, MaxOutput: &output,
	}); err != nil {
		t.Fatalf("SaveModelFacts(%s) error = %v", modelID, err)
	}
	if err := catalogBuilt.Reload(context.Background()); err != nil {
		t.Fatalf("reload catalog: %v", err)
	}
}

// TestGroupBudgetIsTheSmallestMemberStates checks the budget resolution: the
// group's advertised window and output are the smallest a member states, so a
// caller that fits the group's figures fits every member it may reach.
func TestGroupBudgetIsTheSmallest(t *testing.T) {
	catalogBuilt, repo := newBudgetCatalog(t)
	normal := int64(262_000)
	sizeModel(t, catalogBuilt, repo, "alpha", normal, 32_000)
	normalOutput := int64(64_000)
	long := int64(1_000_000)
	sizeModel(t, catalogBuilt, repo, "beta", long, normalOutput)

	snapshot, _ := catalogBuilt.Snapshot()
	group, _ := snapshot.Group("g")
	models := snapshot.GroupModels(group)
	if len(models) != 2 {
		t.Fatalf("resolved members = %d, want both", len(models))
	}
	window, output := smallestValue(models)
	if window == nil || *window != normal {
		t.Fatalf("context = %v, want the smaller member's %d", window, normal)
	}
	if output == nil || *output != 32_000 {
		t.Fatalf("output = %v, want the smaller member's 32000", output)
	}
}

// TestGroupPromisesWhatEveryReachableMemberAllows checks the capability a
// group can publish: true only when a member states it and none deny it,
// false when any member denies it, and absent when nobody states it.
func TestGroupPromisesWhatEveryReachableMemberAllows(t *testing.T) {
	yes := true
	no := false

	t.Run("unknown", func(t *testing.T) {
		built, _ := newBudgetCatalog(t)
		reasoning, vision := promisedCapabilities(t, built)
		if reasoning != nil || vision != nil {
			t.Fatalf("reasoning = %v, vision = %v, want both unknown", reasoning, vision)
		}
	})

	t.Run("true", func(t *testing.T) {
		built, repo := newBudgetCatalog(t)
		stateCapability(t, built, repo, "alpha", &yes, &yes)
		reasoning, vision := promisedCapabilities(t, built)
		if reasoning == nil || !*reasoning || vision == nil || !*vision {
			t.Fatalf("reasoning = %v, vision = %v, want both promised", reasoning, vision)
		}
	})

	t.Run("false", func(t *testing.T) {
		built, repo := newBudgetCatalog(t)
		stateCapability(t, built, repo, "alpha", &no, &no)
		stateCapability(t, built, repo, "beta", &no, &no)
		reasoning, vision := promisedCapabilities(t, built)
		if reasoning == nil || *reasoning || vision == nil || *vision {
			t.Fatalf("reasoning = %v, vision = %v, want both denied", reasoning, vision)
		}
	})

	t.Run("mixed", func(t *testing.T) {
		built, repo := newBudgetCatalog(t)
		stateCapability(t, built, repo, "alpha", &yes, &yes)
		stateCapability(t, built, repo, "beta", &no, &no)
		reasoning, vision := promisedCapabilities(t, built)
		if reasoning == nil || *reasoning || vision == nil || *vision {
			t.Fatalf("reasoning = %v, vision = %v, want a denial to win", reasoning, vision)
		}
		snapshot, ok := built.Snapshot()
		if !ok {
			t.Fatal("snapshot missing")
		}
		group, found := snapshot.Group("g")
		if !found {
			t.Fatal("group g missing")
		}
		warnings := snapshot.CapabilityWarnings(group)
		if len(warnings) != 2 {
			t.Fatalf("warnings = %v, want reasoning and vision both mixed", warnings)
		}
	})
}

func promisedCapabilities(t *testing.T, built *catalog.Catalog) (*bool, *bool) {
	t.Helper()
	snapshot, ok := built.Snapshot()
	if !ok {
		t.Fatal("snapshot missing")
	}
	group, found := snapshot.Group("g")
	if !found {
		t.Fatal("group g missing")
	}
	_, reasoning, vision := snapshot.PromisedCapabilities(group)
	return reasoning, vision
}

// promisedTools reads the tool-calling half of PromisedCapabilities, which is
// what a console filters groups by.
func promisedTools(t *testing.T, built *catalog.Catalog) *bool {
	t.Helper()
	snapshot, ok := built.Snapshot()
	if !ok {
		t.Fatal("snapshot missing")
	}
	group, found := snapshot.Group("g")
	if !found {
		t.Fatal("group g missing")
	}
	tools, _, _ := snapshot.PromisedCapabilities(group)
	return tools
}

func stateCapability(t *testing.T, built *catalog.Catalog, repo *sqlite.CatalogRepo, modelID string, reasoning, vision *bool) {
	t.Helper()
	if err := repo.SaveModelFacts(context.Background(), sqlite.ModelFactsRow{
		ProviderID: "openai", ModelID: modelID, Layer: "override",
		SupportsReasoning: reasoning, SupportsVision: vision,
	}); err != nil {
		t.Fatalf("SaveModelFacts(%s) error = %v", modelID, err)
	}
	if err := built.Reload(context.Background()); err != nil {
		t.Fatalf("reload catalog: %v", err)
	}
}

func stateToolsCapability(t *testing.T, built *catalog.Catalog, repo *sqlite.CatalogRepo, modelID string, tools *bool) {
	t.Helper()
	if err := repo.SaveModelFacts(context.Background(), sqlite.ModelFactsRow{
		ProviderID: "openai", ModelID: modelID, Layer: "override",
		SupportsTools: tools,
	}); err != nil {
		t.Fatalf("SaveModelFacts(%s) error = %v", modelID, err)
	}
	if err := built.Reload(context.Background()); err != nil {
		t.Fatalf("reload catalog: %v", err)
	}
}

// TestGroupPromisesToolCalling checks the third promised capability the same
// way reasoning and vision are checked, since a console filters groups by all
// three. A tool-calling group is one where a member states tool calling and
// none refuses it.
func TestGroupPromisesToolCalling(t *testing.T) {
	yes := true
	no := false

	t.Run("unknown", func(t *testing.T) {
		built, _ := newBudgetCatalog(t)
		if tools := promisedTools(t, built); tools != nil {
			t.Fatalf("tools = %v, want unknown", tools)
		}
	})

	t.Run("true", func(t *testing.T) {
		built, repo := newBudgetCatalog(t)
		stateToolsCapability(t, built, repo, "alpha", &yes)
		tools := promisedTools(t, built)
		if tools == nil || !*tools {
			t.Fatalf("tools = %v, want promised", tools)
		}
	})

	t.Run("false", func(t *testing.T) {
		built, repo := newBudgetCatalog(t)
		stateToolsCapability(t, built, repo, "alpha", &no)
		tools := promisedTools(t, built)
		if tools == nil || *tools {
			t.Fatalf("tools = %v, want denied", tools)
		}
	})

	t.Run("mixed", func(t *testing.T) {
		built, repo := newBudgetCatalog(t)
		stateToolsCapability(t, built, repo, "alpha", &yes)
		stateToolsCapability(t, built, repo, "beta", &no)
		tools := promisedTools(t, built)
		if tools == nil || *tools {
			t.Fatalf("tools = %v, want a refusal to win", tools)
		}
		snapshot, ok := built.Snapshot()
		if !ok {
			t.Fatal("snapshot missing")
		}
		group, found := snapshot.Group("g")
		if !found {
			t.Fatal("group g missing")
		}
		warnings := snapshot.CapabilityWarnings(group)
		if len(warnings) != 1 || warnings[0] != catalog.WarningToolsMixed {
			t.Fatalf("warnings = %v, want tools_mixed", warnings)
		}
	})
}

// smallestValue is the smallest window and output any model states, the pair
// a group read publishes as its budget.
func smallestValue(models []catalog.Model) (*int64, *int64) {
	var window, output *int64
	for _, model := range models {
		if model.ContextWindow != nil && (window == nil || *model.ContextWindow < *window) {
			window = model.ContextWindow
		}
		if model.MaxOutput != nil && (output == nil || *model.MaxOutput < *output) {
			output = model.MaxOutput
		}
	}
	return window, output
}
