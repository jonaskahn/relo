package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	anthropiccodec "github.com/jonaskahn/relo/internal/adapters/wire/anthropic"
	"github.com/jonaskahn/relo/internal/inference"
)

// TestModelListingNamesEveryEntryForAnAgent pins what a coding agent reads: a
// connection's model under the relo namespace, a route under its own, and the
// long-context marker on the surface Claude Code reads.
func TestModelListingNamesEveryEntryForAnAgent(t *testing.T) {
	harness := newHarness(t)
	saveComboRoute(t, harness)
	// One connection-wide write is how an operator sizes every model at once,
	// and it is what the listing has to report.
	bulk := harness.management(http.MethodPost, "/api/v1/connections/openai/models/context", adminToken,
		strings.NewReader(`{"model_ids":["gpt-4o"],"context_window":1000000}`))
	if bulk.Code != http.StatusOK {
		t.Fatalf("bulk context status = %d, body = %s", bulk.Code, bulk.Body.String())
	}

	openAI := listOpenAIModels(t, harness, dataPlaneToken)
	// The connection is not the Claude.ai sign-in, so a million-token model
	// is published once, under the 1M suffix alone, rather than beside a
	// base-rate twin that would misreport it.
	if openAI["relo-openai-gpt-4o-1m"] != "GPT-4o 1M on OpenAI" ||
		openAI["reloc-combo-1m"] != "Combo 1M on Relo" {
		t.Fatalf("listing = %v, want the model and route names under the suffix alone", openAI)
	}
	if _, found := openAI["relo-openai-gpt-4o"]; found {
		t.Fatalf("listing = %v, want no bare name where the suffix is the entry", openAI)
	}
	for id := range openAI {
		if strings.Contains(id, "[1m]") {
			t.Fatalf("listing = %v, want no long-context marker on the OpenAI surface", openAI)
		}
	}
	// The OpenAI model list is open: a missing header and a bearer that is
	// not a Relo key read the same catalog as a live key.
	for _, token := range []string{"", "not-a-key"} {
		got := listOpenAIModels(t, harness, token)
		if len(got) != len(openAI) {
			t.Fatalf("listing with %q = %v, want the same models as a live key", token, got)
		}
		for id, name := range openAI {
			if got[id] != name {
				t.Fatalf("listing with %q = %v, want %s named %s", token, got, id, name)
			}
		}
	}
	if response := harness.dataPlane(http.MethodPost, "/v1/chat/completions", "", strings.NewReader(`{}`)); response.Code != http.StatusUnauthorized {
		t.Fatalf("chat status = %d, want the inference surface to keep requiring a key", response.Code)
	}

	messages := listAnthropicModels(t, harness)
	// The entry is a million tokens on its own connection without the beta
	// twin, so it keeps the suffixed name: the marker is its entry, not a
	// second row beside a base one.
	if messages["claude-relo-openai--gpt-4o[1m]"] != "GPT-4o 1M on OpenAI" {
		t.Fatalf("listing = %v, want the 1M model under its suffixed name", messages)
	}
	if _, bare := messages["claude-relo-openai--gpt-4o"]; bare {
		t.Fatalf("listing = %v, want no bare name where the suffix is the entry", messages)
	}
	if _, glued := messages["claude-relo-openai-gpt-4o[1m]"]; glued {
		t.Fatalf("listing = %v, want the model id after the separator", messages)
	}
	// The route above the same member reads the same way.
	if messages["claude-reloc-combo[1m]"] != "Combo 1M on Relo" {
		t.Fatalf("listing = %v, want the 1M route under its suffixed name", messages)
	}
	for id := range messages {
		if !strings.HasPrefix(id, "claude-") {
			t.Fatalf("listing = %v, want every Anthropic id to start with claude-", messages)
		}
	}
}

// TestAnAgentNameReachesTheProvider proves the name a client asks for is the
// one Relo listed: the namespace is Relo's, and the upstream is asked for the
// provider's own identifier.
func TestAnAgentNameReachesTheProvider(t *testing.T) {
	harness := newHarness(t)
	body := `{"model":"relo-openai-gpt-4o","stream":true,"messages":[{"role":"user","content":"hi"}]}`
	response := harness.dataPlane(http.MethodPost, "/v1/chat/completions", dataPlaneToken, strings.NewReader(body))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if request := harness.upstream.lastBody(); !strings.Contains(request, `"model":"gpt-4o"`) {
		t.Fatalf("upstream request = %q, want the provider's own identifier", request)
	}
}

// saveComboRoute stores a route an operator named, which the listing has to
// expose under the namespace Relo owns.
func saveComboRoute(t *testing.T, harness *harness) {
	t.Helper()
	body := `{"label":"Combo","strategy":"priority","enabled":true,"listed":true,` +
		`"members":[{"provider_id":"openai","model_id":"gpt-4o","kind":"model","weight":1,"enabled":true}]}`
	if recorder := harness.management(http.MethodPut, "/api/v1/routes/combo", adminToken, strings.NewReader(body)); recorder.Code != http.StatusOK {
		t.Fatalf("save route status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}

// listOpenAIModels reads the OpenAI-port listing, including the label beside
// each identifier. An empty token sends no Authorization header.
func listOpenAIModels(t *testing.T, harness *harness, token string) map[string]string {
	t.Helper()
	response := harness.dataPlane(http.MethodGet, "/v1/models", token, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("models status = %d, body = %s", response.Code, response.Body.String())
	}
	payload := struct {
		Models []struct {
			ID   string `json:"slug"`
			Name string `json:"display_name"`
		} `json:"models"`
	}{}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode the listing: %v", err)
	}
	names := make(map[string]string, len(payload.Models))
	for _, entry := range payload.Models {
		names[entry.ID] = entry.Name
	}
	return names
}

// listAnthropicModels reads the Claude listing, including the label the
// picker shows beside each identifier.
func listAnthropicModels(t *testing.T, harness *harness) map[string]string {
	t.Helper()
	handler, found := harness.server.DataPlaneHandler(inference.ProtocolAnthropic)
	if !found {
		t.Fatal("the server serves no anthropic port")
	}
	request := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	request.Header.Set("Authorization", "Bearer "+dataPlaneToken)
	request.Header.Set(anthropiccodec.VersionHeader, anthropiccodec.APIVersion)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("models status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	payload := struct {
		Data []struct {
			ID   string `json:"id"`
			Name string `json:"display_name"`
		} `json:"data"`
	}{}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode the listing: %v", err)
	}
	names := make(map[string]string, len(payload.Data))
	for _, entry := range payload.Data {
		names[entry.ID] = entry.Name
	}
	return names
}
