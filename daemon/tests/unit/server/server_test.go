package server_test

import (
	"context"
	"encoding/json"
	"github.com/jonaskahn/relo/internal/inference"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	"github.com/jonaskahn/relo/internal/config"
	"github.com/jonaskahn/relo/internal/server"
	"github.com/jonaskahn/relo/tests/testkit"
)

func TestHealthz(t *testing.T) {
	harness := newHarness(t)

	t.Run("healthz returns 200 with status and version without a token", func(t *testing.T) {
		response := harness.dataPlane(http.MethodGet, "/healthz", "", nil)
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", response.Code)
		}
		var payload map[string]any
		if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if len(payload) != 2 || payload["status"] != "ok" || payload["version"] != "test-version" {
			t.Fatalf("body = %v, want only status and version", payload)
		}
	})
}

func TestStatus(t *testing.T) {
	harness := newHarness(t)

	t.Run("status returns server info", func(t *testing.T) {
		response := harness.management(http.MethodGet, "/api/v1/status", adminToken, nil)
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", response.Code)
		}
		report := decodeStatus(t, response)
		if report["status"] != "running" || report["version"] != "test-version" {
			t.Fatalf("report = %v, want the running state", report)
		}
		if report["schema_version"].(float64) != float64(sqlite.LatestSchemaVersion()) {
			t.Fatalf("schema_version = %v, want %d", report["schema_version"], sqlite.LatestSchemaVersion())
		}
		harness.clock.Add(90 * time.Second)
		report = decodeStatus(t, harness.management(http.MethodGet, "/api/v1/status", adminToken, nil))
		if report["uptime_seconds"].(float64) != 90 {
			t.Fatalf("uptime_seconds = %v, want 90", report["uptime_seconds"])
		}
	})

	t.Run("status requires the admin token", func(t *testing.T) {
		response := harness.management(http.MethodGet, "/api/v1/status", "", nil)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", response.Code)
		}
	})

	t.Run("status reports defaults without collaborators", func(t *testing.T) {
		bare := newHarness(t)
		bare.server = serverWithoutRelay(t, bare)
		response := bare.management(http.MethodGet, "/api/v1/status", adminToken, nil)
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", response.Code)
		}
		report := decodeStatus(t, response)
		if report["secret_mode"] != "none" {
			t.Fatalf("secret_mode = %v, want none without a secret store", report["secret_mode"])
		}
		if report["schema_version"].(float64) != 0 {
			t.Fatalf("schema_version = %v, want 0 without a database", report["schema_version"])
		}
	})
}

func TestRoutes(t *testing.T) {
	harness := newHarness(t)

	t.Run("each protocol decodes only its own shape", func(t *testing.T) {
		// An empty body is refused by the codec of the surface that answers
		// it, which is what proves the shape is mounted on that port at all.
		for _, probe := range []struct{ protocol, path string }{
			{inference.ProtocolOpenAI, "/v1/responses"},
			{inference.ProtocolAnthropic, "/v1/messages"},
		} {
			response := harness.dataPlaneOn(probe.protocol, http.MethodPost, probe.path, dataPlaneToken, strings.NewReader("{}"))
			if response.Code != http.StatusBadRequest {
				t.Fatalf("%s %s status = %d, want the codec's 400", probe.protocol, probe.path, response.Code)
			}
		}
	})

	t.Run("a port serves no shape of another protocol", func(t *testing.T) {
		for _, probe := range []struct{ protocol, path string }{
			{inference.ProtocolOpenAI, "/v1/messages"},
			{inference.ProtocolAnthropic, "/v1/chat/completions"},
			{inference.ProtocolAnthropic, "/v1/responses"},
		} {
			response := harness.dataPlaneOn(probe.protocol, http.MethodPost, probe.path, dataPlaneToken, strings.NewReader("{}"))
			if response.Code != http.StatusNotFound {
				t.Fatalf("%s %s status = %d, want 404", probe.protocol, probe.path, response.Code)
			}
		}
	})

	t.Run("the management listener names the port each surface moved to", func(t *testing.T) {
		// The refusal names the configured port, so this server is built with
		// the standard addresses and never started.
		standardCfg := config.DefaultConfig()
		standardCfg.Admin.Login = true
		harness.server = harness.newServerWithConfig(&standardCfg, defaultEntry())
		moved := map[string]string{
			"/v1/chat/completions": "127.0.0.1:10201/v1",
			"/v1/responses":        "127.0.0.1:10201/v1",
			"/v1/messages":         "127.0.0.1:10202/v1",
			"/v1/models":           "127.0.0.1:10201/v1",
			"/agent/v1/responses":  "127.0.0.1:10201/v1",
		}
		for path, want := range moved {
			response := harness.management(http.MethodPost, path, dataPlaneToken, strings.NewReader("{}"))
			if response.Code != http.StatusNotFound {
				t.Fatalf("%s status = %d, want 404", path, response.Code)
			}
			assertErrorShape(t, response, "not_found", "the inference surfaces moved to http://"+want)
		}
	})

	t.Run("a protocol this build does not serve has no handler", func(t *testing.T) {
		for _, protocol := range []string{inference.ProtocolGemini, "nothing"} {
			if _, found := harness.server.DataPlaneHandler(protocol); found {
				t.Fatalf("the server answers for %s", protocol)
			}
		}
	})

	t.Run("a management path is not mistaken for an inference path", func(t *testing.T) {
		response := harness.management(http.MethodGet, "/v1/nothing", dataPlaneToken, nil)
		if response.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want the JSON 404", response.Code)
		}
		assertErrorShape(t, response, "not_found", "no matching route")
	})

	t.Run("the inference surface needs a client key", func(t *testing.T) {
		response := harness.dataPlane(http.MethodPost, "/v1/chat/completions", "", strings.NewReader(streamingBody()))
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", response.Code)
		}
		assertErrorShape(t, response, "unauthorized", "missing or invalid Authorization header")
	})

	t.Run("the inference surface relays a request", func(t *testing.T) {
		response := harness.dataPlane(http.MethodPost, "/v1/chat/completions", dataPlaneToken, strings.NewReader(streamingBody()))
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", response.Code)
		}
		if !strings.Contains(response.Body.String(), "data: [DONE]") {
			t.Fatalf("body = %q, want the relayed stream", response.Body.String())
		}
	})

	t.Run("a page without a session is sent to sign in", func(t *testing.T) {
		response := harness.management(http.MethodGet, "/", "", nil)
		if response.Code != http.StatusSeeOther {
			t.Fatalf("status = %d, want the sign-in redirect", response.Code)
		}
		if location := response.Header().Get("Location"); !strings.HasPrefix(location, "/login") {
			t.Fatalf("Location = %q, want the sign-in page", location)
		}
	})

	t.Run("an unknown path is a dashboard page", func(t *testing.T) {
		response := harness.management(http.MethodGet, "/nope", "", nil)
		if response.Code != http.StatusSeeOther {
			t.Fatalf("status = %d, want the sign-in redirect", response.Code)
		}
		if location := response.Header().Get("Location"); !strings.HasPrefix(location, "/login") {
			t.Fatalf("Location = %q, want the sign-in page", location)
		}
	})

	t.Run("an unknown API path is a JSON 404", func(t *testing.T) {
		response := harness.management(http.MethodGet, "/api/v1/nothing", adminToken, nil)
		if response.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want the JSON 404", response.Code)
		}
		assertErrorShape(t, response, "not_found", "no matching route")
	})

	t.Run("an API path without a session keeps the JSON 401", func(t *testing.T) {
		response := harness.management(http.MethodGet, "/api/v1/accounts", "", nil)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want the 401 the session guard writes", response.Code)
		}
		assertErrorShape(t, response, "unauthorized", "the dashboard session is missing or expired")
	})
}

func TestLoggingMiddleware(t *testing.T) {
	t.Run("logging middleware logs method path status duration", func(t *testing.T) {
		logger, buffer := testkit.TestLogger(t)
		handler := server.LoggingMiddleware(logger, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusTeapot)
		}))
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/brewing", nil))
		logged := buffer.String()
		for _, fragment := range []string{"http request", "GET", "/brewing", "418", "duration_ms"} {
			if !strings.Contains(logged, fragment) {
				t.Fatalf("log = %s, want %s", logged, fragment)
			}
		}
	})

	t.Run("logging middleware keeps streaming flushable", func(t *testing.T) {
		logger, _ := testkit.TestLogger(t)
		recorder := &flushRecorder{}
		handler := server.LoggingMiddleware(logger, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			if _, ok := w.(http.Flusher); !ok {
				t.Error("the wrapped writer cannot flush")
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("data: x\n\n"))
		}))
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/stream", nil))
		if recorder.status != http.StatusOK {
			t.Fatalf("status = %d, want 200", recorder.status)
		}
	})
}

func TestStartAndShutdown(t *testing.T) {
	t.Run("the listener serves on an ephemeral port", func(t *testing.T) {
		harness := newHarness(t)
		harness.server = harness.newServerWithConfig(freeConfig(t), defaultEntry())
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		serving := make(chan error, 1)
		go func() { serving <- harness.server.Start(ctx) }()
		addr := waitForAddress(t, harness.server.Addr)
		if addr == "" {
			t.Fatal("the server bound no address")
		}

		health := getHealth(t, addr)
		if health.StatusCode != http.StatusOK {
			t.Fatalf("healthz status = %d, want 200", health.StatusCode)
		}
		_ = health.Body.Close()
		status := getStatus(t, addr, "")
		if status.StatusCode != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401 without a token", status.StatusCode)
		}
		_ = status.Body.Close()

		cancel()
		if err := <-serving; err != nil {
			t.Fatalf("Start() error = %v", err)
		}
	})

	t.Run("graceful shutdown completes in-flight request", func(t *testing.T) {
		harness := newHarness(t)
		harness.upstream.delayBy(150 * time.Millisecond)
		harness.server = harness.newServerWithConfig(freeConfig(t), defaultEntry())
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		serving := make(chan error, 1)
		go func() { serving <- harness.server.Start(ctx) }()
		dataPlane := waitForDataPlaneAddress(t, harness.server, inference.ProtocolOpenAI)

		response := make(chan *http.Response, 1)
		failure := make(chan error, 1)
		go func() {
			request, err := http.NewRequest(http.MethodPost, "http://"+dataPlane+"/v1/chat/completions", strings.NewReader(streamingBody()))
			if err != nil {
				failure <- err
				return
			}
			request.Header.Set("Authorization", "Bearer "+dataPlaneToken)
			reply, err := http.DefaultClient.Do(request)
			if err != nil {
				failure <- err
				return
			}
			response <- reply
		}()

		time.Sleep(50 * time.Millisecond)
		cancel()
		select {
		case reply := <-response:
			body, err := io.ReadAll(reply.Body)
			if err != nil {
				t.Fatalf("read in-flight response: %v", err)
			}
			_ = reply.Body.Close()
			if !strings.Contains(string(body), "[DONE]") {
				t.Fatalf("body = %q, want the completed stream", body)
			}
		case err := <-failure:
			t.Fatalf("in-flight request failed: %v", err)
		case <-time.After(5 * time.Second):
			t.Fatal("the in-flight request never finished")
		}
		if err := <-serving; err != nil {
			t.Fatalf("Start() error = %v", err)
		}
		if harness.usageRows() != 1 {
			t.Fatalf("usage rows = %d, want the drained request recorded", harness.usageRows())
		}
	})

	t.Run("a second listener on a busy port fails fast", func(t *testing.T) {
		busy := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		t.Cleanup(busy.Close)
		cfg := defaultConfig()
		cfg.Server.Bind = "127.0.0.1"
		cfg.Server.Port = portOf(t, busy.URL)
		logger, _ := testkit.TestLogger(t)
		srv := server.New(server.Options{Config: cfg, Logger: logger})
		if err := srv.Start(context.Background()); err == nil {
			t.Fatal("Start() error = nil, want a bind failure")
		}
	})
}

func getHealth(t *testing.T, address string) *http.Response {
	t.Helper()
	response, err := http.Get("http://" + address + "/healthz")
	if err != nil {
		t.Fatalf("healthz request failed: %v", err)
	}
	return response
}

func getStatus(t *testing.T, address, token string) *http.Response {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, "http://"+address+"/api/v1/status", nil)
	if err != nil {
		t.Fatalf("build status request: %v", err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("status request failed: %v", err)
	}
	return response
}

func decodeStatus(t *testing.T, response *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var report map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &report); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	return report
}

func assertErrorShape(t *testing.T, response *httptest.ResponseRecorder, code, message string) {
	t.Helper()
	var payload struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if payload.Error.Code != code || payload.Error.Message != message {
		t.Fatalf("error body = %+v, want %s / %s", payload, code, message)
	}
}
