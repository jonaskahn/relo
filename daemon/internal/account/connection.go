// Connection health: the account that last served a connection, the wait a
// shared lane of it sits out, and the wait one model of it sits out after
// every account of it failed a request for that model.
package account

import "time"

// connectionState is the memory a pool keeps about its connection as a whole.
// It lives in memory only: a restart starts every connection available.
type connectionState struct {
	sticky string
	// spent marks a cooldown the connection earned by exhausting a shared
	// lane, rather than by failing every account. It is what lets the answer
	// name the lane instead of reporting a generic no-route.
	spent bool
	// coolUntil is when a shared lane may be tried again.
	coolUntil time.Time
}

// modelState is the wait one model of a connection sits out. The wait belongs
// to that model alone: the other models of the connection keep being asked.
type modelState struct {
	failures int
	until    time.Time
}

// MarkServed records the account that just answered a request: later requests
// try it first, and the connection's cooldown walk starts over.
func (p *Pool) MarkServed(id string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.conn = connectionState{sticky: id}
}

// MarkModelServed records that one model of the connection answered a
// request. Its wait ends and its next failure starts at the first step of
// the ladder; the other models keep theirs.
func (p *Pool) MarkModelServed(modelID string) {
	if modelID == "" {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.modelWaits, modelID)
}

// MarkModelUnavailable records that every account of the connection failed a
// request for one model. Each failure without a success between moves that
// model one step down the failover ladder, and a step of zero keeps it
// usable; a retry-after longer than the step wins.
//
// Only that model waits: a limit a provider names for one model says nothing
// about the others the same connection serves, so they keep being asked. The
// account the connection remembers is kept for the same reason: the limit
// says nothing about the account either.
func (p *Pool) MarkModelUnavailable(modelID string, retryAfter time.Duration) {
	if modelID == "" {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	state := p.modelWaits[modelID]
	state.failures++
	p.modelWaits[modelID] = state
	if len(p.steps) == 0 {
		return
	}
	wait := max(p.steps[min(state.failures-1, len(p.steps)-1)], retryAfter)
	if wait <= 0 {
		return
	}
	state.until = p.clock.Now().Add(wait)
	p.modelWaits[modelID] = state
}

// ModelCooldownUntil is when one model of the connection may be asked again
// after every account of it failed a request for that model. A zero time
// means the model is not cooling down.
func (p *Pool) ModelCooldownUntil(modelID string) time.Time {
	if modelID == "" {
		return time.Time{}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	state, found := p.modelWaits[modelID]
	if !found {
		return time.Time{}
	}
	if !state.until.IsZero() && !p.clock.Now().Before(state.until) {
		state.until = time.Time{}
		p.modelWaits[modelID] = state
	}
	return state.until
}

// MarkLaneSpent records a connection whose shared allowance is spent. No
// account can help, so the connection sits out the wait the provider named.
// The allowance is shared by the whole connection, so this wait is too.
func (p *Pool) MarkLaneSpent(retryAfter time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.conn.sticky = ""
	p.conn.spent = true
	p.conn.coolUntil = p.clock.Now().Add(max(retryAfter, 0))
}

// LaneSpentUntil reports when a spent shared lane may be tried again. A
// zero time means the lane is not spent, including once its wait expires.
func (p *Pool) LaneSpentUntil() time.Time {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.conn.spent {
		return time.Time{}
	}
	if !p.clock.Now().Before(p.conn.coolUntil) {
		p.conn.spent = false
		p.conn.coolUntil = time.Time{}
		return time.Time{}
	}
	return p.conn.coolUntil
}

// ConnectionCooldownUntil is when the connection may take a request again
// after a wait every account of it shares. A zero time means it is not
// cooling down. A limit one model earned waits for that model instead.
func (p *Pool) ConnectionCooldownUntil() time.Time {
	return p.LaneSpentUntil()
}

// stickyEntry is the account that last served the connection, when it can
// still take this request and the pool's strategy is not one the operator
// picked to spread load.
func (p *Pool) stickyEntry(candidates []PoolEntry) (PoolEntry, bool) {
	p.mu.RLock()
	sticky, strategy := p.conn.sticky, p.strategy
	p.mu.RUnlock()
	if sticky == "" || StrategyName(strategy) != StrategyLeastLoaded {
		return PoolEntry{}, false
	}
	for _, candidate := range candidates {
		if candidate.ID == sticky {
			return candidate, true
		}
	}
	return PoolEntry{}, false
}
