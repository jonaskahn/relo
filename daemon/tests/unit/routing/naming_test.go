package routing_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/internal/routing"
	"github.com/jonaskahn/relo/tests/testkit"
)

// TestSkippedPublishedModelIsNotMissing is what Claude Code depends on: a
// listed model whose account cannot take the request is cooling down, not
// absent, so the client must not be told to pick a different model.
func TestSkippedPublishedModelIsNotMissing(t *testing.T) {
	db := testkit.OpenTestDB(t)
	upstream := testkit.MockUpstream(t, map[string]string{})
	testkit.CustomProvider(t, db, testkit.Upstream{
		ID: "openai", Label: "OpenAI", APIFormat: catalog.FormatOpenAIChat,
		BaseURL: upstream.URL, Auth: catalog.AuthAPIKey,
		Models: []string{"alpha"}, ModelsFormat: catalog.ModelsNone,
	})
	models := testkit.ReloadCatalog(t, db, gatedAccounts{
		active: 1, serving: map[string]bool{"openai alpha": false},
	})
	router := routing.New(routing.Options{Catalog: models})
	ctx := context.Background()

	_, err := router.Plan(ctx, routing.Request{Model: "claude-relo-openai-alpha"})
	if !errors.Is(err, routing.ErrNoRoute) || errors.Is(err, routing.ErrModelNotFound) {
		t.Fatalf("Plan() error = %v, want %v", err, routing.ErrNoRoute)
	}
	if _, err := router.Plan(ctx, routing.Request{Model: "no-such-model"}); !errors.Is(err, routing.ErrModelNotFound) {
		t.Fatalf("Plan() error = %v, want an unknown name to stay %v", err, routing.ErrModelNotFound)
	}
}

func TestCoolingDownAccountIsNamedInTheSkip(t *testing.T) {
	until := time.Date(2026, 9, 30, 5, 3, 0, 0, time.UTC)
	db := testkit.OpenTestDB(t)
	upstream := testkit.MockUpstream(t, map[string]string{})
	testkit.CustomProvider(t, db, testkit.Upstream{
		ID: "openai", Label: "OpenAI", APIFormat: catalog.FormatOpenAIChat,
		BaseURL: upstream.URL, Auth: catalog.AuthAPIKey,
		Models: []string{"alpha"}, ModelsFormat: catalog.ModelsNone,
	})
	models := testkit.ReloadCatalog(t, db, gatedAccounts{
		active: 1, until: until, serving: map[string]bool{"openai alpha": false},
	})
	plan, err := routing.New(routing.Options{Catalog: models}).Plan(
		context.Background(), routing.Request{Model: "claude-relo-openai-alpha"})
	if !errors.Is(err, routing.ErrNoRoute) {
		t.Fatalf("Plan() error = %v, want %v", err, routing.ErrNoRoute)
	}
	if len(plan.Skipped) != 1 || !strings.Contains(plan.Skipped[0].Reason, "cooling down") {
		t.Fatalf("skipped = %+v, want the cooldown named", plan.Skipped)
	}
	if !strings.Contains(plan.Skipped[0].Reason, "05:03 UTC") {
		t.Fatalf("skipped = %+v, want the retry time", plan.Skipped)
	}
}

// TestClaudeSonnetAliasResolvesToTheConnection keeps a model that already
// starts with claude- on the connection that serves it. The picker id splits
// the provider from that model, and a name Relo never published stays missing.
func TestClaudeSonnetAliasResolvesToTheConnection(t *testing.T) {
	db := testkit.OpenTestDB(t)
	upstream := testkit.MockUpstream(t, map[string]string{})
	testkit.CustomProvider(t, db, testkit.Upstream{
		ID: "google-antigravity", Label: "Google Antigravity", APIFormat: catalog.FormatOpenAIChat,
		BaseURL: upstream.URL, Auth: catalog.AuthAPIKey,
		Models: []string{"claude-sonnet-4-6"}, ModelsFormat: catalog.ModelsNone,
	})
	models := testkit.ReloadCatalog(t, db, accounts{})
	router := routing.New(routing.Options{Catalog: models})
	ctx := context.Background()

	alias := catalog.AnthropicModelAlias("google-antigravity", "claude-sonnet-4-6", nil)
	if alias != "claude-relo-google-antigravity--claude-sonnet-4-6" {
		t.Fatalf("alias = %q, want the model id after the separator", alias)
	}
	plan, err := router.Plan(ctx, routing.Request{Model: alias})
	if err != nil {
		t.Fatalf("Plan(%q) error = %v", alias, err)
	}
	if len(plan.Candidates) != 1 || plan.Candidates[0].ProviderID != "google-antigravity" || plan.Candidates[0].ModelID != "claude-sonnet-4-6" {
		t.Fatalf("plan = %+v, want google-antigravity/claude-sonnet-4-6", plan)
	}
	glued := "claude-relo-google-antigravity-claude-sonnet-4-6"
	if _, err := router.Plan(ctx, routing.Request{Model: glued}); err != nil {
		t.Fatalf("Plan(%q) error = %v, want the old spelling to keep resolving", glued, err)
	}
	if _, err := router.Plan(ctx, routing.Request{Model: "no-such-model"}); !errors.Is(err, routing.ErrModelNotFound) {
		t.Fatalf("Plan() error = %v, want an unknown name to stay %v", err, routing.ErrModelNotFound)
	}
}

// TestClientNamesResolve is the contract behind the names Relo publishes: the
// public spelling of a model or a route resolves to the same entry the stored
// identifier names, and the spellings an operator and the console's own tester
// use keep working.
func TestClientNamesResolve(t *testing.T) {
	router, _ := newRouter(t, catalog.StrategyPriority)
	ctx := context.Background()
	cases := []struct {
		name         string
		requested    string
		wantModel    string
		wantKind     routing.Kind
		wantProvider string
		wantUpstream string
	}{
		{name: "the published name of a model", requested: "relo-openai-alpha",
			wantModel: "openai/alpha", wantKind: routing.KindQualified, wantProvider: "openai", wantUpstream: "alpha"},
		{name: "a published name a client marked as long context", requested: "relo-openai-alpha[1m]",
			wantModel: "openai/alpha", wantKind: routing.KindQualified, wantProvider: "openai", wantUpstream: "alpha"},
		{name: "the Claude Code spelling of a model", requested: "claude-relo-openai-alpha",
			wantModel: "openai/alpha", wantKind: routing.KindQualified, wantProvider: "openai", wantUpstream: "alpha"},
		{name: "the Claude Code alias of a model", requested: "claude-relo-openai--alpha",
			wantModel: "openai/alpha", wantKind: routing.KindQualified, wantProvider: "openai", wantUpstream: "alpha"},
		{name: "the Claude Code spelling of a long-context model", requested: "claude-relo-openai-alpha[1m]",
			wantModel: "openai/alpha", wantKind: routing.KindQualified, wantProvider: "openai", wantUpstream: "alpha"},
		{name: "the Claude Code spelling of a route", requested: "claude-reloc-alias",
			wantModel: "alias", wantKind: routing.KindGroup, wantProvider: "openai", wantUpstream: "alpha"},
		{name: "the Claude Code spelling of an unlisted model", requested: "claude-relo-openai-unlisted",
			wantModel: "relo-openai-unlisted", wantKind: routing.KindPassthrough, wantProvider: "openai", wantUpstream: "unlisted"},
		{name: "the published name of a route", requested: "reloc-alias",
			wantModel: "alias", wantKind: routing.KindGroup, wantProvider: "openai", wantUpstream: "alpha"},
		{name: "the stored name of a route", requested: "alias",
			wantModel: "alias", wantKind: routing.KindGroup, wantProvider: "openai", wantUpstream: "alpha"},
		{name: "a bare identifier", requested: "alpha",
			wantModel: "alpha", wantKind: routing.KindBare, wantProvider: "openai", wantUpstream: "alpha"},
		{name: "a qualified identifier", requested: "openai/alpha",
			wantModel: "openai/alpha", wantKind: routing.KindQualified, wantProvider: "openai", wantUpstream: "alpha"},
		{name: "a published name for a model a configured connection answers", requested: "relo-openai-unlisted",
			wantModel: "relo-openai-unlisted", wantKind: routing.KindPassthrough, wantProvider: "openai", wantUpstream: "unlisted"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			plan, err := router.Plan(ctx, routing.Request{Model: testCase.requested})
			if err != nil {
				t.Fatalf("Plan(%q) error = %v", testCase.requested, err)
			}
			if plan.Model != testCase.wantModel || plan.Kind != testCase.wantKind {
				t.Fatalf("plan = %+v, want model %q as %s", plan, testCase.wantModel, testCase.wantKind)
			}
			if len(plan.Candidates) == 0 {
				t.Fatalf("plan = %+v, want a candidate", plan)
			}
			first := plan.Candidates[0]
			if first.ProviderID != testCase.wantProvider || first.ModelID != testCase.wantUpstream {
				t.Fatalf("candidate = %+v, want %s/%s", first, testCase.wantProvider, testCase.wantUpstream)
			}
		})
	}
}
