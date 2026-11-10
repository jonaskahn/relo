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

// TestSpentFreeLaneAnswersRelosOwnRefusal pins the free lane's quota answer:
// the gateway's FreeUsageLimitError spends the connection, so the request is
// not retried on another account, the lane cools for the wait the provider
// named, and the client reads Relo's own words without the vendor's upgrade
// pitch.
func TestSpentFreeLaneAnswersRelosOwnRefusal(t *testing.T) {
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
	if !strings.Contains(response.Body.String(), "free_lane_exhausted") {
		t.Fatalf("body = %s, want the free lane code", response.Body.String())
	}
	if strings.Contains(response.Body.String(), "OpenCode Go") {
		t.Fatalf("body = %s, want nothing of the vendor's message", response.Body.String())
	}
	if got := response.Header().Get("Retry-After"); got != "60" {
		t.Fatalf("Retry-After = %q, want the wait the lane named", got)
	}
	if got := h.upstream.requestCount(); got != 1 {
		t.Fatalf("upstream requests = %d, want one send on a spent lane", got)
	}
	if until := h.pools.ConnectionCooldownUntil(catalog.OpenCodeFreeTemplate); until.IsZero() {
		t.Fatal("the spent lane did not cool down")
	}

	// The next request during the cooldown reads the same refusal rather than
	// a retryable no-route, and never reaches the vendor again.
	again := h.dataPlane(http.MethodPost, "/v1/chat/completions", dataPlaneToken,
		strings.NewReader(`{"model":"free-model","stream":true,"messages":[{"role":"user","content":"hi"}]}`))
	if again.Code != http.StatusTooManyRequests {
		t.Fatalf("second status = %d, body = %s, want Relo's own 429", again.Code, again.Body.String())
	}
	if !strings.Contains(again.Body.String(), "free_lane_exhausted") {
		t.Fatalf("second body = %s, want the free lane code", again.Body.String())
	}
	if got := again.Header().Get("Retry-After"); got == "" || got == "0" {
		t.Fatalf("second Retry-After = %q, want the remaining wait", got)
	}
	if got := h.upstream.requestCount(); got != 1 {
		t.Fatalf("upstream requests = %d, want no send during the cooldown", got)
	}
}

// TestSpentFreeLaneMovesARouteToItsNextMember pins the fallback an operator
// authors: the free lane is spent, so the request leaves it at once and the
// next member serves, while later requests skip the cooling lane entirely.
func TestSpentFreeLaneMovesARouteToItsNextMember(t *testing.T) {
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
		t.Fatalf("next member requests = %d, want the route to move past the spent lane", healthy.requestCount())
	}
	if until := h.pools.LaneSpentUntil(catalog.OpenCodeFreeTemplate); until.IsZero() {
		t.Fatal("the spent lane did not cool down")
	}

	again := h.dataPlane(http.MethodPost, "/v1/chat/completions", dataPlaneToken, strings.NewReader(body))
	if again.Code != http.StatusOK {
		t.Fatalf("second status = %d, body = %s, want the next member again", again.Code, again.Body.String())
	}
	if got := h.upstream.requestCount(); got != 1 {
		t.Fatalf("free lane requests = %d, want the cooling lane skipped", got)
	}
	if healthy.requestCount() != 2 {
		t.Fatalf("next member requests = %d, want one per request", healthy.requestCount())
	}
}
