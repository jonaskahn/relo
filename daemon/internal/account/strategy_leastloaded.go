// Least-loaded strategy: picking by quota headroom.
package account

import (
	"sort"
	"sync/atomic"
)

// Headroom reports how much of a credential's quota is left, as a
// percentage between 0 and 100. A false second value means no quota has
// been observed for it.
type Headroom func(credentialID string) (percent float64, ok bool)

const defaultHeadroom = 50.0

// LeastLoadedStrategy prefers the credential with the most quota headroom,
// then the highest priority, and rotates evenly between the credentials
// that tie. It is the default strategy.
type LeastLoadedStrategy struct {
	Headroom Headroom
	rotation atomic.Uint64
}

// Select returns the least loaded candidate.
func (s *LeastLoadedStrategy) Select(candidates []PoolEntry, _ Selection) (PoolEntry, error) {
	if len(candidates) == 0 {
		return PoolEntry{}, ErrNoCredentials
	}
	ranked := s.ranked(candidates)
	return ranked[s.spread(ranked)].entry, nil
}

type rankedEntry struct {
	entry    PoolEntry
	headroom float64
	priority int
}

func (s *LeastLoadedStrategy) ranked(candidates []PoolEntry) []rankedEntry {
	ranked := make([]rankedEntry, 0, len(candidates))
	for _, candidate := range candidates {
		ranked = append(ranked, rankedEntry{entry: candidate, headroom: s.headroom(candidate.ID), priority: candidate.Priority})
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].headroom != ranked[j].headroom {
			return ranked[i].headroom > ranked[j].headroom
		}
		return ranked[i].priority > ranked[j].priority
	})
	return ranked
}

func (s *LeastLoadedStrategy) headroom(credentialID string) float64 {
	if s.Headroom == nil {
		return defaultHeadroom
	}
	percent, ok := s.Headroom(credentialID)
	if !ok {
		return defaultHeadroom
	}
	return percent
}

func (s *LeastLoadedStrategy) spread(ranked []rankedEntry) int {
	best := ranked[0]
	tied := 0
	for _, entry := range ranked {
		if entry.headroom != best.headroom || entry.priority != best.priority {
			break
		}
		tied++
	}
	return int((s.rotation.Add(1) - 1) % uint64(tied))
}
