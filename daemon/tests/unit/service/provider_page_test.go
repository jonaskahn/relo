// The provider page reads the catalog through these calls, so each one is
// checked against a real database, a real catalog and a local models.dev
// server rather than a stub.
package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/adapters/discovery"
	"github.com/jonaskahn/relo/internal/adapters/modelsdev"
	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	"github.com/jonaskahn/relo/internal/adapters/templates"
	appaccount "github.com/jonaskahn/relo/internal/application/account"
	appcatalog "github.com/jonaskahn/relo/internal/application/catalog"
	approuting "github.com/jonaskahn/relo/internal/application/routing"
	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/tests/testkit"
)

// seedRow stores one provider row exactly as written, which is how a test
// builds a provider a template describes.
func seedRow(t *testing.T, h *harness, row sqlite.ProviderRow) {
	t.Helper()
	if row.Headers == nil {
		row.Headers = map[string]string{}
	}
	if row.Variables == nil {
		row.Variables = map[string]string{}
	}
	if row.PoolStrategy == "" {
		row.PoolStrategy = sqlite.StrategyLeastLoaded
	}
	repo := sqlite.NewCatalogRepo(h.db)
	if err := repo.SaveProvider(context.Background(), row); err != nil {
		t.Fatalf("save provider %s: %v", row.ID, err)
	}
	model := sqlite.ModelRow{ProviderID: row.ID, ModelID: "model-1", Source: "manual", Enabled: true}
	if err := repo.SaveModel(context.Background(), model); err != nil {
		t.Fatalf("save the model of %s: %v", row.ID, err)
	}
	if err := h.catalog.Reload(context.Background()); err != nil {
		t.Fatalf("reload catalog: %v", err)
	}
}

// pageService is a harness whose models.dev catalog is served from this
// process, so nothing in these tests reaches the network.
func pageService(t *testing.T, h *harness, catalogURL string) *session {
	t.Helper()
	client := &http.Client{Timeout: 5 * time.Second}
	return h.buildServiceWith(h.catalog, h.pools, testkit.SecretStore(h.secrets),
		modelsdev.NewDirectory(filepath.Join(h.home, "cache"), catalogURL, client),
		discovery.New(client))
}

// catalogFixture is the trimmed models.dev document these tests read.
func catalogFixture() map[string]map[string]any {
	return map[string]map[string]any{
		"deepseek": {
			"id": "deepseek", "name": "DeepSeek", "npm": "@ai-sdk/deepseek",
			"api": "https://api.deepseek.com",
			"models": map[string]any{
				"deepseek-chat": map[string]any{
					"id": "deepseek-chat", "name": "DeepSeek Chat", "status": "active",
					"cost":  map[string]any{"input": 0.28, "output": 0.42},
					"limit": map[string]any{"context": 128000, "output": 8192},
				},
				"deepseek-reasoner": map[string]any{
					"id": "deepseek-reasoner", "name": "DeepSeek Reasoner", "status": "active",
					"cost":  map[string]any{"input": 0.55, "output": 2.19},
					"limit": map[string]any{"context": 128000, "output": 65536},
				},
			},
		},
	}
}

// deepseekRow is the provider row a template describes, which is what the
// page reads its kind, formats and variables from.
func deepseekRow() sqlite.ProviderRow {
	return sqlite.ProviderRow{
		ID: "deepseek", TemplateID: "deepseek", Origin: string(catalog.OriginTemplate),
		Label: "DeepSeek", Auth: string(catalog.AuthAPIKey),
		APIFormat: string(catalog.FormatAnthropic), KeyHeader: string(catalog.KeyHeaderBearer),
		BaseURL: "https://api.deepseek.com/anthropic/v1", ModelsFormat: string(catalog.ModelsAnthropic),
		ModelsDevProviderID: "deepseek", Enabled: true, Rank: 100,
	}
}

func TestProviderResponsesCarryTheirKindAndFormats(t *testing.T) {
	h := newHarness(t)
	manager := pageService(t, h, catalogServer(t, catalogFixture()).URL)
	seedRow(t, h, deepseekRow())
	seedRow(t, h, sqlite.ProviderRow{
		ID: "amazon-bedrock", TemplateID: "amazon-bedrock", Origin: string(catalog.OriginTemplate),
		Label: "Amazon Bedrock", Auth: string(catalog.AuthAWS),
		APIFormat:    string(catalog.FormatBedrockConverse),
		BaseURL:      "https://bedrock-runtime.us-east-1.amazonaws.com",
		ModelsFormat: string(catalog.ModelsBedrock), ModelsDevProviderID: "amazon-bedrock",
		Variables: map[string]string{"region": "us-east-1"}, Enabled: true, Rank: 100,
	})

	host, err := manager.Provider(context.Background(), "deepseek")
	if err != nil {
		t.Fatalf("Provider() error = %v", err)
	}
	if host.Kind != "key" {
		t.Fatalf("kind = %q, want the API key section", host.Kind)
	}
	if len(host.AvailableFormats) < 2 {
		t.Fatalf("available formats = %+v, want the two shapes DeepSeek speaks", host.AvailableFormats)
	}
	bedrock, err := manager.Provider(context.Background(), "amazon-bedrock")
	if err != nil {
		t.Fatalf("Provider(amazon-bedrock) error = %v", err)
	}
	if bedrock.Kind != "cloud" {
		t.Fatalf("kind = %q, want the cloud section", bedrock.Kind)
	}
	if len(bedrock.VariableDefs) == 0 {
		t.Fatal("variable definitions are empty, want the region the base URL asks for")
	}
}

func TestProviderResponsesCountPausedAndReauthAccounts(t *testing.T) {
	h := newHarness(t)
	manager := pageService(t, h, catalogServer(t, catalogFixture()).URL)
	first := h.addAccount("openai", "work", "sk-1")
	second := h.addAccount("openai", "spare", "sk-2")

	if err := manager.accounts.PauseAccount(context.Background(), first.ID); err != nil {
		t.Fatalf("PauseAccount() error = %v", err)
	}
	if err := h.pools.MarkNeedsReauth(context.Background(), "openai", second.ID); err != nil {
		t.Fatalf("mark the account: %v", err)
	}
	host, err := manager.Provider(context.Background(), "openai")
	if err != nil {
		t.Fatalf("Provider() error = %v", err)
	}
	if host.Counts.PausedAccounts != 1 || host.Counts.ReauthAccounts != 1 {
		t.Fatalf("counts = %+v, want one paused and one needing a sign-in", host.Counts)
	}
	seedRow(t, h, sqlite.ProviderRow{
		ID: "claude", TemplateID: "claude", Origin: string(catalog.OriginTemplate),
		Label: "Claude", Auth: string(catalog.AuthOAuth),
		APIFormat: string(catalog.FormatAnthropic), BaseURL: "https://api.anthropic.com/v1",
		ModelsFormat: string(catalog.ModelsAnthropic), LoginFlows: []string{"claude"},
		Enabled: true, Rank: 100,
	})
	claude, err := manager.Provider(context.Background(), "claude")
	if err != nil {
		t.Fatalf("Provider(claude) error = %v", err)
	}
	if len(claude.LoginMethods) != 1 || claude.LoginMethods[0].Kind != templates.LoginBrowser {
		t.Fatalf("login methods = %+v, want the browser sign-in the template declares", claude.LoginMethods)
	}
}

func TestTemplatesDeclareTheirLoginMethods(t *testing.T) {
	h := newHarness(t)
	manager := pageService(t, h, catalogServer(t, catalogFixture()).URL)

	list, _, err := manager.Templates(context.Background())
	if err != nil {
		t.Fatalf("Templates() error = %v", err)
	}
	byID := map[string]templates.Template{}
	for _, entry := range list {
		byID[entry.ID] = entry
	}
	codex, found := byID["openai-codex"]
	if !found {
		t.Fatal("the ChatGPT sign-in is missing from the template list")
	}
	kinds := map[templates.LoginKind]bool{}
	for _, method := range codex.LoginMethods {
		kinds[method.Kind] = true
	}
	if !kinds[templates.LoginBrowser] || !kinds[templates.LoginDevice] {
		t.Fatalf("login kinds = %+v, want a browser and a device login", kinds)
	}
	// The cloud templates own the ids they cover, so a generated API key row
	// never offers one of them a second time with the wrong credentials.
	if _, offered := byID["azure"]; offered {
		t.Fatal("azure is offered as an API key row beside the Azure cloud template")
	}
	for _, id := range []string{"azure-openai", "amazon-bedrock", "google-vertex"} {
		entry, offered := byID[id]
		if !offered {
			t.Fatalf("the cloud row %s is missing", id)
		}
		if entry.Kind != templates.KindCloud {
			t.Fatalf("%s is a %s row, want a cloud row", id, entry.Kind)
		}
	}
}

func TestPatchModelPricedAsRewritesTheLayer(t *testing.T) {
	h := newHarness(t)
	manager := pageService(t, h, catalogServer(t, catalogFixture()).URL)
	if _, err := manager.modelsDev.Get(context.Background(), modelsdev.ModeForce); err != nil {
		t.Fatalf("fetch the catalog: %v", err)
	}
	seedRow(t, h, deepseekRow())
	seedRow(t, h, sqlite.ProviderRow{
		ID: "deepseek-chat", TemplateID: "deepseek", Origin: string(catalog.OriginTemplate),
		Label: "DeepSeek", Auth: string(catalog.AuthAPIKey),
		APIFormat: string(catalog.FormatOpenAIChat), BaseURL: "https://api.deepseek.com/v1",
		ModelsFormat: string(catalog.ModelsOpenAI), ModelsDevProviderID: "deepseek",
		Enabled: true, Rank: 100,
	})
	repo := sqlite.NewCatalogRepo(h.db)
	if err := repo.SaveModel(context.Background(), sqlite.ModelRow{
		ProviderID: "deepseek-chat", ModelID: "deepseek-chat", Source: "listing", Enabled: true,
	}); err != nil {
		t.Fatalf("save the matching model: %v", err)
	}
	if err := h.catalog.Reload(context.Background()); err != nil {
		t.Fatalf("reload catalog: %v", err)
	}

	ref := "deepseek/deepseek-chat"
	patched, err := manager.PatchModel(context.Background(), "deepseek", "model-1", appcatalog.ModelPatch{PricedAs: &ref})
	if err != nil {
		t.Fatalf("PatchModel() error = %v", err)
	}
	if patched.Match != string(modelsdev.MatchManual) || patched.ModelsDevRef != ref {
		t.Fatalf("match = %q ref = %q, want the reference the operator chose", patched.Match, patched.ModelsDevRef)
	}
	if patched.Prices.ModelsDev.Input == nil {
		t.Fatal("the models.dev layer is empty, want the price of the reference just chosen")
	}
	if !patched.Overridden {
		t.Fatal("a model priced by hand does not read as overridden")
	}

	unknown := "deepseek/nope"
	if _, err := manager.PatchModel(context.Background(), "deepseek", "model-1", appcatalog.ModelPatch{PricedAs: &unknown}); !errors.Is(err, appcatalog.ErrUnknownModelsDevRef) {
		t.Fatalf("PatchModel(unknown) error = %v, want a refusal", err)
	}

	empty := ""
	restored, err := manager.PatchModel(context.Background(), "deepseek-chat", "deepseek-chat", appcatalog.ModelPatch{PricedAs: &empty})
	if err != nil {
		t.Fatalf("PatchModel(empty) error = %v", err)
	}
	if restored.Match == string(modelsdev.MatchManual) {
		t.Fatalf("match = %q, want automatic matching again", restored.Match)
	}
	if restored.Prices.ModelsDev.Input == nil {
		t.Fatal("automatic matching left the models.dev layer empty")
	}
	// A model automatic matching cannot find loses the layer rather than
	// keeping rates that came from somewhere else.
	dropped, err := manager.PatchModel(context.Background(), "deepseek", "model-1", appcatalog.ModelPatch{PricedAs: &empty})
	if err != nil {
		t.Fatalf("PatchModel(empty) error = %v", err)
	}
	if dropped.Prices.ModelsDev.Input != nil {
		t.Fatalf("models.dev price = %v, want no layer for an id nothing matches", dropped.Prices.ModelsDev.Input)
	}
}

func TestPatchModelOverrideReplacesAndClearsTheLayer(t *testing.T) {
	h := newHarness(t)
	manager := pageService(t, h, catalogServer(t, catalogFixture()).URL)

	category := "reasoning"
	window := int64(200000)
	input := int64(2_000_000)
	patched, err := manager.PatchModel(context.Background(), "openai", "model-1", appcatalog.ModelPatch{
		Override: &appcatalog.ModelOverride{
			Category: &category, ContextWindow: &window, Prices: &appcatalog.Prices{Input: &input},
		},
	})
	if err != nil {
		t.Fatalf("PatchModel() error = %v", err)
	}
	if patched.Category != "reasoning" || patched.ContextWindow == nil || *patched.ContextWindow != window {
		t.Fatalf("model = %+v, want the override to win", patched)
	}
	if patched.Details == nil || patched.Details.EffectiveSource.Category != "override" {
		t.Fatalf("details = %+v, want the category to come from the override", patched.Details)
	}
	if patched.Prices.EffectiveSource.Input != "override" {
		t.Fatalf("price source = %q, want the override", patched.Prices.EffectiveSource.Input)
	}

	cleared, err := manager.PatchModel(context.Background(), "openai", "model-1", appcatalog.ModelPatch{
		Override: &appcatalog.ModelOverride{},
	})
	if err != nil {
		t.Fatalf("PatchModel(clear) error = %v", err)
	}
	if cleared.Details != nil && cleared.Details.Override.Category != nil {
		t.Fatalf("override layer = %+v, want it cleared", cleared.Details.Override)
	}
	if cleared.Overridden {
		t.Fatal("a cleared override still reads as overridden")
	}
}

func TestPatchModelRefusesAnUnknownCategory(t *testing.T) {
	h := newHarness(t)
	manager := pageService(t, h, catalogServer(t, catalogFixture()).URL)
	bad := "not-a-category"
	if _, err := manager.PatchModel(context.Background(), "openai", "model-1", appcatalog.ModelPatch{
		Override: &appcatalog.ModelOverride{Category: &bad},
	}); !errors.Is(err, appcatalog.ErrInvalidCatalogRow) {
		t.Fatalf("PatchModel() error = %v, want a refusal", err)
	}
}

func TestAddAndDeleteAModelByHand(t *testing.T) {
	h := newHarness(t)
	manager := pageService(t, h, catalogServer(t, catalogFixture()).URL)
	if _, err := manager.modelsDev.Get(context.Background(), modelsdev.ModeForce); err != nil {
		t.Fatalf("fetch the catalog: %v", err)
	}

	added, err := manager.AddModel(context.Background(), "openai", "ft:gpt-4o:acme", "deepseek/deepseek-chat")
	if err != nil {
		t.Fatalf("AddModel() error = %v", err)
	}
	if added.Source != appcatalog.SourceManual || !added.Enabled {
		t.Fatalf("added = %+v, want a manual model that starts on", added)
	}
	if added.Prices.ModelsDev.Input == nil {
		t.Fatal("the added model is not priced from the reference it named")
	}
	if _, err := manager.AddModel(context.Background(), "openai", "ft:gpt-4o:acme", ""); !errors.Is(err, appcatalog.ErrCatalogConflict) {
		t.Fatalf("AddModel(duplicate) error = %v, want a conflict", err)
	}

	// A model the listing publishes cannot be deleted: the next refresh would
	// only put it back.
	repo := sqlite.NewCatalogRepo(h.db)
	if err := repo.SaveModel(context.Background(), sqlite.ModelRow{
		ProviderID: "openai", ModelID: "gpt-5", Source: "listing", Enabled: true,
	}); err != nil {
		t.Fatalf("save a listed model: %v", err)
	}
	if err := h.catalog.Reload(context.Background()); err != nil {
		t.Fatalf("reload catalog: %v", err)
	}
	if err := manager.DeleteModel(context.Background(), "openai", "gpt-5"); !errors.Is(err, appcatalog.ErrNotCustom) {
		t.Fatalf("DeleteModel(listed) error = %v, want a refusal a caller can fix", err)
	}
	if err := manager.DeleteModel(context.Background(), "openai", "ft:gpt-4o:acme"); err != nil {
		t.Fatalf("DeleteModel(manual) error = %v", err)
	}
	if _, err := manager.Model(context.Background(), "openai", "ft:gpt-4o:acme"); !errors.Is(err, appcatalog.ErrModelNotFound) {
		t.Fatalf("Model() error = %v, want the model to be gone", err)
	}
}

func TestDeleteModelIsRefusedByAGroupThatRoutesToIt(t *testing.T) {
	h := newHarness(t)
	manager := pageService(t, h, catalogServer(t, catalogFixture()).URL)
	if _, err := manager.AddModel(context.Background(), "openai", "ft:acme", ""); err != nil {
		t.Fatalf("AddModel() error = %v", err)
	}
	if err := manager.routes.Save(context.Background(), "fast", approuting.Write{
		Label: "Fast", Strategy: "priority", Enabled: true, Listed: true,
		Members: []approuting.MemberWrite{{ProviderID: "openai", ModelID: "ft:acme", Weight: 1, Enabled: true}},
	}); err != nil {
		t.Fatalf("SaveGroup() error = %v", err)
	}

	err := manager.DeleteModel(context.Background(), "openai", "ft:acme")
	if !errors.Is(err, appcatalog.ErrCatalogConflict) {
		t.Fatalf("DeleteModel() error = %v, want a conflict", err)
	}
	if !strings.Contains(err.Error(), "Fast") {
		t.Fatalf("DeleteModel() error = %q, want it to name the group", err)
	}
}

func TestModelFiltersFindUnpricedAndOverridden(t *testing.T) {
	h := newHarness(t)
	manager := pageService(t, h, catalogServer(t, catalogFixture()).URL)
	if _, err := manager.AddModel(context.Background(), "openai", "ft:acme", ""); err != nil {
		t.Fatalf("AddModel() error = %v", err)
	}
	category := "vision"
	if _, err := manager.PatchModel(context.Background(), "openai", "model-1", appcatalog.ModelPatch{
		Override: &appcatalog.ModelOverride{Category: &category},
	}); err != nil {
		t.Fatalf("PatchModel() error = %v", err)
	}

	unpriced, total, err := manager.Models(context.Background(), appcatalog.ModelQuery{Provider: "openai", Unpriced: true})
	if err != nil {
		t.Fatalf("Models(unpriced) error = %v", err)
	}
	if total != 2 || len(unpriced) != 2 {
		t.Fatalf("unpriced = %+v (total %d), want both models with no rate", unpriced, total)
	}

	overridden, total, err := manager.Models(context.Background(), appcatalog.ModelQuery{Provider: "openai", Overridden: true})
	if err != nil {
		t.Fatalf("Models(overridden) error = %v", err)
	}
	if total != 1 || len(overridden) != 1 || overridden[0].ModelID != "model-1" {
		t.Fatalf("overridden = %+v (total %d), want the model with an override", overridden, total)
	}
}

func TestModelsDevSearchAndStateReadTheSavedCopy(t *testing.T) {
	h := newHarness(t)
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		body, _ := json.Marshal(catalogFixture())
		_, _ = w.Write(body)
	}))
	t.Cleanup(server.Close)

	manager := pageService(t, h, server.URL)
	if _, err := manager.modelsDev.Get(context.Background(), modelsdev.ModeForce); err != nil {
		t.Fatalf("fetch the catalog: %v", err)
	}
	fetched := requests.Load()

	hits, state := manager.SearchModelsDev(context.Background(), "deepseek", 20)
	if requests.Load() != fetched {
		t.Fatalf("a search made %d requests, want none", requests.Load()-fetched)
	}
	if len(hits) == 0 || hits[0].Ref == "" {
		t.Fatalf("hits = %+v, want the saved copy to answer", hits)
	}
	if !state.Available {
		t.Fatalf("state = %+v, want the saved copy to read as available", state)
	}
	if _, err := manager.ModelsDevState(context.Background()); err != nil {
		t.Fatalf("ModelsDevState() error = %v", err)
	}
	if requests.Load() != fetched {
		t.Fatalf("reading the state made %d requests, want none", requests.Load()-fetched)
	}
}

// TestRefreshKeepsTheCatalogFactsItAlreadySaved covers a download that stops
// describing a model: the model keeps the facts already saved, so a rate an
// operator read does not silently disappear, and the report says how many
// models were left behind.
func TestRefreshKeepsTheCatalogFactsItAlreadySaved(t *testing.T) {
	h := newHarness(t)
	body := atomic.Value{}
	body.Store(fixtureJSON(t, catalogFixture()))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body.Load().([]byte))
	}))
	t.Cleanup(server.Close)

	manager := pageService(t, h, server.URL)
	seedRow(t, h, deepseekRow())
	repo := sqlite.NewCatalogRepo(h.db)
	if err := repo.SaveModel(context.Background(), sqlite.ModelRow{
		ProviderID: "deepseek", ModelID: "deepseek-chat", Source: appcatalog.SourceListing, Enabled: true,
	}); err != nil {
		t.Fatalf("save the listed model: %v", err)
	}
	if err := h.catalog.Reload(context.Background()); err != nil {
		t.Fatalf("reload catalog: %v", err)
	}

	first, err := manager.RefreshModelsDev(context.Background())
	if err != nil {
		t.Fatalf("RefreshModelsDev() error = %v", err)
	}
	if first.ModelsMatched == 0 || first.ModelsUnmatched != 0 {
		t.Fatalf("report = %+v, want the downloaded catalog to describe the model", first)
	}
	before, err := manager.Model(context.Background(), "deepseek", "deepseek-chat")
	if err != nil {
		t.Fatalf("Model() error = %v", err)
	}
	if before.Prices.ModelsDev.Input == nil {
		t.Fatal("catalog rate is empty, want the downloaded rate saved")
	}

	// The catalog stops describing the model while still naming the provider.
	body.Store(fixtureJSON(t, map[string]map[string]any{
		"deepseek": {"id": "deepseek", "name": "DeepSeek", "models": map[string]any{}},
	}))
	second, err := manager.RefreshModelsDev(context.Background())
	if err != nil {
		t.Fatalf("RefreshModelsDev() error = %v", err)
	}
	if second.ModelsMatched != 0 || second.ModelsUnmatched != 1 {
		t.Fatalf("report = %+v, want the model the catalog no longer describes reported", second)
	}

	after, err := manager.Model(context.Background(), "deepseek", "deepseek-chat")
	if err != nil {
		t.Fatalf("Model() error = %v", err)
	}
	if after.Prices.ModelsDev.Input == nil || *after.Prices.ModelsDev.Input != *before.Prices.ModelsDev.Input {
		t.Fatalf("catalog rate = %v, want the facts already saved kept", after.Prices.ModelsDev.Input)
	}
}

// fixtureJSON marshals one models.dev fixture for a server that answers it.
func fixtureJSON(t *testing.T, providers map[string]map[string]any) []byte {
	t.Helper()
	data, err := json.Marshal(providers)
	if err != nil {
		t.Fatalf("marshal the fixture: %v", err)
	}
	return data
}

// TestACommittedConnectionIsOneTheUpdateReads asserts the account a probe
// commit stored is in rotation straight away: the update reads a connection
// through its account, so a credential the pool never heard of leaves the
// connection out of the update until a restart.
func TestACommittedConnectionIsOneTheUpdateReads(t *testing.T) {
	h := newHarness(t)
	h.catalog = h.newCredentialCatalog()
	listing := listingServer(t, http.StatusOK, `{"data":[{"id":"listed-model","name":"Listed Model"}]}`)
	manager := pageService(t, h, catalogServer(t, catalogFixture()).URL)

	probe, err := manager.ProbeProvider(context.Background(), probeJSON(t,
		`{"custom_id":"lab","label":"Lab","api_format":"openai-chat","base_url":"`+listing.URL+
			`/v1","models_format":"openai","credential":{"kind":"api_key","secret":"sk-lab"}}`))
	if err != nil {
		t.Fatalf("ProbeProvider() error = %v", err)
	}
	if _, err := manager.CommitProbe(context.Background(), probe.ProbeID, appcatalog.CommitProbeRequest{
		ProviderID: "lab", Label: "Lab", AccountLabel: "lab",
	}); err != nil {
		t.Fatalf("CommitProbe() error = %v", err)
	}

	host, err := manager.Provider(context.Background(), "lab")
	if err != nil {
		t.Fatalf("Provider() error = %v", err)
	}
	if host.Counts.Accounts != 1 || !host.Configured {
		t.Fatalf("provider = %+v, want the committed account in rotation", host)
	}

	report, err := manager.RefreshCatalog(context.Background())
	if err != nil {
		t.Fatalf("RefreshCatalog() error = %v", err)
	}
	if report.Updated != 1 || report.Failed != 0 {
		t.Fatalf("report = %+v, want the connection the commit stored to be read", report)
	}
}

// TestRefreshCatalogUpdatesWhatItCanAndNamesWhatItCannot covers the console's
// one update action: every connected provider is read, a connection that
// failed keeps the roster it had, and the result names it rather than turning
// the whole update into the failure.
func TestRefreshCatalogUpdatesWhatItCanAndNamesWhatItCannot(t *testing.T) {
	h := newHarness(t)
	// The update reads which connections hold a credential, so the catalog has
	// to be the one the daemon builds.
	h.catalog = h.newCredentialCatalog()
	answered := listingServer(t, http.StatusOK, `{"data":[{"id":"from-the-provider","name":"From The Provider"}]}`)
	refused := listingServer(t, http.StatusServiceUnavailable, `{"error":"down"}`)
	manager := pageService(t, h, catalogServer(t, catalogFixture()).URL)
	seedRow(t, h, sqlite.ProviderRow{
		ID: "groq", TemplateID: "groq", Origin: string(catalog.OriginTemplate),
		Label: "Groq", Auth: string(catalog.AuthAPIKey),
		APIFormat: string(catalog.FormatOpenAIChat), BaseURL: answered.URL + "/v1",
		ModelsFormat: string(catalog.ModelsOpenAI), ModelsDevProviderID: "groq",
		Enabled: true, Rank: 100,
	})
	seedRow(t, h, sqlite.ProviderRow{
		ID: "together", Label: "Together", Origin: string(catalog.OriginCustom),
		Auth: string(catalog.AuthAPIKey), APIFormat: string(catalog.FormatOpenAIChat),
		BaseURL: refused.URL + "/v1", ModelsFormat: string(catalog.ModelsOpenAI),
		Enabled: true, Rank: 100,
	})
	// A connection with no account is not asked for a list it cannot reach.
	seedRow(t, h, sqlite.ProviderRow{
		ID: "deepseek", TemplateID: "deepseek", Origin: string(catalog.OriginTemplate),
		Label: "DeepSeek", Auth: string(catalog.AuthAPIKey),
		APIFormat: string(catalog.FormatAnthropic), BaseURL: "https://api.deepseek.com/anthropic/v1",
		ModelsFormat: string(catalog.ModelsAnthropic), ModelsDevProviderID: "deepseek",
		Enabled: true, Rank: 100,
	})
	// The accounts are stored through this service so the roster each one
	// starts is read from this test's own catalog and listing servers.
	for providerID, key := range map[string]string{"groq": "sk-1", "together": "sk-2"} {
		if _, err := manager.accounts.AddAccount(context.Background(), appaccount.NewAccount{
			ProviderID: providerID, Label: "work", SecretValue: key,
		}); err != nil {
			t.Fatalf("AddAccount(%s) error = %v", providerID, err)
		}
	}
	for _, providerID := range []string{"groq", "together"} {
		h.waitForRefresh(providerID)
	}
	if err := h.catalog.Reload(context.Background()); err != nil {
		t.Fatalf("reload catalog: %v", err)
	}

	result, err := manager.RefreshCatalog(context.Background())
	if err != nil {
		t.Fatalf("RefreshCatalog() error = %v", err)
	}
	if result.Updated != 1 || result.Failed != 1 || result.Skipped != 0 {
		t.Fatalf("result = %+v, want one connection updated and one failed", result)
	}
	if len(result.Providers) != 2 {
		t.Fatalf("providers = %+v, want one outcome per connected provider", result.Providers)
	}
	if result.Metadata.Providers == 0 || result.MetadataError != "" {
		t.Fatalf("metadata = %+v (error %q), want the downloaded catalog reported", result.Metadata, result.MetadataError)
	}

	outcomes := map[string]appcatalog.CatalogRefreshOutcome{}
	for _, outcome := range result.Providers {
		outcomes[outcome.ProviderID] = outcome
	}
	if outcomes["groq"].Status != appcatalog.RefreshUpdated || outcomes["groq"].Listed != 1 {
		t.Fatalf("groq = %+v, want the listing it answered with", outcomes["groq"])
	}
	if outcomes["together"].Status != appcatalog.RefreshFailed || outcomes["together"].Detail == "" {
		t.Fatalf("together = %+v, want the failure named beside the connection that worked", outcomes["together"])
	}

	models, _, err := manager.Models(context.Background(), appcatalog.ModelQuery{Provider: "together"})
	if err != nil {
		t.Fatalf("Models() error = %v", err)
	}
	if len(models) != 1 || models[0].ModelID != "model-1" {
		t.Fatalf("models = %+v, want the failed connection to keep the roster it had", models)
	}
	if models[0].Available != nil && !*models[0].Available {
		t.Fatalf("model = %+v, want a failed listing to leave its roster as it was", models[0])
	}
}

func TestRefreshModelsMapsARefusalToTheCodeAConsoleReads(t *testing.T) {
	for _, entry := range []struct {
		status int
		want   error
	}{
		{http.StatusUnauthorized, appcatalog.ErrCredentialRejected},
		{http.StatusServiceUnavailable, appcatalog.ErrListingFailed},
	} {
		h := newHarness(t)
		listing := listingServer(t, entry.status, `{"error":"no"}`)
		manager := pageService(t, h, catalogServer(t, catalogFixture()).URL)
		seedRow(t, h, sqlite.ProviderRow{
			ID: "groq", TemplateID: "groq", Origin: string(catalog.OriginTemplate),
			Label: "Groq", Auth: string(catalog.AuthAPIKey),
			APIFormat: string(catalog.FormatOpenAIChat), BaseURL: listing.URL + "/v1",
			ModelsFormat: string(catalog.ModelsOpenAI), ModelsDevProviderID: "groq",
			Enabled: true, Rank: 100,
		})
		h.addAccount("groq", "work", "sk-1")

		_, err := manager.RefreshProviderModels(context.Background(), "groq")
		if !errors.Is(err, entry.want) {
			t.Fatalf("status %d: RefreshProviderModels() error = %v, want %v", entry.status, err, entry.want)
		}
		host, readErr := manager.Provider(context.Background(), "groq")
		if readErr != nil {
			t.Fatalf("Provider() error = %v", readErr)
		}
		if host.LastRefreshError == "" {
			t.Fatalf("status %d: the failure was not recorded on the provider", entry.status)
		}
	}
}

// TestRefreshKeepsTheRosterOfAProviderThatPublishesNoList covers the refresh a
// connection with no listing dialect runs. Nothing is asked of the vendor, the
// models an operator typed stay exactly as they are, and the refresh says so
// rather than reading a catalog as evidence about the connection.
func TestRefreshKeepsTheRosterOfAProviderThatPublishesNoList(t *testing.T) {
	h := newHarness(t)
	var requests atomic.Int64
	unreached := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	t.Cleanup(unreached.Close)
	manager := pageService(t, h, catalogServer(t, catalogFixture()).URL)
	seedRow(t, h, sqlite.ProviderRow{
		ID: "no-list", Label: "No List", Origin: string(catalog.OriginCustom),
		Auth: string(catalog.AuthAPIKey), APIFormat: string(catalog.FormatOpenAIChat),
		BaseURL: unreached.URL + "/v1", ModelsFormat: string(catalog.ModelsNone),
		ModelsSource: "modelsdev", ModelsDevProviderID: "deepseek", Enabled: true, Rank: 100,
	})
	h.addAccount("no-list", "work", "sk-1")

	result, err := manager.RefreshProviderModels(context.Background(), "no-list")
	if err != nil {
		t.Fatalf("RefreshProviderModels() error = %v, want a connection with no list to refresh", err)
	}
	if result.Status != appcatalog.RefreshSkipped {
		t.Fatalf("status = %q, want the refresh to report the skip it is", result.Status)
	}
	if requests.Load() != 0 {
		t.Fatalf("the connection made %d listing requests, want none", requests.Load())
	}
	models, _, err := manager.Models(context.Background(), appcatalog.ModelQuery{Provider: "no-list"})
	if err != nil {
		t.Fatalf("Models() error = %v", err)
	}
	if len(models) != 1 || models[0].ModelID != "model-1" {
		t.Fatalf("models = %+v, want the typed model left alone", models)
	}
	if models[0].Available != nil && !*models[0].Available {
		t.Fatalf("model = %+v, want a typed model left switched on", models[0])
	}
	if _, found := modelByIDInto(models, "deepseek-chat"); found {
		t.Fatal("the refresh read the catalog as evidence, want the typed roster alone")
	}
}

// TestRefreshMarksTheRosterUnavailableWhenTheListIsEmpty covers the listing
// that answered and named nothing: the provider is the authority, so the ids
// it stopped naming are kept for the routes that reference them and switched
// off.
func TestRefreshMarksTheRosterUnavailableWhenTheListIsEmpty(t *testing.T) {
	h := newHarness(t)
	listing := listingServer(t, http.StatusOK, `{"data":[]}`)
	manager := pageService(t, h, catalogServer(t, catalogFixture()).URL)
	seedRow(t, h, sqlite.ProviderRow{
		ID: "groq", TemplateID: "groq", Origin: string(catalog.OriginTemplate),
		Label: "Groq", Auth: string(catalog.AuthAPIKey),
		APIFormat: string(catalog.FormatOpenAIChat), BaseURL: listing.URL + "/v1",
		ModelsFormat: string(catalog.ModelsOpenAI), ModelsDevProviderID: "groq",
		Enabled: true, Rank: 100,
	})
	h.addAccount("groq", "work", "sk-1")

	result, err := manager.RefreshProviderModels(context.Background(), "groq")
	if err != nil {
		t.Fatalf("RefreshProviderModels() error = %v", err)
	}
	if result.Status != appcatalog.RefreshUpdated || result.Listed != 0 || result.Unavailable != 1 {
		t.Fatalf("result = %+v, want the empty listing to switch its model off", result)
	}

	models, _, err := manager.Models(context.Background(), appcatalog.ModelQuery{Provider: "groq"})
	if err != nil {
		t.Fatalf("Models() error = %v", err)
	}
	if len(models) != 1 || models[0].Available == nil || *models[0].Available {
		t.Fatalf("models = %+v, want the id kept and marked unavailable", models)
	}
	host, err := manager.Provider(context.Background(), "groq")
	if err != nil {
		t.Fatalf("Provider() error = %v", err)
	}
	if host.LastRefreshError != "" {
		t.Fatalf("last refresh error = %q, want an answered listing recorded as a success", host.LastRefreshError)
	}
}

// modelByIDInto finds one model in a page of stored models.
func modelByIDInto(models []appcatalog.Model, id string) (appcatalog.Model, bool) {
	for _, model := range models {
		if model.ModelID == id {
			return model, true
		}
	}
	return appcatalog.Model{}, false
}

// TestRefreshReadsTheDialectTheTemplateDeclares covers a row stored before the
// template changed: the row still says it publishes no list, and the template
// is what says which listing dialect its provider actually speaks. Reading the
// stored row instead is what left a ChatGPT connection with no models.
func TestRefreshReadsTheDialectTheTemplateDeclares(t *testing.T) {
	h := newHarness(t)
	roster := listingServer(t, http.StatusOK, `{"models":[
		{"slug":"gpt-5.5","display_name":"GPT-5.5","context_window":272000,"supported_in_api":true,"visibility":"list"},
		{"slug":"gpt-reserve","display_name":"GPT-Reserve","supported_in_api":true,"visibility":"hide"}
	]}`)
	manager := pageService(t, h, catalogServer(t, catalogFixture()).URL)
	seedRow(t, h, sqlite.ProviderRow{
		ID: "openai-codex", TemplateID: "openai-codex", Origin: string(catalog.OriginSignIn),
		Label: "ChatGPT (Codex sign-in)", Auth: string(catalog.AuthOAuth),
		APIFormat: string(catalog.FormatOpenAIResp), BaseURL: roster.URL,
		// The stored row is what a connection added before the template
		// declared a dialect still carries.
		ModelsSource: "modelsdev", ModelsFormat: string(catalog.ModelsNone),
		ModelsDevProviderID: "openai", Enabled: true, Rank: 100,
	})

	if _, err := manager.RefreshProviderModels(context.Background(), "openai-codex"); err != nil {
		t.Fatalf("RefreshProviderModels() error = %v", err)
	}

	models, _, err := manager.Models(context.Background(), appcatalog.ModelQuery{Provider: "openai-codex"})
	if err != nil {
		t.Fatalf("Models() error = %v", err)
	}
	byID := make(map[string]appcatalog.Model, len(models))
	for _, model := range models {
		byID[model.ModelID] = model
	}
	listed, found := byID["gpt-5.5"]
	if !found || listed.Source != appcatalog.SourceListing {
		t.Fatalf("models = %+v, want the dialect the template declares to have listed gpt-5.5", models)
	}
	if _, found := byID["gpt-reserve"]; found {
		t.Fatal("the vendor's hidden row reached the roster, want it left out")
	}
	// A model no source publishes any more stays for the routes that name it,
	// and is marked unavailable rather than deleted.
	stale, found := byID["model-1"]
	if !found || stale.Available == nil || *stale.Available {
		t.Fatalf("model-1 = %+v, want a model nothing publishes marked unavailable", stale)
	}
}

// TestAddAccountRefreshesTheRoster asserts a stored credential brings the
// models the account may use with it, which is what makes an added account
// routable without an operator asking for a refresh.
func TestAddAccountRefreshesTheRoster(t *testing.T) {
	h := newHarness(t)
	listing := listingServer(t, http.StatusOK, `{"data":[{"id":"model-1","name":"From The Provider"}]}`)
	manager := pageService(t, h, catalogServer(t, catalogFixture()).URL)
	seedRow(t, h, sqlite.ProviderRow{
		ID: "groq", TemplateID: "groq", Origin: string(catalog.OriginTemplate),
		Label: "Groq", Auth: string(catalog.AuthAPIKey),
		APIFormat: string(catalog.FormatOpenAIChat), BaseURL: listing.URL + "/v1",
		ModelsFormat: string(catalog.ModelsOpenAI), ModelsDevProviderID: "groq",
		Enabled: true, Rank: 100,
	})

	if _, err := manager.accounts.AddAccount(context.Background(), appaccount.NewAccount{
		ProviderID: "groq", Label: "work", SecretValue: "sk-1",
	}); err != nil {
		t.Fatalf("AddAccount() error = %v", err)
	}
	h.waitForRefresh("groq")

	models, _, err := manager.Models(context.Background(), appcatalog.ModelQuery{Provider: "groq"})
	if err != nil {
		t.Fatalf("Models() error = %v", err)
	}
	found := false
	for _, model := range models {
		if model.ModelID == "model-1" && model.Source == appcatalog.SourceListing {
			found = true
		}
	}
	if !found {
		t.Fatalf("models = %+v, want the listed model the new credential reached", models)
	}
}

func TestDeleteProviderIsRefusedBeforeItTouchesAccounts(t *testing.T) {
	h := newHarness(t)
	manager := pageService(t, h, catalogServer(t, catalogFixture()).URL)
	account := h.addAccount("openai", "work", "sk-1")
	if err := manager.routes.Save(context.Background(), "fast", approuting.Write{
		Label: "Fast", Strategy: "priority", Enabled: true, Listed: true,
		Members: []approuting.MemberWrite{{ProviderID: "openai", ModelID: "model-1", Weight: 1, Enabled: true}},
	}); err != nil {
		t.Fatalf("SaveGroup() error = %v", err)
	}

	err := manager.DeleteProvider(context.Background(), "openai")
	if !errors.Is(err, appcatalog.ErrCatalogConflict) {
		t.Fatalf("DeleteProvider() error = %v, want a conflict", err)
	}
	if !strings.Contains(err.Error(), "Fast") {
		t.Fatalf("DeleteProvider() error = %q, want it to name the group", err)
	}
	row, found := h.credentialRow(account.ID)
	if !found {
		t.Fatal("the refused delete removed the account")
	}
	if _, found := h.storedSecret(row.SecretRef); !found {
		t.Fatal("the refused delete removed the secret")
	}
	if _, err := manager.Provider(context.Background(), "openai"); err != nil {
		t.Fatalf("the refused delete removed the provider: %v", err)
	}
}

func TestATargetedProbeAddsOnlyACredential(t *testing.T) {
	h := newHarness(t)
	listing := listingServer(t, http.StatusOK, `{"data":[{"id":"model-1"}]}`)
	manager := pageService(t, h, catalogServer(t, catalogFixture()).URL)
	seedRow(t, h, sqlite.ProviderRow{
		ID: "groq", TemplateID: "groq", Origin: string(catalog.OriginTemplate),
		Label: "Groq", Auth: string(catalog.AuthAPIKey),
		APIFormat: string(catalog.FormatOpenAIChat), BaseURL: listing.URL + "/v1",
		ModelsFormat: string(catalog.ModelsOpenAI), ModelsDevProviderID: "groq",
		Enabled: true, Rank: 100,
	})
	before, err := manager.Provider(context.Background(), "groq")
	if err != nil {
		t.Fatalf("Provider() error = %v", err)
	}

	result, err := manager.ProbeProvider(context.Background(), probeJSON(t, `{"provider_id":"groq","credential":{"kind":"api_key","secret":"sk-2"}}`))
	if err != nil {
		t.Fatalf("ProbeProvider() error = %v", err)
	}
	if result.TargetProviderID != "groq" {
		t.Fatalf("target = %q, want the provider the key belongs to", result.TargetProviderID)
	}
	if len(result.Models) != 0 {
		t.Fatalf("a targeted probe carries %d models, want none", len(result.Models))
	}

	if _, err := manager.CommitProbe(context.Background(), result.ProbeID, appcatalog.CommitProbeRequest{AccountLabel: "second"}); err != nil {
		t.Fatalf("CommitProbe() error = %v", err)
	}
	after, err := manager.Provider(context.Background(), "groq")
	if err != nil {
		t.Fatalf("Provider() error = %v", err)
	}
	if after.Counts.Accounts != before.Counts.Accounts+1 {
		t.Fatalf("accounts = %d, want one more than %d", after.Counts.Accounts, before.Counts.Accounts)
	}
	if after.Counts.Models != before.Counts.Models {
		t.Fatalf("models = %d, want the provider to keep the %d it had", after.Counts.Models, before.Counts.Models)
	}
	if after.BaseURL != before.BaseURL || after.Label != before.Label {
		t.Fatal("committing a key changed the provider's own connection")
	}
}

func TestReloadRewritesRetiredConnectionLabels(t *testing.T) {
	h := newHarness(t)
	manager := pageService(t, h, catalogServer(t, catalogFixture()).URL)
	seedRow(t, h, sqlite.ProviderRow{
		ID: "openai-codex", TemplateID: "openai-codex", Origin: string(catalog.OriginSignIn),
		Label: "ChatGPT (Codex sign-in)", Auth: string(catalog.AuthOAuth),
		APIFormat:    string(catalog.FormatOpenAIResp),
		ModelsSource: "listing", ModelsFormat: string(catalog.ModelsCodex),
		ModelsDevProviderID: "openai", Enabled: true, Rank: 100,
	})
	seedRow(t, h, sqlite.ProviderRow{
		ID: "anthropic", TemplateID: "anthropic", Origin: string(catalog.OriginTemplate),
		Label: "Anthropic", Auth: string(catalog.AuthAPIKey),
		APIFormat:    string(catalog.FormatOpenAIChat),
		ModelsSource: "listing", ModelsFormat: string(catalog.ModelsOpenAI),
		ModelsDevProviderID: "anthropic", Enabled: true, Rank: 100,
	})
	seedRow(t, h, sqlite.ProviderRow{
		ID: "claude-work", TemplateID: "claude", Origin: string(catalog.OriginSignIn),
		Label: "Work Claude", Auth: string(catalog.AuthOAuth),
		APIFormat:    string(catalog.FormatAnthropic),
		ModelsSource: "listing", ModelsFormat: string(catalog.ModelsAnthropic),
		Enabled: true, Rank: 100,
	})

	if err := manager.Reload(context.Background()); err != nil {
		t.Fatalf("Reload() error = %v", err)
	}

	codex, err := manager.Provider(context.Background(), "openai-codex")
	if err != nil {
		t.Fatalf("Provider(openai-codex) error = %v", err)
	}
	if codex.Label != "ChatGPT" {
		t.Errorf("openai-codex label = %q, want ChatGPT", codex.Label)
	}
	twin, err := manager.Provider(context.Background(), "anthropic")
	if err != nil {
		t.Fatalf("Provider(anthropic) error = %v", err)
	}
	if twin.Label != "Claude API" {
		t.Errorf("anthropic label = %q, want Claude API", twin.Label)
	}
	custom, err := manager.Provider(context.Background(), "claude-work")
	if err != nil {
		t.Fatalf("Provider(claude-work) error = %v", err)
	}
	if custom.Label != "Work Claude" {
		t.Errorf("claude-work label = %q, want Work Claude", custom.Label)
	}
}
