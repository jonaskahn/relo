package quota_test

import (
	"net/http"
	"testing"

	"github.com/jonaskahn/relo/internal/activity"
	"github.com/jonaskahn/relo/internal/adapters/quota"
)

// TestWindowsFromHeaders covers what Relo reads off a live response, which
// is the quota a coding client has left without anyone asking the provider
// for it.
func TestWindowsFromHeaders(t *testing.T) {
	t.Run("codex reports its two windows as percentages", func(t *testing.T) {
		header := http.Header{}
		header.Set("x-codex-primary-used-percent", "42")
		header.Set("x-codex-primary-window-minutes", "300")
		header.Set("x-codex-primary-reset-at", "1784817996")
		header.Set("x-codex-secondary-used-percent", "12.5")
		header.Set("x-codex-secondary-window-minutes", "10080")
		header.Set("x-codex-secondary-reset-at", "1785300000")
		windows := quota.WindowsFromHeaders(header)
		if len(windows) != 2 {
			t.Fatalf("windows = %+v, want both Codex windows", windows)
		}
		if windows[0].Window != activity.WindowFiveHours || windows[0].UsedPercent != 42 ||
			windows[0].Seconds != 5*60*60 || windows[0].ResetAt != 1784817996 {
			t.Fatalf("primary window = %+v, want the five-hour window at 42%%", windows[0])
		}
		if windows[1].Window != activity.WindowSevenDays || windows[1].Seconds != 7*24*60*60 {
			t.Fatalf("secondary window = %+v, want the seven-day window", windows[1])
		}
	})

	t.Run("claude reports fractions with reset times", func(t *testing.T) {
		header := http.Header{}
		header.Set("anthropic-ratelimit-unified-5h-utilization", "0.25")
		header.Set("anthropic-ratelimit-unified-5h-reset", "1784817996")
		header.Set("anthropic-ratelimit-unified-7d-utilization", "0.9")
		header.Set("anthropic-ratelimit-unified-7d-reset", "1785300000")
		header.Set("anthropic-ratelimit-unified-status", "allowed")
		windows := quota.WindowsFromHeaders(header)
		if len(windows) != 2 {
			t.Fatalf("windows = %+v, want both Claude windows", windows)
		}
		if windows[0].Window != activity.WindowFiveHours || windows[0].UsedPercent != 25 {
			t.Fatalf("five-hour window = %+v, want the fraction rendered as a percentage", windows[0])
		}
		if windows[1].Window != activity.WindowSevenDays || windows[1].UsedPercent != 90 {
			t.Fatalf("seven-day window = %+v, want 90%%", windows[1])
		}
	})

	t.Run("a window length of an hour or more is named by its length", func(t *testing.T) {
		header := http.Header{}
		header.Set("x-codex-primary-used-percent", "5")
		header.Set("x-codex-primary-window-minutes", "1440")
		windows := quota.WindowsFromHeaders(header)
		if len(windows) != 1 || windows[0].Window != "24h" {
			t.Fatalf("windows = %+v, want a window named 24h", windows)
		}
	})

	t.Run("a response without quota headers reports nothing", func(t *testing.T) {
		header := http.Header{}
		header.Set("Content-Type", "application/json")
		if windows := quota.WindowsFromHeaders(header); len(windows) != 0 {
			t.Fatalf("windows = %+v, want none", windows)
		}
		if windows := quota.WindowsFromHeaders(nil); len(windows) != 0 {
			t.Fatalf("windows = %+v, want none for a missing header set", windows)
		}
	})

	t.Run("an unreadable reading is skipped rather than guessed", func(t *testing.T) {
		header := http.Header{}
		header.Set("x-codex-primary-used-percent", "not-a-number")
		header.Set("x-codex-secondary-used-percent", "10")
		windows := quota.WindowsFromHeaders(header)
		if len(windows) != 1 || windows[0].UsedPercent != 10 {
			t.Fatalf("windows = %+v, want only the window that could be read", windows)
		}
	})

	t.Run("a reading outside the scale is clamped", func(t *testing.T) {
		header := http.Header{}
		header.Set("anthropic-ratelimit-unified-5h-utilization", "1.4")
		windows := quota.WindowsFromHeaders(header)
		if len(windows) != 1 || windows[0].UsedPercent != 100 {
			t.Fatalf("windows = %+v, want the reading clamped to 100", windows)
		}
	})
}
