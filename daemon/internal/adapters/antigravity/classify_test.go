package antigravity

import (
	"net/http"
	"testing"
	"time"
)

// TestRetryAfter pins each throttle to the clock that clears it. A spent quota
// is the one that must not wait: it reads as the same 429 an ordinary rate
// limit does, and honouring it as one would hold an account for half a minute
// that another account clears at once.
func TestRetryAfter(t *testing.T) {
	cases := []struct {
		name   string
		status int
		code   string
		detail string
		want   time.Duration
		spread bool
	}{
		{"a rate limit waits for the bucket", http.StatusTooManyRequests, "", "", rateLimitWait, false},
		{"the gRPC status names the same throttle", http.StatusBadRequest, "RESOURCE_EXHAUSTED", "", rateLimitWait, false},
		{"an overloaded model waits around a minute", http.StatusTooManyRequests, "", "Model capacity exhausted", capacityWindow, true},
		{"a spent quota does not wait at all", http.StatusTooManyRequests, "", "You exhausted your capacity on this model", 0, false},
		{"a refusal says nothing about waiting", http.StatusForbidden, "PERMISSION_DENIED", "Caller does not have permission", 0, false},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			got := RetryAfter(test.status, test.code, test.detail)
			if test.spread {
				if got < test.want || got >= test.want+capacitySpread {
					t.Fatalf("RetryAfter() = %v, want a wait inside [%v, %v)", got, test.want, test.want+capacitySpread)
				}
				return
			}
			if got != test.want {
				t.Fatalf("RetryAfter() = %v, want %v", got, test.want)
			}
		})
	}
}

// TestRetryAfterSpreadsAnOverloadedModel covers the jitter: every client
// backing off at one instant arrives again at that same instant.
func TestRetryAfterSpreadsAnOverloadedModel(t *testing.T) {
	seen := map[time.Duration]bool{}
	for range 40 {
		got := RetryAfter(http.StatusTooManyRequests, "", "Model capacity exhausted")
		if got < capacityWindow || got >= capacityWindow+capacitySpread {
			t.Fatalf("RetryAfter() = %v, want a wait inside [%v, %v)", got, capacityWindow, capacityWindow+capacitySpread)
		}
		seen[got] = true
	}
	if len(seen) < 2 {
		t.Fatal("every caller got the same wait, so they all return at the same instant")
	}
}
