package integration_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/codingclients"
	"github.com/jonaskahn/relo/internal/application/integration"
)

// desktopProxyHarness is a proxy harness over an installed Claude Desktop. It
// answers the gateway profile's path, which is where Desktop's env mode lives.
func desktopProxyHarness(t *testing.T) (*harness, *fakeProxy, string) {
	t.Helper()
	return desktopProxyHarnessWith(t, nil)
}

func desktopProxyHarnessWith(t *testing.T, picker integration.DesktopPicker) (*harness, *fakeProxy, string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the Desktop directory is resolved from APPDATA on Windows")
	}
	runtimeProxy := &fakeProxy{env: proxyEnvFixture()}
	h := proxyHarnessFull(t, runtimeProxy, nil, picker)
	t.Setenv("CLAUDE_USER_DATA_DIR", "")
	support := filepath.Join(h.paths.Home, ".config")
	if runtime.GOOS == "darwin" {
		support = filepath.Join(h.paths.Home, "Library", "Application Support")
	}
	if err := os.MkdirAll(filepath.Join(support, "Claude"), 0o755); err != nil {
		t.Fatalf("create the Desktop directory: %v", err)
	}
	library := filepath.Join(support, "Claude-3p", "configLibrary")
	return h, runtimeProxy, codingclients.DesktopProfilePath(library)
}

func enableBoth(t *testing.T, h *harness, desktopMode string) {
	t.Helper()
	ctx := context.Background()
	if _, err := h.manager.Enable(ctx, "claude-code", "rlo_ak_code"); err != nil {
		t.Fatalf("Enable(claude-code) error = %v", err)
	}
	if _, err := h.manager.SetMode(ctx, "claude-code", integration.ModeProxy); err != nil {
		t.Fatalf("SetMode(claude-code, proxy) error = %v", err)
	}
	if _, err := h.manager.Enable(ctx, "claude-desktop", "rlo_ak_desktop"); err != nil {
		t.Fatalf("Enable(claude-desktop) error = %v", err)
	}
	if desktopMode == integration.ModeProxy {
		if _, err := h.manager.SetMode(ctx, "claude-desktop", integration.ModeProxy); err != nil {
			t.Fatalf("SetMode(claude-desktop, proxy) error = %v", err)
		}
	}
}

// TestDesktopProxyModeRidesClaudeCodesIntercept pins the dependency: Desktop's
// processes read Claude Code's settings file, so Desktop cannot be put in proxy
// mode while Claude Code is not, and the refusal names the step to take.
func TestDesktopProxyModeRidesClaudeCodesIntercept(t *testing.T) {
	h, _, _ := desktopProxyHarness(t)
	ctx := context.Background()
	if _, err := h.manager.Enable(ctx, "claude-desktop", "rlo_ak_desktop"); err != nil {
		t.Fatalf("Enable(claude-desktop) error = %v", err)
	}
	if _, err := h.manager.SetMode(ctx, "claude-desktop", integration.ModeProxy); !errors.Is(err, integration.ErrProxyOwnerNeeded) {
		t.Fatalf("SetMode(desktop, proxy) error = %v, want %v", err, integration.ErrProxyOwnerNeeded)
	}
	if _, err := h.manager.Enable(ctx, "claude-code", "rlo_ak_code"); err != nil {
		t.Fatalf("Enable(claude-code) error = %v", err)
	}
	if _, err := h.manager.SetMode(ctx, "claude-desktop", integration.ModeProxy); !errors.Is(err, integration.ErrProxyOwnerNeeded) {
		t.Fatalf("SetMode(desktop, proxy) with Claude Code in env mode error = %v, want %v", err, integration.ErrProxyOwnerNeeded)
	}
}

// TestDesktopProxyModeLeavesTheSettingsAloneAndTakesTheProfileOut pins what the
// switch does on disk: Claude Code's wiring is untouched, the gateway profile
// that made Desktop a third-party client is gone, and no model change is
// reported for a file that is not there on purpose.
func TestDesktopProxyModeLeavesTheSettingsAloneAndTakesTheProfileOut(t *testing.T) {
	h, runtimeProxy, profile := desktopProxyHarness(t)
	enableBoth(t, h, integration.ModeEnv)
	if _, err := os.Stat(profile); err != nil {
		t.Fatalf("the gateway profile is missing before the switch: %v", err)
	}
	settings := readSettings(t, h)
	retiredBefore := len(runtimeProxy.retired)

	view, err := h.manager.SetMode(context.Background(), "claude-desktop", integration.ModeProxy)
	if err != nil {
		t.Fatalf("SetMode(desktop, proxy) error = %v", err)
	}
	if view.Mode != integration.ModeProxy || view.ProxyState != integration.ProxyStateApplied {
		t.Fatalf("view = mode %q proxy state %q, want proxy applied through Claude Code's wiring", view.Mode, view.ProxyState)
	}
	if _, err := os.Stat(profile); !os.IsNotExist(err) {
		t.Fatalf("stat the gateway profile error = %v, want it removed", err)
	}
	if after := readSettings(t, h); after != settings {
		t.Fatalf("Claude Code's settings changed by Desktop's switch:\nbefore %s\nafter %s", settings, after)
	}
	if len(runtimeProxy.retired) != retiredBefore {
		t.Fatal("Desktop's switch stopped the intercept Claude Code uses")
	}
	if view.ModelsStale || len(view.Reasons) != 0 {
		t.Fatalf("view = stale %v reasons %+v, want nothing to repair", view.ModelsStale, view.Reasons)
	}
	for _, file := range view.Files {
		if file.Kind == integration.KindClaudeDesktop {
			t.Fatalf("files = %+v, want no gateway profile recorded in proxy mode", view.Files)
		}
	}
}

// TestDesktopSwitchesBackToItsGatewayProfile keeps the two modes reversible
// and the key Desktop presented: the profile returns with Desktop's own key.
func TestDesktopSwitchesBackToItsGatewayProfile(t *testing.T) {
	h, _, profile := desktopProxyHarness(t)
	enableBoth(t, h, integration.ModeProxy)
	view, err := h.manager.SetMode(context.Background(), "claude-desktop", integration.ModeEnv)
	if err != nil {
		t.Fatalf("SetMode(desktop, env) error = %v", err)
	}
	if view.Mode != integration.ModeEnv {
		t.Fatalf("mode = %q, want %q", view.Mode, integration.ModeEnv)
	}
	if content := readFile(t, profile); !strings.Contains(content, "rlo_ak_desktop") {
		t.Fatalf("profile = %s, want Desktop's own key back in it", content)
	}
}

// TestClaudeCodeKeepsItsProxyWhileDesktopUsesIt pins the other direction: the
// settings file is Claude Code's, but taking it away would strand Desktop, so
// every way out is refused until Desktop has left.
func TestClaudeCodeKeepsItsProxyWhileDesktopUsesIt(t *testing.T) {
	h, runtimeProxy, _ := desktopProxyHarness(t)
	enableBoth(t, h, integration.ModeProxy)
	ctx := context.Background()
	if _, err := h.manager.SetMode(ctx, "claude-code", integration.ModeEnv); !errors.Is(err, integration.ErrProxyInUse) {
		t.Fatalf("SetMode(code, env) error = %v, want %v", err, integration.ErrProxyInUse)
	}
	if _, err := h.manager.Disable(ctx, "claude-code"); !errors.Is(err, integration.ErrProxyInUse) {
		t.Fatalf("Disable(code) error = %v, want %v", err, integration.ErrProxyInUse)
	}
	if _, err := h.manager.Restore(ctx, "claude-code", false); !errors.Is(err, integration.ErrProxyInUse) {
		t.Fatalf("Restore(code) error = %v, want %v", err, integration.ErrProxyInUse)
	}
	if len(runtimeProxy.retired) != 0 {
		t.Fatalf("retired = %v, want the intercept left running", runtimeProxy.retired)
	}
	if _, err := h.manager.SetMode(ctx, "claude-desktop", integration.ModeEnv); err != nil {
		t.Fatalf("SetMode(desktop, env) error = %v", err)
	}
	if _, err := h.manager.SetMode(ctx, "claude-code", integration.ModeEnv); err != nil {
		t.Fatalf("SetMode(code, env) after Desktop left error = %v", err)
	}
}

// TestRestoringDesktopLeavesClaudeCodesProxyWiringInPlace keeps the shared
// settings file Claude Code's: removing Desktop must not strip its address.
func TestRestoringDesktopLeavesClaudeCodesProxyWiringInPlace(t *testing.T) {
	h, runtimeProxy, _ := desktopProxyHarness(t)
	enableBoth(t, h, integration.ModeProxy)
	settings := readSettings(t, h)
	if _, err := h.manager.Restore(context.Background(), "claude-desktop", false); err != nil {
		t.Fatalf("Restore(desktop) error = %v", err)
	}
	if after := readSettings(t, h); after != settings {
		t.Fatalf("restoring Desktop changed Claude Code's settings:\nbefore %s\nafter %s", settings, after)
	}
	if len(runtimeProxy.retired) != 0 {
		t.Fatalf("retired = %v, want the intercept left running", runtimeProxy.retired)
	}
}

// onPicker is a Desktop picker that is on, which is all the manager asks.
type onPicker struct{ enabled bool }

func (p *onPicker) View(context.Context) integration.PickerView {
	return integration.PickerView{Enabled: p.enabled}
}
func (p *onPicker) Enable(context.Context) (integration.PickerView, error) {
	return integration.PickerView{}, nil
}
func (p *onPicker) Disable(context.Context) (integration.PickerView, error) {
	return integration.PickerView{}, nil
}
func (p *onPicker) RemoveTrust(context.Context) (integration.PickerView, error) {
	return integration.PickerView{}, nil
}
func (p *onPicker) EnableCowork(context.Context) (integration.PickerView, error) {
	return integration.PickerView{}, nil
}
func (p *onPicker) DisableCowork(context.Context) (integration.PickerView, error) {
	return integration.PickerView{}, nil
}

// TestDesktopStaysInProxyModeWhileItsPickerIsOn keeps the picker from being
// stranded: it rides Desktop's wiring, so every way out is refused until it is off.
func TestDesktopStaysInProxyModeWhileItsPickerIsOn(t *testing.T) {
	picker := &onPicker{enabled: true}
	h, _, _ := desktopProxyHarnessWith(t, picker)
	enableBoth(t, h, integration.ModeProxy)
	ctx := context.Background()
	if _, err := h.manager.SetMode(ctx, "claude-desktop", integration.ModeEnv); !errors.Is(err, integration.ErrPickerInUse) {
		t.Fatalf("SetMode(desktop, env) error = %v, want %v", err, integration.ErrPickerInUse)
	}
	if _, err := h.manager.Restore(ctx, "claude-desktop", false); !errors.Is(err, integration.ErrPickerInUse) {
		t.Fatalf("Restore(desktop) error = %v, want %v", err, integration.ErrPickerInUse)
	}
	if _, err := h.manager.Disable(ctx, "claude-desktop"); !errors.Is(err, integration.ErrPickerInUse) {
		t.Fatalf("Disable(desktop) error = %v, want %v", err, integration.ErrPickerInUse)
	}
	picker.enabled = false
	if _, err := h.manager.SetMode(ctx, "claude-desktop", integration.ModeEnv); err != nil {
		t.Fatalf("SetMode(desktop, env) with the picker off error = %v", err)
	}
}

// TestSetUpInProxyModeWritesTheModeTheOperatorChose pins the set-up path the
// modal now offers: the mode chosen before anything is written is the mode the
// first set-up writes, for both clients, without a switch afterwards.
func TestSetUpInProxyModeWritesTheModeTheOperatorChose(t *testing.T) {
	h, runtimeProxy, profile := desktopProxyHarness(t)
	ctx := context.Background()
	code, err := h.manager.EnableWith(ctx, "claude-code", "rlo_ak_code", integration.EnableOptions{Mode: integration.ModeProxy})
	if err != nil || code.Mode != integration.ModeProxy || code.ProxyState != integration.ProxyStateApplied {
		t.Fatalf("set up claude-code in proxy mode = %+v %v, want proxy applied", code, err)
	}
	if settings := readSettings(t, h); strings.Contains(settings, "ANTHROPIC_BASE_URL") || !strings.Contains(settings, runtimeProxy.env.URL) {
		t.Fatalf("settings = %s, want the intercept and no settings-mode keys", settings)
	}
	desktop, err := h.manager.EnableWith(ctx, "claude-desktop", "rlo_ak_desktop", integration.EnableOptions{Mode: integration.ModeProxy})
	if err != nil || desktop.Mode != integration.ModeProxy {
		t.Fatalf("set up claude-desktop in proxy mode = %+v %v", desktop, err)
	}
	if _, err := os.Stat(profile); !os.IsNotExist(err) {
		t.Fatalf("stat the gateway profile error = %v, want none written in proxy mode", err)
	}
}

// TestSetUpDesktopInProxyModeNeedsClaudeCodeThere keeps the first set-up honest:
// choosing proxy for Desktop while Claude Code is not in it is refused and
// writes nothing.
func TestSetUpDesktopInProxyModeNeedsClaudeCodeThere(t *testing.T) {
	h, _, profile := desktopProxyHarness(t)
	_, err := h.manager.EnableWith(context.Background(), "claude-desktop", "rlo_ak_desktop", integration.EnableOptions{Mode: integration.ModeProxy})
	if !errors.Is(err, integration.ErrProxyOwnerNeeded) {
		t.Fatalf("error = %v, want %v", err, integration.ErrProxyOwnerNeeded)
	}
	if _, statErr := os.Stat(profile); !os.IsNotExist(statErr) {
		t.Fatalf("stat the gateway profile error = %v, want nothing written", statErr)
	}
}
