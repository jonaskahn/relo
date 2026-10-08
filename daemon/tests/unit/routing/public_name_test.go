package routing_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	"github.com/jonaskahn/relo/internal/adapters/wire/formats"
	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/internal/routing"
	"github.com/jonaskahn/relo/tests/testkit"
)

// TestSlugKeepsWhatAnAgentCanCarry pins the rule a public identifier is built
// from: the characters a coding agent can hold in a picker survive, everything
// else collapses to one separator, and the ends are trimmed.
func TestSlugKeepsWhatAnAgentCanCarry(t *testing.T) {
	cases := []struct{ raw, want string }{
		{raw: "openai", want: "openai"},
		{raw: "GPT-4o", want: "gpt-4o"},
		{raw: "gpt-4o-mini", want: "gpt-4o-mini"},
		{raw: "anthropic/claude-sonnet-4", want: "anthropic-claude-sonnet-4"},
		{raw: "meta-llama/Llama 3.3 70B", want: "meta-llama-llama-3.3-70b"},
		{raw: "  spaced  ", want: "spaced"},
		{raw: "--edge--", want: "edge"},
		{raw: "a_b.c", want: "a_b.c"},
		{raw: "", want: ""},
		{raw: "///", want: ""},
	}
	for _, testCase := range cases {
		if got := catalog.Slug(testCase.raw); got != testCase.want {
			t.Errorf("Slug(%q) = %q, want %q", testCase.raw, got, testCase.want)
		}
	}
}

// TestPublicNamesCarryProviderAndModel covers the two shapes a published name
// takes, and the label a client's picker shows beside it.
func TestPublicNamesCarryProviderAndModel(t *testing.T) {
	if got := catalog.ClientModelID("openai", "gpt-4o"); got != "relo-openai-gpt-4o" {
		t.Errorf("ClientModelID() = %q, want relo-openai-gpt-4o", got)
	}
	if got := catalog.ClientModelName("GPT-4o", "OpenAI", nil); got != "GPT-4o on OpenAI" {
		t.Errorf("ClientModelName() = %q, want GPT-4o on OpenAI", got)
	}
	if got := catalog.ClientModelName("Claude Opus 4.6 (Thinking)", "Google Antigravity", nil); got != "Claude Opus 4.6 (Thinking) on Google Antigravity" {
		t.Errorf("ClientModelName() = %q, want the model before the provider", got)
	}
	million := int64(catalog.MillionContext)
	if got := catalog.ClientModelName("Claude Opus 4.6 (Thinking)", "Google Antigravity", &million); got != "Claude Opus 4.6 (Thinking) 1M on Google Antigravity" {
		t.Errorf("ClientModelName() = %q, want the million-token mark", got)
	}
	smaller := int64(catalog.MillionContext - 1)
	if got := catalog.ClientModelName("GPT-4o", "OpenAI", &smaller); got != "GPT-4o on OpenAI" {
		t.Errorf("ClientModelName() = %q, want no million-token mark below the window", got)
	}
	if got := catalog.ClientModelName("GPT-4o", "", &million); got != "GPT-4o 1M" {
		t.Errorf("ClientModelName() = %q, want the model alone when the provider is empty", got)
	}
	if got := catalog.ClientModelName("Smart Combo", catalog.RouteLabelPrefix, nil); got != "Smart Combo on Relo" {
		t.Errorf("ClientModelName() = %q, want Smart Combo on Relo", got)
	}
	if got := catalog.ClientModelName("Smart Combo", catalog.RouteLabelPrefix, &million); got != "Smart Combo 1M on Relo" {
		t.Errorf("ClientModelName() = %q, want Smart Combo 1M on Relo", got)
	}
	if got := catalog.ClientRouteID("Smart Combo"); got != "reloc-smart-combo" {
		t.Errorf("ClientRouteID() = %q, want reloc-smart-combo", got)
	}
	if got := catalog.ClientRouteName("Smart Combo"); got != "Relo | Smart Combo" {
		t.Errorf("ClientRouteName() = %q, want Relo | Smart Combo", got)
	}
	if got := catalog.MillionAlias("relo-claude-opus"); got != "relo-claude-opus-1m" {
		t.Errorf("MillionAlias() = %q, want relo-claude-opus-1m", got)
	}
	if !catalog.HasMillionContextSuffix("claude-relo-claude--opus[1m]") {
		t.Error("HasMillionContextSuffix() = false, want true")
	}
}

func TestStripClaudePrefixDropsTheVendorWord(t *testing.T) {
	for _, testCase := range []struct{ raw, want string }{
		{raw: "Claude Opus 5.5", want: "Opus 5.5"},
		{raw: "Claude Haiku 5.5", want: "Haiku 5.5"},
		{raw: "Opus", want: "Opus"},
		{raw: "Claude", want: ""},
		{raw: "  Claude Sonnet 5  ", want: "Sonnet 5"},
		{raw: "ClaudeOpus", want: "ClaudeOpus"},
	} {
		if got := catalog.StripClaudePrefix(testCase.raw); got != testCase.want {
			t.Errorf("StripClaudePrefix(%q) = %q, want %q", testCase.raw, got, testCase.want)
		}
	}
}

func TestConfigModelNameNamesTheProviderAndRelo(t *testing.T) {
	if got := catalog.ConfigModelName("GPT-4o", "OpenAI", nil); got != "GPT-4o on OpenAI" {
		t.Errorf("ConfigModelName() = %q, want GPT-4o on OpenAI", got)
	}
	million := int64(catalog.MillionContext)
	if got := catalog.ConfigModelName("Claude Opus 4.6 (Thinking)", "Google Antigravity", &million); got != "Claude Opus 4.6 (Thinking) 1M on Google Antigravity" {
		t.Errorf("ConfigModelName() = %q, want the million-token mark before the provider", got)
	}
	if got := catalog.ConfigModelName("Smart Combo", catalog.RouteLabelPrefix, nil); got != "Smart Combo on Relo" {
		t.Errorf("ConfigModelName() = %q, want a route to name Relo as the provider", got)
	}
	if got := catalog.ConfigModelName("Smart Combo", catalog.RouteLabelPrefix, &million); got != "Smart Combo 1M on Relo" {
		t.Errorf("ConfigModelName() = %q, want a marked route to name Relo as the provider", got)
	}
}

// TestContextWindowRoundsToAWholeThousand pins the size a client is asked for.
// A vendor publishes a figure no preset can name (262,144 for one, 1,456,789
// for another), and a window is a count of tokens rather than an exact
// promise, so it is offered as the nearest thousand an operator reads in K or M.
func TestContextWindowRoundsToAWholeThousand(t *testing.T) {
	for _, testCase := range []struct{ stated, want int64 }{
		{stated: 262_144, want: 262_000},
		{stated: 372_000, want: 372_000},
		{stated: 1_456_789, want: 1_457_000},
		{stated: 1_999, want: 2_000},
		{stated: 128_000, want: 128_000},
		{stated: 4_000_000, want: 4_000_000},
	} {
		stated := testCase.stated
		got := catalog.RoundContextWindow(&stated)
		if got == nil || *got != testCase.want {
			t.Errorf("RoundContextWindow(%d) = %v, want %d", stated, got, testCase.want)
		}
	}
	if got := catalog.RoundContextWindow(nil); got != nil {
		t.Fatalf("RoundContextWindow(nil) = %v, want nil", got)
	}
}

func TestParseTokenCountReadsASuffix(t *testing.T) {
	for _, testCase := range []struct {
		raw  string
		want int64
	}{
		{raw: "262144", want: 262_144},
		{raw: "200k", want: 200_000},
		{raw: "256K", want: 256_000},
		{raw: "1.5M", want: 1_500_000},
		{raw: "1m", want: 1_000_000},
	} {
		got, ok := catalog.ParseTokenCount(testCase.raw)
		if !ok || got != testCase.want {
			t.Errorf("ParseTokenCount(%q) = %d, %v, want %d", testCase.raw, got, ok, testCase.want)
		}
		rounded := catalog.RoundContextWindow(&got)
		if rounded == nil {
			t.Fatalf("RoundContextWindow(%d) = nil", got)
		}
	}
	if _, ok := catalog.ParseTokenCount("abc"); ok {
		t.Fatal("ParseTokenCount(abc) = ok, want a refusal")
	}
}

// TestResolvedModelCarriesTheRoundedWindow is the end the rule exists for: the
// size a vendor states reaches every surface as a whole thousand, not as a
// figure no client can ask for.
func TestResolvedModelCarriesTheRoundedWindow(t *testing.T) {
	db := testkit.OpenTestDB(t)
	testkit.CustomProvider(t, db, testkit.Upstream{
		ID: "openai", Label: "OpenAI", APIFormat: catalog.FormatOpenAIChat,
		BaseURL: "http://openai.test", Auth: catalog.AuthAPIKey,
		Models: []string{"alpha", "odd", "tiny"}, ModelsFormat: catalog.ModelsNone,
	})
	repo := sqlite.NewCatalogRepo(db)
	for modelID, window := range map[string]int64{
		"alpha": 1_456_789, "odd": 262_144, "tiny": 1_999,
	} {
		if err := repo.SaveModelFacts(context.Background(), sqlite.ModelFactsRow{
			ProviderID: "openai", ModelID: modelID, Layer: "provider", ContextWindow: &window,
		}); err != nil {
			t.Fatalf("store the facts for %s: %v", modelID, err)
		}
	}
	snapshot, found := testkit.ReloadCatalog(t, db, gatedAccounts{active: 1}).Snapshot()
	if !found {
		t.Fatal("the catalog has no snapshot")
	}
	for modelID, want := range map[string]int64{
		"alpha": 1_457_000, "odd": 262_000, "tiny": 2_000,
	} {
		model, found := snapshot.Model("openai", modelID)
		if !found || model.ContextWindow == nil {
			t.Fatalf("Model(%s) = %+v, want the model with a window", modelID, model)
		}
		if *model.ContextWindow != want {
			t.Errorf("%s context = %d, want %d", modelID, *model.ContextWindow, want)
		}
	}
}

// TestTwoConnectionsSharingAModelGetTwoNames keeps both reachable: a picker
// shows one entry per connection, not one entry per upstream identifier.
func TestTwoConnectionsSharingAModelGetTwoNames(t *testing.T) {
	db := testkit.OpenTestDB(t)
	seedTwoConnections(t, db)
	models := testkit.ReloadCatalog(t, db, gatedAccounts{active: 1})
	snapshot, found := models.Snapshot()
	if !found {
		t.Fatal("the catalog has no snapshot")
	}
	listed := snapshot.Listed()
	names := map[string]bool{}
	for _, model := range listed {
		names[catalog.ClientModelID(model.ProviderID, model.ID)] = true
	}
	if !names["relo-openai-shared"] || !names["relo-claude-shared"] {
		t.Fatalf("listed = %v, want both connections' entries for the shared model", names)
	}
	for _, name := range []string{"relo-openai-shared", "relo-claude-shared"} {
		target, found := snapshot.ResolveClientID(name)
		if !found || target.ModelID != "shared" {
			t.Fatalf("ResolveClientID(%q) = %+v, %v", name, target, found)
		}
	}
}

func TestClaudeMillionContextModelGetsTwoRoutableNames(t *testing.T) {
	db := testkit.OpenTestDB(t)
	upstream := testkit.MockUpstream(t, map[string]string{})
	testkit.CustomProvider(t, db, testkit.Upstream{
		ID: "claude-work", Label: "Claude", APIFormat: catalog.FormatAnthropic,
		BaseURL: upstream.URL, Auth: catalog.AuthAPIKey,
		Models: []string{"opus"}, ModelsFormat: catalog.ModelsNone, ContextWindow: catalog.MillionContext,
	})
	if _, err := db.SQL().Exec("UPDATE providers SET template_id = 'claude' WHERE id = 'claude-work'"); err != nil {
		t.Fatalf("set provider template: %v", err)
	}
	models := testkit.ReloadCatalog(t, db, gatedAccounts{active: 1})
	router := routing.New(routing.Options{Catalog: models})

	standard, err := router.Plan(context.Background(), routing.Request{Model: "relo-claude-work-opus"})
	if err != nil || standard.ContextLimit != catalog.DefaultContext {
		t.Fatalf("standard plan = %+v, error = %v, want %d-token limit", standard, err, catalog.DefaultContext)
	}
	million, err := router.Plan(context.Background(), routing.Request{Model: "relo-claude-work-opus-1m"})
	if err != nil || million.ContextLimit != 0 {
		t.Fatalf("1M plan = %+v, error = %v, want no default limit", million, err)
	}
	anthropic, err := router.Plan(context.Background(), routing.Request{
		Model: "claude-relo-claude-work--opus[1m]",
	})
	if err != nil || anthropic.ContextLimit != 0 {
		t.Fatalf("Anthropic 1M plan = %+v, error = %v, want no default limit", anthropic, err)
	}
}

// TestDesktopAliasesResolveToTheirModels keeps the default opaque ids
// routable and the 1M spelling distinct.
func TestDesktopAliasesResolveToTheirModels(t *testing.T) {
	db := testkit.OpenTestDB(t)
	seedTwoConnections(t, db)
	models := testkit.ReloadCatalog(t, db, gatedAccounts{active: 1})
	snapshot, found := models.Snapshot()
	if !found {
		t.Fatal("the catalog has no snapshot")
	}
	seen := map[string]bool{}
	for _, model := range snapshot.Listed() {
		window := int64(catalog.MillionContext)
		picker := catalog.AnthropicModelAlias(model.ProviderID, model.ID, &window)
		alias := catalog.DesktopModelAlias(picker)
		if seen[alias] {
			t.Fatalf("alias %q is shared by two models", alias)
		}
		seen[alias] = true
		defaultAlias := catalog.DesktopModelAlias(catalog.AnthropicModelAlias(model.ProviderID, model.ID, nil))
		if alias == defaultAlias {
			t.Fatalf("1M alias %q equals default alias", alias)
		}
		target, found := snapshot.ResolveClientID(defaultAlias)
		if !found || target.ProviderID != model.ProviderID || target.ModelID != model.ID {
			t.Fatalf("ResolveClientID(%q) = %+v, %v, want %s/%s", defaultAlias, target, found, model.ProviderID, model.ID)
		}
	}
	if len(seen) == 0 {
		t.Fatal("no model was listed")
	}
}

// TestCollidingPublicNamesRefuseToLoad is the honesty rule: two entries that
// slug to one name would leave one unreachable, so the catalog reports both
// instead of publishing a duplicate.
func TestCollidingPublicNamesRefuseToLoad(t *testing.T) {
	db := testkit.OpenTestDB(t)
	upstream := testkit.MockUpstream(t, map[string]string{})
	// `a-b` + `c` and `a` + `b-c` both slug to relo-a-b-c.
	testkit.CustomProvider(t, db, testkit.Upstream{
		ID: "a-b", Label: "A B", APIFormat: catalog.FormatOpenAIChat,
		BaseURL: upstream.URL, Auth: catalog.AuthAPIKey,
		Models: []string{"c"}, ModelsFormat: catalog.ModelsNone,
	})
	testkit.CustomProvider(t, db, testkit.Upstream{
		ID: "a", Label: "A", APIFormat: catalog.FormatOpenAIChat,
		BaseURL: upstream.URL, Auth: catalog.AuthAPIKey,
		Models: []string{"b-c"}, ModelsFormat: catalog.ModelsNone,
	})
	built := catalog.New(sqlite.NewCatalogReader(db), gatedAccounts{active: 1}, formats.New())
	err := built.Reload(context.Background())
	if !errors.Is(err, catalog.ErrPublicIDCollision) {
		t.Fatalf("Reload() error = %v, want %v", err, catalog.ErrPublicIDCollision)
	}
	message := err.Error()
	for _, want := range []string{"a-b/c", "a/b-c"} {
		if !strings.Contains(message, want) {
			t.Fatalf("Reload() error = %q, want it to name %q", message, want)
		}
	}
}

// TestRetiredPublicNamesDoNotResolve keeps the switch honest: the spellings
// Relo published before the slug scheme are no longer names anything answers.
func TestRetiredPublicNamesDoNotResolve(t *testing.T) {
	router, _ := newRouter(t, catalog.StrategyPriority)
	ctx := context.Background()
	for _, retired := range []string{"relo/openai/alpha", "relo-alias"} {
		if _, err := router.Plan(ctx, routing.Request{Model: retired}); err == nil {
			t.Errorf("Plan(%q) error = nil, want the retired spelling refused", retired)
		}
	}
}
