package dispatch_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/catalog"
)

// TestFreeLaneRefusalIsNotASpentAllowance pins how the free lane's own 429
// is classified: the gateway refuses a request that does not carry OpenCode's
// prompt, which says nothing about the shared allowance, so the attempt is
// refused and retryable rather than a spent lane.
func TestFreeLaneRefusalIsNotASpentAllowance(t *testing.T) {
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Retry-After", "60")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"type":"error","error":{"type":"FreeUsageLimitError","message":"Free usage limit reached. Subscribe to OpenCode Go to keep going"}}`)
	}))
	t.Cleanup(origin.Close)

	recorder := httptest.NewRecorder()
	exchange := completeExchange(origin.URL)
	exchange.Options.TemplateID = catalog.OpenCodeFreeTemplate
	result, err := newExecutor(t).Execute(context.Background(), exchange, recorder)
	if err == nil {
		t.Fatal("Execute() error = nil, want the refusal")
	}
	if !result.LaneRefused {
		t.Fatalf("result = %+v, want the lane to report a refusal", result)
	}
	if result.LaneExhausted {
		t.Fatalf("result = %+v, want no spent lane claimed", result)
	}
	if !result.Retryable {
		t.Fatalf("result = %+v, want the request to fail over", result)
	}
	if body := recorder.Body.String(); strings.Contains(body, "Subscribe to OpenCode Go") {
		t.Fatalf("body = %q, want the vendor's message kept from the client", body)
	}
}

// TestRateLimitOfAnotherConnectionIsNoFreeLaneRefusal pins that the free
// lane's own code is what classifies a refusal: another connection's 429 is
// an ordinary rate limit.
func TestRateLimitOfAnotherConnectionIsNoFreeLaneRefusal(t *testing.T) {
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"type":"error","error":{"type":"FreeUsageLimitError","message":"Free usage limit reached"}}`)
	}))
	t.Cleanup(origin.Close)

	recorder := httptest.NewRecorder()
	result, err := newExecutor(t).Execute(context.Background(), completeExchange(origin.URL), recorder)
	if err == nil {
		t.Fatal("Execute() error = nil, want the refusal")
	}
	if result.LaneRefused || result.LaneExhausted {
		t.Fatalf("result = %+v, want no free lane claim for another connection", result)
	}
}
