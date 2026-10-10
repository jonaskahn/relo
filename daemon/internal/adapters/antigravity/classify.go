// How long an Antigravity refusal wants the caller to wait.
package antigravity

import (
	"math/rand/v2"
	"net/http"
	"strings"
	"time"
)

// The endpoint names its throttles, and each one clears on its own clock
// rather than on the account's: a rate limit frees the token bucket, an
// overloaded model frees a worker, and a spent quota does not come back until
// the window resets. Retrying sooner than these only lengthens the wait.
const (
	// rateLimitWait clears a token bucket that refills on a short window.
	rateLimitWait = 30 * time.Second

	// capacityWindow is how long an overloaded model usually frees up. The
	// spread is deliberate: every client backing off at the same instant would
	// arrive at the same instant again.
	capacityWindow = 45 * time.Second
	capacitySpread = 15 * time.Second

	// exhaustedMarker is what the endpoint says when the model's quota for the
	// window is gone. Nothing short of the window's own reset clears it, so no
	// wait is named and the caller moves to another account instead.
	exhaustedMarker = "exhausted your capacity on this model"

	// capacityMarker is what it says when the model itself is too busy.
	capacityMarker = "capacity exhausted"

	// resourceExhausted is the gRPC status that names a throttled call.
	resourceExhausted = "RESOURCE_EXHAUSTED"
)

// RetryAfter reports how long a refusal wants the caller to wait, or zero when
// waiting is not the answer. A zero means the caller should try somewhere else,
// not that it should try at once.
//
// The markers are read before the status because a spent quota arrives as the
// same 429 as an ordinary rate limit, and the two want opposite things: one
// clears in half a minute, the other does not clear at all before the window
// resets.
func RetryAfter(status int, code, detail string) time.Duration {
	lowered := strings.ToLower(detail)
	switch {
	case strings.Contains(lowered, exhaustedMarker):
		return 0
	case strings.Contains(lowered, capacityMarker):
		return capacityWindow + time.Duration(rand.Int64N(int64(capacitySpread)))
	case status == http.StatusTooManyRequests || code == resourceExhausted:
		return rateLimitWait
	default:
		return 0
	}
}
