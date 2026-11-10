// Connection health: the account that last served a connection, and the wait
// a connection sits out once every account of it has failed a request.
package account

import "time"

// connectionState is the memory a pool keeps about its connection as a whole.
// It lives in memory only: a restart starts every connection available.
type connectionState struct {
	sticky    string
	failures  int
	coolUntil time.Time
	// spent marks a cooldown the connection earned by exhausting a shared
	// lane, rather than by failing every account. It is what lets the answer
	// name the lane instead of reporting a generic no-route.
	spent bool
}

// MarkServed records the account that just answered a request: later requests
// try it first, and the connection's cooldown walk starts over.
func (p *Pool) MarkServed(id string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.conn = connectionState{sticky: id}
}

// MarkExhausted records that every account of the connection failed one
// request. Each failure without a success between moves the connection one
// step down the failover ladder, and a step of zero keeps it usable; a
// retry-after longer than the step wins. No account is worth starting on once
// they have all failed, so the connection forgets the one it remembered and
// the next request picks again.
func (p *Pool) MarkExhausted(retryAfter time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.conn.sticky = ""
	p.conn.failures++
	if len(p.steps) == 0 {
		return
	}
	wait := max(p.steps[min(p.conn.failures-1, len(p.steps)-1)], retryAfter)
	if wait > 0 {
		p.conn.coolUntil = p.clock.Now().Add(wait)
	}
}

// MarkLaneSpent records a connection whose shared allowance is spent. No
// account can help, so the connection sits out the wait the provider named.
// MarkExhausted records a different cause and never sets this marker.
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
// after every account failed one. A zero time means it is not cooling down.
func (p *Pool) ConnectionCooldownUntil() time.Time {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.conn.coolUntil.IsZero() && !p.clock.Now().Before(p.conn.coolUntil) {
		p.conn.coolUntil = time.Time{}
	}
	return p.conn.coolUntil
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
