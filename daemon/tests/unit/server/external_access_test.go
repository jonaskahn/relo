package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	appsettings "github.com/jonaskahn/relo/internal/application/settings"
	"github.com/jonaskahn/relo/internal/config"
	"github.com/jonaskahn/relo/internal/server"
)

// externalConsole builds a console whose operator turned the sign-in off and
// picked whether forwarded requests may reach it. The choice lives in the
// startup file, which the guard reads on every request.
func externalConsole(t *testing.T, allow bool) http.Handler {
	t.Helper()
	path := config.ConfigPath(t.TempDir())
	if allow {
		if err := os.WriteFile(path, []byte("[admin]\nallow_external = true\n"), 0o600); err != nil {
			t.Fatalf("write the config: %v", err)
		}
	}
	cfg := config.DefaultConfig()
	cfg.Admin.Login = false
	cfg.Admin.AllowExternal = allow
	served := server.New(server.Options{
		Config: &cfg, AdminToken: adminToken, Version: testVersion,
		Settings: appsettings.New(appsettings.Options{ConfigPath: path}),
	})
	served.SetDashboard(stubConsole())
	return served.Handler()
}

// networkRequest builds a management request from one address, addressed to
// one host, the way a browser on another machine sends one.
func networkRequest(method, target, host, remote string) *http.Request {
	request := httptest.NewRequest(method, target, nil)
	request.Host = host
	request.RemoteAddr = remote
	return request
}

// forwardedJSONRequest builds a forwarded request with a JSON body, the way
// the console posts a sign-in through a tunnel.
func forwardedJSONRequest(target, host, body string) *http.Request {
	request := httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
	request.Host = host
	request.RemoteAddr = "127.0.0.1:54321"
	request.Header.Set("Content-Type", "application/json")
	return request
}

func TestExternalConsoleRefusesAForwardedRequestWhenOff(t *testing.T) {
	handler := externalConsole(t, false)

	t.Run("a console request is refused", func(t *testing.T) {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, consoleRequest(http.MethodGet, "/api/v1/status", "relo.ngrok-free.app"))
		if response.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", response.Code)
		}
	})

	t.Run("the session endpoint is refused too", func(t *testing.T) {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, consoleRequest(http.MethodGet, "/api/v1/auth/session", "relo.ngrok-free.app"))
		if response.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", response.Code)
		}
	})
}

func TestExternalConsoleServesLocalRequestsKeyless(t *testing.T) {
	handler := externalConsole(t, true)

	t.Run("this machine is keyless", func(t *testing.T) {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, consoleRequest(http.MethodGet, "/api/v1/status", "127.0.0.1:10101"))
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", response.Code)
		}
	})

	t.Run("the local network is keyless", func(t *testing.T) {
		request := networkRequest(http.MethodGet, "/api/v1/status", "192.168.1.10:10101", "192.168.1.20:44321")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", response.Code)
		}
	})

	t.Run("a container network is keyless", func(t *testing.T) {
		request := networkRequest(http.MethodGet, "/api/v1/status", "172.17.0.1:10101", "172.17.0.2:55000")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", response.Code)
		}
	})
}

func TestExternalConsoleRequiresASignInForForwardedRequests(t *testing.T) {
	handler := externalConsole(t, true)
	const host = "relo.ngrok-free.app"

	t.Run("an API request is told the session is missing", func(t *testing.T) {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, consoleRequest(http.MethodGet, "/api/v1/status", host))
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", response.Code)
		}
	})

	t.Run("a page is sent to the sign-in", func(t *testing.T) {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, consoleRequest(http.MethodGet, "/", host))
		if response.Code != http.StatusSeeOther {
			t.Fatalf("status = %d, want 303", response.Code)
		}
		if location := response.Header().Get("Location"); !strings.HasPrefix(location, server.LoginPath) {
			t.Fatalf("Location = %q, want the sign-in page", location)
		}
	})

	t.Run("the session endpoint asks for the sign-in", func(t *testing.T) {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, consoleRequest(http.MethodGet, "/api/v1/auth/session", host))
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", response.Code)
		}
		report := sessionReport{}
		if err := json.Unmarshal(response.Body.Bytes(), &report); err != nil {
			t.Fatalf("decode the session: %v", err)
		}
		if report.Authenticated || !report.LoginRequired {
			t.Fatalf("session = %+v, want the sign-in asked", report)
		}
	})

	t.Run("the admin token passes a forwarded request", func(t *testing.T) {
		request := consoleRequest(http.MethodGet, "/api/v1/status", host)
		request.Header.Set("Authorization", "Bearer "+adminToken)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", response.Code)
		}
	})

	t.Run("a forwarded browser signs in and then reads", func(t *testing.T) {
		login := forwardedJSONRequest("/api/v1/auth/login", host, `{"admin_token":"`+adminToken+`"}`)
		login.Header.Set("Origin", "https://"+host)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, login)
		if response.Code != http.StatusOK {
			t.Fatalf("sign-in status = %d, body = %s", response.Code, response.Body.String())
		}
		session := cookieOf(response, server.SessionCookieName)
		if session == "" {
			t.Fatal("signing in set no session cookie")
		}

		read := consoleRequest(http.MethodGet, "/api/v1/status", host)
		read.AddCookie(&http.Cookie{Name: server.SessionCookieName, Value: session})
		readResponse := httptest.NewRecorder()
		handler.ServeHTTP(readResponse, read)
		if readResponse.Code != http.StatusOK {
			t.Fatalf("status with a session = %d, want 200", readResponse.Code)
		}
	})

	t.Run("a public forwarded client is not local", func(t *testing.T) {
		request := consoleRequest(http.MethodGet, "/api/v1/status", "127.0.0.1:10101")
		request.Header.Set("X-Forwarded-For", "203.0.113.7, 127.0.0.1")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", response.Code)
		}
	})

	t.Run("a private forwarded client stays local", func(t *testing.T) {
		request := consoleRequest(http.MethodGet, "/api/v1/status", "127.0.0.1:10101")
		request.Header.Set("X-Forwarded-For", "192.168.1.20")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", response.Code)
		}
	})
}
