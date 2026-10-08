// models.dev parsing: index, providers, and model entries.
package modelsdev

import (
	"encoding/json"
	"math"
	"strings"

	"github.com/jonaskahn/relo/internal/catalog"
)

// Index holds the indexed providers and models from models.dev.
type Index = catalog.ModelsDevIndex

// Provider is one provider declared in models.dev.
type Provider = catalog.ModelsDevProvider

// TransportVia names alternative transport endpoint/shape for a model.
type TransportVia = catalog.ModelsDevTransport

// Model is one model declared in models.dev.
type Model = catalog.ModelsDevModel

func parseIndex(data []byte) (*Index, error) {
	// First check if it is snapshot format: {"providers": [...]}
	var snap struct {
		Providers []rawSnapshotProvider `json:"providers"`
	}
	if err := json.Unmarshal(data, &snap); err == nil && len(snap.Providers) > 0 {
		return parseSnapshotIndex(snap.Providers), nil
	}

	// Otherwise it is map format: {"openai": {...}, "anthropic": {...}}
	var rawMap map[string]rawProvider
	if err := json.Unmarshal(data, &rawMap); err != nil {
		return nil, err
	}

	providers := make(map[string]Provider, len(rawMap))
	for pID, rp := range rawMap {
		p := parseIndexProvider(pID, rp)
		providers[p.ID] = p
	}

	return &Index{Providers: providers}, nil
}

func parseIndexProvider(pID string, rp rawProvider) Provider {
	p := Provider{
		ID:     pID,
		Name:   firstNonEmpty(rp.Name, pID),
		NPM:    rp.NPM,
		API:    normalizeURL(rp.API),
		Doc:    rp.Doc,
		Env:    rp.Env,
		Models: make(map[string]Model, len(rp.Models)),
	}
	if rp.ID != "" {
		p.ID = rp.ID
	}

	for mID, rm := range rp.Models {
		m := parseRawModel(mID, rm)
		p.Models[m.ID] = m
	}
	return p
}

type rawSnapshotProvider struct {
	ID     string          `json:"id"`
	Name   string          `json:"name"`
	NPM    string          `json:"npm"`
	API    string          `json:"api"`
	Doc    string          `json:"doc"`
	Env    []string        `json:"env"`
	Models []rawModelEntry `json:"models"`
}

type rawProvider struct {
	ID     string                   `json:"id"`
	Name   string                   `json:"name"`
	NPM    string                   `json:"npm"`
	API    string                   `json:"api"`
	Doc    string                   `json:"doc"`
	Env    []string                 `json:"env"`
	Models map[string]rawModelEntry `json:"models"`
}

type rawModelEntry struct {
	ID               string            `json:"id"`
	Name             string            `json:"name"`
	Description      string            `json:"description"`
	Family           string            `json:"family"`
	Status           string            `json:"status"`
	ReleaseDate      string            `json:"release_date"`
	Reasoning        bool              `json:"reasoning"`
	ReasoningOptions []rawReasoningOpt `json:"reasoning_options"`
	ToolCall         bool              `json:"tool_call"`
	Attachment       bool              `json:"attachment"`
	Input            []string          `json:"input"`
	Output           []string          `json:"output"`
	Modalities       *rawModalities    `json:"modalities"`
	Context          statedTokens      `json:"context"`
	MaxOutput        statedTokens      `json:"max_output"`
	Limit            *rawLimit         `json:"limit"`
	Cost             *rawCost          `json:"cost"`
	Via              *rawVia           `json:"via"`
	Provider         *rawVia           `json:"provider"`
}

type rawReasoningOpt struct {
	Type   string   `json:"type"`
	Values []string `json:"values"`
	Min    *int     `json:"min"`
	Max    *int     `json:"max"`
}

type rawModalities struct {
	Input  []string `json:"input"`
	Output []string `json:"output"`
}

type rawLimit struct {
	Context statedTokens `json:"context"`
	Input   statedTokens `json:"input"`
	Output  statedTokens `json:"output"`
}

type statedTokens struct {
	n *int64
}

// UnmarshalJSON reads a token count the dataset states as a number, a
// quoted label, or null, so all three spellings resolve to one value.
func (s *statedTokens) UnmarshalJSON(data []byte) error {
	text := strings.TrimSpace(string(data))
	if text == "" || text == "null" {
		s.n = nil
		return nil
	}
	var count int64
	if text[0] == '"' {
		var label string
		if err := json.Unmarshal(data, &label); err != nil {
			return err
		}
		parsed, ok := catalog.ParseTokenCount(label)
		if !ok {
			s.n = nil
			return nil
		}
		count = parsed
	} else if err := json.Unmarshal(data, &count); err != nil {
		var asFloat float64
		if err2 := json.Unmarshal(data, &asFloat); err2 != nil {
			return err
		}
		count = int64(math.Round(asFloat))
	}
	s.n = catalog.RoundContextWindow(&count)
	return nil
}

type rawCost struct {
	Input           *float64  `json:"input"`
	Output          *float64  `json:"output"`
	CacheRead       *float64  `json:"cache_read"`
	CacheWrite      *float64  `json:"cache_write"`
	Tiers           []rawTier `json:"tiers"`
	ContextOver200k *rawTier  `json:"context_over_200k"`
	Tier            *struct {
		Size       int64  `json:"size"`
		Input      *int64 `json:"input"`
		Output     *int64 `json:"output"`
		CacheRead  *int64 `json:"cache_read"`
		CacheWrite *int64 `json:"cache_write"`
	} `json:"tier"`
}

type rawTier struct {
	Tier struct {
		Type string `json:"type"`
		Size int64  `json:"size"`
	} `json:"tier"`
	Size       int64    `json:"size"`
	Input      *float64 `json:"input"`
	Output     *float64 `json:"output"`
	CacheRead  *float64 `json:"cache_read"`
	CacheWrite *float64 `json:"cache_write"`
}

type rawVia struct {
	NPM   string `json:"npm"`
	API   string `json:"api"`
	Shape string `json:"shape"`
}

func parseSnapshotIndex(snaps []rawSnapshotProvider) *Index {
	providers := make(map[string]Provider, len(snaps))
	for _, sp := range snaps {
		p := Provider{
			ID:     sp.ID,
			Name:   firstNonEmpty(sp.Name, sp.ID),
			NPM:    sp.NPM,
			API:    normalizeURL(sp.API),
			Doc:    sp.Doc,
			Env:    sp.Env,
			Models: make(map[string]Model, len(sp.Models)),
		}
		for _, sm := range sp.Models {
			m := parseRawModel(sm.ID, sm)
			p.Models[m.ID] = m
		}
		providers[p.ID] = p
	}
	return &Index{Providers: providers}
}

func parseRawModel(fallbackID string, rm rawModelEntry) Model {
	id := firstNonEmpty(rm.ID, fallbackID)
	model := Model{
		ID:                 id,
		Name:               firstNonEmpty(rm.Name, id),
		Description:        rm.Description,
		Family:             rm.Family,
		Category:           guessCategory(id, rm.Family, rm.Reasoning, rawModelInputs(rm), rawModelOutputs(rm)),
		ContextWindow:      rawContextWindow(rm),
		MaxInput:           rawMaxInput(rm),
		MaxOutput:          rawMaxOutput(rm),
		SupportsTools:      boolPtr(rm.ToolCall),
		SupportsReasoning:  boolPtr(rm.Reasoning),
		ReasoningEfforts:   effortLadder(rm.Reasoning, rm.ReasoningOptions),
		ReasoningToggle:    reasoningToggle(rm.Reasoning, rm.ReasoningOptions),
		ReasoningBudget:    reasoningBudget(rm.Reasoning, rm.ReasoningOptions),
		ReasoningBudgetMin: reasoningBound(rm.Reasoning, rm.ReasoningOptions, true),
		ReasoningBudgetMax: reasoningBound(rm.Reasoning, rm.ReasoningOptions, false),
		SupportsVision:     boolPtr(hasModality(rawModelInputs(rm), "image")),
		Status:             normalizeStatus(rm.Status),
		ReleaseDate:        rm.ReleaseDate,
		Prices:             parsePrices(rm.Cost),
	}
	attachModelVia(&model, rm)
	return model
}

func rawModelInputs(rm rawModelEntry) []string {
	if rm.Modalities != nil && len(rm.Modalities.Input) > 0 {
		return rm.Modalities.Input
	}
	return rm.Input
}

func rawModelOutputs(rm rawModelEntry) []string {
	if rm.Modalities != nil && len(rm.Modalities.Output) > 0 {
		return rm.Modalities.Output
	}
	return rm.Output
}

func rawContextWindow(rm rawModelEntry) *int64 {
	if rm.Limit != nil && rm.Limit.Context.n != nil {
		return rm.Limit.Context.n
	}
	return rm.Context.n
}

func rawMaxInput(rm rawModelEntry) *int64 {
	if rm.Limit != nil {
		return rm.Limit.Input.n
	}
	return nil
}

func rawMaxOutput(rm rawModelEntry) *int64 {
	if rm.Limit != nil && rm.Limit.Output.n != nil {
		return rm.Limit.Output.n
	}
	return rm.MaxOutput.n
}

func attachModelVia(model *Model, rm rawModelEntry) {
	via := rm.Via
	if via == nil {
		via = rm.Provider
	}
	if via == nil {
		return
	}
	model.Via = TransportVia{
		NPM:   via.NPM,
		API:   normalizeURL(via.API),
		Shape: via.Shape,
	}
}

func parsePrices(cost *rawCost) catalog.Prices {
	if cost == nil {
		return catalog.Prices{}
	}

	p := catalog.Prices{
		Input:      usdToMicros(cost.Input),
		Output:     usdToMicros(cost.Output),
		CacheRead:  usdToMicros(cost.CacheRead),
		CacheWrite: usdToMicros(cost.CacheWrite),
	}

	if cost.Tier != nil {
		p.ExtThreshold = &cost.Tier.Size
		p.ExtInput = cost.Tier.Input
		p.ExtOutput = cost.Tier.Output
		p.ExtCacheRead = cost.Tier.CacheRead
		p.ExtCacheWrite = cost.Tier.CacheWrite
		return p
	}

	applyBestTier(&p, cost)
	return p
}

func applyBestTier(p *catalog.Prices, cost *rawCost) {
	bestTier := bestPriceTier(cost)
	if bestTier == nil {
		return
	}
	size := bestTier.tierSize()
	if size <= 0 {
		return
	}
	p.ExtThreshold = &size
	p.ExtInput = usdToMicros(bestTier.Input)
	p.ExtOutput = usdToMicros(bestTier.Output)
	p.ExtCacheRead = usdToMicros(bestTier.CacheRead)
	p.ExtCacheWrite = usdToMicros(bestTier.CacheWrite)
}

func bestPriceTier(cost *rawCost) *rawTier {
	var bestTier *rawTier
	for _, t := range cost.Tiers {
		if t.Tier.Type == "context" || t.Size > 0 {
			if bestTier == nil || t.tierSize() < bestTier.tierSize() {
				copied := t
				bestTier = &copied
			}
		}
	}
	if bestTier == nil && cost.ContextOver200k != nil {
		bestTier = cost.ContextOver200k
		if bestTier.Size == 0 && bestTier.Tier.Size == 0 {
			bestTier.Size = 200000
		}
	}
	return bestTier
}

func (t rawTier) tierSize() int64 {
	if t.Tier.Size > 0 {
		return t.Tier.Size
	}
	return t.Size
}

func usdToMicros(usd *float64) *int64 {
	if usd == nil {
		return nil
	}
	micros := int64(math.Round(*usd * 1000000.0))
	return &micros
}

func guessCategory(id, family string, reasoning bool, input, output []string) string {
	switch {
	case hasModality(output, "image"):
		return string(catalog.CategoryImage)
	case hasModality(output, "video"):
		return string(catalog.CategoryVideo)
	case hasModality(output, "audio") && !hasModality(output, "text"):
		return string(catalog.CategoryAudio)
	case strings.Contains(strings.ToLower(id), "embed"), strings.Contains(strings.ToLower(family), "embed"):
		return string(catalog.CategoryEmbedding)
	case reasoning:
		return string(catalog.CategoryReasoning)
	case hasModality(input, "image"):
		return string(catalog.CategoryVision)
	default:
		return string(catalog.CategoryChat)
	}
}

func normalizeStatus(val string) string {
	switch val {
	case "beta", "deprecated":
		return val
	default:
		return "active"
	}
}

func normalizeURL(raw string) string {
	trimmed := strings.TrimSuffix(strings.TrimSpace(raw), "/")
	return strings.TrimSuffix(trimmed, "/chat/completions")
}

func hasModality(modalities []string, wanted string) bool {
	for _, m := range modalities {
		if strings.EqualFold(m, wanted) {
			return true
		}
	}
	return false
}

func boolPtr(b bool) *bool {
	val := b
	return &val
}

func effortLadder(reasoning bool, options []rawReasoningOpt) []string {
	if !reasoning || options == nil {
		return nil
	}
	values := []string{}
	for _, option := range options {
		if option.Type != "effort" {
			continue
		}
		for _, value := range option.Values {
			if value = strings.TrimSpace(value); value != "" {
				values = append(values, value)
			}
		}
	}
	return values
}

func reasoningToggle(reasoning bool, options []rawReasoningOpt) bool {
	return reasoningOption(reasoning, options, "toggle")
}

func reasoningBudget(reasoning bool, options []rawReasoningOpt) bool {
	return reasoningOption(reasoning, options, "budget_tokens")
}

func reasoningOption(reasoning bool, options []rawReasoningOpt, kind string) bool {
	if !reasoning {
		return false
	}
	for _, option := range options {
		if option.Type == kind {
			return true
		}
	}
	return false
}

func reasoningBound(reasoning bool, options []rawReasoningOpt, minimum bool) *int64 {
	if !reasoning {
		return nil
	}
	for _, option := range options {
		if option.Type != "budget_tokens" {
			continue
		}
		bound := option.Max
		if minimum {
			bound = option.Min
		}
		if bound == nil {
			return nil
		}
		value := int64(*bound)
		return &value
	}
	return nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
