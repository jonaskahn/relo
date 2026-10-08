// Failover remembers the route members one request just refused, so the
// next request starts past them instead of on them.
package routing

import (
	"sync"
	"time"

	"github.com/jonaskahn/relo/internal/clock"
)

// Failover keeps refused candidates out of the next plans, walking an
// escalating ladder of waits as the same member fails again without a
// success between. It lives in memory only: a restart starts every member
// available again.
type Failover struct {
	mu       sync.Mutex
	clock    clock.Clock
	steps    []time.Duration
	failures map[string]int
	until    map[string]time.Time
}

// NewFailover returns a failover that skips refused members through the
// given waits, one step per consecutive failure. No steps keeps no denylist:
// members are never skipped.
func NewFailover(clk clock.Clock, steps []time.Duration) *Failover {
	if clk == nil {
		clk = clock.New()
	}
	return &Failover{
		clock: clk, steps: append([]time.Duration(nil), steps...),
		failures: map[string]int{}, until: map[string]time.Time{},
	}
}

// MarkFailed keeps one member out of the next plans, unless the denylist is
// off: with no steps there is nothing to remember. Each refusal moves the
// member one step down the ladder, and a step of zero keeps it immediately
// reusable while still counting toward the next step.
func (f *Failover) MarkFailed(key string) {
	if key == "" {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.steps) == 0 {
		return
	}
	f.failures[key]++
	wait := f.steps[min(f.failures[key]-1, len(f.steps)-1)]
	if wait <= 0 {
		delete(f.until, key)
		return
	}
	f.until[key] = f.clock.Now().Add(wait)
}

// MarkSucceeded returns one member to rotation, leaving the others refused,
// and resets its ladder so its next failure starts at the first step.
func (f *Failover) MarkSucceeded(key string) {
	if key == "" {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.until, key)
	delete(f.failures, key)
}

// SetBackoff changes the waits future failures walk through. No steps keeps
// no denylist: members waiting out a previous wait return to the next plans
// at once.
func (f *Failover) SetBackoff(steps []time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(steps) == 0 {
		f.steps = nil
		clear(f.until)
		clear(f.failures)
		return
	}
	f.steps = append([]time.Duration(nil), steps...)
}

func (f *Failover) cooling(key string) bool {
	expiry, found := f.until[key]
	if !found {
		return false
	}
	if !f.clock.Now().Before(expiry) {
		delete(f.until, key)
		return false
	}
	return true
}

// Filter drops the members still cooling down. When every member is cooling
// the full list is kept, so the request tries them once more rather than
// answering with no candidate at all.
func (f *Failover) Filter(candidates []Candidate) []Candidate {
	if f == nil || len(candidates) < 2 {
		return candidates
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	kept := make([]Candidate, 0, len(candidates))
	for _, candidate := range candidates {
		if !f.cooling(candidateKey(candidate)) {
			kept = append(kept, candidate)
		}
	}
	if len(kept) == 0 {
		return candidates
	}
	return kept
}

// MarkFailed keeps one member out of the next plans, unless the denylist is
// off: with no steps the refusal is forgotten at once.
func (r *Router) MarkFailed(candidate Candidate) {
	if r == nil || r.failover == nil {
		return
	}
	r.failover.MarkFailed(candidateKey(candidate))
}

// MarkSucceeded returns one member to rotation, leaving the others refused.
func (r *Router) MarkSucceeded(candidate Candidate) {
	if r == nil || r.failover == nil {
		return
	}
	r.failover.MarkSucceeded(candidateKey(candidate))
}

// SetFailoverBackoff changes the waits future refused members walk through.
// No steps keeps no denylist: members waiting out a previous wait return to
// the next plans at once.
func (r *Router) SetFailoverBackoff(steps []time.Duration) {
	if r == nil || r.failover == nil {
		return
	}
	r.failover.SetBackoff(steps)
}
