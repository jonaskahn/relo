package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jonaskahn/relo/internal/config"
	"github.com/jonaskahn/relo/internal/server"
)

// openConsole is a server whose operator turned the sign-in off, which is what
// a loopback configuration defaults to.
func openConsole(t *testing.T) http.Handler {
	t.Helper()
	cfg := config.DefaultConfig()
	cfg.Admin.Login = false
	served := server.New(server.Options{Config: &cfg, AdminToken: adminToken, Version: testVersion})
	served.SetDashboard(stubConsole())
	return served.Handler()
}

// consoleRequest builds a request the way a browser on this machine sends one:
// it names the console in Host, comes from loopback, and carries no credential
// at all.
func consoleRequest(method, path, host string) *http.Request {
	request := httptest.NewRequest(method, path, nil)
	request.Host = host
	request.RemoteAddr = "127.0.0.1:54321"
	return request
}

type sessionReport struct {
	Authenticated bool `json:"authenticated"`
	LoginRequired bool `json:"login_required"`
}

// sessionOf asks the management API what it expects of a caller.
func sessionOf(t *testing.T, handler http.Handler) sessionReport {
	t.Helper()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, consoleRequest(http.MethodGet, "/api/v1/auth/session", "127.0.0.1:10101"))
	if response.Code != http.StatusOK {
		t.Fatalf("session status = %d, body = %s", response.Code, response.Body.String())
	}
	report := sessionReport{}
	if err := json.Unmarshal(response.Body.Bytes(), &report); err != nil {
		t.Fatalf("decode the session: %v", err)
	}
	return report
}

func TestOpenConsole(t *testing.T) {
	handler := openConsole(t)

	t.Run("a status read needs no credential", func(t *testing.T) {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, consoleRequest(http.MethodGet, "/api/v1/status", "127.0.0.1:10101"))
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
		}
	})

	t.Run("a page is served rather than redirected to a sign-in", func(t *testing.T) {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, consoleRequest(http.MethodGet, "/", "127.0.0.1:10101"))
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d, want the console itself", response.Code)
		}
	})

	t.Run("a write with no Origin and no CSRF token goes through", func(t *testing.T) {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, consoleRequest(http.MethodPost, "/api/v1/auth/logout", "127.0.0.1:10101"))
		if response.Code != http.StatusNoContent {
			t.Fatalf("status = %d, body = %s, want the write served", response.Code, response.Body.String())
		}
	})

	t.Run("a write from another site is refused", func(t *testing.T) {
		request := consoleRequest(http.MethodPost, "/api/v1/auth/logout", "127.0.0.1:10101")
		request.Header.Set("Origin", "https://evil.example")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want a write from another site refused", response.Code)
		}
	})

	t.Run("a write from an opaque origin is refused", func(t *testing.T) {
		request := consoleRequest(http.MethodPost, "/api/v1/auth/logout", "127.0.0.1:10101")
		request.Header.Set("Origin", "null")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want a sandboxed page refused", response.Code)
		}
	})

	t.Run("a Host that names another site is refused", func(t *testing.T) {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, consoleRequest(http.MethodGet, "/api/v1/status", "relo.evil.example"))
		if response.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want a rebound name refused", response.Code)
		}
	})

	t.Run("a development console on another loopback port is served", func(t *testing.T) {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, consoleRequest(http.MethodGet, "/api/v1/status", "localhost:5173"))
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d, want the development console served", response.Code)
		}
	})

	t.Run("a request from another machine is refused", func(t *testing.T) {
		request := consoleRequest(http.MethodGet, "/api/v1/status", "127.0.0.1:10101")
		request.RemoteAddr = "192.168.1.20:44321"
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want a remote caller refused", response.Code)
		}
	})

	t.Run("the session endpoint reports that no sign-in is needed", func(t *testing.T) {
		report := sessionOf(t, handler)
		if !report.Authenticated || report.LoginRequired {
			t.Fatalf("session = %+v, want a console that asks for nothing", report)
		}
	})
}

func TestGuardedConsoleReportsTheSignIn(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Admin.Login = true
	served := server.New(server.Options{Config: &cfg, AdminToken: adminToken, Version: testVersion})
	served.SetDashboard(stubConsole())

	t.Run("an anonymous caller is told a sign-in is required", func(t *testing.T) {
		report := sessionOf(t, served.Handler())
		if report.Authenticated || !report.LoginRequired {
			t.Fatalf("session = %+v, want the sign-in reported", report)
		}
	})

	t.Run("a page is sent to the sign-in", func(t *testing.T) {
		response := httptest.NewRecorder()
		served.Handler().ServeHTTP(response, consoleRequest(http.MethodGet, "/", "127.0.0.1:10101"))
		if response.Code != http.StatusSeeOther {
			t.Fatalf("status = %d, want the sign-in redirect", response.Code)
		}
	})
}
