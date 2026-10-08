// Default configuration: ports, homes, and the boot baseline.
package config

import (
	"log/slog"

	"github.com/jonaskahn/relo/internal/i18n"
	"github.com/jonaskahn/relo/internal/inference"
)

// Shipped defaults are the startup baseline a fresh home runs on: loopback
// listeners, ports, log level, index source, and update feed.
const (
	DefaultServerBind = "127.0.0.1"
	DefaultServerPort = 10101
	// DefaultOpenAIPort The data plane answers on its own ports, one per client protocol, so a
	// client points at the address its ecosystem expects. The management
	// listener keeps 10101 for the console, the API, and the login callbacks.
	DefaultOpenAIPort      = 10201
	DefaultAnthropicPort   = 10202
	DefaultGeminiPort      = 10203
	DefaultLogLevel        = "info"
	DefaultModelsDevURL    = "https://models.dev/api.json"
	DefaultReloHome        = ".relo"
	DefaultUILanguage      = i18n.Auto
	DefaultUITheme         = "system"
	DefaultUIAccent        = "red"
	DefaultUpdatesURL      = "https://jonaskahn.github.io/relo/appcast.xml"
	DefaultUpdatesDownload = "https://github.com/jonaskahn/relo/releases/latest"
	ReloHomeEnv            = "RELO_HOME"
	// DefaultUpstreamTimeoutSeconds is how long one provider call waits for
	// the next bytes when the operator never picked a preset.
	DefaultUpstreamTimeoutSeconds = 300
	// DefaultFailoverCooldownSeconds is the first denylist step used when the
	// operator never picked one: zero, so the first failure keeps no wait and
	// only a repeated failure starts escalating.
	DefaultFailoverCooldownSeconds = 0
)

// DefaultUpstreamRetryBackoff is the shipped wait before each retry, as
// low–high second ranges: 1–3s, then 3–5s, then 5–10s. The copy it returns
// keeps callers from mutating the shipped baseline.
func DefaultUpstreamRetryBackoff() [][2]int {
	return [][2]int{{1, 3}, {3, 5}, {5, 10}}
}

// DefaultDataPlanePort returns the port a protocol listens on when the
// configuration names none, and zero for a protocol Relo does not serve.
func DefaultDataPlanePort(protocol string) int {
	switch protocol {
	case inference.ProtocolOpenAI:
		return DefaultOpenAIPort
	case inference.ProtocolAnthropic:
		return DefaultAnthropicPort
	case inference.ProtocolGemini:
		return DefaultGeminiPort
	default:
		return 0
	}
}

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
			DataPlane: DataPlaneConfig{
				OpenAI: DefaultOpenAIPort, Anthropic: DefaultAnthropicPort, Gemini: DefaultGeminiPort,
			},
		},
		Upstream: UpstreamConfig{
			TimeoutSeconds:          DefaultUpstreamTimeoutSeconds,
			RetryBackoff:            append([][2]int(nil), DefaultUpstreamRetryBackoff()...),
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
