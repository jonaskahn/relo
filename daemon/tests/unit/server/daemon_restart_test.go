package server_test

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/account"
	"github.com/jonaskahn/relo/internal/server"
)

func restartRequest(path, token string) *http.Request {
	request := httptest.NewRequest(http.MethodPost, path, nil)
	request.RemoteAddr = "127.0.0.1:52311"
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	return request
}

func restartServer(t *testing.T, hook func(force bool)) *harness {
	t.Helper()
	built := newHarness(t)
	built.server = built.newServerWithOptions(built.cfg, []account.PoolEntry{defaultEntry()},
		func(options *server.Options) {
			options.Restart = hook
		})
	return built
}

func TestDaemonRestart(t *testing.T) {
	t.Run("a headless daemon accepts a restart from this machine", func(t *testing.T) {
		var forced atomic.Bool
		called := make(chan struct{}, 1)
		harness := restartServer(t, func(force bool) {
			forced.Store(force)
			called <- struct{}{}
		})
		recorder := httptest.NewRecorder()
		harness.server.Handler().ServeHTTP(recorder, restartRequest("/api/v1/daemon/restart", adminToken))
		if recorder.Code != http.StatusAccepted {
			t.Fatalf("status = %d, want 202", recorder.Code)
		}
		select {
		case <-called:
		case <-time.After(5 * time.Second):
			t.Fatal("the daemon was told to restart and never did")
		}
		if forced.Load() {
			t.Fatal("a graceful restart asked for a force restart")
		}
	})

	t.Run("a force restart is handed to the hook", func(t *testing.T) {
		var forced atomic.Bool
		called := make(chan struct{}, 1)
		harness := restartServer(t, func(force bool) {
			forced.Store(force)
			called <- struct{}{}
		})
		recorder := httptest.NewRecorder()
		harness.server.Handler().ServeHTTP(recorder, restartRequest("/api/v1/daemon/force-restart", adminToken))
		if recorder.Code != http.StatusAccepted {
			t.Fatalf("status = %d, want 202", recorder.Code)
		}
		select {
		case <-called:
		case <-time.After(5 * time.Second):
			t.Fatal("the daemon was told to force-restart and never did")
		}
		if !forced.Load() {
			t.Fatal("a force restart asked for a graceful restart")
		}
	})

	t.Run("a request from another machine is refused", func(t *testing.T) {
		called := make(chan struct{}, 1)
		harness := restartServer(t, func(bool) { called <- struct{}{} })
		request := restartRequest("/api/v1/daemon/restart", adminToken)
		request.RemoteAddr = "203.0.113.7:4444"
		recorder := httptest.NewRecorder()
		harness.server.Handler().ServeHTTP(recorder, request)
		if recorder.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403 for another machine", recorder.Code)
		}
		if len(called) != 0 {
			t.Fatal("a request from another machine restarted the daemon")
		}
	})

	t.Run("a request without the admin token is refused", func(t *testing.T) {
		called := make(chan struct{}, 1)
		harness := restartServer(t, func(bool) { called <- struct{}{} })
		recorder := httptest.NewRecorder()
		harness.server.Handler().ServeHTTP(recorder, restartRequest("/api/v1/daemon/restart", ""))
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401 without the admin token", recorder.Code)
		}
		if len(called) != 0 {
			t.Fatal("a request without the admin token restarted the daemon")
		}
	})

	t.Run("a process that owns the proxy accepts no restart", func(t *testing.T) {
		harness := restartServer(t, nil)
		recorder := httptest.NewRecorder()
		harness.server.Handler().ServeHTTP(recorder, restartRequest("/api/v1/daemon/restart", adminToken))
		if recorder.Code != http.StatusNotImplemented {
			t.Fatalf("status = %d, want 501 from a process that owns the proxy", recorder.Code)
		}
	})
}
