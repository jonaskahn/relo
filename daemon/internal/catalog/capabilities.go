// Provider-authoritative capabilities: one resolver for every surface.
package catalog

import "github.com/jonaskahn/relo/internal/inference"

// AccountFactsRecord is one credential's discovered capability facts for one
// model. Nil fields are unknown; an empty ReasoningEfforts slice is a stated
// absence of effort choices, which is different from a nil slice.
type AccountFactsRecord struct {
	CredentialID       string
	ProviderID         string
	ModelID            string
	Source             string
	ContextWindow      *int64
	MaxInput           *int64
	MaxOutput          *int64
	SupportsTools      *bool
	SupportsReasoning  *bool
	SupportsVision     *bool
	ReasoningEfforts   []string
	ReasoningToggle    *bool
	ReasoningBudget    *bool
	ReasoningBudgetMin *int64
	ReasoningBudgetMax *int64
	ObservedAtMs       int64
}

// Facts converts one account observation into the shared fact vocabulary.
func (r AccountFactsRecord) Facts() FactsRecord {
	return FactsRecord{
		ProviderID: r.ProviderID, ModelID: r.ModelID, Layer: r.Source,
		ContextWindow: r.ContextWindow, MaxInput: r.MaxInput, MaxOutput: r.MaxOutput,
		SupportsTools: r.SupportsTools, SupportsReasoning: r.SupportsReasoning,
		SupportsVision: r.SupportsVision, ReasoningEfforts: cloneEfforts(r.ReasoningEfforts),
		ReasoningToggle: r.ReasoningToggle, ReasoningBudget: r.ReasoningBudget,
		ReasoningBudgetMin: r.ReasoningBudgetMin, ReasoningBudgetMax: r.ReasoningBudgetMax,
	}
}

// CapabilitySources names the layer each protected value came from.
type CapabilitySources struct {
	ContextWindow      string
	MaxInput           string
	MaxOutput          string
	Tools              string
	Reasoning          string
	ReasoningEfforts   string
	ReasoningToggle    string
	ReasoningBudget    string
	ReasoningBudgetMin string
	ReasoningBudgetMax string
	Vision             string
}

// CapabilityEditability reports which protected fields remain open to manual
// completion. A field is editable only when no higher-priority source states
// it. Account editability is measured separately from model editability.
type CapabilityEditability struct {
	ContextWindow bool
	MaxInput      bool
	MaxOutput     bool
	Tools         bool
	Reasoning     bool
	Vision        bool
}

// CapabilityResolution is the effective protected capability set one attempt
// or one public catalog entry reads.
type CapabilityResolution struct {
	ContextWindow      *int64
	MaxInput           *int64
	MaxOutput          *int64
	SupportsTools      *bool
	SupportsReasoning  *bool
	SupportsVision     *bool
	ReasoningEfforts   []string
	ReasoningToggle    *bool
	ReasoningBudget    *bool
	ReasoningBudgetMin *int64
	ReasoningBudgetMax *int64
	Sources            CapabilitySources
	Editable           CapabilityEditability
	ManualInactive     CapabilityEditability
}

// CapabilityInputs are the facts one resolution reads, highest authority
// first. Account is optional because model-level definitions have no single
// credential.
type CapabilityInputs struct {
	Account   *FactsRecord
	Provider  FactsRecord
	ModelsDev FactsRecord
	Manual    FactsRecord
}

// ResolveCapabilities applies the authority order account, provider,
// models.dev, then manual completion. Reasoning is resolved as one family: a
// layer that states any reasoning control supplies every reasoning control,
// so a toggle-only provider cannot inherit an effort ladder from below.
func ResolveCapabilities(in CapabilityInputs) CapabilityResolution {
	layer := capabilityLayers(in)
	var res CapabilityResolution
	res.ContextWindow = pickCapabilityInt(layer, func(f FactsRecord) *int64 { return f.ContextWindow }, &res.Sources.ContextWindow)
	res.MaxInput = pickCapabilityInt(layer, func(f FactsRecord) *int64 { return f.MaxInput }, &res.Sources.MaxInput)
	res.MaxOutput = pickCapabilityInt(layer, func(f FactsRecord) *int64 { return f.MaxOutput }, &res.Sources.MaxOutput)
	res.SupportsTools = pickCapabilityBool(layer, func(f FactsRecord) *bool { return f.SupportsTools }, &res.Sources.Tools)
	res.SupportsVision = pickCapabilityBool(layer, func(f FactsRecord) *bool { return f.SupportsVision }, &res.Sources.Vision)
	res.SupportsReasoning = pickCapabilityBool(layer, func(f FactsRecord) *bool { return f.SupportsReasoning }, &res.Sources.Reasoning)
	applyReasoningFamily(&res, layer)
	res.Editable = editableCapabilities(layer)
	var manual FactsRecord
	for _, entry := range layer {
		if entry.name == "override" {
			manual = entry.facts
			break
		}
	}
	res.ManualInactive = inactiveManuals(manual, res.Sources)
	return res
}

type namedCapabilityLayer struct {
	name  string
	facts FactsRecord
}

func capabilityLayers(in CapabilityInputs) []namedCapabilityLayer {
	layers := make([]namedCapabilityLayer, 0, 4)
	if in.Account != nil {
		layers = append(layers, namedCapabilityLayer{name: "account", facts: *in.Account})
	}
	return append(layers,
		namedCapabilityLayer{name: "provider", facts: in.Provider},
		namedCapabilityLayer{name: "modelsdev", facts: in.ModelsDev},
		namedCapabilityLayer{name: "override", facts: in.Manual},
	)
}

func pickCapabilityInt(layers []namedCapabilityLayer, pick func(FactsRecord) *int64, source *string) *int64 {
	for _, layer := range layers {
		if value := pick(layer.facts); value != nil {
			*source = layer.name
			return value
		}
	}
	return nil
}

func pickCapabilityBool(layers []namedCapabilityLayer, pick func(FactsRecord) *bool, source *string) *bool {
	for _, layer := range layers {
		if value := pick(layer.facts); value != nil {
			*source = layer.name
			return value
		}
	}
	return nil
}

func applyReasoningFamily(res *CapabilityResolution, layers []namedCapabilityLayer) {
	chosen := -1
	for index, layer := range layers {
		if statesReasoning(layer.facts) {
			chosen = index
			break
		}
	}
	if chosen < 0 {
		return
	}
	applyReasoningLayer(res, layers[chosen])
}

// nolint:funlen
func applyReasoningLayer(res *CapabilityResolution, layer namedCapabilityLayer) {
	source := layer.name
	res.Sources.Reasoning = source
	if layer.facts.SupportsReasoning != nil {
		res.SupportsReasoning = layer.facts.SupportsReasoning
	} else {
		res.SupportsReasoning = boolPointer(true)
	}
	if res.SupportsReasoning != nil && !*res.SupportsReasoning {
		res.ReasoningEfforts = []string{}
		res.ReasoningToggle = boolPointer(false)
		res.ReasoningBudget = boolPointer(false)
		res.ReasoningBudgetMin = nil
		res.ReasoningBudgetMax = nil
		res.Sources.ReasoningEfforts = source
		res.Sources.ReasoningToggle = source
		res.Sources.ReasoningBudget = source
		res.Sources.ReasoningBudgetMin = source
		res.Sources.ReasoningBudgetMax = source
		return
	}
	res.ReasoningEfforts = cloneEfforts(layer.facts.ReasoningEfforts)
	if layer.facts.ReasoningEfforts != nil {
		res.Sources.ReasoningEfforts = source
	}
	res.ReasoningToggle = layer.facts.ReasoningToggle
	if res.ReasoningToggle != nil {
		res.Sources.ReasoningToggle = source
	}
	res.ReasoningBudget = layer.facts.ReasoningBudget
	if res.ReasoningBudget != nil {
		res.Sources.ReasoningBudget = source
	}
	res.ReasoningBudgetMin = layer.facts.ReasoningBudgetMin
	if res.ReasoningBudgetMin != nil {
		res.Sources.ReasoningBudgetMin = source
	}
	res.ReasoningBudgetMax = layer.facts.ReasoningBudgetMax
	if res.ReasoningBudgetMax != nil {
		res.Sources.ReasoningBudgetMax = source
	}
}

func statesReasoning(f FactsRecord) bool {
	return f.SupportsReasoning != nil || f.ReasoningEfforts != nil ||
		f.ReasoningToggle != nil || f.ReasoningBudget != nil ||
		f.ReasoningBudgetMin != nil || f.ReasoningBudgetMax != nil
}

func editableCapabilities(layers []namedCapabilityLayer) CapabilityEditability {
	editable := CapabilityEditability{ContextWindow: true, MaxInput: true, MaxOutput: true, Tools: true, Reasoning: true, Vision: true}
	for _, layer := range layers {
		if layer.name == "override" {
			continue
		}
		editable.ContextWindow = editable.ContextWindow && layer.facts.ContextWindow == nil
		editable.MaxInput = editable.MaxInput && layer.facts.MaxInput == nil
		editable.MaxOutput = editable.MaxOutput && layer.facts.MaxOutput == nil
		editable.Tools = editable.Tools && layer.facts.SupportsTools == nil
		editable.Reasoning = editable.Reasoning && !statesReasoning(layer.facts)
		editable.Vision = editable.Vision && layer.facts.SupportsVision == nil
	}
	return editable
}

func cloneEfforts(efforts []string) []string {
	if efforts == nil {
		return nil
	}
	cloned := make([]string, len(efforts))
	copy(cloned, efforts)
	return cloned
}

func inactiveManuals(manual FactsRecord, sources CapabilitySources) CapabilityEditability {
	return CapabilityEditability{
		ContextWindow: manual.ContextWindow != nil && sources.ContextWindow != "override",
		MaxInput:      manual.MaxInput != nil && sources.MaxInput != "override",
		MaxOutput:     manual.MaxOutput != nil && sources.MaxOutput != "override",
		Tools:         manual.SupportsTools != nil && sources.Tools != "override",
		Reasoning:     statesReasoning(manual) && sources.Reasoning != "override",
		Vision:        manual.SupportsVision != nil && sources.Vision != "override",
	}
}

// ApplyAccountCapabilities overlays one credential's discovered facts on a
// model resolved from the shared layers. It returns a copy so retries and
// route candidates never mutate the catalog snapshot.
func ApplyAccountCapabilities(model Model, account FactsRecord) Model {
	res := ResolveCapabilities(CapabilityInputs{
		Account: &account, Provider: model.FactsProvider.factsRecord(),
		ModelsDev: model.FactsModelsDev.factsRecord(), Manual: model.FactsOverride.factsRecord(),
	})
	applyCapabilityResolution(&model, res)
	return model
}

// ApplyAccountFactRecord is the typed form of ApplyAccountCapabilities.
func ApplyAccountFactRecord(model Model, account AccountFactsRecord) Model {
	return ApplyAccountCapabilities(model, account.Facts())
}

func applyCapabilityResolution(model *Model, res CapabilityResolution) {
	model.ContextWindow = res.ContextWindow
	model.MaxInput = res.MaxInput
	model.MaxOutput = res.MaxOutput
	model.SupportsTools = res.SupportsTools
	model.SupportsReasoning = res.SupportsReasoning
	model.SupportsVision = res.SupportsVision
	model.ReasoningEfforts = cloneEfforts(res.ReasoningEfforts)
	model.ReasoningToggle = valueOfBool(res.ReasoningToggle)
	model.ReasoningToggleKnown = res.ReasoningToggle != nil
	model.ReasoningBudget = valueOfBool(res.ReasoningBudget)
	model.ReasoningBudgetKnown = res.ReasoningBudget != nil
	model.ReasoningBudgetMin = res.ReasoningBudgetMin
	model.ReasoningBudgetMax = res.ReasoningBudgetMax
	model.FactSources.ContextWindow = res.Sources.ContextWindow
	model.FactSources.MaxInput = res.Sources.MaxInput
	model.FactSources.MaxOutput = res.Sources.MaxOutput
	model.FactSources.Tools = res.Sources.Tools
	model.FactSources.Reasoning = res.Sources.Reasoning
	model.FactSources.ReasoningEfforts = res.Sources.ReasoningEfforts
	model.FactSources.ReasoningToggle = res.Sources.ReasoningToggle
	model.FactSources.ReasoningBudget = res.Sources.ReasoningBudget
	model.FactSources.ReasoningBudgetMin = res.Sources.ReasoningBudgetMin
	model.FactSources.ReasoningBudgetMax = res.Sources.ReasoningBudgetMax
	model.FactSources.Vision = res.Sources.Vision
}

// ConservativeCapabilities intersects resolved models so a shared public name
// never promises more than every member can serve. A value is reported only
// when every member states one; any unknown makes the shared value unknown.
func ConservativeCapabilities(models []Model) CapabilityResolution {
	if len(models) == 0 {
		return CapabilityResolution{}
	}
	res := CapabilityResolution{
		ContextWindow:      models[0].ContextWindow,
		MaxInput:           models[0].MaxInput,
		MaxOutput:          models[0].MaxOutput,
		SupportsTools:      models[0].SupportsTools,
		SupportsReasoning:  models[0].SupportsReasoning,
		SupportsVision:     models[0].SupportsVision,
		ReasoningEfforts:   cloneEfforts(models[0].ReasoningEfforts),
		ReasoningBudgetMin: models[0].ReasoningBudgetMin,
		ReasoningBudgetMax: models[0].ReasoningBudgetMax,
	}
	res.ReasoningToggle = knownBool(models[0].ReasoningToggleKnown, models[0].ReasoningToggle)
	res.ReasoningBudget = knownBool(models[0].ReasoningBudgetKnown, models[0].ReasoningBudget)
	for _, model := range models[1:] {
		res.ContextWindow = smallerKnown(res.ContextWindow, model.ContextWindow)
		res.MaxInput = smallerKnown(res.MaxInput, model.MaxInput)
		res.MaxOutput = smallerKnown(res.MaxOutput, model.MaxOutput)
		res.SupportsTools = andKnown(res.SupportsTools, model.SupportsTools)
		res.SupportsReasoning = andKnown(res.SupportsReasoning, model.SupportsReasoning)
		res.SupportsVision = andKnown(res.SupportsVision, model.SupportsVision)
		res.ReasoningToggle = andKnown(res.ReasoningToggle, knownBool(model.ReasoningToggleKnown, model.ReasoningToggle))
		res.ReasoningBudget = andKnown(res.ReasoningBudget, knownBool(model.ReasoningBudgetKnown, model.ReasoningBudget))
		res.ReasoningBudgetMin = largerKnown(res.ReasoningBudgetMin, model.ReasoningBudgetMin)
		res.ReasoningBudgetMax = smallerKnown(res.ReasoningBudgetMax, model.ReasoningBudgetMax)
		res.ReasoningEfforts = intersectEfforts(res.ReasoningEfforts, model.ReasoningEfforts)
	}
	return res
}

// toggleOnlyProjection is the synthetic level pair a toggle-only model shows
// on client surfaces. It is presentation-only and is never stored as a fact.
var toggleOnlyProjection = []string{"none", "high"}

// sharedReasoningLadder is the ladder a client offers when a model states no
// effort list. It repeats the adapter's shared list.
var sharedReasoningLadder = []string{"low", "medium", "high", "xhigh", "max"}

// ToggleOnly reports a reasoning model whose facts state an empty effort
// ladder and an on/off thinking switch, with no effort intensity of its own.
func ToggleOnly(model Model) bool {
	return model.SupportsReasoning != nil && *model.SupportsReasoning &&
		inference.ToggleOnly(model.ReasoningEfforts, model.ReasoningToggle)
}

// ClientReasoningEfforts is the effort list a client surface shows for one
// model. A toggle-only model gets the synthetic off/on pair; every other
// model keeps the ladder it stated.
func ClientReasoningEfforts(model Model) []string {
	if ToggleOnly(model) {
		return append([]string(nil), toggleOnlyProjection...)
	}
	return cloneEfforts(model.ReasoningEfforts)
}

// SharedReasoning is the effort list and toggle a route publishes. A
// toggle-only member accepts every positive effort, so it does not narrow the
// ladder; an all-toggle route advertises the synthetic off/on pair. A nil
// ladder keeps the shared fallback, while an explicit empty ladder on a
// non-toggle member forces an empty intersection. The toggle is true only
// when every member carries one.
func SharedReasoning(members []Model) ([]string, bool) {
	if len(members) == 0 {
		return nil, false
	}
	toggle := true
	for _, model := range members {
		if !model.ReasoningToggle {
			toggle = false
		}
	}
	if allToggleOnly(members) {
		return append([]string(nil), toggleOnlyProjection...), toggle
	}
	return sharedStatedLadder(members), toggle
}

func allToggleOnly(members []Model) bool {
	for _, model := range members {
		if !ToggleOnly(model) {
			return false
		}
	}
	return true
}

func sharedStatedLadder(members []Model) []string {
	var shared []string
	stated := false
	for _, model := range members {
		if ToggleOnly(model) {
			continue
		}
		ladder := model.ReasoningEfforts
		if ladder == nil {
			ladder = sharedReasoningLadder
		}
		if !stated {
			shared = append([]string(nil), ladder...)
			stated = true
			continue
		}
		shared = intersectEfforts(shared, ladder)
	}
	return shared
}

func indexAdvertisedModels(snapshot *Snapshot, facts []AccountFactsRecord) {
	roster, ok := snapshot.accounts.(AccountRoster)
	if !ok || len(facts) == 0 {
		return
	}
	byCredential := groupAccountFacts(facts)
	for _, host := range snapshot.Providers {
		credentials := append([]string(nil), roster.ActiveCredentials(host.ID)...)
		if len(credentials) == 0 {
			continue
		}
		for _, model := range snapshot.byProviderM[host.ID] {
			if advertised, ok := advertisedModelForAccounts(roster, credentials, byCredential, model); ok {
				snapshot.advertised[key{providerID: model.ProviderID, modelID: model.ID}] = advertised
			}
		}
	}
}

func groupAccountFacts(facts []AccountFactsRecord) map[string]map[key][]AccountFactsRecord {
	byCredential := make(map[string]map[key][]AccountFactsRecord)
	for _, fact := range facts {
		if byCredential[fact.CredentialID] == nil {
			byCredential[fact.CredentialID] = make(map[key][]AccountFactsRecord)
		}
		k := key{providerID: fact.ProviderID, modelID: fact.ModelID}
		byCredential[fact.CredentialID][k] = append(byCredential[fact.CredentialID][k], fact)
	}
	return byCredential
}

func advertisedModelForAccounts(roster AccountRoster, credentials []string, facts map[string]map[key][]AccountFactsRecord, model Model) (Model, bool) {
	effective := make([]Model, 0, len(credentials))
	for _, credentialID := range credentials {
		if !roster.AccountCanServe(credentialID, modelIDsOf(model)) {
			continue
		}
		combined := CombineAccountFacts(facts[credentialID][key{providerID: model.ProviderID, modelID: model.ID}], model)
		if combined == nil {
			effective = append(effective, model)
			continue
		}
		effective = append(effective, ApplyAccountCapabilities(model, *combined))
	}
	if len(effective) == 0 {
		return Model{}, false
	}
	return conservativeModel(model, effective), true
}

// CombineAccountFacts folds one credential's discovered rows and manual
// completions into the account layer a resolver reads. A manual completion is
// used only when every higher layer is silent about that field.
func CombineAccountFacts(rows []AccountFactsRecord, model Model) *FactsRecord {
	if len(rows) == 0 {
		return nil
	}
	combined := FactsRecord{Layer: "account"}
	for _, row := range rows {
		if row.Source != "manual" {
			mergeAccountFacts(&combined, row.Facts(), true)
		}
	}
	for _, row := range rows {
		if row.Source == "manual" {
			mergeManualAccountFacts(&combined, row.Facts(), model)
		}
	}
	if combined.Empty() {
		return nil
	}
	return &combined
}

func mergeManualAccountFacts(into *FactsRecord, from FactsRecord, model Model) {
	if model.FactsProvider.ContextWindow != nil || model.FactsModelsDev.ContextWindow != nil {
		from.ContextWindow = nil
	}
	if model.FactsProvider.MaxInput != nil || model.FactsModelsDev.MaxInput != nil {
		from.MaxInput = nil
	}
	if model.FactsProvider.MaxOutput != nil || model.FactsModelsDev.MaxOutput != nil {
		from.MaxOutput = nil
	}
	if model.FactsProvider.Tools != nil || model.FactsModelsDev.Tools != nil {
		from.SupportsTools = nil
	}
	if model.FactsProvider.Reasoning != nil || model.FactsModelsDev.Reasoning != nil ||
		model.FactsProvider.MaxOutput != nil || model.FactsModelsDev.MaxOutput != nil {
		from.SupportsReasoning = nil
		from.ReasoningEfforts = nil
		from.ReasoningToggle = nil
		from.ReasoningBudget = nil
		from.ReasoningBudgetMin = nil
		from.ReasoningBudgetMax = nil
	}
	if model.FactsProvider.Vision != nil || model.FactsModelsDev.Vision != nil {
		from.SupportsVision = nil
	}
	mergeAccountFacts(into, from, false)
}

func mergeAccountFacts(into *FactsRecord, from FactsRecord, overwrite bool) {
	setInt := func(dst **int64, src *int64) {
		if src != nil && (overwrite || *dst == nil) {
			*dst = src
		}
	}
	setBool := func(dst **bool, src *bool) {
		if src != nil && (overwrite || *dst == nil) {
			*dst = src
		}
	}
	setInt(&into.ContextWindow, from.ContextWindow)
	setInt(&into.MaxInput, from.MaxInput)
	setInt(&into.MaxOutput, from.MaxOutput)
	setBool(&into.SupportsTools, from.SupportsTools)
	setBool(&into.SupportsReasoning, from.SupportsReasoning)
	setBool(&into.SupportsVision, from.SupportsVision)
	if overwrite || into.ReasoningEfforts == nil {
		into.ReasoningEfforts = cloneEfforts(from.ReasoningEfforts)
	}
	setBool(&into.ReasoningToggle, from.ReasoningToggle)
	setBool(&into.ReasoningBudget, from.ReasoningBudget)
	setInt(&into.ReasoningBudgetMin, from.ReasoningBudgetMin)
	setInt(&into.ReasoningBudgetMax, from.ReasoningBudgetMax)
}

func conservativeModel(base Model, effective []Model) Model {
	if len(effective) == 1 {
		return effective[0]
	}
	res := ConservativeCapabilities(effective)
	applyCapabilityResolution(&base, res)
	for _, source := range []*string{
		&base.FactSources.ContextWindow, &base.FactSources.MaxInput, &base.FactSources.MaxOutput,
		&base.FactSources.Tools, &base.FactSources.Reasoning, &base.FactSources.Vision,
		&base.FactSources.ReasoningToggle, &base.FactSources.ReasoningBudget,
	} {
		if *source != "" {
			*source = "account"
		}
	}
	return base
}

func modelIDsOf(model Model) []string {
	ids := make([]string, 0, 3)
	for _, id := range []string{model.ID, model.UpstreamID, model.ClonedFrom} {
		if id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

func smallerKnown(current, candidate *int64) *int64 {
	if current == nil || candidate == nil {
		return nil
	}
	if *candidate < *current {
		value := *candidate
		return &value
	}
	value := *current
	return &value
}

func largerKnown(current, candidate *int64) *int64 {
	if current == nil || candidate == nil {
		return nil
	}
	if *candidate > *current {
		value := *candidate
		return &value
	}
	value := *current
	return &value
}

func andKnown(left, right *bool) *bool {
	if left == nil || right == nil {
		return nil
	}
	value := *left && *right
	return &value
}

func intersectEfforts(left, right []string) []string {
	if left == nil || right == nil {
		return nil
	}
	allowed := make(map[string]bool, len(right))
	for _, effort := range right {
		allowed[effort] = true
	}
	kept := make([]string, 0, len(left))
	for _, effort := range left {
		if allowed[effort] {
			kept = append(kept, effort)
		}
	}
	return kept
}

func knownBool(known, value bool) *bool {
	if !known {
		return nil
	}
	return boolPointer(value)
}

func boolPointer(value bool) *bool {
	return &value
}

func valueOfBool(value *bool) bool {
	return value != nil && *value
}

func (f Facts) factsRecord() FactsRecord {
	return FactsRecord{
		ContextWindow: f.ContextWindow, MaxInput: f.MaxInput, MaxOutput: f.MaxOutput,
		SupportsTools: f.Tools, SupportsReasoning: f.Reasoning, SupportsVision: f.Vision,
	}
}
