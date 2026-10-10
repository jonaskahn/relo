package config_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/config"
)

// TestValidateUpstreamTimeoutAcceptsOnlyThePresets pins the choices the
// Settings page offers: anything else is refused rather than stored and
// silently capped.
func TestValidateUpstreamTimeoutAcceptsOnlyThePresets(t *testing.T) {
	for _, seconds := range config.UpstreamTimeoutPresets() {
		if err := config.ValidateUpstreamTimeout(seconds); err != nil {
			t.Errorf("ValidateUpstreamTimeout(%d) = %v, want accepted", seconds, err)
		}
	}
	for _, seconds := range []int{-1, 0, 30, 60, 91, 120, 150, 181, 2400} {
		if err := config.ValidateUpstreamTimeout(seconds); !errors.Is(err, config.ErrInvalidUpstreamTimeout) {
			t.Errorf("ValidateUpstreamTimeout(%d) = %v, want ErrInvalidUpstreamTimeout", seconds, err)
		}
	}
}

// TestValidateUpstreamRetryAttemptsAcceptsOnlyThePresets pins the retry
// counts an operator may pick: 1, 3 or 5.
func TestValidateUpstreamRetryAttemptsAcceptsOnlyThePresets(t *testing.T) {
	for _, attempts := range config.RetryAttemptPresets() {
		if err := config.ValidateUpstreamRetryAttempts(attempts); err != nil {
			t.Errorf("preset %d is refused: %v", attempts, err)
		}
	}
	for _, attempts := range []int{-1, 0, 2, 4, 6} {
		if err := config.ValidateUpstreamRetryAttempts(attempts); !errors.Is(err, config.ErrInvalidUpstreamRetryAttempts) {
			t.Errorf("ValidateUpstreamRetryAttempts(%d) = %v, want ErrInvalidUpstreamRetryAttempts", attempts, err)
		}
	}
}

// TestValidateUpstreamFailoverCooldownAcceptsOnlyThePresets pins the
// escalating waits the Settings page offers: no wait, 3m, 5m, 15m, 30m, 1h,
// and 5h.
func TestValidateUpstreamFailoverCooldownAcceptsOnlyThePresets(t *testing.T) {
	for _, seconds := range config.FailoverCooldownPresets() {
		if err := config.ValidateUpstreamFailoverCooldown(seconds); err != nil {
			t.Errorf("ValidateUpstreamFailoverCooldown(%d) = %v, want accepted", seconds, err)
		}
	}
	if got := len(config.FailoverCooldownPresets()); got != 7 {
		t.Fatalf("presets = %d, want the none start plus the six waits the settings page offers", got)
	}
	if config.DefaultFailoverCooldownSeconds != 0 {
		t.Fatalf("default = %d, want the first step without a wait", config.DefaultFailoverCooldownSeconds)
	}
	for _, seconds := range []int{-1, 60, 150, 179, 301, 901, 3601} {
		if err := config.ValidateUpstreamFailoverCooldown(seconds); !errors.Is(err, config.ErrInvalidUpstreamFailoverCooldown) {
			t.Errorf("ValidateUpstreamFailoverCooldown(%d) = %v, want ErrInvalidUpstreamFailoverCooldown", seconds, err)
		}
	}
}

// TestFailoverBackoff pins the ladder a chosen preset walks: the preset
// itself, then every longer one, and a failure past its end stays on the
// longest step.
func TestFailoverBackoff(t *testing.T) {
	steps := config.FailoverBackoff(0)
	want := []time.Duration{0, 3 * time.Minute, 5 * time.Minute, 15 * time.Minute, 30 * time.Minute, time.Hour, 5 * time.Hour}
	if !slices.Equal(steps, want) {
		t.Fatalf("FailoverBackoff(0) = %v, want the full ladder %v", steps, want)
	}
	last := config.FailoverBackoff(18000)
	if len(last) != 1 || last[0] != 5*time.Hour {
		t.Fatalf("FailoverBackoff(18000) = %v, want only the last step", last)
	}
}

// TestUpstreamFailoverCooldownRoundTrip covers the denylist wait the console
// stores: a missing file or key answers the default, a stored preset replaces
// it, and a retired value falls back to the default.
func TestUpstreamFailoverCooldownRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := config.UpdateUpstreamFailoverCooldown(path, 1800); err != nil {
		t.Fatalf("UpdateUpstreamFailoverCooldown() error = %v", err)
	}
	if seconds, err := config.ReadUpstreamFailoverCooldown(path); err != nil || seconds != 1800 {
		t.Fatalf("ReadUpstreamFailoverCooldown() = %d, %v, want the stored preset", seconds, err)
	}
	if err := config.UpdateUpstreamFailoverCooldown(path, 0); err != nil {
		t.Fatalf("UpdateUpstreamFailoverCooldown(0) error = %v, want no denylist stored", err)
	}
	if seconds, err := config.ReadUpstreamFailoverCooldown(path); err != nil || seconds != 0 {
		t.Fatalf("ReadUpstreamFailoverCooldown() = %d, %v, want no denylist", seconds, err)
	}
	if err := config.UpdateUpstreamFailoverCooldown(path, 301); !errors.Is(err, config.ErrInvalidUpstreamFailoverCooldown) {
		t.Fatalf("UpdateUpstreamFailoverCooldown() = %v, want ErrInvalidUpstreamFailoverCooldown", err)
	}
	retired := filepath.Join(t.TempDir(), "retired.toml")
	if err := os.WriteFile(retired, []byte("[upstream]\nfailover_cooldown_seconds = 45\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if seconds, err := config.ReadUpstreamFailoverCooldown(retired); err != nil || seconds != config.DefaultFailoverCooldownSeconds {
		t.Fatalf("ReadUpstreamFailoverCooldown(retired) = %d, %v, want the default", seconds, err)
	}
}

// TestUpstreamTimeoutRoundTripThroughTheStartupFile covers the write and read
// the console performs, including the three shapes the file can be in: a file
// with no upstream table, one whose table has no value, and one that already
// stores a choice. Every other setting in the file has to survive.
func TestUpstreamTimeoutRoundTripThroughTheStartupFile(t *testing.T) {
	for name, original := range map[string]string{
		"no upstream table": "# keep this comment\n[server]\nport = 12345\n\n[ui]\nlanguage = \"de\"\n",
		"empty table":       "[server]\nport = 12345\n\n[upstream]\n\n[logging]\nlevel = \"debug\"\n",
		"existing value":    "[server]\nport = 12345\n\n[upstream]\ntimeout_seconds = 180\n\n[logging]\nlevel = \"debug\"\n",
		"absent file":       "",
		"comment on head":   "# keep this comment\n[server]\nport = 12345\n",
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.toml")
			if original != "" {
				if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
					t.Fatal(err)
				}
			}

			if err := config.UpdateUpstreamTimeout(path, 300); err != nil {
				t.Fatalf("UpdateUpstreamTimeout() error = %v", err)
			}
			seconds, err := config.ReadUpstreamTimeout(path)
			if err != nil || seconds != 300 {
				t.Fatalf("ReadUpstreamTimeout() = %d, %v, want the stored preset", seconds, err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			// The rest of the file is the operator's, not ours to rewrite.
			for _, text := range []string{"port = 12345", "language = \"de\"", "level = \"debug\"", "# keep this comment"} {
				if strings.Contains(original, text) && !strings.Contains(string(data), text) {
					t.Fatalf("lost %q in %s", text, data)
				}
			}

			// Writing over the stored value replaces it rather than adding a
			// second key the file would then read as invalid.
			if err := config.UpdateUpstreamTimeout(path, 600); err != nil {
				t.Fatal(err)
			}
			data, err = os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.Count(string(data), "timeout_seconds"); got != 1 {
				t.Fatalf("the file names a timeout %d times, want once:\n%s", got, data)
			}
			if seconds, err = config.ReadUpstreamTimeout(path); err != nil || seconds != 600 {
				t.Fatalf("ReadUpstreamTimeout() = %d, %v, want the replacement", seconds, err)
			}
		})
	}
}

// TestReadUpstreamTimeoutDefaultsAndNormalizes covers what an operator can
// find stored before the console ever saves one: a missing file or key
// answers the default, a retired preset also reads as the default, and a file
// that is not TOML is reported rather than quietly replaced.
func TestReadUpstreamTimeoutDefaultsAndNormalizes(t *testing.T) {
	dir := t.TempDir()

	seconds, err := config.ReadUpstreamTimeout(filepath.Join(dir, "absent.toml"))
	if err != nil || seconds != config.DefaultUpstreamTimeoutSeconds {
		t.Fatalf("ReadUpstreamTimeout(absent) = %d, %v, want the default", seconds, err)
	}

	withoutKey := filepath.Join(dir, "without-key.toml")
	if err := os.WriteFile(withoutKey, []byte("[server]\nport = 12345\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if seconds, err = config.ReadUpstreamTimeout(withoutKey); err != nil || seconds != config.DefaultUpstreamTimeoutSeconds {
		t.Fatalf("ReadUpstreamTimeout(without key) = %d, %v, want the default", seconds, err)
	}

	retired := filepath.Join(dir, "retired.toml")
	if err := os.WriteFile(retired, []byte("[upstream]\ntimeout_seconds = 150\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if seconds, err = config.ReadUpstreamTimeout(retired); err != nil || seconds != config.DefaultUpstreamTimeoutSeconds {
		t.Fatalf("ReadUpstreamTimeout(retired) = %d, %v, want the default", seconds, err)
	}

	broken := filepath.Join(dir, "broken.toml")
	if err := os.WriteFile(broken, []byte("this is not = = toml\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err = config.ReadUpstreamTimeout(broken); err == nil {
		t.Fatal("ReadUpstreamTimeout() accepted a file that is not TOML")
	}
}

// TestUpstreamRetryAttemptsRoundTrip covers the count the console stores: a
// missing file or key answers the default, a stored count replaces it, and a
// retired value falls back to the default rather than breaking the next
// request.
func TestUpstreamRetryAttemptsRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if stored, err := config.ReadUpstreamRetryAttempts(path); err != nil || stored != config.DefaultUpstreamRetryAttempts {
		t.Fatalf("ReadUpstreamRetryAttempts(missing) = %v, %v, want the default", stored, err)
	}
	if err := config.UpdateUpstreamRetryAttempts(path, 5); err != nil {
		t.Fatalf("UpdateUpstreamRetryAttempts() error = %v", err)
	}
	stored, err := config.ReadUpstreamRetryAttempts(path)
	if err != nil || stored != 5 {
		t.Fatalf("ReadUpstreamRetryAttempts() = %v, %v, want the stored count", stored, err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(string(data), "retry_attempts"); got != 1 {
		t.Fatalf("the file names retry_attempts %d times, want once:\n%s", got, data)
	}

	retired := filepath.Join(t.TempDir(), "retired.toml")
	if err := os.WriteFile(retired, []byte("[upstream]\nretry_attempts = 2\nretry_backoff = [[1, 3], [3, 5]]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if stored, err = config.ReadUpstreamRetryAttempts(retired); err != nil || stored != config.DefaultUpstreamRetryAttempts {
		t.Fatalf("ReadUpstreamRetryAttempts(retired) = %v, %v, want the default", stored, err)
	}
}

// TestUpdateUpstreamTimeoutRefusesAValueNothingUses keeps an unpickable call
// wait from ever reaching the startup file.
func TestUpdateUpstreamTimeoutRefusesAValueNothingUses(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	original := "[server]\nport = 12345\n"
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := config.UpdateUpstreamTimeout(path, 45); !errors.Is(err, config.ErrInvalidUpstreamTimeout) {
		t.Fatalf("UpdateUpstreamTimeout() = %v, want ErrInvalidUpstreamTimeout", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != original {
		t.Fatalf("a refused write changed the file to %s", data)
	}
}

// TestUpdateUpstreamRetryAttemptsRefusesAnInvalidCount keeps a count nothing
// offers out of the startup file.
func TestUpdateUpstreamRetryAttemptsRefusesAnInvalidCount(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	original := "[server]\nport = 12345\n"
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := config.UpdateUpstreamRetryAttempts(path, 4); !errors.Is(err, config.ErrInvalidUpstreamRetryAttempts) {
		t.Fatalf("UpdateUpstreamRetryAttempts() = %v, want ErrInvalidUpstreamRetryAttempts", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != original {
		t.Fatalf("a refused write changed the file to %s", data)
	}
}
