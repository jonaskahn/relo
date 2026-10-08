package server_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/account"
	"github.com/jonaskahn/relo/internal/server"
)

func shutdownRequest(token string) *http.Request {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/daemon/shutdown", nil)
	request.RemoteAddr = "127.0.0.1:52311"
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	return request
}

func shutdownServer(t *testing.T, hook func()) *harness {
	t.Helper()
	built := newHarness(t)
	built.server = built.newServerWithOptions(built.cfg, []account.PoolEntry{defaultEntry()},
		func(options *server.Options) {
			options.Shutdown = hook
		})
	return built
}

func TestDaemonShutdown(t *testing.T) {
	t.Run("a headless daemon accepts a shutdown from this machine", func(t *testing.T) {
		called := make(chan struct{}, 1)
		harness := shutdownServer(t, func() { called <- struct{}{} })
		recorder := httptest.NewRecorder()
		harness.server.Handler().ServeHTTP(recorder, shutdownRequest(adminToken))
		if recorder.Code != http.StatusAccepted {
			t.Fatalf("status = %d, want 202", recorder.Code)
		}
		select {
		case <-called:
		case <-time.After(5 * time.Second):
			t.Fatal("the daemon was told to shut down and never did")
		}
	})

	t.Run("a request from another machine is refused", func(t *testing.T) {
		called := make(chan struct{}, 1)
		harness := shutdownServer(t, func() { called <- struct{}{} })
		request := shutdownRequest(adminToken)
		request.RemoteAddr = "203.0.113.7:4444"
		recorder := httptest.NewRecorder()
		harness.server.Handler().ServeHTTP(recorder, request)
		if recorder.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403 for another machine", recorder.Code)
		}
		if len(called) != 0 {
			t.Fatal("a request from another machine shut the daemon down")
		}
	})

	t.Run("a request without the admin token is refused", func(t *testing.T) {
		called := make(chan struct{}, 1)
		harness := shutdownServer(t, func() { called <- struct{}{} })
		recorder := httptest.NewRecorder()
		harness.server.Handler().ServeHTTP(recorder, shutdownRequest(""))
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401 without the admin token", recorder.Code)
		}
		if len(called) != 0 {
			t.Fatal("a request without the admin token shut the daemon down")
		}
	})

	t.Run("a process that owns the proxy accepts no shutdown", func(t *testing.T) {
		harness := shutdownServer(t, nil)
		recorder := httptest.NewRecorder()
		harness.server.Handler().ServeHTTP(recorder, shutdownRequest(adminToken))
		if recorder.Code != http.StatusNotImplemented {
			t.Fatalf("status = %d, want 501 from a process that owns the proxy", recorder.Code)
		}
	})
}
