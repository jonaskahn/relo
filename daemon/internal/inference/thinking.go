// Thinking levels map a client effort onto the controls one model accepts.
package inference

import "strings"

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

// ChooseThinking maps a requested effort onto one model's ladder.
// level is the effort name to send. off is true when the model turns
// thinking off with a toggle and no effort name should be sent.
// A nil ladder passes the normalized name through. An empty ladder has
// no effort values.
func ChooseThinking(effort string, ladder []string, toggle bool) (level string, off bool) {
	name, ok := NormalizeEffort(effort)
	if !ok {
		return "", false
	}
	if ladder == nil {
		return name, false
	}
	if name == "none" {
		return chooseOff(ladder, toggle)
	}
	if len(ladder) == 0 {
		return "", false
	}
	if match, found := listedEffort(ladder, name); found {
		return match, false
	}
	if ceiling, found := ceilingEffort(ladder, effortRank(name)); found {
		return ceiling, false
	}
	if highest, found := extremeEffort(ladder, true); found {
		return highest, false
	}
	return "", false
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

func chooseOff(ladder []string, toggle bool) (string, bool) {
	if match, found := listedEffort(ladder, "none"); found {
		return match, false
	}
	if match, found := listedEffort(ladder, "minimal"); found {
		return match, false
	}
	if toggle {
		return "", true
	}
	if lowest, found := extremeEffort(ladder, false); found {
		return lowest, false
	}
	return "", false
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
