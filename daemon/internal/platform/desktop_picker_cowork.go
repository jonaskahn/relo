package platform

import (
	"context"
	"errors"
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

// systemRoot is Relo's root as the system-wide store sees it, which is where a
// virtual machine's copy of the host's trust is read from.
func (p *DesktopPicker) systemRoot(authority *proxy.Authority) (ostrust.Root, error) {
	return p.rootFor(authority, proxy.HostAnthropicAPI, ostrust.ScopeSystem)
}

// coworkTrusted is whether the OS trusts Relo's root system-wide for the vendor
// API host.
func (p *DesktopPicker) coworkTrusted(ctx context.Context, authority *proxy.Authority) bool {
	root, err := p.systemRoot(authority)
	return err == nil && p.opts.Trust.Inspect(ctx, root) == ostrust.Trusted
}

// describeCowork adds what the OS says about the system-wide trust, and how to
// install or remove it by hand, to the view.
func (p *DesktopPicker) describeCowork(ctx context.Context, view *appintegration.PickerView, authority *proxy.Authority) {
	root, err := p.systemRoot(authority)
	if err != nil {
		return
	}
	state := p.opts.Trust.Inspect(ctx, root)
	view.Cowork.Trust = string(state)
	if !view.Cowork.Enabled && state == ostrust.Trusted {
		view.Cowork.Reason = appintegration.PickerCoworkTrustRemains
	}
	steps := ostrust.Manual(p.opts.Platform, root, authority.CAPathFile())
	view.Cowork.Manual = &appintegration.PickerManual{
		OS: steps.OS, CertPath: steps.CertPath, SHA1: steps.SHA1, SHA256: steps.SHA256,
		DocsURL: steps.DocsURL, Install: steps.Install, Remove: steps.Remove, Check: steps.Check,
	}
}

// ensureSystemTrust leaves Relo's root trusted system-wide, asking the OS only
// when it is not. The ask is the OS's own approval dialog, so a decline leaves
// nothing changed; a machine with no way to ask leaves the install to the
// operator.
func (p *DesktopPicker) ensureSystemTrust(ctx context.Context, authority *proxy.Authority) error {
	root, err := p.systemRoot(authority)
	if err != nil {
		return fail(appintegration.PickerCAUnverified, err)
	}
	if p.opts.Trust.Inspect(ctx, root) == ostrust.Trusted {
		return nil
	}
	if err := p.opts.Trust.Trust(ctx, root); err != nil {
		if errors.Is(err, ostrust.ErrUnavailable) || errors.Is(err, ostrust.ErrUnsupported) {
			return fail(appintegration.PickerCoworkTrustUnavailable, err)
		}
		return fail(appintegration.PickerCoworkTrustDeclined, err)
	}
	if p.opts.Trust.Inspect(ctx, root) != ostrust.Trusted {
		_ = p.opts.Trust.Untrust(ctx, root)
		return fail(appintegration.PickerCoworkTrustMissing, nil)
	}
	return nil
}

// armCowork recomputes whether the vendor API is handed on: only while the
// operator turned it on, the picker is armed, and the OS trusts the root for
// that host too. Losing any of them splices the host through again.
func (p *DesktopPicker) armCowork(ctx context.Context, authority *proxy.Authority) {
	p.cowork.Store(p.coworkEnabled() && p.armed.Load() && p.coworkTrusted(ctx, authority))
}

// EnableCowork turns Cowork routing on. It refuses unless the picker is already
// armed; it then asks the OS to trust Relo's root system-wide if it does not
// already, which is the one place that is asked, and only on this explicit
// request. A failed ask leaves nothing changed.
func (p *DesktopPicker) EnableCowork(ctx context.Context) (appintegration.PickerView, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	authority, err := p.coworkPreconditions(ctx)
	if err != nil {
		return p.coworkResult(ctx, reasonOf(err)), err
	}
	if err := p.ensureSystemTrust(ctx, authority); err != nil {
		return p.coworkResult(ctx, reasonOf(err)), err
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

// coworkPreconditions answers the authority when Cowork routing may be turned on.
func (p *DesktopPicker) coworkPreconditions(ctx context.Context) (*proxy.Authority, error) {
	switch {
	case !p.enabled() || p.pair == nil || !p.armed.Load():
		return nil, fail(appintegration.PickerCoworkNeedsPicker, nil)
	case !p.opts.DesktopWired(ctx):
		return nil, fail(appintegration.PickerNeedsDesktopProxy, nil)
	case p.opts.Intercept == nil || !p.interceptUp():
		return nil, fail(appintegration.PickerProxyUnavailable, nil)
	}
	authority, err := p.opts.Authority()
	if err != nil || !authority.Constrained() {
		return nil, fail(appintegration.PickerCAUnverified, err)
	}
	return authority, nil
}

func (p *DesktopPicker) interceptUp() bool {
	_, _, ok := p.opts.Intercept()
	return ok
}

// DisableCowork stops handing the vendor API on, before anything else, so the
// next connection is spliced through. The system-wide trust stays until the
// operator removes it, or Desktop leaves proxy mode or is removed.
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

// RemoveSystemTrust takes Relo's root out of the system-wide trust. It refuses
// while Cowork routing is on, because that cannot run without it.
func (p *DesktopPicker) RemoveSystemTrust(ctx context.Context) (appintegration.PickerView, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.coworkEnabled() {
		return p.coworkResult(ctx, appintegration.PickerCoworkActive), fail(appintegration.PickerCoworkActive, nil)
	}
	if err := p.untrustSystem(ctx); err != nil {
		return p.coworkResult(ctx, appintegration.PickerCoworkTrustRemains), fail(appintegration.PickerCoworkTrustRemains, err)
	}
	return p.view(ctx, ""), nil
}

func (p *DesktopPicker) untrustSystem(ctx context.Context) error {
	if !p.supported() {
		return nil
	}
	authority, err := p.opts.Authority()
	if err != nil {
		return err
	}
	root, err := p.systemRoot(authority)
	if err != nil {
		return err
	}
	return p.opts.Trust.Untrust(ctx, root)
}

// Retire takes back everything the picker and Cowork routing added and every
// certificate Relo asked the OS to trust, in the order that keeps Desktop's
// network: routing, then the profile and proxy, then the user's trust, then the
// system-wide trust. It stops at the first step that fails and says which, and
// does nothing for a step with nothing to take back.
func (p *DesktopPicker) Retire(ctx context.Context) (appintegration.PickerView, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.enabled() || p.coworkEnabled() {
		if err := p.turnOff(); err != nil {
			return p.view(ctx, appintegration.PickerCleanupIncomplete), fail(appintegration.PickerCleanupIncomplete, err)
		}
	}
	if err := p.untrust(ctx); err != nil {
		return p.view(ctx, appintegration.PickerTrustRemains), fail(appintegration.PickerTrustRemains, err)
	}
	if err := p.untrustSystem(ctx); err != nil {
		return p.coworkResult(ctx, appintegration.PickerCoworkTrustRemains), fail(appintegration.PickerCoworkTrustRemains, err)
	}
	return p.view(ctx, ""), nil
}
