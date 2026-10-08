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

// accounts answers the two questions the catalog asks about a provider, so a
// route is eligible without a credential store.
type accounts struct{}

func (accounts) Active(string) int     { return 1 }
func (accounts) Available(string) bool { return true }
func (accounts) CooldownUntil(string) time.Time {
	return time.Time{}
}
func (accounts) ServesModel(string, catalog.Model) bool {
	return true
}
func (accounts) Coverage(string, catalog.Model) (int, int) { return 1, 1 }

func newRouter(t *testing.T, strategy catalog.Strategy) (*routing.Router, *catalog.Catalog) {
	t.Helper()
	db := testkit.OpenTestDB(t)
	upstream := testkit.MockUpstream(t, map[string]string{})
	testkit.CustomProvider(t, db, testkit.Upstream{
		ID: "openai", Label: "OpenAI", APIFormat: catalog.FormatOpenAIChat,
		BaseURL: upstream.URL, Auth: catalog.AuthAPIKey,
		Models: []string{"alpha", "beta"}, ModelsFormat: catalog.ModelsNone,
	})
	repo := sqlite.NewCatalogRepo(db)
	group := sqlite.GroupRow{
		ID: "alias", Label: "Alias", Strategy: string(strategy), Enabled: true, Listed: true,
		Members: []sqlite.GroupMemberRow{
			{Position: 0, ProviderID: "openai", ModelID: "alpha", Weight: 1, Enabled: true},
			{Position: 1, ProviderID: "openai", ModelID: "beta", Weight: 5, Enabled: true},
		},
	}
	if err := repo.SaveGroup(context.Background(), group); err != nil {
		t.Fatalf("SaveGroup() error = %v", err)
	}
	models := testkit.ReloadCatalog(t, db, accounts{})
	return routing.New(routing.Options{Catalog: models}), models
}

func firstModel(plan routing.Plan) string {
	if len(plan.Candidates) == 0 {
		return ""
	}
	return plan.Candidates[0].ModelID
}

// TestPreviewLeavesRoutingAlone is the contract a route preview makes: an
// operator may look at where a request would land as often as they like
// without moving where the next request actually lands.
func TestPreviewLeavesRoutingAlone(t *testing.T) {
	router, _ := newRouter(t, catalog.StrategyRoundRobin)
	ctx := context.Background()
	request := routing.Request{Model: "alias"}

	first, err := router.Preview(ctx, request)
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}
	second, err := router.Preview(ctx, request)
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}
	if firstModel(first) != firstModel(second) {
		t.Fatalf("two previews started at %q then %q, want the same candidate", firstModel(first), firstModel(second))
	}

	planned, err := router.Plan(ctx, request)
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if firstModel(planned) != firstModel(first) {
		t.Fatalf("Plan() started at %q after a preview reported %q, want the previewed candidate", firstModel(planned), firstModel(first))
	}
	next, err := router.Plan(ctx, request)
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if firstModel(next) == firstModel(planned) {
		t.Fatalf("two plans both started at %q, want round robin to walk the members", firstModel(planned))
	}
}

// TestPreviewKeepsAConversationPin checks that looking at a route does not
// drop the provider a conversation is already pinned to.
func TestPreviewKeepsAConversationPin(t *testing.T) {
	router, _ := newRouter(t, catalog.StrategyRoundRobin)
	ctx := context.Background()

	plan, err := router.Plan(ctx, routing.Request{Model: "alias"})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	pinned := plan.Candidates[len(plan.Candidates)-1]
	router.Pin("conversation-one", routing.PinPrefix(plan), pinned)

	pinnedRequest := routing.Request{Model: "alias", Conversation: "conversation-one"}
	for attempt := 1; attempt <= 2; attempt++ {
		preview, err := router.Preview(ctx, pinnedRequest)
		if err != nil {
			t.Fatalf("Preview() error = %v", err)
		}
		if got := preview.Candidates[0]; got.ProviderID != pinned.ProviderID || got.ModelID != pinned.ModelID {
			t.Fatalf("preview %d started at %s/%s, want the pinned %s/%s",
				attempt, got.ProviderID, got.ModelID, pinned.ProviderID, pinned.ModelID)
		}
	}
	real, err := router.Plan(ctx, pinnedRequest)
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if got := real.Candidates[0]; got.ModelID != pinned.ModelID {
		t.Fatalf("Plan() started at %s, want the pinned %s", got.ModelID, pinned.ModelID)
	}
}

// TestPreviewOrdersAWeightedGroup covers the strategy with no draw to show: a
// preview ranks by weight, so it is stable to read.
func TestPreviewOrdersAWeightedGroup(t *testing.T) {
	router, _ := newRouter(t, catalog.StrategyWeighted)
	ctx := context.Background()
	request := routing.Request{Model: "alias"}

	first, err := router.Preview(ctx, request)
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}
	if len(first.Candidates) != 2 || first.Candidates[0].ModelID != "beta" {
		t.Fatalf("Preview() = %+v, want the heaviest member first", first.Candidates)
	}
	for attempt := 2; attempt <= 3; attempt++ {
		again, err := router.Preview(ctx, request)
		if err != nil {
			t.Fatalf("Preview() error = %v", err)
		}
		if firstModel(again) != firstModel(first) {
			t.Fatalf("preview %d started at %q, want a stable %q", attempt, firstModel(again), firstModel(first))
		}
	}
}
