package routing_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	"github.com/jonaskahn/relo/internal/adapters/wire/formats"
	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/internal/routing"
	"github.com/jonaskahn/relo/tests/testkit"
)

// nativeAndBetaSnapshot holds one Claude model that is natively a million tokens
// and one that reaches a million on the long-context beta, each on its own route.
func nativeAndBetaSnapshot(t *testing.T) *catalog.Catalog {
	t.Helper()
	db := testkit.OpenTestDB(t)
	upstream := testkit.MockUpstream(t, map[string]string{})
	testkit.CustomProvider(t, db, testkit.Upstream{
		ID: "claude", Label: "Claude", APIFormat: catalog.FormatAnthropic,
		BaseURL: upstream.URL, Auth: catalog.AuthAPIKey, ModelsFormat: catalog.ModelsNone,
		Models: []string{"claude-sonnet-5-5", "claude-sonnet-4-6"}, ContextWindow: catalog.MillionContext,
	})
	if _, err := db.SQL().Exec("UPDATE providers SET template_id = 'claude' WHERE id = 'claude'"); err != nil {
		t.Fatalf("set provider template: %v", err)
	}
	repo := sqlite.NewCatalogRepo(db)
	for id, modelID := range map[string]string{"combo": "claude-sonnet-5-5", "beta": "claude-sonnet-4-6"} {
		if err := repo.SaveRoute(context.Background(), catalog.RouteRecord{
			ID: id, Label: id, Strategy: string(catalog.StrategyPriority), Enabled: true, Listed: true,
			Members: []catalog.RouteMemberRecord{{
				ProviderID: "claude", ModelID: modelID, Kind: catalog.MemberKindModel, Weight: 1, Enabled: true,
			}},
		}); err != nil {
			t.Fatalf("save route %s: %v", id, err)
		}
	}
	return testkit.ReloadCatalog(t, db, gatedAccounts{active: 1})
}

// TestANativeMillionModelAnswersToEverySpelling pins the fix for the reported
// failure. Claude Code knows claude-sonnet-5-5 is natively a million tokens, so
// it refuses to spell the [1m] marker and sends the bare name. That spelling used
// to resolve to the base-rate entry and refuse any prompt that did not fit, so
// every spelling now has to reach the same full window.
func TestANativeMillionModelAnswersToEverySpelling(t *testing.T) {
	models := nativeAndBetaSnapshot(t)
	snapshot, found := models.Snapshot()
	if !found {
		t.Fatal("the catalog has no snapshot")
	}
	for _, name := range []string{
		"claude-relo-claude--claude-sonnet-5-5",
		"claude-relo-claude--claude-sonnet-5-5[1m]",
		"relo-claude-claude-sonnet-5-5-1m",
	} {
		target, resolved := snapshot.ResolveClientID(name)
		if !resolved {
			t.Fatalf("ResolveClientID(%q) found nothing", name)
		}
		if !target.MillionContext || !target.NativeMillion {
			t.Fatalf("ResolveClientID(%q) = %+v, want a native million-token entry", name, target)
		}
		if target.ProviderID != "claude" || target.ModelID != "claude-sonnet-5-5" {
			t.Fatalf("ResolveClientID(%q) = %+v, want the sonnet 5.5 connection", name, target)
		}
	}
}

// TestANativeMillionModelKeepsTheFullWindow checks the limit the router applies,
// which is what turned the stripped marker into a 400 rather than a slow answer.
func TestANativeMillionModelKeepsTheFullWindow(t *testing.T) {
	models := nativeAndBetaSnapshot(t)
	router := routing.New(routing.Options{Catalog: models})

	for _, name := range []string{
		"claude-relo-claude--claude-sonnet-5-5",
		"claude-relo-claude--claude-sonnet-5-5[1m]",
		"reloc-combo",
		"reloc-combo-1m",
	} {
		plan, err := router.Plan(context.Background(), routing.Request{Model: name})
		if err != nil {
			t.Fatalf("Plan(%q) error = %v", name, err)
		}
		if plan.ContextLimit != 0 {
			t.Fatalf("Plan(%q) limit = %d, want no cap for a native million-token entry", name, plan.ContextLimit)
		}
		if !plan.NativeMillion {
			t.Fatalf("Plan(%q) = %+v, want the native flag so no long-context beta is claimed", name, plan)
		}
	}

	// A beta model still pairs, so its bare name is the one that is capped.
	standard, err := router.Plan(context.Background(), routing.Request{Model: "claude-relo-claude--claude-sonnet-4-6"})
	if err != nil || standard.ContextLimit != catalog.DefaultContext {
		t.Fatalf("beta plan = %+v, error = %v, want the %d-token base window", standard, err, catalog.DefaultContext)
	}
	if standard.NativeMillion {
		t.Fatalf("beta plan = %+v, want no native flag", standard)
	}
	marked, err := router.Plan(context.Background(), routing.Request{Model: "claude-relo-claude--claude-sonnet-4-6[1m]"})
	if err != nil || marked.ContextLimit != 0 || !marked.LongContext || marked.NativeMillion {
		t.Fatalf("marked beta plan = %+v, error = %v, want the uncapped 1M half", marked, err)
	}
}

// suffixedSnapshot holds two Claude generations on a connection that is not
// the Claude.ai sign-in, each at a million tokens.
func suffixedSnapshot(t *testing.T) *catalog.Catalog {
	t.Helper()
	db := testkit.OpenTestDB(t)
	upstream := testkit.MockUpstream(t, map[string]string{})
	testkit.CustomProvider(t, db, testkit.Upstream{
		ID: "claude-api", Label: "Claude API", APIFormat: catalog.FormatAnthropic,
		BaseURL: upstream.URL, Auth: catalog.AuthAPIKey, ModelsFormat: catalog.ModelsNone,
		Models: []string{"claude-opus-5-5", "claude-sonnet-4-6"}, ContextWindow: catalog.MillionContext,
	})
	return testkit.ReloadCatalog(t, db, gatedAccounts{active: 1})
}

// TestAModelOffTheSignInPublishesOnceUnderTheSuffix pins the connection rule
// at the routing end: a Claude generation off the Claude.ai sign-in is one
// entry, under the suffix alone, at its own window. No base-rate twin is
// published beside it, and no long-context beta is claimed for it.
func TestAModelOffTheSignInPublishesOnceUnderTheSuffix(t *testing.T) {
	models := suffixedSnapshot(t)
	snapshot, found := models.Snapshot()
	if !found {
		t.Fatal("the catalog has no snapshot")
	}
	for _, modelID := range []string{"claude-opus-5-5", "claude-sonnet-4-6"} {
		model, found := snapshot.Model("claude-api", modelID)
		if !found {
			t.Fatalf("the catalog holds no %s", modelID)
		}
		if snapshot.OffersLongContext(model) {
			t.Fatalf("%s is paired, want no base-rate twin off the sign-in", modelID)
		}
		if !snapshot.OffersSuffixed(model) {
			t.Fatalf("%s has no suffix, want one entry under it", modelID)
		}
	}
	for _, name := range []string{
		"claude-relo-claude-api--claude-opus-5-5[1m]",
		"relo-claude-api-claude-opus-5-5-1m",
	} {
		target, resolved := snapshot.ResolveClientID(name)
		if !resolved {
			t.Fatalf("ResolveClientID(%q) found nothing", name)
		}
		if !target.MillionContext || !target.NativeMillion {
			t.Fatalf("ResolveClientID(%q) = %+v, want the full window with no beta", name, target)
		}
		if target.ProviderID != "claude-api" || target.ModelID != "claude-opus-5-5" {
			t.Fatalf("ResolveClientID(%q) = %+v, want the Claude API connection", name, target)
		}
	}

	router := routing.New(routing.Options{Catalog: models})
	for _, name := range []string{
		"claude-relo-claude-api--claude-opus-5-5[1m]",
		"relo-claude-api-claude-opus-5-5-1m",
	} {
		plan, err := router.Plan(context.Background(), routing.Request{Model: name})
		if err != nil {
			t.Fatalf("Plan(%q) error = %v", name, err)
		}
		if plan.ContextLimit != 0 || !plan.NativeMillion {
			t.Fatalf("Plan(%q) = %+v, want no cap and no long-context beta", name, plan)
		}
	}
	// The bare spelling is not published, but it still reaches the same full
	// window rather than nowhere.
	bare, err := router.Plan(context.Background(), routing.Request{Model: "relo-claude-api-claude-opus-5-5"})
	if err != nil || bare.ContextLimit != 0 || bare.LongContext {
		t.Fatalf("Plan(bare) = %+v, error = %v, want the full window with no beta", bare, err)
	}
}

// routerSnapshot holds the models named, built over a catalog the test builds
// itself so a load failure can be asserted rather than fatal.
func routerSnapshot(t *testing.T, models []string) (*catalog.Catalog, error) {
	t.Helper()
	db := testkit.OpenTestDB(t)
	upstream := testkit.MockUpstream(t, map[string]string{})
	testkit.CustomProvider(t, db, testkit.Upstream{
		ID: "teamorouter", Label: "Teamo Router", APIFormat: catalog.FormatOpenAIChat,
		BaseURL: upstream.URL, Auth: catalog.AuthAPIKey, ModelsFormat: catalog.ModelsNone,
		Models: models, ContextWindow: catalog.MillionContext,
	})
	built := catalog.New(sqlite.NewCatalogReader(db), gatedAccounts{active: 1}, formats.New())
	return built, built.Reload(context.Background())
}

// TestAProvidersOwnMillionTokenSpellingWinsTheName pins the fix for the load
// failure a router provider causes. It publishes kimi-k3 and kimi-k3[1M], and
// slugging the second gives the very name Relo derives for a million-token twin
// of the first. The derived name yields, so the catalog loads and the large
// window is reachable under the provider's own spelling.
func TestAProvidersOwnMillionTokenSpellingWinsTheName(t *testing.T) {
	models, err := routerSnapshot(t, []string{"kimi-k3", "kimi-k3[1M]"})
	if err != nil {
		t.Fatalf("the catalog refused to load: %v", err)
	}
	snapshot, found := models.Snapshot()
	if !found {
		t.Fatal("the catalog has no snapshot")
	}
	router := routing.New(routing.Options{Catalog: models})

	large, resolved := snapshot.ResolveClientID("relo-teamorouter-kimi-k3-1m")
	if !resolved {
		t.Fatal("the provider's own million-token spelling resolves to nothing")
	}
	if large.ModelID != "kimi-k3[1M]" {
		t.Fatalf("relo-teamorouter-kimi-k3-1m = %+v, want the model the provider spells that way", large)
	}
	if !large.MillionContext || !large.NativeMillion {
		t.Fatalf("relo-teamorouter-kimi-k3-1m = %+v, want the full window with no beta claimed", large)
	}

	// The derived twin was not published, so the base name keeps its own window
	// rather than the base rate: there is no second entry to stay below.
	base, resolved := snapshot.ResolveClientID("relo-teamorouter-kimi-k3")
	if !resolved {
		t.Fatal("the base name resolves to nothing")
	}
	if base.ModelID != "kimi-k3" || base.MillionContext {
		t.Fatalf("relo-teamorouter-kimi-k3 = %+v, want the unpaired base model", base)
	}
	baseModel, found := snapshot.Model("teamorouter", "kimi-k3")
	if !found {
		t.Fatal("the catalog holds no kimi-k3")
	}
	if snapshot.OffersLongContext(baseModel) {
		t.Error("the listing would publish a million-token twin that was never claimed")
	}

	// A marked spelling of the yielded base model still reaches its full
	// window: the listing omits it only to avoid shadowing the provider's
	// own spelling, not because the window is unavailable.
	marked, resolved := snapshot.ResolveClientID("claude-relo-teamorouter--kimi-k3[1m]")
	if !resolved {
		t.Fatal("the marked base spelling resolves to nothing")
	}
	if !marked.MillionContext || !marked.NativeMillion {
		t.Fatalf("claude-relo-teamorouter--kimi-k3[1m] = %+v, want the full window with no beta", marked)
	}

	for _, name := range []string{"relo-teamorouter-kimi-k3-1m", "claude-relo-teamorouter--kimi-k3[1M]"} {
		plan, err := router.Plan(context.Background(), routing.Request{Model: name})
		if err != nil {
			t.Fatalf("Plan(%q) error = %v", name, err)
		}
		if plan.ContextLimit != 0 || !plan.NativeMillion {
			t.Fatalf("Plan(%q) = %+v, want no cap and the native flag", name, plan)
		}
	}
}

// TestTwoModelsThatSlugAlikeStillRefuseTheCatalog guards the other half of the
// rule: only a derived name yields. Two entries the operator can both mean are
// still a genuine collision, and silently dropping one would hide it.
func TestTwoModelsThatSlugAlikeStillRefuseTheCatalog(t *testing.T) {
	_, err := routerSnapshot(t, []string{"kimi k3", "kimi-k3"})
	if !errors.Is(err, catalog.ErrPublicIDCollision) {
		t.Fatalf("reload error = %v, want a public identifier collision", err)
	}
}
