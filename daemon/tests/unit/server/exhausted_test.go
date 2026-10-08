package server_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/inference"
)

func TestExhaustedAccountIsNotAnEmptySuccess(t *testing.T) {
	harness := newHarness(t)
	harness.upstream.status = http.StatusTooManyRequests
	body := `{"model":"claude-relo-openai--gpt-4o","max_tokens":16,"stream":true,"messages":[{"role":"user","content":"hi"}]}`
	response := harness.dataPlaneOn(inference.ProtocolAnthropic, http.MethodPost, "/v1/messages", dataPlaneToken, strings.NewReader(body))
	if response.Code == http.StatusOK || response.Body.Len() == 0 {
		t.Fatalf("status = %d, body = %s, want a non-empty error", response.Code, response.Body.String())
	}
	if response.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, body = %s, want the upstream rate limit", response.Code, response.Body.String())
	}
	if got := response.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}
	if !strings.Contains(response.Body.String(), `"rate_limit_error"`) {
		t.Fatalf("body = %s, want an Anthropic rate limit", response.Body.String())
	}
}

func TestUnknownAnthropicModelIsNotFound(t *testing.T) {
	harness := newHarness(t)
	body := `{"model":"no-such-model","max_tokens":16,"messages":[{"role":"user","content":"hi"}]}`
	response := harness.dataPlaneOn(inference.ProtocolAnthropic, http.MethodPost, "/v1/messages", dataPlaneToken, strings.NewReader(body))
	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body = %s, want 404", response.Code, response.Body.String())
	}
}

func TestCoolingAccountNamesTheRetry(t *testing.T) {
	harness := newHarness(t)
	for attempt := 0; attempt < 3; attempt++ {
		harness.pools.GetPool("openai").RecordFailure("one", http.StatusTooManyRequests, 0)
	}
	body := `{"model":"claude-relo-openai--gpt-4o","max_tokens":16,"messages":[{"role":"user","content":"hi"}]}`
	response := harness.dataPlaneOn(inference.ProtocolAnthropic, http.MethodPost, "/v1/messages", dataPlaneToken, strings.NewReader(body))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, body = %s, want 503", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "cooling down") {
		t.Fatalf("body = %s, want the cooldown named", response.Body.String())
	}
}
