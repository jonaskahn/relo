package startup_test

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jonaskahn/relo/internal/inference"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/zalando/go-keyring"

	"github.com/jonaskahn/relo/internal/adapters/secrets"
	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	"github.com/jonaskahn/relo/internal/config"
	"github.com/jonaskahn/relo/internal/platform"
	"github.com/jonaskahn/relo/internal/server"
	"github.com/jonaskahn/relo/tests/testkit"
)

func TestDaemonLifecycle(t *testing.T) {
	keyring.MockInitWithError(errors.New("no keychain in tests"))
	t.Cleanup(keyring.MockInit)

	home := testkit.TempHome(t)
	port := freePort(t)
	dataPlanePort := freePort(t)
	writeConfig(t, home, port, dataPlanePort)
	logger, logBuffer := testkit.TestLogger(t)
	ctx, cancel := context.WithCancel(context.Background())
	serving := make(chan error, 1)
	go func() {
		serving <- platform.Serve(ctx, platform.Options{
			Home:     home,
			Port:     port,
			Logger:   logger,
			Headless: true,
		})
	}()
	waitForHealthz(t, port)

	t.Run("the state directory holds every file the daemon needs", func(t *testing.T) {
		for _, path := range []string{
			config.ConfigPath(home),
			config.DatabasePath(home),
			server.TokenPath(home, server.AdminTokenFile),
			// The vault key is written on the first start, before any
			// credential exists, so a provider login has somewhere to store.
			secrets.KeyPath(home, secrets.DefaultKeyFile),
		} {
			if _, err := os.Stat(path); err != nil {
				t.Fatalf("state file %s is missing: %v", path, err)
			}
		}
	})

	t.Run("healthz is public and free of diagnostics", func(t *testing.T) {
		response, err := http.Get(healthURL(port))
		if err != nil {
			t.Fatalf("healthz request failed: %v", err)
		}
		defer func() { _ = response.Body.Close() }()
		if response.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", response.StatusCode)
		}
		var payload map[string]any
		if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
			t.Fatalf("decode healthz: %v", err)
		}
		if len(payload) != 2 {
			t.Fatalf("healthz = %v, want only status and version", payload)
		}
	})

	t.Run("the data plane rejects requests without the token", func(t *testing.T) {
		response := postJSON(t, dataPlanePort, "/v1/chat/completions", "", "{}")
		if response.StatusCode != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", response.StatusCode)
		}
		_ = response.Body.Close()
	})

	t.Run("the management listener names the port each protocol moved to", func(t *testing.T) {
		response := postJSON(t, port, "/v1/chat/completions", "", "{}")
		defer func() { _ = response.Body.Close() }()
		if response.StatusCode != http.StatusNotFound {
			t.Fatalf("status = %d, want the retired-path refusal", response.StatusCode)
		}
		var payload struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
			t.Fatalf("decode the refusal: %v", err)
		}
		want := "http://127.0.0.1:" + strconv.Itoa(dataPlanePort) + "/v1"
		if !strings.Contains(payload.Error.Message, want) {
			t.Fatalf("message = %q, want it to name %s", payload.Error.Message, want)
		}
	})

	t.Run("status reports the running daemon", func(t *testing.T) {
		token := readToken(t, home, server.AdminTokenFile)
		request, err := http.NewRequest(http.MethodGet, baseURL(port)+"/api/v1/status", nil)
		if err != nil {
			t.Fatalf("build status request: %v", err)
		}
		request.Header.Set("Authorization", "Bearer "+token)
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatalf("status request failed: %v", err)
		}
		defer func() { _ = response.Body.Close() }()
		if response.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", response.StatusCode)
		}
		var report map[string]any
		if err := json.NewDecoder(response.Body).Decode(&report); err != nil {
			t.Fatalf("decode status: %v", err)
		}
		if report["status"] != "running" || report["schema_version"].(float64) != float64(sqlite.LatestSchemaVersion()) {
			t.Fatalf("report = %v, want a migrated running daemon", report)
		}
		if report["secret_mode"] != string(secrets.ModeEncryptedFile) {
			t.Fatalf("secret_mode = %v, want the encrypted store", report["secret_mode"])
		}
		if report["addr"] != "127.0.0.1:"+strconv.Itoa(port) {
			t.Fatalf("addr = %v, want the bound address", report["addr"])
		}
		dataPlane, ok := report["data_plane"].([]any)
		if !ok || len(dataPlane) != 1 {
			t.Fatalf("data_plane = %v, want the one protocol this home serves", report["data_plane"])
		}
		address, _ := dataPlane[0].(map[string]any)
		if address["protocol"] != inference.ProtocolOpenAI ||
			address["base_url"] != "http://127.0.0.1:"+strconv.Itoa(dataPlanePort) {
			t.Fatalf("data_plane entry = %v, want the OpenAI-compatible port", address)
		}
	})

	t.Run("the headless daemon publishes where it listens", func(t *testing.T) {
		published, found := platform.ReadRuntime(home)
		if !found {
			t.Fatalf("no runtime file at %s", platform.RuntimePath(home))
		}
		if want := "127.0.0.1:" + strconv.Itoa(port); published.Address != want {
			t.Fatalf("runtime address = %q, want %q", published.Address, want)
		}
		if published.InstanceID == "" {
			t.Fatal("the runtime file names no instance")
		}
		if published.PID <= 0 {
			t.Fatalf("runtime pid = %d, want the daemon process", published.PID)
		}
	})

	t.Run("the vault key is written before any credential exists", func(t *testing.T) {
		if _, err := os.Stat(secrets.KeyPath(home, secrets.DefaultKeyFile)); err != nil {
			t.Fatalf("the vault key is missing: %v", err)
		}
		if !strings.Contains(logBuffer.String(), "created secret key") {
			t.Fatalf("log = %s, want the new key reported", logBuffer.String())
		}
	})

	t.Run("shutdown stops the daemon cleanly", func(t *testing.T) {
		cancel()
		select {
		case err := <-serving:
			if err != nil {
				t.Fatalf("Serve() error = %v", err)
			}
		case <-time.After(15 * time.Second):
			t.Fatal("Serve() did not return after the context was cancelled")
		}
		// A daemon that stopped cleanly takes its published address with it,
		// so the next start does not read a stale one.
		if _, err := os.Stat(platform.RuntimePath(home)); err == nil {
			t.Fatalf("the runtime file at %s outlived the daemon", platform.RuntimePath(home))
		}
		for _, want := range []string{"daemon stopping", "daemon stopped"} {
			if !strings.Contains(logBuffer.String(), want) {
				t.Fatalf("log = %s, want it to contain %q", logBuffer.String(), want)
			}
		}
	})
}

func baseURL(port int) string {
	return "http://127.0.0.1:" + strconv.Itoa(port)
}

// writeConfig gives the daemon under test ports this machine can actually
// bind, so a development daemon holding the defaults never makes the suite
// fail on a collision.
func writeConfig(t *testing.T, home string, port, dataPlanePort int) {
	t.Helper()
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatalf("create the state directory: %v", err)
	}
	content := "[server]\nbind = \"127.0.0.1\"\nport = " + strconv.Itoa(port) +
		"\n\n[server.data_plane]\nopenai = " + strconv.Itoa(dataPlanePort) +
		"\nanthropic = 0\ngemini = 0\n"
	if err := os.WriteFile(config.ConfigPath(home), []byte(content), 0o600); err != nil {
		t.Fatalf("write the development config: %v", err)
	}
}

func healthURL(port int) string {
	return baseURL(port) + "/healthz"
}

func postJSON(t *testing.T, port int, path, token, body string) *http.Response {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, baseURL(port)+path, strings.NewReader(body))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("request to %s failed: %v", path, err)
	}
	return response
}

func readToken(t *testing.T, home, name string) string {
	t.Helper()
	token, err := os.ReadFile(server.TokenPath(home, name))
	if err != nil {
		t.Fatalf("read token %s: %v", name, err)
	}
	return strings.TrimSpace(string(token))
}

func freePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("find a free port: %v", err)
	}
	defer func() { _ = listener.Close() }()
	return listener.Addr().(*net.TCPAddr).Port
}

func waitForHealthz(t *testing.T, port int) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		response, err := http.Get(healthURL(port))
		if err == nil {
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("the data plane never became healthy")
}
