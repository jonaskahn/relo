// Pool strategy vocabulary and round-robin selection.
package account

import (
	"sync/atomic"
)

// PoolStrategy picks one credential out of the candidates the pool
// already filtered down to usable entries.
type PoolStrategy interface {
	Select(candidates []PoolEntry, ctx Selection) (PoolEntry, error)
}

// RoundRobinStrategy spreads requests evenly across the candidates.
type RoundRobinStrategy struct {
	counter atomic.Uint64
}

// Select returns the next candidate in rotation.
func (s *RoundRobinStrategy) Select(candidates []PoolEntry, _ Selection) (PoolEntry, error) {
	if len(candidates) == 0 {
		return PoolEntry{}, ErrNoCredentials
	}
	index := s.counter.Add(1) - 1
	return candidates[index%uint64(len(candidates))], nil
}
