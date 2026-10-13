package routing_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/account"
	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/internal/routing"
	"github.com/jonaskahn/relo/tests/testkit"
)

// storedCredentials is a credential store that holds nothing, which is all a
// test needs: it asks the manager which pools exist, not what is in them.
type storedCredentials struct{}

func (storedCredentials) List(context.Context) ([]account.PoolEntry, error) { return nil, nil }
func (storedCredentials) Insert(context.Context, account.PoolEntry) error   { return nil }
func (storedCredentials) SetStatus(context.Context, string, string) error   { return nil }
func (storedCredentials) SetLabel(context.Context, string, string) error    { return nil }
func (storedCredentials) SetPriority(context.Context, string, int) error    { return nil }
func (storedCredentials) Remove(context.Context, string) error              { return nil }

// noSecrets reads no secret, so a pool over one stores nothing to send.
type noSecrets struct{}

func (noSecrets) Get(string) (string, error) { return "", nil }

// TestConnectionCooldownReachesTheRefusal pins what a caller learns when the
// reason a request could not be served is on the connection rather than the
// route: the router does not track which accounts are waiting, so it reads the
// manager and answers with the connection's own wording.
func TestConnectionCooldownReachesTheRefusal(t *testing.T) {
	clock := testkit.NewFakeClock(time.Now())
	ctx := context.Background()
	db := testkit.OpenTestDB(t)
	upstream := testkit.MockUpstream(t, map[string]string{})
	testkit.CustomProvider(t, db, testkit.Upstream{
		ID: "openai", Label: "OpenAI", APIFormat: catalog.FormatOpenAIChat,
		BaseURL: upstream.URL, Auth: catalog.AuthAPIKey,
		Models: []string{"alpha", "beta"}, ModelsFormat: catalog.ModelsNone,
	})
	if err := sqlite.NewCatalogRepo(db).SaveGroup(ctx, sqlite.GroupRow{
		ID: "alias", Label: "Alias", Strategy: string(catalog.StrategyPriority),
		Enabled: true, Listed: true,
		Members: []sqlite.GroupMemberRow{
			{Position: 0, ProviderID: "openai", ModelID: "alpha", Weight: 1, Enabled: true},
			{Position: 1, ProviderID: "openai", ModelID: "beta", Weight: 1, Enabled: true},
		},
	}); err != nil {
		t.Fatalf("SaveGroup() error = %v", err)
	}
	// The catalog holds the route, but no account of the connection can serve
	// it, so the plan comes back empty before anything was sent.
	catalogView := testkit.ReloadCatalog(t, db, unavailable{})
	pools := account.NewManagerWithOptions(storedCredentials{}, noSecrets{}, account.ManagerOptions{
		Clock: clock, FailoverBackoff: []time.Duration{15 * time.Minute},
	})
	// Every account of the connection failed one request, which is what puts
	// the connection itself behind a wait.
	pools.GetPool("openai").MarkExhausted(0)

	router := routing.New(routing.Options{Catalog: catalogView, Pools: pools, Clock: clock})
	_, err := router.Plan(ctx, routing.Request{Model: "alias"})
	if !errors.Is(err, routing.ErrNoRoute) {
		t.Fatalf("Plan() error = %v, want a no-route refusal", err)
	}
	var cooling *routing.CoolingError
	if !errors.As(err, &cooling) {
		t.Fatalf("Plan() error = %v, want the connection's own cooldown named", err)
	}
	if !strings.Contains(cooling.Error(), "openai") {
		t.Fatalf("cooling.Error() = %q, want the connection named", cooling.Error())
	}
}

// TestRouteReasonNamesTheKindWithoutIdentifiers pins the stored value a usage
// row carries for how a request resolved: the kind's own name, so the stored
// row stays free of any identifier.
func TestRouteReasonNamesTheKindWithoutIdentifiers(t *testing.T) {
	for _, testCase := range []struct {
		kind routing.Kind
		want string
	}{
		{routing.KindGroup, "group"},
		{routing.KindQualified, "qualified"},
		{routing.KindBare, "bare"},
		{routing.KindPassthrough, "passthrough"},
	} {
		if got := testCase.kind.RouteReason(); got != testCase.want {
			t.Fatalf("Kind(%q).RouteReason() = %q, want %q", testCase.want, got, testCase.want)
		}
	}
}

// TestPassthroughServesAConnectionWithNoCatalogRow covers a request that names
// a connection and a model the catalog has no row for: the connection's own
// format and address are enough to serve it, which is what lets a newly added
// model work before anything has listed it.
func TestPassthroughServesAConnectionWithNoCatalogRow(t *testing.T) {
	ctx := context.Background()
	db := testkit.OpenTestDB(t)
	upstream := testkit.MockUpstream(t, map[string]string{})
	testkit.CustomProvider(t, db, testkit.Upstream{
		ID: "openai", Label: "OpenAI", APIFormat: catalog.FormatOpenAIChat,
		BaseURL: upstream.URL, Auth: catalog.AuthAPIKey,
		Models: []string{"alpha"}, ModelsFormat: catalog.ModelsNone,
	})
	router := routing.New(routing.Options{Catalog: testkit.ReloadCatalog(t, db, accounts{})})

	plan, err := router.Plan(ctx, routing.Request{Model: "openai/a-model-nobody-listed"})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if plan.Kind != routing.KindPassthrough || len(plan.Candidates) != 1 {
		t.Fatalf("Plan() = %+v, want one candidate on the connection itself", plan)
	}
	candidate := plan.Candidates[0]
	if candidate.ModelID != "a-model-nobody-listed" || candidate.Model.APIFormat != catalog.FormatOpenAIChat {
		t.Fatalf("candidate = %+v, want the connection's own format", candidate)
	}
}

// TestPassthroughRefusesAConnectionItCannotSend covers the ways a passthrough
// gives up: a name with no connection in it, and a connection the catalog does
// not know. Each is refused rather than answered from a connection that cannot
// serve the request.
func TestPassthroughRefusesAConnectionItCannotSend(t *testing.T) {
	ctx := context.Background()
	router, _ := newRouter(t, catalog.StrategyPriority)

	for _, model := range []string{"no-connection-in-it", "unknown/alpha"} {
		if _, err := router.Plan(ctx, routing.Request{Model: model}); err == nil {
			t.Fatalf("Plan(%q) succeeded, want it refused", model)
		}
	}
}

// TestGroupMemberRefusalsAreNamed pins that every reason a route drops a
// member is reported, so an operator reading a preview learns which member was
// switched off and which one the catalog has never heard of.
func TestGroupMemberRefusalsAreNamed(t *testing.T) {
	ctx := context.Background()
	db := testkit.OpenTestDB(t)
	upstream := testkit.MockUpstream(t, map[string]string{})
	testkit.CustomProvider(t, db, testkit.Upstream{
		ID: "openai", Label: "OpenAI", APIFormat: catalog.FormatOpenAIChat,
		BaseURL: upstream.URL, Auth: catalog.AuthAPIKey,
		Models: []string{"alpha", "beta"}, ModelsFormat: catalog.ModelsNone,
	})
	group := sqlite.GroupRow{
		ID: "alias", Label: "Alias", Strategy: string(catalog.StrategyPriority),
		Enabled: true, Listed: true,
		Members: []sqlite.GroupMemberRow{
			{Position: 0, ProviderID: "openai", ModelID: "alpha", Weight: 1, Enabled: true},
			{Position: 1, ProviderID: "openai", ModelID: "beta", Weight: 1, Enabled: false},
			{Position: 2, ProviderID: "openai", ModelID: "never-listed", Weight: 1, Enabled: true},
		},
	}
	if err := sqlite.NewCatalogRepo(db).SaveGroup(ctx, group); err != nil {
		t.Fatalf("SaveGroup() error = %v", err)
	}
	router := routing.New(routing.Options{Catalog: testkit.ReloadCatalog(t, db, accounts{})})

	plan, err := router.Plan(ctx, routing.Request{Model: "alias"})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if len(plan.Candidates) != 1 || plan.Candidates[0].ModelID != "alpha" {
		t.Fatalf("Plan() = %+v, want only the member that can serve", plan.Candidates)
	}
	reasons := map[string]string{}
	for _, skipped := range plan.Skipped {
		reasons[skipped.ModelID] = skipped.Reason
	}
	if !strings.Contains(reasons["beta"], "switched off") {
		t.Fatalf("skipped = %v, want a switched-off member named", reasons)
	}
	if !strings.Contains(reasons["never-listed"], "not in the catalog") {
		t.Fatalf("skipped = %v, want an unlisted model named", reasons)
	}
}
