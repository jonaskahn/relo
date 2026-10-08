package cli_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
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
	"github.com/jonaskahn/relo/internal/dashboard"
	"github.com/jonaskahn/relo/internal/platform"
	"github.com/jonaskahn/relo/internal/server"
	"github.com/jonaskahn/relo/tests/testkit"
)

func TestServe(t *testing.T) {
	keyring.MockInitWithError(errors.New("no keychain in tests"))
	t.Cleanup(keyring.MockInit)

	t.Run("serve runs until the context is cancelled", func(t *testing.T) {
		home := testkit.TempHome(t)
		useFreePorts(t, home)
		port := freePort(t)
		logger, buffer := testkit.TestLogger(t)

		ctx, cancel := context.WithCancel(context.Background())
		serving := make(chan error, 1)
		go func() {
			serving <- platform.Serve(ctx, platform.Options{
				Home:   home,
				Port:   port,
				Logger: logger,
			})
		}()

		waitForHealthz(t, port)
		report := fetchStatus(t, port, readToken(t, home, server.AdminTokenFile))
		if report["status"] != "running" || report["schema_version"].(float64) != float64(sqlite.LatestSchemaVersion()) {
			t.Fatalf("status = %v, want a migrated running daemon", report)
		}
		if report["secret_mode"] != string(secrets.ModeEncryptedFile) {
			t.Fatalf("secret_mode = %v, want the encrypted store", report["secret_mode"])
		}
		stateFiles := []string{
			config.ConfigPath(home), config.DatabasePath(home),
			server.TokenPath(home, server.AdminTokenFile),
		}
		for _, path := range stateFiles {
			if _, err := os.Stat(path); err != nil {
				t.Fatalf("state file %s is missing: %v", path, err)
			}
		}
		if !strings.Contains(buffer.String(), "relo serving") {
			t.Fatalf("log = %s, want the startup line", buffer.String())
		}

		cancel()
		select {
		case err := <-serving:
			if err != nil {
				t.Fatalf("Serve() error = %v", err)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("Serve() did not return after the context was cancelled")
		}
	})

	t.Run("serve refuses a home with the wrong permissions", func(t *testing.T) {
		home := testkit.TempHome(t)
		if err := os.Chmod(home, 0o755); err != nil {
			t.Fatalf("chmod home: %v", err)
		}
		err := platform.Serve(context.Background(), platform.Options{Home: home})
		if !errors.Is(err, config.ErrHomePerm) {
			t.Fatalf("Serve() error = %v, want %v", err, config.ErrHomePerm)
		}
	})

	t.Run("serve refuses an invalid configuration", func(t *testing.T) {
		home := testkit.TempHome(t)
		if err := os.WriteFile(config.ConfigPath(home), []byte("[server]\nbind = \"localhost\"\n"), 0o600); err != nil {
			t.Fatalf("write config: %v", err)
		}
		if err := platform.Serve(context.Background(), platform.Options{Home: home}); err == nil {
			t.Fatal("Serve() error = nil, want a validation failure")
		}
	})

	t.Run("serve refuses a busy port", func(t *testing.T) {
		home := testkit.TempHome(t)
		useFreePorts(t, home)
		busy := freePort(t)
		listener, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(busy))
		if err != nil {
			t.Fatalf("occupy the port: %v", err)
		}
		defer func() { _ = listener.Close() }()
		err = platform.Serve(context.Background(), platform.Options{Home: home, Port: busy})
		if err == nil {
			t.Fatal("Serve() error = nil, want a bind failure")
		}
	})
}

func waitForHealthz(t *testing.T, port int) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		response, err := http.Get("http://127.0.0.1:" + strconv.Itoa(port) + "/healthz")
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

func readToken(t *testing.T, home, name string) string {
	t.Helper()
	token, err := os.ReadFile(server.TokenPath(home, name))
	if err != nil {
		t.Fatalf("read token %s: %v", name, err)
	}
	return strings.TrimSpace(string(token))
}

func fetchStatus(t *testing.T, port int, token string) map[string]any {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, "http://127.0.0.1:"+strconv.Itoa(port)+"/api/v1/status", nil)
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
	return report
}

// TestServeSpeaksTheLanguage covers the language a daemon started with a
// language answers in: its status reports it, and the console it serves is
// handed the same one.
func TestServeSpeaksTheLanguage(t *testing.T) {
	keyring.MockInitWithError(errors.New("no keychain in tests"))
	t.Cleanup(keyring.MockInit)

	home := testkit.TempHome(t)
	useFreePorts(t, home)
	port := freePort(t)
	logger, _ := testkit.TestLogger(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		_ = platform.Serve(ctx, platform.Options{
			Home: home, Port: port, Logger: logger, Language: "de",
		})
	}()
	waitForHealthz(t, port)

	report := fetchStatus(t, port, readToken(t, home, server.AdminTokenFile))
	if report["language"] != "de" {
		t.Fatalf("status language = %v, want the language the daemon started in", report["language"])
	}

	if !dashboard.Built() {
		return
	}

	response, err := http.Get("http://127.0.0.1:" + strconv.Itoa(port) + "/login")
	if err != nil {
		t.Fatalf("GET /login error = %v", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET /login status = %d, want 200", response.StatusCode)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read the console shell: %v", err)
	}
	if !strings.Contains(string(body), "window.__reloLanguage=\"de\"") {
		t.Fatalf("the console shell does not carry the language: %s", truncateForTest(string(body)))
	}

	// A language stored after the daemon started is what the console the
	// daemon serves next carries, which is how a console change reaches
	// every browser.
	token := readToken(t, home, server.AdminTokenFile)
	settings := strings.NewReader(`{"language":"zh-Hans"}`)
	request, err := http.NewRequest(http.MethodPatch,
		"http://127.0.0.1:"+strconv.Itoa(port)+"/api/v1/settings/language", settings)
	if err != nil {
		t.Fatalf("build the settings request: %v", err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	stored, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("PATCH /api/v1/settings/language error = %v", err)
	}
	_ = stored.Body.Close()
	if stored.StatusCode != http.StatusOK {
		t.Fatalf("PATCH /api/v1/settings/language status = %d, want 200", stored.StatusCode)
	}

	_ = response.Body.Close()
	served, err := http.Get("http://127.0.0.1:" + strconv.Itoa(port) + "/login")
	if err != nil {
		t.Fatalf("GET /login error = %v", err)
	}
	defer func() { _ = served.Body.Close() }()
	body, err = io.ReadAll(served.Body)
	if err != nil {
		t.Fatalf("read the console shell: %v", err)
	}
	if !strings.Contains(string(body), "window.__reloLanguage=\"zh-Hans\"") {
		t.Fatalf("the console shell does not carry the stored language: %s", truncateForTest(string(body)))
	}
}

// truncateForTest keeps a failure readable when the console shell is the
// value under test.
func truncateForTest(body string) string {
	if len(body) <= 200 {
		return body
	}
	return body[:200] + "…"
}
