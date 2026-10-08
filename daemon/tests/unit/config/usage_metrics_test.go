package config_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/config"
	"github.com/jonaskahn/relo/tests/testkit"
)

// TestReadUsageMetricsFallsBackToTheDefaults covers the choice an install
// that never picked one reports.
func TestReadUsageMetricsFallsBackToTheDefaults(t *testing.T) {
	path := filepath.Join(testkit.TempHome(t), "config.toml")
	metrics, err := config.ReadUsageMetrics(path)
	if err != nil {
		t.Fatalf("ReadUsageMetrics() error = %v", err)
	}
	if !slices.Equal(metrics, config.DefaultOverviewMetrics()) {
		t.Fatalf("metrics = %v, want the defaults", metrics)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("reading the metrics wrote a config file")
	}
}

// TestUpdateUsageMetricsKeepsTheRestOfTheFile covers the write the picker
// makes: the choice lands in the shared startup file, and every other key and
// comment survives it.
func TestUpdateUsageMetricsKeepsTheRestOfTheFile(t *testing.T) {
	home := testkit.TempHome(t)
	path := config.ConfigPath(home)
	source := strings.Join([]string{
		"# my relo config",
		"[server]",
		"port = 12345",
		"",
		"[ui]",
		`language = "de"`,
		`accent = "blue"`,
		"",
		"[logging]",
		`level = "debug"`,
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	chosen := []string{"requests", "errors", "spend", "duration_avg", "attempts", "cache_read"}
	stored, err := config.UpdateUsageMetrics(path, chosen)
	if err != nil {
		t.Fatalf("UpdateUsageMetrics() error = %v", err)
	}
	if !slices.Equal(stored, chosen) {
		t.Fatalf("stored = %v, want the chosen metrics", stored)
	}
	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	for _, kept := range []string{"# my relo config", "port = 12345", `language = "de"`, `accent = "blue"`, `level = "debug"`, "[ui.usage]"} {
		if !strings.Contains(string(written), kept) {
			t.Fatalf("the config lost %q:\n%s", kept, written)
		}
	}
	if strings.Count(string(written), "overview_metrics") != 1 {
		t.Fatalf("the config holds %d metric keys:\n%s", strings.Count(string(written), "overview_metrics"), written)
	}
	read, err := config.ReadUsageMetrics(path)
	if err != nil {
		t.Fatalf("ReadUsageMetrics() error = %v", err)
	}
	if !slices.Equal(read, chosen) {
		t.Fatalf("ReadUsageMetrics() = %v, want %v", read, chosen)
	}
	// A loaded configuration answers with the same choice, which is what
	// every surface reads.
	loaded, err := config.LoadConfig(path, nil)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if !slices.Equal(loaded.UI.Usage.OverviewMetrics, chosen) {
		t.Fatalf("loaded metrics = %v, want %v", loaded.UI.Usage.OverviewMetrics, chosen)
	}
	if err := config.ValidateConfig(loaded); err != nil {
		t.Fatalf("ValidateConfig() error = %v", err)
	}
}

// TestUpdateUsageMetricsReplacesInPlace covers the second save: the key is
// rewritten where it stands, however it was laid out, and no second key
// appears.
func TestUpdateUsageMetricsReplacesInPlace(t *testing.T) {
	path := filepath.Join(testkit.TempHome(t), "config.toml")
	source := "[ui.usage]\noverview_metrics = [\n  \"requests\",\n  \"errors\",\n  \"spend\",\n  \"duration_avg\",\n  \"attempts\",\n  \"retried\"\n]\n\n[logging]\nlevel = \"info\"\n"
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	chosen := []string{"requests", "errors", "spend", "duration_avg", "attempts", "unpriced_requests"}
	if _, err := config.UpdateUsageMetrics(path, chosen); err != nil {
		t.Fatalf("UpdateUsageMetrics() error = %v", err)
	}
	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if strings.Count(string(written), "overview_metrics") != 1 {
		t.Fatalf("the config holds %d metric keys:\n%s", strings.Count(string(written), "overview_metrics"), written)
	}
	if strings.Contains(string(written), "\"retried\"") {
		t.Fatalf("the replaced list kept an old entry:\n%s", written)
	}
	if !strings.Contains(string(written), "[logging]") {
		t.Fatalf("the section after the list was lost:\n%s", written)
	}
	read, err := config.ReadUsageMetrics(path)
	if err != nil {
		t.Fatalf("ReadUsageMetrics() error = %v", err)
	}
	if !slices.Equal(read, chosen) {
		t.Fatalf("ReadUsageMetrics() = %v, want %v", read, chosen)
	}
}

// TestReadUsageMetricsDropsARetiredChoice covers an install that stored a
// metric this build retired: the file keeps loading with the retired entry
// dropped instead of failing validation.
func TestReadUsageMetricsDropsARetiredChoice(t *testing.T) {
	path := filepath.Join(testkit.TempHome(t), "config.toml")
	source := "[ui.usage]\noverview_metrics = [\"requests\", \"errors\", \"spend\", \"duration_avg\", \"attempts\", \"cache_read_share\", \"retried\"]\n"
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	read, err := config.ReadUsageMetrics(path)
	if err != nil {
		t.Fatalf("ReadUsageMetrics() error = %v", err)
	}
	want := []string{"requests", "errors", "spend", "duration_avg", "attempts", "retried"}
	if !slices.Equal(read, want) {
		t.Fatalf("ReadUsageMetrics() = %v, want %v", read, want)
	}
}

// TestUpdateUsageMetricsRefusesABadSet covers what the picker cannot save: a
// set outside the window, a duplicate, and a metric this build does not know.
func TestUpdateUsageMetricsRefusesABadSet(t *testing.T) {
	path := filepath.Join(testkit.TempHome(t), "config.toml")
	cases := []struct {
		name    string
		metrics []string
		want    error
	}{
		{"too few", []string{"requests", "errors", "spend"}, config.ErrInvalidMetricSet},
		{"too many", []string{"requests", "successes", "errors", "success_rate", "error_rate",
			"attempts", "retried", "tokens_input", "tokens_output", "tokens_total", "spend"}, config.ErrInvalidMetricSet},
		{"duplicate", []string{"requests", "requests", "errors", "spend", "attempts", "retried"}, config.ErrInvalidMetricSet},
		{"unknown", []string{"requests", "errors", "spend", "attempts", "retried", "invented"}, config.ErrInvalidMetric},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if _, err := config.UpdateUsageMetrics(path, testCase.metrics); !errors.Is(err, testCase.want) {
				t.Fatalf("UpdateUsageMetrics(%v) error = %v, want %v", testCase.metrics, err, testCase.want)
			}
			if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("a refused choice still wrote the config file")
			}
		})
	}
}
