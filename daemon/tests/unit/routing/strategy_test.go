package routing_test

import (
	"context"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/internal/routing"
	"github.com/jonaskahn/relo/tests/testkit"
)

// orderedModels names the candidates a plan would try, in order.
func orderedModels(plan routing.Plan) []string {
	models := make([]string, 0, len(plan.Candidates))
	for _, candidate := range plan.Candidates {
		models = append(models, candidate.ModelID)
	}
	return models
}

func rate(value int64) *int64 { return &value }

// TestFastestOrdersByMeasuredLatency pins the only strategy whose order comes
// from what the router saw rather than from the catalog: a member the request
// has timed moves to the front, a member timed twice settles on a blend of the
// two, and a member never timed sorts last because it has nothing to say.
func TestFastestOrdersByMeasuredLatency(t *testing.T) {
	ctx := context.Background()
	router, _ := newRouter(t, catalog.StrategyFastest)
	alpha := routing.Candidate{ProviderID: "openai", ModelID: "alpha"}
	beta := routing.Candidate{ProviderID: "openai", ModelID: "beta"}

	router.Record(beta, 500*time.Millisecond)
	router.Record(alpha, 50*time.Millisecond)

	plan, err := router.Plan(ctx, routing.Request{Model: "alias"})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if got := orderedModels(plan); got[0] != "alpha" || got[1] != "beta" {
		t.Fatalf("Plan() = %v, want the fastest measured member first", got)
	}

	// A second reading does not replace the first: it moves the member toward
	// the new answer, so one slow reply cannot move it back to the front. The
	// blend weighs the new reading at three in ten, which is enough to lose the
	// front to a member that has only ever answered once.
	router.Record(alpha, 2*time.Second)
	moved, err := router.Plan(ctx, routing.Request{Model: "alias"})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if got := orderedModels(moved); got[0] != "beta" {
		t.Fatalf("Plan() = %v, want the blended latency to hand the front to beta", got)
	}

	// A reading of zero is not a measurement, so it is refused rather than
	// recorded as an instant answer.
	router.Record(alpha, 0)
	router.Record(alpha, -time.Second)
	unchanged, err := router.Plan(ctx, routing.Request{Model: "alias"})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if got := orderedModels(unchanged); got[0] != "beta" {
		t.Fatalf("Plan() = %v, want a refused reading to leave the order alone", got)
	}
}

// TestFastestRanksAMemberItHasNeverTimed covers the member with no reading
// yet. A fastest route has nothing to compare it against, so it is read as
// faster than anything measured and leads until it answers once; members that
// have answered keep their measured order behind it.
func TestFastestRanksAMemberItHasNeverTimed(t *testing.T) {
	ctx := context.Background()
	router, _ := newRouter(t, catalog.StrategyFastest)
	router.Record(routing.Candidate{ProviderID: "openai", ModelID: "beta"}, 10*time.Millisecond)

	plan, err := router.Plan(ctx, routing.Request{Model: "alias"})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if got := orderedModels(plan); got[0] != "alpha" {
		t.Fatalf("Plan() = %v, want the untimed member ahead of the timed one", got)
	}

	router.Record(routing.Candidate{ProviderID: "openai", ModelID: "alpha"}, 5*time.Second)
	timed, err := router.Plan(ctx, routing.Request{Model: "alias"})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if got := orderedModels(timed); got[0] != "beta" {
		t.Fatalf("Plan() = %v, want the faster measured member ahead once both have answered", got)
	}
}

// TestCheapestOrdersByCatalogRate pins the order a route reads as cheapest:
// the combined input and output rate decides, a member priced on output alone
// still counts, and a member the catalog knows no price for waits for everyone
// else rather than leading on a rate of zero.
func TestCheapestOrdersByCatalogRate(t *testing.T) {
	ctx := context.Background()
	db := testkit.OpenTestDB(t)
	upstream := testkit.MockUpstream(t, map[string]string{})
	testkit.CustomProvider(t, db, testkit.Upstream{
		ID: "openai", Label: "OpenAI", APIFormat: catalog.FormatOpenAIChat,
		BaseURL: upstream.URL, Auth: catalog.AuthAPIKey,
		Models: []string{"alpha", "beta", "gamma", "delta"}, ModelsFormat: catalog.ModelsNone,
	})
	repo := sqlite.NewCatalogRepo(db)
	if err := repo.SaveGroup(ctx, sqlite.GroupRow{
		ID: "alias", Label: "Alias", Strategy: string(catalog.StrategyCheapest),
		Enabled: true, Listed: true,
		Members: []sqlite.GroupMemberRow{
			{Position: 0, ProviderID: "openai", ModelID: "alpha", Weight: 1, Enabled: true},
			{Position: 1, ProviderID: "openai", ModelID: "beta", Weight: 1, Enabled: true},
			{Position: 2, ProviderID: "openai", ModelID: "gamma", Weight: 1, Enabled: true},
			{Position: 3, ProviderID: "openai", ModelID: "delta", Weight: 1, Enabled: true},
		},
	}); err != nil {
		t.Fatalf("SaveGroup() error = %v", err)
	}
	// alpha is priced on both halves, beta on output alone, gamma on input
	// alone, and delta carries no price the catalog knows at all.
	priceModel(t, db, "alpha", sqlite.PriceRow{Input: rate(3), Output: rate(4)})
	priceModel(t, db, "beta", sqlite.PriceRow{Output: rate(1)})
	priceModel(t, db, "gamma", sqlite.PriceRow{Input: rate(2)})
	priceModel(t, db, "delta", sqlite.PriceRow{})
	router := routing.New(routing.Options{Catalog: testkit.ReloadCatalog(t, db, accounts{})})

	plan, err := router.Plan(ctx, routing.Request{Model: "alias"})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	got := orderedModels(plan)
	if got[0] != "beta" || got[1] != "gamma" || got[2] != "alpha" {
		t.Fatalf("Plan() = %v, want the combined rate to lead, then input alone, then both halves", got)
	}
	if got[3] != "delta" {
		t.Fatalf("Plan() = %v, want an unpriced member last", got)
	}
}

// priceModel restates one model's rates, since a member's cost is what decides
// a cheapest route and testkit prices every model of a connection alike.
func priceModel(t *testing.T, db *sqlite.DB, modelID string, prices sqlite.PriceRow) {
	t.Helper()
	name, category, status := modelID, string(catalog.CategoryChat), "active"
	facts := sqlite.ModelFactsRow{
		ProviderID: "openai", ModelID: modelID, Layer: "override",
		Name: &name, Category: &category, Status: &status, Prices: prices,
	}
	if err := sqlite.NewCatalogRepo(db).SaveModelFacts(context.Background(), facts); err != nil {
		t.Fatalf("price model %s: %v", modelID, err)
	}
}

// TestWeightedDrawsItsOrderFromTheWeights covers the branch a preview never
// takes: a real plan draws an order, and over many draws the heaviest member
// leads far more often than the lightest one.
func TestWeightedDrawsItsOrderFromTheWeights(t *testing.T) {
	ctx := context.Background()
	router, _ := newRouter(t, catalog.StrategyWeighted)
	// A member may be stored with no weight at all, which draws the same as a
	// weight of one rather than dropping out of the rotation.
	leads := map[string]int{}
	for attempt := 0; attempt < 400; attempt++ {
		plan, err := router.Plan(ctx, routing.Request{Model: "alias"})
		if err != nil {
			t.Fatalf("Plan() error = %v", err)
		}
		leads[firstModel(plan)]++
	}
	if leads["beta"] <= leads["alpha"] {
		t.Fatalf("leads = %v, want the weighted member to lead more often than the light one", leads)
	}
}
