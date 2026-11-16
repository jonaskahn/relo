// Claude Desktop's picker, run. It owns the egress proxy, the OS's trust of
// Relo's root and the profile that points Desktop at the proxy, and it keeps
// them in an order that leaves Desktop's network working at every step.
package platform

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jonaskahn/relo/internal/adapters/codingclients"
	"github.com/jonaskahn/relo/internal/adapters/ostrust"
	"github.com/jonaskahn/relo/internal/adapters/proxy"
	appintegration "github.com/jonaskahn/relo/internal/application/integration"
)

var _ appintegration.DesktopPicker = (*DesktopPicker)(nil)

// trustRecheck is how often the OS's trust is read again while the picker is on,
// so a root the operator removes by hand disarms it rather than breaking claude.ai.
const trustRecheck = 30 * time.Second

// DesktopPickerOptions configure the picker's runtime.
type DesktopPickerOptions struct {
	// Home is Relo's state directory.
	Home string
	// Platform is the OS the picker runs on.
	Platform string
	// Library is Desktop's config library.
	Library func() string
	// Installed reports whether Desktop has run on this machine.
	Installed func() error
	// Authority answers Relo's one root.
	Authority func() (*proxy.Authority, error)
	// Trust reads and changes the OS's trust of that root.
	Trust ostrust.Store
	// Catalog answers the models added to Desktop's picker.
	Catalog func() []proxy.CatalogRow
	// DesktopWired reports whether Desktop is on and in proxy mode, which is what
	// the picker rides.
	DesktopWired func(ctx context.Context) bool
	Logger       *slog.Logger
	// Upstream, Transport and Dial are for a test; the defaults reach claude.ai.
	Upstream  string
	Transport http.RoundTripper
	Dial      func(ctx context.Context, network, address string) (net.Conn, error)
}

// DesktopPicker is the picker's runtime.
type DesktopPicker struct {
	opts  DesktopPickerOptions
	armed atomic.Bool

	mu     sync.Mutex
	pair   *proxy.PickerProxy
	cancel context.CancelFunc
}

// NewDesktopPicker returns the picker's runtime. Nothing starts until Run, or
// until the operator turns the picker on.
func NewDesktopPicker(options DesktopPickerOptions) *DesktopPicker {
	if options.Logger == nil {
		options.Logger = slog.New(slog.DiscardHandler)
	}
	if options.Upstream == "" {
		options.Upstream = "https://" + proxy.HostClaudeAI
	}
	return &DesktopPicker{opts: options}
}

func (p *DesktopPicker) stateDir() string { return filepath.Join(p.opts.Home, "desktop-picker") }

func (p *DesktopPicker) markerPath() string { return filepath.Join(p.stateDir(), "enabled") }

func (p *DesktopPicker) addressPath() string { return filepath.Join(p.stateDir(), "address") }

func (p *DesktopPicker) enabled() bool {
	_, err := os.Stat(p.markerPath())
	return err == nil
}

func (p *DesktopPicker) supported() bool {
	return p.opts.Platform == "darwin" || p.opts.Platform == "windows"
}

// root describes Relo's root to the OS, with a leaf that proves the trust.
func (p *DesktopPicker) root(authority *proxy.Authority) (ostrust.Root, error) {
	leaf, err := authority.LeafPEM(proxy.HostClaudeAI)
	if err != nil {
		return ostrust.Root{}, err
	}
	return ostrust.Root{
		PEM: authority.PEM(), SHA1: authority.SHA1(), CommonName: proxy.CommonName,
		Host: proxy.HostClaudeAI, LeafPEM: leaf,
	}, nil
}

// View reports where the picker stands. It reads the OS and the library and
// changes neither.
func (p *DesktopPicker) View(ctx context.Context) appintegration.PickerView {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.view(ctx, "")
}

func (p *DesktopPicker) view(ctx context.Context, reason appintegration.PickerReason) appintegration.PickerView {
	view := appintegration.PickerView{
		Supported: p.supported(), Enabled: p.enabled(), Armed: p.armed.Load(),
		Trust: string(ostrust.Unknown), Profile: string(codingclients.DesktopPickerAbsent),
	}
	if p.pair != nil {
		view.Address = p.pair.Addr()
		view.Connections = pickerConnections(p.pair.Connections())
	}
	if !view.Supported {
		view.Trust = string(ostrust.Unsupported)
		view.Reason = appintegration.PickerUnsupportedOS
		return view
	}
	status, _ := codingclients.InspectDesktopPickerProfile(p.opts.Library())
	view.Profile = string(status)
	if authority, err := p.opts.Authority(); err == nil {
		view.Fingerprint = authority.SHA256()
		if root, err := p.root(authority); err == nil {
			view.Trust = string(p.opts.Trust.Inspect(ctx, root))
		}
	}
	view.Reason = p.reasonFor(view, reason)
	return view
}

func pickerConnections(seen proxy.Connections) appintegration.PickerConnections {
	out := appintegration.PickerConnections{RefusedPeers: seen.RefusedPeers}
	for _, host := range seen.Hosts {
		out.Hosts = append(out.Hosts, appintegration.PickerHost{
			Host: host.Host, Count: host.Count, LastSeenMs: host.LastUnixMs,
		})
	}
	return out
}

func (p *DesktopPicker) reasonFor(view appintegration.PickerView, given appintegration.PickerReason) appintegration.PickerReason {
	switch {
	case given != "":
		return given
	case !view.Enabled && view.Trust == string(ostrust.Trusted):
		return appintegration.PickerTrustRemains
	case !view.Enabled:
		return appintegration.PickerOff
	case view.Trust != string(ostrust.Trusted):
		return appintegration.PickerTrustPending
	}
	return appintegration.PickerActive
}

func fail(reason appintegration.PickerReason, cause error) error {
	return &appintegration.PickerError{Reason: reason, Cause: cause}
}

// Enable turns the picker on, in the order that never leaves Desktop worse off
// than it was: the proxy is up and not yet terminating, then the OS trusts the
// root and is read back, then Desktop is pointed at the proxy, and only then is
// claude.ai terminated. A step that fails undoes the ones before it.
func (p *DesktopPicker) Enable(ctx context.Context) (appintegration.PickerView, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.setUp(ctx); err != nil {
		return p.view(ctx, reasonOf(err)), err
	}
	p.armed.Store(true)
	return p.view(ctx, appintegration.PickerRestartRequired), nil
}

// setUp runs the steps of Enable and undoes the ones already done when a later
// one fails.
func (p *DesktopPicker) setUp(ctx context.Context) error {
	if err := p.preconditions(ctx); err != nil {
		return err
	}
	authority, root, err := p.usableRoot()
	if err != nil {
		return err
	}
	if err := p.start(ctx, authority); err != nil {
		return fail(appintegration.PickerProxyUnavailable, err)
	}
	addedTrust, err := p.ensureTrust(ctx, root)
	if err != nil {
		p.stop()
		return err
	}
	if err := p.pointDesktop(); err != nil {
		p.stop()
		if addedTrust {
			_ = p.opts.Trust.Untrust(ctx, root)
		}
		return err
	}
	if err := p.markOn(); err != nil {
		p.undoProfile()
		p.stop()
		return fail(appintegration.PickerProfileFailed, err)
	}
	return nil
}

// usableRoot is Relo's root and what the OS is told about it, refused unless it
// still carries the constraint it was minted with.
func (p *DesktopPicker) usableRoot() (*proxy.Authority, ostrust.Root, error) {
	authority, err := p.opts.Authority()
	if err != nil || !authority.Constrained() {
		return nil, ostrust.Root{}, fail(appintegration.PickerCAUnverified, err)
	}
	root, err := p.root(authority)
	if err != nil {
		return nil, ostrust.Root{}, fail(appintegration.PickerCAUnverified, err)
	}
	return authority, root, nil
}

func (p *DesktopPicker) markOn() error {
	if err := os.MkdirAll(p.stateDir(), 0o700); err != nil {
		return err
	}
	return os.WriteFile(p.markerPath(), []byte("on\n"), 0o600)
}

func reasonOf(err error) appintegration.PickerReason {
	var refusal *appintegration.PickerError
	if errors.As(err, &refusal) {
		return refusal.Reason
	}
	return appintegration.PickerProfileFailed
}

func (p *DesktopPicker) preconditions(ctx context.Context) error {
	switch {
	case !p.supported():
		return fail(appintegration.PickerUnsupportedOS, nil)
	case p.opts.Installed() != nil:
		return fail(appintegration.PickerNotInstalled, nil)
	case !p.opts.DesktopWired(ctx):
		return fail(appintegration.PickerNeedsDesktopProxy, nil)
	}
	return nil
}

// ensureTrust leaves the root trusted, asking the OS only when it is not. The
// ask is the OS's own prompt, so a decline leaves nothing changed.
func (p *DesktopPicker) ensureTrust(ctx context.Context, root ostrust.Root) (added bool, err error) {
	if p.opts.Trust.Inspect(ctx, root) == ostrust.Trusted {
		return false, nil
	}
	if err := p.opts.Trust.Trust(ctx, root); err != nil {
		return false, fail(appintegration.PickerTrustDeclined, err)
	}
	if p.opts.Trust.Inspect(ctx, root) != ostrust.Trusted {
		_ = p.opts.Trust.Untrust(ctx, root)
		return false, fail(appintegration.PickerTrustPending, nil)
	}
	return true, nil
}

func (p *DesktopPicker) pointDesktop() error {
	port, err := portOf(p.pair.Addr())
	if err != nil {
		return fail(appintegration.PickerProxyUnavailable, err)
	}
	err = codingclients.ApplyDesktopPickerProfile(p.opts.Library(), p.stateDir(), port)
	switch {
	case errors.Is(err, codingclients.ErrDesktopGatewaySelected):
		return fail(appintegration.PickerGatewaySelected, err)
	case err != nil:
		return fail(appintegration.PickerProfileFailed, err)
	}
	return nil
}

func (p *DesktopPicker) undoProfile() {
	if err := codingclients.RemoveDesktopPickerProfile(p.opts.Library(), p.stateDir()); err != nil {
		p.opts.Logger.Warn("remove the Desktop picker profile", "error", err)
	}
}

func portOf(address string) (int, error) {
	_, port, err := net.SplitHostPort(address)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(port)
}

// start brings the egress proxy up, on the address it had last time when that is
// free, so the port Desktop's profile names survives a restart. It does not
// terminate claude.ai until the picker is armed.
func (p *DesktopPicker) start(ctx context.Context, authority *proxy.Authority) error {
	if p.pair != nil {
		return nil
	}
	pair, err := proxy.NewPicker(proxy.PickerOptions{
		Upstream: p.opts.Upstream, Authority: authority, Catalog: p.opts.Catalog,
		Armed: p.armed.Load, PreferredAddr: p.lastAddress(), Logger: p.opts.Logger,
		Dial: p.opts.Dial, Transport: p.opts.Transport,
	})
	if err != nil {
		return err
	}
	runCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	if err := pair.Start(runCtx); err != nil {
		cancel()
		return err
	}
	p.pair, p.cancel = pair, cancel
	p.rememberAddress(pair.Addr())
	return nil
}

func (p *DesktopPicker) lastAddress() string {
	content, err := os.ReadFile(p.addressPath())
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(content))
}

func (p *DesktopPicker) rememberAddress(address string) {
	if p.lastAddress() == address {
		return
	}
	if err := os.MkdirAll(p.stateDir(), 0o700); err == nil {
		_ = os.WriteFile(p.addressPath(), []byte(address+"\n"), 0o600)
	}
}

// stop ends the egress proxy and disarms it first, so a connection that arrives
// while it closes is spliced rather than answered with a certificate.
func (p *DesktopPicker) stop() {
	p.armed.Store(false)
	pair, cancel := p.pair, p.cancel
	p.pair, p.cancel = nil, nil
	if cancel != nil {
		cancel()
	}
	if pair != nil {
		pair.Wait()
	}
}

// Disable turns the picker off and takes back what it added, in the order that
// keeps Desktop's network: stop terminating, point Desktop back, stop the proxy,
// then take the trust out. A step that fails stops there and says which, with
// the proxy left running while Desktop still names it.
func (p *DesktopPicker) Disable(ctx context.Context) (appintegration.PickerView, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.armed.Store(false)
	if err := codingclients.RemoveDesktopPickerProfile(p.opts.Library(), p.stateDir()); err != nil {
		return p.view(ctx, appintegration.PickerCleanupIncomplete), fail(appintegration.PickerCleanupIncomplete, err)
	}
	p.stop()
	if err := os.Remove(p.markerPath()); err != nil && !os.IsNotExist(err) {
		return p.view(ctx, appintegration.PickerCleanupIncomplete), fail(appintegration.PickerCleanupIncomplete, err)
	}
	if err := p.untrust(ctx); err != nil {
		return p.view(ctx, appintegration.PickerTrustRemains), fail(appintegration.PickerTrustRemains, err)
	}
	return p.view(ctx, ""), nil
}

// RemoveTrust takes Relo's root out of the OS trust store. It refuses while the
// picker is on, because the picker cannot run without it.
func (p *DesktopPicker) RemoveTrust(ctx context.Context) (appintegration.PickerView, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.enabled() {
		return p.view(ctx, ""), fail(appintegration.PickerActive, nil)
	}
	if err := p.untrust(ctx); err != nil {
		return p.view(ctx, appintegration.PickerTrustRemains), fail(appintegration.PickerTrustRemains, err)
	}
	return p.view(ctx, ""), nil
}

func (p *DesktopPicker) untrust(ctx context.Context) error {
	if !p.supported() {
		return nil
	}
	authority, err := p.opts.Authority()
	if err != nil {
		return err
	}
	root, err := p.root(authority)
	if err != nil {
		return err
	}
	return p.opts.Trust.Untrust(ctx, root)
}

// Run restores a picker the operator turned on before the daemon stopped, and
// keeps it honest until ctx ends. It never asks the OS to trust anything: a root
// that is no longer trusted only leaves the proxy splicing, and the operator is
// told. The profile is untouched on a stop, so Desktop needs Relo running.
func (p *DesktopPicker) Run(ctx context.Context) {
	p.restore(ctx)
	ticker := time.NewTicker(trustRecheck)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			p.mu.Lock()
			p.stop()
			p.mu.Unlock()
			return
		case <-ticker.C:
			p.recheck(ctx)
		}
	}
}

func (p *DesktopPicker) restore(ctx context.Context) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.enabled() || !p.supported() {
		return
	}
	authority, err := p.opts.Authority()
	if err != nil || !authority.Constrained() {
		p.opts.Logger.Warn("the Desktop picker is on but Relo's root is not usable", "error", err)
		return
	}
	if err := p.start(ctx, authority); err != nil {
		p.opts.Logger.Warn("start the Desktop picker proxy", "error", err)
		return
	}
	if err := p.pointDesktop(); err != nil {
		p.opts.Logger.Warn("point Desktop at the picker proxy", "error", err)
	}
	p.armed.Store(p.trusted(ctx, authority))
}

func (p *DesktopPicker) recheck(ctx context.Context) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.pair == nil {
		return
	}
	authority, err := p.opts.Authority()
	p.armed.Store(err == nil && p.trusted(ctx, authority))
}

func (p *DesktopPicker) trusted(ctx context.Context, authority *proxy.Authority) bool {
	root, err := p.root(authority)
	return err == nil && p.opts.Trust.Inspect(ctx, root) == ostrust.Trusted
}
