package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/account"
	"github.com/jonaskahn/relo/internal/server"
)

// instanceUnderTest is the run a headless daemon in these tests published.
const instanceUnderTest = "instance-under-test"

// stopRequest builds the request the lifecycle command sends: from the
// machine itself, carrying the admin token, naming the instance to stop. An
// empty token or instance is left out, which is how a foreign or stale
// caller looks.
func stopRequest(token, instanceID string) *http.Request {
	body := "{\"instance_id\":\"" + instanceID + "\"}"
	request := httptest.NewRequest(http.MethodPost, "/api/v1/daemon/stop", strings.NewReader(body))
	request.RemoteAddr = "127.0.0.1:52311"
	request.Header.Set("Content-Type", "application/json")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	return request
}

// headlessServer builds one server the way the serve command does: with the
// instance it answers for and the hook that ends the run. A nil hook is a
// process that owns the proxy itself, like the tray app.
func headlessServer(t *testing.T, instance string, stopped chan<- struct{}) *harness {
	t.Helper()
	built := newHarness(t)
	built.server = built.newServerWithOptions(built.cfg, []account.PoolEntry{defaultEntry()},
		func(options *server.Options) {
			options.InstanceID = instance
			if stopped != nil {
				options.Shutdown = func() { stopped <- struct{}{} }
			}
		})
	return built
}

func TestDaemonStop(t *testing.T) {
	t.Run("a headless daemon stops for the instance it published", func(t *testing.T) {
		stopped := make(chan struct{}, 1)
		harness := headlessServer(t, instanceUnderTest, stopped)
		recorder := httptest.NewRecorder()
		harness.server.Handler().ServeHTTP(recorder, stopRequest(adminToken, instanceUnderTest))
		if recorder.Code != http.StatusAccepted {
			t.Fatalf("status = %d, want 202", recorder.Code)
		}
		select {
		case <-stopped:
		case <-time.After(5 * time.Second):
			t.Fatal("the daemon was told to stop and never did")
		}
	})

	t.Run("a request naming another instance is refused", func(t *testing.T) {
		stopped := make(chan struct{}, 1)
		harness := headlessServer(t, instanceUnderTest, stopped)
		recorder := httptest.NewRecorder()
		harness.server.Handler().ServeHTTP(recorder, stopRequest(adminToken, "some-other-run"))
		if recorder.Code != http.StatusConflict {
			t.Fatalf("status = %d, want 409 for a stale instance", recorder.Code)
		}
		if got := errorCode(t, recorder); got != "conflict" {
			t.Fatalf("error code = %q, want conflict", got)
		}
		if len(stopped) != 0 {
			t.Fatal("a stop request naming another instance ended this one")
		}
	})

	t.Run("a request without the admin token is refused", func(t *testing.T) {
		stopped := make(chan struct{}, 1)
		harness := headlessServer(t, instanceUnderTest, stopped)
		recorder := httptest.NewRecorder()
		harness.server.Handler().ServeHTTP(recorder, stopRequest("", instanceUnderTest))
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401 without the admin token", recorder.Code)
		}
		if len(stopped) != 0 {
			t.Fatal("a request without the admin token ended the daemon")
		}
	})

	t.Run("a wrong admin token is refused", func(t *testing.T) {
		stopped := make(chan struct{}, 1)
		harness := headlessServer(t, instanceUnderTest, stopped)
		recorder := httptest.NewRecorder()
		harness.server.Handler().ServeHTTP(recorder, stopRequest("not-the-admin-token", instanceUnderTest))
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401 for a wrong token", recorder.Code)
		}
		if len(stopped) != 0 {
			t.Fatal("a wrong admin token ended the daemon")
		}
	})

	t.Run("a request from another machine is refused", func(t *testing.T) {
		stopped := make(chan struct{}, 1)
		harness := headlessServer(t, instanceUnderTest, stopped)
		request := stopRequest(adminToken, instanceUnderTest)
		request.RemoteAddr = "203.0.113.7:4444"
		recorder := httptest.NewRecorder()
		harness.server.Handler().ServeHTTP(recorder, request)
		if recorder.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403 for another machine", recorder.Code)
		}
		if len(stopped) != 0 {
			t.Fatal("a request from another machine ended the daemon")
		}
	})

	t.Run("a process that runs the proxy itself accepts no stop request", func(t *testing.T) {
		harness := headlessServer(t, "", nil)
		recorder := httptest.NewRecorder()
		harness.server.Handler().ServeHTTP(recorder, stopRequest(adminToken, instanceUnderTest))
		if recorder.Code != http.StatusNotImplemented {
			t.Fatalf("status = %d, want 501 from a process that owns the proxy", recorder.Code)
		}
		if got := errorCode(t, recorder); got != "not_implemented" {
			t.Fatalf("error code = %q, want not_implemented", got)
		}
	})
}

// errorCode reads the code of one error response.
func errorCode(t *testing.T, recorder *httptest.ResponseRecorder) string {
	t.Helper()
	var payload struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode the refusal: %v (body %s)", err, recorder.Body.String())
	}
	return payload.Error.Code
}
