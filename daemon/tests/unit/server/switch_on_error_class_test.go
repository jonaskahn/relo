package server_test

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/account"
	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/internal/inference"
	"github.com/jonaskahn/relo/internal/server"
)

// spareCredentialRef and altCredentialRef name the stored secrets of the extra
// accounts these tests switch to.
const (
	spareCredentialRef = "apikey/openai/two"
	altCredentialRef   = "apikey/openai-alt/one"
)

// spareEntry is a second account of the harness connection.
func spareEntry() account.PoolEntry {
	return account.PoolEntry{
		ID: "two", ProviderID: "openai", Kind: "api_key",
		Label: "spare", SecretRef: spareCredentialRef, Status: account.StatusActive,
	}
}

// altEntry is the one account of the second connection.
func altEntry() account.PoolEntry {
	return account.PoolEntry{
		ID: "alt-one", ProviderID: "openai-alt", Kind: "api_key",
		Label: "alt", SecretRef: altCredentialRef, Status: account.StatusActive,
	}
}

// TestUnlistedStatusSwitchesToTheNextAccount pins the connection's own choice:
// a refusal the relay does not always retry reaches another account of the
// same connection when the operator allows it, and is answered as it is when
// they do not.
func TestUnlistedStatusSwitchesToTheNextAccount(t *testing.T) {
	h := newHarness(t)
	h.upstream.status = http.StatusBadRequest
	h.secrets[spareCredentialRef] = "sk-second"
	h.server = h.newServerWithCredentials(h.cfg.Server.Port, defaultEntry(), spareEntry())
	body := `{"model":"claude-relo-openai--gpt-4o","max_tokens":16,"messages":[{"role":"user","content":"hi"}]}`

	response := h.dataPlaneOn(inference.ProtocolAnthropic, http.MethodPost, "/v1/messages", dataPlaneToken,
		strings.NewReader(body))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s, want the provider's own 400", response.Code, response.Body.String())
	}
	if !askedWith(h, "sk-second") {
		t.Fatalf("authorizations = %v, want the refusal to reach the second account", authorizations(h))
	}

	// The operator turns the switch off, and the refusal is the answer.
	patched := h.management(http.MethodPatch, "/api/v1/connections/openai", adminToken,
		strings.NewReader(`{"switch_on_4xx":false}`))
	if patched.Code != http.StatusOK {
		t.Fatalf("patch status = %d, body = %s", patched.Code, patched.Body.String())
	}
	before := h.upstream.requestCount()
	response = h.dataPlaneOn(inference.ProtocolAnthropic, http.MethodPost, "/v1/messages", dataPlaneToken,
		strings.NewReader(body))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s, want the provider's own 400", response.Code, response.Body.String())
	}
	if got := h.upstream.requestCount() - before; got != 1 {
		t.Fatalf("upstream requests = %d, want the one account the connection stays on", got)
	}
}

// TestSwitchableStatusRetriesEveryAccountBeforeLeaving pins the unified retry
// walk: a refusal the connection's own option made switchable waits the retry
// window before each next attempt, and every account of the connection answers
// before the request is answered with the refusal.
func TestSwitchableStatusRetriesEveryAccountBeforeLeaving(t *testing.T) {
	h := newHarness(t)
	h.upstream.status = http.StatusBadRequest
	h.secrets[spareCredentialRef] = "sk-second"
	var waits []int
	h.server = h.newServerWithOptions(h.cfg, []account.PoolEntry{defaultEntry(), spareEntry()},
		func(options *server.Options) {
			options.RetryDelay = func(retry int) time.Duration {
				waits = append(waits, retry)
				return time.Millisecond
			}
		})
	body := `{"model":"claude-relo-openai--gpt-4o","max_tokens":16,"messages":[{"role":"user","content":"hi"}]}`
	before := h.upstream.requestCount()

	response := h.dataPlaneOn(inference.ProtocolAnthropic, http.MethodPost, "/v1/messages", dataPlaneToken,
		strings.NewReader(body))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s, want the provider's own 400", response.Code, response.Body.String())
	}
	if !askedWith(h, "sk-test") || !askedWith(h, "sk-second") {
		t.Fatalf("authorizations = %v, want every account of the connection to answer", authorizations(h))
	}
	if len(waits) != 3 || waits[0] != 0 || waits[1] != 1 || waits[2] != 2 {
		t.Fatalf("waits = %v, want one retry window before each next attempt", waits)
	}
	if got := h.upstream.requestCount() - before; got != 3 {
		t.Fatalf("upstream requests = %d, want both accounts asked before the final answer", got)
	}
}

// TestRouteSwitchDecidesWhetherARequestReachesTheNextMember pins the route's
// own choice: a route that allows a switch on 4xx reaches its second member
// after a refusal the relay does not always retry, and a route that does not
// answers with the refusal its first member gave.
func TestRouteSwitchDecidesWhetherARequestReachesTheNextMember(t *testing.T) {
	cases := []struct {
		name        string
		switchOn4xx bool
		wantSecond  bool
	}{
		{"a route that allows the switch reaches its next member", true, true},
		{"a route that refuses the switch keeps the request where it is", false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			saveAltConnection(t, h)
			h.secrets[altCredentialRef] = "sk-alt"
			h.server = h.newServerWithCredentials(h.cfg.Server.Port, defaultEntry(), altEntry())
			saveComboMembers(t, h, tc.switchOn4xx)
			h.upstream.status = http.StatusBadRequest

			body := `{"model":"reloc-combo","stream":true,"messages":[{"role":"user","content":"hi"}]}`
			response := h.dataPlane(http.MethodPost, "/v1/chat/completions", dataPlaneToken, strings.NewReader(body))
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, body = %s, want the provider's own 400", response.Code, response.Body.String())
			}
			if askedWith(h, "sk-alt") != tc.wantSecond {
				t.Fatalf("authorizations = %v, want the second member asked = %v", authorizations(h), tc.wantSecond)
			}
		})
	}
}

// saveAltConnection stores a second connection serving the same model the
// harness connection serves, so a route can have two members.
func saveAltConnection(t *testing.T, h *harness) {
	t.Helper()
	saveAltConnectionAt(t, h, h.upstream.URL())
}

// saveAltConnectionAt stores the second connection pointing at one upstream,
// which is how a test gives the two members different upstream behavior.
func saveAltConnectionAt(t *testing.T, h *harness, baseURL string) {
	t.Helper()
	ctx := context.Background()
	repo := sqlite.NewCatalogRepo(h.db)
	if err := repo.SaveProvider(ctx, sqlite.ProviderRow{
		ID: "openai-alt", Origin: string(catalog.OriginCustom), Label: "OpenAI Alt",
		Auth: string(catalog.AuthAPIKey), APIFormat: string(catalog.FormatOpenAIChat),
		BaseURL: baseURL, ModelsFormat: string(catalog.ModelsNone),
		Enabled: true, Rank: 100, PoolStrategy: "least-loaded",
	}); err != nil {
		t.Fatalf("save the second connection: %v", err)
	}
	if err := repo.SaveModel(ctx, sqlite.ModelRow{
		ProviderID: "openai-alt", ModelID: "gpt-4o", Source: "manual",
		APIFormat: string(catalog.FormatOpenAIChat), Enabled: true,
	}); err != nil {
		t.Fatalf("save the second connection's model: %v", err)
	}
}

// saveComboMembers stores one route over both connections, in priority order,
// with the failover choice the case is about.
func saveComboMembers(t *testing.T, h *harness, switchOn4xx bool) {
	t.Helper()
	body := fmt.Sprintf(`{"label":"Combo","strategy":"priority","enabled":true,"listed":true,`+
		`"switch_on_4xx":%t,"members":[`+
		`{"provider_id":"openai","model_id":"gpt-4o","kind":"model","weight":1,"enabled":true},`+
		`{"provider_id":"openai-alt","model_id":"gpt-4o","kind":"model","weight":1,"enabled":true}]}`, switchOn4xx)
	if recorder := h.management(http.MethodPut, "/api/v1/routes/combo", adminToken,
		strings.NewReader(body)); recorder.Code != http.StatusOK {
		t.Fatalf("save route status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}

// authorizations lists the credentials the mock upstream was asked with, which
// is how a test tells which connection's account served an attempt.
func authorizations(h *harness) []string {
	h.upstream.mu.Lock()
	defer h.upstream.mu.Unlock()
	return append([]string(nil), h.upstream.auths...)
}

// askedWith reports whether the mock upstream was ever asked with one secret.
func askedWith(h *harness, secret string) bool {
	for _, header := range authorizations(h) {
		if strings.Contains(header, secret) {
			return true
		}
	}
	return false
}
