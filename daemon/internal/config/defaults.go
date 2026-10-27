// Default configuration: the boot baseline.
package config

import (
	"log/slog"

	"github.com/jonaskahn/relo/internal/i18n"
)

// Shipped defaults are the startup baseline a fresh home runs on: the
// loopback listener, log level, index source, and update feed.
const (
	DefaultServerBind   = "127.0.0.1"
	DefaultServerPort   = 10101
	DefaultLogLevel     = "info"
	DefaultModelsDevURL = "https://models.dev/api.json"
	DefaultReloHome     = ".relo"
	DefaultUILanguage   = i18n.Auto
	DefaultUITheme      = "system"
	// The radar's default colour, kept from the accent older builds shipped
	// as red so a fresh home looks the way it did before the 2026 board.
	DefaultUIAccent        = "#c8102e"
	DefaultUpdatesURL      = "https://jonaskahn.github.io/relo/appcast.xml"
	DefaultUpdatesDownload = "https://github.com/jonaskahn/relo/releases/latest"
	ReloHomeEnv            = "RELO_HOME"
	// DefaultUpstreamTimeoutSeconds is how long one provider call waits for
	// the next bytes when the operator never picked a preset.
	DefaultUpstreamTimeoutSeconds = 300
	// DefaultUpstreamRetryAttempts is how many times one account is retried
	// when the operator never picked a preset.
	DefaultUpstreamRetryAttempts = 3
	// DefaultFailoverCooldownSeconds is the first denylist step used when the
	// operator never picked one: zero, so the first failure keeps no wait and
	// only a repeated failure starts escalating.
	DefaultFailoverCooldownSeconds = 0
)

// DefaultAutostart is the shipped default: a proxy an operator relies on
// should come back after a restart.
const DefaultAutostart = true

// The vault key file name is a frozen contract documented in config.example.toml.
const defaultKeyFile = "secret.key"

// DefaultConfig returns a Config with every default applied.
func DefaultConfig() Config {
	return Config{
		System: SystemConfig{
			Autostart: DefaultAutostart,
			Logging:   LoggingConfig{Level: DefaultLogLevel},
			Updates:   UpdatesConfig{URL: DefaultUpdatesURL, Download: DefaultUpdatesDownload},
		},
		Server: ServerConfig{
			Bind: DefaultServerBind, Port: DefaultServerPort,
		},
		Upstream: UpstreamConfig{
			TimeoutSeconds:          DefaultUpstreamTimeoutSeconds,
			RetryAttempts:           DefaultUpstreamRetryAttempts,
			FailoverCooldownSeconds: DefaultFailoverCooldownSeconds,
		},
		Catalog: CatalogConfig{ModelsDevURL: DefaultModelsDevURL},
		Secrets: SecretsConfig{Keychain: false, KeyFile: defaultKeyFile},
		Admin:   AdminConfig{Login: false},
		UI: UIConfig{
			Language: DefaultUILanguage, Theme: DefaultUITheme, Accent: DefaultUIAccent,
			Usage: UsageConfig{OverviewMetrics: append([]string(nil), DefaultOverviewMetrics()...)},
		},
	}
}

// SlogLevel returns the slog level that matches the configured log level.
func (c LoggingConfig) SlogLevel() slog.Level {
	switch c.Level {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
