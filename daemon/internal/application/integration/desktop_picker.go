// Claude Desktop's own model picker. It is opt-in and off by default, because it
// is the one piece of Relo that asks the operating system to trust a certificate
// and terminates TLS for a host the operator signs in to.
package integration

import (
	"context"
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
	// PickerCoworkTrustDeclined is the operator turning down the system approval
	// dialog. PickerCoworkTrustUnavailable is a machine with no way to ask for one.
	// Both leave the certificate to be installed by hand.
	PickerCoworkTrustDeclined    PickerReason = "cowork_trust_declined"
	PickerCoworkTrustUnavailable PickerReason = "cowork_trust_unavailable"
	// PickerCoworkTrustRemains is system-wide trust that could not be removed, or
	// that is still installed after Cowork routing was turned off.
	PickerCoworkTrustRemains PickerReason = "cowork_trust_remains"
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
	// picker armed and the OS to trust Relo's root system-wide.
	Armed bool
	// Trust is the OS's system-wide trust of Relo's root: trusted, untrusted,
	// unknown or unsupported.
	Trust  string
	Reason PickerReason
	// Manual is how to install or remove the certificate by hand. It is set only
	// while that is what is left to do.
	Manual *PickerManual
}

// PickerManual is what an operator needs to install or remove Relo's
// certificate without Relo's help.
type PickerManual struct {
	OS       string
	CertPath string
	SHA1     string
	SHA256   string
	DocsURL  string
	Install  string
	Remove   string
	Check    string
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
	// picker on and armed, and asks the OS to trust Relo's root system-wide when it
	// does not already.
	EnableCowork(ctx context.Context) (PickerView, error)
	// DisableCowork stops handing it on, at once. The system-wide trust stays
	// until RemoveSystemTrust or Retire takes it out.
	DisableCowork(ctx context.Context) (PickerView, error)
	// RemoveSystemTrust takes Relo's root out of the system-wide trust. It
	// refuses while Cowork routing is on.
	RemoveSystemTrust(ctx context.Context) (PickerView, error)
	// Retire takes back everything the picker and Cowork routing added, and every
	// certificate Relo asked the OS to trust, because Desktop is leaving proxy mode
	// or being removed. It does nothing when there is nothing to take back.
	Retire(ctx context.Context) (PickerView, error)
}

// retirePicker takes Desktop's picker and the trust Relo added out of the way
// before Desktop leaves proxy mode or goes away. When it cannot, the change is
// refused with the picker's own reason, so Relo never leaves a certificate
// trusted on a machine it no longer serves.
func (m *Manager) retirePicker(ctx context.Context, agent Agent) error {
	if agent.ID != ProxyDesktop || m.picker == nil {
		return nil
	}
	_, err := m.picker.Retire(ctx)
	return err
}
