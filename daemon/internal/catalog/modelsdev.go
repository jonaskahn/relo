// models.dev vocabulary: fetch mode, state, and index.
package catalog

import (
	"sort"
	"strings"
)

// ModelsDevFetchMode controls how a models.dev fetch treats the network.
type ModelsDevFetchMode string

// Fetch modes name how the index answers: network first, saved copy only,
// and the source either reads from.
const (
	// FetchOnlineFirst reads the network and falls back to the saved copy
	// when the fetch fails.
	FetchOnlineFirst ModelsDevFetchMode = "online-first"
	// FetchForce reads the network whether or not a saved copy exists.
	FetchForce ModelsDevFetchMode = "force"
	// ModelsDevDefaultURL is where the models.dev dataset is published.
	ModelsDevDefaultURL = "https://models.dev/api.json"
)

// ModelsDevState reports the freshness of the models.dev data a surface
// reads, so a console can show an age without waiting on a network.
type ModelsDevState struct {
	SourceURL       string
	FetchedAtMs     int64
	Stale           bool
	Available       bool
	ETag            string
	LastModified    string
	ProvidersCount  int
	ModelsCount     int
	LastAttemptAtMs int64
	LastError       string
}

// ModelsDevIndex is the parsed models.dev dataset: every provider it
// describes and the fetch state the copy was read under.
type ModelsDevIndex struct {
	Providers map[string]ModelsDevProvider
	State     ModelsDevState
}

// ModelsDevProvider is one provider declared in models.dev.
type ModelsDevProvider struct {
	ID     string
	Name   string
	NPM    string
	API    string
	Doc    string
	Env    []string
	Models map[string]ModelsDevModel
}

// ModelsDevTransport names an alternative transport endpoint and shape for
// a model, which is what tells a transport this build has no codec for.
type ModelsDevTransport struct {
	NPM   string
	API   string
	Shape string
}

// ModelsDevModel is one model declared in models.dev.
type ModelsDevModel struct {
	ID                string
	Name              string
	Description       string
	Family            string
	Category          string
	ContextWindow     *int64
	MaxInput          *int64
	MaxOutput         *int64
	SupportsTools     *bool
	SupportsReasoning *bool
	SupportsVision    *bool
	// ReasoningEfforts are the effort values the model accepts, or nil when
	// it states none. An empty slice means the model stated no effort list.
	// A toggle or a token budget is a separate control and does not fill this
	// list.
	ReasoningEfforts   []string
	ReasoningToggle    bool
	ReasoningBudget    bool
	ReasoningBudgetMin *int64
	ReasoningBudgetMax *int64
	Status             string
	ReleaseDate        string
	Prices             Prices
	Via                ModelsDevTransport
}

// TotalModels counts every model the dataset describes.
func (idx *ModelsDevIndex) TotalModels() int {
	total := 0
	for _, p := range idx.Providers {
		total += len(p.Models)
	}
	return total
}

// ModelsDevMatch describes how a model identifier matched the dataset.
type ModelsDevMatch string

// Match kinds name how a model met the index: exactly, after folding, by
// vendor alias, by operator hand, or not at all.
const (
	ModelsDevMatchExact      ModelsDevMatch = "exact"
	ModelsDevMatchNormalized ModelsDevMatch = "normalized"
	ModelsDevMatchVendor     ModelsDevMatch = "vendor"
	ModelsDevMatchManual     ModelsDevMatch = "manual"
	ModelsDevMatchNone       ModelsDevMatch = "none"
)

var fallbackVendors = []string{
	"openai",
	"anthropic",
	"google",
	"xai",
	"mistral",
	"deepseek",
	"moonshotai",
	"zai",
	"alibaba",
	"minimax",
}

// Match finds the best matching model for a provider and model identifier:
// the identifier itself, then a normalized form of it, then the vendor its
// prefix names, and finally the vendors Relo prices from most often.
func (idx *ModelsDevIndex) Match(modelsdevProviderID, modelID string) (ModelsDevModel, ModelsDevMatch, bool) {
	if idx == nil || len(idx.Providers) == 0 {
		return ModelsDevModel{}, ModelsDevMatchNone, false
	}

	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return ModelsDevModel{}, ModelsDevMatchNone, false
	}

	if m, found := matchProviderModel(idx, modelsdevProviderID, modelID); found {
		return m, ModelsDevMatchExact, true
	}
	if m, found := matchNormalizedModel(idx, modelsdevProviderID, modelID); found {
		return m, ModelsDevMatchNormalized, true
	}
	if m, found := matchVendorModel(idx, modelID); found {
		return m, ModelsDevMatchVendor, true
	}
	return matchFallbackVendor(idx, modelsdevProviderID, modelID)
}

func matchProviderModel(idx *ModelsDevIndex, modelsdevProviderID, modelID string) (ModelsDevModel, bool) {
	if p, ok := idx.Providers[modelsdevProviderID]; ok {
		if m, found := p.Models[modelID]; found {
			return m, true
		}
	}
	return ModelsDevModel{}, false
}

func matchNormalizedModel(idx *ModelsDevIndex, modelsdevProviderID, modelID string) (ModelsDevModel, bool) {
	normID := normalizeID(modelID)
	if p, ok := idx.Providers[modelsdevProviderID]; ok {
		for id, m := range p.Models {
			if normalizeID(id) == normID {
				return m, true
			}
		}
	}
	return ModelsDevModel{}, false
}

func matchVendorModel(idx *ModelsDevIndex, modelID string) (ModelsDevModel, bool) {
	parts := strings.Split(modelID, "/")
	if len(parts) <= 1 {
		return ModelsDevModel{}, false
	}
	vendor := parts[0]
	subModel := strings.Join(parts[1:], "/")
	if p, ok := idx.Providers[vendor]; ok {
		if m, found := p.Models[subModel]; found {
			return m, true
		}
		normSub := normalizeID(subModel)
		for id, m := range p.Models {
			if normalizeID(id) == normSub {
				return m, true
			}
		}
	}
	return ModelsDevModel{}, false
}

func matchFallbackVendor(idx *ModelsDevIndex, modelsdevProviderID, modelID string) (ModelsDevModel, ModelsDevMatch, bool) {
	normID := normalizeID(modelID)
	for _, vendorID := range fallbackVendors {
		if vendorID == modelsdevProviderID {
			continue
		}
		if p, ok := idx.Providers[vendorID]; ok {
			if m, found := p.Models[modelID]; found {
				return m, ModelsDevMatchVendor, true
			}
			for id, m := range p.Models {
				if normalizeID(id) == normID {
					return m, ModelsDevMatchVendor, true
				}
			}
		}
	}
	return ModelsDevModel{}, ModelsDevMatchNone, false
}

var regionPrefixes = []string{
	"us.",
	"eu.",
	"apac.",
	"jp.",
	"global.",
}

func normalizeID(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	s = strings.TrimPrefix(s, "models/")

	for _, prefix := range regionPrefixes {
		s = strings.TrimPrefix(s, prefix)
	}

	if idx := strings.LastIndex(s, "/"); idx >= 0 {
		s = s[idx+1:]
	}

	return s
}

// ModelsDevHit is one model a search over the saved copy found, with the
// name of the provider it belongs to so a list can show it without a
// second lookup.
type ModelsDevHit struct {
	Ref           string
	ProviderID    string
	ProviderName  string
	ModelID       string
	Name          string
	ContextWindow *int64
	Prices        Prices
}

// Search finds the models whose identifier, name or full reference matches
// a query, best first. The exact identifier wins, then the vendors Relo
// prices from most often, then the rest by identifier, which is the order
// an operator looking for one model expects to read.
func (idx *ModelsDevIndex) Search(query string, limit int) []ModelsDevHit {
	if idx == nil || len(idx.Providers) == 0 {
		return []ModelsDevHit{}
	}
	needle := strings.ToLower(strings.TrimSpace(query))
	hits := collectModelsDevHits(idx, needle)
	sort.SliceStable(hits, func(i, j int) bool {
		return hitLess(hits[i], hits[j], needle)
	})
	if limit > 0 && len(hits) > limit {
		return hits[:limit]
	}
	return hits
}

func collectModelsDevHits(idx *ModelsDevIndex, needle string) []ModelsDevHit {
	hits := make([]ModelsDevHit, 0, 64)
	for providerID, p := range idx.Providers {
		for modelID, m := range p.Models {
			hit := ModelsDevHit{
				Ref:           providerID + "/" + modelID,
				ProviderID:    providerID,
				ProviderName:  p.Name,
				ModelID:       modelID,
				Name:          m.Name,
				ContextWindow: m.ContextWindow,
				Prices:        m.Prices,
			}
			if needle != "" && !hitMatches(hit, needle) {
				continue
			}
			hits = append(hits, hit)
		}
	}
	return hits
}

func hitMatches(hit ModelsDevHit, needle string) bool {
	return strings.Contains(strings.ToLower(hit.ModelID), needle) ||
		strings.Contains(strings.ToLower(hit.Ref), needle) ||
		strings.Contains(strings.ToLower(hit.Name), needle) ||
		strings.Contains(strings.ToLower(hit.ProviderName), needle)
}

func hitLess(left, right ModelsDevHit, needle string) bool {
	leftExact := strings.ToLower(left.ModelID) == needle
	rightExact := strings.ToLower(right.ModelID) == needle
	if leftExact != rightExact {
		return leftExact
	}
	leftRank, rightRank := vendorRank(left.ProviderID), vendorRank(right.ProviderID)
	if leftRank != rightRank {
		return leftRank < rightRank
	}
	if left.ProviderID != right.ProviderID {
		return left.ProviderID < right.ProviderID
	}
	return left.ModelID < right.ModelID
}

func vendorRank(providerID string) int {
	for i, vendor := range fallbackVendors {
		if vendor == providerID {
			return i
		}
	}
	return len(fallbackVendors)
}
