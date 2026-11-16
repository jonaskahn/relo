package service_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/codingclients"
	appintegration "github.com/jonaskahn/relo/internal/application/integration"
	"github.com/jonaskahn/relo/internal/config"
	"github.com/jonaskahn/relo/internal/platform"
	"github.com/jonaskahn/relo/tests/testkit"
)

const interceptURL = "http://relo:secret@127.0.0.1:51999"

// stubIntercept answers a fixed environment, so the test reads what the use
// cases wrote without opening a listener.
type stubIntercept struct{}

func (stubIntercept) Ensure(context.Context, string, appintegration.ProxyTarget) (appintegration.ProxyEnv, error) {
	return appintegration.ProxyEnv{URL: interceptURL, CAPath: "/state/ca/ca.pem"}, nil
}

func (stubIntercept) Status(string) (string, bool) { return "", true }

func (stubIntercept) Retire(string) error { return nil }

// refusingSave fails saving the integrations it names, which is the step a
// Desktop change takes after Claude Code's wiring is already up.
type refusingSave struct {
	appintegration.Store
	refuse func(id string) bool
}

func (s refusingSave) SaveIntegration(ctx context.Context, record appintegration.Record) error {
	if s.refuse(record.ID) {
		return errInjected
	}
	return s.Store.SaveIntegration(ctx, record)
}

// proxyDesktopService is the integration use cases over an installed Claude
// Desktop and a stub intercept. It answers the service and the Claude settings
// file Claude Code's wiring is written to.
func proxyDesktopService(t *testing.T, refuse func(id string) bool) (*appintegration.Service, string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the Desktop directory is resolved from APPDATA on Windows")
	}
	h := newHarness(t)
	home := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(home, ".claude"))
	t.Setenv("CLAUDE_USER_DATA_DIR", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	support := filepath.Join(home, ".config")
	if runtime.GOOS == "darwin" {
		support = filepath.Join(home, "Library", "Application Support")
	}
	if err := os.MkdirAll(filepath.Join(support, "Claude"), 0o755); err != nil {
		t.Fatalf("create the Desktop directory: %v", err)
	}
	settings := &config.Config{}
	settings.Server.Bind = "127.0.0.1"
	settings.Server.Port = 10101
	paths := codingclients.Paths{
		Home: home, StateHome: h.home, Relo: "/usr/local/bin/relo",
		DataPlane: func(protocol string) string { return dataPlaneURL(settings, protocol) },
	}
	var store appintegration.Store = platform.NewIntegrationStore(h.db)
	if refuse != nil {
		store = refusingSave{Store: store, refuse: refuse}
	}
	logger, _ := testkit.TestLogger(t)
	service := appintegration.NewService(appintegration.ServiceOptions{
		Paths: platform.NewPathResolver(paths), Agents: platform.NewAgentRegistry(),
		Files: platform.NewClientFiles(paths), Processes: platform.NewAgentProcesses(),
		Wire: platform.NewWireCodec(), Store: store, Keys: h.keys, Catalog: h.catalog,
		Logger: logger, Now: h.clock.Now, Proxy: stubIntercept{},
		ClaudeAuth: func() appintegration.ClaudeAuth { return appintegration.ClaudeAuthLogin },
	})
	return service, filepath.Join(home, ".claude", "settings.json")
}

func settingsOf(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(content)
}

func viewOf(t *testing.T, service *appintegration.Service, id string) appintegration.IntegrationView {
	t.Helper()
	view, err := service.Integration(context.Background(), id)
	if err != nil {
		t.Fatalf("Integration(%s) error = %v", id, err)
	}
	return view
}

// TestDesktopProxySetupSetsClaudeCodeUpWhenAsked is the one-step setup: Claude
// Code was never set up, and a Desktop setup that asks for it brings Claude
// Code's proxy wiring up first instead of being refused.
func TestDesktopProxySetupSetsClaudeCodeUpWhenAsked(t *testing.T) {
	service, settings := proxyDesktopService(t, nil)
	ctx := context.Background()
	request := appintegration.ModeRequest{Mode: appintegration.ModeProxy}
	if _, err := service.EnableIntegrationIn(ctx, "claude-desktop", "", request); !errors.Is(err, appintegration.ErrProxyOwnerNeeded) {
		t.Fatalf("setup without the flag error = %v, want %v", err, appintegration.ErrProxyOwnerNeeded)
	}
	if view := viewOf(t, service, "claude-code"); view.Enabled {
		t.Fatal("a refused Desktop setup set Claude Code up")
	}
	request.WithClaudeCode = true
	if _, err := service.EnableIntegrationIn(ctx, "claude-desktop", "", request); err != nil {
		t.Fatalf("setup with the flag error = %v", err)
	}
	code, desktop := viewOf(t, service, "claude-code"), viewOf(t, service, "claude-desktop")
	if !code.Enabled || code.Mode != appintegration.ModeProxy || !desktop.Enabled || desktop.Mode != appintegration.ModeProxy {
		t.Fatalf("code = enabled %v mode %q, desktop = enabled %v mode %q, want both on in proxy mode",
			code.Enabled, code.Mode, desktop.Enabled, desktop.Mode)
	}
	if content := settingsOf(t, settings); !strings.Contains(content, "127.0.0.1:51999") {
		t.Fatalf("settings = %s, want Claude Code's wiring to name the intercept", content)
	}
}

// TestDesktopProxySwitchMovesClaudeCodeOverWhenAsked covers Claude Code already
// set up in the settings mode, which the shared file cannot be at the same time
// as the intercept.
func TestDesktopProxySwitchMovesClaudeCodeOverWhenAsked(t *testing.T) {
	service, settings := proxyDesktopService(t, nil)
	ctx := context.Background()
	for _, id := range []string{"claude-code", "claude-desktop"} {
		if _, err := service.EnableIntegration(ctx, id, ""); err != nil {
			t.Fatalf("EnableIntegration(%s) error = %v", id, err)
		}
	}
	request := appintegration.ModeRequest{Mode: appintegration.ModeProxy}
	if _, err := service.SetIntegrationMode(ctx, "claude-desktop", request); !errors.Is(err, appintegration.ErrProxyOwnerNeeded) {
		t.Fatalf("switch without the flag error = %v, want %v", err, appintegration.ErrProxyOwnerNeeded)
	}
	if view := viewOf(t, service, "claude-code"); view.Mode != appintegration.ModeEnv {
		t.Fatalf("a refused switch moved Claude Code to %q", view.Mode)
	}
	request.WithClaudeCode = true
	if _, err := service.SetIntegrationMode(ctx, "claude-desktop", request); err != nil {
		t.Fatalf("switch with the flag error = %v", err)
	}
	if view := viewOf(t, service, "claude-code"); view.Mode != appintegration.ModeProxy {
		t.Fatalf("Claude Code mode = %q, want proxy", view.Mode)
	}
	if content := settingsOf(t, settings); !strings.Contains(content, "127.0.0.1:51999") {
		t.Fatalf("settings = %s, want the intercept in Claude Code's wiring", content)
	}
}

// TestDesktopProxyFlagLeavesClaudeCodeAloneWhenItIsReady keeps the flag from
// doing anything the request did not need.
func TestDesktopProxyFlagLeavesClaudeCodeAloneWhenItIsReady(t *testing.T) {
	service, settings := proxyDesktopService(t, nil)
	ctx := context.Background()
	if _, err := service.EnableIntegrationIn(ctx, "claude-code", "", appintegration.ModeRequest{Mode: appintegration.ModeProxy}); err != nil {
		t.Fatalf("EnableIntegrationIn(claude-code) error = %v", err)
	}
	before := settingsOf(t, settings)
	request := appintegration.ModeRequest{Mode: appintegration.ModeProxy, WithClaudeCode: true}
	if _, err := service.EnableIntegrationIn(ctx, "claude-desktop", "", request); err != nil {
		t.Fatalf("setup error = %v", err)
	}
	if after := settingsOf(t, settings); after != before {
		t.Fatalf("Claude Code's settings changed although it was already in proxy mode:\nbefore %s\nafter %s", before, after)
	}
}

// TestDesktopProxyFlagMeansNothingForOtherClients keeps the flag Desktop's.
func TestDesktopProxyFlagMeansNothingForOtherClients(t *testing.T) {
	service, _ := proxyDesktopService(t, nil)
	request := appintegration.ModeRequest{Mode: appintegration.ModeEnv, WithClaudeCode: true}
	if _, err := service.EnableIntegrationIn(context.Background(), "codex", "", request); err != nil {
		t.Fatalf("EnableIntegrationIn(codex) error = %v", err)
	}
	if view := viewOf(t, service, "claude-code"); view.Enabled {
		t.Fatal("the flag set Claude Code up for a client that is not Claude Desktop")
	}
}

// TestDesktopProxySetupPutsClaudeCodeBackWhenDesktopFails is the undo of the
// first case: Claude Code was set up only for this request, so a failed Desktop
// setup removes it again and leaves its settings as they were.
func TestDesktopProxySetupPutsClaudeCodeBackWhenDesktopFails(t *testing.T) {
	service, settings := proxyDesktopService(t, func(id string) bool { return id == "claude-desktop" })
	request := appintegration.ModeRequest{Mode: appintegration.ModeProxy, WithClaudeCode: true}
	_, err := service.EnableIntegrationIn(context.Background(), "claude-desktop", "", request)
	if !errors.Is(err, errInjected) {
		t.Fatalf("setup error = %v, want %v", err, errInjected)
	}
	if view := viewOf(t, service, "claude-code"); view.Enabled {
		t.Fatal("Claude Code stayed set up after the Desktop setup failed")
	}
	if content := settingsOf(t, settings); strings.Contains(content, "127.0.0.1:51999") {
		t.Fatalf("settings = %s, want the intercept taken back out", content)
	}
}

// TestDesktopProxySwitchPutsClaudeCodeBackWhenDesktopFails is the undo of the
// second case: Claude Code returns to the settings mode it was in.
func TestDesktopProxySwitchPutsClaudeCodeBackWhenDesktopFails(t *testing.T) {
	failing := false
	service, _ := proxyDesktopService(t, func(id string) bool { return failing && id == "claude-desktop" })
	ctx := context.Background()
	for _, id := range []string{"claude-code", "claude-desktop"} {
		if _, err := service.EnableIntegration(ctx, id, ""); err != nil {
			t.Fatalf("EnableIntegration(%s) error = %v", id, err)
		}
	}
	failing = true
	request := appintegration.ModeRequest{Mode: appintegration.ModeProxy, WithClaudeCode: true}
	if _, err := service.SetIntegrationMode(ctx, "claude-desktop", request); !errors.Is(err, errInjected) {
		t.Fatalf("switch error = %v, want %v", err, errInjected)
	}
	if view := viewOf(t, service, "claude-code"); view.Mode != appintegration.ModeEnv {
		t.Fatalf("Claude Code mode = %q, want it back in %q", view.Mode, appintegration.ModeEnv)
	}
}
