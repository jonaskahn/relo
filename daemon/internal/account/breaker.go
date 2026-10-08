// Credential breaker: classifying failures and pausing accounts.
package account

import (
	"sync"
	"time"

	"github.com/jonaskahn/relo/internal/clock"
)

// Breaker states are the credential health a pool reads: serving, held
// out of rotation, or testing whether it recovered.
const (
	BreakerClosed   = "closed"
	BreakerOpen     = "open"
	BreakerHalfOpen = "half_open"
)

const (
	breakerCeiling = 5 * time.Hour
)

// DefaultFailoverBackoff is the shipped escalating denylist, one wait per
// consecutive failure: a refused credential is immediately reusable after its
// first failure, and waits longer as the same credential keeps failing
// without a success between. The copy it returns keeps callers from mutating
// the shipped waits.
func DefaultFailoverBackoff() []time.Duration {
	return []time.Duration{
		0,
		3 * time.Minute,
		5 * time.Minute,
		15 * time.Minute,
		30 * time.Minute,
		time.Hour,
		5 * time.Hour,
	}
}

// CredentialVerdict is what one upstream answer says about the credential
// that produced it.
type CredentialVerdict int

// Pool verdicts are what one failure means for a credential: keep serving,
// retry later, or stop until the operator logs in again.
const (
	VerdictHealthy CredentialVerdict = iota
	VerdictRetry
	VerdictNeedsReauth
)

// ClassifyStatus reports what an upstream status means for a credential.
// Only rate limiting and server faults count against a breaker; an
// authorization failure means the account must log in again.
func ClassifyStatus(status int) CredentialVerdict {
	switch {
	case status == 401 || status == 403:
		return VerdictNeedsReauth
	case status == 429 || status >= 500:
		return VerdictRetry
	default:
		return VerdictHealthy
	}
}

// Breaker keeps one credential out of rotation after a transient failure,
// walking a ladder of waits as the same credential fails again without a
// success between. It lets a single trial through once a wait has passed.
type Breaker struct {
	clock    clock.Clock
	mu       sync.Mutex
	state    string
	failures int
	backoff  time.Duration
	until    time.Time
	trialOut bool
	steps    []time.Duration
}

// NewBreaker returns a closed breaker that reads time from clk and walks the
// shipped escalating waits.
func NewBreaker(clk clock.Clock) *Breaker {
	if clk == nil {
		clk = clock.New()
	}
	return &Breaker{
		clock: clk, state: BreakerClosed,
		steps: append([]time.Duration(nil), DefaultFailoverBackoff()...),
	}
}

// NewBreakerWithBackoff returns a closed breaker that keeps a refused
// credential out of rotation through the given waits, one step per
// consecutive failure. No steps keeps no denylist.
func NewBreakerWithBackoff(clk clock.Clock, steps []time.Duration) *Breaker {
	breaker := NewBreaker(clk)
	breaker.SetFailoverBackoff(steps)
	return breaker
}

// SetFailoverBackoff changes the waits future failures walk through: the
// first step applies to the first failure, the next step to the next, and a
// failure past the end of the ladder stays on the last step. No steps keeps
// no denylist: the breaker closes at once, so the next request starts on the
// credential again. A success resets the walk.
func (b *Breaker) SetFailoverBackoff(steps []time.Duration) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(steps) == 0 {
		b.steps = nil
		b.state = BreakerClosed
		b.failures = 0
		b.backoff = 0
		b.until = time.Time{}
		b.trialOut = false
		return
	}
	clamped := make([]time.Duration, 0, len(steps))
	for _, step := range steps {
		clamped = append(clamped, min(step, breakerCeiling))
	}
	b.steps = clamped
}

// RecordSuccess closes the breaker and forgets the failure history.
func (b *Breaker) RecordSuccess() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.state = BreakerClosed
	b.failures = 0
	b.backoff = 0
	b.until = time.Time{}
	b.trialOut = false
}

// RecordFailure records one upstream answer and reports what it means.
// Every rate limit or server fault moves the credential one step down its
// wait ladder: the first step may keep no wait at all, and further failures
// without a success between wait longer. Without a denylist the breaker
// never opens: the refusal still counts as a retry, but the credential stays
// in rotation. A Retry-After the provider named is still waited out.
func (b *Breaker) RecordFailure(status int, retryAfter time.Duration) CredentialVerdict {
	verdict := ClassifyStatus(status)
	if verdict != VerdictRetry {
		return verdict
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if retryAfter <= 0 && len(b.steps) == 0 {
		return VerdictRetry
	}
	b.failures++
	wait := b.nextBackoffLocked(retryAfter)
	if wait <= 0 {
		// The step keeps no wait: the credential stays in rotation, and the
		// failure still counts toward the next step.
		b.state = BreakerClosed
		b.backoff = 0
		b.until = time.Time{}
		b.trialOut = false
		return VerdictRetry
	}
	b.state = BreakerOpen
	b.backoff = wait
	b.until = b.clock.Now().Add(wait)
	b.trialOut = false
	return VerdictRetry
}

// IsAvailable reports whether the credential may take a request now, and
// spends the single half-open trial once the backoff has passed.
func (b *Breaker) IsAvailable() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.settle()
	switch b.state {
	case BreakerOpen:
		return false
	case BreakerHalfOpen:
		if b.trialOut {
			return false
		}
		b.trialOut = true
		return true
	default:
		return true
	}
}

// Allows reports whether a request may be sent without spending the
// half-open trial, which is what a coverage check needs.
func (b *Breaker) Allows() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.settle()
	switch b.state {
	case BreakerOpen:
		return false
	case BreakerHalfOpen:
		return !b.trialOut
	default:
		return true
	}
}

// State returns the state the breaker is in, for diagnostics.
func (b *Breaker) State() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.settle()
	return b.state
}

// BreakerSnapshot is the live state of one breaker, which is what the
// console reports as an account's rate-limit health.
type BreakerSnapshot struct {
	State    string
	Until    time.Time
	Failures int
}

// Snapshot settles the breaker against the clock first, so a report reads
// the same state the next request would meet.
func (b *Breaker) Snapshot() BreakerSnapshot {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.settle()
	return BreakerSnapshot{State: b.state, Until: b.until, Failures: b.failures}
}

func (b *Breaker) settle() {
	if b.state != BreakerOpen || b.clock.Now().Before(b.until) {
		return
	}
	b.state = BreakerHalfOpen
	b.trialOut = false
}

func (b *Breaker) nextBackoffLocked(retryAfter time.Duration) time.Duration {
	if retryAfter > 0 {
		return min(retryAfter, breakerCeiling)
	}
	if len(b.steps) == 0 {
		return 0
	}
	return b.steps[min(b.failures-1, len(b.steps)-1)]
}
