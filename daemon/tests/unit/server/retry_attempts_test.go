package server_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/inference"
)

// setRetryAttempts patches a connection's own retry count, which is what each
// test below walks the accounts with.
func setRetryAttempts(t *testing.T, h *harness, providerID string, attempts int) {
	t.Helper()
	patched := h.management(http.MethodPatch, "/api/v1/connections/"+providerID, adminToken,
		strings.NewReader(`{"retry_attempts":`+itoa(attempts)+`}`))
	if patched.Code != http.StatusOK {
		t.Fatalf("patch retry_attempts status = %d, body = %s", patched.Code, patched.Body.String())
	}
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	digits := ""
	for value > 0 {
		digits = string(rune('0'+value%10)) + digits
		value /= 10
	}
	return digits
}

// TestAccountIsRetriedItsCountThenTheNextAccountServes pins the connection
// walk: a refused account is asked its own retry count plus one, the request
// then reaches the other account, and a connection every account of which
// failed twice parks the request instead of asking again.
func TestAccountIsRetriedItsCountThenTheNextAccountServes(t *testing.T) {
	h := newHarness(t)
	h.upstream.status = http.StatusServiceUnavailable
	h.secrets[spareCredentialRef] = "sk-second"
	h.server = h.newServerWithCredentials(h.cfg.Server.Port, defaultEntry(), spareEntry())
	setRetryAttempts(t, h, "openai", 1)

	body := `{"model":"claude-relo-openai--gpt-4o","max_tokens":16,"stream":true,"messages":[{"role":"user","content":"hi"}]}`
	before := h.upstream.requestCount()

	first := h.dataPlaneOn(inference.ProtocolAnthropic, http.MethodPost, "/v1/messages", dataPlaneToken,
		strings.NewReader(body))
	if first.Code != http.StatusServiceUnavailable {
		t.Fatalf("first status = %d, body = %s, want the provider's own 503", first.Code, first.Body.String())
	}
	if got := h.upstream.requestCount() - before; got != 4 {
		t.Fatalf("upstream requests = %d, want each account retried once before the answer", got)
	}
	if !askedWith(h, "sk-test") || !askedWith(h, "sk-second") {
		t.Fatalf("authorizations = %v, want both accounts walked", authorizations(h))
	}

	second := h.dataPlaneOn(inference.ProtocolAnthropic, http.MethodPost, "/v1/messages", dataPlaneToken,
		strings.NewReader(body))
	if second.Code != http.StatusServiceUnavailable {
		t.Fatalf("second status = %d, body = %s, want the refusal again", second.Code, second.Body.String())
	}
	if got := h.upstream.requestCount() - before; got != 8 {
		t.Fatalf("upstream requests = %d, want the second round to walk both accounts again", got)
	}

	third := h.dataPlaneOn(inference.ProtocolAnthropic, http.MethodPost, "/v1/messages", dataPlaneToken,
		strings.NewReader(body))
	if third.Code != http.StatusServiceUnavailable {
		t.Fatalf("third status = %d, body = %s, want the cooling connection named", third.Code, third.Body.String())
	}
	if third.Header().Get("Retry-After") == "" {
		t.Fatal("the cooling answer names no Retry-After")
	}
	if got := h.upstream.requestCount() - before; got != 8 {
		t.Fatalf("upstream requests = %d, want the cooling connection left alone", got)
	}
}

// TestAProviderWaitLongerThanARetryMovesOn pins the walk against a provider
// that names a wait far longer than one request may spend: the account is left
// to its cooldown and the next account answers, instead of the request sitting
// on the provider's clock until the caller gives up.
func TestAProviderWaitLongerThanARetryMovesOn(t *testing.T) {
	h := newHarness(t)
	h.secrets[spareCredentialRef] = "sk-second"
	h.server = h.newServerWithCredentials(h.cfg.Server.Port, defaultEntry(), spareEntry())
	setRetryAttempts(t, h, "openai", 3)
	h.upstream.status = http.StatusTooManyRequests
	h.upstream.headers = map[string]string{"Retry-After": "1625156"}

	body := `{"model":"claude-relo-openai--gpt-4o","max_tokens":16,"stream":true,"messages":[{"role":"user","content":"hi"}]}`
	before := h.upstream.requestCount()
	started := time.Now()

	response := h.dataPlaneOn(inference.ProtocolAnthropic, http.MethodPost, "/v1/messages", dataPlaneToken,
		strings.NewReader(body))

	if response.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, body = %s, want the provider's own refusal", response.Code, response.Body.String())
	}
	if got := h.upstream.requestCount() - before; got != 2 {
		t.Fatalf("upstream requests = %d, want one per account and no long retry", got)
	}
	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Fatalf("elapsed = %v, want the answer without the provider's wait", elapsed)
	}
}

// TestConnectionKeepsTheAccountThatServed pins the connection's own memory:
// the account that answered the last request is asked first again, instead of
// the pool's rotation moving the next request to another account.
func TestConnectionKeepsTheAccountThatServed(t *testing.T) {
	h := newHarness(t)
	h.secrets[spareCredentialRef] = "sk-second"
	h.server = h.newServerWithCredentials(h.cfg.Server.Port, defaultEntry(), spareEntry())
	setRetryAttempts(t, h, "openai", 1)

	body := `{"model":"claude-relo-openai--gpt-4o","max_tokens":16,"stream":true,"messages":[{"role":"user","content":"hi"}]}`
	first := h.dataPlaneOn(inference.ProtocolAnthropic, http.MethodPost, "/v1/messages", dataPlaneToken,
		strings.NewReader(body))
	if first.Code != http.StatusOK {
		t.Fatalf("first status = %d, body = %s", first.Code, first.Body.String())
	}
	served := h.upstream.lastAuthorization()
	if served == "" {
		t.Fatal("the first request reached no account")
	}

	second := h.dataPlaneOn(inference.ProtocolAnthropic, http.MethodPost, "/v1/messages", dataPlaneToken,
		strings.NewReader(body))
	if second.Code != http.StatusOK {
		t.Fatalf("second status = %d, body = %s", second.Code, second.Body.String())
	}
	if again := h.upstream.lastAuthorization(); again != served {
		t.Fatalf("second request served by %q, want the account that served first (%q)", again, served)
	}
}

// TestGroupCoolsDownWhenEveryMemberFails pins the route's own memory: one
// request that every member refused walks the route onto its cooldown, and
// the request after it is answered at once instead of walking the members
// again.
func TestGroupCoolsDownWhenEveryMemberFails(t *testing.T) {
	h := newHarness(t)
	healthy := newUpstreamServer(t)
	healthy.status = http.StatusServiceUnavailable
	saveAltConnectionAt(t, h, healthy.URL())
	h.secrets[altCredentialRef] = "sk-alt"
	h.server = h.newServerWithCredentials(h.cfg.Server.Port, defaultEntry(), altEntry())
	saveComboMembers(t, h, true)
	setRetryAttempts(t, h, "openai", 1)
	setRetryAttempts(t, h, "openai-alt", 1)
	h.upstream.status = http.StatusServiceUnavailable

	body := `{"model":"reloc-combo","stream":true,"messages":[{"role":"user","content":"hi"}]}`
	before := h.upstream.requestCount()

	first := h.dataPlane(http.MethodPost, "/v1/chat/completions", dataPlaneToken, strings.NewReader(body))
	if first.Code != http.StatusServiceUnavailable {
		t.Fatalf("first status = %d, body = %s, want the last member's own refusal", first.Code, first.Body.String())
	}
	if got := h.upstream.requestCount() - before; got != 2 {
		t.Fatalf("first member requests = %d, want its account retried once", got)
	}
	if healthy.requestCount() != 2 {
		t.Fatalf("second member requests = %d, want its account retried once (first=%d auths=%v)",
			healthy.requestCount(), h.upstream.requestCount()-before, authorizations(h))
	}

	second := h.dataPlane(http.MethodPost, "/v1/chat/completions", dataPlaneToken, strings.NewReader(body))
	if second.Code != http.StatusServiceUnavailable {
		t.Fatalf("second status = %d, body = %s", second.Code, second.Body.String())
	}
	if got := h.upstream.requestCount() - before; got != 4 || healthy.requestCount() != 4 {
		t.Fatalf("member requests = %d/%d, want both members walked again", h.upstream.requestCount()-before, healthy.requestCount())
	}

	third := h.dataPlane(http.MethodPost, "/v1/chat/completions", dataPlaneToken, strings.NewReader(body))
	if third.Code != http.StatusServiceUnavailable {
		t.Fatalf("third status = %d, body = %s, want the cooling route named", third.Code, third.Body.String())
	}
	if third.Header().Get("Retry-After") == "" {
		t.Fatal("the cooling answer names no Retry-After")
	}
	if got := h.upstream.requestCount() - before; got != 4 || healthy.requestCount() != 4 {
		t.Fatalf("member requests = %d/%d, want the cooling route left alone", h.upstream.requestCount()-before, healthy.requestCount())
	}
}
