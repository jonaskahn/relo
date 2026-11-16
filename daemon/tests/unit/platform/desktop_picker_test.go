package platform_test

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/codingclients"
	"github.com/jonaskahn/relo/internal/adapters/ostrust"
	"github.com/jonaskahn/relo/internal/adapters/proxy"
	appintegration "github.com/jonaskahn/relo/internal/application/integration"
	"github.com/jonaskahn/relo/internal/platform"
)

// fakeTrust is an OS trust store the test controls: it answers the state it is
// left in, changes it only when asked, and records every call.
type fakeTrust struct {
	state ostrust.State
	// byHost overrides the state for a root read through one host's leaf.
	byHost    map[string]ostrust.State
	declines  bool
	untrustOK bool
	calls     []string
}

func (f *fakeTrust) Inspect(_ context.Context, root ostrust.Root) ostrust.State {
	if state, ok := f.byHost[root.Host]; ok {
		return state
	}
	return f.state
}

func (f *fakeTrust) Trust(context.Context, ostrust.Root) error {
	f.calls = append(f.calls, "trust")
	if f.declines {
		return ostrust.ErrDeclined
	}
	f.state = ostrust.Trusted
	return nil
}

func (f *fakeTrust) Untrust(context.Context, ostrust.Root) error {
	f.calls = append(f.calls, "untrust")
	if !f.untrustOK {
		return ostrust.ErrDeclined
	}
	f.state = ostrust.Untrusted
	return nil
}

func (f *fakeTrust) asked(call string) int {
	count := 0
	for _, seen := range f.calls {
		if seen == call {
			count++
		}
	}
	return count
}

type pickerRig struct {
	picker  *platform.DesktopPicker
	trust   *fakeTrust
	library string
	home    string
	wired   bool
	// interceptUp is whether the vendor API's intercept answers.
	interceptUp bool
}

func newPickerRig(t *testing.T) *pickerRig {
	t.Helper()
	home := t.TempDir()
	rig := &pickerRig{
		trust: &fakeTrust{state: ostrust.Untrusted, untrustOK: true}, home: home, wired: true, interceptUp: true,
		library: filepath.Join(t.TempDir(), "configLibrary"),
	}
	if err := os.MkdirAll(rig.library, 0o755); err != nil {
		t.Fatal(err)
	}
	rig.picker = rig.build(t, "darwin")
	return rig
}

func (r *pickerRig) build(t *testing.T, goos string) *platform.DesktopPicker {
	t.Helper()
	authority, err := proxy.LoadOrCreateAuthority(filepath.Join(r.home, "ca"), proxy.InterceptHosts...)
	if err != nil {
		t.Fatalf("LoadOrCreateAuthority() error = %v", err)
	}
	return platform.NewDesktopPicker(platform.DesktopPickerOptions{
		Home: r.home, Platform: goos, Library: func() string { return r.library },
		Installed: func() error { return nil }, Authority: func() (*proxy.Authority, error) { return authority, nil },
		Trust: r.trust, Catalog: func() []proxy.CatalogRow { return nil },
		DesktopWired: func(context.Context) bool { return r.wired },
		Intercept:    func() (string, string, bool) { return "127.0.0.1:1", "token", r.interceptUp },
		Upstream:     "http://127.0.0.1:1", Transport: http.DefaultTransport,
	})
}

func reasonOfError(t *testing.T, err error) appintegration.PickerReason {
	t.Helper()
	var refusal *appintegration.PickerError
	if !errors.As(err, &refusal) {
		t.Fatalf("error = %v, want a picker refusal", err)
	}
	return refusal.Reason
}

func (r *pickerRig) profile() codingclients.DesktopPickerStatus {
	status, _ := codingclients.InspectDesktopPickerProfile(r.library)
	return status
}

// TestEnablingAsksTheOSOnceAndLeavesTheProxyArmedAndDesktopPointedAtIt pins the
// happy path in the order it is built: trusted, pointed, armed.
func TestEnablingAsksTheOSOnceAndLeavesTheProxyArmedAndDesktopPointedAtIt(t *testing.T) {
	rig := newPickerRig(t)
	t.Cleanup(func() { _, _ = rig.picker.Disable(context.Background()) })
	view, err := rig.picker.Enable(context.Background())
	if err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	if !view.Enabled || !view.Armed || view.Trust != string(ostrust.Trusted) || view.Address == "" ||
		view.Reason != appintegration.PickerRestartRequired || rig.profile() != codingclients.DesktopPickerApplied {
		t.Fatalf("view = %+v profile %s, want on, armed, trusted, pointed and a restart asked for", view, rig.profile())
	}
	if rig.trust.asked("trust") != 1 {
		t.Fatalf("trust asked %d times, want once", rig.trust.asked("trust"))
	}
	if again, err := rig.picker.Enable(context.Background()); err != nil || rig.trust.asked("trust") != 1 || !again.Armed {
		t.Fatalf("enabling twice = %+v %v, trust asked %d, want no second ask", again, err, rig.trust.asked("trust"))
	}
}

func TestADeclinedTrustLeavesNothingBehind(t *testing.T) {
	rig := newPickerRig(t)
	rig.trust.declines = true
	view, err := rig.picker.Enable(context.Background())
	if reasonOfError(t, err) != appintegration.PickerTrustDeclined || view.Enabled || view.Armed || view.Address != "" {
		t.Fatalf("view = %+v error = %v, want declined with the proxy stopped and nothing on", view, err)
	}
	if rig.profile() != codingclients.DesktopPickerAbsent {
		t.Fatalf("profile = %s, want none", rig.profile())
	}
}

func TestThePickerRefusesWhatItCannotRideBeforeTouchingAnything(t *testing.T) {
	rig := newPickerRig(t)
	rig.wired = false
	if _, err := rig.picker.Enable(context.Background()); reasonOfError(t, err) != appintegration.PickerNeedsDesktopProxy {
		t.Fatalf("error = %v, want %s", err, appintegration.PickerNeedsDesktopProxy)
	}
	linux := rig.build(t, "linux")
	rig.wired = true
	if _, err := linux.Enable(context.Background()); reasonOfError(t, err) != appintegration.PickerUnsupportedOS {
		t.Fatalf("error = %v, want %s", err, appintegration.PickerUnsupportedOS)
	}
	if len(rig.trust.calls) != 0 {
		t.Fatalf("the OS was asked %v before the preconditions held", rig.trust.calls)
	}
}

// TestAGatewayProfileUndoesTheTrustTheAttemptAdded keeps a failed enable from
// leaving a trusted root behind: the trust it added is taken back out.
func TestAGatewayProfileUndoesTheTrustTheAttemptAdded(t *testing.T) {
	rig := newPickerRig(t)
	if _, _, err := codingclients.WriteDesktopProfile(rig.library, "http://127.0.0.1:10101", "rlo_ak_x", nil); err != nil {
		t.Fatal(err)
	}
	view, err := rig.picker.Enable(context.Background())
	if reasonOfError(t, err) != appintegration.PickerGatewaySelected || view.Address != "" {
		t.Fatalf("view = %+v error = %v, want the gateway refused with the proxy stopped", view, err)
	}
	if rig.trust.asked("untrust") != 1 || rig.trust.state == ostrust.Trusted {
		t.Fatalf("calls = %v state = %s, want the added trust removed", rig.trust.calls, rig.trust.state)
	}
}

func TestDisablingPointsDesktopBackStopsTheProxyAndTakesTheTrustOut(t *testing.T) {
	rig := newPickerRig(t)
	if _, err := rig.picker.Enable(context.Background()); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	view, err := rig.picker.Disable(context.Background())
	if err != nil {
		t.Fatalf("Disable() error = %v", err)
	}
	if view.Enabled || view.Armed || view.Address != "" || rig.profile() != codingclients.DesktopPickerAbsent ||
		rig.trust.state != ostrust.Untrusted || view.Reason != appintegration.PickerOff {
		t.Fatalf("view = %+v profile %s trust %s, want everything taken back", view, rig.profile(), rig.trust.state)
	}
}

// TestATrustThatWillNotComeOutIsNamedNotHidden keeps the cleanup honest: the
// picker is off and Desktop is pointed back, and the root that remains is
// reported so the operator can remove it.
func TestATrustThatWillNotComeOutIsNamedNotHidden(t *testing.T) {
	rig := newPickerRig(t)
	if _, err := rig.picker.Enable(context.Background()); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	rig.trust.untrustOK = false
	view, err := rig.picker.Disable(context.Background())
	if reasonOfError(t, err) != appintegration.PickerTrustRemains || view.Enabled || rig.profile() != codingclients.DesktopPickerAbsent {
		t.Fatalf("view = %+v error = %v, want off, pointed back and the remaining trust named", view, err)
	}
	if view.Reason != appintegration.PickerTrustRemains {
		t.Fatalf("reason = %s, want %s", view.Reason, appintegration.PickerTrustRemains)
	}
	rig.trust.untrustOK = true
	if view, err := rig.picker.RemoveTrust(context.Background()); err != nil || view.Trust != string(ostrust.Untrusted) {
		t.Fatalf("RemoveTrust() = %+v %v, want the root out", view, err)
	}
}

// TestARestartRestoresTheProxyButNeverAsksTheOSForTrust pins the restart: the
// picker comes back on its own port and is armed only while the OS still trusts
// the root, and the OS is never asked to trust anything.
func TestARestartRestoresTheProxyButNeverAsksTheOSForTrust(t *testing.T) {
	rig := newPickerRig(t)
	first, err := rig.picker.Enable(context.Background())
	if err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	cancelFirst(t, rig)
	asked := rig.trust.asked("trust")

	rig.trust.state = ostrust.Untrusted
	restarted := rig.build(t, "darwin")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { restarted.Run(ctx); close(done) }()
	t.Cleanup(func() { cancel(); <-done })

	waitFor(t, func() bool { return restarted.View(ctx).Address != "" })
	view := restarted.View(ctx)
	if view.Address != first.Address {
		t.Fatalf("address = %q, want the same port %q so Desktop's profile still names it", view.Address, first.Address)
	}
	if view.Armed || view.Reason != appintegration.PickerTrustPending {
		t.Fatalf("view = %+v, want unarmed with the trust pending while the OS does not trust the root", view)
	}
	if rig.trust.asked("trust") != asked {
		t.Fatal("a restart asked the OS to trust the root")
	}
}

// cancelFirst stops the proxy the first picker started, as a daemon stop would,
// without touching the profile or the marker a restart restores from.
func cancelFirst(t *testing.T, rig *pickerRig) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { rig.picker.Run(ctx); close(done) }()
	waitFor(t, func() bool { return rig.picker.View(ctx).Address != "" })
	cancel()
	<-done
}

func waitFor(t *testing.T, condition func() bool) {
	t.Helper()
	for deadline := 0; deadline < 200; deadline++ {
		if condition() {
			return
		}
		sleep()
	}
	t.Fatal("the condition did not hold in time")
}
