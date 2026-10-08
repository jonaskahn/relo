// Rate-limit windows: parsing vendor response headers.
package quota

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/jonaskahn/relo/internal/activity"
)

// WindowsFromHeaders reads the quota windows a provider reported on a live
// response. Codex publishes its two windows as a percentage over a window
// length, Claude as a fraction with a reset time, and one response carries
// one family, so both are read and the family that is absent contributes
// nothing.
func WindowsFromHeaders(header http.Header) []activity.WindowSample {
	if header == nil {
		return nil
	}
	windows := codexHeaderWindows(header)
	return append(windows, claudeHeaderWindows(header)...)
}

func codexHeaderWindows(header http.Header) []activity.WindowSample {
	windows := make([]activity.WindowSample, 0, 2)
	for _, prefix := range []string{"x-codex-primary", "x-codex-secondary"} {
		used, found := floatHeader(header, prefix+"-used-percent")
		if !found {
			continue
		}
		minutes, _ := intHeader(header, prefix+"-window-minutes")
		reset, _ := intHeader(header, prefix+"-reset-at")
		seconds := minutes * 60
		windows = append(windows, activity.WindowSample{
			Window: windowName(seconds), UsedPercent: clampPercent(used), ResetAt: reset, Seconds: seconds,
		})
	}
	return windows
}

func claudeHeaderWindows(header http.Header) []activity.WindowSample {
	known := []struct {
		window  string
		prefix  string
		seconds int64
	}{
		{activity.WindowFiveHours, "anthropic-ratelimit-unified-5h", 5 * 60 * 60},
		{activity.WindowSevenDays, "anthropic-ratelimit-unified-7d", 7 * 24 * 60 * 60},
	}
	windows := make([]activity.WindowSample, 0, len(known))
	for _, entry := range known {
		used, found := floatHeader(header, entry.prefix+"-utilization")
		if !found {
			continue
		}
		reset, _ := intHeader(header, entry.prefix+"-reset")
		windows = append(windows, activity.WindowSample{
			Window: entry.window, UsedPercent: clampPercent(used * 100),
			ResetAt: reset, Seconds: entry.seconds,
		})
	}
	return windows
}

func windowName(seconds int64) string {
	switch {
	case seconds <= 0:
		return ""
	case seconds == 5*60*60:
		return activity.WindowFiveHours
	case seconds == 7*24*60*60:
		return activity.WindowSevenDays
	case seconds%(60*60) == 0:
		return strconv.FormatInt(seconds/(60*60), 10) + "h"
	case seconds%60 == 0:
		return strconv.FormatInt(seconds/60, 10) + "m"
	default:
		return strconv.FormatInt(seconds, 10) + "s"
	}
}

func clampPercent(value float64) float64 {
	switch {
	case value < 0:
		return 0
	case value > 100:
		return 100
	default:
		return value
	}
}

func floatHeader(header http.Header, name string) (float64, bool) {
	raw := strings.TrimSpace(header.Get(name))
	if raw == "" {
		return 0, false
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, false
	}
	return value, true
}

func intHeader(header http.Header, name string) (int64, bool) {
	raw := strings.TrimSpace(header.Get(name))
	if raw == "" {
		return 0, false
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, false
	}
	return value, true
}
