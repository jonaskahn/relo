package discovery

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/antigravity"
	"github.com/jonaskahn/relo/internal/adapters/codex"
	"github.com/jonaskahn/relo/internal/adapters/upstream"
	"github.com/jonaskahn/relo/internal/catalog"
)

// codexRoster is the roster the Codex endpoint publishes: two rows an account
// may pick, one the vendor keeps out of every picker, and one the API cannot
// drive at all.
const codexRoster = `{
  "models": [
    {"slug": "gpt-5.5", "display_name": "GPT-5.5", "context_window": 272000, "supported_in_api": true, "visibility": "list"},
    {"slug": "gpt-6-luna", "display_name": "GPT-6-Luna", "context_window": 372000, "supported_in_api": true, "visibility": "list"},
    {"slug": "gpt-reserve", "display_name": "GPT-Reserve", "context_window": 272000, "supported_in_api": true, "visibility": "hide"},
    {"slug": "chat-only", "display_name": "Chat Only", "supported_in_api": false, "visibility": "list"}
  ]
}`

// TestListCodexReadsTheAccountsOwnRoster asserts the request the Codex
// dialect builds and the entitlement it keeps, because the vendor filters the
// roster by the client version and marks the rows an account may not pick.
func TestListCodexReadsTheAccountsOwnRoster(t *testing.T) {
	var gotPath, gotVersion, gotAuth, gotAccount, gotOriginator, gotHeaderVersion, gotBeta, gotAccept string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotVersion = r.URL.Query().Get("client_version")
		gotAuth = r.Header.Get("Authorization")
		gotAccount = r.Header.Get("ChatGPT-Account-Id")
		gotOriginator = r.Header.Get("originator")
		gotHeaderVersion = r.Header.Get("version")
		gotBeta = r.Header.Get("OpenAI-Beta")
		gotAccept = r.Header.Get("Accept")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(codexRoster))
	}))
	t.Cleanup(server.Close)

	listed, err := New(server.Client()).List(context.Background(), Target{
		Format:  catalog.ModelsCodex,
		BaseURL: server.URL,
		Auth: upstream.Authorization{
			Token: "tok", KeyHeader: catalog.KeyHeaderBearer,
			Headers: map[string]string{"chatgpt-account-id": "acct-1"},
		},
		Headers: map[string]string{"originator": "old-client"},
	})
	if err != nil {
		t.Fatalf("List(codex) error = %v", err)
	}

	if gotPath != "/models" {
		t.Errorf("path = %q, want /models", gotPath)
	}
	if gotVersion != codex.Version {
		t.Errorf("client_version = %q, want %q", gotVersion, codex.Version)
	}
	if gotAuth != "Bearer tok" {
		t.Errorf("Authorization = %q, want the bearer token", gotAuth)
	}
	if gotAccount != "acct-1" {
		t.Errorf("ChatGPT-Account-Id = %q, want the account the credential names", gotAccount)
	}
	if gotOriginator != codex.Originator || gotHeaderVersion != codex.Version ||
		gotBeta != codex.OpenAIBeta || gotAccept != "application/json" {
		t.Errorf("originator = %q, version = %q, beta = %q, accept = %q",
			gotOriginator, gotHeaderVersion, gotBeta, gotAccept)
	}

	ids := make([]string, 0, len(listed))
	for _, entry := range listed {
		ids = append(ids, entry.ID)
	}
	if len(ids) != 2 || ids[0] != "gpt-5.5" || ids[1] != "gpt-6-luna" {
		t.Fatalf("ids = %v, want only the listed, api-supported rows", ids)
	}
	if listed[0].Name != "GPT-5.5" {
		t.Errorf("name = %q, want the display name", listed[0].Name)
	}
	if listed[0].ContextWindow == nil || *listed[0].ContextWindow != 272000 {
		t.Errorf("context window = %v, want 272000", listed[0].ContextWindow)
	}
	if listed[1].ContextWindow == nil || *listed[1].ContextWindow != 372000 {
		t.Errorf("second context window = %v, want 372000", listed[1].ContextWindow)
	}
}

// TestListCodexRefusesARejectedAccount asserts a refused credential is
// reported the way a listing failure every caller already reads, rather than
// as an empty roster an operator would read as "this account serves nothing".
func TestListCodexRefusesARejectedAccount(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"no"}`))
	}))
	t.Cleanup(server.Close)

	_, err := New(server.Client()).List(context.Background(), Target{
		Format: catalog.ModelsCodex, BaseURL: server.URL,
		Auth: upstream.Authorization{Token: "tok", KeyHeader: catalog.KeyHeaderBearer},
	})
	if err == nil {
		t.Fatal("List(codex) error = nil, want a refused listing")
	}
}

// TestListFailureStatusIsInspectable covers a failed listing with errors.Is,
// so a caller can tell a vendor failure from a rejected key.
func TestListFailureStatusIsInspectable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	t.Cleanup(server.Close)

	_, err := New(server.Client()).List(context.Background(), Target{
		Format: catalog.ModelsOpenAI, BaseURL: server.URL,
		Auth: upstream.Authorization{Token: "tok", KeyHeader: catalog.KeyHeaderBearer},
	})
	if !errors.Is(err, ErrListStatus) {
		t.Fatalf("List(openai) error = %v, want ErrListStatus", err)
	}
}

// TestListCodexSkipsARowItCannotName asserts a roster row without a slug is
// dropped rather than stored as a model with no identifier.
func TestListCodexSkipsARowItCannotName(t *testing.T) {
	payload, err := json.Marshal(map[string]any{"models": []map[string]any{
		{"display_name": "No Slug", "supported_in_api": true, "visibility": "list"},
		{"slug": "   ", "supported_in_api": true, "visibility": "list"},
		{"slug": "gpt-5.5", "supported_in_api": true, "visibility": "list"},
	}})
	if err != nil {
		t.Fatalf("marshal the fixture: %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(payload)
	}))
	t.Cleanup(server.Close)

	listed, err := New(server.Client()).List(context.Background(), Target{
		Format: catalog.ModelsCodex, BaseURL: server.URL + "/",
		Auth: upstream.Authorization{},
	})
	if err != nil {
		t.Fatalf("List(codex) error = %v", err)
	}
	if len(listed) != 1 || listed[0].ID != "gpt-5.5" {
		t.Fatalf("listed = %+v, want only the named row", listed)
	}
}

// TestListRejectsAnUnknownDialect asserts the registry refuses a format no
// dialect implements, so a provider is never silently read as empty.
func TestListRejectsAnUnknownDialect(t *testing.T) {
	_, err := New(nil).List(context.Background(), Target{
		Format: catalog.ModelsFormat("none"), BaseURL: "https://example.test",
	})
	if err == nil {
		t.Fatal("List(none) error = nil, want an unsupported dialect")
	}
}

// TestListAntigravityCarriesTheIdeFingerprint asserts the listing request the
// Cloud Code Assist dialect builds: the fingerprint the backend gates an
// account's models on, and the billing project the endpoint reads.
func TestListAntigravityCarriesTheIdeFingerprint(t *testing.T) {
	var gotPath, gotUserAgent, gotAuth, gotBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotUserAgent = r.Header.Get("User-Agent")
		gotAuth = r.Header.Get("Authorization")
		raw, _ := io.ReadAll(r.Body)
		gotBody = string(raw)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"models":{"gemini-3.8-flash":{"displayName":"Gemini 3.8 Flash"}}}`))
	}))
	t.Cleanup(server.Close)

	listed, err := New(server.Client()).List(context.Background(), Target{
		Format:  catalog.ModelsAntigravity,
		BaseURL: server.URL,
		Auth: upstream.Authorization{
			Token: "tok", KeyHeader: catalog.KeyHeaderBearer, Project: "project-1",
		},
		Headers: map[string]string{"User-Agent": antigravity.UserAgent()},
	})
	if err != nil {
		t.Fatalf("List(antigravity) error = %v", err)
	}
	if gotPath != "/v1internal:fetchAvailableModels" {
		t.Errorf("path = %q, want the Cloud Code Assist listing", gotPath)
	}
	if gotUserAgent != antigravity.UserAgent() {
		t.Errorf("user agent = %q, want the Antigravity IDE fingerprint", gotUserAgent)
	}
	if gotAuth != "Bearer tok" {
		t.Errorf("authorization = %q, want the credential", gotAuth)
	}
	if gotBody != `{"project":"project-1"}` {
		t.Errorf("body = %q, want the billing project", gotBody)
	}
	if len(listed) != 1 || listed[0].ID != "gemini-3.8-flash" || listed[0].Name != "Gemini 3.8 Flash" {
		t.Fatalf("listed = %+v, want the published model", listed)
	}
}

// TestListAntigravityWithoutAProject asserts a credential that names no
// project still posts the empty object the endpoint reads, rather than the
// project lookup's metadata envelope.
func TestListAntigravityWithoutAProject(t *testing.T) {
	var gotBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		gotBody = string(raw)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"models":{}}`))
	}))
	t.Cleanup(server.Close)

	if _, err := New(server.Client()).List(context.Background(), Target{
		Format: catalog.ModelsAntigravity, BaseURL: server.URL,
		Auth: upstream.Authorization{Token: "tok", KeyHeader: catalog.KeyHeaderBearer},
	}); err != nil {
		t.Fatalf("List(antigravity) error = %v", err)
	}
	if gotBody != "{}" {
		t.Fatalf("body = %q, want an empty object without a project", gotBody)
	}
}

// TestListAntigravityNamesARejectedListing asserts a refusal says what the
// endpoint answered while keeping the sentinel the console and the catalog
// already read.
func TestListAntigravityNamesARejectedListing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":{"status":"PERMISSION_DENIED","message":"Caller does not have permission"}}`))
	}))
	t.Cleanup(server.Close)

	_, err := New(server.Client()).List(context.Background(), Target{
		Format: catalog.ModelsAntigravity, BaseURL: server.URL,
		Auth: upstream.Authorization{Token: "tok", KeyHeader: catalog.KeyHeaderBearer},
	})
	if !errors.Is(err, ErrListRejected) {
		t.Fatalf("List(antigravity) error = %v, want %v", err, ErrListRejected)
	}
	for _, want := range []string{"403", "PERMISSION_DENIED", "Caller does not have permission"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %q, want it to name %q", err, want)
		}
	}
}

// antigravityListing is a live-shaped Cloud Code Assist answer: the vendor
// names what every model is for, and the same models are reachable under a
// legacy alias.
const antigravityListing = `{
  "models": {
    "gemini-3.8-flash-high": {"displayName": "Gemini 3.8 Flash (High)"},
    "gemini-3.8-flash-tiered": {"displayName": ""},
    "gemini-3.7-flash-low": {"displayName": "Gemini 3.7 Flash (Low)"},
    "gemini-3.6-flash-medium": {"displayName": "Gemini 3.6 Flash (Medium)"},
    "gemini-3.5-flash-extra-low": {"displayName": "Gemini 3.5 Flash (Low)"},
    "gemini-3.5-flash-lite": {"displayName": "Gemini 3.5 Flash Lite"},
    "gemini-2.5-pro": {"displayName": "Gemini 2.5 Pro"},
    "gemini-pro-agent": {"displayName": "Gemini 3.1 Pro (High)"},
    "gemini-3.1-pro-low": {"displayName": "Gemini 3.1 Pro (Low)"},
    "gemini-3.1-pro-high": {"displayName": "Gemini 3.1 Pro (High)"},
    "gemini-3.1-flash-image": {"displayName": "Gemini 3.1 Flash Image"},
    "gemini-3.1-flash-lite": {"displayName": "Gemini 3.1 Flash Lite"},
    "gemini-3-flash": {"displayName": "Gemini 3 Flash"},
    "claude-sonnet-4-6": {"displayName": "Claude Sonnet 4.6 (Thinking)"},
    "claude-opus-4-6-thinking": {"displayName": "Claude Opus 4.6 (Thinking)"},
    "gpt-oss-120b-medium": {"displayName": "GPT-OSS 120B (Medium)"},
    "chat_20706": {"displayName": ""},
    "tab_flash_lite_preview": {"displayName": ""}
  },
  "agentModelSorts": [{"displayName": "Recommended", "groups": [{"modelIds": [
    "gemini-3.8-flash-high", "gemini-3.7-flash-low", "gemini-3.6-flash-medium",
    "gemini-pro-agent", "gemini-3.1-pro-low", "claude-sonnet-4-6",
    "claude-opus-4-6-thinking", "gpt-oss-120b-medium"
  ]}]}],
  "tieredModelIds": {"flash": ["gemini-3.8-flash-tiered"], "flashLite": ["gemini-3.5-flash-lite"]},
  "imageGenerationModelIds": ["gemini-3.1-flash-image"],
  "tabModelIds": ["chat_20706", "tab_flash_lite_preview"],
  "commandModelIds": ["gemini-3-flash"],
  "webSearchModelIds": ["gemini-3.1-flash-lite"],
  "deprecatedModelIds": {"gemini-3.1-pro-high": {"newModelId": "gemini-pro-agent"}}
}`

// antigravityListIDs reads one listing against the given answer and returns
// the identifiers it published, sorted by the caller's own comparison.
func antigravityListIDs(t *testing.T, body string) []string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	listed, err := New(server.Client()).List(context.Background(), Target{
		Format: catalog.ModelsAntigravity, BaseURL: server.URL,
		Auth: upstream.Authorization{Token: "tok", KeyHeader: catalog.KeyHeaderBearer},
	})
	if err != nil {
		t.Fatalf("List(antigravity) error = %v", err)
	}
	ids := make([]string, 0, len(listed))
	for _, item := range listed {
		ids = append(ids, item.ID)
	}
	sort.Strings(ids)
	return ids
}

// TestListAntigravityKeepsOnlyTheAgentSurface is the rule the listing filter
// exists for: a model the vendor places on another surface, retires onto a
// newer generation, or marks deprecated is not a model an operator chats
// with, and a model that only a legacy alias names still is.
func TestListAntigravityKeepsOnlyTheAgentSurface(t *testing.T) {
	got := antigravityListIDs(t, antigravityListing)
	want := []string{
		"claude-opus-4-6-thinking", "claude-sonnet-4-6", "gemini-2.5-pro",
		"gemini-3.1-flash-image", "gemini-3.1-pro-low", "gemini-3.5-flash-lite",
		"gemini-3.7-flash-low", "gemini-3.8-flash-high", "gemini-3.8-flash-tiered",
		"gemini-pro-agent", "gpt-oss-120b-medium",
	}
	if len(got) != len(want) {
		t.Fatalf("listed = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("listed = %v, want %v", got, want)
		}
	}
}

// TestListAntigravityWithoutBucketsKeepsWhatItLists covers an answer that
// carries no surface buckets at all: an absent bucket is no evidence, so only
// an id the vendor is known to have retired is left out.
func TestListAntigravityWithoutBucketsKeepsWhatItLists(t *testing.T) {
	body := `{"models":{
		"gemini-3.6-flash-medium":{"displayName":"Gemini 3.6 Flash (Medium)"},
		"tab_flash_lite_preview":{"displayName":""},
		"gemini-3.8-flash-high":{"displayName":"Gemini 3.8 Flash (High)"}
	}}`
	got := antigravityListIDs(t, body)
	want := []string{"gemini-3.8-flash-high", "tab_flash_lite_preview"}
	if len(got) != len(want) {
		t.Fatalf("listed = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("listed = %v, want %v", got, want)
		}
	}
}

// openAIRoster carries the three states a listing can leave the free flag in:
// marked free, marked paid, and silent about it.
const openAIRoster = `{
  "data": [
    {"id": "stealth/glyph-cluster", "isFree": true},
    {"id": "anthropic/claude-opus-5.5", "isFree": false},
    {"id": "vendor/unmarked"}
  ]
}`

// TestListOpenAIReportsWhatTheProviderMarksFree asserts the dialect carries a
// provider's own free flag through, because a gateway publishes free models
// whose names state no price and a name test would drop them.
func TestListOpenAIReportsWhatTheProviderMarksFree(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(openAIRoster))
	}))
	t.Cleanup(server.Close)

	listed, err := New(server.Client()).List(context.Background(), Target{
		Format: catalog.ModelsOpenAI, BaseURL: server.URL, Auth: upstream.Authorization{},
	})
	if err != nil {
		t.Fatalf("List(openai) error = %v", err)
	}
	want := []*bool{new(true), new(false), nil}
	if len(listed) != len(want) {
		t.Fatalf("listed = %+v, want %d rows", listed, len(want))
	}
	for i, entry := range listed {
		if free := listed[i].IsFree; free == nil != (want[i] == nil) {
			t.Fatalf("%s IsFree = %v, want %v", entry.ID, free, want[i])
		} else if free != nil && *free != *want[i] {
			t.Fatalf("%s IsFree = %v, want %v", entry.ID, *free, *want[i])
		}
	}
}
