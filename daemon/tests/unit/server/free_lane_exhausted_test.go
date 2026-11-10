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
}
