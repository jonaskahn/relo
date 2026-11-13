package server_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	"github.com/jonaskahn/relo/internal/catalog"
)

// TestParkedModelIsRefusedWhileItsConnectionServes pins the scope of a park:
// a model that waits out a limit is refused with the wait named, and the
// other models of the same connection are still asked.
func TestParkedModelIsRefusedWhileItsConnectionServes(t *testing.T) {
	h := newHarness(t)
	saveSecondModel(t, h)
	h.server = h.newServerWithCredentials(h.cfg.Server.Port, defaultEntry())

	h.pools.MarkModelUnavailable("openai", "gpt-4o", 10*time.Minute)

	before := h.upstream.requestCount()
	response := h.dataPlane(http.MethodPost, "/v1/chat/completions", dataPlaneToken,
		strings.NewReader(`{"model":"gpt-4o","stream":true,"messages":[{"role":"user","content":"hi"}]}`))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, body = %s, want the parked model refused", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "gpt-4o") {
		t.Fatalf("body = %s, want the parked model named", response.Body.String())
	}
	if response.Header().Get("Retry-After") == "" {
		t.Fatal("Retry-After = empty, want the wait named")
	}
	if got := h.upstream.requestCount(); got != before {
		t.Fatalf("upstream requests = %d, want the parked model not asked", got)
	}

	response = h.dataPlane(http.MethodPost, "/v1/chat/completions", dataPlaneToken,
		strings.NewReader(`{"model":"gpt-4o-mini","stream":true,"messages":[{"role":"user","content":"hi"}]}`))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s, want the other model served", response.Code, response.Body.String())
	}
	if got := h.upstream.requestCount(); got != before+1 {
		t.Fatalf("upstream requests = %d, want the other model asked", got)
	}
}

// TestRateLimitedModelKeepsItsConnectionServing pins what a rate limit does
// to the connection it happened on: the limited model waits, and the other
// models of the same connection keep being served.
func TestRateLimitedModelKeepsItsConnectionServing(t *testing.T) {
	h := newHarness(t)
	saveSecondModel(t, h)
	h.secrets[spareCredentialRef] = "sk-spare"
	h.server = h.newServerWithCredentials(h.cfg.Server.Port, defaultEntry(), spareEntry())
	// The spare account cannot serve the limited model, so that model is
	// served by the default account alone and the other model still has both.
	if err := h.pools.LearnUnavailable(context.Background(), "two", "gpt-4o"); err != nil {
		t.Fatalf("LearnUnavailable() error = %v", err)
	}
	h.upstream.modelStatuses = map[string]int{"gpt-4o": http.StatusTooManyRequests}

	// Two refusals of the same model walk it one step down the failover
	// ladder, which is where it starts waiting.
	for attempt := 0; attempt < 2; attempt++ {
		response := h.dataPlane(http.MethodPost, "/v1/chat/completions", dataPlaneToken,
			strings.NewReader(`{"model":"gpt-4o","stream":true,"messages":[{"role":"user","content":"hi"}]}`))
		if response.Code != http.StatusTooManyRequests {
			t.Fatalf("status = %d, body = %s, want the upstream rate limit", response.Code, response.Body.String())
		}
	}

	response := h.dataPlane(http.MethodPost, "/v1/chat/completions", dataPlaneToken,
		strings.NewReader(`{"model":"gpt-4o","stream":true,"messages":[{"role":"user","content":"hi"}]}`))
	if response.Code == http.StatusOK {
		t.Fatalf("status = %d, body = %s, want the limited model refused", response.Code, response.Body.String())
	}

	// The other model of the same connection is still served: the limit
	// belonged to one model, not to the connection.
	before := h.upstream.requestCount()
	response = h.dataPlane(http.MethodPost, "/v1/chat/completions", dataPlaneToken,
		strings.NewReader(`{"model":"gpt-4o-mini","stream":true,"messages":[{"role":"user","content":"hi"}]}`))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s, want the other model served", response.Code, response.Body.String())
	}
	if got := h.upstream.requestCount(); got != before+1 {
		t.Fatalf("upstream requests = %d, want the other model asked", got)
	}
}

// saveSecondModel stores a second model of the harness connection, so a test
// can tell one model's rate limit from the connection's own health.
func saveSecondModel(t *testing.T, h *harness) {
	t.Helper()
	ctx := context.Background()
	repo := sqlite.NewCatalogRepo(h.db)
	if err := repo.SaveModel(ctx, sqlite.ModelRow{
		ProviderID: "openai", ModelID: "gpt-4o-mini", Source: "manual",
		APIFormat: string(catalog.FormatOpenAIChat), Enabled: true,
	}); err != nil {
		t.Fatalf("save the second model: %v", err)
	}
}
