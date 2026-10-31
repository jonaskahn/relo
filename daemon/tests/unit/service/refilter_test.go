package service_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/jonaskahn/relo/internal/account"
	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	appcatalog "github.com/jonaskahn/relo/internal/application/catalog"
	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/internal/platform"
)

type refilterTemplates struct {
	appcatalog.Templates
	template catalog.Template
}

func (r refilterTemplates) Get(id string, idx *catalog.ModelsDevIndex) (catalog.Template, bool) {
	if id == r.template.ID {
		return r.template, true
	}
	return r.Templates.Get(id, idx)
}

func TestRefilterStoredModelsWithoutDiscovery(t *testing.T) {
	for _, withPool := range []bool{false, true} {
		for _, hasTimestamp := range []bool{false, true} {
			name := "without pool"
			if withPool {
				name = "with pool"
			}
			if hasTimestamp {
				name += "/preserved timestamp"
			} else {
				name += "/new timestamp"
			}
			t.Run(name, func(t *testing.T) {
				ctx := context.Background()
				h := newHarness(t)
				repo := sqlite.NewCatalogRepo(h.db)
				var refreshed *int64
				if hasTimestamp {
					refreshed = new(int64(1_700_000_000_123))
				}
				seedRow(t, h, sqlite.ProviderRow{
					ID: "stored", Origin: string(catalog.OriginTemplate), TemplateID: "refilter-test", Label: "Stored", Auth: string(catalog.AuthAPIKey),
					APIFormat: string(catalog.FormatOpenAIChat), BaseURL: "https://unused.example/v1",
					ModelsFormat: string(catalog.ModelsNone), Enabled: true, LastRefreshedAtMs: refreshed,
				})
				for _, id := range []string{"keep-model", "keep-retired", "other-model", "clone"} {
					model := sqlite.ModelRow{ProviderID: "stored", ModelID: id, Source: "listing", Enabled: true, Available: new(true)}
					if id == "clone" {
						model.ClonedFrom = "keep-model"
					}
					if err := repo.SaveModel(ctx, model); err != nil {
						t.Fatal(err)
					}
				}
				if err := repo.SaveModelFacts(ctx, sqlite.ModelFactsRow{
					ProviderID: "stored", ModelID: "keep-model", Layer: "provider", Name: new("Kept model"),
					ContextWindow: new(int64(200000)), MaxOutput: new(int64(8192)),
					SupportsTools: new(true), SupportsVision: new(true), SupportsReasoning: new(true),
					ReasoningToggle: new(true), ReasoningEfforts: []string{"low", "high"},
				}); err != nil {
					t.Fatal(err)
				}
				credentialID := ""
				if withPool {
					entry := h.addAccount("stored", "Account", "key")
					h.refreshes.wait()
					credentialID = entry.ID
					if err := h.pools.SetRoster(ctx, credentialID, []string{"keep-model", "keep-retired", "other-model"}, account.ModelSourceListing); err != nil {
						t.Fatal(err)
					}
					configs := []catalog.ModelRequestConfig{
						{ModelID: "keep-model", Selection: "default", WireConfig: "{}"},
						{ModelID: "keep-retired", Selection: "default", WireConfig: "{}"},
						{ModelID: "other-model", Selection: "default", WireConfig: "{}"},
					}
					if err := repo.SaveModelRequestConfigs(ctx, "stored", credentialID, configs); err != nil {
						t.Fatal(err)
					}
					h.addAccount("stored", "Empty roster", "another-key")
					h.refreshes.wait()
				}
				row, err := repo.GetProvider(ctx, "stored")
				if err != nil {
					t.Fatal(err)
				}
				row.ModelsFormat = string(catalog.ModelsOpenAI)
				if err := repo.SaveProvider(ctx, row); err != nil {
					t.Fatal(err)
				}
				if err := h.catalog.Reload(ctx); err != nil {
					t.Fatal(err)
				}
				registry := refilterTemplates{Templates: platform.TemplateRegistry{}, template: catalog.Template{
					ID: "refilter-test", ModelsSource: "listing", ModelsFormat: catalog.ModelsOpenAI,
					FilterModels: func(entry catalog.Listed) bool { return entry.ID != "keep-retired" },
				}}
				opts := appcatalog.Options{
					Store: repo, Catalog: h.catalog, ModelsDev: h.modelsDev, Templates: registry,
					TemplatePrefixes: func(_ context.Context, id string) ([]string, bool) {
						if id != "refilter-test" {
							t.Errorf("prefix template = %q", id)
						}
						return []string{"keep-"}, true
					},
				}
				if withPool {
					opts.Pools = h.pools
				}
				service := appcatalog.New(opts)
				result, err := service.RefilterProviderModels(ctx, "stored")
				if err != nil {
					t.Fatal(err)
				}
				if result.Status != appcatalog.RefreshUpdated || result.Listed != 4 || result.Unavailable != 3 {
					t.Fatalf("refilter result = %+v", result)
				}
				snap, err := service.Snapshot()
				if err != nil {
					t.Fatal(err)
				}
				kept, found := snap.Model("stored", "keep-model")
				if !found || kept.Available == nil || !*kept.Available || kept.Name != "Kept model" {
					t.Fatalf("kept model = %+v", kept)
				}
				if kept.ContextWindow == nil || *kept.ContextWindow != 200000 || kept.MaxOutput == nil || *kept.MaxOutput != 8192 {
					t.Fatalf("model limits changed: %+v", kept)
				}
				for capability, value := range map[string]*bool{
					"tools": kept.SupportsTools, "vision": kept.SupportsVision, "reasoning": kept.SupportsReasoning,
				} {
					if value == nil || !*value {
						t.Errorf("lost %s capability", capability)
					}
				}
				if !kept.ReasoningToggle || !slices.Equal(kept.ReasoningEfforts, []string{"low", "high"}) {
					t.Fatalf("reasoning settings changed: %+v", kept)
				}
				for _, id := range []string{"model-1", "keep-retired", "other-model"} {
					model, found := snap.Model("stored", id)
					if !found || model.Available == nil || *model.Available {
						t.Errorf("filtered model %q = %+v, found %t", id, model, found)
					}
				}
				clone, found := snap.Model("stored", "clone")
				if !found || (clone.Available == nil || !*clone.Available) || clone.ClonedFrom != "keep-model" {
					t.Fatalf("clone = %+v", clone)
				}
				host, _ := snap.Provider("stored")
				if host.LastRefreshedAtMs == nil || (hasTimestamp && *host.LastRefreshedAtMs != *refreshed) || (!hasTimestamp && *host.LastRefreshedAtMs <= 0) {
					t.Fatalf("refresh timestamp = %v, prior %v", host.LastRefreshedAtMs, refreshed)
				}
				if withPool {
					_, roster := h.pools.Roster(credentialID)
					if !slices.Equal(roster, []string{"keep-model"}) {
						t.Fatalf("account roster = %v", roster)
					}
					configs, err := repo.ModelRequestConfigs(ctx, "stored", credentialID)
					if err != nil || len(configs) != 1 || configs[0].ModelID != "keep-model" {
						t.Fatalf("request configs = %+v, %v", configs, err)
					}
				}
			})
		}
	}
}

func TestRefilterSkipsManualModelsAndRejectsMissingConnections(t *testing.T) {
	h := newHarness(t)
	result, err := h.service.RefilterProviderModels(context.Background(), "openai")
	if err != nil || result.Status != appcatalog.RefreshSkipped {
		t.Fatalf("manual refilter = %+v, %v", result, err)
	}
	if _, err := h.service.RefilterProviderModels(context.Background(), "missing"); !errors.Is(err, appcatalog.ErrProviderNotFound) {
		t.Fatalf("missing connection error = %v", err)
	}
	if _, err := h.withoutCatalog().RefilterProviderModels(context.Background(), "openai"); err == nil {
		t.Fatal("missing catalog returned success")
	}
}
