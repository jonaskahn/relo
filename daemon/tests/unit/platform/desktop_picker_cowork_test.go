package platform_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/codingclients"
	"github.com/jonaskahn/relo/internal/adapters/ostrust"
	"github.com/jonaskahn/relo/internal/adapters/proxy"
	appintegration "github.com/jonaskahn/relo/internal/application/integration"
)

func coworkMarker(rig *pickerRig) string { return filepath.Join(rig.home, "desktop-picker", "cowork") }

func enableThePicker(t *testing.T, rig *pickerRig) {
	t.Helper()
	t.Cleanup(func() { _, _ = rig.picker.Disable(context.Background()) })
	if _, err := rig.picker.Enable(context.Background()); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
}

// TestCoworkRoutingNeedsThePickerFirst pins the dependency: it rides the
// picker's proxy, profile and trust, so it is refused while the picker is off.
func TestCoworkRoutingNeedsThePickerFirst(t *testing.T) {
	rig := newPickerRig(t)
	_, err := rig.picker.EnableCowork(context.Background())
	if reason := reasonOfError(t, err); reason != appintegration.PickerCoworkNeedsPicker {
		t.Fatalf("reason = %q, want %q", reason, appintegration.PickerCoworkNeedsPicker)
	}
	if _, statErr := os.Stat(coworkMarker(rig)); !os.IsNotExist(statErr) {
		t.Fatal("a refused request left the marker behind")
	}
}

// TestCoworkRoutingAsksTheOSOnceForSystemWideTrustAndAsksForARestart is the happy
// path: the picker's user trust is not enough, so the OS is asked to trust the root
// system-wide, once, and only now.
func TestCoworkRoutingAsksTheOSOnceForSystemWideTrustAndAsksForARestart(t *testing.T) {
	rig := newPickerRig(t)
	rig.trust.system = ostrust.Untrusted
	enableThePicker(t, rig)
	if rig.trust.asked("trust-system") != 0 {
		t.Fatal("enabling the picker asked for system-wide trust")
	}
	view, err := rig.picker.EnableCowork(context.Background())
	if err != nil {
		t.Fatalf("EnableCowork() error = %v", err)
	}
	if !view.Cowork.Enabled || !view.Cowork.Armed || view.Cowork.Reason != appintegration.PickerCoworkRestart ||
		view.Cowork.Trust != string(ostrust.Trusted) {
		t.Fatalf("cowork = %+v, want on, armed, trusted and a restart asked for", view.Cowork)
	}
	if rig.trust.asked("trust-system") != 1 {
		t.Fatalf("system-wide trust asked %d times, want once", rig.trust.asked("trust-system"))
	}
	if view.Cowork.Manual == nil || view.Cowork.Manual.SHA256 == "" || view.Cowork.Manual.CertPath == "" {
		t.Fatalf("manual = %+v, want the steps to do it by hand", view.Cowork.Manual)
	}
	if got := rig.picker.View(context.Background()).Cowork.Reason; got != appintegration.PickerCoworkActive {
		t.Fatalf("reason on the next read = %q, want %q", got, appintegration.PickerCoworkActive)
	}
}

// TestCoworkRoutingDoesNotAskAgainWhenTheSystemAlreadyTrustsTheRoot keeps the
// approval dialog for the one time it is needed.
func TestCoworkRoutingDoesNotAskAgainWhenTheSystemAlreadyTrustsTheRoot(t *testing.T) {
	rig := newPickerRig(t)
	enableThePicker(t, rig)
	if _, err := rig.picker.EnableCowork(context.Background()); err != nil {
		t.Fatalf("EnableCowork() error = %v", err)
	}
	if rig.trust.asked("trust-system") != 0 {
		t.Fatal("the OS was asked to trust a root it already trusted")
	}
}

// TestADeclinedOrImpossibleSystemTrustLeavesNothingChangedAndNamesTheManualWay
// pins both refusals: the operator turns the dialog down, or the machine has no
// way to ask. Either way Cowork stays off, the picker keeps working, and the
// steps to do it by hand come back with the answer.
func TestADeclinedOrImpossibleSystemTrustLeavesNothingChangedAndNamesTheManualWay(t *testing.T) {
	cases := map[string]struct {
		err    error
		reason appintegration.PickerReason
	}{
		"declined":    {ostrust.ErrDeclined, appintegration.PickerCoworkTrustDeclined},
		"unavailable": {ostrust.ErrUnavailable, appintegration.PickerCoworkTrustUnavailable},
	}
	for name, test := range cases {
		rig := newPickerRig(t)
		rig.trust.system = ostrust.Untrusted
		rig.trust.trustErr = test.err
		enableThePicker(t, rig)
		view, err := rig.picker.EnableCowork(context.Background())
		if reason := reasonOfError(t, err); reason != test.reason {
			t.Fatalf("%s: reason = %q, want %q", name, reason, test.reason)
		}
		if view.Cowork.Enabled || view.Cowork.Armed || !view.Armed || view.Cowork.Manual == nil {
			t.Fatalf("%s: view = %+v, want Cowork off, the picker working and the manual steps", name, view)
		}
		if _, statErr := os.Stat(coworkMarker(rig)); !os.IsNotExist(statErr) {
			t.Fatalf("%s: a refused request left the marker behind", name)
		}
	}
}

// TestSystemTrustThatDoesNotTakeIsTakenBackOut keeps a failed read-back from
// leaving a root trusted that nothing checked.
func TestSystemTrustThatDoesNotTakeIsTakenBackOut(t *testing.T) {
	rig := newPickerRig(t)
	rig.trust.byHost = map[string]ostrust.State{proxy.HostAnthropicAPI: ostrust.Untrusted}
	enableThePicker(t, rig)
	_, err := rig.picker.EnableCowork(context.Background())
	if reason := reasonOfError(t, err); reason != appintegration.PickerCoworkTrustMissing {
		t.Fatalf("reason = %q, want %q", reason, appintegration.PickerCoworkTrustMissing)
	}
	if rig.trust.asked("untrust-system") != 1 {
		t.Fatalf("system-wide untrust asked %d times, want the failed install taken back", rig.trust.asked("untrust-system"))
	}
}

func TestCoworkRoutingIsRefusedWhileTheInterceptIsDown(t *testing.T) {
	rig := newPickerRig(t)
	enableThePicker(t, rig)
	rig.interceptUp = false
	_, err := rig.picker.EnableCowork(context.Background())
	if reason := reasonOfError(t, err); reason != appintegration.PickerProxyUnavailable {
		t.Fatalf("reason = %q, want %q", reason, appintegration.PickerProxyUnavailable)
	}
}

// TestTurningCoworkOffLeavesThePickerOn is the kill switch: it disarms at once
// and forgets, and the model picker keeps working.
func TestTurningCoworkOffLeavesThePickerOn(t *testing.T) {
	rig := newPickerRig(t)
	enableThePicker(t, rig)
	if _, err := rig.picker.EnableCowork(context.Background()); err != nil {
		t.Fatalf("EnableCowork() error = %v", err)
	}
	view, err := rig.picker.DisableCowork(context.Background())
	if err != nil {
		t.Fatalf("DisableCowork() error = %v", err)
	}
	if view.Cowork.Enabled || view.Cowork.Armed || !view.Enabled || !view.Armed {
		t.Fatalf("view = %+v, want Cowork off and the picker still on and armed", view)
	}
	if _, statErr := os.Stat(coworkMarker(rig)); !os.IsNotExist(statErr) {
		t.Fatal("the marker outlived the switch")
	}
	if rig.trust.asked("untrust-system") != 0 {
		t.Fatal("turning Cowork off removed the system-wide trust, which stays until it is removed on purpose")
	}
	if view.Cowork.Reason != appintegration.PickerCoworkTrustRemains || view.Cowork.Trust != string(ostrust.Trusted) {
		t.Fatalf("cowork = %+v, want the trust reported as still installed", view.Cowork)
	}
}

// TestTurningThePickerOffTurnsCoworkOffButLeavesTheSystemTrust keeps routing from
// outliving the proxy it rides, and the wider trust from being removed by a
// switch that is not about it.
func TestTurningThePickerOffTurnsCoworkOffButLeavesTheSystemTrust(t *testing.T) {
	rig := newPickerRig(t)
	enableThePicker(t, rig)
	if _, err := rig.picker.EnableCowork(context.Background()); err != nil {
		t.Fatalf("EnableCowork() error = %v", err)
	}
	rig.trust.untrustOK = true
	view, err := rig.picker.Disable(context.Background())
	if err != nil {
		t.Fatalf("Disable() error = %v", err)
	}
	if view.Cowork.Enabled || view.Cowork.Armed {
		t.Fatalf("cowork = %+v, want it off with the picker", view.Cowork)
	}
	if _, statErr := os.Stat(coworkMarker(rig)); !os.IsNotExist(statErr) {
		t.Fatal("the marker outlived the picker")
	}
	if rig.trust.asked("untrust-system") != 0 {
		t.Fatal("turning the picker off removed the system-wide trust")
	}
}

// TestRemovingSystemTrustRefusesWhileCoworkIsOnAndOtherwiseTakesItOut pins the
// explicit button.
func TestRemovingSystemTrustRefusesWhileCoworkIsOnAndOtherwiseTakesItOut(t *testing.T) {
	rig := newPickerRig(t)
	enableThePicker(t, rig)
	if _, err := rig.picker.EnableCowork(context.Background()); err != nil {
		t.Fatalf("EnableCowork() error = %v", err)
	}
	_, err := rig.picker.RemoveSystemTrust(context.Background())
	if reason := reasonOfError(t, err); reason != appintegration.PickerCoworkActive {
		t.Fatalf("reason = %q, want %q", reason, appintegration.PickerCoworkActive)
	}
	if _, err := rig.picker.DisableCowork(context.Background()); err != nil {
		t.Fatalf("DisableCowork() error = %v", err)
	}
	view, err := rig.picker.RemoveSystemTrust(context.Background())
	if err != nil || view.Cowork.Trust != string(ostrust.Untrusted) {
		t.Fatalf("view = %+v error = %v, want the system trust gone", view.Cowork, err)
	}
}

func TestSystemTrustThatCannotBeRemovedIsReportedWithTheManualSteps(t *testing.T) {
	rig := newPickerRig(t)
	enableThePicker(t, rig)
	if _, err := rig.picker.EnableCowork(context.Background()); err != nil {
		t.Fatalf("EnableCowork() error = %v", err)
	}
	if _, err := rig.picker.DisableCowork(context.Background()); err != nil {
		t.Fatalf("DisableCowork() error = %v", err)
	}
	rig.trust.systemUntrustErr = ostrust.ErrDeclined
	view, err := rig.picker.RemoveSystemTrust(context.Background())
	if reason := reasonOfError(t, err); reason != appintegration.PickerCoworkTrustRemains {
		t.Fatalf("reason = %q, want %q", reason, appintegration.PickerCoworkTrustRemains)
	}
	if view.Cowork.Trust != string(ostrust.Trusted) || view.Cowork.Manual == nil || view.Cowork.Manual.Remove == "" {
		t.Fatalf("cowork = %+v, want the trust still there and the steps to remove it", view.Cowork)
	}
}

// TestRetireTakesBackEverythingInOrderAndStopsAtTheFirstFailure is what Desktop
// leaving proxy mode, or being removed, runs.
func TestRetireTakesBackEverythingInOrderAndStopsAtTheFirstFailure(t *testing.T) {
	rig := newPickerRig(t)
	rig.trust.untrustOK = true
	if _, err := rig.picker.Enable(context.Background()); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	if _, err := rig.picker.EnableCowork(context.Background()); err != nil {
		t.Fatalf("EnableCowork() error = %v", err)
	}
	view, err := rig.picker.Retire(context.Background())
	if err != nil {
		t.Fatalf("Retire() error = %v", err)
	}
	if view.Enabled || view.Cowork.Enabled || rig.profile() != codingclients.DesktopPickerAbsent {
		t.Fatalf("view = %+v profile %s, want the picker and Cowork off and the profile gone", view, rig.profile())
	}
	untrust, system := indexOf(rig.trust.calls, "untrust"), indexOf(rig.trust.calls, "untrust-system")
	if untrust < 0 || system < 0 || untrust > system {
		t.Fatalf("calls = %v, want the user's trust out before the system-wide trust", rig.trust.calls)
	}

	failing := newPickerRig(t)
	failing.trust.untrustOK = false
	if _, err := failing.picker.Enable(context.Background()); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	_, err = failing.picker.Retire(context.Background())
	if reason := reasonOfError(t, err); reason != appintegration.PickerTrustRemains {
		t.Fatalf("reason = %q, want %q", reason, appintegration.PickerTrustRemains)
	}
	if failing.trust.asked("untrust-system") != 0 {
		t.Fatal("the system-wide trust was touched after the first step failed")
	}
}

// TestRetireWithNothingInstalledChangesNothing keeps Desktop's mode switch quiet
// for everyone who never used the picker.
func TestRetireWithNothingInstalledChangesNothing(t *testing.T) {
	rig := newPickerRig(t)
	rig.trust.untrustOK = true
	if _, err := rig.picker.Retire(context.Background()); err != nil {
		t.Fatalf("Retire() error = %v", err)
	}
	if rig.profile() != codingclients.DesktopPickerAbsent {
		t.Fatalf("profile = %s, want nothing written", rig.profile())
	}
}

func indexOf(calls []string, want string) int {
	for index, call := range calls {
		if call == want {
			return index
		}
	}
	return -1
}

// TestARestartRestoresCoworkRoutingOnlyWhileTheRootIsTrustedForTheVendorHost
// pins the restart: it keeps what the operator turned on, never asks the OS, and
// stays disarmed when the vendor host's trust has gone.
func TestARestartRestoresCoworkRoutingOnlyWhileTheRootIsTrustedForTheVendorHost(t *testing.T) {
	rig := newPickerRig(t)
	if _, err := rig.picker.Enable(context.Background()); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	if _, err := rig.picker.EnableCowork(context.Background()); err != nil {
		t.Fatalf("EnableCowork() error = %v", err)
	}
	cancelFirst(t, rig)
	asked := rig.trust.asked("trust")

	rig.trust.system = ostrust.Untrusted
	restarted := rig.build(t, "darwin")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { restarted.Run(ctx); close(done) }()
	t.Cleanup(func() { cancel(); <-done })
	waitFor(t, func() bool { return restarted.View(ctx).Address != "" })

	view := restarted.View(ctx)
	if !view.Armed || !view.Cowork.Enabled || view.Cowork.Armed || view.Cowork.Reason != appintegration.PickerCoworkTrustMissing {
		t.Fatalf("view = %+v, want the picker armed, Cowork remembered and disarmed for want of trust", view)
	}
	if rig.trust.asked("trust") != asked {
		t.Fatal("a restart asked the OS to trust the root")
	}
}
