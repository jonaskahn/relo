// Usage metric choices: validation and startup-file storage.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/BurntSushi/toml"
)

// Usage-metric errors name the refusals a console maps to its own wording:
// an unknown card and a set outside the allowed size.
var (
	usageMetricsMu      sync.Mutex
	usageSectionHeader  = regexp.MustCompile(`^\s*\[ui\.usage\]\s*(?:#.*)?$`)
	usageMetricsLine    = regexp.MustCompile(`^(\s*)overview_metrics(\s*=\s*)(.*)$`)
	ErrInvalidMetric    = errors.New("unknown usage metric")
	ErrInvalidMetricSet = errors.New("usage metrics must name between 6 and 10 distinct metrics")
)

// MinOverviewMetrics and MaxOverviewMetrics bound how many cards the usage
// overview shows, so the page stays scannable whatever an operator picks.
const (
	MinOverviewMetrics = 6
	MaxOverviewMetrics = 10
)

// UsageMetricIDs lists every metric the usage overview may show, in the order
// the picker offers them. The console mirrors this list by identifier. The
// copy it returns keeps callers from mutating the shipped list.
func UsageMetricIDs() []string {
	return []string{
		"requests",
		"successes",
		"errors",
		"success_rate",
		"error_rate",
		"attempts",
		"retried",
		"tokens_input",
		"tokens_output",
		"tokens_total",
		"tokens_per_request",
		"cache_read",
		"cache_write",
		"spend",
		"plan_usage",
		"spend_per_request",
		"priced_requests",
		"unpriced_requests",
		"duration_avg",
		"duration_max",
	}
}

// RetiredUsageMetricIDs names metrics an older install may still store.
// They are dropped on read so an existing config keeps loading; writes stay
// strict and never accept them again. The copy it returns keeps callers from
// mutating the shipped list.
func RetiredUsageMetricIDs() []string {
	return []string{
		"cache_read_share",
	}
}

// DefaultOverviewMetrics is what an install that never chose metrics shows:
// volume, cost, reliability, and speed, in that order. The copy it returns
// keeps callers from mutating the shipped choice.
func DefaultOverviewMetrics() []string {
	return []string{
		"requests",
		"tokens_total",
		"spend",
		"plan_usage",
		"success_rate",
		"error_rate",
		"duration_avg",
		"retried",
		"cache_read",
	}
}

// ValidateUsageMetrics reports the first choice a usage overview cannot show.
func ValidateUsageMetrics(metrics []string) error {
	if len(metrics) < MinOverviewMetrics || len(metrics) > MaxOverviewMetrics {
		return fmt.Errorf("overview_metrics: %w (got %d)", ErrInvalidMetricSet, len(metrics))
	}
	known := make(map[string]bool, len(UsageMetricIDs()))
	for _, id := range UsageMetricIDs() {
		known[id] = true
	}
	seen := make(map[string]bool, len(metrics))
	for _, id := range metrics {
		if !known[id] {
			return fmt.Errorf("overview_metrics: %q: %w", id, ErrInvalidMetric)
		}
		if seen[id] {
			return fmt.Errorf("overview_metrics: %q: %w", id, ErrInvalidMetricSet)
		}
		seen[id] = true
	}
	return nil
}

// ReadUsageMetrics reads the metric choice straight from the file, so a
// change another process made is picked up without a restart.
func ReadUsageMetrics(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return append([]string(nil), DefaultOverviewMetrics()...), nil
	}
	if err != nil {
		return nil, err
	}
	return decodeUsageMetrics(data)
}

func decodeUsageMetrics(data []byte) ([]string, error) {
	var file struct {
		UI UIConfig `toml:"ui"`
	}
	file.UI.Usage.OverviewMetrics = append([]string(nil), DefaultOverviewMetrics()...)
	if _, err := toml.Decode(string(data), &file); err != nil {
		return nil, err
	}
	metrics := dropRetiredUsageMetrics(file.UI.Usage.OverviewMetrics)
	if len(metrics) == 0 {
		return append([]string(nil), DefaultOverviewMetrics()...), nil
	}
	if err := ValidateUsageMetrics(metrics); err != nil {
		return nil, err
	}
	return append([]string(nil), metrics...), nil
}

func dropRetiredUsageMetrics(metrics []string) []string {
	retired := make(map[string]bool, len(RetiredUsageMetricIDs()))
	for _, id := range RetiredUsageMetricIDs() {
		retired[id] = true
	}
	kept := make([]string, 0, len(metrics))
	for _, id := range metrics {
		if !retired[id] {
			kept = append(kept, id)
		}
	}
	if len(kept) < MinOverviewMetrics {
		return append([]string(nil), DefaultOverviewMetrics()...)
	}
	return kept
}

// UpdateUsageMetrics replaces the chosen metrics in place, keeping every
// other key, comment, and section of the startup file.
func UpdateUsageMetrics(path string, metrics []string) ([]string, error) {
	usageMetricsMu.Lock()
	defer usageMetricsMu.Unlock()
	if err := ValidateUsageMetrics(metrics); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	updated := replaceUsageMetrics(string(data), metrics)
	if _, err := decodeUsageMetrics([]byte(updated)); err != nil {
		return nil, fmt.Errorf("update usage metrics: %w", err)
	}
	if err := writeFileAtomically(path, updated); err != nil {
		return nil, err
	}
	return append([]string(nil), metrics...), nil
}

func replaceUsageMetrics(source string, metrics []string) string {
	lines := strings.SplitAfter(source, "\n")
	value := renderUsageMetrics(metrics)
	paint := &usageMetricsPaint{lines: lines, value: value}
	for i := 0; i < len(paint.lines); i++ {
		if done, result := paint.visitLine(i); done {
			return result
		}
	}
	return paint.finish()
}

type usageMetricsPaint struct {
	lines        []string
	value        string
	inside       bool
	foundSection bool
}

func (p *usageMetricsPaint) visitLine(i int) (bool, string) {
	trimmed := strings.TrimRight(p.lines[i], "\r\n")
	if usageSectionHeader.MatchString(trimmed) {
		p.inside, p.foundSection = true, true
		return false, ""
	}
	if p.inside && tableHeader.MatchString(trimmed) {
		// The section ended without the key, so it is added before the
		// next heading keeps the file's sections in order.
		p.lines[i] = metricLine(p.value) + p.lines[i]
		return true, strings.Join(p.lines, "")
	}
	if !p.inside {
		return false, ""
	}
	// The newline is stripped before matching: the pattern anchors on the
	// end of the line, and a line that ends in one would never match it.
	body := strings.TrimRight(p.lines[i], "\r\n")
	if parts := usageMetricsLine.FindStringSubmatch(body); parts != nil {
		end := metricValueEnd(p.lines, i, parts[3])
		p.lines[i] = parts[1] + "overview_metrics" + parts[2] +
			p.value + lineEnding(p.lines[end])
		p.lines = append(p.lines[:i+1], p.lines[end+1:]...)
		return true, strings.Join(p.lines, "")
	}
	return false, ""
}

func (p *usageMetricsPaint) finish() string {
	result := strings.Join(p.lines, "")
	if p.foundSection {
		if !strings.HasSuffix(result, "\n") {
			result += "\n"
		}
		return result + metricLine(p.value)
	}
	if result != "" && !strings.HasSuffix(result, "\n") {
		result += "\n"
	}
	return result + "\n[ui.usage]\n" + metricLine(p.value)
}

func metricLine(value string) string {
	return "overview_metrics = " + value + "\n"
}

func metricValueEnd(lines []string, i int, remainder string) int {
	balance := strings.Count(remainder, "[") - strings.Count(remainder, "]")
	end := i
	for balance > 0 && end+1 < len(lines) {
		end++
		body := strings.TrimRight(lines[end], "\r\n")
		balance += strings.Count(body, "[") - strings.Count(body, "]")
	}
	return end
}

func renderUsageMetrics(metrics []string) string {
	quoted := make([]string, 0, len(metrics))
	for _, metric := range metrics {
		quoted = append(quoted, `"`+metric+`"`)
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}

func lineEnding(line string) string {
	body := strings.TrimRight(line, "\r\n")
	return line[len(body):]
}

func writeFileAtomically(path, content string) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".config-*.toml")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if err := tmp.Chmod(configMode); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.WriteString(content); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
