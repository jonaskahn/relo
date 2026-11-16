package proxy_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/proxy"
)

func TestClientOfTellsDesktopSpawnedProcessesFromTheTerminal(t *testing.T) {
	cases := map[string]string{
		"claude-cli/2.1.0 (external, cli)":                        proxy.ClientCode,
		"claude-cli/2.1.0 (external, claude-desktop, agent-sdk)":  proxy.ClientDesktop,
		"claude-cli/2.1.0 (external, claude-desktop-3p)":          proxy.ClientDesktop,
		"claude-cli/2.1.0 (external, local-agent, agent-sdk/0.2)": proxy.ClientDesktop,
		"curl/8.0": proxy.ClientCode,
		"":         proxy.ClientCode,
	}
	for agent, want := range cases {
		if got := proxy.ClientOf(agent); got != want {
			t.Errorf("ClientOf(%q) = %q, want %q", agent, got, want)
		}
	}
}

// clientGate runs one intercept whose routing is admitted for the clients in
// the set, and answers where a request from a given user-agent landed.
type clientGate struct {
	vendor, dataPlane *side
	client            *http.Client
	admitted          map[string]bool
}

func newClientGate(t *testing.T) *clientGate {
	t.Helper()
	g := &clientGate{vendor: newSide(t, "vendor"), dataPlane: newSide(t, "data-plane"), admitted: map[string]bool{}}
	authority, err := proxy.LoadOrCreateAuthority(t.TempDir(), "api.anthropic.com")
	if err != nil {
		t.Fatalf("LoadOrCreateAuthority() error = %v", err)
	}
	pair, err := proxy.New(proxy.Options{
		Host: "api.anthropic.com", Upstream: g.vendor.server.URL, DataPlaneURL: g.dataPlane.server.URL,
		Token: "token", Authority: authority,
		Routes:       func(model string) bool { return strings.HasPrefix(model, "claude-relo-") },
		Admits:       func(client string) bool { return g.admitted[client] },
		Bind:         func(string, string) (proxy.Binding, bool) { return proxy.Binding{}, false },
		DataPlaneKey: func(client string) string { return "key-of-" + client },
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	if err := pair.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() { cancel(); pair.Wait() })
	g.client = interceptedClient(t, authority, pair.Addr())
	return g
}

func (g *clientGate) send(t *testing.T, userAgent, model string) {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, "https://api.anthropic.com/v1/messages",
		strings.NewReader(`{"model":"`+model+`","max_tokens":8,"messages":[]}`))
	if err != nil {
		t.Fatalf("build the request: %v", err)
	}
	request.Header.Set("User-Agent", userAgent)
	request.Header.Set("Authorization", "Bearer user-oauth")
	response, err := g.client.Do(request)
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	_, _ = io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()
}

const (
	terminalAgent = "claude-cli/2.1.0 (external, cli)"
	desktopAgent  = "claude-cli/2.1.0 (external, claude-desktop)"
)

// TestAClientOutsideProxyModeIsRelayedWhateverItAsksFor pins the gate: the one
// intercept serves both clients, but only the ones in proxy mode have their
// requests routed. A Relo-named model from a client that is not admitted goes
// to the vendor untouched, rather than to Relo under a key it never asked for.
func TestAClientOutsideProxyModeIsRelayedWhateverItAsksFor(t *testing.T) {
	g := newClientGate(t)
	g.admitted[proxy.ClientCode] = true

	g.send(t, terminalAgent, "claude-relo-openai--alpha")
	if g.dataPlane.calls.Load() != 1 || g.vendor.calls.Load() != 0 {
		t.Fatalf("terminal: data plane %d, vendor %d, want the admitted client routed", g.dataPlane.calls.Load(), g.vendor.calls.Load())
	}
	g.send(t, desktopAgent, "claude-relo-openai--alpha")
	if g.dataPlane.calls.Load() != 1 || g.vendor.calls.Load() != 1 {
		t.Fatalf("desktop: data plane %d, vendor %d, want the refused client relayed", g.dataPlane.calls.Load(), g.vendor.calls.Load())
	}

	g.admitted[proxy.ClientCode], g.admitted[proxy.ClientDesktop] = false, true
	g.send(t, terminalAgent, "claude-relo-openai--alpha")
	g.send(t, desktopAgent, "claude-relo-openai--alpha")
	if g.dataPlane.calls.Load() != 2 || g.vendor.calls.Load() != 2 {
		t.Fatalf("swapped: data plane %d, vendor %d, want one more each", g.dataPlane.calls.Load(), g.vendor.calls.Load())
	}
}

// TestEachClientPresentsItsOwnKeyToRelo keeps usage attributable: a routed
// request carries the key of the integration that sent it.
func TestEachClientPresentsItsOwnKeyToRelo(t *testing.T) {
	g := newClientGate(t)
	g.admitted[proxy.ClientCode], g.admitted[proxy.ClientDesktop] = true, true
	g.send(t, terminalAgent, "claude-relo-openai--alpha")
	if got := g.dataPlane.text(&g.dataPlane.key); got != "key-of-claude-code" {
		t.Fatalf("terminal key = %q", got)
	}
	g.send(t, desktopAgent, "claude-relo-openai--alpha")
	if got := g.dataPlane.text(&g.dataPlane.key); got != "key-of-claude-desktop" {
		t.Fatalf("desktop key = %q", got)
	}
}

// TestThePreferredAddressNeverWidensTheListener keeps the intercept on
// loopback: an address anywhere else is ignored rather than bound.
func TestThePreferredAddressNeverWidensTheListener(t *testing.T) {
	authority, err := proxy.LoadOrCreateAuthority(t.TempDir(), "api.anthropic.com")
	if err != nil {
		t.Fatalf("LoadOrCreateAuthority() error = %v", err)
	}
	pair, err := proxy.New(proxy.Options{
		Host: "api.anthropic.com", Upstream: "https://api.anthropic.com",
		Token: "token", Authority: authority, PreferredAddr: "0.0.0.0:0",
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	if err := pair.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() { cancel(); pair.Wait() })
	if !strings.HasPrefix(pair.Addr(), "127.0.0.1:") {
		t.Fatalf("address = %q, want loopback", pair.Addr())
	}
}
