package server

import (
	"context"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/catalog"
)

// TestRandomRetryWaitStaysInsideItsRange pins the one wait the retry schedule
// promises: 1–3s before every retry of an account.
func TestRandomRetryWaitStaysInsideItsRange(t *testing.T) {
	for sample := 0; sample < 200; sample++ {
		delay := randomDelay(retryWaitLow, retryWaitHigh)
		if delay < time.Second || delay >= 3*time.Second {
			t.Fatalf("randomDelay() = %v, want [1s, 3s)", delay)
		}
	}
}

// TestRetryAttemptsPreferTheConnection covers the resolution: a connection's
// own count replaces the global one, and a connection without one uses it.
func TestRetryAttemptsPreferTheConnection(t *testing.T) {
	own := 5
	if got := retryAttemptsFor(catalog.Provider{}, 3); got != 3 {
		t.Fatalf("retryAttemptsFor(no override) = %d, want the global count", got)
	}
	if got := retryAttemptsFor(catalog.Provider{RetryAttempts: &own}, 3); got != 5 {
		t.Fatalf("retryAttemptsFor(override) = %d, want the connection's count", got)
	}
}

// TestWaitBeforeRetryStopsWithTheClientContext covers the disconnect that
// cancels the wait: a request whose client is gone must not keep retrying.
func TestWaitBeforeRetryStopsWithTheClientContext(t *testing.T) {
	served := New(Options{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if served.waitBeforeRetry(ctx, 0, 0) {
		t.Fatal("waitBeforeRetry() = true, want the cancelled client to stop the retries")
	}
}

// TestWaitBeforeRetryHonoursALongerRetryAfter covers a provider naming a wait
// longer than the drawn one: the provider's own Retry-After wins.
func TestWaitBeforeRetryHonoursALongerRetryAfter(t *testing.T) {
	served := New(Options{RetryDelay: func(int) time.Duration { return time.Millisecond }})
	started := time.Now()
	if !served.waitBeforeRetry(context.Background(), 0, 25*time.Millisecond) {
		t.Fatal("waitBeforeRetry() = false, want the wait to finish")
	}
	if elapsed := time.Since(started); elapsed < 25*time.Millisecond {
		t.Fatalf("waitBeforeRetry() waited %v, want at least the provider's Retry-After", elapsed)
	}
}
