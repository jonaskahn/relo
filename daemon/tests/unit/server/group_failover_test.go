package server_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/account"
	"github.com/jonaskahn/relo/internal/inference"
)

// thirdCredentialRef names the stored secret of the first member's third
// account.
const thirdCredentialRef = "apikey/openai/three"

// thirdEntry is the third account of the harness connection.
func thirdEntry() account.PoolEntry {
	return account.PoolEntry{
		ID: "three", ProviderID: "openai", Kind: "api_key",
		Label: "third", SecretRef: thirdCredentialRef, Status: account.StatusActive,
	}
}

// TestRouteWalksEveryAccountBeforeTheNextMember pins the walk: three
// rate-limited accounts on the first member are each retried the full count
// before the request reaches the second member, which serves it. The member
// that served becomes the route's first choice, so the requests after it
// start there instead of walking the refused connection again.
func TestRouteWalksEveryAccountBeforeTheNextMember(t *testing.T) {
	h := newHarness(t)
	healthy := newUpstreamServer(t)
	saveAltConnectionAt(t, h, healthy.URL())
	h.secrets[spareCredentialRef] = "sk-second"
	h.secrets[thirdCredentialRef] = "sk-third"
	h.secrets[altCredentialRef] = "sk-alt"
	h.server = h.newServerWithCredentials(h.cfg.Server.Port,
		defaultEntry(), spareEntry(), thirdEntry(), altEntry())
	saveComboMembers(t, h, true)
	h.upstream.status = http.StatusTooManyRequests

	body := `{"model":"reloc-combo","stream":true,"messages":[{"role":"user","content":"hi"}]}`

	// The first request retries each account of the first member its full
	// count and then one send on the second member, which answers it.
	openBefore := h.upstream.requestCount()
	first := h.dataPlane(http.MethodPost, "/v1/chat/completions", dataPlaneToken, strings.NewReader(body))
	if first.Code != http.StatusOK {
		t.Fatalf("first status = %d, body = %s", first.Code, first.Body.String())
	}
	if got := h.upstream.requestCount() - openBefore; got != 12 {
		t.Fatalf("first member requests = %d, want every account retried three times", got)
	}
	if healthy.requestCount() != 1 {
		t.Fatalf("second member requests = %d, want the failover target to serve", healthy.requestCount())
	}
	if !askedWith(h, "sk-test") || !askedWith(h, "sk-second") || !askedWith(h, "sk-third") {
		t.Fatalf("authorizations = %v, want every account of the first member asked", authorizations(h))
	}

	// The member that served the first request is now the route's own first
	// choice: the request after it starts there, and the refused connection
	// is not walked again.
	second := h.dataPlane(http.MethodPost, "/v1/chat/completions", dataPlaneToken, strings.NewReader(body))
	if second.Code != http.StatusOK {
		t.Fatalf("second status = %d, body = %s", second.Code, second.Body.String())
	}
	if got := h.upstream.requestCount() - openBefore; got != 12 {
		t.Fatalf("first member requests after two rounds = %d, want the served member to be tried first", got)
	}
	if healthy.requestCount() != 2 {
		t.Fatalf("second member requests = %d, want the served member to answer the next request", healthy.requestCount())
	}
	third := h.dataPlane(http.MethodPost, "/v1/chat/completions", dataPlaneToken, strings.NewReader(body))
	if third.Code != http.StatusOK {
		t.Fatalf("third status = %d, body = %s", third.Code, third.Body.String())
	}
	if got := h.upstream.requestCount() - openBefore; got != 12 {
		t.Fatalf("first member requests after the sticky member = %d, want the refused connection left alone", got)
	}
	if healthy.requestCount() != 3 {
		t.Fatalf("second member requests = %d, want one per request", healthy.requestCount())
	}
}

// TestSpentRouteAnswersWithItsOwnRateLimit pins what a client reads once every
// member of a route is limited: one Relo-authored 429 naming the route, never
// a provider's message, with the wait the slowest member named.
func TestSpentRouteAnswersWithItsOwnRateLimit(t *testing.T) {
	h := newHarness(t)
	saveAltConnection(t, h)
	h.secrets[altCredentialRef] = "sk-alt"
	h.server = h.newServerWithCredentials(h.cfg.Server.Port, defaultEntry(), altEntry())
	saveComboMembers(t, h, true)
	setRetryAttempts(t, h, "openai", 1)
	setRetryAttempts(t, h, "openai-alt", 1)
	h.upstream.status = http.StatusTooManyRequests
	h.upstream.headers = map[string]string{"Retry-After": "9"}

	body := `{"model":"reloc-combo","stream":true,"messages":[{"role":"user","content":"hi"}]}`
	response := h.dataPlane(http.MethodPost, "/v1/chat/completions", dataPlaneToken, strings.NewReader(body))
	if response.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, body = %s, want the route's own 429", response.Code, response.Body.String())
	}
	if got := response.Header().Get("Retry-After"); got != "9" {
		t.Fatalf("Retry-After = %q, want the wait a member named", got)
	}
	if !strings.Contains(response.Body.String(), "group_exhausted") ||
		!strings.Contains(response.Body.String(), "combo: every model in this group") {
		t.Fatalf("body = %s, want the route named by Relo", response.Body.String())
	}
	if strings.Contains(response.Body.String(), "Hello") {
		t.Fatalf("body = %s, want nothing of the provider's body", response.Body.String())
	}
	if !askedWith(h, "sk-test") || !askedWith(h, "sk-alt") {
		t.Fatalf("authorizations = %v, want every member asked", authorizations(h))
	}
}

// TestSpentRouteAnswersAnthropicClientsWithARateLimit covers the Messages
// surface: the same spent route reads as the error type a Claude client
// already retries, with no provider words in it.
func TestSpentRouteAnswersAnthropicClientsWithARateLimit(t *testing.T) {
	h := newHarness(t)
	saveAltConnection(t, h)
	h.secrets[altCredentialRef] = "sk-alt"
	h.server = h.newServerWithCredentials(h.cfg.Server.Port, defaultEntry(), altEntry())
	saveComboMembers(t, h, true)
	setRetryAttempts(t, h, "openai", 1)
	setRetryAttempts(t, h, "openai-alt", 1)
	h.upstream.status = http.StatusTooManyRequests
	h.upstream.headers = map[string]string{"Retry-After": "9"}

	body := `{"model":"claude-reloc-combo","max_tokens":16,"stream":true,"messages":[{"role":"user","content":"hi"}]}`
	response := h.dataPlaneOn(inference.ProtocolAnthropic, http.MethodPost, "/v1/messages", dataPlaneToken,
		strings.NewReader(body))
	if response.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, body = %s, want the route's own 429", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"rate_limit_error"`) {
		t.Fatalf("body = %s, want an Anthropic rate limit", response.Body.String())
	}
	if strings.Contains(response.Body.String(), "Hello") {
		t.Fatalf("body = %s, want nothing of the provider's body", response.Body.String())
	}
}

// TestRouteKeepsTheLastMembersFaultAsItsOwnAnswer pins that only a rate limit
// is held inside a route: a spent route whose last member reported a server
// fault still answers the way it did before.
func TestRouteKeepsTheLastMembersFaultAsItsOwnAnswer(t *testing.T) {
	h := newHarness(t)
	saveAltConnection(t, h)
	h.secrets[altCredentialRef] = "sk-alt"
	h.server = h.newServerWithCredentials(h.cfg.Server.Port, defaultEntry(), altEntry())
	saveComboMembers(t, h, true)
	setRetryAttempts(t, h, "openai", 1)
	setRetryAttempts(t, h, "openai-alt", 1)
	h.upstream.status = http.StatusServiceUnavailable

	body := `{"model":"reloc-combo","stream":true,"messages":[{"role":"user","content":"hi"}]}`
	response := h.dataPlane(http.MethodPost, "/v1/chat/completions", dataPlaneToken, strings.NewReader(body))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, body = %s, want the provider's own 503", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "group_exhausted") {
		t.Fatalf("body = %s, want no route-owned rate limit", response.Body.String())
	}
}
