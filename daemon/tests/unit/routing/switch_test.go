package routing_test

import (
	"context"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/internal/routing"
	"github.com/jonaskahn/relo/tests/testkit"
)

// TestPlanCarriesTheRoutesSwitchChoice pins what a route tells the relay: the
// failover choice an operator made on the route travels with the plan, so a
// request the route produced can be kept on its member after a status the
// relay does not always retry.
func TestPlanCarriesTheRoutesSwitchChoice(t *testing.T) {
	ctx := context.Background()
	db := testkit.OpenTestDB(t)
	upstream := testkit.MockUpstream(t, map[string]string{})
	testkit.CustomProvider(t, db, testkit.Upstream{
		ID: "openai", Label: "OpenAI", APIFormat: catalog.FormatOpenAIChat,
		BaseURL: upstream.URL, Auth: catalog.AuthAPIKey,
		Models: []string{"alpha"}, ModelsFormat: catalog.ModelsNone,
	})
	off := false
	if err := sqlite.NewCatalogRepo(db).SaveGroup(ctx, sqlite.GroupRow{
		ID: "alias", Label: "Alias", Strategy: string(catalog.StrategyPriority),
		Enabled: true, Listed: true, SwitchOn4xx: &off,
		Members: []sqlite.GroupMemberRow{
			{Position: 0, ProviderID: "openai", ModelID: "alpha", Weight: 1, Enabled: true},
		},
	}); err != nil {
		t.Fatalf("SaveGroup() error = %v", err)
	}
	router := routing.New(routing.Options{Catalog: testkit.ReloadCatalog(t, db, accounts{})})

	plan, err := router.Plan(ctx, routing.Request{Model: "alias"})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if plan.GroupSwitchOn4xx == nil || *plan.GroupSwitchOn4xx {
		t.Fatalf("plan = %+v, want the route's refusal of a 4xx switch to travel with it", plan)
	}
	// The option the operator never touched answers the shipped default, which
	// allows the switch.
	if plan.GroupSwitchOn5xx == nil || !*plan.GroupSwitchOn5xx {
		t.Fatalf("plan = %+v, want an unset 5xx option to read as allowed", plan)
	}

	direct, err := router.Plan(ctx, routing.Request{Model: "openai/alpha"})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if direct.GroupSwitchOn4xx != nil || direct.GroupSwitchOn5xx != nil {
		t.Fatalf("plan = %+v, want no route choice on a plan no route produced", direct)
	}
}
