package config_test

import (
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/secrets"
	"github.com/jonaskahn/relo/internal/config"
	"github.com/jonaskahn/relo/internal/inference"
	"github.com/jonaskahn/relo/tests/testkit"
)

func TestLoadConfigValidDefaults(t *testing.T) {
	t.Run("valid defaults", func(t *testing.T) {
		loaded, err := config.LoadConfig(filepath.Join(testkit.TempHome(t), "config.toml"), nil)
		if err != nil {
			t.Fatalf("LoadConfig() error = %v", err)
		}
		// A default config is compared field by field on the settings a load
		// can change; the metric list is a slice and the file holds no key
		// for it until an operator picks one.
		defaults := config.DefaultConfig()
		if loaded.System != defaults.System || loaded.Server != defaults.Server ||
			loaded.Secrets != defaults.Secrets || loaded.Admin != defaults.Admin ||
			loaded.Catalog != defaults.Catalog {
			t.Fatalf("LoadConfig() = %+v, want the defaults", *loaded)
		}
		if loaded.UI.Language != defaults.UI.Language || loaded.UI.Theme != defaults.UI.Theme ||
			loaded.UI.Accent != defaults.UI.Accent {
			t.Fatalf("ui = %+v, want the defaults", loaded.UI)
		}
		if loaded.Server.Port != 10101 || loaded.Server.Bind != "127.0.0.1" {
			t.Fatalf("server = %+v, want the loopback default", loaded.Server)
		}
		if loaded.Secrets.Keychain {
			t.Fatal("the keychain is on by default, want it off")
		}
		if loaded.Secrets.KeyFile != secrets.DefaultKeyFile {
			t.Fatalf("secrets.key_file = %q, want %q", loaded.Secrets.KeyFile, secrets.DefaultKeyFile)
		}
		if loaded.Admin.Login {
			t.Fatal("the admin login is on by default, want it off")
		}
	})

	t.Run("missing file creates default", func(t *testing.T) {
		home := testkit.TempHome(t)
		path := config.ConfigPath(home)
		if _, err := config.LoadConfig(path, nil); err != nil {
			t.Fatalf("LoadConfig() error = %v", err)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("default config was not written: %v", err)
		}
		if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
			t.Fatalf("config mode = %v, want 0600", info.Mode().Perm())
		}
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read the written config: %v", err)
		}
		for _, key := range []string{"[system]", "autostart = true", "key_file", "[admin]", "login = false"} {
			if !strings.Contains(string(content), key) {
				t.Fatalf("the default config does not carry %q: %s", key, content)
			}
		}
		reloaded, err := config.LoadConfig(path, nil)
		if err != nil {
			t.Fatalf("reload error = %v", err)
		}
		if !sameConfig(*reloaded, config.DefaultConfig()) {
			t.Fatalf("reloaded = %+v, want the defaults", *reloaded)
		}
	})

	t.Run("existing file is never rewritten", func(t *testing.T) {
		home := testkit.TempHome(t)
		path := config.ConfigPath(home)
		custom := "[server]\nbind = \"127.0.0.1\"\nport = 12345\n"
		if err := os.WriteFile(path, []byte(custom), 0o600); err != nil {
			t.Fatalf("write config: %v", err)
		}
		loaded, err := config.LoadConfig(path, nil)
		if err != nil {
			t.Fatalf("LoadConfig() error = %v", err)
		}
		if loaded.Server.Port != 12345 {
			t.Fatalf("port = %d, want 12345", loaded.Server.Port)
		}
		if loaded.System.Logging.Level != config.DefaultLogLevel {
			t.Fatalf("log level = %q, want the default", loaded.System.Logging.Level)
		}
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read config: %v", err)
		}
		if string(content) != custom {
			t.Fatalf("config file was rewritten: %s", content)
		}
	})

	t.Run("unknown keys warn not fail", func(t *testing.T) {
		logger, buffer := testkit.TestLogger(t)
		loaded, err := config.LoadConfig(testkit.FixturePath("config", "unknown_keys.toml"), logger)
		if err != nil {
			t.Fatalf("LoadConfig() error = %v", err)
		}
		if loaded.Server.Port != 12000 || loaded.System.Logging.Level != "warn" {
			t.Fatalf("known keys were not applied: %+v", *loaded)
		}
		for _, key := range []string{"retries", "format", "title"} {
			if !strings.Contains(buffer.String(), key) {
				t.Fatalf("no warning for unknown key %q in %s", key, buffer.String())
			}
		}
	})

	t.Run("valid fixture loads", func(t *testing.T) {
		loaded, err := config.LoadConfig(testkit.FixturePath("config", "valid.toml"), nil)
		if err != nil {
			t.Fatalf("LoadConfig() error = %v", err)
		}
		want := config.Config{
			System: config.SystemConfig{
				Autostart: config.DefaultAutostart,
				Logging:   config.LoggingConfig{Level: "debug"},
				Updates:   config.UpdatesConfig{URL: config.DefaultUpdatesURL, Download: config.DefaultUpdatesDownload},
			},
			Server: config.ServerConfig{
				Bind: "127.0.0.1", Port: 12000,
				DataPlane: config.DataPlaneConfig{
					OpenAI: config.DefaultOpenAIPort, Anthropic: config.DefaultAnthropicPort,
					Gemini: config.DefaultGeminiPort,
				},
			},
			Secrets: config.SecretsConfig{Keychain: true, KeyFile: secrets.DefaultKeyFile},
			Admin:   config.AdminConfig{},
			UI: config.UIConfig{
				Language: config.DefaultUILanguage, Theme: config.DefaultUITheme, Accent: config.DefaultUIAccent,
				Usage: config.UsageConfig{OverviewMetrics: config.DefaultOverviewMetrics()},
			},
			Catalog: config.CatalogConfig{ModelsDevURL: config.DefaultModelsDevURL},
			Upstream: config.UpstreamConfig{
				TimeoutSeconds: config.DefaultUpstreamTimeoutSeconds,
				RetryBackoff:   config.DefaultUpstreamRetryBackoff(),
			},
		}
		if !sameConfig(*loaded, want) {
			t.Fatalf("LoadConfig() = %+v, want %+v", *loaded, want)
		}
	})

	t.Run("a retired desktop section is ignored and the file still loads", func(t *testing.T) {
		path := filepath.Join(testkit.TempHome(t), "config.toml")
		if err := os.WriteFile(path, []byte("[desktop]\nhide = true\nwindow = true\nshow_on_dock = false\n"), 0o600); err != nil {
			t.Fatalf("write config: %v", err)
		}
		loaded, err := config.LoadConfig(path, nil)
		if err != nil {
			t.Fatalf("LoadConfig() error = %v", err)
		}
		if !loaded.System.Autostart {
			t.Fatal("autostart = false, want the default true")
		}
	})

	t.Run("a retired top-level autostart false is kept", func(t *testing.T) {
		path := filepath.Join(testkit.TempHome(t), "config.toml")
		if err := os.WriteFile(path, []byte("autostart = false\n"), 0o600); err != nil {
			t.Fatalf("write config: %v", err)
		}
		loaded, err := config.LoadConfig(path, nil)
		if err != nil {
			t.Fatalf("LoadConfig() error = %v", err)
		}
		if loaded.System.Autostart {
			t.Fatal("autostart = true, want the retired key's false")
		}
	})

	t.Run("a retired desktop autostart is honored when system names none", func(t *testing.T) {
		path := filepath.Join(testkit.TempHome(t), "config.toml")
		if err := os.WriteFile(path, []byte("[desktop]\nautostart = false\n"), 0o600); err != nil {
			t.Fatalf("write config: %v", err)
		}
		loaded, err := config.LoadConfig(path, nil)
		if err != nil {
			t.Fatalf("LoadConfig() error = %v", err)
		}
		if loaded.System.Autostart {
			t.Fatal("autostart = true, want the stored desktop false")
		}
	})

	t.Run("the system autostart wins over the retired homes without a warning", func(t *testing.T) {
		logger, buffer := testkit.TestLogger(t)
		path := filepath.Join(testkit.TempHome(t), "config.toml")
		body := "[system]\nautostart = true\n\n[desktop]\nautostart = false\n"
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatalf("write config: %v", err)
		}
		loaded, err := config.LoadConfig(path, logger)
		if err != nil {
			t.Fatalf("LoadConfig() error = %v", err)
		}
		if !loaded.System.Autostart {
			t.Fatal("autostart = false, want the system true")
		}
		if strings.Contains(buffer.String(), "desktop.autostart") {
			t.Fatalf("the retired key warned in %s", buffer.String())
		}
	})

	t.Run("the retired logging and updates tables still answer", func(t *testing.T) {
		logger, buffer := testkit.TestLogger(t)
		path := filepath.Join(testkit.TempHome(t), "config.toml")
		body := "[logging]\nlevel = \"warn\"\n\n[updates]\nurl = \"https://example.test/appcast.xml\"\ndownload = \"https://example.test/releases\"\n"
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatalf("write config: %v", err)
		}
		loaded, err := config.LoadConfig(path, logger)
		if err != nil {
			t.Fatalf("LoadConfig() error = %v", err)
		}
		if loaded.System.Logging.Level != "warn" ||
			loaded.System.Updates.URL != "https://example.test/appcast.xml" ||
			loaded.System.Updates.Download != "https://example.test/releases" {
			t.Fatalf("system = %+v, want the retired tables' values", loaded.System)
		}
		for _, key := range []string{"logging.level", "updates.url", "updates.download"} {
			if strings.Contains(buffer.String(), key) {
				t.Fatalf("the retired key %s warned in %s", key, buffer.String())
			}
		}
	})

	t.Run("the system table parses", func(t *testing.T) {
		path := filepath.Join(testkit.TempHome(t), "config.toml")
		body := "[system]\nautostart = false\n\n[system.logging]\nlevel = \"debug\"\n\n[system.updates]\nurl = \"https://example.test/appcast.xml\"\ndownload = \"https://example.test/releases\"\n"
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatalf("write config: %v", err)
		}
		loaded, err := config.LoadConfig(path, nil)
		if err != nil {
			t.Fatalf("LoadConfig() error = %v", err)
		}
		if loaded.System.Autostart {
			t.Fatal("system.autostart = true, want the file's false")
		}
		if loaded.System.Logging.Level != "debug" {
			t.Fatalf("system.logging.level = %q, want debug", loaded.System.Logging.Level)
		}
		if loaded.System.Updates.URL != "https://example.test/appcast.xml" ||
			loaded.System.Updates.Download != "https://example.test/releases" {
			t.Fatalf("system.updates = %+v, want the file's values", loaded.System.Updates)
		}
	})

	t.Run("the system table wins over the retired logging key", func(t *testing.T) {
		path := filepath.Join(testkit.TempHome(t), "config.toml")
		body := "[system.logging]\nlevel = \"debug\"\n\n[logging]\nlevel = \"warn\"\n"
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatalf("write config: %v", err)
		}
		loaded, err := config.LoadConfig(path, nil)
		if err != nil {
			t.Fatalf("LoadConfig() error = %v", err)
		}
		if loaded.System.Logging.Level != "debug" {
			t.Fatalf("system.logging.level = %q, want the system table's debug", loaded.System.Logging.Level)
		}
	})

	t.Run("malformed file reports parse error", func(t *testing.T) {
		path := filepath.Join(testkit.TempHome(t), "config.toml")
		if err := os.WriteFile(path, []byte("not = = toml"), 0o600); err != nil {
			t.Fatalf("write config: %v", err)
		}
		if _, err := config.LoadConfig(path, nil); err == nil {
			t.Fatal("LoadConfig() error = nil, want a parse error")
		}
	})

	t.Run("a config path that cannot be read returns an error", func(t *testing.T) {
		path := filepath.Join(testkit.TempHome(t), "config.toml")
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatalf("create a directory in place of the config: %v", err)
		}
		if _, err := config.LoadConfig(path, nil); err == nil {
			t.Fatal("LoadConfig() error = nil, want a read failure")
		}
	})

	t.Run("an unwritable home cannot take the default config", func(t *testing.T) {
		path := filepath.Join(testkit.TempHome(t), "absent", "config.toml")
		if _, err := config.LoadConfig(path, nil); err == nil {
			t.Fatal("LoadConfig() error = nil, want a write failure")
		}
	})
}

// TestLoadOffline covers the read a diagnostic and an offline management
// session take: the defaults for a state directory that has never written
// its startup file, the stored values when they exist, and an error for a
// file nothing can read.
func TestLoadOffline(t *testing.T) {
	t.Run("a home without a config reads the defaults and creates nothing", func(t *testing.T) {
		home := testkit.TempHome(t)
		loaded, err := config.LoadOffline(home)
		if err != nil {
			t.Fatalf("LoadOffline() error = %v", err)
		}
		if !sameConfig(loaded, config.DefaultConfig()) {
			t.Fatalf("LoadOffline() = %+v, want the defaults", loaded)
		}
		if _, err := os.Stat(config.ConfigPath(home)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("the offline read wrote %s", config.ConfigPath(home))
		}
	})

	t.Run("a stored config is read as written", func(t *testing.T) {
		home := testkit.TempHome(t)
		body := "[server]\nbind = \"127.0.0.1\"\nport = 12345\n"
		if err := os.WriteFile(config.ConfigPath(home), []byte(body), 0o600); err != nil {
			t.Fatalf("write config: %v", err)
		}
		loaded, err := config.LoadOffline(home)
		if err != nil {
			t.Fatalf("LoadOffline() error = %v", err)
		}
		if loaded.Server.Port != 12345 {
			t.Fatalf("server port = %d, want 12345", loaded.Server.Port)
		}
	})

	t.Run("a malformed config is reported", func(t *testing.T) {
		home := testkit.TempHome(t)
		if err := os.WriteFile(config.ConfigPath(home), []byte("not = = toml"), 0o600); err != nil {
			t.Fatalf("write config: %v", err)
		}
		if _, err := config.LoadOffline(home); err == nil {
			t.Fatal("LoadOffline() error = nil, want a parse error")
		}
	})

	t.Run("a config path that cannot be read is reported", func(t *testing.T) {
		home := testkit.TempHome(t)
		if err := os.Mkdir(config.ConfigPath(home), 0o700); err != nil {
			t.Fatalf("create a directory in place of the config: %v", err)
		}
		if _, err := config.LoadOffline(home); err == nil {
			t.Fatal("LoadOffline() error = nil, want a read failure")
		}
	})
}

// sameConfig reports whether two configurations agree on every field a file
// can set. A configuration holds a metric list, so it is no longer a value
// two of which may be compared with ==.
func sameConfig(got, want config.Config) bool {
	if got.System != want.System || got.Server != want.Server || got.Secrets != want.Secrets || got.Admin != want.Admin {
		return false
	}
	if got.Catalog != want.Catalog || got.Proxy != want.Proxy {
		return false
	}
	if got.Upstream.TimeoutSeconds != want.Upstream.TimeoutSeconds ||
		!slices.Equal(got.Upstream.RetryBackoff, want.Upstream.RetryBackoff) {
		return false
	}
	if got.UI.Language != want.UI.Language || got.UI.Theme != want.UI.Theme || got.UI.Accent != want.UI.Accent {
		return false
	}
	return slices.Equal(got.UI.Usage.OverviewMetrics, want.UI.Usage.OverviewMetrics)
}

func TestValidateConfig(t *testing.T) {
	valid := config.DefaultConfig()
	tests := []struct {
		name    string
		mutate  func(cfg *config.Config)
		wantErr error
		path    string
	}{
		{"defaults are valid", func(*config.Config) {}, nil, ""},
		{"invalid port returns field path", func(cfg *config.Config) { cfg.Server.Port = 70000 }, config.ErrInvalidPort, "server.port"},
		{"invalid bind returns field path", func(cfg *config.Config) { cfg.Server.Bind = "localhost" }, config.ErrInvalidBind, "server.bind"},
		{"invalid log level returns field path", func(cfg *config.Config) { cfg.System.Logging.Level = "trace" }, config.ErrInvalidLogLevel, "system.logging.level"},
		{"a data plane port out of range returns its field path",
			func(cfg *config.Config) { cfg.Server.DataPlane.Anthropic = 70000 }, config.ErrInvalidPort, "server.data_plane.anthropic"},
		{"a negative data plane port returns its field path",
			func(cfg *config.Config) { cfg.Server.DataPlane.OpenAI = -1 }, config.ErrInvalidPort, "server.data_plane.openai"},
		{"a protocol may be left off", func(cfg *config.Config) { cfg.Server.DataPlane.Gemini = 0 }, nil, ""},
		{"login off inside loopback is accepted", func(cfg *config.Config) { cfg.Admin.Login = false }, nil, ""},
		{"an IPv6 loopback bind is accepted", func(cfg *config.Config) { cfg.Server.Bind = "::1" }, nil, ""},
		{"login on inside loopback is accepted", func(cfg *config.Config) { cfg.Admin.Login = true }, nil, ""},
		{"login on outside loopback is accepted",
			func(cfg *config.Config) { cfg.Admin.Login = true; cfg.Server.Bind = "0.0.0.0" }, nil, ""},
		{"login off outside loopback is refused",
			func(cfg *config.Config) { cfg.Admin.Login = false; cfg.Server.Bind = "0.0.0.0" },
			config.ErrLoginOffOutsideLoopback, "admin.login"},
		{"external access allows login off outside loopback",
			func(cfg *config.Config) {
				cfg.Admin.Login = false
				cfg.Admin.AllowExternal = true
				cfg.Server.Bind = "0.0.0.0"
			}, nil, ""},
		{"two protocols may not share a port",
			func(cfg *config.Config) { cfg.Server.DataPlane.Anthropic = cfg.Server.DataPlane.OpenAI },
			config.ErrDuplicatePort, "server.data_plane.anthropic"},
		{"a data plane port may not take the management port",
			func(cfg *config.Config) { cfg.Server.DataPlane.OpenAI = cfg.Server.Port },
			config.ErrDuplicatePort, "server.data_plane.openai"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := valid
			tt.mutate(&cfg)
			err := config.ValidateConfig(&cfg)
			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("ValidateConfig() error = %v, want nil", err)
				}
				return
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("ValidateConfig() error = %v, want %v", err, tt.wantErr)
			}
			if !strings.HasPrefix(err.Error(), tt.path+": ") {
				t.Fatalf("ValidateConfig() error = %q, want the %s field path", err, tt.path)
			}
		})
	}
}

// the configuration, the status API, and the setup steps all read.
func TestDataPlanePorts(t *testing.T) {
	cfg := config.DefaultConfig()

	t.Run("the defaults name one port per protocol", func(t *testing.T) {
		for protocol, want := range map[string]int{
			inference.ProtocolOpenAI:    config.DefaultOpenAIPort,
			inference.ProtocolAnthropic: config.DefaultAnthropicPort,
			inference.ProtocolGemini:    config.DefaultGeminiPort,
		} {
			if got := cfg.Server.DataPlane.Port(protocol); got != want {
				t.Fatalf("port of %s = %d, want %d", protocol, got, want)
			}
			if got := config.DefaultDataPlanePort(protocol); got != want {
				t.Fatalf("default port of %s = %d, want %d", protocol, got, want)
			}
			if !inference.ProtocolKnown(protocol) {
				t.Fatalf("%s is not a known protocol", protocol)
			}
		}
		if len(inference.DataPlaneProtocols()) != 3 {
			t.Fatalf("protocols = %v, want the three Relo serves", inference.DataPlaneProtocols())
		}
	})

	t.Run("a protocol Relo does not serve has no port", func(t *testing.T) {
		if got := cfg.Server.DataPlane.Port("nothing"); got != 0 {
			t.Fatalf("port = %d, want none", got)
		}
		if got := config.DefaultDataPlanePort("nothing"); got != 0 {
			t.Fatalf("default port = %d, want none", got)
		}
		if inference.ProtocolKnown("nothing") {
			t.Fatal("an unknown protocol reported itself as served")
		}
	})

	t.Run("the address of one protocol is its own bind and port", func(t *testing.T) {
		if got := cfg.Server.DataPlaneAddr(inference.ProtocolAnthropic); got != "127.0.0.1:"+strconv.Itoa(config.DefaultAnthropicPort) {
			t.Fatalf("address = %q, want the configured port", got)
		}
		// The whole-config spelling reaches the same answer, so a surface
		// holding a Config rather than its server block reads one address.
		if got := cfg.DataPlaneAddr(inference.ProtocolAnthropic); got != cfg.Server.DataPlaneAddr(inference.ProtocolAnthropic) {
			t.Fatalf("address = %q, want the server block's", got)
		}
		// A protocol no listener answers on has no address to hand a client.
		if got := cfg.DataPlaneAddr("nothing"); got != "" {
			t.Fatalf("address = %q, want empty for a protocol with no port", got)
		}
	})

	t.Run("only the protocols with a port are listeners", func(t *testing.T) {
		listeners := cfg.DataPlaneListeners()
		if len(listeners) != 3 {
			t.Fatalf("listeners = %+v, want one per protocol", listeners)
		}
		cfg.Server.DataPlane.Anthropic = 0
		listeners = cfg.DataPlaneListeners()
		if len(listeners) != 2 {
			t.Fatalf("listeners = %+v, want the two protocols left on", listeners)
		}
		for _, listener := range listeners {
			if listener.Protocol == inference.ProtocolAnthropic {
				t.Fatal("a protocol left off still reported a listener")
			}
		}
	})
}

func TestPaths(t *testing.T) {
	t.Run("custom RELO_HOME respected", func(t *testing.T) {
		home := testkit.TempHome(t)
		t.Setenv(config.ReloHomeEnv, home)
		got, err := config.ReloHome()
		if err != nil {
			t.Fatalf("ReloHome() error = %v", err)
		}
		if got != home {
			t.Fatalf("ReloHome() = %q, want %q", got, home)
		}
	})

	t.Run("default RELO_HOME is under the user home", func(t *testing.T) {
		t.Setenv(config.ReloHomeEnv, "")
		got, err := config.ReloHome()
		if err != nil {
			t.Fatalf("ReloHome() error = %v", err)
		}
		if !strings.HasSuffix(got, string(filepath.Separator)+config.DefaultReloHome) {
			t.Fatalf("ReloHome() = %q, want a path ending in %q", got, config.DefaultReloHome)
		}
	})

	t.Run("database and config paths live in the home", func(t *testing.T) {
		home := filepath.Join(string(filepath.Separator), "tmp", "relo-paths")
		if got := config.DatabasePath(home); got != filepath.Join(home, "state.sqlite") {
			t.Fatalf("DatabasePath() = %q", got)
		}
		if got := config.ConfigPath(home); got != filepath.Join(home, "config.toml") {
			t.Fatalf("ConfigPath() = %q", got)
		}
	})
}

func TestEnsureReloHome(t *testing.T) {
	t.Run("EnsureReloHome creates dir 0700", func(t *testing.T) {
		home := filepath.Join(testkit.TempHome(t), "nested", "state")
		if err := config.EnsureReloHome(home); err != nil {
			t.Fatalf("EnsureReloHome() error = %v", err)
		}
		info, err := os.Stat(home)
		if err != nil {
			t.Fatalf("stat home: %v", err)
		}
		if runtime.GOOS != "windows" && info.Mode().Perm() != 0o700 {
			t.Fatalf("home mode = %v, want 0700", info.Mode().Perm())
		}
	})

	t.Run("EnsureReloHome wrong perms returns error", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("windows does not enforce unix permissions")
		}
		home := testkit.TempHome(t)
		if err := os.Chmod(home, 0o755); err != nil {
			t.Fatalf("chmod home: %v", err)
		}
		if err := config.EnsureReloHome(home); !errors.Is(err, config.ErrHomePerm) {
			t.Fatalf("EnsureReloHome() error = %v, want %v", err, config.ErrHomePerm)
		}
	})
}

func TestLoggingConfigSlogLevel(t *testing.T) {
	tests := []struct {
		level string
		want  slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"error", slog.LevelError},
		{"anything-else", slog.LevelInfo},
	}
	for _, tt := range tests {
		t.Run(tt.level, func(t *testing.T) {
			got := config.LoggingConfig{Level: tt.level}.SlogLevel()
			if got != tt.want {
				t.Fatalf("SlogLevel() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestListenAddrs(t *testing.T) {
	cfg := config.DefaultConfig()
	if got := cfg.Server.Addr(); got != "127.0.0.1:10101" {
		t.Fatalf("server Addr() = %q", got)
	}
}

func TestConfiguredLanguage(t *testing.T) {
	write := func(t *testing.T, body string) string {
		t.Helper()
		path := filepath.Join(testkit.TempHome(t), "config.toml")
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatalf("write config: %v", err)
		}
		return path
	}

	t.Run("a configuration without a ui section reports nothing", func(t *testing.T) {
		path := write(t, "[server]\nport = 10101\n")
		tag, found, err := config.ConfiguredLanguage(path)
		if err != nil || found || tag != "" {
			t.Fatalf("ConfiguredLanguage() = %q, %v, %v, want nothing", tag, found, err)
		}
	})

	t.Run("a stored language is reported", func(t *testing.T) {
		path := write(t, "[ui]\nlanguage = \"zh-Hans\"\n")
		tag, found, err := config.ConfiguredLanguage(path)
		if err != nil || !found || tag != "zh-Hans" {
			t.Fatalf("ConfiguredLanguage() = %q, %v, %v, want zh-Hans", tag, found, err)
		}
	})

	t.Run("a missing file reports nothing rather than failing", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.toml")
		tag, found, err := config.ConfiguredLanguage(path)
		if err != nil || found || tag != "" {
			t.Fatalf("ConfiguredLanguage() = %q, %v, %v, want nothing", tag, found, err)
		}
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("stat %s = %v, want no file written by a reader", path, err)
		}
	})

	t.Run("a file that does not parse reports the failure", func(t *testing.T) {
		path := write(t, "this is not toml")
		if _, _, err := config.ConfiguredLanguage(path); err == nil {
			t.Fatal("ConfiguredLanguage() error = nil, want the parse failure")
		}
	})
}

func TestValidateLanguage(t *testing.T) {
	tests := []struct {
		name     string
		language string
		wantErr  bool
	}{
		{"auto follows the system", config.DefaultUILanguage, false},
		{"english", "en", false},
		{"german", "de", false},
		{"simplified chinese", "zh-Hans", false},
		{"a regional german tag", "de-AT", false},
		{"french", "fr", false},
		{"brazilian portuguese", "pt-BR", false},
		{"traditional chinese", "zh-Hant", false},
		{"swahili has no catalog", "sw", true},
		{"an empty language is refused", "", true},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			cfg := config.DefaultConfig()
			cfg.UI.Language = testCase.language
			err := config.ValidateConfig(&cfg)
			if testCase.wantErr && !errors.Is(err, config.ErrInvalidLanguage) {
				t.Fatalf("ValidateConfig() error = %v, want ErrInvalidLanguage", err)
			}
			if !testCase.wantErr && err != nil {
				t.Fatalf("ValidateConfig() error = %v, want the language accepted", err)
			}
		})
	}
}

func TestUpdateAppearanceStoresTheLanguage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("[ui]\ntheme = \"dark\"\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	tag := "de"
	if _, err := config.UpdateAppearance(path, config.AppearancePatch{Language: &tag}); err != nil {
		t.Fatalf("UpdateAppearance() error = %v", err)
	}
	got, found, err := config.ConfiguredLanguage(path)
	if err != nil || !found || got != "de" {
		t.Fatalf("ConfiguredLanguage() = %q, %v, %v, want de", got, found, err)
	}
	bad := "sw"
	if _, err := config.UpdateAppearance(path, config.AppearancePatch{Language: &bad}); !errors.Is(err, config.ErrInvalidLanguage) {
		t.Fatalf("UpdateAppearance() error = %v, want ErrInvalidLanguage", err)
	}
}

// TestExampleConfig guards the shipped example: it must parse, validate,
// show the defaults, and name no key the loader would ignore, so it stays a
// true copy of the schema.
func TestExampleConfig(t *testing.T) {
	path := filepath.Join("..", "..", "..", "..", "config.example.toml")
	logger, buffer := testkit.TestLogger(t)
	loaded, err := config.LoadConfig(path, logger)
	if err != nil {
		t.Fatalf("LoadConfig(%s) error = %v", path, err)
	}
	if err := config.ValidateConfig(loaded); err != nil {
		t.Fatalf("ValidateConfig() error = %v", err)
	}
	if warned := buffer.String(); warned != "" {
		t.Fatalf("the example names keys the loader ignores: %s", warned)
	}
	if !sameConfig(*loaded, config.DefaultConfig()) {
		t.Fatalf("the example does not show the defaults: %+v", *loaded)
	}
}

// TestShippedDefaultsAreCopies covers the preset accessors with mutation
// isolation: changing a returned slice never moves the shipped baseline.
func TestShippedDefaultsAreCopies(t *testing.T) {
	backoff := config.DefaultUpstreamRetryBackoff()
	backoff[0][0] = 999
	if again := config.DefaultUpstreamRetryBackoff(); again[0][0] == 999 {
		t.Fatal("DefaultUpstreamRetryBackoff() shares its storage with callers")
	}
	timeouts := config.UpstreamTimeoutPresets()
	timeouts[0] = 999
	if again := config.UpstreamTimeoutPresets(); again[0] == 999 {
		t.Fatal("UpstreamTimeoutPresets() shares its storage with callers")
	}
	metrics := config.UsageMetricIDs()
	metrics[0] = "mutated"
	if again := config.UsageMetricIDs(); again[0] == "mutated" {
		t.Fatal("UsageMetricIDs() shares its storage with callers")
	}
}
