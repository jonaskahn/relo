package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/adapters/discovery"
	"github.com/jonaskahn/relo/internal/adapters/modelsdev"
	appcatalog "github.com/jonaskahn/relo/internal/application/catalog"
	"github.com/jonaskahn/relo/internal/config"
	"github.com/jonaskahn/relo/tests/testkit"
)

// listingServer answers a provider model list with one status and body.
func listingServer(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(server.Close)
	return server
}

// catalogServer serves the models.dev document a probe reads.
func catalogServer(t *testing.T, providers map[string]map[string]any) *httptest.Server {
	t.Helper()
	body, err := json.Marshal(providers)
	if err != nil {
		t.Fatalf("marshal the models.dev fixture: %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(server.Close)
	return server
}

// probeService is a harness whose models.dev catalog and provider listings are
// both served from this process, so a probe needs no network.
func probeService(t *testing.T, h *harness, catalogURL string) *session {
	t.Helper()
	client := &http.Client{Timeout: 5 * time.Second}
	return h.buildServiceWith(h.catalog, h.pools, testkit.SecretStore(h.secrets),
		modelsdev.NewDirectory(filepath.Join(h.home, "cache"), catalogURL, client),
		discovery.New(client))
}

// wireProbeRequest mirrors the probe body the handler decodes, so the tests
// read the same wire the API serves rather than the application type.
type wireProbeRequest struct {
	TemplateID   string            `json:"template_id,omitempty"`
	ProviderID   string            `json:"provider_id,omitempty"`
	CustomID     string            `json:"custom_id,omitempty"`
	Label        string            `json:"label,omitempty"`
	APIFormat    string            `json:"api_format,omitempty"`
	BaseURL      string            `json:"base_url,omitempty"`
	KeyHeader    string            `json:"key_header,omitempty"`
	ModelsFormat string            `json:"models_format,omitempty"`
	Variables    map[string]string `json:"variables,omitempty"`
	Headers      map[string]string `json:"headers,omitempty"`
	Credential   struct {
		Kind     string `json:"kind"`
		Secret   string `json:"secret"`
		Priority int    `json:"priority"`
	} `json:"credential"`
	ManualModels []struct {
		ModelID  string `json:"model_id"`
		PricedAs string `json:"priced_as,omitempty"`
	} `json:"manual_models,omitempty"`
}

// probeJSON decodes one probe request, which keeps the tests off the anonymous
// credential shape the handler owns.
func probeJSON(t *testing.T, body string) appcatalog.ProbeRequest {
	t.Helper()
	var raw wireProbeRequest
	if err := json.Unmarshal([]byte(body), &raw); err != nil {
		t.Fatalf("decode the probe request: %v", err)
	}
	req := appcatalog.ProbeRequest{
		TemplateID: raw.TemplateID, ProviderID: raw.ProviderID,
		CustomID: raw.CustomID, Label: raw.Label,
		APIFormat: raw.APIFormat, BaseURL: raw.BaseURL,
		KeyHeader: raw.KeyHeader, ModelsFormat: raw.ModelsFormat,
		Variables: raw.Variables, Headers: raw.Headers,
	}
	req.Credential.Kind = raw.Credential.Kind
	req.Credential.Secret = raw.Credential.Secret
	req.Credential.Priority = raw.Credential.Priority
	for _, manual := range raw.ManualModels {
		req.ManualModels = append(req.ManualModels, appcatalog.ManualModel{
			ModelID: manual.ModelID, PricedAs: manual.PricedAs,
		})
	}
	return req
}

// sourcesOf names each probed model with the layer its id came from.
func sourcesOf(result appcatalog.ProbeResult) []string {
	out := make([]string, 0, len(result.Models))
	for _, model := range result.Models {
		out = append(out, model.ID+"="+model.Source)
	}
	sort.Strings(out)
	return out
}

// modelByID finds one probed model.
func modelByID(result appcatalog.ProbeResult, id string) (appcatalog.ProbeModel, bool) {
	for _, model := range result.Models {
		if model.ID == id {
			return model, true
		}
	}
	return appcatalog.ProbeModel{}, false
}

// checkStatus reads one checklist line.
func checkStatus(result appcatalog.ProbeResult, name string) string {
	for _, check := range result.Checks {
		if check.Name == name {
			return check.Status
		}
	}
	return ""
}

// TestProbeProviderKeepsTheProvidersOwnRoster is the rule a probe exists for:
// the ids the provider publishes are the models the connection serves. A
// catalog describes models, and describing one the connection cannot reach is
// no reason to offer it.
func TestProbeProviderKeepsTheProvidersOwnRoster(t *testing.T) {
	h := newHarness(t)
	listing := listingServer(t, http.StatusOK,
		`{"data":[{"id":"live-1","name":"Live One"},{"id":"retired-1","name":"Retired One"}]}`)
	catalog := catalogServer(t, map[string]map[string]any{
		"groq": {
			"id": "groq", "name": "Groq", "npm": "@ai-sdk/groq", "api": listing.URL + "/v1",
			"models": map[string]any{
				"live-1": map[string]any{
					"id": "live-1", "name": "Catalog Name",
					"status": "active", "cost": map[string]any{"input": 1.0, "output": 2.0},
				},
				"catalog-1": map[string]any{
					"id": "catalog-1", "name": "Catalog One",
					"status": "active", "cost": map[string]any{"input": 3.0, "output": 4.0},
				},
				"retired-1": map[string]any{"id": "retired-1", "name": "Retired", "status": "deprecated"},
				"beta-1":    map[string]any{"id": "beta-1", "name": "Beta", "status": "beta"},
			},
		},
	})
	manager := probeService(t, h, catalog.URL)

	req := probeJSON(t, `{"template_id":"groq","credential":{"kind":"api_key","secret":"sk-test"}}`)
	result, err := manager.ProbeProvider(context.Background(), req)
	if err != nil {
		t.Fatalf("ProbeProvider() error = %v", err)
	}

	want := []string{"live-1=listing", "retired-1=listing"}
	got := sourcesOf(result)
	if len(got) != len(want) {
		t.Fatalf("roster = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("roster = %v, want %v", got, want)
		}
	}

	if result.Counts.Listed != 2 || result.Counts.FromListing != 2 || result.Counts.FromManual != 0 {
		t.Fatalf("counts = %+v, want the two ids the provider listed", result.Counts)
	}

	// A catalog model the provider does not list is not a model of this
	// connection, however well the catalog describes it.
	for _, id := range []string{"catalog-1", "beta-1"} {
		if _, found := modelByID(result, id); found {
			t.Fatalf("roster carries %s, want the provider's own list to decide", id)
		}
	}

	// The listed id keeps the provider name and still gets the catalog rate.
	live, found := modelByID(result, "live-1")
	if !found || live.Name != "Live One" {
		t.Fatalf("live row = %+v, want the provider name", live)
	}
	if live.Prices.ModelsDev.Input == nil || *live.Prices.ModelsDev.Input != 1_000_000 {
		t.Fatalf("live catalog price = %v, want the catalog rate", live.Prices.ModelsDev.Input)
	}

	// A catalog entry that calls a listed model deprecated describes it; the
	// provider listing it is what keeps the connection serving it.
	if retired, found := modelByID(result, "retired-1"); !found || !retired.Enabled {
		t.Fatalf("retired row = %+v, want the provider's own list to keep it on", retired)
	}
	if checkStatus(result, "credential") != "pass" {
		t.Fatalf("credential check = %q, want a verified key", checkStatus(result, "credential"))
	}
}

func TestProbeProviderRefusesARejectedKeyWithoutCatalogIds(t *testing.T) {
	h := newHarness(t)
	listing := listingServer(t, http.StatusUnauthorized, `{"error":"bad key"}`)
	catalog := catalogServer(t, map[string]map[string]any{
		"groq": {
			"id": "groq", "name": "Groq", "npm": "@ai-sdk/groq", "api": listing.URL + "/v1",
			"models": map[string]any{"catalog-1": map[string]any{"id": "catalog-1", "name": "Catalog One"}},
		},
	})
	manager := probeService(t, h, catalog.URL)

	req := probeJSON(t, `{"template_id":"groq","credential":{"kind":"api_key","secret":"sk-refused"}}`)
	result, err := manager.ProbeProvider(context.Background(), req)
	if !errors.Is(err, appcatalog.ErrCredentialRejected) {
		t.Fatalf("ProbeProvider() error = %v, want a refused credential", err)
	}
	if len(result.Models) != 0 {
		t.Fatalf("roster = %v, want the catalog to leave a refused key alone", sourcesOf(result))
	}
	if checkStatus(result, "credential") != "fail" {
		t.Fatalf("credential check = %q, want a failure", checkStatus(result, "credential"))
	}
	if _, err := manager.Provider(context.Background(), "groq"); err == nil {
		t.Fatal("a refused probe stored a provider, want nothing saved")
	}
}

// codexRoster is the roster the ChatGPT Codex endpoint publishes for one
// account: two offers it may pick, one the vendor keeps out of every picker,
// and one the API cannot drive.
const codexRoster = `{"models":[
	{"slug":"gpt-5.5","display_name":"GPT-5.5","context_window":272000,"supported_in_api":true,"visibility":"list"},
	{"slug":"gpt-6-luna","display_name":"GPT-6-Luna","context_window":372000,"supported_in_api":true,"visibility":"list"},
	{"slug":"gpt-reserve","display_name":"GPT-Reserve","supported_in_api":true,"visibility":"hide"},
	{"slug":"chat-only","display_name":"Chat Only","supported_in_api":false,"visibility":"list"}
]}`

// TestProbeProviderReadsTheCodexRoster asserts the ChatGPT sign-in fills its
// roster from the vendor's own endpoint, which describes what that account may
// use, and that models.dev still prices what the vendor published.
func TestProbeProviderReadsTheCodexRoster(t *testing.T) {
	h := newHarness(t)
	roster := listingServer(t, http.StatusOK, codexRoster)
	catalog := catalogServer(t, map[string]map[string]any{
		"openai": {
			"id": "openai", "name": "OpenAI", "npm": "@ai-sdk/openai",
			"models": map[string]any{
				"gpt-5.5": map[string]any{
					"id": "gpt-5.5", "name": "GPT-5.5", "status": "active",
					"cost": map[string]any{"input": 1.25, "output": 10.0},
				},
				"gpt-6-luna": map[string]any{
					"id": "gpt-6-luna", "name": "GPT-6-Luna", "status": "active",
					"cost": map[string]any{"input": 2.5, "output": 20.0},
				},
			},
		},
	})
	manager := probeService(t, h, catalog.URL)

	result, err := manager.ProbeProvider(context.Background(), probeJSON(t,
		`{"template_id":"openai-codex","base_url":"`+roster.URL+`"}`))
	if err != nil {
		t.Fatalf("ProbeProvider() error = %v", err)
	}

	got := sourcesOf(result)
	want := []string{"gpt-5.5=listing", "gpt-6-luna=listing"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("roster = %v, want the account's own offers alone: %v", got, want)
	}
	// The vendor's hidden and api-unsupported rows are not offers this account
	// may pick, so neither reaches the roster.
	for _, id := range []string{"gpt-reserve", "chat-only"} {
		if _, found := modelByID(result, id); found {
			t.Fatalf("roster carries %s, want the vendor's hidden rows left out", id)
		}
	}
	// The vendor publishes the roster and the context window, and models.dev
	// still supplies the rate the review step shows.
	live, found := modelByID(result, "gpt-5.5")
	if !found || live.ContextWindow == nil || *live.ContextWindow != 272000 {
		t.Fatalf("gpt-5.5 = %+v, want the vendor's context window", live)
	}
	if live.Prices.ModelsDev.Input == nil || *live.Prices.ModelsDev.Input != 1_250_000 {
		t.Fatalf("gpt-5.5 catalog rate = %v, want models.dev to price it", live.Prices.ModelsDev.Input)
	}
	// The checklist reports the listing that actually ran, which is what a
	// sign-in connection has instead of a key to test.
	if checkStatus(result, "listing") != "pass" {
		t.Fatalf("listing check = %q, want the vendor roster read", checkStatus(result, "listing"))
	}
}

func TestProbeProviderKeepsAzureDeploymentsAlone(t *testing.T) {
	h := newHarness(t)
	catalog := catalogServer(t, map[string]map[string]any{
		"azure": {
			"id": "azure", "name": "Azure", "npm": "@ai-sdk/azure",
			"models": map[string]any{"gpt-4o": map[string]any{"id": "gpt-4o", "name": "GPT-4o"}},
		},
	})
	manager := probeService(t, h, catalog.URL)

	body := `{"template_id":"azure-openai","credential":{"kind":"api_key","secret":"azure-key"},"manual_models":[{"model_id":"my-deployment"}]}`
	result, err := manager.ProbeProvider(context.Background(), probeJSON(t, body))
	if err != nil {
		t.Fatalf("ProbeProvider() error = %v", err)
	}

	got := sourcesOf(result)
	if len(got) != 1 || got[0] != "my-deployment=manual" {
		t.Fatalf("roster = %v, want the typed deployment alone", got)
	}
}

func TestProbeProviderLeavesALocalServerOffTheCatalog(t *testing.T) {
	h := newHarness(t)
	listing := listingServer(t, http.StatusOK, `{"data":[{"id":"llama3"}]}`)
	catalog := catalogServer(t, map[string]map[string]any{
		"ollama": {
			"id": "ollama", "name": "Ollama", "api": listing.URL + "/v1",
			"models": map[string]any{"catalog-only": map[string]any{"id": "catalog-only", "name": "Catalog Only"}},
		},
	})
	manager := probeService(t, h, catalog.URL)

	body := `{"template_id":"ollama","base_url":"` + listing.URL + `/v1"}`
	result, err := manager.ProbeProvider(context.Background(), probeJSON(t, body))
	if err != nil {
		t.Fatalf("ProbeProvider() error = %v", err)
	}

	got := sourcesOf(result)
	if len(got) != 1 || got[0] != "llama3=listing" {
		t.Fatalf("roster = %v, want the machine's own list", got)
	}
}

// TestProbeProviderKeepsASignInToWhatItLists covers a sign-in connection: the
// account's own list is the roster, and a catalog id the account does not
// reach stays out of it.
func TestProbeProviderKeepsASignInToWhatItLists(t *testing.T) {
	h := newHarness(t)
	listing := listingServer(t, http.StatusOK, `{"data":[{"id":"kimi-k2.7-code","name":"Kimi K2.7 Code"}]}`)
	catalog := catalogServer(t, map[string]map[string]any{
		"kimi-code-plan-cn": {
			"id": "kimi-code-plan-cn", "name": "Kimi", "api": listing.URL + "/v1",
			"models": map[string]any{"k3": map[string]any{"id": "k3", "name": "Kimi K3"}},
		},
	})
	manager := probeService(t, h, catalog.URL)

	body := `{"template_id":"kimi","base_url":"` + listing.URL + `/v1","credential":{"kind":"oauth","secret":"session"}}`
	result, err := manager.ProbeProvider(context.Background(), probeJSON(t, body))
	if err != nil {
		t.Fatalf("ProbeProvider() error = %v", err)
	}

	if result.Counts.Listed != 1 || result.Counts.FromListing != 1 || result.Counts.FromManual != 0 {
		t.Fatalf("counts = %+v, want the one id the account's own list named", result.Counts)
	}
	got := sourcesOf(result)
	if len(got) != 1 || got[0] != "kimi-k2.7-code=listing" {
		t.Fatalf("roster = %v, want the account's own list alone", got)
	}
	for _, id := range []string{"k3", "kimi-k2.6"} {
		if _, found := modelByID(result, id); found {
			t.Fatalf("roster carries %s, want nothing the account did not list", id)
		}
	}
}

func TestProbeProviderTakesTheChosenFormatHeader(t *testing.T) {
	h := newHarness(t)
	listing := listingServer(t, http.StatusOK, `{"data":[{"id":"claude-fable-5-1","name":"Claude Fable 5.1"}]}`)
	manager := probeService(t, h, catalogServer(t, map[string]map[string]any{}).URL)

	result, err := manager.ProbeProvider(context.Background(), probeJSON(t,
		`{"template_id":"teamorouter","api_format":"anthropic","base_url":"`+listing.URL+
			`/v1","credential":{"kind":"api_key","secret":"sk-teamo-test"}}`))
	if err != nil {
		t.Fatalf("ProbeProvider() error = %v", err)
	}
	host, err := manager.CommitProbe(context.Background(), result.ProbeID, appcatalog.CommitProbeRequest{
		ProviderID: "teamorouter", Label: "TeamoRouter",
	})
	if err != nil {
		t.Fatalf("CommitProbe() error = %v", err)
	}
	if host.APIFormat != "anthropic" {
		t.Errorf("api format = %q, want anthropic", host.APIFormat)
	}
	if host.KeyHeader != "x-api-key" {
		t.Errorf("key header = %q, want x-api-key", host.KeyHeader)
	}
	if host.ModelsFormat != "openai" {
		t.Errorf("models format = %q, want openai", host.ModelsFormat)
	}
	if host.UseProxy {
		t.Fatal("use proxy is on for a connection that defaults off")
	}
}

func TestProbeOpenCodeFreeKeepsFreeModels(t *testing.T) {
	h := newHarness(t)
	var authorization, userAgent string
	var proxied bool
	listing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization = r.Header.Get("Authorization")
		userAgent = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[{"id":"exo-free"},{"id":"big-pickle"},{"id":"gpt-5.5"},{"id":"jev-1.13-free"}]}`)
	}))
	t.Cleanup(listing.Close)
	catalog := catalogServer(t, map[string]map[string]any{
		"opencode": {
			"id": "opencode", "name": "OpenCode",
			"models": map[string]any{
				"gpt-5.5": map[string]any{"id": "gpt-5.5", "name": "GPT 5.5", "cost": map[string]any{"input": 1, "output": 2}},
			},
		},
	})
	h.proxyURL = func() string { return "http://127.0.0.1:9" }
	client := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{
		Proxy: func(r *http.Request) (*url.URL, error) {
			if config.OutboundProxyFrom(r.Context()) != "" {
				proxied = true
			}
			return nil, nil
		},
	}}
	manager := h.buildServiceWith(h.catalog, h.pools, testkit.SecretStore(h.secrets),
		modelsdev.NewDirectory(filepath.Join(h.home, "cache"), catalog.URL, client),
		discovery.New(client))

	result, err := manager.ProbeProvider(context.Background(), probeJSON(t,
		`{"template_id":"opencode-free","base_url":"`+listing.URL+`/v1"}`))
	if err != nil {
		t.Fatalf("ProbeProvider() error = %v", err)
	}
	got := sourcesOf(result)
	if len(got) != 2 || got[0] != "big-pickle=listing" || got[1] != "exo-free=listing" {
		t.Fatalf("roster = %v, want the two free models the listing published", got)
	}
	if authorization != "Bearer public" || userAgent != "opencode/1.18.33" {
		t.Fatalf("authorization = %q, user agent = %q", authorization, userAgent)
	}
	if !proxied {
		t.Fatal("the free-tier listing did not ask for the proxy")
	}
	host, err := manager.CommitProbe(context.Background(), result.ProbeID, appcatalog.CommitProbeRequest{Label: "Opencode Free"})
	if err != nil {
		t.Fatalf("CommitProbe() error = %v", err)
	}
	if !host.UseProxy {
		t.Fatal("Opencode Free was saved with the proxy off")
	}
}
