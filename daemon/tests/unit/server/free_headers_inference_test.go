package server_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/internal/config"
	"github.com/jonaskahn/relo/internal/inference"
)

// TestFreeLaneSendsTheClientsOwnAgent covers the identity the installed client
// labels its turns with: a client header and a root affinity both travel,
// while the per-attempt forged session stays out of the session alias.
func TestFreeLaneSendsTheClientsOwnAgent(t *testing.T) {
	h := newHarness(t)
	if err := config.UpdateConnectionsConfig(h.settingsPath, true); err != nil {
		t.Fatal(err)
	}
	repo := sqlite.NewCatalogRepo(h.db)
	if err := repo.SaveProvider(context.Background(), sqlite.ProviderRow{
		ID: catalog.OpenCodeFreeTemplate, TemplateID: catalog.OpenCodeFreeTemplate,
		Origin: "template", Label: "OpenCode Zen Free", Auth: "none",
		APIFormat: "openai-chat", BaseURL: h.upstream.URL(), ModelsFormat: "openai",
		Enabled: true, Rank: 100,
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveModel(context.Background(), sqlite.ModelRow{
		ProviderID: catalog.OpenCodeFreeTemplate, ModelID: "free-model",
		APIFormat: string(catalog.FormatOpenAIChat), Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := h.catalog.Reload(context.Background()); err != nil {
		t.Fatal(err)
	}
	h.server = h.newServerWithCredentials(h.cfg.Server.Port)

	root := "ses_" + "abcdef123456" + "AbCdEfGhIjKlMn"
	own := "ses_" + "123456abcdef" + "MnKlIjGhFeDcBa"
	request, err := http.NewRequest(http.MethodPost, "/v1/chat/completions",
		strings.NewReader(`{"model":"free-model","stream":true,"messages":[{"role":"user","content":"hi"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+dataPlaneToken)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "opencode/latest/2.2.6/cli")
	request.Header.Set("x-opencode-client", "build")
	request.Header.Set("x-opencode-session", root)
	request.Header.Set("x-session-affinity", root)
	request.Header.Set("x-session-id", root)
	request.Header.Set("x-opencode-session-id", own)

	handler, found := h.server.DataPlaneHandler(inference.ProtocolOpenAI)
	if !found {
		t.Fatal("the server serves no openai port")
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if got := h.upstream.requestCount(); got != 1 {
		t.Fatalf("upstream requests = %d, want one send", got)
	}
	seen := h.upstream.headerAt(h.upstream.requestCount() - 1)
	if seen.Get("x-opencode-client") != "build" {
		t.Fatalf("client = %q, want the agent name", seen.Get("x-opencode-client"))
	}
	for _, alias := range []string{"x-opencode-session", "x-session-affinity", "x-session-id"} {
		if seen.Get(alias) != root {
			t.Fatalf("%s = %q, want the root session %q", alias, seen.Get(alias), root)
		}
	}
	if seen.Get("x-opencode-session-id") != own {
		t.Fatalf("session-id = %q, want the client's own %q", seen.Get("x-opencode-session-id"), own)
	}
}
