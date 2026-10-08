// Tray status lines: human-readable daemon state.
package desktop

import (
	"errors"
	"net"
	"strconv"
	"strings"

	"github.com/jonaskahn/relo/internal/i18n"
	"github.com/jonaskahn/relo/internal/platform"
)

const dash = "—"

// The state markers the status row carries.
const (
	glyphRunning = "●"
	glyphStopped = "○"
	glyphFailed  = "⚠"
)

func statusLine(state platform.DaemonState, addr string, lastErr error, localized *i18n.Translator) string {
	switch state {
	case platform.StateRunning:
		return glyphRunning + " " + localized.Text("tray.status.running", map[string]any{"Addr": addr})
	case platform.StateFailed:
		return glyphFailed + " " + localized.Text("tray.status.failed", map[string]any{
			"Detail": shortError(lastErr, localized),
		})
	default:
		return glyphStopped + " " + localized.Text("tray.status.stopped", nil)
	}
}

func shortError(err error, localized *i18n.Translator) string {
	if err == nil {
		return localized.Text("tray.error.unknown", nil)
	}
	detail := strings.ToLower(err.Error())
	switch {
	case errors.Is(err, platform.ErrListenerNotReady):
		return localized.Text("tray.error.not_ready", nil)
	case strings.Contains(detail, "address already in use"):
		return localized.Text("tray.error.port_in_use", nil)
	case strings.Contains(detail, "timeout"), strings.Contains(detail, "deadline exceeded"):
		return localized.Text("tray.error.timed_out", nil)
	case strings.Contains(detail, "database"), strings.Contains(detail, "sqlite"):
		return localized.Text("tray.error.database", nil)
	default:
		return localized.Text("tray.error.start_failed", nil)
	}
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func httpURL(addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "http://" + addr
	}
	switch host {
	case "", "0.0.0.0", "::":
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, port)
}

// DashboardURL builds the dashboard address from the configured bind and
// port, for the times the proxy is not running and its live address is
// unknown.
func DashboardURL(bind string, port int) string {
	return httpURL(net.JoinHostPort(bind, strconv.Itoa(port)))
}

func displayOr(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
