package service_test

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/adapters/discovery"
	"github.com/jonaskahn/relo/internal/adapters/modelsdev"
	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/tests/testkit"
)

var seedFetchedAt = time.Unix(1_700_000_000, 0)

func failingCatalogClient() *http.Client {
	return &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errCatalogUnreachable
	})}
}

var errCatalogUnreachable = &catalogUnreachable{}

type catalogUnreachable struct{}

func (*catalogUnreachable) Error() string { return "catalog unreachable" }

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func seedDirectory(t *testing.T, providers map[string]map[string]any) *modelsdev.Directory {
	t.Helper()
	dataset, err := json.Marshal(providers)
	if err != nil {
		t.Fatalf("marshal the seed: %v", err)
	}
	seed, err := modelsdev.SeedFromDataset(dataset, seedFetchedAt)
	if err != nil {
		t.Fatalf("build the seed: %v", err)
	}
	return modelsdev.NewDirectoryWithSeed(t.TempDir(), catalog.ModelsDevDefaultURL, failingCatalogClient(), seed)
}

func cacheDirectory(t *testing.T, home string, providers map[string]map[string]any) *modelsdev.Directory {
	t.Helper()
	data, err := json.Marshal(providers)
	if err != nil {
		t.Fatalf("marshal the cache: %v", err)
	}
	cache := filepath.Join(home, "cache")
	if err := os.MkdirAll(cache, 0755); err != nil {
		t.Fatalf("create the cache directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cache, "modelsdev.json"), data, 0644); err != nil {
		t.Fatalf("write the cache: %v", err)
	}
	return modelsdev.NewDirectoryWithSeed(cache, catalog.ModelsDevDefaultURL, failingCatalogClient(), modelsdev.Seed{})
}

func TestSeedSuppliesTemplatesPricingAndRefresh(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	manager := h.buildServiceWith(h.catalog, h.pools, testkit.SecretStore(h.secrets),
		seedDirectory(t, catalogFixture()), discovery.New(nil))
	seedRow(t, h, deepseekRow())
	if err := replaceModelID(h, "deepseek", "deepseek-chat"); err != nil {
		t.Fatalf("save the catalog model: %v", err)
	}

	templates, state, err := manager.Templates(ctx)
	if err == nil {
		t.Fatal("Templates() error = nil, want the failed fetch reported")
	}
	if len(templates) == 0 {
		t.Fatal("Templates() = no templates, want the seed-backed registry")
	}
	if !state.Available || state.Stale || state.Origin != catalog.ModelsDevOriginSeed {
		t.Fatalf("state = %+v, want the seed answering", state)
	}

	report, err := manager.RefreshModelsDev(ctx)
	if err != nil {
		t.Fatalf("RefreshModelsDev() error = %v", err)
	}
	if report.ModelsMatched == 0 {
		t.Fatalf("report = %+v, want the seed to price the listed model", report)
	}
	model, err := manager.Model(ctx, "deepseek", "deepseek-chat")
	if err != nil {
		t.Fatalf("Model() error = %v", err)
	}
	if model.Prices.ModelsDev.Input == nil {
		t.Fatal("models.dev price is empty, want the seed rate")
	}
}

func TestSeedPricesAProbeWithoutTheNetwork(t *testing.T) {
	h := newHarness(t)
	listing := listingServer(t, http.StatusOK, `{"data":[{"id":"deepseek-chat","name":"DeepSeek Chat"}]}`)
	manager := h.buildServiceWith(h.catalog, h.pools, testkit.SecretStore(h.secrets),
		seedDirectory(t, catalogFixture()), discovery.New(listing.Client()))

	result, err := manager.ProbeProvider(context.Background(), probeJSON(t,
		`{"template_id":"deepseek","base_url":"`+listing.URL+
			`/v1","credential":{"kind":"api_key","secret":"sk-lab"}}`))
	if err != nil {
		t.Fatalf("ProbeProvider() error = %v", err)
	}
	if result.ModelsDevState.Origin != catalog.ModelsDevOriginSeed {
		t.Fatalf("state = %+v, want the seed", result.ModelsDevState)
	}
	model, found := modelByID(result, "deepseek-chat")
	if !found || model.Prices.ModelsDev.Input == nil {
		t.Fatalf("model = %+v, found %v; want the seed to price it", model, found)
	}
	if checkStatus(result, "modelsdev") != "pass" {
		t.Fatalf("modelsdev check = %q, want the seed reported as active", checkStatus(result, "modelsdev"))
	}
}

func TestCacheSuppliesTemplatesPricingAndSearch(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	manager := h.buildServiceWith(h.catalog, h.pools, testkit.SecretStore(h.secrets),
		cacheDirectory(t, h.home, catalogFixture()), discovery.New(nil))
	seedRow(t, h, deepseekRow())
	if err := replaceModelID(h, "deepseek", "deepseek-chat"); err != nil {
		t.Fatalf("save the catalog model: %v", err)
	}

	_, state, err := manager.Templates(ctx)
	if err == nil {
		t.Fatal("Templates() error = nil, want the failed fetch reported")
	}
	if !state.Available || !state.Stale || state.Origin != catalog.ModelsDevOriginCache {
		t.Fatalf("state = %+v, want the stale cache answering", state)
	}
	hits, searchState := manager.SearchModelsDev(ctx, "deepseek", 20)
	if len(hits) == 0 || !searchState.Available {
		t.Fatalf("search = %+v, state = %+v; want the cache", hits, searchState)
	}
	if _, err := manager.RefreshModelsDev(ctx); err != nil {
		t.Fatalf("RefreshModelsDev() error = %v", err)
	}
	model, err := manager.Model(ctx, "deepseek", "deepseek-chat")
	if err != nil || model.Prices.ModelsDev.Input == nil {
		t.Fatalf("model = %+v, %v; want the cache to price it", model, err)
	}
}

func TestNoLocalCopyLeavesTheStateUnavailable(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	manager := h.buildServiceWith(h.catalog, h.pools, testkit.SecretStore(h.secrets),
		modelsdev.NewDirectoryWithSeed(t.TempDir(), catalog.ModelsDevDefaultURL, failingCatalogClient(), modelsdev.Seed{}),
		discovery.New(nil))

	_, state, err := manager.Templates(ctx)
	if err == nil {
		t.Fatal("Templates() error = nil, want the failed fetch reported")
	}
	if state.Available || state.LastError == "" {
		t.Fatalf("state = %+v, want an unavailable copy with its reason", state)
	}
	hits, searchState := manager.SearchModelsDev(ctx, "deepseek", 20)
	if len(hits) != 0 || searchState.Available {
		t.Fatalf("search = %+v, state = %+v; want no local copy", hits, searchState)
	}
	if _, err := manager.RefreshModelsDev(ctx); err == nil {
		t.Fatal("RefreshModelsDev() error = nil, want the network failure")
	}
}

// replaceModelID swaps the harness's default model id for one the catalog
// fixture describes.
func replaceModelID(h *harness, providerID, modelID string) error {
	repo := sqlite.NewCatalogRepo(h.db)
	if err := repo.DeleteModelRow(context.Background(), providerID, "model-1"); err != nil {
		return err
	}
	if err := repo.SaveModel(context.Background(), sqlite.ModelRow{
		ProviderID: providerID, ModelID: modelID, Source: "manual", Enabled: true,
	}); err != nil {
		return err
	}
	return h.catalog.Reload(context.Background())
}
