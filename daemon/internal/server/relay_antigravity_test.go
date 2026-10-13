package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/internal/inference"
	"github.com/jonaskahn/relo/internal/routing"
)

func antigravityTestSpec(authorized catalog.Authorization, effort string) *attemptSpec {
	outcome := &requestOutcome{
		requestID: "req-1",
		request:   &inference.Request{Model: "gemini"},
	}
	if effort != "" {
		outcome.request.Reasoning = &inference.ReasoningConfig{Effort: effort}
	}
	return &attemptSpec{
		request:   httptest.NewRequest(http.MethodPost, "/", nil),
		candidate: routing.Candidate{ProviderID: "antigravity", ModelID: "gemini", Model: catalog.Model{ID: "gemini"}},
		outcome:   outcome,
		host: catalog.Provider{
			ID: "antigravity", TemplateID: "google-antigravity", Auth: catalog.AuthOAuth,
			KeyHeader: "x-goog-api-key", Variables: map[string]string{"region": "eu", "service": "cloudaicompanion"},
		},
		authorized: authorized,
	}
}

func antigravityVariant(selection, effort, wireModel string) catalog.ModelRequestConfig {
	return catalog.EncodeAntigravityConfig(catalog.AntigravityVariant{
		LogicalID: "gemini", Selection: selection, Effort: effort,
		WireModel: wireModel, ModelEnum: "MODEL_" + wireModel, MaxOutput: 8192,
	})
}

func TestSelectAntigravityAttempt(t *testing.T) {
	first := catalog.Authorization{CredentialID: "cred-a", Label: "a", Token: "a"}
	second := catalog.Authorization{CredentialID: "cred-b", Label: "b", Token: "b"}
	configs := attemptStubConfigs{configs: map[string][]catalog.ModelRequestConfig{
		"cred-a": {antigravityVariant("high", "high", "gemini-3.8-flash-high")},
		"cred-b": {
			antigravityVariant("low", "low", "gemini-3.8-flash-low"),
			antigravityVariant("medium", "medium", "gemini-3.8-flash"),
		},
	}}
	configs.configs["cred-b"][1].Default = true

	t.Run("an eligible account keeps the attempt", func(t *testing.T) {
		relay := &attemptStubRelay{accounts: []catalog.Authorization{first, second}}
		server := attemptTestServer(relay, configs)
		selection, err := server.selectAntigravityAttempt(context.Background(), antigravityTestSpec(first, "high"), map[string][]string{})
		if err != nil {
			t.Fatalf("selectAntigravityAttempt() error = %v", err)
		}
		if selection.WireModel != "gemini-3.8-flash-high" || selection.ModelEnum != "MODEL_gemini-3.8-flash-high" {
			t.Fatalf("selection = %+v, want the high SKU", selection)
		}
		if relay.calls != 0 {
			t.Fatalf("authorize calls = %d, want no re-authorization", relay.calls)
		}
	})

	t.Run("an ineligible account yields to the next one under the connection's signing contract", func(t *testing.T) {
		relay := &attemptStubRelay{accounts: []catalog.Authorization{first, second}}
		server := attemptTestServer(relay, configs)
		tried := map[string][]string{}
		spec := antigravityTestSpec(first, "medium")
		selection, err := server.selectAntigravityAttempt(context.Background(), spec, tried)
		if err != nil {
			t.Fatalf("selectAntigravityAttempt() error = %v", err)
		}
		if spec.authorized.CredentialID != "cred-b" {
			t.Fatalf("authorized = %q, want the serving account", spec.authorized.CredentialID)
		}
		if selection.WireModel != "gemini-3.8-flash" {
			t.Fatalf("selection = %+v, want the medium SKU", selection)
		}
		if len(tried["antigravity"]) != 1 || tried["antigravity"][0] != "cred-a" {
			t.Fatalf("tried = %v, want cred-a excluded", tried)
		}
		if len(relay.requests[0].Selection.Exclude) != 1 {
			t.Fatalf("exclude = %v, want the account that just failed", relay.requests[0].Selection.Exclude)
		}
		// The renewal runs as the vendor whose Cloud Code credential is being
		// read, so the attempt has to name where the key travels and which
		// region and service the signer addresses.
		request := relay.requests[0]
		if request.Auth != catalog.AuthOAuth || request.KeyHeader != "x-goog-api-key" {
			t.Fatalf("auth request = %+v, want the connection's signing contract", request)
		}
		if request.Region != "eu" || request.Service != "cloudaicompanion" {
			t.Fatalf("region/service = %q/%q, want the connection's variables", request.Region, request.Service)
		}
	})

	t.Run("an empty effort takes the default", func(t *testing.T) {
		relay := &attemptStubRelay{accounts: []catalog.Authorization{second}}
		server := attemptTestServer(relay, configs)
		selection, err := server.selectAntigravityAttempt(context.Background(), antigravityTestSpec(second, ""), map[string][]string{})
		if err != nil {
			t.Fatalf("selectAntigravityAttempt() error = %v", err)
		}
		if selection.WireModel != "gemini-3.8-flash" {
			t.Fatalf("selection = %+v, want the default SKU", selection)
		}
	})

	t.Run("no serving account fails the selection", func(t *testing.T) {
		relay := &attemptStubRelay{accounts: []catalog.Authorization{first}}
		server := attemptTestServer(relay, configs)
		if _, err := server.selectAntigravityAttempt(context.Background(), antigravityTestSpec(first, "medium"), map[string][]string{}); err == nil {
			t.Fatal("selectAntigravityAttempt() succeeded without a serving account")
		}
	})

	t.Run("a claude model passes through with no selection", func(t *testing.T) {
		relay := &attemptStubRelay{accounts: []catalog.Authorization{first, second}}
		server := attemptTestServer(relay, configs)
		spec := antigravityTestSpec(first, "high")
		spec.candidate.ModelID = "claude-sonnet-4-6"
		spec.candidate.Model = catalog.Model{ID: "claude-sonnet-4-6"}
		tried := map[string][]string{}
		selection, err := server.selectAntigravityAttempt(context.Background(), spec, tried)
		if err != nil {
			t.Fatalf("selectAntigravityAttempt() error = %v", err)
		}
		if selection != nil {
			t.Fatalf("selection = %+v, want no SKU for a model that names its own id", selection)
		}
		if relay.calls != 0 {
			t.Fatalf("authorize calls = %d, want no re-authorization", relay.calls)
		}
		if len(tried) != 0 {
			t.Fatalf("tried = %v, want no account excluded", tried)
		}
	})
}
