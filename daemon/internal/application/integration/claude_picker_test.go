package integration

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/adapters/codingclients"
	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/tests/testkit"
)

type activeAccounts struct{}

func (activeAccounts) Active(string) int                         { return 1 }
func (activeAccounts) Available(string) bool                     { return true }
func (activeAccounts) CooldownUntil(string) time.Time            { return time.Time{} }
func (activeAccounts) ServesModel(string, catalog.Model) bool    { return true }
func (activeAccounts) Coverage(string, catalog.Model) (int, int) { return 1, 1 }

// TestClaudePickerNamesStayOffOtherClients pins the two lists a catalog
// publishes: Claude's cache says "Model by Provider", and the list other
// clients write keeps the catalog name and the Relo route label.
func TestClaudePickerNamesStayOffOtherClients(t *testing.T) {
	db := testkit.OpenTestDB(t)
	upstream := testkit.MockUpstream(t, map[string]string{})
	testkit.CustomProvider(t, db, testkit.Upstream{
		ID: "google-antigravity", Label: "Google Antigravity", APIFormat: catalog.FormatAnthropic,
		BaseURL: upstream.URL, Auth: catalog.AuthAPIKey,
		Models: []string{"opus"}, ModelsFormat: catalog.ModelsNone, ContextWindow: 1_000_000,
	})
	testkit.CustomProvider(t, db, testkit.Upstream{
		ID: "openai", Label: "OpenAI", APIFormat: catalog.FormatOpenAIChat,
		BaseURL: upstream.URL, Auth: catalog.AuthAPIKey,
		Models: []string{"gpt-4o"}, ModelsFormat: catalog.ModelsNone, ContextWindow: 128_000,
	})
	repo := sqlite.NewCatalogRepo(db)
	err := repo.SaveRoute(context.Background(), catalog.RouteRecord{
		ID: "combo", Label: "Combo", Strategy: string(catalog.StrategyPriority),
		Enabled: true, Listed: true,
		Members: []catalog.RouteMemberRecord{{
			ProviderID: "google-antigravity", ModelID: "opus", Kind: catalog.MemberKindModel, Weight: 1, Enabled: true,
		}},
	})
	if err != nil {
		t.Fatalf("SaveRoute() error = %v", err)
	}
	built := testkit.ReloadCatalog(t, db, activeAccounts{})
	service := NewService(ServiceOptions{Catalog: built})

	plain := indexModels(t, service.publishedModels())
	if plain["relo-google-antigravity-opus-1m"].Name != "opus" || plain["relo-openai-gpt-4o"].Name != "gpt-4o" {
		t.Fatalf("published = %+v, want the catalog names", plain)
	}
	if _, found := plain["relo-google-antigravity-opus"]; found {
		t.Fatalf("published = %+v, want the million-token model under the suffix alone", plain)
	}
	if plain["reloc-combo-1m"].Name != "Combo" || plain["reloc-combo-1m"].Connection != catalog.RouteLabelPrefix || plain["reloc-combo-1m"].ContextWindow == nil || *plain["reloc-combo-1m"].ContextWindow != 1_000_000 {
		t.Fatalf("published route = %+v, want Combo by Relo at the member window", plain["reloc-combo-1m"])
	}
	if _, found := plain["reloc-combo"]; found {
		t.Fatalf("published = %+v, want the million-token route under the suffix alone", plain)
	}

	claude := indexModels(t, service.claudePickerModels())
	if claude["claude-relo-google-antigravity--opus[1m]"].Name != "opus" || claude["claude-relo-google-antigravity--opus[1m]"].Connection != "Google Antigravity" {
		t.Fatalf("claude opus = %+v, want the million-token picker fields", claude["claude-relo-google-antigravity--opus[1m]"])
	}
	if claude["claude-relo-openai--gpt-4o"].Name != "gpt-4o" || claude["claude-relo-openai--gpt-4o"].Connection != "OpenAI" {
		t.Fatalf("claude gpt-4o = %+v, want the unmarked picker fields", claude["claude-relo-openai--gpt-4o"])
	}
	if claude["claude-reloc-combo[1m]"].Name != "Combo" || claude["claude-reloc-combo[1m]"].Connection != catalog.RouteLabelPrefix || claude["claude-reloc-combo[1m]"].ContextWindow == nil || *claude["claude-reloc-combo[1m]"].ContextWindow != 1_000_000 {
		t.Fatalf("claude route = %+v, want Combo by Relo at the member window", claude["claude-reloc-combo[1m]"])
	}
	if _, found := claude["claude-reloc-combo"]; found {
		t.Fatalf("claude = %+v, want the million-token route under the suffix alone", claude)
	}

	home := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", home)
	refresh := NewService(ServiceOptions{
		Catalog: built, Store: claudeEnabled{},
		Paths: testPaths{home: home, anthropicBase: "http://127.0.0.1:10202"},
		Files: testFiles{},
	})
	refresh.RefreshClaudeGateway(context.Background())
	content, err := os.ReadFile(codingclients.GatewayCachePath(home))
	if err != nil {
		t.Fatalf("read the cache: %v", err)
	}
	text := string(content)
	for _, want := range []string{
		"opus 1M on Google Antigravity", "claude-relo-google-antigravity--opus[1m]",
		"gpt-4o on OpenAI", "claude-relo-openai--gpt-4o",
		"Combo 1M on Relo", "claude-reloc-combo[1m]",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("cache = %s, want %s", text, want)
		}
	}
	if strings.Contains(text, "claude-relo-openai-gpt-4o[1m]") || strings.Contains(text, "Relo | Combo") {
		t.Fatalf("cache = %s, want the short model unmarked and the route in the picker form", text)
	}
}

func TestClaudeSignInPublishesDefaultAndMillionEntries(t *testing.T) {
	db := testkit.OpenTestDB(t)
	upstream := testkit.MockUpstream(t, map[string]string{})
	testkit.CustomProvider(t, db, testkit.Upstream{
		ID: "claude-work", Label: "Claude Work", APIFormat: catalog.FormatAnthropic,
		BaseURL: upstream.URL, Auth: catalog.AuthAPIKey,
		Models: []string{"opus"}, ModelsFormat: catalog.ModelsNone, ContextWindow: catalog.MillionContext,
	})
	if _, err := db.SQL().Exec("UPDATE providers SET template_id = 'claude' WHERE id = 'claude-work'"); err != nil {
		t.Fatalf("set provider template: %v", err)
	}
	if err := sqlite.NewCatalogRepo(db).SaveRoute(context.Background(), catalog.RouteRecord{
		ID: "claude-route", Label: "Claude route", Strategy: string(catalog.StrategyPriority),
		Enabled: true, Listed: true,
		Members: []catalog.RouteMemberRecord{{
			ProviderID: "claude-work", ModelID: "opus", Kind: catalog.MemberKindModel,
			Weight: 1, Enabled: true,
		}},
	}); err != nil {
		t.Fatalf("save Claude route: %v", err)
	}
	built := testkit.ReloadCatalog(t, db, activeAccounts{})
	service := NewService(ServiceOptions{Catalog: built})

	plain := indexModels(t, service.publishedModels())
	for _, id := range []string{
		"relo-claude-work-opus", "relo-claude-work-opus-1m",
		"reloc-claude-route", "reloc-claude-route-1m",
	} {
		if _, found := plain[id]; !found {
			t.Fatalf("published models = %+v, want %s", plain, id)
		}
	}
	if *plain["relo-claude-work-opus"].ContextWindow != catalog.DefaultContext {
		t.Fatalf("default window = %d, want %d", *plain["relo-claude-work-opus"].ContextWindow, catalog.DefaultContext)
	}

	claude := indexModels(t, service.claudePickerModels())
	for _, id := range []string{
		"claude-relo-claude-work--opus",
		"claude-relo-claude-work--opus[1m]",
		"claude-reloc-claude-route",
		"claude-reloc-claude-route[1m]",
	} {
		if _, found := claude[id]; !found {
			t.Fatalf("Claude models = %+v, want %s", claude, id)
		}
	}
}

// TestClaudeSignInDropsTheVendorWord pins the picker label on the Claude.ai
// sign-in: a model the provider names Claude Opus 5.5 reads as Opus 5.5 on
// Claude.ai, while the same model on another connection keeps its name and
// takes the generated suffix.
func TestClaudeSignInDropsTheVendorWord(t *testing.T) {
	db := testkit.OpenTestDB(t)
	upstream := testkit.MockUpstream(t, map[string]string{})
	for _, conn := range []struct{ id, label, template string }{
		{id: "claude-ai", label: "Claude.ai", template: "claude"},
		{id: "claude-api", label: "Claude API", template: "anthropic"},
	} {
		testkit.CustomProvider(t, db, testkit.Upstream{
			ID: conn.id, Label: conn.label, APIFormat: catalog.FormatAnthropic,
			BaseURL: upstream.URL, Auth: catalog.AuthAPIKey,
			Models: []string{"claude-opus-5-5"}, ModelsFormat: catalog.ModelsNone, ContextWindow: catalog.MillionContext,
		})
		if _, err := db.SQL().Exec("UPDATE providers SET template_id = ? WHERE id = ?", conn.template, conn.id); err != nil {
			t.Fatalf("set provider template: %v", err)
		}
		name := "Claude Opus 5.5"
		if _, err := db.SQL().Exec("UPDATE model_facts SET name = ? WHERE provider_id = ? AND model_id = ? AND layer = 'override'", name, conn.id, "claude-opus-5-5"); err != nil {
			t.Fatalf("name the model: %v", err)
		}
	}
	service := NewService(ServiceOptions{Catalog: testkit.ReloadCatalog(t, db, activeAccounts{})})

	plain := indexModels(t, service.publishedModels())
	if len(plain) != 2 {
		t.Fatalf("published = %+v, want one entry per connection", plain)
	}
	signIn := plain["relo-claude-ai-claude-opus-5-5"]
	if catalog.ConfigModelName(signIn.Name, signIn.Connection, signIn.ContextWindow) != "Opus 5.5 1M on Claude.ai" {
		t.Fatalf("published = %+v, want the vendor word dropped on the sign-in", plain)
	}
	api := plain["relo-claude-api-claude-opus-5-5-1m"]
	if catalog.ConfigModelName(api.Name, api.Connection, api.ContextWindow) != "Claude Opus 5.5 1M on Claude API" {
		t.Fatalf("published = %+v, want the name kept elsewhere", plain)
	}

	claude := indexModels(t, service.claudePickerModels())
	if len(claude) != 2 {
		t.Fatalf("claude = %+v, want one entry per connection", claude)
	}
	if _, found := claude["claude-relo-claude-ai--claude-opus-5-5"]; !found {
		t.Fatalf("claude = %+v, want the bare native name on the sign-in", claude)
	}
	if _, found := claude["claude-relo-claude-api--claude-opus-5-5[1m]"]; !found {
		t.Fatalf("claude = %+v, want the suffixed name elsewhere", claude)
	}
}

func indexModels(t *testing.T, models []ModelRef) map[string]ModelRef {
	t.Helper()
	indexed := make(map[string]ModelRef, len(models))
	for _, model := range models {
		indexed[model.ID] = model
	}
	return indexed
}

type claudeEnabled struct{}

func (claudeEnabled) Integration(_ context.Context, id string) (Record, bool, error) {
	if id != "claude-code" {
		return Record{}, false, nil
	}
	return Record{ID: id, Enabled: true}, true, nil
}

func (claudeEnabled) SaveIntegration(context.Context, Record) error { return nil }
func (claudeEnabled) Files(context.Context, string) ([]FileRecord, error) {
	return nil, nil
}
func (claudeEnabled) SaveFile(context.Context, FileRecord) error { return nil }
func (claudeEnabled) DeleteFiles(context.Context, string) error  { return nil }
func (claudeEnabled) AppendOp(context.Context, OpRecord) error   { return nil }

// TestAProvidersOwnMillionTokenSpellingIsPublishedOnce pins both shapes a
// picker reads. A router that publishes kimi-k3 and kimi-k3[1M] gives the
// second model the name Relo derives for a suffixed twin of the first, so the
// derived suffix yields: the base model keeps its bare name at its own window
// rather than shadowing the provider's own spelling. The OpenAI list carries
// the -1m spelling only on the provider's own model, and the Claude list the
// bracketed one, and neither names a model twice.
func TestAProvidersOwnMillionTokenSpellingIsPublishedOnce(t *testing.T) {
	db := testkit.OpenTestDB(t)
	upstream := testkit.MockUpstream(t, map[string]string{})
	testkit.CustomProvider(t, db, testkit.Upstream{
		ID: "teamorouter", Label: "Teamo Router", APIFormat: catalog.FormatOpenAIChat,
		BaseURL: upstream.URL, Auth: catalog.AuthAPIKey, ModelsFormat: catalog.ModelsNone,
		Models: []string{"kimi-k3", "kimi-k3[1M]"}, ContextWindow: catalog.MillionContext,
	})
	service := NewService(ServiceOptions{Catalog: testkit.ReloadCatalog(t, db, activeAccounts{})})

	plain := service.publishedModels()
	if len(plain) != 2 {
		t.Fatalf("openai list = %+v, want one entry per model", plain)
	}
	openai := indexModels(t, plain)
	for _, id := range []string{"relo-teamorouter-kimi-k3", "relo-teamorouter-kimi-k3-1m"} {
		if got := openai[id].ContextWindow; got == nil || *got != catalog.MillionContext {
			t.Errorf("%s = %+v, want the full window, since no twin was published", id, openai[id])
		}
	}
	if openai["relo-teamorouter-kimi-k3-1m"].Upstream != "kimi-k3[1M]" {
		t.Errorf("openai list = %+v, want the -1m name to serve the provider's own spelling", plain)
	}

	claude := service.claudePickerModels()
	if len(claude) != 2 {
		t.Fatalf("claude list = %+v, want one entry per model", claude)
	}
	picker := indexModels(t, claude)
	if _, found := picker["claude-relo-teamorouter--kimi-k3[1M]"]; !found {
		t.Errorf("claude list = %+v, want the provider's own marker on the large model", claude)
	}
	if _, found := picker["claude-relo-teamorouter--kimi-k3"]; !found {
		t.Errorf("claude list = %+v, want the other model's name left alone", claude)
	}
	if _, found := picker["claude-relo-teamorouter--kimi-k3[1m]"]; found {
		t.Errorf("claude list = %+v, want no derived suffix beside the provider's own spelling", claude)
	}
}
