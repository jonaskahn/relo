// Claude Desktop's own model picker. It is opt-in and off by default, because it
// is the one piece of Relo that asks the operating system to trust a certificate
// and terminates TLS for a host the operator signs in to.
package integration

import (
	"context"
	"errors"
	"fmt"
)

// PickerReason is why the picker stands where it does, in the codes a console
// translates.
type PickerReason string

// The reasons a picker reports.
const (
	PickerActive            PickerReason = "active"
	PickerOff               PickerReason = "off"
	PickerRestartRequired   PickerReason = "restart_required"
	PickerUnsupportedOS     PickerReason = "unsupported_platform"
	PickerNotInstalled      PickerReason = "not_installed"
	PickerNeedsDesktopProxy PickerReason = "needs_desktop_proxy"
	PickerTrustPending      PickerReason = "trust_pending"
	PickerTrustDeclined     PickerReason = "trust_declined"
	PickerTrustRemains      PickerReason = "trust_remains"
	PickerProfileFailed     PickerReason = "profile_failed"
	PickerGatewaySelected   PickerReason = "gateway_selected"
	PickerCAUnverified      PickerReason = "ca_unverified"
	PickerProxyUnavailable  PickerReason = "proxy_unavailable"
	PickerCleanupIncomplete PickerReason = "cleanup_incomplete"
	// The reasons Cowork routing reports, beside the picker's own.
	PickerCoworkNeedsPicker  PickerReason = "cowork_needs_picker"
	PickerCoworkTrustMissing PickerReason = "cowork_trust_missing"
	PickerCoworkRestart      PickerReason = "cowork_restart_required"
	PickerCoworkActive       PickerReason = "cowork_active"
)

// PickerHost is how many connections one host has asked the picker's egress
// proxy for. Only the hosts Relo intercepts are named; every other is counted
// together as "other".
type PickerHost struct {
	Host       string
	Count      int
	LastSeenMs int64
}

// PickerConnections is what the egress proxy has been asked since it started.
type PickerConnections struct {
	Hosts []PickerHost
	// RefusedPeers counts connections from an address that is not loopback.
	RefusedPeers int
}

// PickerCowork is where Cowork routing stands: whether Claude Desktop's Cowork
// virtual machine has its api.anthropic.com traffic handed to Relo's intercept.
type PickerCowork struct {
	// Enabled is whether the operator turned it on.
	Enabled bool
	// Armed is whether traffic is being handed on right now, which needs the
	// picker armed and the OS to trust Relo's root for the vendor API host too.
	Armed  bool
	Reason PickerReason
}

// PickerView is where the picker stands.
type PickerView struct {
	// Supported is whether this OS can trust Relo's root at all.
	Supported bool
	// Enabled is whether the operator turned the picker on.
	Enabled bool
	// Armed is whether claude.ai is being terminated right now, which needs the
	// OS to trust the root.
	Armed bool
	// Trust is the OS's trust of Relo's root: trusted, untrusted, unknown or
	// unsupported.
	Trust string
	// Profile is how Desktop's config library stands: absent, applied,
	// not_selected or unsafe.
	Profile string
	// Fingerprint is the root's SHA-256, for the operator to compare.
	Fingerprint string
	// Address is where the egress proxy listens, empty while it is down.
	Address string
	Reason  PickerReason
	// Connections is what the egress proxy has been asked, so an operator can see
	// whether an app's traffic reaches it at all.
	Connections PickerConnections
	// Cowork is where Cowork routing stands, which rides the picker.
	Cowork PickerCowork
}

// PickerError is a refusal with the reason a console shows.
type PickerError struct {
	Reason PickerReason
	Cause  error
}

func (e *PickerError) Error() string {
	if e.Cause == nil {
		return fmt.Sprintf("desktop picker: %s", e.Reason)
	}
	return fmt.Sprintf("desktop picker: %s: %v", e.Reason, e.Cause)
}

func (e *PickerError) Unwrap() error { return e.Cause }

// ErrPickerInUse reports Desktop asked to leave proxy mode, or to be removed,
// while its picker still depends on it.
var ErrPickerInUse = errors.New("the Desktop picker is on: turn it off first")

// DesktopPicker runs Desktop's picker. The composition root backs it with the
// egress proxy, the OS trust store and Desktop's config library, so the
// workflows never reach any of them.
type DesktopPicker interface {
	View(ctx context.Context) PickerView
	// Enable turns the picker on. It is the only call that may ask the OS to
	// trust Relo's root, and it is never made on its own.
	Enable(ctx context.Context) (PickerView, error)
	// Disable turns the picker off and takes back everything it added.
	Disable(ctx context.Context) (PickerView, error)
	// RemoveTrust takes Relo's root out of the OS trust store, which is what is
	// left to do when Disable could not.
	RemoveTrust(ctx context.Context) (PickerView, error)
	// EnableCowork hands Cowork's vendor API traffic to the intercept. It needs the
	// picker on and armed and asks the OS for nothing.
	EnableCowork(ctx context.Context) (PickerView, error)
	// DisableCowork stops handing it on, at once.
	DisableCowork(ctx context.Context) (PickerView, error)
}

// pickerEnabled reports whether Desktop's picker is on. A build with no picker
// has none to keep.
func (m *Manager) pickerEnabled(ctx context.Context) bool {
	return m.picker != nil && m.picker.View(ctx).Enabled
}

// requirePickerOff refuses taking Desktop out of proxy mode, or removing it,
// while the picker is on: the picker rides the Code tab's wiring and Desktop's
// network, which Relo must not pull out from under an open app.
func (m *Manager) requirePickerOff(ctx context.Context, agent Agent) error {
	if agent.ID == ProxyDesktop && m.pickerEnabled(ctx) {
		return fmt.Errorf("%s: %w", agent.ID, ErrPickerInUse)
	}
	return nil
}
