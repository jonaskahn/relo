// Failover remembers the route members one request just refused, so the
// next request starts past them instead of on them.
package routing

import (
	"fmt"
	"sync"
	"time"

	"github.com/jonaskahn/relo/internal/catalog"
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

// Until reports when a cooling key may be used again. A key that is not
// cooling reports false.
func (f *Failover) Until(key string) (time.Time, bool) {
	if f == nil {
		return time.Time{}, false
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.cooling(key) {
		return time.Time{}, false
	}
	return f.until[key], true
}

// Filter drops the members still cooling down. When every member is cooling
// it keeps none and reports the earliest moment one comes back, so the
// request fails at once instead of asking members that just failed.
func (f *Failover) Filter(candidates []Candidate) ([]Candidate, time.Time) {
	if f == nil || len(candidates) < 2 {
		return candidates, time.Time{}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	kept := make([]Candidate, 0, len(candidates))
	var earliest time.Time
	for _, candidate := range candidates {
		key := candidateKey(candidate)
		if !f.cooling(key) {
			kept = append(kept, candidate)
			continue
		}
		if earliest.IsZero() || f.until[key].Before(earliest) {
			earliest = f.until[key]
		}
	}
	if len(kept) > 0 {
		return kept, time.Time{}
	}
	return kept, earliest
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

// MarkGroupFailed keeps a group out of the next plans after every member
// failed one request, walking the same ladder a member does.
func (r *Router) MarkGroupFailed(plan Plan) {
	if r == nil || r.failover == nil || plan.GroupID == "" {
		return
	}
	r.failover.MarkFailed(groupKey(plan.GroupID))
}

// MarkGroupSucceeded makes the member that answered the first one the group
// tries, and ends the group's cooldown walk.
func (r *Router) MarkGroupSucceeded(plan Plan, candidate Candidate) {
	if r == nil || plan.GroupID == "" {
		return
	}
	r.mu.Lock()
	r.sticky[plan.GroupID] = candidateKey(candidate)
	r.mu.Unlock()
	if r.failover != nil {
		r.failover.MarkSucceeded(groupKey(plan.GroupID))
	}
}

// stickyFirst moves the member that last served a priority group to the
// front. A group an operator spread by another strategy keeps that order.
func (r *Router) stickyFirst(group catalog.Group, candidates []Candidate) []Candidate {
	if group.Strategy != catalog.StrategyPriority || len(candidates) < 2 {
		return candidates
	}
	r.mu.Lock()
	key := r.sticky[group.ID]
	r.mu.Unlock()
	for index, candidate := range candidates {
		if candidateKey(candidate) != key {
			continue
		}
		reordered := make([]Candidate, 0, len(candidates))
		reordered = append(reordered, candidate)
		reordered = append(reordered, candidates[:index]...)
		return append(reordered, candidates[index+1:]...)
	}
	return candidates
}

func groupKey(groupID string) string {
	return "group " + groupID
}

// CoolingError reports a request refused because its connection or group is
// waiting out a failure. It is a no-route refusal that also says when to ask
// again.
type CoolingError struct {
	Name  string
	Until time.Time
	// Reason is what the caller knows about the failure, when it knows more
	// than this type does: a connection's own accounts cooling down, or a
	// route whose every member failed.
	Reason string
}

func (e *CoolingError) Error() string {
	if e.Reason != "" {
		return e.Reason
	}
	return fmt.Sprintf("%s: %s: every member failed, retry after %s",
		e.Name, ErrNoRoute, e.Until.UTC().Format("15:04:05 MST"))
}

// Unwrap lets a cooling refusal read as ErrNoRoute.
func (e *CoolingError) Unwrap() error { return ErrNoRoute }
