package platform

import (
	"context"
	"os"
	"path/filepath"

	"github.com/jonaskahn/relo/internal/adapters/ostrust"
	"github.com/jonaskahn/relo/internal/adapters/proxy"
	appintegration "github.com/jonaskahn/relo/internal/application/integration"
)

// Cowork routing hands the connections Claude Desktop's Cowork virtual machine
// makes for api.anthropic.com to Relo's intercept, through the picker's egress
// proxy. It is its own switch, off by default, because it makes Relo terminate
// TLS for the vendor API, which the picker alone never does.

func (p *DesktopPicker) coworkMarkerPath() string { return filepath.Join(p.stateDir(), "cowork") }

func (p *DesktopPicker) coworkEnabled() bool {
	_, err := os.Stat(p.coworkMarkerPath())
	return err == nil
}

// coworkView is where Cowork routing stands, in the words the console reads.
func (p *DesktopPicker) coworkView() appintegration.PickerCowork {
	view := appintegration.PickerCowork{Enabled: p.coworkEnabled(), Armed: p.cowork.Load()}
	switch {
	case !view.Enabled:
		view.Reason = appintegration.PickerOff
	case view.Armed:
		view.Reason = appintegration.PickerCoworkActive
	case !p.armed.Load():
		view.Reason = appintegration.PickerCoworkNeedsPicker
	default:
		view.Reason = appintegration.PickerCoworkTrustMissing
	}
	return view
}

// coworkTrusted is whether the OS trusts Relo's root for the vendor API host.
func (p *DesktopPicker) coworkTrusted(ctx context.Context, authority *proxy.Authority) bool {
	root, err := p.rootFor(authority, proxy.HostAnthropicAPI)
	return err == nil && p.opts.Trust.Inspect(ctx, root) == ostrust.Trusted
}

// armCowork recomputes whether the vendor API is handed on: only while the
// operator turned it on, the picker is armed, and the OS trusts the root for
// that host too. Losing any of them splices the host through again.
func (p *DesktopPicker) armCowork(ctx context.Context, authority *proxy.Authority) {
	p.cowork.Store(p.coworkEnabled() && p.armed.Load() && p.coworkTrusted(ctx, authority))
}

// EnableCowork turns Cowork routing on. It asks the OS for nothing: it refuses
// unless the picker is already armed and the root is already trusted for the
// vendor API host.
func (p *DesktopPicker) EnableCowork(ctx context.Context) (appintegration.PickerView, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.coworkPreconditions(ctx); err != nil {
		return p.view(ctx, ""), err
	}
	if err := os.MkdirAll(p.stateDir(), 0o700); err != nil {
		return p.view(ctx, ""), fail(appintegration.PickerProfileFailed, err)
	}
	if err := os.WriteFile(p.coworkMarkerPath(), []byte("on\n"), 0o600); err != nil {
		return p.view(ctx, ""), fail(appintegration.PickerProfileFailed, err)
	}
	p.cowork.Store(true)
	return p.coworkResult(ctx, appintegration.PickerCoworkRestart), nil
}

func (p *DesktopPicker) coworkPreconditions(ctx context.Context) error {
	switch {
	case !p.enabled() || p.pair == nil || !p.armed.Load():
		return fail(appintegration.PickerCoworkNeedsPicker, nil)
	case !p.opts.DesktopWired(ctx):
		return fail(appintegration.PickerNeedsDesktopProxy, nil)
	case p.opts.Intercept == nil || !p.interceptUp():
		return fail(appintegration.PickerProxyUnavailable, nil)
	}
	authority, err := p.opts.Authority()
	if err != nil || !authority.Constrained() {
		return fail(appintegration.PickerCAUnverified, err)
	}
	if !p.coworkTrusted(ctx, authority) {
		return fail(appintegration.PickerCoworkTrustMissing, nil)
	}
	return nil
}

func (p *DesktopPicker) interceptUp() bool {
	_, _, ok := p.opts.Intercept()
	return ok
}

// DisableCowork stops handing the vendor API on, before anything else, so the
// next connection is spliced through.
func (p *DesktopPicker) DisableCowork(ctx context.Context) (appintegration.PickerView, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.dropCowork(); err != nil {
		return p.view(ctx, ""), fail(appintegration.PickerCleanupIncomplete, err)
	}
	return p.view(ctx, ""), nil
}

// dropCowork disarms Cowork routing and forgets that it was turned on.
func (p *DesktopPicker) dropCowork() error {
	p.cowork.Store(false)
	if err := os.Remove(p.coworkMarkerPath()); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// coworkResult is the picker as it stands, with the reason Cowork routing
// reports for the action just taken.
func (p *DesktopPicker) coworkResult(ctx context.Context, reason appintegration.PickerReason) appintegration.PickerView {
	view := p.view(ctx, "")
	view.Cowork.Reason = reason
	return view
}
