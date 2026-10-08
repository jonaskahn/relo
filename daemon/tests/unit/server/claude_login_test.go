package server_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/account"
	"github.com/jonaskahn/relo/internal/inference"
	"github.com/jonaskahn/relo/internal/server"
)

func TestClaudeLoginRoutesAnAliasAndForwardsANativeModel(t *testing.T) {
	harness := newHarness(t)
	const oauth = "user-oauth-token"
	body := `{"model":"claude-relo-openai-gpt-4o","max_tokens":16,"messages":[{"role":"user","content":"hi"}]}`

	loopback := anthropicRequest(http.MethodPost, "/v1/messages", oauth, body)
	loopback.RemoteAddr = "127.0.0.1:9"
	handler, found := harness.server.DataPlaneHandler(inference.ProtocolAnthropic)
	if !found {
		t.Fatal("the server serves no anthropic port")
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, loopback)
	if !strings.Contains(harness.upstream.lastBody(), `"model":"gpt-4o"`) {
		t.Fatalf("upstream body = %q, want the provider's own model", harness.upstream.lastBody())
	}
	if strings.Contains(harness.upstream.lastAuthorization(), oauth) {
		t.Fatalf("upstream authorization = %q, want the login credential dropped", harness.upstream.lastAuthorization())
	}

	remote := anthropicRequest(http.MethodPost, "/v1/messages", oauth, body)
	before := harness.upstream.requestCount()
	remoteRecorder := httptest.NewRecorder()
	handler.ServeHTTP(remoteRecorder, remote)
	if remoteRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("non-loopback status = %d, want 401", remoteRecorder.Code)
	}
	if harness.upstream.requestCount() != before {
		t.Fatal("a non-loopback login request reached the upstream")
	}

	native := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := bearer(r); got != oauth {
			t.Errorf("forwarded authorization = %q, want the caller's credential", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"msg","type":"message","role":"assistant","content":[]}`)
	}))
	t.Cleanup(native.Close)
	harness.server = harness.newServerWithOptions(harness.cfg, []account.PoolEntry{defaultEntry()}, func(options *server.Options) {
		options.NativeAnthropicURL = native.URL
	})
	handler, found = harness.server.DataPlaneHandler(inference.ProtocolAnthropic)
	if !found {
		t.Fatal("the rebuilt server serves no anthropic port")
	}
	nativeRequest := anthropicRequest(http.MethodPost, "/v1/messages", oauth,
		`{"model":"claude-opus-4","max_tokens":16,"messages":[{"role":"user","content":"hi"}]}`)
	nativeRequest.RemoteAddr = "127.0.0.1:9"
	nativeRecorder := httptest.NewRecorder()
	handler.ServeHTTP(nativeRecorder, nativeRequest)
	if nativeRecorder.Code != http.StatusOK {
		t.Fatalf("native forward status = %d, body = %s", nativeRecorder.Code, nativeRecorder.Body.String())
	}

	models := anthropicRequest(http.MethodGet, "/v1/models", "", "")
	models.RemoteAddr = "127.0.0.1:9"
	modelsRecorder := httptest.NewRecorder()
	handler.ServeHTTP(modelsRecorder, models)
	if modelsRecorder.Code != http.StatusOK {
		t.Fatalf("models status = %d, body = %s", modelsRecorder.Code, modelsRecorder.Body.String())
	}
	if !strings.Contains(modelsRecorder.Body.String(), "claude-relo-") {
		t.Fatalf("models = %s, want claude- ids", modelsRecorder.Body.String())
	}
}

func anthropicRequest(method, path, token, body string) *http.Request {
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	request := httptest.NewRequest(method, path, reader)
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	request.Header.Set("anthropic-version", "2023-06-01")
	return request
}

func bearer(r *http.Request) string {
	value := r.Header.Get("Authorization")
	return strings.TrimPrefix(value, "Bearer ")
}
