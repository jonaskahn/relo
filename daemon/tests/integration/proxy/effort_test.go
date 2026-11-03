package proxy_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	"github.com/jonaskahn/relo/internal/catalog"
)

// effortUpstream records the reasoning control one chat request reached it
// with, which is what proves the coerced value rather than the requested one.
type effortUpstream struct {
	server *httptest.Server
	mu     sync.Mutex
	body   map[string]any
	calls  int
}

func newEffortUpstream(t *testing.T) *effortUpstream {
	t.Helper()
	upstream := &effortUpstream{}
	upstream.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstream.mu.Lock()
		upstream.calls++
		upstream.body = map[string]any{}
		_ = json.NewDecoder(r.Body).Decode(&upstream.body)
		upstream.mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, strings.Join([]string{
			`data: {"choices":[{"index":0,"delta":{"content":"Hello"}}]}`,
			``,
			`data: {"choices":[{"index":0,"delta":{},"finish_reason":"stop"}],` +
				`"usage":{"prompt_tokens":5,"completion_tokens":1}}`,
			``,
			`data: [DONE]`,
			``,
		}, "\n"))
	}))
	t.Cleanup(upstream.server.Close)
	return upstream
}

func (u *effortUpstream) URL() string { return u.server.URL }

func (u *effortUpstream) received() (map[string]any, int) {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.body, u.calls
}

func startEffortDaemon(t *testing.T, upstreamURL string) daemon {
	t.Helper()
	yes := true
	return startDaemonSeeded(t, seed{
		Provider: sqlite.ProviderRow{
			ID: "deepseek", TemplateID: "deepseek", Origin: string(catalog.OriginCustom), Label: "DeepSeek",
			Auth: string(catalog.AuthAPIKey), KeyHeader: string(catalog.KeyHeaderBearer),
			APIFormat: string(catalog.FormatOpenAIChat), BaseURL: upstreamURL + "/v1",
			ModelsFormat: string(catalog.ModelsNone), Enabled: true, Rank: 100, PoolStrategy: sqlite.StrategyLeastLoaded,
		},
		Format:    catalog.FormatOpenAIChat,
		ModelRows: []seedModel{{ID: "deepseek-v4.1-flash"}},
		Facts: []sqlite.ModelFactsRow{{
			ProviderID: "deepseek", ModelID: "deepseek-v4.1-flash", Layer: "modelsdev",
			SupportsReasoning: &yes, ReasoningEfforts: []string{"low", "high"},
		}},
		Account: "sk-test",
	})
}

// TestReasoningEffortCoercesOntoTheModelLadder reproduces the reported case:
// the client asks for medium, the catalog ladder is low/high, and the turn
// succeeds with high on the wire.
func TestReasoningEffortCoercesOntoTheModelLadder(t *testing.T) {
	upstream := newEffortUpstream(t)
	daemon := startEffortDaemon(t, upstream.URL())

	body := `{"model":"deepseek-v4.1-flash","stream":true,"reasoning_effort":"medium",` +
		`"messages":[{"role":"user","content":"hello"}]}`
	response := send(t, daemon.dataPlane, "/v1/chat/completions", dataPlaneToken, body)
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		answer, _ := io.ReadAll(response.Body)
		t.Fatalf("status = %d, body = %s", response.StatusCode, answer)
	}
	received, calls := upstream.received()
	if calls != 1 {
		t.Fatalf("upstream calls = %d, want one attempt", calls)
	}
	if received["reasoning_effort"] != "high" {
		t.Fatalf("upstream body = %v, want the coerced high effort", received)
	}
}

// TestUnknownReasoningEffortIsRefusedBeforeRouting pins that a value outside
// the vocabulary is a client error that never contacts the provider.
func TestUnknownReasoningEffortIsRefusedBeforeRouting(t *testing.T) {
	upstream := newEffortUpstream(t)
	daemon := startEffortDaemon(t, upstream.URL())

	body := `{"model":"deepseek-v4.1-flash","stream":true,"reasoning_effort":"invent",` +
		`"messages":[{"role":"user","content":"hello"}]}`
	response := send(t, daemon.dataPlane, "/v1/chat/completions", dataPlaneToken, body)
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusBadRequest {
		answer, _ := io.ReadAll(response.Body)
		t.Fatalf("status = %d, body = %s, want 400", response.StatusCode, answer)
	}
	answer, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read answer: %v", err)
	}
	if !strings.Contains(string(answer), "bad_request") || !strings.Contains(string(answer), "unknown reasoning effort") {
		t.Fatalf("answer = %s, want the vocabulary refusal", answer)
	}
	if _, calls := upstream.received(); calls != 0 {
		t.Fatalf("upstream calls = %d, want the request refused locally", calls)
	}
	event := daemon.lastUsageEvent(t)
	if event.Status != http.StatusBadRequest {
		t.Fatalf("usage status = %d, want the local refusal recorded", event.Status)
	}
}
