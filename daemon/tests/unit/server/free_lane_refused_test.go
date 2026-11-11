package server_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/internal/config"
)

// TestRefusedFreeLaneAnswersRelosOwnRefusal pins the free lane's refusal:
// the gateway refuses a request that does not carry OpenCode's own prompt, so
// the answer names that restriction instead of a quota, no lane is marked
// spent, and the connection does not cool down.
func TestRefusedFreeLaneAnswersRelosOwnRefusal(t *testing.T) {
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
	h.upstream.rawBody = `{"type":"error","error":{"type":"FreeUsageLimitError","message":"Free usage limit reached. Subscribe to OpenCode Go to keep going: https://opencode.ai/go"}}`
	h.upstream.status = http.StatusTooManyRequests
	h.upstream.setHeaders(map[string]string{"Retry-After": "60"})

	response := h.dataPlane(http.MethodPost, "/v1/chat/completions", dataPlaneToken,
		strings.NewReader(`{"model":"free-model","stream":true,"messages":[{"role":"user","content":"hi"}]}`))
	if response.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, body = %s, want Relo's own 429", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "free_lane_refused") {
		t.Fatalf("body = %s, want the free lane code", response.Body.String())
	}
	if strings.Contains(response.Body.String(), "Free usage limit reached") {
		t.Fatalf("body = %s, want nothing of the vendor's message", response.Body.String())
	}
	if got := response.Header().Get("Retry-After"); got != "" {
		t.Fatalf("Retry-After = %q, want no promise to come back", got)
	}
	if got := h.upstream.requestCount(); got != 1 {
		t.Fatalf("upstream requests = %d, want one send on the refused lane", got)
	}
	if until := h.pools.LaneSpentUntil(catalog.OpenCodeFreeTemplate); !until.IsZero() {
		t.Fatalf("lane spent until %s, want a refused lane to stay available", until)
	}
	if until := h.pools.ConnectionCooldownUntil(catalog.OpenCodeFreeTemplate); !until.IsZero() {
		t.Fatalf("connection cools until %s, want a refused lane to stay available", until)
	}
}

// TestFreeLaneKeepsTheClientsOwnPrompt pins the middle ground the lane
// leaves open: a request that brings a system message of its own is sent with
// that message and with the client's own tools, so the gateway's prompt gate
// is satisfied by the client rather than by Relo.
func TestFreeLaneKeepsTheClientsOwnPrompt(t *testing.T) {
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

	tool := func(name string) string {
		return `{"type":"function","function":{"name":"` + name + `","description":"a tool","parameters":{"type":"object","properties":{}}}}`
	}
	body := `{"model":"free-model","stream":true,"messages":[` +
		`{"role":"system","content":"You are the client's own agent."},` +
		`{"role":"user","content":"hi"}],"tools":[` +
		tool("bash") + `,` + tool("glob") + `,` + tool("grep") + `,` + tool("read") + `,` + tool("edit") + `]}`

	response := h.dataPlane(http.MethodPost, "/v1/chat/completions", dataPlaneToken, strings.NewReader(body))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s, want the lane to serve the request", response.Code, response.Body.String())
	}
	sent := h.upstream.lastBody()
	if !strings.Contains(sent, "You are the client's own agent.") {
		t.Fatalf("upstream body = %s, want the client's own prompt", sent)
	}
	if !strings.Contains(sent, `"edit"`) {
		t.Fatalf("upstream body = %s, want the client's own tools", sent)
	}
	if strings.Contains(sent, "currently unavailable") {
		t.Fatalf("upstream body = %s, want no placeholder tool beside the client's own", sent)
	}
}

// TestRefusedFreeLaneMovesARouteToItsNextMember pins the fallback an operator
// authors: the free lane refuses, so the request leaves it at once and the
// next member serves, while the lane itself stays available for a client that
// brings its own prompt.
func TestRefusedFreeLaneMovesARouteToItsNextMember(t *testing.T) {
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
	healthy := newUpstreamServer(t)
	saveAltConnectionAt(t, h, healthy.URL())
	h.secrets[altCredentialRef] = "sk-alt"
	h.server = h.newServerWithCredentials(h.cfg.Server.Port, altEntry())
	route := `{"label":"Free then paid","strategy":"priority","enabled":true,"listed":true,"members":[{"provider_id":"opencode-free","model_id":"free-model","kind":"model","weight":1,"enabled":true},{"provider_id":"openai-alt","model_id":"gpt-4o","kind":"model","weight":1,"enabled":true}]}`
	if saved := h.management(http.MethodPut, "/api/v1/routes/free-then-paid", adminToken, strings.NewReader(route)); saved.Code != http.StatusOK {
		t.Fatalf("save route status = %d, body = %s", saved.Code, saved.Body.String())
	}
	h.upstream.rawBody = `{"type":"error","error":{"type":"FreeUsageLimitError","message":"Free usage limit reached"}}`
	h.upstream.status = http.StatusTooManyRequests
	h.upstream.setHeaders(map[string]string{"Retry-After": "60"})

	body := `{"model":"reloc-free-then-paid","stream":true,"messages":[{"role":"user","content":"hi"}]}`
	first := h.dataPlane(http.MethodPost, "/v1/chat/completions", dataPlaneToken, strings.NewReader(body))
	if first.Code != http.StatusOK {
		t.Fatalf("first status = %d, body = %s, want the next member to serve", first.Code, first.Body.String())
	}
	if got := h.upstream.requestCount(); got != 1 {
		t.Fatalf("free lane requests = %d, want one attempt before moving on", got)
	}
	if healthy.requestCount() != 1 {
		t.Fatalf("next member requests = %d, want the route to move past the refused lane", healthy.requestCount())
	}
	if until := h.pools.LaneSpentUntil(catalog.OpenCodeFreeTemplate); !until.IsZero() {
		t.Fatalf("lane spent until %s, want a refused lane to stay available", until)
	}

	// The refused lane stays available: the route moves on, and the next
	// request is served without the lane being marked spent.
	again := h.dataPlane(http.MethodPost, "/v1/chat/completions", dataPlaneToken, strings.NewReader(body))
	if again.Code != http.StatusOK {
		t.Fatalf("second status = %d, body = %s, want the next member again", again.Code, again.Body.String())
	}
	if healthy.requestCount() != 2 {
		t.Fatalf("next member requests = %d, want one per request", healthy.requestCount())
	}
	if until := h.pools.LaneSpentUntil(catalog.OpenCodeFreeTemplate); !until.IsZero() {
		t.Fatalf("lane spent until %s, want a refused lane to stay available", until)
	}
}
