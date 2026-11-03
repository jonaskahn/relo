// Thinking levels map a client effort onto the controls one model accepts.
package inference

import (
	"sort"
	"strings"
)

var effortOrder = []string{"none", "minimal", "low", "medium", "high", "xhigh", "max"}

var effortBudgets = map[string]int{
	"none":    0,
	"minimal": 1024,
	"low":     4096,
	"medium":  8192,
	"high":    16384,
	"xhigh":   24576,
	"max":     32000,
}

// ToggleOnly reports a model whose canonical facts state an empty effort
// ladder and an on/off thinking switch. Such a model has no effort intensity;
// presentation layers project a synthetic choice onto it instead.
func ToggleOnly(ladder []string, toggle bool) bool {
	return ladder != nil && len(ladder) == 0 && toggle
}

// EffortBudget reports the token budget a canonical effort stands for.
func EffortBudget(effort string) (int, bool) {
	name, ok := NormalizeEffort(effort)
	if !ok {
		return 0, false
	}
	budget, found := effortBudgets[name]
	return budget, found
}

// LowerEffort returns the effort one step below name on the shared ladder.
// It reports false when name is the lowest step or is not ranked.
func LowerEffort(name string) (string, bool) {
	rank, ranked := effortRankOf(name)
	if !ranked || rank == 0 {
		return "", false
	}
	return effortOrder[rank-1], true
}

// OrderEfforts returns the stated efforts on the shared ladder, lowest first.
// A name outside the ladder follows the ranked ones, so a client shows the
// levels in the order a model offers them rather than the order a provider
// happened to name them.
func OrderEfforts(efforts []string) []string {
	ordered := append([]string(nil), efforts...)
	sort.SliceStable(ordered, func(i, j int) bool {
		leftRank, leftRanked := effortRankOf(ordered[i])
		rightRank, rightRanked := effortRankOf(ordered[j])
		switch {
		case leftRanked && rightRanked:
			return leftRank < rightRank
		case leftRanked != rightRanked:
			return leftRanked
		default:
			return ordered[i] < ordered[j]
		}
	})
	return ordered
}

// ClampBudget keeps a token budget inside the range a model declared.
// A nil bound is no bound.
func ClampBudget(budget int, min, max *int64) int {
	if min != nil && budget < int(*min) {
		budget = int(*min)
	}
	if max != nil && *max > 0 && budget > int(*max) {
		budget = int(*max)
	}
	return budget
}

func listedEffort(ladder []string, name string) (string, bool) {
	for _, value := range ladder {
		if strings.EqualFold(strings.TrimSpace(value), name) {
			return strings.TrimSpace(value), true
		}
	}
	return "", false
}

func ceilingEffort(ladder []string, rank int) (string, bool) {
	bestRank := -1
	best := ""
	for _, value := range ladder {
		valueRank, ranked := effortRankOf(value)
		if !ranked || valueRank < rank {
			continue
		}
		if bestRank < 0 || valueRank < bestRank {
			bestRank = valueRank
			best = strings.TrimSpace(value)
		}
	}
	if bestRank < 0 {
		return "", false
	}
	return best, true
}

func extremeEffort(ladder []string, highest bool) (string, bool) {
	bestRank := -1
	best := ""
	for _, value := range ladder {
		valueRank, ranked := effortRankOf(value)
		if !ranked {
			continue
		}
		if bestRank < 0 || (highest && valueRank > bestRank) || (!highest && valueRank < bestRank) {
			bestRank = valueRank
			best = strings.TrimSpace(value)
		}
	}
	if bestRank < 0 {
		return "", false
	}
	return best, true
}

func effortRank(name string) int {
	rank, _ := effortRankOf(name)
	return rank
}

func effortRankOf(name string) (int, bool) {
	name = strings.ToLower(strings.TrimSpace(name))
	for rank, value := range effortOrder {
		if value == name {
			return rank, true
		}
	}
	return 0, false
}
