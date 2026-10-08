package routing_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/internal/routing"
	"github.com/jonaskahn/relo/tests/testkit"
)

// gatedAccounts answers the catalog's questions about a provider, and lets a
// test say that one connection's accounts cannot serve a model.
type gatedAccounts struct {
	serving map[string]bool
	active  int
	until   time.Time
}

func (a gatedAccounts) Active(string) int     { return a.active }
func (a gatedAccounts) Available(string) bool { return a.active > 0 }
func (a gatedAccounts) CooldownUntil(string) time.Time {
	return a.until
}

func (a gatedAccounts) ServesModel(providerID string, model catalog.Model) bool {
	if allowed, found := a.serving[providerID+" "+model.ID]; found {
		return allowed
	}
	return true
}

func (a gatedAccounts) Coverage(string, catalog.Model) (int, int) { return a.active, a.active }

// autoRoute stores a route whose single member names a bare model identifier.
func autoRoute(t *testing.T, db *sqlite.DB, modelID string) {
	t.Helper()
	repo := sqlite.NewCatalogRepo(db)
	group := sqlite.GroupRow{
		ID: "smart", Label: "Smart", Strategy: string(catalog.StrategyPriority),
		Enabled: true, Listed: true,
		Members: []sqlite.GroupMemberRow{
			{Position: 0, ModelID: modelID, Kind: catalog.MemberKindAuto, Weight: 1, Enabled: true},
		},
	}
	if err := repo.SaveGroup(context.Background(), group); err != nil {
		t.Fatalf("SaveGroup() error = %v", err)
	}
}

// seedTwoConnections stores two connections that both serve "shared".
func seedTwoConnections(t *testing.T, db *sqlite.DB) {
	t.Helper()
	upstream := testkit.MockUpstream(t, map[string]string{})
	testkit.CustomProvider(t, db, testkit.Upstream{
		ID: "openai", Label: "OpenAI", APIFormat: catalog.FormatOpenAIChat,
		BaseURL: upstream.URL, Auth: catalog.AuthAPIKey,
		Models: []string{"shared"}, ModelsFormat: catalog.ModelsNone,
	})
	testkit.CustomProvider(t, db, testkit.Upstream{
		ID: "claude", Label: "Claude", APIFormat: catalog.FormatAnthropic,
		BaseURL: upstream.URL, Auth: catalog.AuthAPIKey,
		Models: []string{"shared"}, ModelsFormat: catalog.ModelsNone,
	})
}

// TestAutoMemberFollowsEveryConnectionThatServesTheModel is the point of a bare
// member: adding a connection joins the route without editing it.
func TestAutoMemberFollowsEveryConnectionThatServesTheModel(t *testing.T) {
	db := testkit.OpenTestDB(t)
	seedTwoConnections(t, db)
	autoRoute(t, db, "shared")
	models := testkit.ReloadCatalog(t, db, gatedAccounts{active: 1})
	router := routing.New(routing.Options{Catalog: models})

	plan, err := router.Plan(context.Background(), routing.Request{Model: "smart"})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if len(plan.Candidates) != 2 {
		t.Fatalf("candidates = %+v, want both connections that serve the model", plan.Candidates)
	}
	providers := map[string]bool{}
	for _, candidate := range plan.Candidates {
		providers[candidate.ProviderID] = true
	}
	if !providers["openai"] || !providers["claude"] {
		t.Fatalf("candidates = %+v, want openai and claude", plan.Candidates)
	}
}

// TestAutoMemberSkipsAConnectionNoAccountCanServe keeps the route honest: a
// member only lists the connections a request can actually reach.
func TestAutoMemberSkipsAConnectionNoAccountCanServe(t *testing.T) {
	db := testkit.OpenTestDB(t)
	seedTwoConnections(t, db)
	autoRoute(t, db, "shared")
	models := testkit.ReloadCatalog(t, db, gatedAccounts{
		active: 1, serving: map[string]bool{"openai shared": false},
	})
	router := routing.New(routing.Options{Catalog: models})

	plan, err := router.Plan(context.Background(), routing.Request{Model: "smart"})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if len(plan.Candidates) != 1 || plan.Candidates[0].ProviderID != "claude" {
		t.Fatalf("candidates = %+v, want only the connection that can serve the model", plan.Candidates)
	}
	if len(plan.Skipped) == 0 || plan.Skipped[0].Reason == "" {
		t.Fatalf("skipped = %+v, want the reason the other connection was left out", plan.Skipped)
	}
}

// TestAutoMemberWithoutAModelIsNoRoute reports a route nothing can serve.
func TestAutoMemberWithoutAModelIsNoRoute(t *testing.T) {
	db := testkit.OpenTestDB(t)
	seedTwoConnections(t, db)
	autoRoute(t, db, "nowhere")
	models := testkit.ReloadCatalog(t, db, gatedAccounts{active: 1})
	router := routing.New(routing.Options{Catalog: models})

	if _, err := router.Plan(context.Background(), routing.Request{Model: "smart"}); !errors.Is(err, routing.ErrNoRoute) {
		t.Fatalf("Plan() error = %v, want %v", err, routing.ErrNoRoute)
	}
}

// TestCoverageDecidesEligibility covers the rule the console reads: a model no
// account of a connection can serve is not one a request may name.
func TestCoverageDecidesEligibility(t *testing.T) {
	db := testkit.OpenTestDB(t)
	seedTwoConnections(t, db)
	models := testkit.ReloadCatalog(t, db, gatedAccounts{
		active: 1, serving: map[string]bool{"claude shared": false},
	})
	snapshot, found := models.Snapshot()
	if !found {
		t.Fatal("the catalog has no snapshot")
	}
	model, found := snapshot.Model("claude", "shared")
	if !found {
		t.Fatal("the seeded model is not in the catalog")
	}
	reason := snapshot.SkipReason(model, catalog.Requirements{})
	if reason != "no account of this connection can serve the model" {
		t.Fatalf("SkipReason() = %q, want the coverage reason", reason)
	}
	openaiModel, found := snapshot.Model("openai", "shared")
	if !found {
		t.Fatal("the openai model is not in the catalog")
	}
	if reason := snapshot.SkipReason(openaiModel, catalog.Requirements{}); reason != "" {
		t.Fatalf("SkipReason() = %q, want the connection that can serve it left alone", reason)
	}
}

// TestSwitchedOffMemberIsNotRouted keeps the member switch meaningful.
func TestSwitchedOffMemberIsNotRouted(t *testing.T) {
	db := testkit.OpenTestDB(t)
	seedTwoConnections(t, db)
	repo := sqlite.NewCatalogRepo(db)
	group := sqlite.GroupRow{
		ID: "picked", Label: "Picked", Strategy: string(catalog.StrategyPriority),
		Enabled: true, Listed: true,
		Members: []sqlite.GroupMemberRow{
			{Position: 0, ProviderID: "openai", ModelID: "shared", Kind: catalog.MemberKindModel, Weight: 1, Enabled: false},
			{Position: 1, ProviderID: "claude", ModelID: "shared", Kind: catalog.MemberKindModel, Weight: 1, Enabled: true},
		},
	}
	if err := repo.SaveGroup(context.Background(), group); err != nil {
		t.Fatalf("SaveGroup() error = %v", err)
	}
	models := testkit.ReloadCatalog(t, db, gatedAccounts{active: 1})
	router := routing.New(routing.Options{Catalog: models})
	plan, err := router.Plan(context.Background(), routing.Request{Model: "picked"})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if len(plan.Candidates) != 1 || plan.Candidates[0].ProviderID != "claude" {
		t.Fatalf("candidates = %+v, want the switched-on member only", plan.Candidates)
	}
}
