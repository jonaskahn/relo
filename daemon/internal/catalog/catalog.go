// Catalog core: providers, strategies, and account contracts.
package catalog

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync/atomic"
	"time"
)

// Catalog errors name the refusals callers map to their own wording: a
// catalog that never loaded and a route id that matches nothing.
var (
	ErrNoCatalog     = errors.New("the catalog is not loaded")
	ErrGroupNotFound = errors.New("group not found")
	// ErrPublicIDCollision reports two catalog entries that share one public
	// identifier after slugging. One of them would be unreachable under that
	// name, so the catalog refuses to load rather than silently dropping one. Only
	// the names entries carry can collide: a derived million-token name yields to
	// one a model of the provider already holds.
	ErrPublicIDCollision = errors.New("public identifier collision")
)

// Strategy is how a route picks among its members: by order, by weight, or
// by measured cost and speed.
type Strategy string

// Route strategies a route may choose from, which is what the console offers
// when an operator changes how a route spreads requests.
const (
	StrategyPriority   Strategy = "priority"
	StrategyRoundRobin Strategy = "round-robin"
	StrategyWeighted   Strategy = "weighted"
	StrategyCheapest   Strategy = "cheapest"
	StrategyFastest    Strategy = "fastest"
)

// Strategies returns every route strategy this build offers, so a console
// never offers one routing cannot run.
func Strategies() []Strategy {
	return []Strategy{StrategyPriority, StrategyRoundRobin, StrategyWeighted, StrategyCheapest, StrategyFastest}
}

// Accounts is the live credential state routing reads: which providers have
// usable accounts and which models each one can serve right now.
type Accounts interface {
	Active(providerID string) int
	Available(providerID string) bool
	// ServesModel reports whether an account that could take a request right
	// now is known to serve the model, named by any of its identifiers. It is
	// read live rather than stored in a snapshot, so an account-level refusal
	// or a fresh listing is honoured by the next request.
	ServesModel(providerID string, model Model) bool
	// Coverage counts the accounts that could take a request now, and how many
	// of them can serve the model.
	Coverage(providerID string, model Model) (int, int)
	// CooldownUntil is when every active account of a provider may take a
	// request again. A zero time means the provider is not cooling down.
	CooldownUntil(providerID string) time.Time
}

// Provider is one connection the catalog resolved: its settings, roster,
// and whether routing may use it.
type Provider struct {
	ID                  string
	TemplateID          string
	Label               string
	Origin              Origin
	Auth                Auth
	APIFormat           APIFormat
	KeyHeader           KeyHeader
	BaseURL             string
	ModelsSource        string
	ModelsFormat        ModelsFormat
	ModelsDevProviderID string
	Headers             map[string]string
	DocURL              string
	KeyEnv              []string
	LoginFlows          []string
	Variables           map[string]string
	NeedsSetup          []string
	Enabled             bool
	Rank                int
	PoolStrategy        string
	LastRefreshedAtMs   *int64
	LastRefreshError    string
	Routable            bool
	UnroutableReason    string
	Configured          bool
	CreatedAtMs         int64
	UpdatedAtMs         int64
	// UseProxy sends this connection through the proxy configured in Settings.
	UseProxy bool
	// TimeoutSeconds is this connection's own call wait in seconds; nil
	// inherits the global value.
	TimeoutSeconds *int
	// RetryBackoff is this connection's own retry windows; nil inherits the
	// global windows.
	RetryBackoff [][2]int
	// SwitchOn4xx and SwitchOn5xx fail this connection over on an unlisted
	// status of that class. An unset option reads as true: Relo switches.
	SwitchOn4xx bool
	SwitchOn5xx bool
}

// PriceSource names where each resolved rate came from: the operator, the
// provider, or the model index.
type PriceSource struct {
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

// Facts is one layer of model details exactly as it is stored, so a surface
// can show which layer a value came from. A nil field is a layer that is
// silent about that value rather than one that clears it.
type Facts struct {
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

// FactSource names the layer each resolved detail came from, the way
// PriceSource does for rates.
type FactSource struct {
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

// Model is one served model with its resolved facts: identity, windows,
// capabilities, rates, and whether routing may use it.
type Model struct {
	ProviderID string
	ID         string
	// UpstreamID is the identifier Relo sends to the provider. It is the
	// model's own id unless the model is a clone, which points at another
	// identifier the provider knows.
	UpstreamID string
	// ClonedFrom names the model a clone was copied from, empty otherwise.
	ClonedFrom        string
	Source            string
	Name              string
	Description       string
	Family            string
	Category          Category
	APIFormat         APIFormat
	BaseURL           string
	ModelsDevRef      string
	Match             string
	ContextWindow     *int64
	MaxInput          *int64
	MaxOutput         *int64
	SupportsTools     *bool
	SupportsReasoning *bool
	SupportsVision    *bool
	// ReasoningEfforts are the effort values the model accepts, resolved from
	// the layers in the same order as every other fact. A nil result means no
	// layer stated a ladder, which is not the same as a model that states an
	// empty one: an unstated ladder leaves the current pass-through alone,
	// while a model that declares only a toggle gets no effort field at all.
	ReasoningEfforts []string
	// ReasoningToggle reports that the resolved model can turn thinking on
	// or off without an effort name.
	ReasoningToggle bool
	// ReasoningBudget reports that the resolved model takes a token budget.
	// The bounds are nil when the model stated a budget without a limit.
	ReasoningBudget    bool
	ReasoningBudgetMin *int64
	ReasoningBudgetMax *int64
	Status             string
	ReleaseDate        string
	Prices             Prices
	PricesOverride     Prices
	PricesProvider     Prices
	PricesModelsDev    Prices
	PriceSources       PriceSource
	FactsOverride      Facts
	FactsProvider      Facts
	FactsModelsDev     Facts
	FactSources        FactSource
	// Overridden reports whether an operator changed anything about the
	// model, which is what the console lists under its overrides.
	Overridden       bool
	Enabled          bool
	Available        *bool
	ListedAtMs       *int64
	UpdatedAtMs      int64
	Routable         bool
	UnroutableReason string
}

// Member is one model inside a route: which connection serves it, how much
// traffic it takes, and whether it is switched on.
type Member struct {
	ProviderID string
	ModelID    string
	// Kind is how the member names its model: MemberKindModel for one
	// connection's own row, MemberKindAuto for a bare identifier the router
	// resolves against every connection that serves it.
	Kind    string
	Weight  int
	Enabled bool
}

// The two ways a route member names a model.
const (
	// MemberKindModel names one connection's model, which is what every
	// member stored before a route could name a bare identifier is.
	MemberKindModel = "model"
	// MemberKindAuto names a model identifier without a connection, so the
	// route follows whichever connection serves it.
	MemberKindAuto = "auto"
)

// MemberKindOf reports the kind of one member, defaulting to the
// connection-qualified form a row without one means.
func MemberKindOf(kind string) string {
	if kind == MemberKindAuto {
		return MemberKindAuto
	}
	return MemberKindModel
}

// Group is one named route: its members, strategy, and failover choices.
type Group struct {
	ID          string
	Label       string
	Strategy    Strategy
	Enabled     bool
	Listed      bool
	Members     []Member
	CreatedAtMs int64
	UpdatedAtMs int64
	// SwitchOn4xx and SwitchOn5xx move this route to its next member on an
	// unlisted status of that class. An unset option reads as true.
	SwitchOn4xx bool
	SwitchOn5xx bool
}

type key struct {
	providerID string
	modelID    string
}

// Snapshot is the resolved catalog routing reads without touching storage:
// connections, models, and routes with the indexes that answer lookups.
type Snapshot struct {
	Providers   []Provider
	Groups      []Group
	accounts    Accounts
	byProvider  map[string]Provider
	byModelKey  map[key]Model
	byModelID   map[string][]Model
	byProviderM map[string][]Model
	byGroupID   map[string]Group
	// byClientID maps the identifier a coding agent asks for to the entry it
	// names, which is how a hyphenated public name resolves without being
	// split back into a provider and a model.
	byClientID map[string]ClientTarget
	// byDesktopAlias maps the opaque id Claude Desktop lists to the entry it
	// names.
	byDesktopAlias map[string]ClientTarget
	// paired holds the base public identifier of every entry that was published
	// beside a separate million-token one, so the listing reports the same set of
	// names the catalog can actually resolve.
	paired map[string]bool
	// suffixed holds the base public identifier of every entry that was
	// published once, under the generated 1M suffix alone. A derived suffix
	// that collides with a name a model of the provider already holds is not
	// published, so the listing reports the same set of names the catalog
	// can actually resolve.
	suffixed map[string]bool
}

// ClientTarget names what one public identifier resolves to: a route, or one
// connection's model. Exactly one of the two is set.
type ClientTarget struct {
	GroupID        string
	ProviderID     string
	ModelID        string
	MillionContext bool
	// NativeMillion reports that the named entry is a million tokens on its own
	// rather than the long-context half of a pair, so a request naming it is
	// already at the full window whatever spelling arrived.
	NativeMillion bool
}

// ResolveClientID returns what one public identifier names, and whether it
// names anything at all. The name is matched exactly, after the million-token
// marker and the Claude Code picker prefix are taken off.
func (s *Snapshot) ResolveClientID(name string) (ClientTarget, bool) {
	if target, found := s.byDesktopAlias[strings.TrimSpace(name)]; found {
		return target, true
	}
	if providerID, modelID, ok := AnthropicModelTarget(name); ok {
		if model, found := s.Model(providerID, modelID); found {
			// A model that is a million-token entry on its own is published under
			// its bare name only, so the bare spelling and a marked one reach the
			// same window. A marked spelling of any other million-token model
			// selects the full window, whether it was published beside a base
			// entry or under the suffix alone.
			alone := s.MillionAlone(model)
			marked := HasMillionContextSuffix(name) &&
				(s.paired[ClientModelID(providerID, modelID)] || s.MillionSuffixed(model))
			return ClientTarget{
				ProviderID: providerID, ModelID: modelID,
				MillionContext: alone || marked,
				NativeMillion:  alone || (marked && s.MillionSuffixed(model)),
			}, true
		}
	}
	publicID := StripClaudeAlias(StripContextSuffix(name))
	if HasMillionContextSuffix(name) {
		if target, found := s.byClientID[MillionAlias(publicID)]; found {
			return target, true
		}
	}
	target, found := s.byClientID[publicID]
	return target, found
}

// Formats reports which upstream wire formats this build implements, so a
// connection or model whose format has no codec is never routable.
type Formats interface {
	Supports(format APIFormat) bool
}

// Catalog keeps the current resolved snapshot, rebuilt whenever stored
// rows change so readers never block on storage.
type Catalog struct {
	reader   Reader
	accounts Accounts
	formats  Formats
	current  atomic.Pointer[Snapshot]
}

// New wires a catalog over its row reader and live account state, empty
// until the first reload.
func New(reader Reader, accounts Accounts, formats Formats) *Catalog {
	return &Catalog{reader: reader, accounts: accounts, formats: formats}
}

// Reload rebuilds the resolved snapshot from stored rows, so routing and
// the console read what the operator just changed.
func (c *Catalog) Reload(ctx context.Context) error {
	snapshot, err := c.build(ctx)
	if err != nil {
		return err
	}
	c.current.Store(snapshot)
	return nil
}

// Snapshot returns the current resolved catalog, or false before the first
// reload.
func (c *Catalog) Snapshot() (*Snapshot, bool) {
	current := c.current.Load()
	return current, current != nil
}

func (c *Catalog) build(ctx context.Context) (*Snapshot, error) {
	providers, err := c.reader.ListConnections(ctx)
	if err != nil {
		return nil, fmt.Errorf("read providers: %w", err)
	}
	models, err := c.reader.ListModels(ctx)
	if err != nil {
		return nil, fmt.Errorf("read models: %w", err)
	}
	facts, err := c.reader.ListModelFacts(ctx)
	if err != nil {
		return nil, fmt.Errorf("read model facts: %w", err)
	}
	groups, err := c.reader.ListRoutes(ctx)
	if err != nil {
		return nil, fmt.Errorf("read groups: %w", err)
	}
	return newSnapshot(snapshotInputs{providers: providers, models: models, facts: facts, groups: groups, accounts: c.accounts, formats: c.formats})
}

func indexSnapshotProviders(snapshot *Snapshot, providers []ConnectionRecord, accounts Accounts, formats Formats) {
	for _, row := range providers {
		host := providerFrom(row, formats)
		host.Configured = configured(host, accounts)
		snapshot.byProvider[host.ID] = host
	}
}

func indexSnapshotModels(snapshot *Snapshot, models []ModelRecord, facts []FactsRecord, formats Formats) {
	factsIndex := make(map[key]map[string]FactsRecord, len(models))
	for _, f := range facts {
		k := key{providerID: f.ProviderID, modelID: f.ModelID}
		if factsIndex[k] == nil {
			factsIndex[k] = make(map[string]FactsRecord)
		}
		factsIndex[k][f.Layer] = f
	}

	for _, row := range models {
		host, found := snapshot.byProvider[row.ProviderID]
		if !found {
			continue
		}
		k := key{providerID: row.ProviderID, modelID: row.ModelID}
		model := resolveModel(row, factsIndex[k], host, formats)
		snapshot.byModelKey[k] = model
		snapshot.byModelID[model.ID] = append(snapshot.byModelID[model.ID], model)
		snapshot.byProviderM[model.ProviderID] = append(snapshot.byProviderM[model.ProviderID], model)
	}
}

type snapshotInputs struct {
	providers []ConnectionRecord
	models    []ModelRecord
	facts     []FactsRecord
	groups    []RouteRecord
	accounts  Accounts
	formats   Formats
}

func newSnapshot(in snapshotInputs) (*Snapshot, error) {
	providers, models, facts, groups, accounts, formats := in.providers, in.models, in.facts, in.groups, in.accounts, in.formats
	snapshot, owner := emptySnapshot(providers, models, groups, accounts)

	indexSnapshotProviders(snapshot, providers, accounts, formats)

	indexSnapshotModels(snapshot, models, facts, formats)

	snapshot.Providers = sortedProviders(snapshot.byProvider)
	snapshot.Groups = sortedGroups(groups)
	for _, group := range snapshot.Groups {
		snapshot.byGroupID[group.ID] = group
	}
	// A public identifier names exactly one entry: a collision refuses the
	// load naming both sides, and a derived million-token name yields to
	// the real name it shadows.
	if err := claimRouteIdentifiers(snapshot, owner); err != nil {
		return nil, err
	}
	if err := claimModelIdentifiers(snapshot, owner); err != nil {
		return nil, err
	}
	claimMillionRouteAliases(snapshot, owner)
	claimMillionModelAliases(snapshot, owner)
	return snapshot, nil
}

func emptySnapshot(providers []ConnectionRecord, models []ModelRecord, groups []RouteRecord, accounts Accounts) (*Snapshot, map[string]string) {
	snapshot := &Snapshot{
		accounts:    accounts,
		byProvider:  make(map[string]Provider, len(providers)),
		byModelKey:  make(map[key]Model, len(models)),
		byModelID:   make(map[string][]Model),
		byProviderM: make(map[string][]Model),
		byGroupID:   make(map[string]Group, len(groups)),
		byClientID:  make(map[string]ClientTarget, len(models)+len(groups)),

		byDesktopAlias: make(map[string]ClientTarget, len(models)+len(groups)),
		paired:         make(map[string]bool),
		suffixed:       make(map[string]bool),
	}
	// owner names, for each public identifier, the catalog entry that claims
	// it, so a collision reports both sides rather than only the loser.
	owner := make(map[string]string, len(models)+len(groups))
	return snapshot, owner
}

func claimRouteIdentifiers(snapshot *Snapshot, owner map[string]string) error {
	for _, group := range snapshot.Groups {
		target := ClientTarget{GroupID: group.ID}
		id := ClientRouteID(group.ID)
		if snapshot.NativeMillionGroup(group) {
			// A route whose members are all million-token entries on their own is
			// one entry at that window. It still answers to the marked spellings a
			// client may send for it.
			target.MillionContext, target.NativeMillion = true, true
		}
		if err := snapshot.claim(owner, id, target, "route "+group.ID); err != nil {
			return err
		}
		snapshot.claimDesktopAlias(AnthropicClientID(id, nil), target)
	}
	return nil
}

func claimModelIdentifiers(snapshot *Snapshot, owner map[string]string) error {
	for _, host := range snapshot.Providers {
		for _, model := range snapshot.byProviderM[host.ID] {
			target := ClientTarget{ProviderID: model.ProviderID, ModelID: model.ID}
			if snapshot.MillionAlone(model) {
				// The window is the model's own, so it is published under the bare
				// name alone. A marked spelling is still accepted, so a client or an
				// operator that sends one lands on this entry rather than nowhere.
				target.MillionContext, target.NativeMillion = true, true
			}
			if err := snapshot.claim(owner, ClientModelID(model.ProviderID, model.ID), target,
				"model "+model.ProviderID+"/"+model.ID); err != nil {
				return err
			}
			snapshot.claimDesktopAlias(AnthropicModelAlias(model.ProviderID, model.ID, nil), target)
		}
	}
	return nil
}

func claimMillionRouteAliases(snapshot *Snapshot, owner map[string]string) {
	millionWindow := int64(MillionContext)
	for _, group := range snapshot.Groups {
		id := ClientRouteID(group.ID)
		target, found := snapshot.byClientID[id]
		if !found {
			continue
		}
		alias := MillionAlias(id)
		if _, taken := snapshot.byClientID[alias]; taken {
			continue
		}
		if target.NativeMillion {
			// The route is the million-token entry already, so the derived name
			// resolves to it and no second entry is published.
			snapshot.byClientID[alias] = target
			owner[alias] = "1M route " + group.ID
			continue
		}
		if snapshot.SuffixedMillionGroup(group) {
			claimSuffixedRoute(snapshot, owner, group, target)
			continue
		}
		if !snapshot.pairsWithOneMillionGroup(group) {
			continue
		}
		target.MillionContext = true
		snapshot.byClientID[alias] = target
		owner[alias] = "1M route " + group.ID
		snapshot.paired[id] = true
		snapshot.claimDesktopAlias(AnthropicClientID(id, &millionWindow), target)
	}
}

func claimSuffixedRoute(snapshot *Snapshot, owner map[string]string, group Group, target ClientTarget) {
	// The route is published once, under the suffix alone, at the
	// members' own window.
	target.MillionContext, target.NativeMillion = true, true
	id := ClientRouteID(group.ID)
	alias := MillionAlias(id)
	snapshot.byClientID[alias] = target
	owner[alias] = "1M route " + group.ID
	snapshot.suffixed[id] = true
	millionWindow := int64(MillionContext)
	snapshot.claimDesktopAlias(AnthropicClientID(id, &millionWindow), target)
}

func claimMillionModelAliases(snapshot *Snapshot, owner map[string]string) {
	millionWindow := int64(MillionContext)
	for _, host := range snapshot.Providers {
		for _, model := range snapshot.byProviderM[host.ID] {
			claimModelMillionAlias(snapshot, owner, model, millionWindow)
		}
	}
}

func claimModelMillionAlias(snapshot *Snapshot, owner map[string]string, model Model, millionWindow int64) {
	id := ClientModelID(model.ProviderID, model.ID)
	target, found := snapshot.byClientID[id]
	if !found {
		return
	}
	alias := MillionAlias(id)
	if _, taken := snapshot.byClientID[alias]; taken {
		return
	}
	if target.NativeMillion {
		// A marked spelling of this name still lands on it, so a client
		// that sends one is not left with a model it cannot find.
		snapshot.byClientID[alias] = target
		owner[alias] = "1M model " + model.ProviderID + "/" + model.ID
		return
	}
	if snapshot.MillionSuffixed(model) {
		// The model is published once, under the suffix alone, so the
		// suffixed name is its entry rather than a twin beside a base one.
		// No base-rate cap applies, and no long-context beta is claimed.
		target.MillionContext, target.NativeMillion = true, true
		snapshot.byClientID[alias] = target
		owner[alias] = "1M model " + model.ProviderID + "/" + model.ID
		snapshot.suffixed[id] = true
		snapshot.claimDesktopAlias(AnthropicModelAlias(model.ProviderID, model.ID, &millionWindow), target)
		return
	}
	if !snapshot.pairsWithOneMillion(model) {
		return
	}
	target.MillionContext = true
	snapshot.byClientID[alias] = target
	owner[alias] = "1M model " + model.ProviderID + "/" + model.ID
	snapshot.paired[id] = true
	snapshot.claimDesktopAlias(AnthropicModelAlias(model.ProviderID, model.ID, &millionWindow), target)
}

func (s *Snapshot) claimDesktopAlias(pickerID string, target ClientTarget) {
	alias := DesktopModelAlias(pickerID)
	if _, taken := s.byDesktopAlias[alias]; !taken {
		s.byDesktopAlias[alias] = target
	}
}

func (s *Snapshot) claim(owner map[string]string, id string, target ClientTarget, description string) error {
	if existing, taken := owner[id]; taken {
		return fmt.Errorf("%s and %s: %w", existing, description, ErrPublicIDCollision)
	}
	owner[id] = description
	s.byClientID[id] = target
	return nil
}

func configured(host Provider, accounts Accounts) bool {
	if !host.Routable || !host.Enabled {
		return false
	}
	if host.Auth == AuthNone {
		return true
	}
	return accounts != nil && accounts.Active(host.ID) > 0
}

func providerFrom(row ConnectionRecord, formats Formats) Provider {
	host := providerIdentity(row)
	finishProviderFrom(&host, row, formats)
	return host
}

func providerIdentity(row ConnectionRecord) Provider {
	return Provider{
		ID:                  row.ID,
		TemplateID:          row.TemplateID,
		Label:               row.Label,
		Origin:              Origin(row.Origin),
		Auth:                Auth(row.Auth),
		APIFormat:           APIFormat(row.APIFormat),
		KeyHeader:           KeyHeader(row.KeyHeader),
		ModelsSource:        row.ModelsSource,
		ModelsFormat:        ModelsFormat(row.ModelsFormat),
		ModelsDevProviderID: row.ModelsDevProviderID,
		Headers:             row.Headers,
		DocURL:              row.DocURL,
		KeyEnv:              row.KeyEnv,
		LoginFlows:          row.LoginFlows,
		Enabled:             row.Enabled,
		Rank:                row.Rank,
		PoolStrategy:        row.PoolStrategy,
		LastRefreshedAtMs:   row.LastRefreshedAtMs,
		LastRefreshError:    row.LastRefreshError,
		Variables:           row.Variables,
	}
}

func finishProviderFrom(host *Provider, row ConnectionRecord, formats Formats) {
	host.CreatedAtMs = row.CreatedAtMs
	host.UpdatedAtMs = row.UpdatedAtMs
	host.UseProxy = row.UseProxy
	host.TimeoutSeconds = row.TimeoutSeconds
	host.RetryBackoff = row.RetryBackoff
	host.SwitchOn4xx = switchOn(row.SwitchOn4xx)
	host.SwitchOn5xx = switchOn(row.SwitchOn5xx)
	host.BaseURL, host.NeedsSetup = substitute(row.BaseURL, row.Variables)
	host.Routable = routableProvider(*host, formats)
	host.UnroutableReason = unroutableReason(*host)
}

func resolveModel(m ModelRecord, facts map[string]FactsRecord, host Provider, formats Formats) Model {
	ov := facts["override"]
	pr := facts["provider"]
	md := facts["modelsdev"]

	res := resolvedIdentity(m, host, ov, pr, md)
	resolveModelWindows(&res, ov, pr, md)
	resolveModelCapabilities(&res, host, ov, pr, md)
	resolveModelPricing(&res, ov, pr, md)
	finishResolvedModel(&res, m, host, formats, factLayers{override: ov, provider: pr, models: md})
	return res
}

func resolvedIdentity(m ModelRecord, host Provider, ov, pr, md FactsRecord) Model {
	apiFmt, baseURL := resolveModelEndpoint(m, host)
	return Model{
		ProviderID:   host.ID,
		ID:           m.ModelID,
		UpstreamID:   upstreamIDOf(m),
		ClonedFrom:   m.ClonedFrom,
		Source:       m.Source,
		Name:         pickString(ov.Name, pr.Name, md.Name, m.ModelID),
		Description:  pickString(ov.Description, pr.Description, md.Description, ""),
		Family:       pickString(ov.Family, pr.Family, md.Family, ""),
		Category:     Category(pickString(ov.Category, pr.Category, md.Category, string(CategoryChat))),
		APIFormat:    apiFmt,
		BaseURL:      baseURL,
		ModelsDevRef: m.ModelsDevRef,
		Match:        m.Match,
		Status:       pickString(ov.Status, pr.Status, md.Status, "active"),
		ReleaseDate:  pickString(ov.ReleaseDate, pr.ReleaseDate, md.ReleaseDate, ""),
		Enabled:      m.Enabled,
		Available:    m.Available,
		ListedAtMs:   m.ListedAtMs,
		UpdatedAtMs:  m.UpdatedAtMs,
	}
}

func resolveModelEndpoint(m ModelRecord, host Provider) (APIFormat, string) {
	apiFmt := host.APIFormat
	if m.APIFormat != "" {
		apiFmt = APIFormat(m.APIFormat)
	}
	baseURL := host.BaseURL
	if m.BaseURL != "" {
		baseURL, _ = substitute(m.BaseURL, host.Variables)
	}
	return apiFmt, baseURL
}

func resolveModelWindows(res *Model, ov, pr, md FactsRecord) {
	res.ContextWindow = RoundContextWindow(pickInt64(ov.ContextWindow, pr.ContextWindow, md.ContextWindow))
	res.MaxInput = RoundContextWindow(pickInt64(ov.MaxInput, pr.MaxInput, md.MaxInput))
	res.MaxOutput = pickInt64(ov.MaxOutput, pr.MaxOutput, md.MaxOutput)
}

func resolveModelCapabilities(res *Model, host Provider, ov, pr, md FactsRecord) {
	res.SupportsTools = pickBool(ov.SupportsTools, pr.SupportsTools, md.SupportsTools)
	res.SupportsReasoning = pickBool(ov.SupportsReasoning, pr.SupportsReasoning, md.SupportsReasoning)
	res.SupportsVision = pickBool(ov.SupportsVision, pr.SupportsVision, md.SupportsVision)
	res.ReasoningEfforts = xiaomiEfforts(host, res.SupportsReasoning, pickEfforts(ov.ReasoningEfforts, pr.ReasoningEfforts, md.ReasoningEfforts))
	res.ReasoningToggle = boolValue(pickBool(ov.ReasoningToggle, pr.ReasoningToggle, md.ReasoningToggle))
	res.ReasoningBudget = boolValue(pickBool(ov.ReasoningBudget, pr.ReasoningBudget, md.ReasoningBudget))
	res.ReasoningBudgetMin = pickInt64(ov.ReasoningBudgetMin, pr.ReasoningBudgetMin, md.ReasoningBudgetMin)
	res.ReasoningBudgetMax = pickInt64(ov.ReasoningBudgetMax, pr.ReasoningBudgetMax, md.ReasoningBudgetMax)
}

func resolveModelPricing(res *Model, ov, pr, md FactsRecord) {
	effPrices, pSources := resolvePrices(ov.Prices, pr.Prices, md.Prices)
	res.Prices = effPrices
	res.PricesOverride = toProviderPrices(ov.Prices)
	res.PricesProvider = toProviderPrices(pr.Prices)
	res.PricesModelsDev = toProviderPrices(md.Prices)
	res.PriceSources = pSources
}

type factLayers struct {
	override FactsRecord
	provider FactsRecord
	models   FactsRecord
}

func finishResolvedModel(res *Model, m ModelRecord, host Provider, formats Formats, layers factLayers) {
	ov, pr, md := layers.override, layers.provider, layers.models
	res.Routable = routableModel(*res, host, formats)
	res.UnroutableReason = unroutableModelReason(*res, host)
	res.FactsOverride = factsOf(ov)
	res.FactsProvider = factsOf(pr)
	res.FactsModelsDev = factsOf(md)
	res.FactSources = factSources(ov, pr, md)
	res.Overridden = overridden(ov, m)
}

func factsOf(f FactsRecord) Facts {
	return Facts{
		Name: f.Name, Description: f.Description, Family: f.Family, Category: f.Category,
		ContextWindow: f.ContextWindow, MaxInput: f.MaxInput, MaxOutput: f.MaxOutput,
		Tools: f.SupportsTools, Reasoning: f.SupportsReasoning, Vision: f.SupportsVision,
		Status: f.Status, ReleaseDate: f.ReleaseDate,
	}
}

func factSources(ov, pr, md FactsRecord) FactSource {
	return FactSource{
		Name:          pickSource(ov.Name, pr.Name, md.Name),
		Description:   pickSource(ov.Description, pr.Description, md.Description),
		Family:        pickSource(ov.Family, pr.Family, md.Family),
		Category:      pickSource(ov.Category, pr.Category, md.Category),
		ContextWindow: pickSource(ov.ContextWindow, pr.ContextWindow, md.ContextWindow),
		MaxInput:      pickSource(ov.MaxInput, pr.MaxInput, md.MaxInput),
		MaxOutput:     pickSource(ov.MaxOutput, pr.MaxOutput, md.MaxOutput),
		Tools:         pickSource(ov.SupportsTools, pr.SupportsTools, md.SupportsTools),
		Reasoning:     pickSource(ov.SupportsReasoning, pr.SupportsReasoning, md.SupportsReasoning),
		Vision:        pickSource(ov.SupportsVision, pr.SupportsVision, md.SupportsVision),
		Status:        pickSource(ov.Status, pr.Status, md.Status),
		ReleaseDate:   pickSource(ov.ReleaseDate, pr.ReleaseDate, md.ReleaseDate),
	}
}

func pickSource[T any](ov, pr, md *T) string {
	if ov != nil {
		return "override"
	}
	if pr != nil {
		return "provider"
	}
	if md != nil {
		return "modelsdev"
	}
	return ""
}

var xiaomiThinkLevels = []string{"low", "medium", "high"}

func xiaomiEfforts(host Provider, reasoning *bool, efforts []string) []string {
	if reasoning == nil || !*reasoning || !xiaomiConnection(host) {
		return efforts
	}
	return append([]string(nil), xiaomiThinkLevels...)
}

func xiaomiConnection(host Provider) bool {
	for _, id := range []string{host.ID, host.TemplateID, host.ModelsDevProviderID} {
		if xiaomiProviderID(id) {
			return true
		}
	}
	return strings.Contains(strings.ToLower(host.BaseURL), "xiaomimimo.com")
}

func xiaomiProviderID(id string) bool {
	id = strings.ToLower(strings.TrimSpace(id))
	return id == "xiaomi" || strings.HasPrefix(id, "xiaomi-token-plan")
}

func pickEfforts(ov, pr, md []string) []string {
	for _, values := range [][]string{ov, pr, md} {
		if values != nil {
			return values
		}
	}
	return nil
}

func overridden(ov FactsRecord, m ModelRecord) bool {
	if m.Match == "manual" {
		return true
	}
	return ov.Name != nil || ov.Description != nil || ov.Family != nil || ov.Category != nil ||
		ov.ContextWindow != nil || ov.MaxInput != nil || ov.MaxOutput != nil ||
		ov.SupportsTools != nil || ov.SupportsReasoning != nil || ov.SupportsVision != nil ||
		ov.Status != nil || ov.ReleaseDate != nil || ov.Prices.Known()
}

func upstreamIDOf(m ModelRecord) string {
	if upstream := strings.TrimSpace(m.UpstreamModelID); upstream != "" {
		return upstream
	}
	return m.ModelID
}
func resolvePrices(ov, pr, md PriceRecord) (Prices, PriceSource) {
	var eff Prices
	var src PriceSource

	eff.Input, src.Input = pickPriceField(ov.Input, pr.Input, md.Input)
	eff.Output, src.Output = pickPriceField(ov.Output, pr.Output, md.Output)
	eff.CacheRead, src.CacheRead = pickPriceField(ov.CacheRead, pr.CacheRead, md.CacheRead)
	eff.CacheWrite, src.CacheWrite = pickPriceField(ov.CacheWrite, pr.CacheWrite, md.CacheWrite)
	eff.ExtThreshold, src.ExtThreshold = pickPriceField(ov.ExtThreshold, pr.ExtThreshold, md.ExtThreshold)
	eff.ExtInput, src.ExtInput = pickPriceField(ov.ExtInput, pr.ExtInput, md.ExtInput)
	eff.ExtOutput, src.ExtOutput = pickPriceField(ov.ExtOutput, pr.ExtOutput, md.ExtOutput)
	eff.ExtCacheRead, src.ExtCacheRead = pickPriceField(ov.ExtCacheRead, pr.ExtCacheRead, md.ExtCacheRead)
	eff.ExtCacheWrite, src.ExtCacheWrite = pickPriceField(ov.ExtCacheWrite, pr.ExtCacheWrite, md.ExtCacheWrite)

	return eff, src
}

func pickPriceField(ov, pr, md *int64) (*int64, string) {
	if ov != nil {
		return ov, "override"
	}
	if pr != nil {
		return pr, "provider"
	}
	if md != nil {
		return md, "modelsdev"
	}
	return nil, ""
}

func toProviderPrices(p PriceRecord) Prices {
	return Prices(p)
}

func pickString(vals ...any) string {
	for _, v := range vals {
		if sPtr, ok := v.(*string); ok && sPtr != nil && strings.TrimSpace(*sPtr) != "" {
			return *sPtr
		}
		if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

func boolValue(value *bool) bool {
	return value != nil && *value
}

func switchOn(value *bool) bool {
	return value == nil || *value
}

func pickInt64(vals ...*int64) *int64 {
	for _, v := range vals {
		if v != nil {
			return v
		}
	}
	return nil
}

func pickBool(vals ...*bool) *bool {
	for _, v := range vals {
		if v != nil {
			return v
		}
	}
	return nil
}

func sortedProviders(index map[string]Provider) []Provider {
	rows := make([]Provider, 0, len(index))
	for _, host := range index {
		rows = append(rows, host)
	}
	sort.Slice(rows, func(i, j int) bool { return providerLess(rows[i], rows[j]) })
	return rows
}

func providerLess(left, right Provider) bool {
	if left.Rank != right.Rank {
		return left.Rank < right.Rank
	}
	leftAuth, rightAuth := authOrder(left.Auth), authOrder(right.Auth)
	if leftAuth != rightAuth {
		return leftAuth < rightAuth
	}
	return left.ID < right.ID
}

func authOrder(auth Auth) int {
	switch auth {
	case AuthOAuth:
		return 0
	case AuthNone:
		return 1
	default:
		return 2
	}
}

func sortedGroups(rows []RouteRecord) []Group {
	groups := make([]Group, 0, len(rows))
	for _, row := range rows {
		group := Group{
			ID:          row.ID,
			Label:       row.Label,
			Strategy:    Strategy(row.Strategy),
			Enabled:     row.Enabled,
			Listed:      row.Listed,
			CreatedAtMs: row.CreatedAtMs,
			UpdatedAtMs: row.UpdatedAtMs,
			SwitchOn4xx: switchOn(row.SwitchOn4xx),
			SwitchOn5xx: switchOn(row.SwitchOn5xx),
		}
		for _, member := range row.Members {
			group.Members = append(group.Members, Member{
				ProviderID: member.ProviderID,
				ModelID:    member.ModelID,
				Kind:       MemberKindOf(member.Kind),
				Weight:     member.Weight,
				Enabled:    member.Enabled,
			})
		}
		groups = append(groups, group)
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].ID < groups[j].ID })
	return groups
}

const (
	variableOpen  = "${"
	variableClose = "}"
)

func substitute(raw string, variables map[string]string) (string, []string) {
	if !strings.Contains(raw, variableOpen) {
		return raw, nil
	}
	missing := []string{}
	resolved := raw
	for {
		start := strings.Index(resolved, variableOpen)
		if start < 0 {
			break
		}
		rest := resolved[start:]
		end := strings.Index(rest, variableClose)
		if end < 0 {
			break
		}
		name := rest[len(variableOpen):end]
		value := variables[name]
		if value == "" {
			missing = append(missing, name)
			break
		}
		resolved = resolved[:start] + value + rest[end+1:]
	}
	sort.Strings(missing)
	return resolved, missing
}

func routableProvider(host Provider, formats Formats) bool {
	if host.APIFormat == FormatUnsupported {
		return false
	}
	if !formats.Supports(host.APIFormat) {
		return false
	}
	if strings.Contains(host.BaseURL, variableOpen) {
		return false
	}
	return isHTTPURL(host.BaseURL)
}

func unroutableReason(host Provider) string {
	switch {
	case host.APIFormat == FormatUnsupported:
		return "no API format is declared"
	case strings.Contains(host.BaseURL, variableOpen):
		return "the base URL still needs its variables"
	case !isHTTPURL(host.BaseURL):
		return "the provider has no base URL"
	default:
		return ""
	}
}

func routableModel(model Model, host Provider, formats Formats) bool {
	if !host.Routable {
		return false
	}
	if !formats.Supports(model.APIFormat) {
		return false
	}
	return isHTTPURL(model.BaseURL)
}

func unroutableModelReason(model Model, host Provider) string {
	switch {
	case !host.Routable:
		return host.UnroutableReason
	case !isHTTPURL(model.BaseURL):
		return "the endpoint has no base URL"
	default:
		return ""
	}
}

func isHTTPURL(raw string) bool {
	return strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://")
}
