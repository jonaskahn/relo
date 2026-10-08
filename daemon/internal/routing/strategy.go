// Route member ordering: rotation, weights, and cheapness.
package routing

import (
	"math"
	"sort"
	"time"

	"github.com/jonaskahn/relo/internal/catalog"
)

const latencyAlpha = 0.3

func (r *Router) orderGroup(group catalog.Group, candidates []Candidate, preview bool) []Candidate {
	ordered := make([]Candidate, len(candidates))
	copy(ordered, candidates)
	switch group.Strategy {
	case catalog.StrategyRoundRobin:
		return r.rotate(group.ID, ordered, preview)
	case catalog.StrategyWeighted:
		return r.weigh(ordered, preview)
	case catalog.StrategyCheapest:
		sort.SliceStable(ordered, func(i, j int) bool {
			return cheapness(ordered[i]) < cheapness(ordered[j])
		})
		return ordered
	case catalog.StrategyFastest:
		sort.SliceStable(ordered, func(i, j int) bool {
			return r.latencyOf(ordered[i]) < r.latencyOf(ordered[j])
		})
		return ordered
	default:
		return ordered
	}
}

func (r *Router) rotate(groupID string, candidates []Candidate, preview bool) []Candidate {
	if len(candidates) < 2 {
		return candidates
	}
	r.mu.Lock()
	start := r.rotation[groupID] % len(candidates)
	if !preview {
		r.rotation[groupID] = (start + 1) % len(candidates)
	}
	r.mu.Unlock()
	rotated := make([]Candidate, 0, len(candidates))
	rotated = append(rotated, candidates[start:]...)
	return append(rotated, candidates[:start]...)
}

func (r *Router) weigh(candidates []Candidate, preview bool) []Candidate {
	if preview {
		ordered := make([]Candidate, len(candidates))
		copy(ordered, candidates)
		sort.SliceStable(ordered, func(i, j int) bool {
			return ordered[i].Weight > ordered[j].Weight
		})
		return ordered
	}
	return r.drawWeighted(candidates)
}

func (r *Router) drawWeighted(candidates []Candidate) []Candidate {
	type draw struct {
		candidate Candidate
		key       float64
	}
	draws := make([]draw, 0, len(candidates))
	for _, candidate := range candidates {
		weight := candidate.Weight
		if weight < 1 {
			weight = 1
		}
		value := r.rand()
		if value <= 0 {
			value = math.SmallestNonzeroFloat64
		}
		draws = append(draws, draw{candidate: candidate, key: math.Pow(value, 1/float64(weight))})
	}
	sort.SliceStable(draws, func(i, j int) bool { return draws[i].key > draws[j].key })
	ordered := make([]Candidate, 0, len(draws))
	for _, entry := range draws {
		ordered = append(ordered, entry.candidate)
	}
	return ordered
}

func cheapness(candidate Candidate) int64 {
	prices := candidate.Model.Prices
	if prices.Input == nil && prices.Output == nil {
		return math.MaxInt64
	}
	return valueOf(prices.Input) + valueOf(prices.Output)
}

func valueOf(rate *int64) int64 {
	if rate == nil {
		return 0
	}
	return *rate
}

func (r *Router) latencyOf(candidate Candidate) float64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	measured, found := r.latency[candidateKey(candidate)]
	if !found {
		return -1
	}
	return measured
}

// Record observes how long one attempt took to answer with headers, which is
// the only signal a fastest-first group orders by.
func (r *Router) Record(candidate Candidate, latency time.Duration) {
	if latency <= 0 {
		return
	}
	seconds := latency.Seconds()
	r.mu.Lock()
	defer r.mu.Unlock()
	key := candidateKey(candidate)
	previous, found := r.latency[key]
	if !found {
		r.latency[key] = seconds
		return
	}
	r.latency[key] = previous*(1-latencyAlpha) + seconds*latencyAlpha
}

func (r *Router) pinned(conversation, prefix string, candidates []Candidate, preview bool) []Candidate {
	if conversation == "" || len(candidates) < 2 {
		return candidates
	}
	pinned, found := r.pins.Get(pinKey(prefix, conversation))
	if !found {
		return candidates
	}
	for index, candidate := range candidates {
		if candidateKey(candidate) != pinned {
			continue
		}
		reordered := make([]Candidate, 0, len(candidates))
		reordered = append(reordered, candidate)
		reordered = append(reordered, candidates[:index]...)
		return append(reordered, candidates[index+1:]...)
	}
	if !preview {
		r.pins.Evict(pinKey(prefix, conversation))
	}
	return candidates
}

// Pin remembers the candidate a conversation reached, so its next request
// starts there.
func (r *Router) Pin(conversation, prefix string, candidate Candidate) {
	if conversation == "" {
		return
	}
	r.pins.Pin(pinKey(prefix, conversation), candidateKey(candidate))
}

// Unpin forgets a conversation's provider after a failure, so the next
// request is free to land somewhere else.
func (r *Router) Unpin(conversation, prefix string) {
	if conversation == "" {
		return
	}
	r.pins.Evict(pinKey(prefix, conversation))
}

// PinPrefix names the routing decision a pin belongs to, which is what the
// caller passes back to Pin and Unpin.
func PinPrefix(plan Plan) string {
	if plan.GroupID != "" {
		return prefixGroup(plan.GroupID)
	}
	return prefixBare(plan.Model)
}

func pinKey(prefix, conversation string) string {
	return prefix + " # " + conversation
}

// ConversationOf returns the identifier a request keeps its provider on, from
// the first header a client may use to name one.
func ConversationOf(headers map[string]string) string {
	for _, name := range []string{"session_id", "conversation_id"} {
		if value := headers[name]; value != "" {
			return value
		}
	}
	return ""
}
