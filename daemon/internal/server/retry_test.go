package server

import (
	"context"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/internal/config"
)

// TestRetryDelayStaysInsideItsWindow pins the three shipped waits the retry
// schedule promises: 1–3s before the first retry, 3–5s before the second, and
// 5–10s before the third.
func TestRetryDelayStaysInsideItsWindow(t *testing.T) {
	windows := config.DefaultUpstreamRetryBackoff()
	for retry, window := range windows {
		low := time.Duration(window[0]) * time.Second
		high := time.Duration(window[1]) * time.Second
		for sample := 0; sample < 100; sample++ {
			delay := retryDelay(windows, retry)
			if delay < low || delay >= high {
				t.Fatalf("retryDelay(%d) = %v, want [%v, %v)", retry, delay, low, high)
			}
		}
	}
	if delay := retryDelay(windows, 3); delay != 0 {
		t.Fatalf("retryDelay(3) = %v, want no wait beyond the three retries", delay)
	}
}

// TestRetryDelayUsesTheResolvedWindows covers an operator's windows replacing
// the shipped ones.
func TestRetryDelayUsesTheResolvedWindows(t *testing.T) {
	windows := [][2]int{{10, 11}, {20, 21}, {30, 31}}
	for retry, window := range windows {
		low := time.Duration(window[0]) * time.Second
		high := time.Duration(window[1]) * time.Second
		delay := retryDelay(windows, retry)
		if delay < low || delay >= high {
			t.Fatalf("retryDelay(%d) = %v, want [%v, %v)", retry, delay, low, high)
		}
	}
}

// TestRetryWindowsPreferTheConnection covers the resolution: a connection's
// own windows replace the global ones, and a connection without them uses the
// global windows.
func TestRetryWindowsPreferTheConnection(t *testing.T) {
	global := [][2]int{{1, 3}, {3, 5}, {5, 10}}
	own := [][2]int{{2, 4}, {4, 6}, {6, 8}}

	if got := retryWindowsFor(catalog.Provider{}, global); got[0] != global[0] {
		t.Fatalf("retryWindowsFor(no override) = %v, want the global windows", got)
	}
	if got := retryWindowsFor(catalog.Provider{RetryBackoff: own}, global); got[1] != own[1] {
		t.Fatalf("retryWindowsFor(override) = %v, want the connection's windows", got)
	}
}

// TestWaitBeforeRetryStopsWithTheClientContext covers the disconnect that
// cancels the wait: a request whose client is gone must not keep retrying.
func TestWaitBeforeRetryStopsWithTheClientContext(t *testing.T) {
	served := New(Options{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if served.waitBeforeRetry(ctx, config.DefaultUpstreamRetryBackoff(), 0, 0) {
		t.Fatal("waitBeforeRetry() = true, want the cancelled client to stop the retries")
	}
}

// TestWaitBeforeRetryHonoursALongerRetryAfter covers a provider naming a wait
// longer than the drawn window: the provider's own Retry-After wins.
func TestWaitBeforeRetryHonoursALongerRetryAfter(t *testing.T) {
	served := New(Options{RetryDelay: func(int) time.Duration { return time.Millisecond }})
	started := time.Now()
	if !served.waitBeforeRetry(context.Background(), config.DefaultUpstreamRetryBackoff(), 0, 25*time.Millisecond) {
		t.Fatal("waitBeforeRetry() = false, want the wait to finish")
	}
	if elapsed := time.Since(started); elapsed < 25*time.Millisecond {
		t.Fatalf("waitBeforeRetry() waited %v, want at least the provider's Retry-After", elapsed)
	}
}
