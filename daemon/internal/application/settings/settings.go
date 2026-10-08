// Package settings owns what an operator configures about Relo itself: the
// usage-log retention budget, the interface language, the console appearance,
// and the cards the usage overview shows.
package settings

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jonaskahn/relo/internal/activity"
	"github.com/jonaskahn/relo/internal/clock"
	"github.com/jonaskahn/relo/internal/config"
	"github.com/jonaskahn/relo/internal/i18n"
)

// ErrInvalidWrite reports a settings write no store would accept.
var ErrInvalidWrite = errors.New("invalid settings")

// RetentionSettings is the usage-log budget: how long events are kept, how
// many, and how many bytes they may occupy. A zero field is unlimited.
type RetentionSettings struct {
	UsageDays int
	MaxEvents int64
	MaxBytes  int64
}

// Settings is everything the settings page reads and writes.
type Settings struct {
	Retention  RetentionSettings
	Language   string
	Appearance AppearanceSettings
	// System is when the daemon runs, how it logs, and how it learns about
	// newer builds.
	System SystemSettings
	// Server is where the daemon listens.
	Server ServerSettings
	// Providers is how the daemon reaches providers and their directories.
	Providers ProvidersSettings
	// Access is who may open the console.
	Access AccessSettings
	// Secrets is where provider credentials live. Read-only: editing the
	// vault from the console could strand every stored credential.
	Secrets SecretsSettings
	// RestartPending reports that stored choices await a daemon restart
	// before they take effect, such as a listener port or the log level.
	RestartPending bool
}

// SystemSettings is when the daemon runs, how it logs, and how it learns
// about newer builds.
type SystemSettings struct {
	Autostart       bool
	LogLevel        string
	UpdatesURL      string
	UpdatesDownload string
}

// ServerSettings is where the management listener and the data plane answer.
type ServerSettings struct {
	Bind          string
	Port          int
	OpenAIPort    int
	AnthropicPort int
	GeminiPort    int
}

// ProvidersSettings is how the daemon reaches providers and their
// directories.
type ProvidersSettings struct {
	CatalogURL string
	ProxyURL   string
	// TimeoutSeconds is how long a provider call waits for the next bytes
	// before the relay cancels it and retries: the wait for headers, and then
	// the silence between body bytes. A stream that keeps sending runs until
	// the provider is done. Zero means the operator never picked one, which
	// stores and reads as the default.
	TimeoutSeconds int
	// RetryBackoff is the wait before each retry as a low–high second range,
	// in retry order.
	RetryBackoff [][2]int
	// FailoverCooldownSeconds is the first step of the wait ladder a refused
	// account or route member walks: the next failure without a success
	// between waits the next, longer preset. Zero starts with no wait at
	// all. A body that omits the field reads as zero, so a partial write
	// without it starts the ladder from the beginning.
	FailoverCooldownSeconds int
}

// AccessSettings is who may open the console.
type AccessSettings struct {
	// AllowExternal lets the console be reached through a forwarded address
	// while the sign-in is off: requests from this machine and its local
	// network stay keyless, and everything else must present the admin token.
	AllowExternal bool
	// Login asks for the admin token on the sign-in page. Read-only: it
	// changes the sign-in model of the whole console and stays in config.toml.
	Login bool
}

// SecretsSettings is where provider credentials live.
type SecretsSettings struct {
	Keychain bool
	KeyFile  string
}

// AppearanceSettings is the console theme an operator picked.
type AppearanceSettings struct {
	Theme  string
	Accent string
	// QuotaDisplay says how the console draws a quota chart: the share used
	// or the share left.
	QuotaDisplay string
}

// RetentionStore persists the usage-log budget and reports what a run would
// do.
type RetentionStore interface {
	Budget(ctx context.Context) (activity.RetentionBudget, bool, error)
	SaveBudget(ctx context.Context, budget activity.RetentionBudget) error
	Preview(ctx context.Context, budget activity.RetentionBudget) (activity.RetentionReport, error)
	RunOnce(ctx context.Context) (activity.RetentionReport, error)
	Sweep(ctx context.Context) (activity.CleanupReport, error)
}

// QuotaDisplayStore persists how the console draws a quota chart.
type QuotaDisplayStore interface {
	QuotaDisplay(ctx context.Context) (string, bool, error)
	SaveQuotaDisplay(ctx context.Context, display string, nowMs int64) error
}

// AppearancePatch changes only the named appearance fields.
type AppearancePatch struct {
	Theme        *string
	Accent       *string
	QuotaDisplay *string
}

// Options configure the settings use cases. The appearance and usage-metric
// settings live in the startup file, so they are read and written through its
// path rather than through a store.
type Options struct {
	Retention    RetentionStore
	QuotaDisplay QuotaDisplayStore
	ConfigPath   string
	Clock        clock.Clock
	// ApplyAutostart mirrors a changed start-at-login choice onto the
	// platform's login items. It is best-effort: the wiring logs a failure
	// rather than failing a save whose configuration is already stored.
	ApplyAutostart func(enabled bool)
}

// Service is the settings use cases.
type Service struct {
	retention      RetentionStore
	quotaDisplay   QuotaDisplayStore
	configPath     string
	clock          clock.Clock
	applyAutostart func(enabled bool)
}

// New returns the settings use cases over the given stores.
func New(options Options) *Service {
	clk := options.Clock
	if clk == nil {
		clk = clock.New()
	}
	return &Service{
		retention: options.Retention, quotaDisplay: options.QuotaDisplay,
		configPath: options.ConfigPath, clock: clk,
		applyAutostart: options.ApplyAutostart,
	}
}

// Read returns every setting the settings page shows.
func (s *Service) Read(ctx context.Context) (Settings, error) {
	settings := Settings{Language: i18n.Auto}
	if err := s.readRetention(ctx, &settings); err != nil {
		return Settings{}, err
	}
	if err := s.readAppearanceLanguage(ctx, &settings); err != nil {
		return Settings{}, err
	}
	file, err := config.ReadFile(s.configPath)
	if err != nil {
		return Settings{}, err
	}
	readSystemServer(&settings, file)
	if err := s.readUpstream(&settings, file); err != nil {
		return Settings{}, err
	}
	if err := s.readAccessSecrets(&settings, file); err != nil {
		return Settings{}, err
	}
	return settings, nil
}

func (s *Service) readRetention(ctx context.Context, settings *Settings) error {
	budget, _, err := s.retention.Budget(ctx)
	if err != nil {
		return err
	}
	settings.Retention = RetentionSettings{
		UsageDays: budget.UsageDays, MaxEvents: budget.MaxEvents, MaxBytes: budget.MaxBytes,
	}
	return nil
}

func (s *Service) readAppearanceLanguage(ctx context.Context, settings *Settings) error {
	appearance, err := s.Appearance(ctx)
	if err != nil {
		return err
	}
	settings.Appearance = appearance
	language, err := s.Language()
	if err != nil {
		return err
	}
	settings.Language = language
	return nil
}

func readSystemServer(settings *Settings, file config.Config) {
	settings.System = SystemSettings{
		Autostart:       file.System.Autostart,
		LogLevel:        file.System.Logging.Level,
		UpdatesURL:      file.System.Updates.URL,
		UpdatesDownload: file.System.Updates.Download,
	}
	settings.Server = ServerSettings{
		Bind: file.Server.Bind, Port: file.Server.Port,
		OpenAIPort:    file.Server.DataPlane.OpenAI,
		AnthropicPort: file.Server.DataPlane.Anthropic,
		GeminiPort:    file.Server.DataPlane.Gemini,
	}
}

func (s *Service) readUpstream(settings *Settings, file config.Config) error {
	proxyURL, err := config.ReadProxyURL(s.configPath)
	if err != nil {
		return err
	}
	timeout, err := config.ReadUpstreamTimeout(s.configPath)
	if err != nil {
		return err
	}
	backoff, err := config.ReadUpstreamRetryBackoff(s.configPath)
	if err != nil {
		return err
	}
	cooldown, err := config.ReadUpstreamFailoverCooldown(s.configPath)
	if err != nil {
		return err
	}
	settings.Providers = ProvidersSettings{
		CatalogURL: file.Catalog.ModelsDevURL,
		ProxyURL:   proxyURL, TimeoutSeconds: timeout, RetryBackoff: backoff,
		FailoverCooldownSeconds: cooldown,
	}
	return nil
}

func (s *Service) readAccessSecrets(settings *Settings, file config.Config) error {
	allowExternal, err := config.ReadAdminAllowExternal(s.configPath)
	if err != nil {
		return err
	}
	settings.Access = AccessSettings{AllowExternal: allowExternal, Login: file.Admin.Login}
	settings.Secrets = SecretsSettings{Keychain: file.Secrets.Keychain, KeyFile: file.Secrets.KeyFile}
	return nil
}

// RestartPending reports whether the stored file names a listener, log
// level, update feed, model directory, sign-in, or vault the running daemon
// did not boot with. Those choices apply at the next start; everything else
// the settings page edits takes effect on save.
func (s *Service) RestartPending(boot config.Config) (bool, error) {
	file, err := config.ReadFile(s.configPath)
	if err != nil {
		return false, err
	}
	return boot.Server != file.Server ||
		boot.System.Logging.Level != file.System.Logging.Level ||
		boot.System.Updates != file.System.Updates ||
		boot.Catalog.ModelsDevURL != file.Catalog.ModelsDevURL ||
		boot.Admin.Login != file.Admin.Login ||
		boot.Secrets != file.Secrets, nil
}

// ProxyURL returns the outbound proxy stored in the startup file.
func (s *Service) ProxyURL() (string, error) {
	return config.ReadProxyURL(s.configPath)
}

// UpstreamTimeout returns the call wait one provider attempt runs under, in
// seconds: the wait for headers and the silence between body bytes, read from
// the startup file on every request.
func (s *Service) UpstreamTimeout() (int, error) {
	return config.ReadUpstreamTimeout(s.configPath)
}

// UpstreamRetryBackoff returns the random wait before each retry, as
// low–high second ranges, read from the startup file on every request.
func (s *Service) UpstreamRetryBackoff() ([][2]int, error) {
	return config.ReadUpstreamRetryBackoff(s.configPath)
}

// AllowExternal reports whether the console may be reached through a
// forwarded address while the sign-in is off. The management guard reads it
// on every request, so a console toggle takes effect at once.
func (s *Service) AllowExternal() (bool, error) {
	return config.ReadAdminAllowExternal(s.configPath)
}

// Appearance returns the console theme stored in the startup file and the
// quota reading stored in the database.
func (s *Service) Appearance(ctx context.Context) (AppearanceSettings, error) {
	ui, err := config.ReadAppearance(s.configPath)
	if err != nil {
		return AppearanceSettings{}, err
	}
	display := config.QuotaDisplayUsed
	if s.quotaDisplay != nil {
		stored, found, err := s.quotaDisplay.QuotaDisplay(ctx)
		if err != nil {
			return AppearanceSettings{}, err
		}
		if found {
			display = stored
		}
	}
	return AppearanceSettings{Theme: ui.Theme, Accent: ui.Accent, QuotaDisplay: display}, nil
}

// SaveAppearance stores the console theme and quota reading an operator
// picked.
func (s *Service) SaveAppearance(ctx context.Context, patch AppearancePatch) (AppearanceSettings, error) {
	if patch.Theme == nil && patch.Accent == nil && patch.QuotaDisplay == nil {
		return AppearanceSettings{}, ErrInvalidWrite
	}
	if patch.QuotaDisplay != nil {
		if err := config.ValidateQuotaDisplay(*patch.QuotaDisplay); err != nil {
			return AppearanceSettings{}, fmt.Errorf("%w: %v", ErrInvalidWrite, err)
		}
	}
	if patch.Theme != nil || patch.Accent != nil {
		if _, err := config.UpdateAppearance(s.configPath, config.AppearancePatch{
			Theme: patch.Theme, Accent: patch.Accent,
		}); err != nil {
			if errors.Is(err, config.ErrInvalidTheme) || errors.Is(err, config.ErrInvalidAccent) {
				return AppearanceSettings{}, fmt.Errorf("%w: %v", ErrInvalidWrite, err)
			}
			return AppearanceSettings{}, err
		}
	}
	if patch.QuotaDisplay != nil && s.quotaDisplay != nil {
		if err := s.quotaDisplay.SaveQuotaDisplay(ctx, *patch.QuotaDisplay, s.clock.Now().UnixMilli()); err != nil {
			return AppearanceSettings{}, err
		}
	}
	return s.Appearance(ctx)
}

// Language returns the language stored in the startup file, which is auto
// until an operator picks one. Every surface reads it before it speaks.
func (s *Service) Language() (string, error) {
	ui, err := config.ReadAppearance(s.configPath)
	if err != nil {
		return "", err
	}
	return ui.Language, nil
}

// SaveLanguage stores the language an operator picked, which every surface
// reads before it speaks.
func (s *Service) SaveLanguage(tag string) error {
	if err := checkLanguage(tag); err != nil {
		return err
	}
	if tag == "" {
		tag = i18n.Auto
	}
	_, err := config.UpdateAppearance(s.configPath, config.AppearancePatch{Language: &tag})
	return err
}

// SaveRetention stores the usage-log budget the maintenance pass applies.
func (s *Service) SaveRetention(ctx context.Context, budget RetentionSettings) (RetentionSettings, error) {
	now := s.clock.Now().UnixMilli()
	if err := s.retention.SaveBudget(ctx, activity.RetentionBudget{
		UsageDays: budget.UsageDays, MaxEvents: budget.MaxEvents,
		MaxBytes: budget.MaxBytes, UpdatedAtMs: now,
	}); err != nil {
		return RetentionSettings{}, err
	}
	return budget, nil
}

// SaveAccess stores the external-access choice, which the management guard
// reads on every request.
func (s *Service) SaveAccess(allowExternal bool) error {
	if err := config.UpdateAdminAllowExternal(s.configPath, allowExternal); err != nil {
		if errors.Is(err, config.ErrLoginOffOutsideLoopback) {
			return fmt.Errorf("%w: %v", ErrInvalidWrite, err)
		}
		return err
	}
	return nil
}

// SaveNetwork stores where the management listener and the data plane answer.
// A bind or a port that would leave the next start unaccepted is refused.
func (s *Service) SaveNetwork(server ServerSettings) (ServerSettings, error) {
	if err := config.UpdateServerConfig(s.configPath, config.ServerConfigPatch{
		Bind:       &server.Bind,
		Port:       &server.Port,
		OpenAIPort: &server.OpenAIPort, AnthropicPort: &server.AnthropicPort, GeminiPort: &server.GeminiPort,
	}); err != nil {
		if errors.Is(err, config.ErrInvalidBind) || errors.Is(err, config.ErrInvalidPort) ||
			errors.Is(err, config.ErrDuplicatePort) || errors.Is(err, config.ErrLoginOffOutsideLoopback) {
			return ServerSettings{}, fmt.Errorf("%w: %v", ErrInvalidWrite, err)
		}
		return ServerSettings{}, err
	}
	return server, nil
}

// SaveProviders stores how the daemon reaches providers and their
// directories: the outbound proxy, the model directory, and the call,
// retry, and failover waits.
func (s *Service) SaveProviders(input ProvidersSettings) (ProvidersSettings, error) {
	proxyURL := strings.TrimSpace(input.ProxyURL)
	if err := config.ValidateProxyURL(proxyURL); err != nil {
		return ProvidersSettings{}, fmt.Errorf("%w: %v", ErrInvalidWrite, err)
	}
	timeout, backoff, cooldown, err := validatedProviderWaits(input)
	if err != nil {
		return ProvidersSettings{}, err
	}
	if err := s.writeProviderConfig(input, proxyURL, timeout, backoff, cooldown); err != nil {
		return ProvidersSettings{}, err
	}
	return ProvidersSettings{
		CatalogURL: strings.TrimSpace(input.CatalogURL), ProxyURL: proxyURL,
		TimeoutSeconds: timeout, RetryBackoff: backoff,
		FailoverCooldownSeconds: cooldown,
	}, nil
}

func validatedProviderWaits(input ProvidersSettings) (int, [][2]int, int, error) {
	timeout := input.TimeoutSeconds
	if timeout == 0 {
		timeout = config.DefaultUpstreamTimeoutSeconds
	}
	if err := config.ValidateUpstreamTimeout(timeout); err != nil {
		return 0, nil, 0, fmt.Errorf("%w: %v", ErrInvalidWrite, err)
	}
	backoff := input.RetryBackoff
	if len(backoff) == 0 {
		backoff = config.DefaultUpstreamRetryBackoff()
	}
	if err := config.ValidateUpstreamRetryBackoff(backoff); err != nil {
		return 0, nil, 0, fmt.Errorf("%w: %v", ErrInvalidWrite, err)
	}
	cooldown := input.FailoverCooldownSeconds
	if err := config.ValidateUpstreamFailoverCooldown(cooldown); err != nil {
		return 0, nil, 0, fmt.Errorf("%w: %v", ErrInvalidWrite, err)
	}
	return timeout, backoff, cooldown, nil
}

func (s *Service) writeProviderConfig(input ProvidersSettings, proxyURL string, timeout int, backoff [][2]int, cooldown int) error {
	if err := config.UpdateProxyURL(s.configPath, proxyURL); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidWrite, err)
	}
	if err := config.UpdateCatalogURL(s.configPath, strings.TrimSpace(input.CatalogURL)); err != nil {
		if errors.Is(err, config.ErrInvalidURL) {
			return fmt.Errorf("%w: %v", ErrInvalidWrite, err)
		}
		return err
	}
	if err := config.UpdateUpstreamTimeout(s.configPath, timeout); err != nil {
		return err
	}
	if err := config.UpdateUpstreamRetryBackoff(s.configPath, backoff); err != nil {
		return err
	}
	return config.UpdateUpstreamFailoverCooldown(s.configPath, cooldown)
}

// SaveSystem stores when the daemon runs, how it logs, and how it learns
// about newer builds. A changed start-at-login choice is mirrored onto the
// platform's login items at once, so the toggle means what it says.
func (s *Service) SaveSystem(system SystemSettings) (SystemSettings, error) {
	before, err := config.ReadFile(s.configPath)
	if err != nil {
		return SystemSettings{}, err
	}
	level := system.LogLevel
	updatesURL := strings.TrimSpace(system.UpdatesURL)
	download := strings.TrimSpace(system.UpdatesDownload)
	if err := config.UpdateSystemConfig(s.configPath, config.SystemConfigPatch{
		Autostart: &system.Autostart, LogLevel: &level,
		UpdatesURL: &updatesURL, UpdatesDownload: &download,
	}); err != nil {
		if errors.Is(err, config.ErrInvalidLogLevel) || errors.Is(err, config.ErrInvalidURL) {
			return SystemSettings{}, fmt.Errorf("%w: %v", ErrInvalidWrite, err)
		}
		return SystemSettings{}, err
	}
	if before.System.Autostart != system.Autostart && s.applyAutostart != nil {
		s.applyAutostart(system.Autostart)
	}
	return SystemSettings{
		Autostart: system.Autostart, LogLevel: level,
		UpdatesURL: updatesURL, UpdatesDownload: download,
	}, nil
}

// PreviewRetention reports what the given budget would delete, without
// changing a row.
func (s *Service) PreviewRetention(ctx context.Context, budget RetentionSettings) (activity.RetentionReport, error) {
	report, err := s.retention.Preview(ctx, activity.RetentionBudget{
		UsageDays: budget.UsageDays, MaxEvents: budget.MaxEvents, MaxBytes: budget.MaxBytes,
	})
	if err != nil {
		return activity.RetentionReport{}, err
	}
	return report, nil
}

// RunRetention applies the stored budget now, so a save or a console action
// cleans up without waiting for the maintenance tick.
func (s *Service) RunRetention(ctx context.Context) (activity.RetentionReport, error) {
	return s.retention.RunOnce(ctx)
}

// Sweep runs the maintenance pass the console's cleanup action reports: the
// archive is closed, the stored budget applied, the captured bodies purged,
// and the database compacted.
func (s *Service) Sweep(ctx context.Context) (activity.CleanupReport, error) {
	return s.retention.Sweep(ctx)
}

// UsageMetrics returns the cards the usage overview shows, in order, read
// from the startup file every surface shares.
func (s *Service) UsageMetrics() ([]string, error) {
	return config.ReadUsageMetrics(s.configPath)
}

// SaveUsageMetrics stores the chosen cards, refusing a set the overview
// cannot render.
func (s *Service) SaveUsageMetrics(metrics []string) ([]string, error) {
	stored, err := config.UpdateUsageMetrics(s.configPath, metrics)
	if err != nil {
		if errors.Is(err, config.ErrInvalidMetric) || errors.Is(err, config.ErrInvalidMetricSet) {
			return nil, fmt.Errorf("%w: %v", ErrInvalidWrite, err)
		}
		return nil, err
	}
	return stored, nil
}

func checkLanguage(tag string) error {
	if tag == "" || tag == i18n.Auto {
		return nil
	}
	if _, ok := i18n.Match(tag); ok {
		return nil
	}
	return fmt.Errorf("%w: %s", ErrInvalidWrite, i18n.ErrUnsupportedLanguage)
}
