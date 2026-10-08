package service_test

import (
	"context"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/activity"
	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	appsettings "github.com/jonaskahn/relo/internal/application/settings"
	"github.com/jonaskahn/relo/internal/config"
	"github.com/jonaskahn/relo/internal/i18n"
)

func TestSettingsRoundTrip(t *testing.T) {
	harness := newHarness(t)
	ctx := context.Background()

	empty, err := harness.settings.Read(ctx)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	// An install that never stored a budget keeps the default detail window;
	// totals older than it stay readable in the daily archive.
	if empty.Retention.UsageDays != sqlite.DefaultUsageDays {
		t.Fatalf("Read() = %+v, want the default detail window", empty)
	}
	if _, stored, err := sqlite.NewRetention(harness.db, sqlite.RetentionOptions{}).Config(ctx); err != nil || stored {
		t.Fatalf("a fresh install reported a stored budget: stored = %v, error = %v", stored, err)
	}
	// A file that names nothing reads as the shipped defaults, so the page
	// never shows a zero.
	want := appsettings.SystemSettings{
		Autostart: config.DefaultAutostart, LogLevel: config.DefaultLogLevel,
		UpdatesURL: config.DefaultUpdatesURL, UpdatesDownload: config.DefaultUpdatesDownload,
	}
	if empty.System != want {
		t.Fatalf("Read() system = %+v, want %+v", empty.System, want)
	}

	budget := appsettings.RetentionSettings{UsageDays: 30, MaxEvents: 500_000, MaxBytes: 1 << 30}
	saved, err := harness.settings.SaveRetention(ctx, budget)
	if err != nil {
		t.Fatalf("SaveRetention() error = %v", err)
	}
	if saved != budget {
		t.Fatalf("SaveRetention() = %+v, want %+v", saved, budget)
	}
	stored, found, err := sqlite.NewRetention(harness.db, sqlite.RetentionOptions{}).Config(ctx)
	if err != nil || !found {
		t.Fatalf("stored retention found = %v, error = %v", found, err)
	}
	if stored.UsageDays != 30 || stored.MaxEvents != 500_000 {
		t.Fatalf("stored retention = %+v, want the saved budget", stored)
	}
	if stored.UpdatedAtMs != harness.clock.Now().UnixMilli() {
		t.Fatalf("UpdatedAtMs = %d, want the service clock", stored.UpdatedAtMs)
	}
}

// TestReadShowsEveryStoredChoice covers the groups the settings page shows:
// every one reads from the shared startup file, and the vault and sign-in
// choices it may not edit are shown too.
func TestReadShowsEveryStoredChoice(t *testing.T) {
	harness := newHarness(t)
	ctx := context.Background()
	writeConfigFile(t, harness.home, `[system]
autostart = false

[system.logging]
level = "debug"

[system.updates]
url = "https://example.test/appcast.xml"
download = "https://example.test/releases"

[server]
bind = "0.0.0.0"
port = 12000

[server.data_plane]
openai = 12001
anthropic = 12002
gemini = 12003

[proxy]
url = "https://proxy.example:8443"

[upstream]
timeout_seconds = 600
retry_backoff = [[2, 4], [4, 6], [6, 8]]

[catalog]
modelsdev_url = "https://models.example.test/api.json"

[desktop]
hide = true
window = false
show_on_dock = false
show_on_taskbar = false

[admin]
login = false
allow_external = true

[secrets]
keychain = true
key_file = "/tmp/other.key"
`)
	settings, err := harness.settings.Read(ctx)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if settings.System != (appsettings.SystemSettings{
		Autostart: false, LogLevel: "debug",
		UpdatesURL: "https://example.test/appcast.xml", UpdatesDownload: "https://example.test/releases",
	}) {
		t.Fatalf("system = %+v, want the stored choices", settings.System)
	}
	wantServer := appsettings.ServerSettings{
		Bind: "0.0.0.0", Port: 12000, OpenAIPort: 12001, AnthropicPort: 12002, GeminiPort: 12003,
	}
	if settings.Server != wantServer {
		t.Fatalf("server = %+v, want %+v", settings.Server, wantServer)
	}
	wantProviders := appsettings.ProvidersSettings{
		CatalogURL: "https://models.example.test/api.json", ProxyURL: "https://proxy.example:8443",
		TimeoutSeconds: 600, RetryBackoff: [][2]int{{2, 4}, {4, 6}, {6, 8}},
	}
	if settings.Providers.CatalogURL != wantProviders.CatalogURL ||
		settings.Providers.ProxyURL != wantProviders.ProxyURL ||
		settings.Providers.TimeoutSeconds != wantProviders.TimeoutSeconds ||
		!reflect.DeepEqual(settings.Providers.RetryBackoff, wantProviders.RetryBackoff) {
		t.Fatalf("providers = %+v, want %+v", settings.Providers, wantProviders)
	}
	wantAccess := appsettings.AccessSettings{AllowExternal: true, Login: false}
	if settings.Access != wantAccess {
		t.Fatalf("access = %+v, want %+v", settings.Access, wantAccess)
	}
	wantSecrets := appsettings.SecretsSettings{Keychain: true, KeyFile: "/tmp/other.key"}
	if settings.Secrets != wantSecrets {
		t.Fatalf("secrets = %+v, want %+v", settings.Secrets, wantSecrets)
	}
}

// writeConfigFile stores one startup file in a harness home.
func writeConfigFile(t *testing.T, home, body string) {
	t.Helper()
	if err := os.WriteFile(config.ConfigPath(home), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestSaveProvidersRejectsANonHTTPProxy(t *testing.T) {
	harness := newHarness(t)
	ctx := context.Background()
	current, err := harness.settings.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	current.Providers.ProxyURL = "socks5://127.0.0.1:1080"
	if _, err := harness.settings.SaveProviders(current.Providers); !errors.Is(err, appsettings.ErrInvalidWrite) {
		t.Fatalf("SaveProviders() error = %v, want an invalid write", err)
	}
	current.Providers.ProxyURL = "https://proxy.example:8443"
	saved, err := harness.settings.SaveProviders(current.Providers)
	if err != nil {
		t.Fatal(err)
	}
	if saved.ProxyURL != "https://proxy.example:8443" {
		t.Fatalf("proxy = %q", saved.ProxyURL)
	}
}

// TestSaveProvidersRejectsAnUnusableCatalogURL covers the model directory the
// next fetch reads: an empty or non-HTTP address would leave the catalog with
// nothing to fetch.
func TestSaveProvidersRejectsAnUnusableCatalogURL(t *testing.T) {
	harness := newHarness(t)
	ctx := context.Background()
	current, err := harness.settings.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"", "ftp://models.example.test/api.json", "not a url"} {
		current.Providers.CatalogURL = bad
		if _, err := harness.settings.SaveProviders(current.Providers); !errors.Is(err, appsettings.ErrInvalidWrite) {
			t.Fatalf("SaveProviders(%q) error = %v, want an invalid write", bad, err)
		}
	}
	current.Providers.ProxyURL = "https://proxy.example:8443"
	if _, err := harness.settings.SaveProviders(current.Providers); !errors.Is(err, appsettings.ErrInvalidWrite) {
		t.Fatalf("SaveProviders() error = %v, want an invalid write", err)
	}
	current.Providers.CatalogURL = "https://models.example.test/api.json"
	if _, err := harness.settings.SaveProviders(current.Providers); err != nil {
		t.Fatalf("SaveProviders() error = %v", err)
	}
}

func TestSaveRetentionRejections(t *testing.T) {
	harness := newHarness(t)
	ctx := context.Background()
	cases := []appsettings.RetentionSettings{
		{UsageDays: -1},
		{MaxEvents: -1},
		{MaxBytes: -1},
	}
	for _, budget := range cases {
		if _, err := harness.settings.SaveRetention(ctx, budget); err == nil {
			t.Fatalf("SaveRetention(%+v) returned no error", budget)
		}
	}
	if _, _, err := sqlite.NewRetention(harness.db, sqlite.RetentionOptions{}).Config(ctx); err != nil {
		t.Fatalf("Config() error = %v", err)
	}
}

// TestProxyURLReadsTheStoredProxy covers the one setting the relay reads on
// every upstream request rather than through Read: an install with no proxy
// stored answers empty, and one with a proxy answers the URL it was given.
func TestProxyURLReadsTheStoredProxy(t *testing.T) {
	harness := newHarness(t)
	ctx := context.Background()

	raw, err := harness.settings.ProxyURL()
	if err != nil {
		t.Fatalf("ProxyURL() error = %v", err)
	}
	if raw != "" {
		t.Fatalf("ProxyURL() = %q, want empty before one is stored", raw)
	}

	current, err := harness.settings.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	current.Providers.ProxyURL = "https://proxy.example:8443"
	if _, err := harness.settings.SaveProviders(current.Providers); err != nil {
		t.Fatal(err)
	}
	raw, err = harness.settings.ProxyURL()
	if err != nil {
		t.Fatalf("ProxyURL() error = %v", err)
	}
	if raw != "https://proxy.example:8443" {
		t.Fatalf("ProxyURL() = %q, want the proxy the save stored", raw)
	}
}

// TestUpstreamTimeoutReadsTheStoredPreset covers the one setting the relay
// reads on every attempt rather than through Read: an install with nothing
// stored answers the default, a saved preset persists, and a value outside
// the presets is refused.
func TestUpstreamTimeoutReadsTheStoredPreset(t *testing.T) {
	harness := newHarness(t)
	ctx := context.Background()

	seconds, err := harness.settings.UpstreamTimeout()
	if err != nil {
		t.Fatalf("UpstreamTimeout() error = %v", err)
	}
	if seconds != config.DefaultUpstreamTimeoutSeconds {
		t.Fatalf("UpstreamTimeout() = %d, want the default", seconds)
	}

	current, err := harness.settings.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	current.Providers.TimeoutSeconds = 300
	saved, err := harness.settings.SaveProviders(current.Providers)
	if err != nil {
		t.Fatalf("SaveProviders() error = %v", err)
	}
	if saved.TimeoutSeconds != 300 {
		t.Fatalf("SaveProviders() timeout = %d, want the stored preset", saved.TimeoutSeconds)
	}
	if seconds, err = harness.settings.UpstreamTimeout(); err != nil || seconds != 300 {
		t.Fatalf("UpstreamTimeout() = %d, %v, want the preset the save stored", seconds, err)
	}

	current.Providers.TimeoutSeconds = 150
	if _, err := harness.settings.SaveProviders(current.Providers); !errors.Is(err, appsettings.ErrInvalidWrite) {
		t.Fatalf("SaveProviders() error = %v, want an invalid write", err)
	}
	if seconds, err = harness.settings.UpstreamTimeout(); err != nil || seconds != 300 {
		t.Fatalf("UpstreamTimeout() = %d, %v, want the stored preset after a refused write", seconds, err)
	}
}

// TestUpstreamRetryBackoffReadsTheStoredWindows covers the windows the relay
// draws from on every retry: an install with nothing stored answers the
// default, a saved set persists, and an unusable set is refused.
func TestUpstreamRetryBackoffReadsTheStoredWindows(t *testing.T) {
	harness := newHarness(t)
	ctx := context.Background()

	windows, err := harness.settings.UpstreamRetryBackoff()
	if err != nil {
		t.Fatalf("UpstreamRetryBackoff() error = %v", err)
	}
	if len(windows) != len(config.DefaultUpstreamRetryBackoff()) || windows[0] != config.DefaultUpstreamRetryBackoff()[0] {
		t.Fatalf("UpstreamRetryBackoff() = %v, want the default", windows)
	}

	current, err := harness.settings.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	current.Providers.RetryBackoff = [][2]int{{2, 4}, {4, 6}, {6, 8}}
	saved, err := harness.settings.SaveProviders(current.Providers)
	if err != nil {
		t.Fatalf("SaveProviders() error = %v", err)
	}
	if saved.RetryBackoff[0] != [2]int{2, 4} {
		t.Fatalf("SaveProviders() retry_backoff = %v, want the saved windows", saved.RetryBackoff)
	}
	if windows, err = harness.settings.UpstreamRetryBackoff(); err != nil || windows[0] != [2]int{2, 4} {
		t.Fatalf("UpstreamRetryBackoff() = %v, %v, want the windows the save stored", windows, err)
	}

	current.Providers.RetryBackoff = [][2]int{{2, 4}, {4, 2}, {6, 8}}
	if _, err := harness.settings.SaveProviders(current.Providers); !errors.Is(err, appsettings.ErrInvalidWrite) {
		t.Fatalf("SaveProviders() error = %v, want an invalid write", err)
	}
	if windows, err = harness.settings.UpstreamRetryBackoff(); err != nil || windows[1] != [2]int{4, 6} {
		t.Fatalf("UpstreamRetryBackoff() = %v, %v, want the stored windows after a refused write", windows, err)
	}
}

// TestExternalAccessReadsTheStoredChoice covers the setting the management
// guard reads on every request: an install with nothing stored answers off, a
// saved choice persists, and turning it off outside loopback with the sign-in
// off is refused so the next start still validates.
func TestExternalAccessReadsTheStoredChoice(t *testing.T) {
	harness := newHarness(t)

	enabled, err := harness.settings.AllowExternal()
	if err != nil || enabled {
		t.Fatalf("AllowExternal() = %v, %v, want off before a choice", enabled, err)
	}

	if err := harness.settings.SaveAccess(true); err != nil {
		t.Fatalf("SaveAccess() error = %v", err)
	}
	if enabled, err = harness.settings.AllowExternal(); err != nil || !enabled {
		t.Fatalf("AllowExternal() = %v, %v, want the saved choice", enabled, err)
	}

	// The refused direction: an unguarded console on a non-loopback bind
	// would leave a configuration no start accepts.
	writeConfigFile(t, harness.home, "[server]\nbind = \"0.0.0.0\"\n\n[admin]\nlogin = false\nallow_external = true\n")
	if err := harness.settings.SaveAccess(false); !errors.Is(err, appsettings.ErrInvalidWrite) {
		t.Fatalf("SaveAccess() error = %v, want an invalid write", err)
	}
	if enabled, err = harness.settings.AllowExternal(); err != nil || !enabled {
		t.Fatalf("AllowExternal() = %v, %v, want the stored choice after a refused write", enabled, err)
	}
}

// TestSaveNetworkStoresTheListeners covers where the daemon listens: the
// choices round-trip, and a port another listener already claims is refused.
func TestSaveNetworkStoresTheListeners(t *testing.T) {
	harness := newHarness(t)
	ctx := context.Background()
	want := appsettings.ServerSettings{
		Bind: "127.0.0.1", Port: 12000, OpenAIPort: 12001, AnthropicPort: 12002, GeminiPort: 0,
	}
	saved, err := harness.settings.SaveNetwork(want)
	if err != nil {
		t.Fatalf("SaveNetwork() error = %v", err)
	}
	if saved != want {
		t.Fatalf("SaveNetwork() = %+v, want %+v", saved, want)
	}
	stored, err := harness.settings.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Server != want {
		t.Fatalf("stored server = %+v, want %+v", stored.Server, want)
	}

	duplicate := want
	duplicate.OpenAIPort = duplicate.Port
	if _, err := harness.settings.SaveNetwork(duplicate); !errors.Is(err, appsettings.ErrInvalidWrite) {
		t.Fatalf("SaveNetwork() error = %v, want an invalid write", err)
	}
	if stored, err = harness.settings.Read(ctx); err != nil || stored.Server != want {
		t.Fatalf("server after a refused write = %+v, %v, want %+v", stored.Server, err, want)
	}
}

// TestSaveSystemStoresTheDaemonChoices covers when the daemon runs, how it
// logs, and how it updates. A changed start-at-login choice is mirrored onto
// the platform at once; an unchanged one is left alone.
func TestSaveSystemStoresTheDaemonChoices(t *testing.T) {
	harness := newHarness(t)
	ctx := context.Background()
	var applied []bool
	service := appsettings.New(appsettings.Options{
		Retention:  sqlite.NewRetentionSettings(harness.db, sqlite.RetentionOptions{Now: harness.clock.Now}),
		ConfigPath: config.ConfigPath(harness.home),
		Clock:      harness.clock,
		ApplyAutostart: func(enabled bool) {
			applied = append(applied, enabled)
		},
	})
	current, err := service.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	current.System.LogLevel = "debug"
	current.System.Autostart = !current.System.Autostart
	saved, err := service.SaveSystem(current.System)
	if err != nil {
		t.Fatalf("SaveSystem() error = %v", err)
	}
	if saved != current.System {
		t.Fatalf("SaveSystem() = %+v, want %+v", saved, current.System)
	}
	if len(applied) != 1 || applied[0] != current.System.Autostart {
		t.Fatalf("applied = %v, want one change to %v", applied, current.System.Autostart)
	}
	// Saving again without touching the choice leaves the registration alone.
	if _, err := service.SaveSystem(current.System); err != nil {
		t.Fatal(err)
	}
	if len(applied) != 1 {
		t.Fatalf("applied = %v, want the unchanged choice left alone", applied)
	}
	stored, err := service.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stored.System != current.System {
		t.Fatalf("stored system = %+v, want %+v", stored.System, current.System)
	}

	current.System.LogLevel = "trace"
	if _, err := service.SaveSystem(current.System); !errors.Is(err, appsettings.ErrInvalidWrite) {
		t.Fatalf("SaveSystem() error = %v, want an invalid write", err)
	}
}

// TestRestartPending reports the choices that await a restart: a listener,
// the log level, the update feed, the model directory, the sign-in, and the
// vault. Everything else the page edits takes effect on save.
func TestRestartPending(t *testing.T) {
	harness := newHarness(t)
	ctx := context.Background()
	boot, err := config.ReadFile(config.ConfigPath(harness.home))
	if err != nil {
		t.Fatal(err)
	}
	if pending, err := harness.settings.RestartPending(boot); err != nil || pending {
		t.Fatalf("RestartPending() = %v, %v, want false before any change", pending, err)
	}
	current, err := harness.settings.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	server := current.Server
	server.Port = 12000
	if _, err := harness.settings.SaveNetwork(server); err != nil {
		t.Fatal(err)
	}
	if pending, err := harness.settings.RestartPending(boot); err != nil || !pending {
		t.Fatalf("RestartPending() = %v, %v, want true after a listener is stored", pending, err)
	}

	// A choice that applies on save leaves the marker alone.
	boot, err = config.ReadFile(config.ConfigPath(harness.home))
	if err != nil {
		t.Fatal(err)
	}
	if err := harness.settings.SaveAccess(true); err != nil {
		t.Fatal(err)
	}
	if pending, err := harness.settings.RestartPending(boot); err != nil || pending {
		t.Fatalf("RestartPending() = %v, %v, want false after an access change", pending, err)
	}
}

func TestPreviewRetention(t *testing.T) {
	harness := newHarness(t)
	ctx := context.Background()
	// The harness clock reads 2023-11-14, so a day in October sits outside
	// the three-day floor and is what a count rule may plan against.
	harness.seedUsageOn(t, 12, time.Date(2023, 10, 20, 0, 0, 0, 0, time.UTC))

	preview, err := harness.settings.PreviewRetention(ctx, appsettings.RetentionSettings{MaxEvents: 4})
	if err != nil {
		t.Fatalf("PreviewRetention() error = %v", err)
	}
	if preview.RowsDeleted != 8 {
		t.Fatalf("RowsDeleted = %d, want 8", preview.RowsDeleted)
	}
	if count := countEvents(t, harness, ctx); count != 12 {
		t.Fatalf("usage_events holds %d rows after a preview, want 12", count)
	}

	t.Run("nothing inside the floor is planned", func(t *testing.T) {
		recent := newHarness(t)
		recent.seedUsageOn(t, 6, time.Unix(1_700_000_000, 0))
		preview, err := recent.settings.PreviewRetention(ctx, appsettings.RetentionSettings{MaxEvents: 1, MaxBytes: 1})
		if err != nil {
			t.Fatalf("PreviewRetention() error = %v", err)
		}
		if preview.RowsDeleted != 0 {
			t.Fatalf("RowsDeleted = %d, want the floor to protect every row", preview.RowsDeleted)
		}
	})

	t.Run("invalid budget", func(t *testing.T) {
		_, err := harness.settings.PreviewRetention(ctx, appsettings.RetentionSettings{UsageDays: -1})
		if !errors.Is(err, activity.ErrInvalidRetention) {
			t.Fatalf("PreviewRetention() error = %v, want ErrInvalidRetention", err)
		}
	})
}

func countEvents(t *testing.T, harness *harness, ctx context.Context) int {
	t.Helper()
	var count int
	if err := harness.db.SQL().QueryRowContext(ctx, "SELECT count(*) FROM usage_events").Scan(&count); err != nil {
		t.Fatalf("count usage events: %v", err)
	}
	return count
}

func TestQuotaDisplayLivesInTheDatabase(t *testing.T) {
	harness := newHarness(t)
	ctx := context.Background()
	remaining := config.QuotaDisplayRemaining
	saved, err := harness.settings.SaveAppearance(ctx, appsettings.AppearancePatch{QuotaDisplay: &remaining})
	if err != nil || saved.QuotaDisplay != remaining {
		t.Fatalf("SaveAppearance() = %+v, %v, want remaining", saved, err)
	}
	stored, found, err := sqlite.NewQuotaDisplaySettings(harness.db).QuotaDisplay(ctx)
	if err != nil || !found || stored != remaining {
		t.Fatalf("stored = %q, %v, %v, want remaining", stored, found, err)
	}
	data, err := os.ReadFile(config.ConfigPath(harness.home))
	if err == nil && strings.Contains(string(data), "quota_display") {
		t.Fatalf("config file carries quota_display: %s", data)
	}
	bad := "left"
	if _, err := harness.settings.SaveAppearance(ctx, appsettings.AppearancePatch{QuotaDisplay: &bad}); !errors.Is(err, appsettings.ErrInvalidWrite) {
		t.Fatalf("SaveAppearance() error = %v, want ErrInvalidWrite", err)
	}
	// A patch that names nothing is not a write: the console sends one when an
	// operator edits a field and changes nothing, and it must not clear the
	// appearance that is already stored.
	if _, err := harness.settings.SaveAppearance(ctx, appsettings.AppearancePatch{}); !errors.Is(err, appsettings.ErrInvalidWrite) {
		t.Fatalf("SaveAppearance(empty) error = %v, want ErrInvalidWrite", err)
	}
}

func TestSaveAppearanceRejectsAThemeOrAccentNothingDraws(t *testing.T) {
	harness := newHarness(t)
	ctx := context.Background()

	bad := "chartreuse"
	if _, err := harness.settings.SaveAppearance(ctx, appsettings.AppearancePatch{Theme: &bad}); !errors.Is(err, appsettings.ErrInvalidWrite) {
		t.Fatalf("SaveAppearance(theme) error = %v, want ErrInvalidWrite", err)
	}
	if _, err := harness.settings.SaveAppearance(ctx, appsettings.AppearancePatch{Accent: &bad}); !errors.Is(err, appsettings.ErrInvalidWrite) {
		t.Fatalf("SaveAppearance(accent) error = %v, want ErrInvalidWrite", err)
	}

	// A refused write leaves the stored appearance as it was.
	stored, err := harness.settings.Appearance(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Theme != "system" {
		t.Fatalf("theme = %q, want the stored default after a refused write", stored.Theme)
	}
}

func TestLanguageRoundTrip(t *testing.T) {
	harness := newHarness(t)

	// An install that never chose speaks the operator's browser language,
	// which the daemon names as automatic.
	tag, err := harness.settings.Language()
	if err != nil {
		t.Fatalf("Language() error = %v", err)
	}
	if tag != i18n.Auto {
		t.Fatalf("Language() = %q, want %q before a choice is made", tag, i18n.Auto)
	}

	if err := harness.settings.SaveLanguage("de"); err != nil {
		t.Fatalf("SaveLanguage(de) error = %v", err)
	}
	if tag, err = harness.settings.Language(); err != nil || tag != "de" {
		t.Fatalf("Language() = %q, %v, want the language saved", tag, err)
	}

	// An empty choice means the browser decides again, which is stored as the
	// automatic reading rather than as an empty language.
	if err := harness.settings.SaveLanguage(""); err != nil {
		t.Fatalf("SaveLanguage(empty) error = %v", err)
	}
	if tag, err = harness.settings.Language(); err != nil || tag != i18n.Auto {
		t.Fatalf("Language() = %q, %v, want %q again", tag, err, i18n.Auto)
	}
}

func TestSaveLanguageRefusesALanguageTheConsoleDoesNotShip(t *testing.T) {
	harness := newHarness(t)

	if err := harness.settings.SaveLanguage("sw"); err == nil {
		t.Fatal("SaveLanguage() accepted a language the console does not ship")
	}
	// A refused choice leaves the stored language alone.
	tag, err := harness.settings.Language()
	if err != nil || tag != i18n.Auto {
		t.Fatalf("Language() = %q, %v, want the stored language after a refused write", tag, err)
	}
}

func TestUsageMetricsRoundTrip(t *testing.T) {
	harness := newHarness(t)

	// An install that never chose shows the defaults, in order.
	metrics, err := harness.settings.UsageMetrics()
	if err != nil {
		t.Fatalf("UsageMetrics() error = %v", err)
	}
	if len(metrics) != len(config.DefaultOverviewMetrics()) {
		t.Fatalf("UsageMetrics() = %v, want the defaults", metrics)
	}

	chosen := []string{"requests", "errors", "success_rate", "error_rate", "spend", "duration_avg"}
	stored, err := harness.settings.SaveUsageMetrics(chosen)
	if err != nil {
		t.Fatalf("SaveUsageMetrics() error = %v", err)
	}
	if len(stored) != len(chosen) {
		t.Fatalf("SaveUsageMetrics() = %v, want the choice saved", stored)
	}
	if metrics, err = harness.settings.UsageMetrics(); err != nil || len(metrics) != len(chosen) {
		t.Fatalf("UsageMetrics() = %v, %v, want the saved choice", metrics, err)
	}
}

func TestSaveUsageMetricsRefusesAChoiceTheOverviewCannotShow(t *testing.T) {
	harness := newHarness(t)

	// Too few cards leaves the overview empty, and a card the console has no
	// renderer for would draw nothing at all.
	for name, chosen := range map[string][]string{
		"too few": {"requests", "errors"},
		"too many": {
			"requests", "successes", "errors", "success_rate", "error_rate", "attempts",
			"retried", "tokens_input", "tokens_output", "tokens_total", "spend",
		},
		"unknown":  {"requests", "errors", "success_rate", "error_rate", "spend", "nope"},
		"repeated": {"requests", "errors", "success_rate", "error_rate", "spend", "requests"},
	} {
		if _, err := harness.settings.SaveUsageMetrics(chosen); !errors.Is(err, appsettings.ErrInvalidWrite) {
			t.Fatalf("SaveUsageMetrics(%s) error = %v, want ErrInvalidWrite", name, err)
		}
	}

	// Every refused write left the defaults in place.
	metrics, err := harness.settings.UsageMetrics()
	if err != nil {
		t.Fatal(err)
	}
	if len(metrics) != len(config.DefaultOverviewMetrics()) {
		t.Fatalf("UsageMetrics() = %v, want the defaults after every refused write", metrics)
	}
}
