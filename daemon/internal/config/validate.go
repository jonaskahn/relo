// Config validation: the rules a startup file must satisfy.
package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"

	"github.com/jonaskahn/relo/internal/i18n"
)

// Config errors name the startup values an operator must fix, each phrased
// as the rule the value broke.
var (
	ErrInvalidPort             = errors.New("must be between 1 and 65535, or 0 to leave it off")
	ErrInvalidBind             = errors.New("must be a valid IP address")
	ErrInvalidLogLevel         = errors.New("must be one of: debug, info, warn, error")
	ErrInvalidLanguage         = errors.New("must be auto, or one of the languages Relo ships")
	ErrInvalidTheme            = errors.New("must be system, light, or dark")
	ErrInvalidAccent           = errors.New("must be a supported accent")
	ErrInvalidQuotaDisplay     = errors.New("must be used or remaining")
	ErrInvalidURL              = errors.New("must be an http or https address")
	ErrHomePerm                = errors.New("RELO_HOME directory must have mode 0700")
	ErrLoginOffOutsideLoopback = errors.New("must be true, or admin.allow_external must be on, when server.bind is not a loopback address")
)

// ValidateConfig reports the first invalid field, named by its config path.
func ValidateConfig(cfg *Config) error {
	if err := validateServer(cfg); err != nil {
		return err
	}
	if err := validateAdmin(cfg); err != nil {
		return err
	}
	if err := validateLogLevel(cfg.System.Logging.Level); err != nil {
		return err
	}
	if err := validateLanguage(cfg.UI.Language); err != nil {
		return err
	}
	if len(cfg.UI.Usage.OverviewMetrics) > 0 {
		if err := ValidateUsageMetrics(cfg.UI.Usage.OverviewMetrics); err != nil {
			return err
		}
	}
	if err := ValidateUpstreamTimeout(cfg.Upstream.TimeoutSeconds); err != nil {
		return err
	}
	if err := ValidateUpstreamRetryAttempts(cfg.Upstream.RetryAttempts); err != nil {
		return err
	}
	if err := ValidateUpstreamFailoverCooldown(cfg.Upstream.FailoverCooldownSeconds); err != nil {
		return err
	}
	return ValidateAppearance(cfg.UI.Theme, cfg.UI.Accent)
}

// ValidateAppearance checks the stored appearance choices.
func ValidateAppearance(theme, accent string) error {
	switch theme {
	case "system", "light", "dark":
	default:
		return fmt.Errorf("ui.theme: %w", ErrInvalidTheme)
	}
	// The 2.4 emerald and violet stay accepted so a config written before the
	// 3.0 accent set still starts; green and purple are their 3.0 names.
	switch accent {
	case "red", "orange", "amber", "green", "teal", "cyan", "blue", "indigo", "purple", "rose",
		"emerald", "violet":
		return nil
	default:
		return fmt.Errorf("ui.accent: %w", ErrInvalidAccent)
	}
}

func validateAdmin(cfg *Config) error {
	if cfg.Admin.Login || cfg.Admin.AllowExternal {
		return nil
	}
	if address := net.ParseIP(cfg.Server.Bind); address != nil && address.IsLoopback() {
		return nil
	}
	return fmt.Errorf("admin.login: %w", ErrLoginOffOutsideLoopback)
}

func validateServer(cfg *Config) error {
	if err := validateBind("server.bind", cfg.Server.Bind); err != nil {
		return err
	}
	return validatePort("server.port", cfg.Server.Port)
}

func validateBind(path, bind string) error {
	if net.ParseIP(bind) == nil {
		return fmt.Errorf("%s: %w", path, ErrInvalidBind)
	}
	return nil
}

func validateHTTPURL(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return ErrInvalidURL
	}
	return nil
}

func validatePort(path string, port int) error {
	if port < 0 || port > 65535 {
		return fmt.Errorf("%s: %w", path, ErrInvalidPort)
	}
	return nil
}

func validateLogLevel(level string) error {
	switch level {
	case "debug", "info", "warn", "error":
		return nil
	default:
		return fmt.Errorf("system.logging.level: %w", ErrInvalidLogLevel)
	}
}

func validateLanguage(tag string) error {
	if tag == i18n.Auto {
		return nil
	}
	if _, ok := i18n.Match(tag); ok {
		return nil
	}
	return fmt.Errorf("ui.language: %w", ErrInvalidLanguage)
}
