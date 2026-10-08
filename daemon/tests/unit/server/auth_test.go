package server_test

import (
	"context"
	"github.com/jonaskahn/relo/internal/inference"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/access"
	appaccess "github.com/jonaskahn/relo/internal/application/access"
	"github.com/jonaskahn/relo/internal/server"
	"github.com/jonaskahn/relo/tests/testkit"
)

func TestDataPlaneAPIKeyAuth(t *testing.T) {
	harness := newHarness(t)

	// probe sends one authenticated inference request that stops at the
	// mock upstream, which is the cheapest way to prove whether a key was
	// accepted: a key the gate refuses never reaches the provider.
	probe := func(token string) *httptest.ResponseRecorder {
		t.Helper()
		return harness.dataPlane(http.MethodPost, "/v1/chat/completions", token, strings.NewReader(streamingBody()))
	}

	t.Run("a live client key passes through", func(t *testing.T) {
		if response := probe(dataPlaneToken); response.Code != http.StatusOK {
			t.Fatalf("status = %d, want the relayed response after authentication", response.Code)
		}
	})

	t.Run("a missing key returns 401", func(t *testing.T) {
		response := probe("")
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", response.Code)
		}
		assertErrorShape(t, response, "unauthorized", "missing or invalid Authorization header")
	})

	t.Run("a string that is not a relo key returns 401", func(t *testing.T) {
		for _, token := range []string{"not-the-token", "rlo_ak_", "rlo_ak_short_secret", "sk-live-abc"} {
			response := probe(token)
			if response.Code != http.StatusUnauthorized {
				t.Fatalf("%q status = %d, want 401", token, response.Code)
			}
			assertErrorShape(t, response, "unauthorized", "invalid API key")
		}
	})

	t.Run("the right key id with a wrong secret returns 401", func(t *testing.T) {
		id, _, err := access.Parse(dataPlaneToken)
		if err != nil {
			t.Fatalf("Parse() error = %v", err)
		}
		forged := access.Prefix + id + "_" + strings.Repeat("a", 43)
		if response := probe(forged); response.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401 for a forged secret", response.Code)
		}
	})

	t.Run("an unknown key id returns 401", func(t *testing.T) {
		unknown := access.Prefix + strings.Repeat("0", access.IDBytes*2) + "_" + strings.Repeat("b", 43)
		if response := probe(unknown); response.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", response.Code)
		}
	})

	t.Run("a revoked key returns 401 at once", func(t *testing.T) {
		issued, err := harness.keys.CreateAccessKey(context.Background(), appaccess.NewAccessKey{
			Name: "retired", Kind: appaccess.Agent, Client: access.CustomClient,
		})
		if err != nil {
			t.Fatalf("CreateAccessKey() error = %v", err)
		}
		if response := probe(issued.Token); response.Code != http.StatusOK {
			t.Fatalf("status = %d, want the key accepted before revocation", response.Code)
		}
		if err := harness.keys.RevokeAccessKey(context.Background(), issued.ID); err != nil {
			t.Fatalf("RevokeAccessKey() error = %v", err)
		}
		harness.server.InvalidateAccessKey(issued.ID)
		if response := probe(issued.Token); response.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401 after revocation", response.Code)
		}
	})

	t.Run("a rotated key retires its previous secret", func(t *testing.T) {
		issued, err := harness.keys.CreateAccessKey(context.Background(), appaccess.NewAccessKey{
			Name: "rotated", Kind: appaccess.Agent, Client: access.CustomClient,
		})
		if err != nil {
			t.Fatalf("CreateAccessKey() error = %v", err)
		}
		rotated, err := harness.keys.RotateAccessKey(context.Background(), issued.ID)
		if err != nil {
			t.Fatalf("RotateAccessKey() error = %v", err)
		}
		harness.server.InvalidateAccessKey(issued.ID)
		if rotated.Token == issued.Token {
			t.Fatal("rotation returned the same secret")
		}
		if response := probe(issued.Token); response.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want the old secret refused", response.Code)
		}
		if response := probe(rotated.Token); response.Code != http.StatusOK {
			t.Fatalf("status = %d, want the new secret accepted", response.Code)
		}
	})

	t.Run("an expired key returns 401", func(t *testing.T) {
		issued, err := harness.keys.CreateAccessKey(context.Background(), appaccess.NewAccessKey{
			Name: "short-lived", Kind: appaccess.Agent, Client: access.CustomClient,
			ExpiresAtMs: harness.clock.Now().Add(time.Hour).UnixMilli(),
		})
		if err != nil {
			t.Fatalf("CreateAccessKey() error = %v", err)
		}
		if response := probe(issued.Token); response.Code != http.StatusOK {
			t.Fatalf("status = %d, want the key accepted before it expires", response.Code)
		}
		harness.clock.Add(2 * time.Hour)
		if response := probe(issued.Token); response.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401 after the expiry", response.Code)
		}
	})

	t.Run("the x-api-key header carries a key too", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(streamingBody()))
		request.Header.Set("x-api-key", dataPlaneToken)
		recorder := httptest.NewRecorder()
		dataPlane, found := harness.server.DataPlaneHandler(inference.ProtocolOpenAI)
		if !found {
			t.Fatal("the server serves no OpenAI-compatible port")
		}
		dataPlane.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, want the authenticated response", recorder.Code)
		}
	})

	t.Run("a bearer header without a value is rejected", func(t *testing.T) {
		request := request(http.MethodPost, "/v1/chat/completions", "", strings.NewReader(streamingBody()))
		request.Header.Set("Authorization", "Bearer ")
		recorder := httptest.NewRecorder()
		dataPlane, found := harness.server.DataPlaneHandler(inference.ProtocolOpenAI)
		if !found {
			t.Fatal("the server serves no OpenAI-compatible port")
		}
		dataPlane.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", recorder.Code)
		}
	})

	t.Run("healthz works without a key", func(t *testing.T) {
		if response := harness.dataPlane(http.MethodGet, "/healthz", "", nil); response.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", response.Code)
		}
	})

	t.Run("the admin token does not open the data plane", func(t *testing.T) {
		if response := probe(adminToken); response.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401: the admin token is a management credential", response.Code)
		}
	})

	t.Run("the admin token does not open the inference surface", func(t *testing.T) {
		response := harness.dataPlane(http.MethodPost, "/v1/chat/completions", adminToken, strings.NewReader(streamingBody()))
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401: the admin token is a management credential", response.Code)
		}
	})

	t.Run("a client key does not open the management API", func(t *testing.T) {
		if response := harness.management(http.MethodGet, "/api/v1/status", dataPlaneToken, nil); response.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401: a client key is an inference credential", response.Code)
		}
	})
}

// TestAccessKeyGateWithoutAStore covers a build that has no key store at
// all: nothing authenticates, and the failure is not a panic.
func TestAccessKeyGateWithoutAStore(t *testing.T) {
	gate := server.NewAccessKeyGate(nil, nil, nil)
	if _, err := gate.Verify(context.Background(), "rlo_ak_abc_def"); err == nil {
		t.Fatal("Verify() error = nil, want a refusal without a store")
	}
	gate.Invalidate("anything")
}

func TestEnsureToken(t *testing.T) {
	t.Run("EnsureToken creates file on first call", func(t *testing.T) {
		home := testkit.TempHome(t)
		path := server.TokenPath(home, server.AdminTokenFile)
		token, err := server.EnsureToken(path)
		if err != nil {
			t.Fatalf("EnsureToken() error = %v", err)
		}
		if len(token) != 64 {
			t.Fatalf("token length = %d, want 64 hex characters", len(token))
		}
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read token file: %v", err)
		}
		if string(content) != token {
			t.Fatalf("token file = %q, want the returned token with no newline", content)
		}
		if strings.HasSuffix(string(content), "\n") {
			t.Fatal("the token file ends with a newline")
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat token file: %v", err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("token file mode = %v, want 0600", info.Mode().Perm())
		}
	})

	t.Run("EnsureToken reads existing file", func(t *testing.T) {
		path := filepath.Join(testkit.TempHome(t), "admin-token")
		if err := os.WriteFile(path, []byte("  existing-token\n"), 0o600); err != nil {
			t.Fatalf("write token file: %v", err)
		}
		token, err := server.EnsureToken(path)
		if err != nil {
			t.Fatalf("EnsureToken() error = %v", err)
		}
		if token != "existing-token" {
			t.Fatalf("token = %q, want the trimmed file content", token)
		}
	})

	t.Run("an empty token file is rejected", func(t *testing.T) {
		path := filepath.Join(testkit.TempHome(t), "admin-token")
		if err := os.WriteFile(path, []byte("\n"), 0o600); err != nil {
			t.Fatalf("write token file: %v", err)
		}
		if _, err := server.EnsureToken(path); err == nil {
			t.Fatal("EnsureToken() error = nil, want an empty-token failure")
		}
	})

	t.Run("a client key never appears in log output", func(t *testing.T) {
		harness := newHarness(t)
		if response := harness.dataPlane(http.MethodPost, "/v1/chat/completions", dataPlaneToken, strings.NewReader(streamingBody())); response.Code != http.StatusOK {
			t.Fatalf("status = %d, want the authenticated response", response.Code)
		}
		logged := harness.logBuffer.String()
		if strings.Contains(logged, dataPlaneToken) {
			t.Fatalf("the key reached the log: %s", logged)
		}
		if harness.upstream.requestCount() != 1 {
			t.Fatalf("upstream requests = %d, want the authenticated request relayed once", harness.upstream.requestCount())
		}
	})
}
