package desktop

import (
	"errors"
	"fmt"
	"testing"

	"github.com/jonaskahn/relo/internal/i18n"
	"github.com/jonaskahn/relo/internal/platform"
)

// english builds the translator the tray renders with, so these assert on what
// an operator in the default language actually reads.
func english(t *testing.T) *i18n.Translator {
	t.Helper()
	catalogs, err := i18n.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	return catalogs.Translate("en")
}

// TestStatusLineNamesTheDaemonState covers the tray row that states what the
// proxy is doing. Each state has its own marker, so a row is readable without
// reading the words.
func TestStatusLineNamesTheDaemonState(t *testing.T) {
	localized := english(t)

	running := statusLine(platform.StateRunning, "127.0.0.1:10101", nil, localized)
	if running != glyphRunning+" "+localized.Text("tray.status.running", map[string]any{"Addr": "127.0.0.1:10101"}) {
		t.Fatalf("running row = %q, want the running marker and address", running)
	}

	stopped := statusLine(platform.StateStopped, "", nil, localized)
	if stopped != glyphStopped+" "+localized.Text("tray.status.stopped", nil) {
		t.Fatalf("stopped row = %q, want the stopped marker", stopped)
	}

	failed := statusLine(platform.StateFailed, "", errors.New("boom"), localized)
	if failed != glyphFailed+" "+localized.Text("tray.status.failed", map[string]any{"Detail": shortError(errors.New("boom"), localized)}) {
		t.Fatalf("failed row = %q, want the failed marker and a short detail", failed)
	}
}

// TestShortErrorNamesWhatToDo covers the one line a failure is reduced to. The
// point is that the operator is told which of the few things to try is the one
// that applies, rather than being handed a Go error string.
func TestShortErrorNamesWhatToDo(t *testing.T) {
	localized := english(t)
	for name, testCase := range map[string]struct {
		err  error
		want string
	}{
		"nothing went wrong as far as the tray knows": {nil, localized.Text("tray.error.unknown", nil)},
		"the listener never came up":                  {platform.ErrListenerNotReady, localized.Text("tray.error.not_ready", nil)},
		"the port is taken":                           {errors.New("listen tcp 127.0.0.1:10101: bind: address already in use"), localized.Text("tray.error.port_in_use", nil)},
		"it timed out":                                {errors.New("context deadline exceeded"), localized.Text("tray.error.timed_out", nil)},
		"it said timeout":                             {errors.New("request timeout"), localized.Text("tray.error.timed_out", nil)},
		"the database would not open":                 {errors.New("unable to open database file"), localized.Text("tray.error.database", nil)},
		"sqlite complained":                           {errors.New("SQLITE_BUSY"), localized.Text("tray.error.database", nil)},
		"something else":                              {errors.New("no route to host"), localized.Text("tray.error.start_failed", nil)},
	} {
		if got := shortError(testCase.err, localized); got != testCase.want {
			t.Errorf("%s: shortError() = %q, want %q", name, got, testCase.want)
		}
	}

	// The matching is on the message rather than on an error type, so the
	// wording the platform layers add does not lose the diagnosis.
	wrapped := fmt.Errorf("start the proxy: %w", platform.ErrListenerNotReady)
	if got := shortError(wrapped, localized); got != localized.Text("tray.error.not_ready", nil) {
		t.Errorf("a wrapped listener failure = %q, want the not-ready line", got)
	}
}

// TestDashboardURLIsOpenableInABrowser covers the address the tray hands the
// browser when the daemon is not running. A wildcard bind is read as this
// machine, because that is the only address the listener answers on.
func TestDashboardURLIsOpenableInABrowser(t *testing.T) {
	for name, testCase := range map[string]struct {
		addr string
		want string
	}{
		"a loopback bind":         {"127.0.0.1:10101", "http://127.0.0.1:10101"},
		"a wildcard bind":         {"0.0.0.0:10101", "http://127.0.0.1:10101"},
		"an ipv6 wildcard":        {"[::]:10101", "http://127.0.0.1:10101"},
		"a named host":            {"relo.local:10101", "http://relo.local:10101"},
		"an address with no port": {"127.0.0.1", "http://127.0.0.1"},
	} {
		if got := httpURL(testCase.addr); got != testCase.want {
			t.Errorf("%s: httpURL(%q) = %q, want %q", name, testCase.addr, got, testCase.want)
		}
	}

	if got := DashboardURL("0.0.0.0", 10101); got != "http://127.0.0.1:10101" {
		t.Errorf("DashboardURL() = %q, want an address a browser can open", got)
	}
}

// TestDisplayOrFallsBack covers the helper every label and row uses when a
// value may be absent: an empty value reads as the fallback rather than as a
// blank space in the menu.
func TestDisplayOrFallsBack(t *testing.T) {
	if got := displayOr("0.1.0", "unknown"); got != "0.1.0" {
		t.Errorf("displayOr(present) = %q, want the value", got)
	}
	if got := displayOr("", "unknown"); got != "unknown" {
		t.Errorf("displayOr(empty) = %q, want the fallback", got)
	}
}
