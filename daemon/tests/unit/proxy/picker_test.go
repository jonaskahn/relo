package proxy_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/adapters/proxy"
)

const desktopBootstrap = `{"model_selector_config":[
 {"id":"ccd","models":[{"id":"claude-sonnet-4-5","name":"Sonnet","section":"main","description":"x","badge":"new"}]},
 {"id":"ccr","models":[{"id":"claude-sonnet-4-5","name":"Sonnet","section":"main"}]},
 {"id":"chat","models":[{"id":"claude-sonnet-4-5","name":"Sonnet","section":"main"}]}],"other":1}`

type pickerRig struct {
	upstream *httptest.Server
	proxy    *proxy.PickerProxy
	armed    atomic.Bool
	rows     atomic.Value
	dialed   atomic.Value
	roots    *x509.CertPool
}

func newPickerRig(t *testing.T, bootstrap string) *pickerRig {
	t.Helper()
	rig := &pickerRig{}
	rig.armed.Store(true)
	rig.rows.Store([]proxy.CatalogRow{{ID: "claude-relo-openai--alpha", Name: "Alpha", ContextWindow: 200000}})
	rig.upstream = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/bootstrap" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, bootstrap)
			return
		}
		w.Header().Set("X-Seen-Auth", r.Header.Get("Authorization"))
		w.Header().Set("X-Seen-Host", r.Host)
		_, _ = io.WriteString(w, "plain:"+r.URL.Path)
	}))
	t.Cleanup(rig.upstream.Close)
	authority, err := proxy.LoadOrCreateAuthority(t.TempDir(), proxy.InterceptHosts...)
	if err != nil {
		t.Fatalf("LoadOrCreateAuthority() error = %v", err)
	}
	rig.roots = x509.NewCertPool()
	rig.roots.AddCert(authority.Certificate())
	pair, err := proxy.NewPicker(proxy.PickerOptions{
		Upstream: rig.upstream.URL, Authority: authority,
		Catalog: func() []proxy.CatalogRow { return rig.rows.Load().([]proxy.CatalogRow) },
		Armed:   rig.armed.Load,
		Dial: func(_ context.Context, _, address string) (net.Conn, error) {
			rig.dialed.Store(address)
			return net.Dial("tcp", rig.upstream.Listener.Addr().String())
		},
	})
	if err != nil {
		t.Fatalf("NewPicker() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	if err := pair.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() { cancel(); pair.Wait() })
	rig.proxy = pair
	return rig
}

func (r *pickerRig) client(h2 bool) *http.Client {
	proxyURL, _ := url.Parse("http://" + r.proxy.Addr())
	return &http.Client{Transport: &http.Transport{
		Proxy:             http.ProxyURL(proxyURL),
		TLSClientConfig:   &tls.Config{RootCAs: r.roots, MinVersion: tls.VersionTLS12},
		ForceAttemptHTTP2: h2,
	}}
}

func get(t *testing.T, client *http.Client, target string) (*http.Response, string) {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, target, nil)
	if err != nil {
		t.Fatalf("build the request: %v", err)
	}
	request.Header.Set("Authorization", "Bearer app-session")
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("GET %s: %v", target, err)
	}
	defer func() { _ = response.Body.Close() }()
	body, _ := io.ReadAll(response.Body)
	return response, string(body)
}

func surfaceModels(t *testing.T, body, id string) []map[string]any {
	t.Helper()
	var parsed struct {
		Surfaces []struct {
			ID     string           `json:"id"`
			Models []map[string]any `json:"models"`
		} `json:"model_selector_config"`
	}
	if err := json.Unmarshal([]byte(body), &parsed); err != nil {
		t.Fatalf("decode the bootstrap: %v\n%s", err, body)
	}
	for _, surface := range parsed.Surfaces {
		if surface.ID == id {
			return surface.Models
		}
	}
	return nil
}

// TestThePickerAddsReloModelsToTheCodeSurfacesOverBothProtocols pins the
// rewrite: the Code tab's lists gain the Relo model, the remote and chat lists
// do not, the cloned row sheds what described its template, and it works for a
// client that speaks HTTP/2 as well as one that speaks HTTP/1.1.
func TestThePickerAddsReloModelsToTheCodeSurfacesOverBothProtocols(t *testing.T) {
	rig := newPickerRig(t, desktopBootstrap)
	for _, h2 := range []bool{false, true} {
		response, body := get(t, rig.client(h2), "https://claude.ai/api/bootstrap")
		if wantMajor := map[bool]int{false: 1, true: 2}[h2]; response.ProtoMajor != wantMajor {
			t.Fatalf("h2=%v: protocol = HTTP/%d, want HTTP/%d", h2, response.ProtoMajor, wantMajor)
		}
		code := surfaceModels(t, body, "ccd")
		if len(code) != 2 || code[1]["id"] != "claude-relo-openai--alpha" || code[1]["name"] != "Alpha" {
			t.Fatalf("h2=%v: ccd models = %v, want the Relo row added", h2, code)
		}
		if _, kept := code[1]["badge"]; kept || code[1]["description"] != nil {
			t.Fatalf("h2=%v: the row kept what described its template: %v", h2, code[1])
		}
		if len(surfaceModels(t, body, "ccr")) != 1 || len(surfaceModels(t, body, "chat")) != 1 {
			t.Fatalf("h2=%v: a surface the Code tab does not read was changed: %s", h2, body)
		}
	}
}

// TestThePickerLeavesEverythingElseAlone keeps the relay honest: another path
// reaches claude.ai with the app's own credential and host, and a bootstrap it
// cannot change comes through as it was.
func TestThePickerLeavesEverythingElseAlone(t *testing.T) {
	rig := newPickerRig(t, desktopBootstrap)
	response, body := get(t, rig.client(true), "https://claude.ai/api/organizations")
	if body != "plain:/api/organizations" || response.Header.Get("X-Seen-Auth") != "Bearer app-session" {
		t.Fatalf("relayed = %q auth %q, want the path and the app's own credential", body, response.Header.Get("X-Seen-Auth"))
	}
	if response.Header.Get("X-Seen-Host") != strings.TrimPrefix(rig.upstream.URL, "http://") {
		t.Fatalf("upstream host = %q, want the upstream's own", response.Header.Get("X-Seen-Host"))
	}
	rig.rows.Store([]proxy.CatalogRow(nil))
	if _, plain := get(t, rig.client(false), "https://claude.ai/api/bootstrap"); plain != desktopBootstrap {
		t.Fatalf("bootstrap with no Relo models = %s, want the vendor's bytes", plain)
	}
}

func TestThePickerPassesABootstrapItCannotReadThrough(t *testing.T) {
	rig := newPickerRig(t, `{"model_selector_config": "unexpected"`)
	if _, body := get(t, rig.client(false), "https://claude.ai/api/bootstrap"); body != `{"model_selector_config": "unexpected"` {
		t.Fatalf("body = %s, want the vendor's bytes", body)
	}
}

// TestADisarmedPickerSplicesClaudeAIThroughUntouched pins the fail-open: while
// it is not armed the app is handed claude.ai's own connection, so the app
// never meets a certificate its operating system does not trust.
func TestADisarmedPickerSplicesClaudeAIThroughUntouched(t *testing.T) {
	rig := newPickerRig(t, desktopBootstrap)
	rig.armed.Store(false)
	request, _ := http.NewRequest(http.MethodGet, "https://claude.ai/api/bootstrap", nil)
	if response, err := rig.client(false).Do(request); err == nil {
		_ = response.Body.Close()
		t.Fatal("a disarmed picker answered with Relo's certificate")
	}
	if rig.dialed.Load() != "claude.ai:443" {
		t.Fatalf("dialed = %v, want claude.ai spliced through", rig.dialed.Load())
	}
}

func TestThePickerSplicesAnyOtherHostThrough(t *testing.T) {
	rig := newPickerRig(t, desktopBootstrap)
	conn, err := net.Dial("tcp", rig.proxy.Addr())
	if err != nil {
		t.Fatalf("dial the picker: %v", err)
	}
	defer func() { _ = conn.Close() }()
	_, _ = io.WriteString(conn, "CONNECT example.com:80 HTTP/1.1\r\nHost: example.com:80\r\n\r\n")
	_, _ = io.WriteString(conn, "GET /api/other HTTP/1.1\r\nHost: example.com\r\nConnection: close\r\n\r\n")
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	answer, _ := io.ReadAll(conn)
	if !strings.Contains(string(answer), "200 Connection Established") || !strings.Contains(string(answer), "plain:/api/other") {
		t.Fatalf("answer = %q, want the tunnel and the upstream's reply", answer)
	}
	if rig.dialed.Load() != "example.com:80" {
		t.Fatalf("dialed = %v, want the requested host", rig.dialed.Load())
	}
}

// TestThePickerIsNotADoorIntoThisMachine keeps it from being an open proxy: no
// plain request, and no CONNECT to a loopback address.
func TestThePickerIsNotADoorIntoThisMachine(t *testing.T) {
	rig := newPickerRig(t, desktopBootstrap)
	for request, want := range map[string]string{
		"GET http://example.com/ HTTP/1.1\r\nHost: example.com\r\n\r\n": "405",
		"CONNECT 127.0.0.1:22 HTTP/1.1\r\nHost: 127.0.0.1:22\r\n\r\n":   "403",
		"CONNECT localhost:22 HTTP/1.1\r\nHost: localhost:22\r\n\r\n":   "403",
	} {
		conn, err := net.Dial("tcp", rig.proxy.Addr())
		if err != nil {
			t.Fatalf("dial the picker: %v", err)
		}
		_, _ = io.WriteString(conn, request)
		_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		answer, _ := io.ReadAll(conn)
		_ = conn.Close()
		if !strings.Contains(string(answer), want) {
			t.Fatalf("%q answered %q, want %s", request, answer, want)
		}
	}
}
