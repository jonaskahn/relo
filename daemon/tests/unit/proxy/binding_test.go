package proxy_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/proxy"
)

// side is a stand-in for the vendor or for Relo's data plane: it answers a
// fixed status and records what it was asked.
type side struct {
	server  *httptest.Server
	status  atomic.Int32
	calls   atomic.Int32
	body    atomic.Value
	key     atomic.Value
	auth    atomic.Value
	reached atomic.Value
}

func newSide(t *testing.T, name string) *side {
	t.Helper()
	s := &side{}
	s.status.Store(http.StatusOK)
	s.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		s.calls.Add(1)
		s.body.Store(string(body))
		s.key.Store(r.Header.Get("X-Api-Key"))
		s.auth.Store(r.Header.Get("Authorization"))
		s.reached.Store(r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(int(s.status.Load()))
		_, _ = io.WriteString(w, `{"from":"`+name+`"}`)
	}))
	t.Cleanup(s.server.Close)
	return s
}

func (s *side) text(value *atomic.Value) string {
	if loaded, ok := value.Load().(string); ok {
		return loaded
	}
	return ""
}

type bound struct {
	vendor, dataPlane *side
	client            *http.Client
}

func newBound(t *testing.T, binding proxy.Binding) *bound {
	t.Helper()
	b := &bound{vendor: newSide(t, "vendor"), dataPlane: newSide(t, "data-plane")}
	authority, err := proxy.LoadOrCreateAuthority(t.TempDir(), "api.anthropic.com")
	if err != nil {
		t.Fatalf("LoadOrCreateAuthority() error = %v", err)
	}
	pair, err := proxy.New(proxy.Options{
		Host: "api.anthropic.com", Upstream: b.vendor.server.URL, DataPlaneURL: b.dataPlane.server.URL,
		Token: "token", Authority: authority,
		Bind: func(model string) (proxy.Binding, bool) {
			return binding, strings.HasPrefix(model, "claude-haiku")
		},
		DataPlaneKey: func() string { return "relo-client-key" },
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	if err := pair.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() { cancel(); pair.Wait() })
	b.client = interceptedClient(t, authority, pair.Addr())
	return b
}

func (b *bound) post(t *testing.T, path, model string) (int, string) {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, "https://api.anthropic.com"+path,
		strings.NewReader(`{"model":"`+model+`","max_tokens":8,"stream": true,"messages":[{"role":"user","content":"hi"}]}`))
	if err != nil {
		t.Fatalf("build the request: %v", err)
	}
	request.Header.Set("Authorization", "Bearer user-oauth")
	request.Header.Set("Content-Type", "application/json")
	response, err := b.client.Do(request)
	if err != nil {
		t.Fatalf("send %s: %v", model, err)
	}
	defer func() { _ = response.Body.Close() }()
	answer, _ := io.ReadAll(response.Body)
	return response.StatusCode, string(answer)
}

const target = "claude-relo-openai--alpha"

func TestABoundIdReachesReloWithTheModelRewrittenAndNothingElseChanged(t *testing.T) {
	b := newBound(t, proxy.Binding{Rewrite: target, Reason: "binding"})
	status, answer := b.post(t, "/v1/messages", "claude-haiku-4-5-20251001")
	if status != http.StatusOK || !strings.Contains(answer, "data-plane") {
		t.Fatalf("answer = %d %s, want Relo's", status, answer)
	}
	var sent map[string]json.RawMessage
	if err := json.Unmarshal([]byte(b.dataPlane.text(&b.dataPlane.body)), &sent); err != nil {
		t.Fatalf("the data plane body is not JSON: %v", err)
	}
	if string(sent["model"]) != `"`+target+`"` || string(sent["max_tokens"]) != "8" ||
		string(sent["stream"]) != "true" || !strings.Contains(string(sent["messages"]), `"content":"hi"`) {
		t.Fatalf("data plane body = %v, want only the model changed", sent)
	}
	if b.dataPlane.text(&b.dataPlane.key) != "relo-client-key" || b.dataPlane.text(&b.dataPlane.auth) != "" {
		t.Fatal("the caller's credential reached Relo, or Relo's own key was not used")
	}
	if b.vendor.calls.Load() != 0 {
		t.Fatal("a replaced request also reached the vendor")
	}
}

func TestAnIdThatIsNotBoundStaysWithTheVendorAndItsCredential(t *testing.T) {
	b := newBound(t, proxy.Binding{Rewrite: target})
	status, _ := b.post(t, "/v1/messages", "claude-opus-5")
	if status != http.StatusOK || b.dataPlane.calls.Load() != 0 {
		t.Fatalf("status = %d, data plane calls = %d, want the vendor only", status, b.dataPlane.calls.Load())
	}
	if b.vendor.text(&b.vendor.auth) != "Bearer user-oauth" {
		t.Fatal("the vendor did not get the caller's own credential")
	}
}

func TestTheTokenCounterIsRewrittenToo(t *testing.T) {
	b := newBound(t, proxy.Binding{Rewrite: target})
	b.post(t, "/v1/messages/count_tokens", "claude-haiku-4-5")
	if b.dataPlane.text(&b.dataPlane.reached) != "/v1/messages/count_tokens" ||
		!strings.Contains(b.dataPlane.text(&b.dataPlane.body), target) {
		t.Fatal("the token counter did not follow the binding")
	}
}

func TestAReplacedRequestReloRefusedGoesToTheVendorOnceWhenAsked(t *testing.T) {
	b := newBound(t, proxy.Binding{Rewrite: target, VendorFallback: true})
	b.dataPlane.status.Store(http.StatusServiceUnavailable)
	status, answer := b.post(t, "/v1/messages", "claude-haiku-4-5")
	if status != http.StatusOK || !strings.Contains(answer, "vendor") {
		t.Fatalf("answer = %d %s, want the vendor's after Relo refused", status, answer)
	}
	if !strings.Contains(b.vendor.text(&b.vendor.body), "claude-haiku-4-5") {
		t.Fatal("the vendor was not sent the caller's own model")
	}
	if b.vendor.text(&b.vendor.auth) != "Bearer user-oauth" {
		t.Fatal("the vendor did not get the caller's own credential")
	}
	if b.dataPlane.calls.Load() != 1 || b.vendor.calls.Load() != 1 {
		t.Fatalf("calls = %d/%d, want one each", b.dataPlane.calls.Load(), b.vendor.calls.Load())
	}
}

func TestAReplacedRequestReloRefusedIsNotSentToTheVendorUnlessAsked(t *testing.T) {
	b := newBound(t, proxy.Binding{Rewrite: target})
	b.dataPlane.status.Store(http.StatusTooManyRequests)
	status, _ := b.post(t, "/v1/messages", "claude-haiku-4-5")
	if status != http.StatusTooManyRequests || b.vendor.calls.Load() != 0 {
		t.Fatalf("status = %d, vendor calls = %d, want Relo's refusal and no vendor spend", status, b.vendor.calls.Load())
	}
}

func TestACallersOwnMistakeIsNotReplayedToTheOtherSide(t *testing.T) {
	b := newBound(t, proxy.Binding{Rewrite: target, VendorFallback: true})
	b.dataPlane.status.Store(http.StatusBadRequest)
	if status, _ := b.post(t, "/v1/messages", "claude-haiku-4-5"); status != http.StatusBadRequest {
		t.Fatalf("status = %d, want the 400 kept", status)
	}
	if b.vendor.calls.Load() != 0 {
		t.Fatal("a 400 was replayed to the vendor")
	}
}

func TestVendorFirstReplaysToReloWhenTheVendorSaysWait(t *testing.T) {
	b := newBound(t, proxy.Binding{Rewrite: target, VendorFirst: true})
	b.vendor.status.Store(http.StatusTooManyRequests)
	status, answer := b.post(t, "/v1/messages", "claude-haiku-4-5")
	if status != http.StatusOK || !strings.Contains(answer, "data-plane") {
		t.Fatalf("answer = %d %s, want Relo's after the vendor refused", status, answer)
	}
	if !strings.Contains(b.dataPlane.text(&b.dataPlane.body), target) {
		t.Fatal("Relo was not sent the rewritten model")
	}
}

func TestVendorFirstKeepsASuccessfulVendorAnswerAndAClientMistake(t *testing.T) {
	b := newBound(t, proxy.Binding{Rewrite: target, VendorFirst: true})
	if status, answer := b.post(t, "/v1/messages", "claude-haiku-4-5"); status != http.StatusOK || !strings.Contains(answer, "vendor") {
		t.Fatalf("answer = %d %s, want the vendor's", status, answer)
	}
	b.vendor.status.Store(http.StatusBadRequest)
	if status, _ := b.post(t, "/v1/messages", "claude-haiku-4-5"); status != http.StatusBadRequest {
		t.Fatalf("status = %d, want the 400 kept", status)
	}
	if b.dataPlane.calls.Load() != 0 {
		t.Fatal("Relo was asked although the vendor answered")
	}
}

func TestARequestIsNeverBouncedBackAndForth(t *testing.T) {
	b := newBound(t, proxy.Binding{Rewrite: target, VendorFallback: true})
	b.dataPlane.status.Store(http.StatusServiceUnavailable)
	b.vendor.status.Store(http.StatusServiceUnavailable)
	status, _ := b.post(t, "/v1/messages", "claude-haiku-4-5")
	if status != http.StatusServiceUnavailable || b.dataPlane.calls.Load() != 1 || b.vendor.calls.Load() != 1 {
		t.Fatalf("status = %d calls = %d/%d, want one attempt on each side and the last refusal",
			status, b.dataPlane.calls.Load(), b.vendor.calls.Load())
	}
}
