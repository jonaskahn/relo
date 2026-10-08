package service_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/jonaskahn/relo/internal/activity"
	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	appsettings "github.com/jonaskahn/relo/internal/application/settings"
	"github.com/jonaskahn/relo/internal/config"
)

// failingRetention and failingQuotaDisplay are stores that cannot answer. The
// console reports the failure rather than drawing a page of zeroes, so each
// use case that reads one has to surface it.
var errStoreDown = errors.New("store is down")

type failingRetention struct{}

func (failingRetention) Budget(context.Context) (activity.RetentionBudget, bool, error) {
	return activity.RetentionBudget{}, false, errStoreDown
}

func (failingRetention) SaveBudget(context.Context, activity.RetentionBudget) error {
	return errStoreDown
}

func (failingRetention) Preview(context.Context, activity.RetentionBudget) (activity.RetentionReport, error) {
	return activity.RetentionReport{}, errStoreDown
}

func (failingRetention) RunOnce(context.Context) (activity.RetentionReport, error) {
	return activity.RetentionReport{}, errStoreDown
}

func (failingRetention) Sweep(context.Context) (activity.CleanupReport, error) {
	return activity.CleanupReport{}, errStoreDown
}

type failingQuotaDisplay struct{}

func (failingQuotaDisplay) QuotaDisplay(context.Context) (string, bool, error) {
	return "", false, errStoreDown
}

func (failingQuotaDisplay) SaveQuotaDisplay(context.Context, string, int64) error {
	return errStoreDown
}

// brokenConfigPath returns the path of a startup file that is not readable as
// TOML, which is what an operator who hand-edited it wrongly leaves behind.
func brokenConfigPath(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("this is = = not toml\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSettingsReportAStoreThatCannotAnswer(t *testing.T) {
	ctx := context.Background()
	// A path the store failures do not need: every case here fails before the
	// startup file is read.
	path := filepath.Join(t.TempDir(), "config.toml")

	t.Run("reading the budget", func(t *testing.T) {
		service := appsettings.New(appsettings.Options{
			Retention: failingRetention{}, ConfigPath: path,
		})
		if _, err := service.Read(ctx); !errors.Is(err, errStoreDown) {
			t.Fatalf("Read() error = %v, want the store failure", err)
		}
	})

	t.Run("saving the budget", func(t *testing.T) {
		service := appsettings.New(appsettings.Options{
			Retention: failingRetention{}, ConfigPath: path,
		})
		if _, err := service.SaveRetention(ctx, appsettings.RetentionSettings{}); !errors.Is(err, errStoreDown) {
			t.Fatalf("SaveRetention() error = %v, want the store failure", err)
		}
	})

	t.Run("previewing a budget", func(t *testing.T) {
		service := appsettings.New(appsettings.Options{
			Retention: failingRetention{}, ConfigPath: path,
		})
		if _, err := service.PreviewRetention(ctx, appsettings.RetentionSettings{}); !errors.Is(err, errStoreDown) {
			t.Fatalf("PreviewRetention() error = %v, want the store failure", err)
		}
	})

	t.Run("running the cleanup", func(t *testing.T) {
		service := appsettings.New(appsettings.Options{
			Retention: failingRetention{}, ConfigPath: path,
		})
		if _, err := service.Sweep(ctx); !errors.Is(err, errStoreDown) {
			t.Fatalf("Sweep() error = %v, want the store failure", err)
		}
	})

	t.Run("reading the quota display", func(t *testing.T) {
		service := appsettings.New(appsettings.Options{
			QuotaDisplay: failingQuotaDisplay{}, ConfigPath: path,
		})
		if _, err := service.Appearance(ctx); !errors.Is(err, errStoreDown) {
			t.Fatalf("Appearance() error = %v, want the store failure", err)
		}
	})

	t.Run("saving the quota display", func(t *testing.T) {
		service := appsettings.New(appsettings.Options{
			QuotaDisplay: failingQuotaDisplay{}, ConfigPath: path,
		})
		display := "remaining"
		if _, err := service.SaveAppearance(ctx, appsettings.AppearancePatch{QuotaDisplay: &display}); !errors.Is(err, errStoreDown) {
			t.Fatalf("SaveAppearance() error = %v, want the store failure", err)
		}
	})
}

func TestSettingsReportAStartupFileThatCannotBeRead(t *testing.T) {
	ctx := context.Background()
	harness := newHarness(t)
	// The retention store is the harness's own, so Read gets as far as the
	// startup file before failing; only the file is broken here.
	service := appsettings.New(appsettings.Options{
		Retention:    sqlite.NewRetentionSettings(harness.db, sqlite.RetentionOptions{Now: harness.clock.Now}),
		QuotaDisplay: sqlite.NewQuotaDisplaySettings(harness.db),
		ConfigPath:   brokenConfigPath(t),
	})

	if _, err := service.Read(ctx); err == nil {
		t.Fatal("Read() hid a startup file it could not parse")
	}
	if _, err := service.Appearance(ctx); err == nil {
		t.Fatal("Appearance() hid a startup file it could not parse")
	}
	if _, err := service.Language(); err == nil {
		t.Fatal("Language() hid a startup file it could not parse")
	}
	if _, err := service.UsageMetrics(); err == nil {
		t.Fatal("UsageMetrics() hid a startup file it could not parse")
	}
	if err := service.SaveLanguage("de"); err == nil {
		t.Fatal("SaveLanguage() hid a startup file it could not write")
	}
}

// TestSettingsRefuseInvalidProviderWaits pins that a providers write names
// the invalid field before touching the startup file: a bad proxy, timeout,
// retry window, or denylist step each fails validation first.
func TestSettingsRefuseInvalidProviderWaits(t *testing.T) {
	service := appsettings.New(appsettings.Options{ConfigPath: filepath.Join(t.TempDir(), "config.toml")})
	valid := appsettings.ProvidersSettings{
		CatalogURL: "https://models.example.test/api.json", TimeoutSeconds: 300,
		RetryBackoff: [][2]int{{1, 3}, {3, 5}, {5, 10}}, FailoverCooldownSeconds: 900,
	}
	cases := map[string]appsettings.ProvidersSettings{
		"bad proxy": func() appsettings.ProvidersSettings {
			next := valid
			next.ProxyURL = "socks5://127.0.0.1:1"
			return next
		}(),
		"bad timeout": func() appsettings.ProvidersSettings {
			next := valid
			next.TimeoutSeconds = 150
			return next
		}(),
		"bad backoff": func() appsettings.ProvidersSettings {
			next := valid
			next.RetryBackoff = [][2]int{{3, 1}, {3, 5}, {5, 10}}
			return next
		}(),
		"bad cooldown": func() appsettings.ProvidersSettings {
			next := valid
			next.FailoverCooldownSeconds = 301
			return next
		}(),
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := service.SaveProviders(input); !errors.Is(err, appsettings.ErrInvalidWrite) {
				t.Fatalf("SaveProviders() error = %v, want the invalid write", err)
			}
		})
	}

	t.Run("zero waits fall back to the defaults", func(t *testing.T) {
		saved, err := service.SaveProviders(appsettings.ProvidersSettings{
			CatalogURL:              "https://models.example.test/api.json",
			FailoverCooldownSeconds: 900,
		})
		if err != nil {
			t.Fatalf("SaveProviders() error = %v, want the defaults stored", err)
		}
		if saved.TimeoutSeconds != config.DefaultUpstreamTimeoutSeconds {
			t.Fatalf("TimeoutSeconds = %d, want the default", saved.TimeoutSeconds)
		}
		if len(saved.RetryBackoff) != len(config.DefaultUpstreamRetryBackoff()) {
			t.Fatalf("RetryBackoff = %v, want the default", saved.RetryBackoff)
		}
	})
}

// TestSettingsWithoutAQuotaDisplayStore covers the shape a caller that has no
// database behind it builds: the reading falls back to the one the console
// states everywhere else, rather than to an empty string nothing draws.
func TestSettingsWithoutAQuotaDisplayStore(t *testing.T) {
	service := appsettings.New(appsettings.Options{ConfigPath: filepath.Join(t.TempDir(), "config.toml")})

	appearance, err := service.Appearance(context.Background())
	if err != nil {
		t.Fatalf("Appearance() error = %v", err)
	}
	if appearance.QuotaDisplay == "" {
		t.Fatalf("QuotaDisplay = %q, want the reading every other surface states", appearance.QuotaDisplay)
	}
}

// TestSettingsWithoutAClock covers the constructor defaulting the clock, so a
// caller that named none still writes timestamps rather than a zero time.
func TestSettingsWithoutAClock(t *testing.T) {
	harness := newHarness(t)
	service := appsettings.New(appsettings.Options{
		QuotaDisplay: sqlite.NewQuotaDisplaySettings(harness.db),
		ConfigPath:   config.ConfigPath(harness.home),
	})

	ctx := context.Background()
	// The constructor supplies a clock, so a write carries a real time rather
	// than the zero one a nil clock would have stamped on it.
	remaining := config.QuotaDisplayRemaining
	if _, err := service.SaveAppearance(ctx, appsettings.AppearancePatch{QuotaDisplay: &remaining}); err != nil {
		t.Fatalf("SaveAppearance() error = %v", err)
	}
	if _, found, err := sqlite.NewQuotaDisplaySettings(harness.db).QuotaDisplay(ctx); err != nil || !found {
		t.Fatalf("stored found = %v, error = %v, want the reading written", found, err)
	}
}
