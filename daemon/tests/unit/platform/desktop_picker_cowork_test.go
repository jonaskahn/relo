package platform_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

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

// TestCoworkRoutingIsRefusedWhenTheOSDoesNotTrustTheRootForTheVendorHost keeps
// it from arming on a guess: the picker's claude.ai trust is not enough.
func TestCoworkRoutingIsRefusedWhenTheOSDoesNotTrustTheRootForTheVendorHost(t *testing.T) {
	rig := newPickerRig(t)
	enableThePicker(t, rig)
	rig.trust.byHost = map[string]ostrust.State{proxy.HostAnthropicAPI: ostrust.Untrusted}
	view, err := rig.picker.EnableCowork(context.Background())
	if reason := reasonOfError(t, err); reason != appintegration.PickerCoworkTrustMissing {
		t.Fatalf("reason = %q, want %q", reason, appintegration.PickerCoworkTrustMissing)
	}
	if view.Cowork.Enabled || view.Cowork.Armed || !view.Armed {
		t.Fatalf("view = %+v, want Cowork off and the picker left working", view)
	}
}

// TestCoworkRoutingArmsWithoutAskingTheOSAndAsksForARestart is the happy path.
func TestCoworkRoutingArmsWithoutAskingTheOSAndAsksForARestart(t *testing.T) {
	rig := newPickerRig(t)
	enableThePicker(t, rig)
	asked := rig.trust.asked("trust")
	view, err := rig.picker.EnableCowork(context.Background())
	if err != nil {
		t.Fatalf("EnableCowork() error = %v", err)
	}
	if !view.Cowork.Enabled || !view.Cowork.Armed || view.Cowork.Reason != appintegration.PickerCoworkRestart {
		t.Fatalf("cowork = %+v, want on and armed with a restart asked for", view.Cowork)
	}
	if rig.trust.asked("trust") != asked {
		t.Fatal("turning Cowork on asked the OS to trust something")
	}
	if got := rig.picker.View(context.Background()).Cowork.Reason; got != appintegration.PickerCoworkActive {
		t.Fatalf("reason on the next read = %q, want %q", got, appintegration.PickerCoworkActive)
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
}

// TestTurningThePickerOffTurnsCoworkOffFirst keeps the trust from being taken
// out from under a route that still depends on it.
func TestTurningThePickerOffTurnsCoworkOffFirst(t *testing.T) {
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

	rig.trust.byHost = map[string]ostrust.State{proxy.HostAnthropicAPI: ostrust.Untrusted}
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
