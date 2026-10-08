// Package management_test drives the daemon the way an operator does: the
// management API, the dashboard, and the event streams of a running Relo.
package management_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/zalando/go-keyring"

	"github.com/jonaskahn/relo/internal/config"
	"github.com/jonaskahn/relo/internal/dashboard"
	"github.com/jonaskahn/relo/internal/platform"
	"github.com/jonaskahn/relo/internal/server"
	"github.com/jonaskahn/relo/tests/testkit"
)

const (
	adminHeader = "Authorization"
	waitTimeout = 10 * time.Second
)

// daemon is one running Relo instance under test.
type daemon struct {
	home   string
	port   int
	token  string
	cancel context.CancelFunc
	done   chan error
}

// startDaemonWithConfig starts one daemon over a home whose config.toml holds
// the given body, so a test asks for the settings it exercises.
func startDaemonWithConfig(t *testing.T, configBody string) *daemon {
	t.Helper()
	keyring.MockInitWithError(errors.New("no keychain in tests"))
	t.Cleanup(keyring.MockInit)

	home := testkit.TempHome(t)
	// A daemon a test starts takes free data plane ports, including when a
	// test supplies other config sections, so it never collides with a daemon.
	openai, anthropic := freePort(t), freePort(t)
	for anthropic == openai {
		anthropic = freePort(t)
	}
	configBody += "\n[server.data_plane]\nopenai = " + strconv.Itoa(openai) +
		"\nanthropic = " + strconv.Itoa(anthropic) + "\n"
	if configBody != "" {
		if err := os.WriteFile(config.ConfigPath(home), []byte(configBody), 0o600); err != nil {
			t.Fatalf("write the config: %v", err)
		}
	}
	port := freePort(t)
	logger, _ := testkit.TestLogger(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- platform.Serve(ctx, platform.Options{
			Home: home, Port: port, Logger: logger,
		})
	}()
	token := readToken(t, home)
	running := &daemon{home: home, port: port, token: token, cancel: cancel, done: done}
	t.Cleanup(func() {
		running.cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("Serve() error = %v", err)
			}
		case <-time.After(waitTimeout):
			t.Error("the daemon never stopped")
		}
	})
	waitForDaemon(t, port, token)
	return running
}

func freePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatalf("close the free port: %v", err)
	}
	return port
}

func readToken(t *testing.T, home string) string {
	t.Helper()
	deadline := time.Now().Add(waitTimeout)
	for time.Now().Before(deadline) {
		token, err := server.EnsureToken(server.TokenPath(home, server.AdminTokenFile))
		if err == nil {
			return token
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the daemon never wrote its admin token")
	return ""
}

func waitForDaemon(t *testing.T, port int, token string) {
	t.Helper()
	deadline := time.Now().Add(waitTimeout)
	client := &http.Client{Timeout: time.Second}
	for time.Now().Before(deadline) {
		request, err := http.NewRequest(http.MethodGet, address(port, "/api/v1/status"), nil)
		if err != nil {
			t.Fatalf("build the status request: %v", err)
		}
		request.Header.Set(adminHeader, "Bearer "+token)
		response, err := client.Do(request)
		if err == nil {
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("the daemon never answered on its listen port")
}

func address(port int, path string) string {
	return "http://127.0.0.1:" + strconv.Itoa(port) + path
}

// call performs one management request and decodes its JSON body.
func (d *daemon) call(t *testing.T, method, path, body string, target any) int {
	t.Helper()
	request, err := http.NewRequest(method, address(d.port, path), strings.NewReader(body))
	if err != nil {
		t.Fatalf("build %s %s: %v", method, path, err)
	}
	request.Header.Set(adminHeader, "Bearer "+d.token)
	request.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: waitTimeout}).Do(request)
	if err != nil {
		t.Fatalf("%s %s failed: %v", method, path, err)
	}
	defer func() { _ = response.Body.Close() }()
	payload, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read %s %s: %v", method, path, err)
	}
	if target != nil && len(payload) > 0 {
		if err := json.Unmarshal(payload, target); err != nil {
			t.Fatalf("decode %s %s: %v (%s)", method, path, err, payload)
		}
	}
	return response.StatusCode
}

func TestManagementAPIDrivesARunningDaemon(t *testing.T) {
	// A daemon that asks for a sign-in exercises both the console redirect and
	// the token the management API takes, and models.dev is pointed at an empty
	// local document so no check leaves the machine.
	catalog := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, "{}")
	}))
	defer catalog.Close()
	configBody := "[admin]\nlogin = true\n[catalog]\nmodelsdev_url = " + strconv.Quote(catalog.URL) + "\n"
	running := startDaemonWithConfig(t, configBody)

	t.Run("status reports the running install", func(t *testing.T) {
		var report struct {
			Status string `json:"status"`
			Schema int    `json:"schema_version"`
		}
		if code := running.call(t, http.MethodGet, "/api/v1/status", "", &report); code != http.StatusOK {
			t.Fatalf("status code = %d, want 200", code)
		}
		if report.Status != "running" || report.Schema == 0 {
			t.Fatalf("report = %+v, want a running install", report)
		}
	})

	t.Run("a provider is added and an account is stored, listed, and removed", func(t *testing.T) {
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			body, err := json.Marshal(map[string]any{"data": []map[string]any{{"id": "local-model", "name": "Local Model"}}})
			if err != nil {
				t.Errorf("encode the upstream list: %v", err)
				return
			}
			_, _ = w.Write(body)
		}))
		defer upstream.Close()

		// A provider only arrives through a probe and the commit that follows it,
		// so this subtest adds one against a local upstream first.
		probe, err := json.Marshal(map[string]any{
			"custom_id":     "local-upstream",
			"label":         "Local Upstream",
			"api_format":    "openai-chat",
			"models_format": "openai",
			"key_header":    "bearer",
			"base_url":      upstream.URL + "/v1",
			"credential":    map[string]any{"kind": "api_key", "secret": "sk-integration-key"},
		})
		if err != nil {
			t.Fatalf("encode the probe: %v", err)
		}
		var probed struct {
			ProbeID string `json:"probe_id"`
			Models  []struct {
				ID     string `json:"id"`
				Source string `json:"source"`
			} `json:"models"`
		}
		if code := running.call(t, http.MethodPost, "/api/v1/connections/probe", string(probe), &probed); code != http.StatusOK {
			t.Fatalf("probe code = %d, want 200", code)
		}
		if len(probed.Models) != 1 || probed.Models[0].Source != "listing" {
			t.Fatalf("probe roster = %+v, want the id the upstream published", probed.Models)
		}

		commit, err := json.Marshal(map[string]any{
			"provider_id":     "local-upstream",
			"label":           "Local Upstream",
			"disabled_models": []string{},
		})
		if err != nil {
			t.Fatalf("encode the commit: %v", err)
		}
		var committed struct {
			ID string `json:"id"`
		}
		if code := running.call(t, http.MethodPost, "/api/v1/connections/probes/"+probed.ProbeID+"/commit", string(commit), &committed); code != http.StatusCreated {
			t.Fatalf("commit code = %d, want 201", code)
		}
		if committed.ID != "local-upstream" {
			t.Fatalf("provider = %q, want the id the operator typed", committed.ID)
		}

		var created struct {
			ID         string `json:"id"`
			SecretMask string `json:"secret_mask"`
		}
		body, err := json.Marshal(map[string]any{
			"provider_id": "local-upstream",
			"label":       "work",
			"secret":      "sk-integration-key-2",
		})
		if err != nil {
			t.Fatalf("encode the account: %v", err)
		}
		if code := running.call(t, http.MethodPost, "/api/v1/accounts", string(body), &created); code != http.StatusCreated {
			t.Fatalf("create code = %d, want 201", code)
		}
		if created.SecretMask == "sk-integration-key" {
			t.Fatalf("the API returned the secret itself: %+v", created)
		}
		var listed struct {
			Accounts []struct {
				ID string `json:"id"`
			} `json:"items"`
		}
		if code := running.call(t, http.MethodGet, "/api/v1/accounts", "", &listed); code != http.StatusOK {
			t.Fatalf("list code = %d, want 200", code)
		}
		// The commit stored the credential the probe tested, so the new one joins it.
		found := false
		for _, account := range listed.Accounts {
			if account.ID == created.ID {
				found = true
			}
		}
		if !found {
			t.Fatalf("accounts = %+v, want the created credential among them", listed.Accounts)
		}
		if code := running.call(t, http.MethodDelete, "/api/v1/accounts/"+created.ID, "", nil); code != http.StatusNoContent {
			t.Fatalf("delete code = %d, want 204", code)
		}
	})

	t.Run("settings round-trip", func(t *testing.T) {
		body := `{"usage_days":14,"max_events":1000,"max_bytes":1048576}`
		var saved struct {
			UsageDays int `json:"usage_days"`
		}
		if code := running.call(t, http.MethodPut, "/api/v1/settings/retention", body, &saved); code != http.StatusOK {
			t.Fatalf("settings code = %d, want 200", code)
		}
		if saved.UsageDays != 14 {
			t.Fatalf("settings = %+v, want the saved budget", saved)
		}
	})

	t.Run("one update downloads the catalog and reads every connection", func(t *testing.T) {
		var report struct {
			Metadata struct {
				Providers int `json:"providers"`
			} `json:"metadata"`
			Providers []map[string]any `json:"providers"`
		}
		if code := running.call(t, http.MethodPost, "/api/v1/catalog/refresh", "", &report); code != http.StatusOK {
			t.Fatalf("catalog refresh code = %d, want 200", code)
		}
		// The local catalog document is empty, and the empty answer is what
		// makes the download a success rather than a failure the update keeps.
		if report.Providers == nil {
			t.Fatal("the report names no connections, want the list a console reads")
		}
	})

	t.Run("the doctor answers", func(t *testing.T) {
		var report struct {
			Checks []struct {
				Name string `json:"name"`
			} `json:"checks"`
		}
		if code := running.call(t, http.MethodGet, "/api/v1/doctor", "", &report); code != http.StatusOK {
			t.Fatalf("doctor code = %d, want 200", code)
		}
		if len(report.Checks) == 0 {
			t.Fatal("the doctor returned no check")
		}
	})

	t.Run("the console signs in over the API", func(t *testing.T) {
		client := &http.Client{
			Timeout: waitTimeout,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
		payload := strings.NewReader(`{"admin_token":"` + running.token + `"}`)
		request, err := http.NewRequest(http.MethodPost, address(running.port, "/api/v1/auth/login"), payload)
		if err != nil {
			t.Fatalf("build the sign-in request: %v", err)
		}
		request.Header.Set("Content-Type", "application/json")
		response, err := client.Do(request)
		if err != nil {
			t.Fatalf("sign-in request failed: %v", err)
		}
		defer func() { _ = response.Body.Close() }()
		if response.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(response.Body)
			t.Fatalf("sign-in status = %d, want 200 (body %s)", response.StatusCode, body)
		}
		session, csrf := dashboardCookies(response)
		if session == "" || csrf == "" {
			t.Fatalf("cookies = %v, want a session and a CSRF token", response.Cookies())
		}
		var issued struct {
			CSRFToken string `json:"csrf_token"`
		}
		if err := json.NewDecoder(response.Body).Decode(&issued); err != nil {
			t.Fatalf("decode the sign-in body: %v", err)
		}
		if issued.CSRFToken != csrf {
			t.Fatalf("body token = %q, cookie = %q, want the same token", issued.CSRFToken, csrf)
		}
		status, body := dashboardFetch(t, running, session)
		if dashboard.Built() {
			if status != http.StatusOK || !bytes.Contains(body, []byte("Relo")) {
				t.Fatalf("console status = %d body = %.80s, want the console shell", status, body)
			}
			return
		}
		if status != http.StatusNotImplemented {
			t.Fatalf("console status = %d, want 501 from a binary without a console build", status)
		}
	})

	t.Run("the dashboard needs a sign-in", func(t *testing.T) {
		client := &http.Client{
			Timeout: waitTimeout,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
		response, err := client.Get(address(running.port, "/"))
		if err != nil {
			t.Fatalf("dashboard request failed: %v", err)
		}
		defer func() { _ = response.Body.Close() }()
		if response.StatusCode != http.StatusSeeOther {
			t.Fatalf("status = %d, want the sign-in redirect", response.StatusCode)
		}
		if location := response.Header.Get("Location"); !strings.HasPrefix(location, "/login") {
			t.Fatalf("Location = %q, want the sign-in page", location)
		}
		if session, _ := dashboardCookies(response); session != "" {
			t.Fatalf("an unauthenticated request minted the session %q", session)
		}
	})

	t.Run("the event stream stays open", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		target := address(running.port, "/api/v1/events/logs")
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
		if err != nil {
			t.Fatalf("build the stream request: %v", err)
		}
		request.Header.Set(adminHeader, "Bearer "+running.token)
		response, err := (&http.Client{}).Do(request)
		if err != nil {
			t.Fatalf("stream request failed: %v", err)
		}
		defer func() { _ = response.Body.Close() }()
		if response.StatusCode != http.StatusOK {
			t.Fatalf("stream status = %d, want 200", response.StatusCode)
		}
		if contentType := response.Header.Get("Content-Type"); !strings.HasPrefix(contentType, "text/event-stream") {
			t.Fatalf("Content-Type = %q, want an event stream", contentType)
		}
		buffer := make([]byte, 64)
		read, err := response.Body.Read(buffer)
		if err != nil && !errors.Is(err, io.EOF) {
			t.Fatalf("read the stream: %v", err)
		}
		if !strings.Contains(string(buffer[:read]), "retry") {
			t.Fatalf("stream preface = %q, want the retry hint", string(buffer[:read]))
		}
	})

	t.Run("an unknown route is a JSON 404", func(t *testing.T) {
		if code := running.call(t, http.MethodGet, "/api/v1/nothing", "", nil); code != http.StatusNotFound {
			t.Fatalf("code = %d, want 404", code)
		}
	})

	t.Run("the management API needs the admin token", func(t *testing.T) {
		client := &http.Client{Timeout: waitTimeout}
		response, err := client.Get(address(running.port, "/api/v1/accounts"))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer func() { _ = response.Body.Close() }()
		if response.StatusCode != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", response.StatusCode)
		}
	})
}

// dashboardCookies returns the session and CSRF cookies of a bootstrap.
func dashboardCookies(response *http.Response) (string, string) {
	session, csrf := "", ""
	for _, cookie := range response.Cookies() {
		switch cookie.Name {
		case server.SessionCookieName:
			session = cookie.Value
		case server.CSRFCookieName:
			csrf = cookie.Value
		}
	}
	return session, csrf
}

// dashboardFetch requests the console root with a session cookie.
func dashboardFetch(t *testing.T, running *daemon, session string) (int, []byte) {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, address(running.port, "/"), nil)
	if err != nil {
		t.Fatalf("build the page request: %v", err)
	}
	request.AddCookie(&http.Cookie{Name: server.SessionCookieName, Value: session})
	response, err := (&http.Client{Timeout: waitTimeout}).Do(request)
	if err != nil {
		t.Fatalf("page request failed: %v", err)
	}
	defer func() { _ = response.Body.Close() }()
	body, _ := io.ReadAll(response.Body)
	return response.StatusCode, body
}
