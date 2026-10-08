// Config loading: files, offline homes, and strict decoding.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"

	"github.com/BurntSushi/toml"
)

// LoadConfig reads the startup file into typed structs, filling every
// missing field from the defaults. A missing file is written once with
// the defaults; an existing file is never rewritten.
func LoadConfig(path string, logger *slog.Logger) (*Config, error) {
	cfg := DefaultConfig()
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return &cfg, writeDefaultConfig(path, cfg, logger)
	}
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}
	return decodeConfig(path, data, &cfg, logger)
}

// LoadOffline reads the startup file of a state directory without creating
// or changing any file, so a diagnostic and a management session never
// write. A state directory without a startup file reads as the defaults.
func LoadOffline(home string) (Config, error) {
	return ReadFile(ConfigPath(home))
}

// ReadFile reads the startup file at path into the defaults without creating
// or changing anything. A missing file reads as the defaults.
func ReadFile(path string) (Config, error) {
	cfg := DefaultConfig()
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("read config %s: %w", path, err)
	}
	loaded, err := decodeConfig(path, data, &cfg, nil)
	if err != nil {
		return Config{}, err
	}
	return *loaded, nil
}

func decodeConfig(path string, data []byte, cfg *Config, logger *slog.Logger) (*Config, error) {
	meta, err := toml.Decode(string(data), cfg)
	if err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	normalizeUpstream(cfg)
	// A metric this build retired is dropped on read, so an existing file
	// keeps starting after the picker list changed.
	if meta.IsDefined("ui", "usage", "overview_metrics") {
		cfg.UI.Usage.OverviewMetrics = dropRetiredUsageMetrics(cfg.UI.Usage.OverviewMetrics)
	}
	applyLegacySystem(data, cfg, meta)
	warnUnknownKeys(meta, logger)
	return cfg, nil
}

func normalizeUpstream(cfg *Config) {
	if ValidateUpstreamTimeout(cfg.Upstream.TimeoutSeconds) != nil {
		cfg.Upstream.TimeoutSeconds = DefaultUpstreamTimeoutSeconds
	}
	if ValidateUpstreamRetryBackoff(cfg.Upstream.RetryBackoff) != nil {
		cfg.Upstream.RetryBackoff = append([][2]int(nil), DefaultUpstreamRetryBackoff()...)
	}
	if ValidateUpstreamFailoverCooldown(cfg.Upstream.FailoverCooldownSeconds) != nil {
		cfg.Upstream.FailoverCooldownSeconds = DefaultFailoverCooldownSeconds
	}
}

type legacySystemFile struct {
	Autostart *bool `toml:"autostart"`
	Desktop   struct {
		Autostart *bool `toml:"autostart"`
	} `toml:"desktop"`
	Logging struct {
		Level *string `toml:"level"`
	} `toml:"logging"`
	Updates struct {
		URL      *string `toml:"url"`
		Download *string `toml:"download"`
	} `toml:"updates"`
}

func applyLegacyAutostart(cfg *Config, file legacySystemFile) {
	switch {
	case file.Autostart != nil:
		cfg.System.Autostart = *file.Autostart
	case file.Desktop.Autostart != nil:
		cfg.System.Autostart = *file.Desktop.Autostart
	}
}

func applyLegacySystem(data []byte, cfg *Config, meta toml.MetaData) {
	var file legacySystemFile
	if err := toml.Unmarshal(data, &file); err != nil {
		return
	}
	if !meta.IsDefined("system", "autostart") {
		applyLegacyAutostart(cfg, file)
	}
	if !meta.IsDefined("system", "logging", "level") && file.Logging.Level != nil {
		cfg.System.Logging.Level = *file.Logging.Level
	}
	if !meta.IsDefined("system", "updates", "url") && file.Updates.URL != nil {
		cfg.System.Updates.URL = *file.Updates.URL
	}
	if !meta.IsDefined("system", "updates", "download") && file.Updates.Download != nil {
		cfg.System.Updates.Download = *file.Updates.Download
	}
}

func writeDefaultConfig(path string, cfg Config, logger *slog.Logger) error {
	encoded, err := toml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("encode default config: %w", err)
	}
	if err := os.WriteFile(path, encoded, configMode); err != nil {
		return fmt.Errorf("write config %s: %w", path, err)
	}
	if logger != nil {
		logger.Info("wrote default config", "path", path)
	}
	return nil
}

func warnUnknownKeys(meta toml.MetaData, logger *slog.Logger) {
	if logger == nil {
		return
	}
	for _, key := range meta.Undecoded() {
		if retiredKeys[key.String()] {
			continue
		}
		logger.Warn("unknown config key ignored", "key", key.String())
	}
}

var retiredKeys = map[string]bool{
	"autostart":         true,
	"desktop.autostart": true,
	"logging.level":     true,
	"updates.url":       true,
	"updates.download":  true,
}

// ConfiguredLanguage reads the interface language from the startup file
// without creating one: the command line resolves its language before it
// decides whether the state directory should exist at all. A file that
// cannot be read or parsed reports no language, and the command that needs
// the configuration reports the failure itself.
func ConfiguredLanguage(path string) (string, bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("read config %s: %w", path, err)
	}
	var file struct {
		UI UIConfig `toml:"ui"`
	}
	if err := toml.Unmarshal(data, &file); err != nil {
		return "", false, fmt.Errorf("parse config %s: %w", path, err)
	}
	if file.UI.Language == "" {
		return "", false, nil
	}
	return file.UI.Language, true, nil
}
