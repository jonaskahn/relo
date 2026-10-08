package testkit

import (
	"context"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	"github.com/jonaskahn/relo/internal/adapters/wire/formats"
	"github.com/jonaskahn/relo/internal/catalog"
)

// Upstream describes one provider a test points at a local server, which
// is how the whole data plane is exercised without leaving the machine.
type Upstream struct {
	ID             string
	Label          string
	APIFormat      catalog.APIFormat
	BaseURL        string
	Auth           catalog.Auth
	Models         []string
	ModelsFormat   catalog.ModelsFormat
	ContextWindow  int64
	InputPrice     int64
	OutputPrice    int64
	SupportsTools  bool
	SupportsVision bool
	SupportsReason bool
}

// CustomProvider inserts one provider an operator would have added by hand,
// pointing at baseURL, with the given models.
func CustomProvider(t testing.TB, db *sqlite.DB, upstream Upstream) {
	t.Helper()
	repo := sqlite.NewCatalogRepo(db)
	insertCustomProvider(t, repo, upstream)
	for _, id := range upstream.Models {
		insertCustomModel(t, repo, upstream, id)
	}
}

func insertCustomProvider(t testing.TB, repo *sqlite.CatalogRepo, upstream Upstream) {
	t.Helper()
	host := sqlite.ProviderRow{
		ID: upstream.ID, Origin: string(catalog.OriginCustom), Label: upstream.Label,
		Auth: string(upstream.Auth), APIFormat: string(upstream.APIFormat),
		BaseURL: upstream.BaseURL, ModelsFormat: string(upstream.ModelsFormat),
		Headers: map[string]string{}, Variables: map[string]string{},
		Enabled: true, Rank: 100, PoolStrategy: sqlite.StrategyLeastLoaded,
	}
	if host.ModelsFormat == "" {
		host.ModelsFormat = string(catalog.ModelsNone)
	}
	if err := repo.SaveProvider(context.Background(), host); err != nil {
		t.Fatalf("insert provider %s: %v", upstream.ID, err)
	}
}

func insertCustomModel(t testing.TB, repo *sqlite.CatalogRepo, upstream Upstream, id string) {
	t.Helper()
	model := sqlite.ModelRow{
		ProviderID: upstream.ID, ModelID: id, Source: "manual",
		Enabled: true,
	}
	if err := repo.SaveModel(context.Background(), model); err != nil {
		t.Fatalf("insert model %s/%s: %v", upstream.ID, id, err)
	}
	insertCustomFacts(t, repo, upstream, id)
}

func insertCustomFacts(t testing.TB, repo *sqlite.CatalogRepo, upstream Upstream, id string) {
	t.Helper()
	name := id
	cat := string(catalog.CategoryChat)
	status := "active"
	facts := sqlite.ModelFactsRow{
		ProviderID:        upstream.ID,
		ModelID:           id,
		Layer:             "override",
		Name:              &name,
		Category:          &cat,
		Status:            &status,
		ContextWindow:     intPtr(upstream.ContextWindow),
		SupportsTools:     boolPtr(upstream.SupportsTools),
		SupportsVision:    boolPtr(upstream.SupportsVision),
		SupportsReasoning: boolPtr(upstream.SupportsReason),
		Prices: sqlite.PriceRow{
			Input:  intPtr(upstream.InputPrice),
			Output: intPtr(upstream.OutputPrice),
		},
	}
	if err := repo.SaveModelFacts(context.Background(), facts); err != nil {
		t.Fatalf("insert model facts %s/%s: %v", upstream.ID, id, err)
	}
}

// ReloadCatalog rebuilds a catalog over the given database.
func ReloadCatalog(t testing.TB, db *sqlite.DB, accounts catalog.Accounts) *catalog.Catalog {
	t.Helper()
	built := catalog.New(sqlite.NewCatalogReader(db), accounts, formats.New())
	if err := built.Reload(context.Background()); err != nil {
		t.Fatalf("reload catalog: %v", err)
	}
	return built
}

func intPtr(value int64) *int64 {
	if value == 0 {
		return nil
	}
	return &value
}

func boolPtr(value bool) *bool {
	stated := value
	return &stated
}
