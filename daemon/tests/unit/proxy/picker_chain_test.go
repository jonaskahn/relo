package proxy_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/adapters/proxy"
)

// chainRig is a picker proxy chained to a real intercept, which is the path
// Cowork's traffic takes: the app asks the picker for api.anthropic.com, the
// picker hands the tunnel to the intercept, and the intercept routes.
type chainRig struct {
	vendor, dataPlane *side
	picker            *proxy.PickerProxy
	roots             *x509.CertPool
	armed             atomic.Bool
	available         atomic.Bool
	dialed            atomic.Value
	token             string
}

func newChainRig(t *testing.T) *chainRig {
	t.Helper()
	rig := &chainRig{vendor: newSide(t, "vendor"), dataPlane: newSide(t, "data-plane"), token: "chain-token"}
	rig.armed.Store(true)
	rig.available.Store(true)
	authority, err := proxy.LoadOrCreateAuthority(t.TempDir(), proxy.InterceptHosts...)
	if err != nil {
		t.Fatalf("LoadOrCreateAuthority() error = %v", err)
	}
	rig.roots = x509.NewCertPool()
	rig.roots.AddCert(authority.Certificate())
	intercept, err := proxy.New(proxy.Options{
		Host: proxy.HostAnthropicAPI, Upstream: rig.vendor.server.URL, DataPlaneURL: rig.dataPlane.server.URL,
		Token: rig.token, Authority: authority,
		Routes:       func(model string) bool { return strings.HasPrefix(model, "claude-relo-") },
		Admits:       func(client string) bool { return client == proxy.ClientDesktop },
		Bind:         func(string, string) (proxy.Binding, bool) { return proxy.Binding{}, false },
		DataPlaneKey: func(client string) string { return "key-of-" + client },
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	if err := intercept.Start(ctx); err != nil {
		t.Fatalf("intercept Start() error = %v", err)
	}
	picker, err := proxy.NewPicker(proxy.PickerOptions{
		Upstream: rig.vendor.server.URL, Authority: authority,
		Chain: func() (string, string, bool) {
			return intercept.Addr(), rig.token, rig.available.Load()
		},
		CoworkArmed: rig.armed.Load,
		Dial: func(_ context.Context, _, address string) (net.Conn, error) {
			rig.dialed.Store(address)
			return net.Dial("tcp", rig.vendor.server.Listener.Addr().String())
		},
	})
	if err != nil {
		t.Fatalf("NewPicker() error = %v", err)
	}
	if err := picker.Start(ctx); err != nil {
		t.Fatalf("picker Start() error = %v", err)
	}
	t.Cleanup(func() { cancel(); picker.Wait(); intercept.Wait() })
	rig.picker = picker
	return rig
}

func (r *chainRig) send(model, userAgent string) (*http.Response, error) {
	proxyURL, _ := url.Parse("http://" + r.picker.Addr())
	client := &http.Client{Transport: &http.Transport{
		Proxy:             http.ProxyURL(proxyURL),
		TLSClientConfig:   &tls.Config{RootCAs: r.roots, MinVersion: tls.VersionTLS12},
		DisableKeepAlives: true,
	}}
	request, err := http.NewRequest(http.MethodPost, "https://api.anthropic.com/v1/messages",
		strings.NewReader(`{"model":"`+model+`","max_tokens":8,"messages":[]}`))
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", userAgent)
	request.Header.Set("Authorization", "Bearer user-oauth")
	return client.Do(request)
}

const coworkAgent = "claude-cli/2.1.0 (external, local-agent)"

// TestAChainedConnectionIsRoutedByTheIntercept is the Cowork path end to end: a
// model Relo serves reaches Relo under Desktop's own key and without the user's
// credential, and any other model reaches the vendor untouched.
func TestAChainedConnectionIsRoutedByTheIntercept(t *testing.T) {
	rig := newChainRig(t)

	response, err := rig.send("claude-relo-openai--alpha", coworkAgent)
	if err != nil {
		t.Fatalf("send a Relo model through the chain: %v", err)
	}
	_, _ = io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()
	if rig.dataPlane.calls.Load() != 1 || rig.vendor.calls.Load() != 0 {
		t.Fatalf("data plane %d, vendor %d, want the Relo model routed to Relo", rig.dataPlane.calls.Load(), rig.vendor.calls.Load())
	}
	if got := rig.dataPlane.text(&rig.dataPlane.key); got != "key-of-claude-desktop" {
		t.Fatalf("key presented to Relo = %q, want Desktop's own", got)
	}
	if got := rig.dataPlane.text(&rig.dataPlane.auth); got != "" {
		t.Fatalf("authorization presented to Relo = %q, want the user's credential dropped", got)
	}

	response, err = rig.send("claude-sonnet-4-5", coworkAgent)
	if err != nil {
		t.Fatalf("send a vendor model through the chain: %v", err)
	}
	_, _ = io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()
	if rig.vendor.calls.Load() != 1 {
		t.Fatalf("vendor calls = %d, want the other model relayed to the vendor", rig.vendor.calls.Load())
	}
	if got := rig.vendor.text(&rig.vendor.auth); got != "Bearer user-oauth" {
		t.Fatalf("authorization at the vendor = %q, want the caller's own", got)
	}
}

// TestWithoutTheChainArmedTheVendorHostIsSplicedThrough keeps the default: a
// picker that is not asked to chain never answers for api.anthropic.com.
func TestWithoutTheChainArmedTheVendorHostIsSplicedThrough(t *testing.T) {
	rig := newChainRig(t)
	rig.armed.Store(false)
	if _, err := rig.send("claude-relo-openai--alpha", coworkAgent); err == nil {
		t.Fatal("the request succeeded, want the spliced connection to fail the TLS check")
	}
	if rig.dialed.Load() != "api.anthropic.com:443" {
		t.Fatalf("dialed = %v, want the host spliced through to the network", rig.dialed.Load())
	}
	if rig.dataPlane.calls.Load() != 0 {
		t.Fatal("Relo was asked while the chain was not armed")
	}
}

// TestAChainThatCannotBeReachedFailsTheConnection pins the failure: the app is
// told the proxy failed rather than being handed a certificate from here.
func TestAChainThatCannotBeReachedFailsTheConnection(t *testing.T) {
	rig := newChainRig(t)
	rig.available.Store(false)
	if _, err := rig.send("claude-relo-openai--alpha", coworkAgent); err == nil {
		t.Fatal("the request succeeded, want the connection refused")
	}
	if rig.dialed.Load() != nil {
		t.Fatal("the host was spliced to the network although the chain was armed")
	}
}

// TestAChainWithTheWrongTokenFailsTheConnection keeps the intercept's own
// authentication in force: the picker does not bypass it.
func TestAChainWithTheWrongTokenFailsTheConnection(t *testing.T) {
	rig := newChainRig(t)
	rig.token = "not-the-token"
	if _, err := rig.send("claude-relo-openai--alpha", coworkAgent); err == nil {
		t.Fatal("the request succeeded, want the intercept's refusal to fail the connection")
	}
}

// TestOnlyTheVendorHostIsChained keeps the chain from being a way to reach the
// intercept for any other name.
func TestOnlyTheVendorHostIsChained(t *testing.T) {
	rig := newChainRig(t)
	proxyURL, _ := url.Parse("http://" + rig.picker.Addr())
	client := &http.Client{Transport: &http.Transport{
		Proxy:             http.ProxyURL(proxyURL),
		TLSClientConfig:   &tls.Config{RootCAs: rig.roots, MinVersion: tls.VersionTLS12},
		DisableKeepAlives: true,
	}}
	if response, err := client.Get("https://example.test/v1/messages"); err == nil {
		_ = response.Body.Close()
	}
	if rig.dialed.Load() != "example.test:443" {
		t.Fatalf("dialed = %v, want the other host spliced to the network", rig.dialed.Load())
	}
	for _, host := range rig.picker.Connections().Hosts {
		if host.Host == "api.anthropic.com" {
			t.Fatalf("hosts = %+v, want no vendor connection counted", rig.picker.Connections().Hosts)
		}
	}
}

// TestAClientThatDoesNotTrustTheRootIsNamedInTheLog is how the operator learns
// that a client, such as the Cowork VM, rejects Relo's certificate: the failed
// handshake is logged with the reason the client gave, and nothing else.
func TestAClientThatDoesNotTrustTheRootIsNamedInTheLog(t *testing.T) {
	logs := &lockedBuffer{}
	authority, err := proxy.LoadOrCreateAuthority(t.TempDir(), proxy.InterceptHosts...)
	if err != nil {
		t.Fatalf("LoadOrCreateAuthority() error = %v", err)
	}
	intercept, err := proxy.New(proxy.Options{
		Host: proxy.HostAnthropicAPI, Upstream: "http://127.0.0.1:1", DataPlaneURL: "http://127.0.0.1:1",
		Token: "token", Authority: authority, Logger: slog.New(slog.NewTextHandler(logs, nil)),
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	if err := intercept.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() { cancel(); intercept.Wait() })

	proxyURL, _ := url.Parse("http://relo:token@" + intercept.Addr())
	client := &http.Client{Transport: &http.Transport{
		Proxy:             http.ProxyURL(proxyURL),
		TLSClientConfig:   &tls.Config{RootCAs: x509.NewCertPool(), MinVersion: tls.VersionTLS12},
		DisableKeepAlives: true,
	}}
	if response, err := client.Get("https://api.anthropic.com/"); err == nil {
		_ = response.Body.Close()
		t.Fatal("the request succeeded without trusting the root")
	}
	waitForLog(t, logs, "intercept handshake failed")
	if got := logs.String(); !strings.Contains(got, "count=1") || strings.Contains(got, "BEGIN") {
		t.Fatalf("log = %s, want one counted failure and no certificate in it", got)
	}
}

func waitForLog(t *testing.T, logs *lockedBuffer, want string) {
	t.Helper()
	for attempt := 0; attempt < 200; attempt++ {
		if strings.Contains(logs.String(), want) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("log = %s, want %q", logs.String(), want)
}
