// Connection views: providers, counts, and prices.
package catalog

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/jonaskahn/relo/internal/account"
	appaccount "github.com/jonaskahn/relo/internal/application/account"
	approuting "github.com/jonaskahn/relo/internal/application/routing"
	"github.com/jonaskahn/relo/internal/catalog"
)

// Catalog errors name the refusals a console maps to its own wording:
// missing rows, rows another object uses, rejected credentials, and listings
// that could not be read.
var (
	ErrModelNotFound = errors.New("model not found")
	ErrGroupNotFound = errors.New("group not found")
	// ErrInvalidCatalogRow reports a catalog write the rules refuse, which is
	// the account lifecycle's own sentinel for the same class of refusal.
	ErrInvalidCatalogRow = appaccount.ErrInvalidCatalogRow
	// ErrProviderNotFound names a connection id that matches nothing, which
	// is the account lifecycle's own sentinel: both features answer the same
	// "no such connection".
	ErrProviderNotFound = appaccount.ErrProviderNotFound
	// ErrInvalidRouteMember reports a route member the catalog cannot serve.
	ErrInvalidRouteMember = approuting.ErrInvalidRouteMember
	ErrCatalogConflict    = errors.New("the catalog row is in use")
	ErrNotCustom          = errors.New("only a row the operator added can change this")
	ErrProbeNotFound      = errors.New("probe session not found or expired")
	ErrCredentialRejected = errors.New("credential rejected by upstream")
	ErrListingFailed      = errors.New("model listing failed")
	// ErrUnknownModelsDevRef names a Priced as value that the saved
	// models.dev copy does not hold.
	ErrUnknownModelsDevRef = errors.New("unknown models.dev reference")
	// ErrListAuthoritative names a connection whose own model list decides
	// which models it serves, so no model can be added to it by hand.
	ErrListAuthoritative = errors.New("this connection publishes its own model list")
)

// Prices represents token pricing rates in integer USD micros per million tokens.
type Prices struct {
	Input         *int64
	Output        *int64
	CacheRead     *int64
	CacheWrite    *int64
	ExtThreshold  *int64
	ExtInput      *int64
	ExtOutput     *int64
	ExtCacheRead  *int64
	ExtCacheWrite *int64
}

// PriceSourceMap names where each effective rate came from: the operator,
// the provider, or the model index.
type PriceSourceMap struct {
	Input         string
	Output        string
	CacheRead     string
	CacheWrite    string
	ExtThreshold  string
	ExtInput      string
	ExtOutput     string
	ExtCacheRead  string
	ExtCacheWrite string
}

// ProviderCounts totals one connection's models and accounts for its status page.
type ProviderCounts struct {
	Models          int
	EnabledModels   int
	AvailableModels int
	UnpricedModels  int
	Accounts        int
	ActiveAccounts  int
	PausedAccounts  int
	ReauthAccounts  int
}

// Provider is one connection as a console reads it: settings, roster totals, and health.
type Provider struct {
	ID         string
	TemplateID string
	Label      string
	// Kind is the group a console lists this provider under, and
	// AvailableFormats is every shape it may be switched to. VariableDefs is
	// what its base URL still needs.
	Kind             string
	AvailableFormats []catalog.FormatOption
	VariableDefs     []catalog.Variable
	Origin           string
	Auth             string
	APIFormat        string
	APIFormats       []string
	KeyHeader        string
	ModelsSource     string
	ModelsFormat     string
	// ModelsPerAccount reports whether each account of this connection
	// publishes a roster of its own, which is what makes an account-level
	// model list worth reading and showing.
	ModelsPerAccount    bool
	ModelsDevProviderID string
	BaseURL             string
	DocURL              string
	KeyEnv              []string
	LoginFlows          []string
	// LoginMethods is how this provider signs in, which is what a console
	// offers when an operator adds another account to it.
	LoginMethods     []catalog.LoginMethod
	Headers          map[string]string
	Variables        map[string]string
	NeedsSetup       []string
	Routable         bool
	UnroutableReason string
	Enabled          bool
	UseProxy         bool
	// TimeoutSeconds is this connection's own call wait in seconds; null
	// inherits the global value. RetryBackoff is its own retry windows; null
	// inherits the global windows.
	TimeoutSeconds *int
	RetryBackoff   [][2]int
	// SwitchOn4xx and SwitchOn5xx report whether this connection fails over on
	// an unlisted status of that class. Both are resolved, so a console never
	// repeats the default.
	SwitchOn4xx       bool
	SwitchOn5xx       bool
	Rank              int
	PoolStrategy      string
	Configured        bool
	LastRefreshedAtMs *int64
	LastRefreshError  string
	Counts            ProviderCounts
	CreatedAtMs       int64
	UpdatedAtMs       int64
}

// Capabilities are the tools, reasoning, and vision flags one model offers,
// null where no layer states one.
type Capabilities struct {
	Tools     *bool
	Reasoning *bool
	Vision    *bool
}

// DetailLayer is one layer of a model's details exactly as it is stored. A
// null field is a layer that is silent about that value rather than one that
// clears it.
type DetailLayer struct {
	Name          *string
	Description   *string
	Family        *string
	Category      *string
	ContextWindow *int64
	MaxInput      *int64
	MaxOutput     *int64
	Tools         *bool
	Reasoning     *bool
	Vision        *bool
	Status        *string
	ReleaseDate   *string
}

// DetailSourceMap names the layer each resolved detail came from, the way
// PriceSourceMap does for rates.
type DetailSourceMap struct {
	Name          string
	Description   string
	Family        string
	Category      string
	ContextWindow string
	MaxInput      string
	MaxOutput     string
	Tools         string
	Reasoning     string
	Vision        string
	Status        string
	ReleaseDate   string
}

// ModelDetails is every detail layer of one model beside the name of the
// layer each resolved value came from, which is what lets a console show
// where a price or a context window actually came from.
type ModelDetails struct {
	Override        DetailLayer
	Provider        DetailLayer
	ModelsDev       DetailLayer
	EffectiveSource DetailSourceMap
}

// ModelPrices carries one model's rates from every layer beside the
// effective rate a request pays.
type ModelPrices struct {
	Override        Prices
	Provider        Prices
	ModelsDev       Prices
	Effective       Prices
	EffectiveSource PriceSourceMap
}

// ContextLayers is the context window each layer states, exactly as stored, so
// a console can show where the effective value came from and offer to fall
// back to a layer below it. A null layer is silent about the value rather than
// stating zero.
type ContextLayers struct {
	Override  *int64
	Provider  *int64
	ModelsDev *int64
}

// Model is one served model as a console reads it: facts, capabilities,
// rates, and route membership.
type Model struct {
	ProviderID string
	ModelID    string
	// UpstreamModelID is the identifier Relo sends to the provider, which a
	// clone points at a model the provider has not listed yet. ClonedFrom
	// names the model a clone was copied from, empty on every other row.
	UpstreamModelID string
	ClonedFrom      string
	Source          string
	Name            string
	Description     string
	Family          string
	Category        string
	APIFormat       string
	BaseURL         string
	ModelsDevRef    string
	Match           string
	ContextWindow   *int64
	MaxInput        *int64
	MaxOutput       *int64
	ContextLayers   ContextLayers
	ContextSource   string
	// MaxOutputLayers is the max output each layer states, and MaxOutputSource
	// names the layer the effective value came from, the same way the context
	// window reports its own layers.
	MaxOutputLayers ContextLayers
	MaxOutputSource string
	Capabilities    Capabilities
	// CapabilityOverride is the operator's own tools, reasoning, and vision,
	// null where they have not set one. The effective flags stay on Capabilities.
	CapabilityOverride Capabilities
	Status             string
	ReleaseDate        string
	Enabled            bool
	Available          *bool
	Routable           bool
	UnroutableReason   string
	Prices             ModelPrices
	GroupRefs          []string
	// Overridden reports whether an operator changed anything about this
	// model, and Details carries every layer of its facts. Details is present
	// on a single model read, not on a list.
	Overridden  bool
	Details     *ModelDetails
	ListedAtMs  *int64
	UpdatedAtMs int64
	// ServingAccounts and ActiveAccounts report how many accounts of the
	// connection could take a request now and how many of them can serve this
	// model. They are equal on a connection whose accounts share one roster.
	ServingAccounts int
	ActiveAccounts  int
}

// GroupMember is one model inside a route, with its weight and current eligibility.
type GroupMember struct {
	ProviderID string
	ModelID    string
	// Kind is how the member names its model: one connection's own row, or a
	// bare identifier resolved against every connection that serves it.
	Kind     string
	Weight   int
	Enabled  bool
	Eligible bool
	Reason   string
	// ServingAccounts and ActiveAccounts report how many accounts behind the
	// member could take a request now and how many of them can serve the model,
	// which is what tells a member that only lists from one that routes.
	ServingAccounts int
	ActiveAccounts  int
}

// Group is one named route as a console reads it: members, strategy, and
// the budgets and capabilities it can promise.
type Group struct {
	ID       string
	Label    string
	Strategy string
	Enabled  bool
	Listed   bool
	// ClientID is the name an agent asks for, which is the route's own
	// identifier under the namespace Relo publishes.
	ClientID     string
	ShadowsModel bool
	Members      []GroupMember
	// SwitchOn4xx and SwitchOn5xx report whether this route moves to its next
	// member on an unlisted status of that class. Both are resolved, so a
	// console never repeats the default.
	SwitchOn4xx bool
	SwitchOn5xx bool
	// ContextWindow and MaxOutput are the budget a route can promise whichever
	// member answers: the smallest the enabled members state, so a prompt that
	// fits them fits every one of them.
	ContextWindow *int64
	MaxOutput     *int64
	// CapabilityWarnings names each capability the route cannot promise when a
	// client reads the published model list: a member lacking reasoning or
	// vision, or a mix of tool-capable and tool-less members.
	CapabilityWarnings []string
	// SupportsTools, SupportsReasoning and SupportsVision are what the route
	// can promise whichever member answers. True only when at least one
	// reachable member states the capability and none deny it. False when any
	// reachable member denies it. Nil when no reachable member states it.
	SupportsTools     *bool
	SupportsReasoning *bool
	SupportsVision    *bool
	CreatedAtMs       int64
	UpdatedAtMs       int64
}

// ProviderQuery narrows a provider listing.
type ProviderQuery struct {
	Origin     string
	Auth       string
	Search     string
	Configured bool
}

func (q ProviderQuery) matches(host catalog.Provider) bool {
	if q.Origin != "" && string(host.Origin) != q.Origin {
		return false
	}
	if q.Auth != "" && string(host.Auth) != q.Auth {
		return false
	}
	if q.Configured && !host.Configured {
		return false
	}
	if q.Search == "" {
		return true
	}
	return matchesText(host.ID, host.Label, q.Search)
}

// Providers returns all configured providers matching query.
func (s *Service) Providers(ctx context.Context, query ProviderQuery) ([]Provider, error) {
	snapshot, err := s.snapshot()
	if err != nil {
		return nil, err
	}
	rows := make([]Provider, 0, len(snapshot.Providers))
	for _, host := range snapshot.Providers {
		if !query.matches(host) {
			continue
		}
		rows = append(rows, s.describeProvider(snapshot, host))
	}
	return rows, nil
}

// Provider returns one provider by id.
func (s *Service) Provider(ctx context.Context, id string) (Provider, error) {
	snapshot, err := s.snapshot()
	if err != nil {
		return Provider{}, err
	}
	host, found := snapshot.Provider(id)
	if !found {
		return Provider{}, fmt.Errorf("%s: %w", id, ErrProviderNotFound)
	}
	return s.describeProvider(snapshot, host), nil
}

func (s *Service) describeProvider(snapshot *catalog.Snapshot, host catalog.Provider) Provider {
	models := snapshot.Models(host.ID)
	row := baseProviderView(s, host, models)
	row.Counts = countModels(models)
	if s.pools != nil {
		countProviderAccounts(s.pools, host.ID, &row.Counts)
	}
	return row
}

func baseProviderView(s *Service, host catalog.Provider, models []catalog.Model) Provider {
	view := providerIdentityView(s, host, models)
	view.TimeoutSeconds = host.TimeoutSeconds
	view.RetryBackoff = host.RetryBackoff
	view.SwitchOn4xx = host.SwitchOn4xx
	view.SwitchOn5xx = host.SwitchOn5xx
	view.Rank = host.Rank
	view.PoolStrategy = host.PoolStrategy
	view.Configured = host.Configured
	view.LastRefreshedAtMs = host.LastRefreshedAtMs
	view.LastRefreshError = host.LastRefreshError
	view.CreatedAtMs = host.CreatedAtMs
	view.UpdatedAtMs = host.UpdatedAtMs
	return view
}

func providerIdentityView(s *Service, host catalog.Provider, models []catalog.Model) Provider {
	view := Provider{
		ID:                  host.ID,
		TemplateID:          host.TemplateID,
		Label:               host.Label,
		Origin:              string(host.Origin),
		Auth:                string(host.Auth),
		APIFormat:           string(host.APIFormat),
		KeyHeader:           string(host.KeyHeader),
		ModelsSource:        host.ModelsSource,
		ModelsFormat:        string(host.ModelsFormat),
		ModelsDevProviderID: host.ModelsDevProviderID,
		BaseURL:             host.BaseURL,
		DocURL:              host.DocURL,
		KeyEnv:              host.KeyEnv,
		LoginFlows:          host.LoginFlows,
		Headers:             host.Headers,
		Variables:           host.Variables,
		NeedsSetup:          host.NeedsSetup,
		Routable:            host.Routable,
		UnroutableReason:    host.UnroutableReason,
		Enabled:             host.Enabled,
		UseProxy:            host.UseProxy,
	}
	fillProviderDerived(&view, s, host, models)
	return view
}

func fillProviderDerived(view *Provider, s *Service, host catalog.Provider, models []catalog.Model) {
	view.Kind = s.providerKind(host)
	view.AvailableFormats = s.providerFormats(host)
	view.VariableDefs = s.providerVariables(host)
	view.APIFormats = apiFormats(models)
	view.ModelsPerAccount = catalog.PerAccountRoster(host.ModelsFormat)
	view.LoginMethods = s.providerLoginMethods(host)
}

func countProviderAccounts(pools *account.Manager, providerID string, counts *ProviderCounts) {
	entries := pools.GetPool(providerID).Entries()
	counts.Accounts = len(entries)
	counts.ActiveAccounts = pools.Active(providerID)
	for _, entry := range entries {
		switch entry.Status {
		case account.StatusPaused:
			counts.PausedAccounts++
		case account.StatusNeedsReauth:
			counts.ReauthAccounts++
		}
	}
}

func countModels(models []catalog.Model) ProviderCounts {
	counts := ProviderCounts{}
	for _, m := range models {
		counts.Models++
		if m.Enabled {
			counts.EnabledModels++
		}
		if m.Available == nil || *m.Available {
			counts.AvailableModels++
		}
		if !m.Prices.Known() {
			counts.UnpricedModels++
		}
	}
	return counts
}

func apiFormats(models []catalog.Model) []string {
	seen := map[string]bool{}
	formats := []string{}
	for _, m := range models {
		if m.APIFormat == catalog.FormatUnsupported || seen[string(m.APIFormat)] {
			continue
		}
		seen[string(m.APIFormat)] = true
		formats = append(formats, string(m.APIFormat))
	}
	sort.Strings(formats)
	return formats
}

// ModelQuery narrows a model listing.
type ModelQuery struct {
	Provider   string
	Category   string
	Origin     string
	Search     string
	Available  string
	Enabled    *bool
	Overridden bool
	Unpriced   bool
	Configured bool
	Limit      int
	Offset     int
}

// Models returns models matching a query.
func (s *Service) Models(ctx context.Context, query ModelQuery) ([]Model, int, error) {
	snapshot, err := s.snapshot()
	if err != nil {
		return nil, 0, err
	}
	matched := make([]Model, 0, 64)
	for _, host := range snapshot.Providers {
		if query.Provider != "" && host.ID != query.Provider {
			continue
		}
		if query.Configured && !host.Configured {
			continue
		}
		for _, model := range snapshot.Models(host.ID) {
			if !query.matches(model) {
				continue
			}
			matched = append(matched, s.describeModel(snapshot, model, false))
		}
	}
	total := len(matched)
	return page(matched, query.Offset, query.Limit), total, nil
}

func (q ModelQuery) matches(model catalog.Model) bool {
	if q.Category != "" && string(model.Category) != q.Category {
		return false
	}
	if q.Enabled != nil && model.Enabled != *q.Enabled {
		return false
	}
	if !availableMatches(model, q.Available) {
		return false
	}
	if q.Overridden && !hasOverrides(model) {
		return false
	}
	if q.Unpriced && model.Prices.Known() {
		return false
	}
	return matchesText(model.ID, model.Name, q.Search)
}

func availableMatches(model catalog.Model, wanted string) bool {
	switch wanted {
	case "true":
		return model.Available != nil && *model.Available
	case "false":
		return model.Available != nil && !*model.Available
	case "unknown":
		return model.Available == nil
	default:
		return true
	}
}

func hasOverrides(model catalog.Model) bool {
	return model.Overridden
}

// Model returns one model by provider and model ID.
func (s *Service) Model(ctx context.Context, providerID, modelID string) (Model, error) {
	snapshot, err := s.snapshot()
	if err != nil {
		return Model{}, err
	}
	m, found := snapshot.Model(providerID, modelID)
	if !found {
		return Model{}, fmt.Errorf("%s/%s: %w", providerID, modelID, ErrModelNotFound)
	}
	return s.describeModel(snapshot, m, true), nil
}

func (s *Service) describeModel(snapshot *catalog.Snapshot, model catalog.Model, withDetails bool) Model {
	described := baseModelView(model)
	describeModelLayers(&described, model)
	describeModelPrices(&described, model)
	described.GroupRefs = groupsOf(snapshot, model)
	described.ServingAccounts, described.ActiveAccounts = snapshot.Coverage(model)
	if withDetails {
		described.Details = &ModelDetails{
			Override:        toDetailLayer(model.FactsOverride),
			Provider:        toDetailLayer(model.FactsProvider),
			ModelsDev:       toDetailLayer(model.FactsModelsDev),
			EffectiveSource: toDetailSourceMap(model.FactSources),
		}
	}
	return described
}

func baseModelView(model catalog.Model) Model {
	return Model{
		ProviderID:       model.ProviderID,
		ModelID:          model.ID,
		UpstreamModelID:  model.UpstreamID,
		ClonedFrom:       model.ClonedFrom,
		Source:           model.Source,
		Name:             model.Name,
		Description:      model.Description,
		Family:           model.Family,
		Category:         string(model.Category),
		APIFormat:        string(model.APIFormat),
		BaseURL:          model.BaseURL,
		ModelsDevRef:     model.ModelsDevRef,
		Match:            model.Match,
		ContextWindow:    model.ContextWindow,
		MaxInput:         model.MaxInput,
		MaxOutput:        model.MaxOutput,
		Status:           model.Status,
		ReleaseDate:      model.ReleaseDate,
		Enabled:          model.Enabled,
		Available:        model.Available,
		Routable:         model.Routable,
		UnroutableReason: model.UnroutableReason,
		Overridden:       model.Overridden,
		ListedAtMs:       model.ListedAtMs,
		UpdatedAtMs:      model.UpdatedAtMs,
	}
}

func describeModelLayers(described *Model, model catalog.Model) {
	described.ContextSource = model.FactSources.ContextWindow
	described.MaxOutputSource = model.FactSources.MaxOutput
	described.ContextLayers = ContextLayers{
		Override:  model.FactsOverride.ContextWindow,
		Provider:  model.FactsProvider.ContextWindow,
		ModelsDev: model.FactsModelsDev.ContextWindow,
	}
	described.MaxOutputLayers = ContextLayers{
		Override:  model.FactsOverride.MaxOutput,
		Provider:  model.FactsProvider.MaxOutput,
		ModelsDev: model.FactsModelsDev.MaxOutput,
	}
	described.Capabilities = Capabilities{
		Tools:     model.SupportsTools,
		Reasoning: model.SupportsReasoning,
		Vision:    model.SupportsVision,
	}
	described.CapabilityOverride = Capabilities{
		Tools:     model.FactsOverride.Tools,
		Reasoning: model.FactsOverride.Reasoning,
		Vision:    model.FactsOverride.Vision,
	}
}

func describeModelPrices(described *Model, model catalog.Model) {
	described.Prices = ModelPrices{
		Override:  toPrices(model.PricesOverride),
		Provider:  toPrices(model.PricesProvider),
		ModelsDev: toPrices(model.PricesModelsDev),
		Effective: toPrices(model.Prices),
		EffectiveSource: PriceSourceMap{
			Input:         model.PriceSources.Input,
			Output:        model.PriceSources.Output,
			CacheRead:     model.PriceSources.CacheRead,
			CacheWrite:    model.PriceSources.CacheWrite,
			ExtThreshold:  model.PriceSources.ExtThreshold,
			ExtInput:      model.PriceSources.ExtInput,
			ExtOutput:     model.PriceSources.ExtOutput,
			ExtCacheRead:  model.PriceSources.ExtCacheRead,
			ExtCacheWrite: model.PriceSources.ExtCacheWrite,
		},
	}
}

func toDetailLayer(f catalog.Facts) DetailLayer {
	return DetailLayer{
		Name: f.Name, Description: f.Description, Family: f.Family, Category: f.Category,
		ContextWindow: f.ContextWindow, MaxInput: f.MaxInput, MaxOutput: f.MaxOutput,
		Tools: f.Tools, Reasoning: f.Reasoning, Vision: f.Vision,
		Status: f.Status, ReleaseDate: f.ReleaseDate,
	}
}

func toDetailSourceMap(s catalog.FactSource) DetailSourceMap {
	return DetailSourceMap{
		Name: s.Name, Description: s.Description, Family: s.Family, Category: s.Category,
		ContextWindow: s.ContextWindow, MaxInput: s.MaxInput, MaxOutput: s.MaxOutput,
		Tools: s.Tools, Reasoning: s.Reasoning, Vision: s.Vision,
		Status: s.Status, ReleaseDate: s.ReleaseDate,
	}
}
func toPrices(p catalog.Prices) Prices {
	return Prices{
		Input:         p.Input,
		Output:        p.Output,
		CacheRead:     p.CacheRead,
		CacheWrite:    p.CacheWrite,
		ExtThreshold:  p.ExtThreshold,
		ExtInput:      p.ExtInput,
		ExtOutput:     p.ExtOutput,
		ExtCacheRead:  p.ExtCacheRead,
		ExtCacheWrite: p.ExtCacheWrite,
	}
}

func groupsOf(snapshot *catalog.Snapshot, model catalog.Model) []string {
	refs := []string{}
	for _, group := range snapshot.Groups {
		for _, member := range group.Members {
			if member.ProviderID == model.ProviderID && member.ModelID == model.ID {
				refs = append(refs, group.ID)
				break
			}
		}
	}
	return refs
}

// Groups returns all groups.
func (s *Service) Groups(ctx context.Context) ([]Group, error) {
	snapshot, err := s.snapshot()
	if err != nil {
		return nil, err
	}
	rows := make([]Group, 0, len(snapshot.Groups))
	for _, g := range snapshot.Groups {
		rows = append(rows, s.describeGroup(snapshot, g))
	}
	return rows, nil
}

// Group returns one group by id.
func (s *Service) Group(ctx context.Context, id string) (Group, error) {
	snapshot, err := s.snapshot()
	if err != nil {
		return Group{}, err
	}
	g, found := snapshot.Group(id)
	if !found {
		return Group{}, fmt.Errorf("%s: %w", id, ErrGroupNotFound)
	}
	return s.describeGroup(snapshot, g), nil
}

func (s *Service) describeGroup(snapshot *catalog.Snapshot, group catalog.Group) Group {
	row := Group{
		ID:           group.ID,
		Label:        group.Label,
		Strategy:     string(group.Strategy),
		Enabled:      group.Enabled,
		Listed:       group.Listed,
		ClientID:     catalog.ClientRouteID(group.ID),
		ShadowsModel: snapshot.ShadowsModel(group.ID),
		SwitchOn4xx:  group.SwitchOn4xx,
		SwitchOn5xx:  group.SwitchOn5xx,
		CreatedAtMs:  group.CreatedAtMs,
		UpdatedAtMs:  group.UpdatedAtMs,
	}
	row.ContextWindow, row.MaxOutput = smallestBudget(snapshot.GroupModels(group))
	row.SupportsTools, row.SupportsReasoning, row.SupportsVision = snapshot.PromisedCapabilities(group)
	row.CapabilityWarnings = snapshot.CapabilityWarnings(group)
	for _, m := range group.Members {
		row.Members = append(row.Members, describeGroupMember(snapshot, m))
	}
	return row
}

func describeGroupMember(snapshot *catalog.Snapshot, m catalog.Member) GroupMember {
	described := GroupMember{
		ProviderID: m.ProviderID,
		ModelID:    m.ModelID,
		Kind:       catalog.MemberKindOf(m.Kind),
		Weight:     m.Weight,
		Enabled:    m.Enabled,
	}
	if described.Kind == catalog.MemberKindAuto {
		describeAutoMember(snapshot, &described)
		return described
	}
	model, found := snapshot.Model(m.ProviderID, m.ModelID)
	if !found {
		described.Reason = "the model is not in the catalog"
	} else {
		described.Reason = snapshot.SkipReason(model, catalog.Requirements{})
		described.ServingAccounts, described.ActiveAccounts = snapshot.Coverage(model)
	}
	described.Eligible = found && described.Reason == ""
	return described
}

func smallestBudget(models []catalog.Model) (*int64, *int64) {
	var window, output *int64
	for _, model := range models {
		window = smallerValue(window, model.ContextWindow)
		output = smallerValue(output, model.MaxOutput)
	}
	return window, output
}

func smallerValue(current, candidate *int64) *int64 {
	if candidate == nil {
		return current
	}
	if current == nil || *candidate < *current {
		return candidate
	}
	return current
}

func describeAutoMember(snapshot *catalog.Snapshot, described *GroupMember) {
	models := snapshot.ModelsNamed(described.ModelID)
	if len(models) == 0 {
		described.Reason = "the model is not in the catalog"
		return
	}
	firstReason := ""
	for _, model := range models {
		reason := snapshot.SkipReason(model, catalog.Requirements{})
		serving, active := snapshot.Coverage(model)
		described.ServingAccounts += serving
		described.ActiveAccounts += active
		if reason == "" {
			described.Eligible = true
		} else if firstReason == "" {
			firstReason = reason
		}
	}
	if !described.Eligible {
		described.Reason = firstReason
	}
}

// Templates returns available provider templates and modelsdev status.
func (s *Service) Templates(ctx context.Context) ([]catalog.Template, catalog.ModelsDevState, error) {
	idx, err := s.modelsDev.Get(ctx, catalog.FetchOnlineFirst)
	all := s.templates.All(idx)
	st := s.modelsDev.CurrentState()
	return all, st, err
}

func (s *Service) reload(ctx context.Context) error {
	if s.catalog == nil {
		return nil
	}
	if err := s.catalog.Reload(ctx); err != nil {
		return err
	}
	if rewritten, err := s.rewriteRetiredLabels(ctx); err != nil {
		return err
	} else if rewritten {
		if err := s.catalog.Reload(ctx); err != nil {
			return err
		}
	}
	if s.afterReload != nil {
		s.afterReload(ctx)
	}
	return nil
}

func (s *Service) rewriteRetiredLabels(ctx context.Context) (bool, error) {
	if s.templates == nil {
		return false, nil
	}
	snap, err := s.snapshot()
	if err != nil {
		return false, err
	}
	rewritten := false
	for _, host := range snap.Providers {
		next, ok := s.templates.RewrittenLabel(host.TemplateID, host.Label)
		if !ok {
			continue
		}
		row, err := s.store.GetProvider(ctx, host.ID)
		if err != nil {
			return false, err
		}
		row.Label = next
		row.UpdatedAtMs = catalog.NowMs()
		if err := s.store.SaveProvider(ctx, row); err != nil {
			return false, err
		}
		rewritten = true
	}
	return rewritten, nil
}

func (s *Service) snapshot() (*catalog.Snapshot, error) {
	if s.catalog == nil {
		return nil, catalog.ErrNoCatalog
	}
	snap, found := s.catalog.Snapshot()
	if !found {
		return nil, catalog.ErrNoCatalog
	}
	return snap, nil
}

func page(rows []Model, offset, limit int) []Model {
	if offset >= len(rows) {
		return []Model{}
	}
	rows = rows[offset:]
	if limit <= 0 || limit > len(rows) {
		return rows
	}
	return rows[:limit]
}

func matchesText(id, label, search string) bool {
	needle := strings.ToLower(strings.TrimSpace(search))
	if needle == "" {
		return true
	}
	return strings.Contains(strings.ToLower(id), needle) || strings.Contains(strings.ToLower(label), needle)
}
