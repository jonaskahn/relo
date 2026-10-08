package integration

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/codingclients"
	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/tests/testkit"
	"gopkg.in/yaml.v3"
)

// TestPublishedLimitsFollowTheOverride checks that a client's catalog receives
// the resolved context and max output. An operator override wins over the
// provider layer and models.dev, and a model that states neither limit omits both.
func TestPublishedLimitsFollowTheOverride(t *testing.T) {
	db := testkit.OpenTestDB(t)
	upstream := testkit.MockUpstream(t, map[string]string{})
	testkit.CustomProvider(t, db, testkit.Upstream{
		ID: "openai", Label: "OpenAI", APIFormat: catalog.FormatOpenAIChat,
		BaseURL: upstream.URL, Auth: catalog.AuthAPIKey,
		Models: []string{"gpt-4o", "bare"}, ModelsFormat: catalog.ModelsNone,
	})
	repo := sqlite.NewCatalogRepo(db)
	modelsdevWindow, providerWindow, overrideWindow := int64(100_000), int64(200_000), int64(400_000)
	modelsdevOutput, providerOutput, overrideOutput := int64(4_096), int64(8_192), int64(32_000)
	category := string(catalog.CategoryChat)
	layers := []sqlite.ModelFactsRow{
		{ProviderID: "openai", ModelID: "gpt-4o", Layer: "modelsdev", ContextWindow: &modelsdevWindow, MaxOutput: &modelsdevOutput},
		{ProviderID: "openai", ModelID: "gpt-4o", Layer: "provider", ContextWindow: &providerWindow, MaxOutput: &providerOutput},
		{ProviderID: "openai", ModelID: "gpt-4o", Layer: "override", Category: &category, ContextWindow: &overrideWindow, MaxOutput: &overrideOutput},
	}
	for _, layer := range layers {
		if err := repo.SaveModelFacts(context.Background(), layer); err != nil {
			t.Fatalf("SaveModelFacts(%s) error = %v", layer.Layer, err)
		}
	}
	service := NewService(ServiceOptions{Catalog: testkit.ReloadCatalog(t, db, activeAccounts{})})

	published := indexModels(t, service.publishedModels())
	got := published["relo-openai-gpt-4o"]
	if got.ContextWindow == nil || *got.ContextWindow != overrideWindow || got.MaxOutput == nil || *got.MaxOutput != overrideOutput {
		t.Fatalf("published = %+v, want the override limits", got)
	}
	if bare := published["relo-openai-bare"]; bare.ContextWindow != nil || bare.MaxOutput != nil {
		t.Fatalf("bare = %+v, want no stated limits", bare)
	}

	for _, kind := range []string{codingclients.FilePi, codingclients.FileOMP} {
		merged, err := testFiles{}.MergeKind(kind, "", "http://127.0.0.1:10201", service.publishedModels())
		if err != nil {
			t.Fatalf("MergeKind(%s) error = %v", kind, err)
		}
		if !strings.Contains(merged, "400000") || !strings.Contains(merged, "32000") {
			t.Fatalf("%s catalog = %s, want the override limits", kind, merged)
		}
		for _, stale := range []string{"100000", "200000", "4096", "8192"} {
			if strings.Contains(merged, stale) {
				t.Fatalf("%s catalog = %s, want %s left out", kind, merged, stale)
			}
		}
	}
}

// TestPublishedRouteUsesTheSmallerMemberBudget checks that a route promises
// the tightest context and max output among its members, including an override.
func TestPublishedRouteUsesTheSmallerMemberBudget(t *testing.T) {
	db := testkit.OpenTestDB(t)
	upstream := testkit.MockUpstream(t, map[string]string{})
	testkit.CustomProvider(t, db, testkit.Upstream{
		ID: "openai", Label: "OpenAI", APIFormat: catalog.FormatOpenAIChat,
		BaseURL: upstream.URL, Auth: catalog.AuthAPIKey,
		Models: []string{"wide", "narrow"}, ModelsFormat: catalog.ModelsNone,
	})
	repo := sqlite.NewCatalogRepo(db)
	category := string(catalog.CategoryChat)
	wideWindow, narrowWindow := int64(400_000), int64(200_000)
	wideOutput, narrowOutput := int64(64_000), int64(16_000)
	for _, layer := range []sqlite.ModelFactsRow{
		{ProviderID: "openai", ModelID: "wide", Layer: "override", Category: &category, ContextWindow: &wideWindow, MaxOutput: &wideOutput},
		{ProviderID: "openai", ModelID: "narrow", Layer: "override", Category: &category, ContextWindow: &narrowWindow, MaxOutput: &narrowOutput},
	} {
		if err := repo.SaveModelFacts(context.Background(), layer); err != nil {
			t.Fatalf("SaveModelFacts(%s) error = %v", layer.ModelID, err)
		}
	}
	if err := repo.SaveRoute(context.Background(), catalog.RouteRecord{
		ID: "mixed", Label: "Mixed", Strategy: string(catalog.StrategyPriority),
		Enabled: true, Listed: true,
		Members: []catalog.RouteMemberRecord{
			{ProviderID: "openai", ModelID: "wide", Kind: catalog.MemberKindModel, Weight: 1, Enabled: true},
			{Position: 1, ProviderID: "openai", ModelID: "narrow", Kind: catalog.MemberKindModel, Weight: 1, Enabled: true},
		},
	}); err != nil {
		t.Fatalf("SaveRoute() error = %v", err)
	}
	service := NewService(ServiceOptions{Catalog: testkit.ReloadCatalog(t, db, activeAccounts{})})

	got := indexModels(t, service.publishedModels())["reloc-mixed"]
	if got.ContextWindow == nil || *got.ContextWindow != narrowWindow || got.MaxOutput == nil || *got.MaxOutput != narrowOutput {
		t.Fatalf("published route = %+v, want the smaller member limits", got)
	}

	for _, kind := range []string{codingclients.FilePi, codingclients.FileOMP} {
		merged, err := testFiles{}.MergeKind(kind, "", "http://127.0.0.1:10201", service.publishedModels())
		if err != nil {
			t.Fatalf("MergeKind(%s) error = %v", kind, err)
		}
		window, output := writtenLimits(t, kind, merged, "reloc-mixed")
		if window == nil || *window != narrowWindow || output == nil || *output != narrowOutput {
			t.Fatalf("%s route limits = %v %v, want %d %d", kind, window, output, narrowWindow, narrowOutput)
		}
	}
}

func writtenLimits(t *testing.T, kind, merged, id string) (window, output *int64) {
	t.Helper()
	type model struct {
		ID            string `json:"id" yaml:"id"`
		ContextWindow *int64 `json:"contextWindow" yaml:"contextWindow"`
		MaxTokens     *int64 `json:"maxTokens" yaml:"maxTokens"`
	}
	var models []model
	switch kind {
	case FilePi:
		var document struct {
			Providers map[string]struct {
				Models []model `json:"models"`
			} `json:"providers"`
		}
		if err := json.Unmarshal([]byte(merged), &document); err != nil {
			t.Fatalf("parse Pi catalog: %v\n%s", err, merged)
		}
		models = document.Providers["relo"].Models
	case FileOMP:
		var document struct {
			Providers map[string]struct {
				Models []model `yaml:"models"`
			} `yaml:"providers"`
		}
		if err := yaml.Unmarshal([]byte(merged), &document); err != nil {
			t.Fatalf("parse Oh My Pi catalog: %v\n%s", err, merged)
		}
		models = document.Providers["relo"].Models
	default:
		t.Fatalf("kind %s has no limit catalog", kind)
	}
	for _, model := range models {
		if model.ID == id {
			return model.ContextWindow, model.MaxTokens
		}
	}
	t.Fatalf("%s catalog has no %s", kind, id)
	return nil, nil
}
