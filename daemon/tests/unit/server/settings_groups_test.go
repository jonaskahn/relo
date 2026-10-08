package server_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/config"
	"github.com/jonaskahn/relo/internal/inference"
)

// wireSettings mirrors the settings object one response carries. The
// application types no longer carry wire tags, so tests read the wire
// through this local shape instead of through them.
type wireSettings struct {
	System         wireSystem    `json:"system"`
	Server         wireServer    `json:"server"`
	Access         wireAccess    `json:"access"`
	RestartPending bool          `json:"restart_pending"`
	Language       string        `json:"language"`
	Retention      wireRetain    `json:"retention"`
	Appearance     wireAppear    `json:"appearance"`
	Providers      wireProviders `json:"providers"`
	Secrets        wireSecrets   `json:"secrets"`
}

type wireSystem struct {
	Autostart       bool   `json:"autostart"`
	LogLevel        string `json:"log_level"`
	UpdatesURL      string `json:"updates_url"`
	UpdatesDownload string `json:"updates_download"`
}

type wireServer struct {
	Bind          string `json:"bind"`
	Port          int    `json:"port"`
	OpenAIPort    int    `json:"openai_port"`
	AnthropicPort int    `json:"anthropic_port"`
	GeminiPort    int    `json:"gemini_port"`
}

type wireAccess struct {
	AllowExternal bool `json:"allow_external"`
	Login         bool `json:"login"`
}

type wireRetain struct {
	UsageDays int   `json:"usage_days"`
	MaxEvents int64 `json:"max_events"`
	MaxBytes  int64 `json:"max_bytes"`
}

type wireAppear struct {
	Theme        string `json:"theme"`
	Accent       string `json:"accent"`
	QuotaDisplay string `json:"quota_display"`
}

type wireProviders struct {
	CatalogURL              string   `json:"catalog_url"`
	ProxyURL                string   `json:"proxy_url"`
	TimeoutSeconds          int      `json:"timeout_seconds"`
	RetryBackoff            [][2]int `json:"retry_backoff"`
	FailoverCooldownSeconds int      `json:"failover_cooldown_seconds"`
}

type wireSecrets struct {
	Keychain bool   `json:"keychain"`
	KeyFile  string `json:"key_file"`
}

// decodeSettings reads the settings object one response carries.
func decodeSettings(t *testing.T, recorder interface{ Bytes() []byte }) wireSettings {
	t.Helper()
	var settings wireSettings
	if err := json.Unmarshal(recorder.Bytes(), &settings); err != nil {
		t.Fatalf("decode settings: %v", err)
	}
	return settings
}

// decodeGroup reads the one group a PATCH response carries.
func decodeGroup[T any](t *testing.T, recorder interface{ Bytes() []byte }) T {
	t.Helper()
	var value T
	if err := json.Unmarshal(recorder.Bytes(), &value); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return value
}

// TestSettingsExposeEveryGroup covers the read the page starts from: every
// group is present, the read-only vault and sign-in choices are shown, and a
// running daemon whose file matches its boot reports no restart pending.
func TestSettingsExposeEveryGroup(t *testing.T) {
	h := newHarness(t)

	read := h.management(http.MethodGet, "/api/v1/settings", adminToken, nil)
	if read.Code != http.StatusOK {
		t.Fatalf("GET settings status = %d, body = %s", read.Code, read.Body.String())
	}
	settings := decodeSettings(t, read.Body)
	if settings.System.Autostart != config.DefaultAutostart || settings.System.LogLevel != config.DefaultLogLevel {
		t.Fatalf("system = %+v, want the defaults", settings.System)
	}
	if settings.System.UpdatesURL != config.DefaultUpdatesURL || settings.System.UpdatesDownload != config.DefaultUpdatesDownload {
		t.Fatalf("system updates = %+v, want the defaults", settings.System)
	}
	if settings.Server.Bind != h.cfg.Server.Bind || settings.Server.Port != h.cfg.Server.Port {
		t.Fatalf("server = %+v, want the boot configuration", settings.Server)
	}
	if !settings.Access.Login {
		t.Fatal("access.login = false, want the stored sign-in choice shown")
	}
	if settings.RestartPending {
		t.Fatal("restart_pending = true for a file that matches the boot configuration")
	}
}

// TestSystemPatchStoresTheDaemonChoices covers the daemon card: the choices
// round-trip, a restart is reported once the log level changes, and a level
// nothing draws is refused.
func TestSystemPatchStoresTheDaemonChoices(t *testing.T) {
	h := newHarness(t)

	patch := `{"autostart":true,"log_level":"debug",` +
		`"updates_url":"https://example.test/appcast.xml","updates_download":"https://example.test/releases"}`
	saved := h.management(http.MethodPatch, "/api/v1/settings/system", adminToken, strings.NewReader(patch))
	if saved.Code != http.StatusOK {
		t.Fatalf("PATCH system status = %d, body = %s", saved.Code, saved.Body.String())
	}
	if got := decodeGroup[wireSystem](t, saved.Body); got.LogLevel != "debug" ||
		got.UpdatesURL != "https://example.test/appcast.xml" || got.UpdatesDownload != "https://example.test/releases" {
		t.Fatalf("system = %+v, want the saved choices", got)
	}
	read := h.management(http.MethodGet, "/api/v1/settings", adminToken, nil)
	settings := decodeSettings(t, read.Body)
	if settings.System.LogLevel != "debug" {
		t.Fatalf("system = %+v, want the saved level", settings.System)
	}
	if !settings.RestartPending {
		t.Fatal("restart_pending = false after the log level changed from the boot configuration")
	}

	invalid := h.management(http.MethodPatch, "/api/v1/settings/system", adminToken,
		strings.NewReader(`{"autostart":true,"log_level":"trace","updates_url":"","updates_download":""}`))
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("invalid level status = %d, want 400 (body %s)", invalid.Code, invalid.Body.String())
	}
}

// TestNetworkPatchStoresTheListeners covers the network card: the listeners
// round-trip, a restart is reported, and an impossible listener is refused.
func TestNetworkPatchStoresTheListeners(t *testing.T) {
	h := newHarness(t)

	patch := `{"bind":"127.0.0.1","port":12000,"openai_port":12001,"anthropic_port":12002,"gemini_port":0}`
	saved := h.management(http.MethodPatch, "/api/v1/settings/network", adminToken, strings.NewReader(patch))
	if saved.Code != http.StatusOK {
		t.Fatalf("PATCH network status = %d, body = %s", saved.Code, saved.Body.String())
	}
	want := wireServer{
		Bind: "127.0.0.1", Port: 12000, OpenAIPort: 12001, AnthropicPort: 12002, GeminiPort: 0,
	}
	if got := decodeGroup[wireServer](t, saved.Body); got != want {
		t.Fatalf("server = %+v, want %+v", got, want)
	}
	read := h.management(http.MethodGet, "/api/v1/settings", adminToken, nil)
	if settings := decodeSettings(t, read.Body); settings.Server != want || !settings.RestartPending {
		t.Fatalf("settings = %+v, want the stored listeners and a pending restart", settings)
	}

	duplicate := `{"bind":"127.0.0.1","port":12000,"openai_port":12000,"anthropic_port":12002,"gemini_port":0}`
	refused := h.management(http.MethodPatch, "/api/v1/settings/network", adminToken, strings.NewReader(duplicate))
	if refused.Code != http.StatusBadRequest {
		t.Fatalf("duplicate port status = %d, want 400 (body %s)", refused.Code, refused.Body.String())
	}
}

// TestProvidersPatchStoresTheDirectoryAndWaits covers the providers card: the
// proxy, model directory, call wait, and retry waits round-trip, and an
// unusable value is refused.
func TestProvidersPatchStoresTheDirectoryAndWaits(t *testing.T) {
	h := newHarness(t)

	patch := `{"catalog_url":"https://models.example.test/api.json","proxy_url":"https://proxy.example:8443",` +
		`"timeout_seconds":600,"retry_backoff":[[2,4],[4,6],[6,8]],"failover_cooldown_seconds":1800}`
	saved := h.management(http.MethodPatch, "/api/v1/settings/providers", adminToken, strings.NewReader(patch))
	if saved.Code != http.StatusOK {
		t.Fatalf("PATCH providers status = %d, body = %s", saved.Code, saved.Body.String())
	}
	got := decodeGroup[wireProviders](t, saved.Body)
	if got.CatalogURL != "https://models.example.test/api.json" || got.ProxyURL != "https://proxy.example:8443" ||
		got.TimeoutSeconds != 600 || len(got.RetryBackoff) != 3 || got.FailoverCooldownSeconds != 1800 {
		t.Fatalf("providers = %+v, want the saved choices", got)
	}

	none := h.management(http.MethodPatch, "/api/v1/settings/providers", adminToken,
		strings.NewReader(`{"catalog_url":"https://models.example.test/api.json","proxy_url":"","timeout_seconds":300,`+
			`"retry_backoff":[[1,3],[3,5],[5,10]],"failover_cooldown_seconds":0}`))
	if none.Code != http.StatusOK {
		t.Fatalf("PATCH providers status = %d, body = %s, want no denylist stored", none.Code, none.Body.String())
	}
	if got := decodeGroup[wireProviders](t, none.Body); got.FailoverCooldownSeconds != 0 {
		t.Fatalf("providers = %+v, want no denylist stored", got)
	}

	for name, body := range map[string]string{
		"empty catalog": `{"catalog_url":"","proxy_url":"","timeout_seconds":300,"retry_backoff":[[1,3],[3,5],[5,10]]}`,
		"bad catalog":   `{"catalog_url":"ftp://example.test","proxy_url":"","timeout_seconds":300,"retry_backoff":[[1,3],[3,5],[5,10]]}`,
		"bad proxy":     `{"catalog_url":"https://models.example.test/api.json","proxy_url":"socks5://127.0.0.1:1","timeout_seconds":300,"retry_backoff":[[1,3],[3,5],[5,10]]}`,
		"bad timeout":   `{"catalog_url":"https://models.example.test/api.json","proxy_url":"","timeout_seconds":150,"retry_backoff":[[1,3],[3,5],[5,10]]}`,
		"bad backoff":   `{"catalog_url":"https://models.example.test/api.json","proxy_url":"","timeout_seconds":300,"retry_backoff":[[3,1],[3,5],[5,10]],"failover_cooldown_seconds":900}`,
		"bad cooldown":  `{"catalog_url":"https://models.example.test/api.json","proxy_url":"","timeout_seconds":300,"retry_backoff":[[1,3],[3,5],[5,10]],"failover_cooldown_seconds":301}`,
	} {
		refused := h.management(http.MethodPatch, "/api/v1/settings/providers", adminToken, strings.NewReader(body))
		if refused.Code != http.StatusBadRequest {
			t.Fatalf("%s status = %d, want 400 (body %s)", name, refused.Code, refused.Body.String())
		}
	}
}

// TestNoneFailoverStepKeepsFirstRefusalsInRotation pins the live effect of
// the None step: the first rate-limit refusal leaves the account in rotation
// for the next request instead of parking it, while the repeated refusal
// starts the three-minute wait and parks it for the one after.
func TestNoneFailoverStepKeepsFirstRefusalsInRotation(t *testing.T) {
	h := newHarness(t)
	h.upstream.status = http.StatusTooManyRequests
	h.server = h.newServerWithCredentials(h.cfg.Server.Port, defaultEntry())

	saved := h.management(http.MethodPatch, "/api/v1/settings/providers", adminToken,
		strings.NewReader(`{"catalog_url":"https://models.example.test/api.json","proxy_url":"","timeout_seconds":300,`+
			`"retry_backoff":[[1,3],[3,5],[5,10]],"failover_cooldown_seconds":0}`))
	if saved.Code != http.StatusOK {
		t.Fatalf("PATCH providers status = %d, body = %s", saved.Code, saved.Body.String())
	}

	before := h.upstream.requestCount()
	body := `{"model":"claude-relo-openai--gpt-4o","max_tokens":16,"messages":[{"role":"user","content":"hi"}]}`
	response := h.dataPlaneOn(inference.ProtocolAnthropic, http.MethodPost, "/v1/messages", dataPlaneToken,
		strings.NewReader(body))
	if response.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, body = %s, want the provider's own 429", response.Code, response.Body.String())
	}
	if got := h.upstream.requestCount() - before; got != 1 {
		t.Fatalf("upstream requests = %d, want one send per usable account", got)
	}

	second := h.dataPlaneOn(inference.ProtocolAnthropic, http.MethodPost, "/v1/messages", dataPlaneToken,
		strings.NewReader(body))
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("second status = %d, body = %s, want the refusal again while the account is still in rotation", second.Code, second.Body.String())
	}
	if got := h.upstream.requestCount() - before; got != 2 {
		t.Fatalf("upstream requests = %d, want the next request to serve the refusal again", got)
	}

	third := h.dataPlaneOn(inference.ProtocolAnthropic, http.MethodPost, "/v1/messages", dataPlaneToken,
		strings.NewReader(body))
	if third.Code != http.StatusServiceUnavailable {
		t.Fatalf("third status = %d, body = %s, want the cooling account named", third.Code, third.Body.String())
	}
}

// TestDesktopSettingsAreGone covers the retired desktop card: the app has no
// window and no presence choices, so both the settings group and its endpoint
// are gone.
func TestDesktopSettingsAreGone(t *testing.T) {
	h := newHarness(t)

	read := h.management(http.MethodGet, "/api/v1/settings", adminToken, nil)
	var got map[string]any
	if err := json.Unmarshal(read.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode settings: %v", err)
	}
	if _, present := got["desktop"]; present {
		t.Fatalf("settings = %v, want no desktop group", got)
	}

	patch := h.management(http.MethodPatch, "/api/v1/settings/desktop", adminToken,
		strings.NewReader(`{"hide":true}`))
	if patch.Code != http.StatusNotFound {
		t.Fatalf("PATCH settings/desktop status = %d, body = %s, want 404", patch.Code, patch.Body.String())
	}
}

// TestAccessPatchStoresTheExternalChoice covers the access card: the switch
// round-trips and a request that names nothing is refused.
func TestAccessPatchStoresTheExternalChoice(t *testing.T) {
	h := newHarness(t)

	saved := h.management(http.MethodPatch, "/api/v1/settings/access", adminToken,
		strings.NewReader(`{"allow_external":true}`))
	if saved.Code != http.StatusOK {
		t.Fatalf("PATCH access status = %d, body = %s", saved.Code, saved.Body.String())
	}
	read := h.management(http.MethodGet, "/api/v1/settings", adminToken, nil)
	settings := decodeSettings(t, read.Body)
	if !settings.Access.AllowExternal || !settings.Access.Login {
		t.Fatalf("access = %+v, want external access on and the sign-in shown", settings.Access)
	}

	empty := h.management(http.MethodPatch, "/api/v1/settings/access", adminToken, strings.NewReader(`{}`))
	if empty.Code != http.StatusBadRequest {
		t.Fatalf("empty access patch status = %d, want 400 (body %s)", empty.Code, empty.Body.String())
	}
}
