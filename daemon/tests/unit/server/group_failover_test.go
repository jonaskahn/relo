package server_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/account"
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

// TestRouteWalksEveryAccountBeforeTheNextMember pins the group budget: three
// rate-limited accounts on the first member answer once each before the
// request reaches the second member, which serves it. A repeated refusal
// escalates the first member out, so the request after that starts on the
// second member.
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

	// The first request spends one send per account of the first member and
	// then one on the second member, which answers it.
	openBefore := h.upstream.requestCount()
	first := h.dataPlane(http.MethodPost, "/v1/chat/completions", dataPlaneToken, strings.NewReader(body))
	if first.Code != http.StatusOK {
		t.Fatalf("first status = %d, body = %s", first.Code, first.Body.String())
	}
	if got := h.upstream.requestCount() - openBefore; got != 3 {
		t.Fatalf("first member requests = %d, want every account once", got)
	}
	if healthy.requestCount() != 1 {
		t.Fatalf("second member requests = %d, want the failover target to serve", healthy.requestCount())
	}
	if !askedWith(h, "sk-test") || !askedWith(h, "sk-second") || !askedWith(h, "sk-third") {
		t.Fatalf("authorizations = %v, want every account of the first member asked", authorizations(h))
	}

	// The repeated refusal still starts on the first member, and the one
	// after it finds every account cooling and starts on the second member.
	second := h.dataPlane(http.MethodPost, "/v1/chat/completions", dataPlaneToken, strings.NewReader(body))
	if second.Code != http.StatusOK {
		t.Fatalf("second status = %d, body = %s", second.Code, second.Body.String())
	}
	if got := h.upstream.requestCount() - openBefore; got != 6 {
		t.Fatalf("first member requests after two rounds = %d, want each account asked twice", got)
	}
	third := h.dataPlane(http.MethodPost, "/v1/chat/completions", dataPlaneToken, strings.NewReader(body))
	if third.Code != http.StatusOK {
		t.Fatalf("third status = %d, body = %s", third.Code, third.Body.String())
	}
	if got := h.upstream.requestCount() - openBefore; got != 6 {
		t.Fatalf("first member requests after the escalation = %d, want the cooling member skipped", got)
	}
	if healthy.requestCount() != 3 {
		t.Fatalf("second member requests = %d, want one per request", healthy.requestCount())
	}
}
