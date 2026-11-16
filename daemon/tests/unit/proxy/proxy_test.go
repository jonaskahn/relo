package proxy_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/adapters/proxy"
)

// TestAuthorityIsConstrainedToItsHost pins what makes the intercept's root
// safe to hand a client: it vouches for one host and marks the constraint
// critical, so a client that trusts it for the process cannot be talked into
// trusting anything else.
func TestAuthorityIsConstrainedToItsHost(t *testing.T) {
	dir := t.TempDir()
	authority, err := proxy.LoadOrCreateAuthority(dir, "api.anthropic.com")
	if err != nil {
		t.Fatalf("LoadOrCreateAuthority() error = %v", err)
	}
	certificate := authority.Certificate()
	if !certificate.IsCA {
		t.Fatal("the intercept root is not a CA")
	}
	if !authority.HostsListed("api.anthropic.com") {
		t.Fatalf("permitted domains = %v, want the one host, marked critical", certificate.PermittedDNSDomains)
	}
	info, err := os.Stat(authority.CAPathFile())
	if err != nil {
		t.Fatalf("stat the certificate: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("certificate mode = %v, want 0600", info.Mode().Perm())
	}
}

// TestAuthorityIsStableAcrossLoads pins the key that must not change: a client
// already trusts the root on disk, so loading it again returns the same root
// rather than minting a second one.
func TestAuthorityIsStableAcrossLoads(t *testing.T) {
	dir := t.TempDir()
	first, err := proxy.LoadOrCreateAuthority(dir, "api.anthropic.com")
	if err != nil {
		t.Fatalf("LoadOrCreateAuthority() error = %v", err)
	}
	second, err := proxy.LoadOrCreateAuthority(dir, "api.anthropic.com")
	if err != nil {
		t.Fatalf("LoadOrCreateAuthority() again error = %v", err)
	}
	if !first.Certificate().Equal(second.Certificate()) {
		t.Fatal("loading the authority twice minted two roots")
	}
}

// TestLeafVerifiesOnlyForItsHost pins the constraint from the client's side:
// a leaf from this root verifies for the intercepted host and for nothing
// else.
func TestLeafVerifiesOnlyForItsHost(t *testing.T) {
	authority, err := proxy.LoadOrCreateAuthority(t.TempDir(), "api.anthropic.com")
	if err != nil {
		t.Fatalf("LoadOrCreateAuthority() error = %v", err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(authority.Certificate())
	leaf := authority.Leaf().Leaf
	if _, err := leaf.Verify(x509.VerifyOptions{Roots: roots, DNSName: "api.anthropic.com"}); err != nil {
		t.Fatalf("the leaf does not verify for its host: %v", err)
	}
	if _, err := leaf.Verify(x509.VerifyOptions{Roots: roots, DNSName: "example.com"}); err == nil {
		t.Fatal("the leaf verified for a host the root does not permit")
	}
}

// TestProxyRoutesAClaimedModelAndRelaysTheRest pins the decision the pair
// makes per request: a model Relo serves reaches the data plane with Relo's
// own client key, and everything else goes to the vendor with the caller's
// request untouched.
func TestProxyRoutesAClaimedModelAndRelaysTheRest(t *testing.T) {
	var dataPlaneKey, dataPlaneModel, vendorModel, vendorAuth string
	dataPlane := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		dataPlaneKey, dataPlaneModel = r.Header.Get("X-Api-Key"), string(body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"type":"message","role":"assistant","content":[]}`)
	}))
	defer dataPlane.Close()

	// The vendor stands in for the real host: the pair relays to it verbatim,
	// so a plain listener proves the relay without a second certificate to
	// trust in the test itself.
	vendor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		vendorModel, vendorAuth = string(body), r.Header.Get("Authorization")
		_, _ = io.WriteString(w, `{"type":"message","role":"assistant","content":[]}`)
	}))
	defer vendor.Close()

	authority, err := proxy.LoadOrCreateAuthority(t.TempDir(), "api.anthropic.com")
	if err != nil {
		t.Fatalf("LoadOrCreateAuthority() error = %v", err)
	}
	pair, err := proxy.New(proxy.Options{
		Host: "api.anthropic.com", Upstream: vendor.URL, DataPlaneURL: dataPlane.URL,
		Token: "token", Authority: authority,
		Routes:       func(model string) bool { return strings.HasPrefix(model, "claude-relo-") },
		DataPlaneKey: func(string) string { return "relo-client-key" },
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	if err := pair.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() { cancel(); pair.Wait() })

	client := interceptedClient(t, authority, pair.Addr())
	send := func(model string) {
		t.Helper()
		request, err := http.NewRequest(http.MethodPost, "https://api.anthropic.com/v1/messages",
			strings.NewReader(`{"model":"`+model+`","max_tokens":8,"messages":[]}`))
		if err != nil {
			t.Fatalf("build the request: %v", err)
		}
		request.Header.Set("Authorization", "Bearer user-oauth")
		request.Header.Set("Content-Type", "application/json")
		response, err := client.Do(request)
		if err != nil {
			t.Fatalf("send %s: %v", model, err)
		}
		answer, _ := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if response.StatusCode != http.StatusOK {
			t.Fatalf("%s = %d, want 200 (body %s)", model, response.StatusCode, answer)
		}
	}

	send("claude-relo-openai-gpt-4o")
	if !strings.Contains(dataPlaneModel, "claude-relo-openai-gpt-4o") {
		t.Fatalf("data plane body = %q, want the routed request", dataPlaneModel)
	}
	if dataPlaneKey != "relo-client-key" {
		t.Fatalf("data plane key = %q, want Relo's own key", dataPlaneKey)
	}
	if strings.Contains(dataPlaneModel, "user-oauth") {
		t.Fatal("the caller's credential reached the data plane")
	}

	send("claude-opus-4")
	if !strings.Contains(vendorModel, "claude-opus-4") {
		t.Fatalf("vendor body = %q, want the relayed request", vendorModel)
	}
	if vendorAuth != "Bearer user-oauth" {
		t.Fatalf("vendor authorization = %q, want the caller's own credential", vendorAuth)
	}
}

// TestProxyRefusesAConnectWithoutItsToken keeps the loopback listener from
// being an open proxy for anything on the machine.
func TestProxyRefusesAConnectWithoutItsToken(t *testing.T) {
	authority, err := proxy.LoadOrCreateAuthority(t.TempDir(), "api.anthropic.com")
	if err != nil {
		t.Fatalf("LoadOrCreateAuthority() error = %v", err)
	}
	pair, err := proxy.New(proxy.Options{
		Host: "api.anthropic.com", Upstream: "https://api.anthropic.com",
		Token: "token", Authority: authority,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	if err := pair.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() { cancel(); pair.Wait() })

	conn, err := net.Dial("tcp", pair.Addr())
	if err != nil {
		t.Fatalf("dial the intercept: %v", err)
	}
	defer func() { _ = conn.Close() }()
	_, _ = io.WriteString(conn, "CONNECT api.anthropic.com:443 HTTP/1.1\r\nHost: api.anthropic.com:443\r\n\r\n")
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	answer, _ := io.ReadAll(conn)
	if !strings.Contains(string(answer), "407") {
		t.Fatalf("answer = %q, want the proxy to demand its token", answer)
	}
}

// TestProxyRefusesALoopbackTarget keeps the intercept from being turned into
// a way to reach this machine through itself.
func TestProxyRefusesALoopbackTarget(t *testing.T) {
	authority, err := proxy.LoadOrCreateAuthority(t.TempDir(), "api.anthropic.com")
	if err != nil {
		t.Fatalf("LoadOrCreateAuthority() error = %v", err)
	}
	pair, err := proxy.New(proxy.Options{
		Host: "api.anthropic.com", Upstream: "https://api.anthropic.com",
		Token: "token", Authority: authority,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	if err := pair.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() { cancel(); pair.Wait() })

	credentials := "relo:token"
	conn, err := net.Dial("tcp", pair.Addr())
	if err != nil {
		t.Fatalf("dial the intercept: %v", err)
	}
	defer func() { _ = conn.Close() }()
	_, _ = io.WriteString(conn, "CONNECT 127.0.0.1:22 HTTP/1.1\r\nHost: 127.0.0.1:22\r\n"+
		"Proxy-Authorization: Basic "+basicAuth(credentials)+"\r\n\r\n")
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	answer, _ := io.ReadAll(conn)
	if !strings.Contains(string(answer), "403") {
		t.Fatalf("answer = %q, want the loopback target refused", answer)
	}
}

// interceptedClient is an HTTP client whose requests reach the intercept
// through CONNECT, trusting only the authority's own root.
func interceptedClient(t *testing.T, authority *proxy.Authority, address string) *http.Client {
	t.Helper()
	roots := x509.NewCertPool()
	roots.AddCert(authority.Certificate())
	proxyURL, err := url.Parse("http://relo:" + proxyTokenFor(t) + "@" + address)
	if err != nil {
		t.Fatalf("build the proxy address: %v", err)
	}
	return &http.Client{Transport: &http.Transport{
		Proxy:           http.ProxyURL(proxyURL),
		TLSClientConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12},
	}}
}

// proxyTokenFor reads the token the test's pair was built with.
func proxyTokenFor(*testing.T) string { return "token" }

func basicAuth(credentials string) string {
	return base64.StdEncoding.EncodeToString([]byte(credentials))
}
