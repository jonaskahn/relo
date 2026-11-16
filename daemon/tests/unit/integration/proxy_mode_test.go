package integration_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/adapters/codingclients"
	"github.com/jonaskahn/relo/internal/application/integration"
	"github.com/jonaskahn/relo/internal/platform"
)

// fakeProxy is the intercept runtime a manager test points Claude Code at: it
// answers a fixed environment and records the integrations it was asked to
// run, so the test reads what the manager wired without opening a listener.
type fakeProxy struct {
	env     integration.ProxyEnv
	err     error
	ensured []string
	retired []string
}

func (f *fakeProxy) Ensure(_ context.Context, id string, _ integration.ProxyTarget) (integration.ProxyEnv, error) {
	f.ensured = append(f.ensured, id)
	if f.err != nil {
		return integration.ProxyEnv{}, f.err
	}
	return f.env, nil
}

func (f *fakeProxy) Status(string) (string, bool) { return "", true }

func (f *fakeProxy) Retire(id string) error {
	f.retired = append(f.retired, id)
	return nil
}

func proxyHarness(t *testing.T, runtime integration.ProxyRuntime) *harness {
	t.Helper()
	home := t.TempDir()
	state := t.TempDir()
	claudeHome := filepath.Join(home, ".claude")
	t.Setenv("CLAUDE_CONFIG_DIR", claudeHome)
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
	paths := codingclients.Paths{
		Home: home, StateHome: state, Relo: "/usr/local/bin/relo", DataPlane: dataPlane,
	}
	built := newStore()
	manager := integration.New(integration.Options{
		Paths: platform.NewPathResolver(paths), Agents: platform.NewAgentRegistry(),
		Files: platform.NewClientFiles(paths),
		Store: built, Now: func() time.Time { return time.Unix(1_700_000_000, 0) },
		ClaudeAuth: func() integration.ClaudeAuth { return integration.ClaudeAuthLogin },
		Proxy:      runtime,
	})
	return &harness{manager: manager, paths: paths, files: platform.NewClientFiles(paths), store: built}
}

func proxyEnvFixture() integration.ProxyEnv {
	return integration.ProxyEnv{
		URL:    "http://relo:secret@127.0.0.1:51999",
		CAPath: "/state/proxy-mode/claude-code/ca.pem",
	}
}

// TestProxyModeWritesTheInterceptAndNoGatewayKeys pins what switching to proxy
// mode leaves in Claude Code's settings: the proxy and the certificate it
// trusts, and none of the keys the settings mode wrote.
func TestProxyModeWritesTheInterceptAndNoGatewayKeys(t *testing.T) {
	runtime := &fakeProxy{env: proxyEnvFixture()}
	h := proxyHarness(t, runtime)

	if _, err := h.manager.Enable(context.Background(), "claude-code", "rlo_ak_test"); err != nil {
		t.Fatalf("Enable() in settings mode error = %v", err)
	}
	view, err := h.manager.SetMode(context.Background(), "claude-code", integration.ModeProxy)
	if err != nil {
		t.Fatalf("SetMode(proxy) error = %v", err)
	}
	if view.Mode != integration.ModeProxy {
		t.Fatalf("mode = %q, want %q", view.Mode, integration.ModeProxy)
	}
	if view.ProxyState != integration.ProxyStateApplied {
		t.Fatalf("proxy state = %q, want %q", view.ProxyState, integration.ProxyStateApplied)
	}
	settings := readSettings(t, h)
	if !strings.Contains(settings, runtime.env.URL) {
		t.Fatalf("settings = %s, want the intercept's address", settings)
	}
	if !strings.Contains(settings, runtime.env.CAPath) {
		t.Fatalf("settings = %s, want the certificate path", settings)
	}
	if strings.Contains(settings, "ANTHROPIC_BASE_URL") || strings.Contains(settings, integration.ClaudeGatewayDiscoveryEnv) {
		t.Fatalf("settings = %s, want no key the settings mode wrote", settings)
	}
	if strings.Contains(settings, "apiKeyHelper") {
		t.Fatalf("settings = %s, want no key helper in proxy mode", settings)
	}
	if len(runtime.ensured) == 0 || runtime.ensured[len(runtime.ensured)-1] != "claude-code" {
		t.Fatalf("ensured = %v, want the intercept started for claude-code", runtime.ensured)
	}
}

// TestProxyModeSwitchesBackAndRestoresTheSettings keeps the two modes
// reversible: switching back writes the settings the gateway mode writes and
// takes the intercept's own keys out.
func TestProxyModeSwitchesBackAndRestoresTheSettings(t *testing.T) {
	runtime := &fakeProxy{env: proxyEnvFixture()}
	h := proxyHarness(t, runtime)
	ctx := context.Background()

	if _, err := h.manager.Enable(ctx, "claude-code", "rlo_ak_test"); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	if _, err := h.manager.SetMode(ctx, "claude-code", integration.ModeProxy); err != nil {
		t.Fatalf("SetMode(proxy) error = %v", err)
	}
	view, err := h.manager.SetMode(ctx, "claude-code", integration.ModeEnv)
	if err != nil {
		t.Fatalf("SetMode(env) error = %v", err)
	}
	if view.Mode != integration.ModeEnv {
		t.Fatalf("mode = %q, want %q", view.Mode, integration.ModeEnv)
	}
	settings := readSettings(t, h)
	if strings.Contains(settings, "HTTPS_PROXY") || strings.Contains(settings, "NODE_EXTRA_CA_CERTS") {
		t.Fatalf("settings = %s, want the intercept's keys gone", settings)
	}
	if !strings.Contains(settings, "ANTHROPIC_BASE_URL") {
		t.Fatalf("settings = %s, want the settings mode's address back", settings)
	}
	if len(runtime.retired) == 0 {
		t.Fatal("switching back left the intercept running")
	}
}

// TestARefreshKeepsProxyModeAndFollowsTheInterceptsNewAddress pins the
// restart: the startup refresh must not write the settings mode's keys over a
// client wired through the intercept, and the new port replaces the old one.
func TestARefreshKeepsProxyModeAndFollowsTheInterceptsNewAddress(t *testing.T) {
	runtime := &fakeProxy{env: proxyEnvFixture()}
	h := proxyHarness(t, runtime)
	ctx := context.Background()
	if _, err := h.manager.Enable(ctx, "claude-code", "rlo_ak_test"); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	if _, err := h.manager.SetMode(ctx, "claude-code", integration.ModeProxy); err != nil {
		t.Fatalf("SetMode(proxy) error = %v", err)
	}
	if err := h.manager.RefreshOutdatedFiles(ctx, "claude-code", "rlo_ak_test"); err != nil {
		t.Fatalf("RefreshOutdatedFiles() error = %v", err)
	}
	if settings := readSettings(t, h); strings.Contains(settings, "ANTHROPIC_BASE_URL") || strings.Contains(settings, integration.ClaudeGatewayDiscoveryEnv) {
		t.Fatalf("settings = %s, want no key the settings mode wrote after a refresh", settings)
	}
	runtime.env.URL = "http://relo:secret@127.0.0.1:52000"
	if err := h.manager.RefreshOutdatedFiles(ctx, "claude-code", "rlo_ak_test"); err != nil {
		t.Fatalf("RefreshOutdatedFiles() after a restart error = %v", err)
	}
	settings := readSettings(t, h)
	if !strings.Contains(settings, runtime.env.URL) || strings.Contains(settings, ":51999") || strings.Contains(settings, "ANTHROPIC_BASE_URL") {
		t.Fatalf("settings = %s, want the new address only and no settings-mode keys", settings)
	}
}

// TestProxyModeRefusesWhenTheInterceptCannotStart keeps a failed bind from
// half-wiring the client: nothing is written when the runtime refuses.
func TestProxyModeRefusesWhenTheInterceptCannotStart(t *testing.T) {
	runtime := &fakeProxy{err: integration.ErrProxyUnavailable}
	h := proxyHarness(t, runtime)

	if _, err := h.manager.Enable(context.Background(), "claude-code", "rlo_ak_test"); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	before := readSettings(t, h)
	if _, err := h.manager.SetMode(context.Background(), "claude-code", integration.ModeProxy); err == nil {
		t.Fatal("SetMode(proxy) error = nil, want the runtime's refusal")
	}
	if after := readSettings(t, h); after != before {
		t.Fatalf("settings changed by a refused switch:\nbefore %s\nafter %s", before, after)
	}
}

// TestProxyModeIsClaudeCodeOnly keeps a mode with no meaning for a client out
// of its stored record.
func TestProxyModeIsClaudeCodeOnly(t *testing.T) {
	runtime := &fakeProxy{env: proxyEnvFixture()}
	h := proxyHarness(t, runtime)
	ctx := context.Background()
	if _, err := h.manager.Enable(ctx, "codex", "rlo_ak_test"); err != nil {
		t.Fatalf("Enable(codex) error = %v", err)
	}
	if _, err := h.manager.EnableWith(ctx, "codex", "rlo_ak_test", integration.EnableOptions{Mode: integration.ModeProxy}); err == nil {
		t.Fatal("EnableWith(codex, proxy) error = nil, want proxy mode refused")
	}
}

func readSettings(t *testing.T, h *harness) string {
	t.Helper()
	content, err := os.ReadFile(h.paths.ClaudeSettings())
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("read the Claude settings: %v", err)
	}
	return string(content)
}
