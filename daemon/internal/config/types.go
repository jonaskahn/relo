// Package config loads the startup TOML file Relo reads once at boot.
package config

import (
	"github.com/jonaskahn/relo/internal/inference"
	"net"
	"strconv"
)

// Config is the top-level startup configuration.
type Config struct {
	System   SystemConfig   `toml:"system"`
	Server   ServerConfig   `toml:"server"`
	Proxy    ProxyConfig    `toml:"proxy"`
	Upstream UpstreamConfig `toml:"upstream"`
	Catalog  CatalogConfig  `toml:"catalog"`
	Secrets  SecretsConfig  `toml:"secrets"`
	Admin    AdminConfig    `toml:"admin"`
	UI       UIConfig       `toml:"ui"`
}

// SystemConfig is how the process behaves: when it starts at login, how it
// logs, whether it may open a browser, and how it learns about newer builds.
type SystemConfig struct {
	// Autostart registers Relo to start at login. A deployment that starts
	// Relo some other way sets it to false.
	Autostart bool          `toml:"autostart"`
	Logging   LoggingConfig `toml:"logging"`
	// Headless assumes no browser can open: provider logins print their
	// authorization URL instead of opening one. A CI runner and Linux
	// without a display are headless without this setting.
	Headless bool          `toml:"headless"`
	Updates  UpdatesConfig `toml:"updates"`
}

// UpdatesConfig names the feed Relo checks for a newer build.
type UpdatesConfig struct {
	// URL is the Sparkle appcast Relo reads. An empty URL turns the check
	// off.
	URL string `toml:"url"`
	// Download is the page Windows, Linux, and daemon-only installs open
	// when a newer build is published. Relo.app on macOS uses Sparkle
	// instead.
	Download string `toml:"download"`
}

// CatalogConfig configures the models.dev data source.
type CatalogConfig struct {
	ModelsDevURL string `toml:"modelsdev_url"`
}

// UIConfig configures the language every surface speaks: the command
// line, the desktop tray app, the management API, and the console.
type UIConfig struct {
	Language string      `toml:"language"`
	Theme    string      `toml:"theme"`
	Accent   string      `toml:"accent"`
	Usage    UsageConfig `toml:"usage"`
}

// UsageConfig holds what the usage page shows, which an operator picks in the
// console and reads back from the same file every surface shares.
type UsageConfig struct {
	// OverviewMetrics names the cards the usage overview shows, in order.
	OverviewMetrics []string `toml:"overview_metrics"`
}

// ServerConfig configures the single listener serving the management API,
// the dashboard, the login callbacks, and the data plane prefixes.
type ServerConfig struct {
	Bind string `toml:"bind"`
	Port int    `toml:"port"`
}

// SecretsConfig configures where provider credentials are stored.
type SecretsConfig struct {
	Keychain bool   `toml:"keychain"`
	KeyFile  string `toml:"key_file"`
}

// AdminConfig configures who may reach the console and the management API.
type AdminConfig struct {
	Login bool `toml:"login"`
	// AllowExternal permits the console to be reached through a forwarded
	// address, such as a tunnel or another device. Requests from this
	// machine, its private network, and container networks stay without
	// sign-in; everything else must present the admin token, even while
	// Login is off.
	AllowExternal bool `toml:"allow_external"`
}

// LoggingConfig configures the process-wide logger.
type LoggingConfig struct {
	Level string `toml:"level"`
	// Traffic also writes every request and provider exchange, in full, to a
	// daily JSONL file in the logs folder. Off by default: the file holds prompts.
	Traffic bool `toml:"traffic"`
}

// Addr returns the host:port the listener binds.
func (c ServerConfig) Addr() string {
	return net.JoinHostPort(c.Bind, strconv.Itoa(c.Port))
}

// DataPlaneBaseURL returns the address a client of one protocol points at,
// and an empty string for a protocol the daemon does not serve.
func (c ServerConfig) DataPlaneBaseURL(protocol string) string {
	prefix := inference.DataPlanePath(protocol)
	if prefix == "" {
		return ""
	}
	return "http://" + c.Addr() + prefix
}

// DataPlaneBaseURL returns the address a client of one protocol points at,
// so a caller can hand a client its address without reading the server block.
func (c Config) DataPlaneBaseURL(protocol string) string {
	return c.Server.DataPlaneBaseURL(protocol)
}
