package server_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/account"
)

// TestZeroTokenCompletionIsRelayedWithoutBlame covers the provider stream
// that ends before reporting usage: the relay answers with what arrived
// beside its synthesized terminal in a single attempt, and the account
// breaker stays closed because the usage-less answer is not the account's
// fault.
func TestZeroTokenCompletionIsRelayedWithoutBlame(t *testing.T) {
	harness := newHarness(t)
	harness.upstream.fixture = "openai/chat_streaming_no_usage.txt"
	harness.upstream.sse = true

	response := harness.dataPlane(http.MethodPost, "/v1/chat/completions", dataPlaneToken, strings.NewReader(streamingBody()))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s, want 200", response.Code, response.Body.String())
	}
	body := response.Body.String()
	if !strings.Contains(body, "partial answer") || !strings.Contains(body, "[DONE]") {
		t.Fatalf("body = %s, want the partial answer beside the sentinel", body)
	}
	if !strings.Contains(body, `"finish_reason":"stop"`) {
		t.Fatalf("body = %s, want the synthesized terminal", body)
	}
	if got := harness.upstream.requestCount(); got != 1 {
		t.Fatalf("upstream requests = %d, want a single attempt", got)
	}
	if health := harness.pools.GetPool("openai").Health("one"); health.State != account.BreakerClosed {
		t.Fatalf("breaker = %+v, want the usage-less answer ignored", health)
	}
}
