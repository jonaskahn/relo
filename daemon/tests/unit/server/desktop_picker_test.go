package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	appintegration "github.com/jonaskahn/relo/internal/application/integration"
)

type fakePicker struct {
	view  appintegration.PickerView
	err   error
	calls []string
}

func (f *fakePicker) View(context.Context) appintegration.PickerView { return f.view }

func (f *fakePicker) act(name string) (appintegration.PickerView, error) {
	f.calls = append(f.calls, name)
	return f.view, f.err
}

func (f *fakePicker) Enable(context.Context) (appintegration.PickerView, error) {
	return f.act("enable")
}
func (f *fakePicker) Disable(context.Context) (appintegration.PickerView, error) {
	return f.act("disable")
}
func (f *fakePicker) RemoveTrust(context.Context) (appintegration.PickerView, error) {
	return f.act("remove-trust")
}
func (f *fakePicker) EnableCowork(context.Context) (appintegration.PickerView, error) {
	return f.act("enable-cowork")
}
func (f *fakePicker) DisableCowork(context.Context) (appintegration.PickerView, error) {
	return f.act("disable-cowork")
}

func TestThePickerIsReadWithoutChangingAnything(t *testing.T) {
	picker := &fakePicker{view: appintegration.PickerView{Supported: true, Trust: "untrusted", Fingerprint: "AB12", Reason: appintegration.PickerOff}}
	h, served := bindingsServerWith(t, picker)
	status, body := call(t, h, served, http.MethodGet, "/api/v1/integrations/claude-desktop/picker", "")
	if status != http.StatusOK || !strings.Contains(body, `"fingerprint":"AB12"`) || !strings.Contains(body, `"reason":"off"`) {
		t.Fatalf("GET picker = %d %s", status, body)
	}
	if len(picker.calls) != 0 {
		t.Fatalf("a read called %v", picker.calls)
	}
}

// TestOnlyAnExplicitPutTurnsThePickerOn pins that no read, and no other verb,
// can make the OS ask for trust.
func TestOnlyAnExplicitPutTurnsThePickerOn(t *testing.T) {
	picker := &fakePicker{view: appintegration.PickerView{Supported: true, Enabled: true, Reason: appintegration.PickerRestartRequired}}
	h, served := bindingsServerWith(t, picker)
	if status, _ := call(t, h, served, http.MethodPost, "/api/v1/integrations/claude-desktop/picker", `{"enabled":true}`); status == http.StatusOK {
		t.Fatal("a POST turned the picker on")
	}
	if status, body := call(t, h, served, http.MethodPut, "/api/v1/integrations/claude-desktop/picker", `{"enabled":true}`); status != http.StatusOK {
		t.Fatalf("PUT enabled = %d %s", status, body)
	}
	if status, _ := call(t, h, served, http.MethodPut, "/api/v1/integrations/claude-desktop/picker", `{"enabled":false}`); status != http.StatusOK {
		t.Fatalf("PUT disabled = %d", status)
	}
	if got := strings.Join(picker.calls, ","); got != "enable,disable" {
		t.Fatalf("calls = %s, want enable then disable", got)
	}
}

func TestARefusedPickerNamesTheReasonAndShowsWhereItStands(t *testing.T) {
	picker := &fakePicker{
		view: appintegration.PickerView{Supported: true, Reason: appintegration.PickerTrustDeclined},
		err:  &appintegration.PickerError{Reason: appintegration.PickerTrustDeclined},
	}
	h, served := bindingsServerWith(t, picker)
	status, body := call(t, h, served, http.MethodPut, "/api/v1/integrations/claude-desktop/picker", `{"enabled":true}`)
	if status != http.StatusConflict {
		t.Fatalf("status = %d %s, want 409", status, body)
	}
	var decoded struct {
		Error  struct{ Code string }
		Picker struct{ Reason string }
	}
	if err := json.Unmarshal([]byte(body), &decoded); err != nil || decoded.Error.Code != "trust_declined" || decoded.Picker.Reason != "trust_declined" {
		t.Fatalf("body = %s (%v), want the code and the picker's state", body, err)
	}
}

func TestRemovingTheTrustIsItsOwnRequest(t *testing.T) {
	picker := &fakePicker{view: appintegration.PickerView{Supported: true}}
	h, served := bindingsServerWith(t, picker)
	if status, body := call(t, h, served, http.MethodDelete, "/api/v1/integrations/claude-desktop/picker/trust", ""); status != http.StatusOK {
		t.Fatalf("DELETE trust = %d %s", status, body)
	}
	if got := strings.Join(picker.calls, ","); got != "remove-trust" {
		t.Fatalf("calls = %s", got)
	}
}

func TestABuildWithNoPickerSaysSo(t *testing.T) {
	h, served := bindingsServer(t)
	if status, _ := call(t, h, served, http.MethodGet, "/api/v1/integrations/claude-desktop/picker", ""); status != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", status)
	}
}

func TestCoworkRoutingIsSwitchedByItsOwnPut(t *testing.T) {
	picker := &fakePicker{view: appintegration.PickerView{
		Supported: true, Enabled: true, Armed: true,
		Cowork: appintegration.PickerCowork{Enabled: true, Armed: true, Reason: appintegration.PickerCoworkRestart},
	}}
	h, served := bindingsServerWith(t, picker)
	status, body := call(t, h, served, http.MethodPut, "/api/v1/integrations/claude-desktop/picker/cowork", `{"enabled":true}`)
	if status != http.StatusOK || !strings.Contains(body, `"cowork":{"enabled":true,"armed":true,"reason":"cowork_restart_required"}`) {
		t.Fatalf("PUT cowork on = %d %s, want the Cowork block with its reason", status, body)
	}
	if status, _ := call(t, h, served, http.MethodPut, "/api/v1/integrations/claude-desktop/picker/cowork", `{"enabled":false}`); status != http.StatusOK {
		t.Fatalf("PUT cowork off = %d", status)
	}
	if status, _ := call(t, h, served, http.MethodPut, "/api/v1/integrations/claude-desktop/picker", `{"enabled":true}`); status != http.StatusOK {
		t.Fatalf("PUT picker = %d", status)
	}
	want := []string{"enable-cowork", "disable-cowork", "enable"}
	if strings.Join(picker.calls, ",") != strings.Join(want, ",") {
		t.Fatalf("calls = %v, want %v: the picker's own switch never turns Cowork on", picker.calls, want)
	}
}

func TestCoworkRefusalCarriesItsReasonAndThePicker(t *testing.T) {
	picker := &fakePicker{
		view: appintegration.PickerView{Supported: true, Enabled: true},
		err:  &appintegration.PickerError{Reason: appintegration.PickerCoworkTrustMissing},
	}
	h, served := bindingsServerWith(t, picker)
	status, body := call(t, h, served, http.MethodPut, "/api/v1/integrations/claude-desktop/picker/cowork", `{"enabled":true}`)
	if status != http.StatusConflict || !strings.Contains(body, `"code":"cowork_trust_missing"`) {
		t.Fatalf("PUT cowork without trust = %d %s, want a conflict naming the reason", status, body)
	}
}
