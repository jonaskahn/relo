// The upstream's own answer facts reach the client, and only those.
package upstream

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/wire/anthropic"
	"github.com/jonaskahn/relo/internal/inference"
)

func TestRelayAnswerFacts(t *testing.T) {
	source := http.Header{
		"Request-Id":                            {"req_1"},
		"Retry-After":                           {"7"},
		"X-Should-Retry":                        {"true"},
		"Anthropic-Ratelimit-Unified-5h-Status": {"allowed"},
		"Anthropic-Organization-Id":             {"org_1"},
		"Set-Cookie":                            {"a=b"},
		"Content-Length":                        {"12"},
		"Server":                                {"cloudflare"},
	}
	target := http.Header{}
	relayAnswerFacts(target, source)

	for _, name := range []string{"Request-Id", "Retry-After", "X-Should-Retry", "Anthropic-Ratelimit-Unified-5h-Status"} {
		if got := target.Get(name); got == "" {
			t.Fatalf("header %s = %q, want the upstream fact", name, got)
		}
	}
	for _, name := range []string{"Anthropic-Organization-Id", "Set-Cookie", "Content-Length", "Server"} {
		if got := target.Get(name); got != "" {
			t.Fatalf("header %s = %q, want it left out", name, got)
		}
	}
}

func TestWriteFailureKeepsTheUpstreamFacts(t *testing.T) {
	recorder := httptest.NewRecorder()
	info := &inference.ErrorInfo{Code: "rate_limit_error", Message: "slow down", Status: http.StatusTooManyRequests}

	writeFailure(recorder, anthropic.NewInbound(), info, http.Header{
		"Request-Id":  {"req_9"},
		"Retry-After": {"3"},
	})

	if recorder.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusTooManyRequests)
	}
	if got := recorder.Header().Get("Request-Id"); got != "req_9" {
		t.Fatalf("request-id = %q, want the upstream id", got)
	}
	if got := recorder.Header().Get("Retry-After"); got != "3" {
		t.Fatalf("retry-after = %q, want the wait the upstream asked for", got)
	}
}
