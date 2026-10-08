// Package router turns one inbound request into the ordered providers and
// models that may serve it, following a fixed precedence so a caller can
// predict where a request lands.
package routing

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/jonaskahn/relo/internal/account"
	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/internal/clock"
)

var (
	// ErrModelRequired reports a request that named no model.
	ErrModelRequired = errors.New("a model is required")
	// ErrModelNotFound reports a model no configured provider serves.
	ErrModelNotFound = errors.New("model not found")
	// ErrModelUnconfigured reports a model the catalog knows but no account
	// can reach, which tells an operator what to link rather than that the
	// identifier was wrong.
	ErrModelUnconfigured = errors.New("model is not configured")
	// ErrNoRoute reports a request whose every candidate was ineligible.
	ErrNoRoute = errors.New("no provider can serve this request")
	// ErrNoCatalog reports a router built without the catalog.
	ErrNoCatalog = errors.New("the catalog is not loaded")
)

// Kind names how a request resolved, which is what a log records.
type Kind string

const (
	// KindGroup reports a name an operator defined as a group.
	KindGroup Kind = "group"
	// KindQualified reports a request that named its provider.
	KindQualified Kind = "qualified"
	// KindBare reports a model several providers serve, tried in order.
	KindBare Kind = "bare"
	// KindPassthrough reports a model a configured provider serves without a
	// catalog row of its own.
	KindPassthrough Kind = "passthrough"
)

// RouteReason names the route kind in a usage row, which keeps the stored
// value stable and free of identifiers.
func (k Kind) RouteReason() string {
	return string(k)
}

// Request is what routing needs to know about one inbound request.
type Request struct {
	// Model is the identifier the caller asked for.
	Model string
	// Conversation is the identifier a conversation keeps its provider on,
	// empty when the caller sent none.
	Conversation string
	// Needs is what the request requires from a model, so a candidate that
	// cannot serve it is skipped before anything is sent.
	Needs catalog.Requirements
}

// Candidate is one provider model a request may go to.
type Candidate struct {
	ProviderID string
	ModelID    string
	Model      catalog.Model
	Weight     int
	// Warnings name a catalog capability the request needs and this model does
	// not declare. They do not stop the attempt: the provider decides what it
	// serves, so the request is sent and the warning travels beside it.
	Warnings []string
}

// Warning records one advisory mismatch between a request and a candidate.
type Warning struct {
	ProviderID string
	ModelID    string
	Reason     string
}

// Skipped records a candidate a filter refused, so a preview explains why a
// request did not land where an operator expected.
type Skipped struct {
	ProviderID string
	ModelID    string
	Reason     string
}

// Plan is where one request will go: the ordered candidates it tries, and the
// ones that were refused on the way.
type Plan struct {
	Model        string
	Kind         Kind
	GroupID      string
	ContextLimit int64
	// LongContext reports that the caller named the million-token entry, so
	// a codec may claim the long-context beta from its own profile.
	LongContext bool
	// NativeMillion reports that the named entry is a million tokens on its own
	// rather than the long-context half of a pair. Such a request is already at
	// the full window, and a codec must not claim the long-context beta for it.
	NativeMillion bool
	Candidates    []Candidate
	Skipped       []Skipped
	// Warnings are the advisory mismatches the plan accepted anyway, so a
	// preview or a log can say what the request asked for that the catalog
	// doubts.
	Warnings []Warning
	// GroupSwitchOn4xx and GroupSwitchOn5xx are the route's own failover
	// choice, carried only by a plan a route produced: whether this request
	// may move to the route's next member after an unlisted status of that
	// class. A plan no route produced has nowhere to move to and leaves them
	// unset.
	GroupSwitchOn4xx *bool
	GroupSwitchOn5xx *bool
}

func withWarnings(plan *Plan, snapshot *catalog.Snapshot, model catalog.Model, needs catalog.Requirements, weight int) Candidate {
	warnings := snapshot.Warnings(model, needs)
	for _, reason := range warnings {
		plan.Warnings = append(plan.Warnings, Warning{
			ProviderID: model.ProviderID, ModelID: model.ID, Reason: reason,
		})
	}
	return Candidate{
		ProviderID: model.ProviderID, ModelID: model.ID, Model: model,
		Weight: weight, Warnings: warnings,
	}
}

// Options tunes a router.
type Options struct {
	Catalog *catalog.Catalog
	Pools   *account.Manager
	// Rand returns a value in [0,1), which a weighted group draws its order
	// from and a test injects.
	Rand      func() float64
	PinsLimit int
	// Clock reads the time failover waits are measured against.
	Clock clock.Clock
	// FailoverBackoff is the escalating waits a refused member walks through,
	// one step per consecutive failure. No steps keeps no denylist: a refused
	// member is immediately reusable.
	FailoverBackoff []time.Duration
}

// Router resolves requests against the catalog and orders the candidates a
// request may go to.
type Router struct {
	mu       sync.Mutex
	catalog  *catalog.Catalog
	pools    *account.Manager
	rand     func() float64
	rotation map[string]int
	latency  map[string]float64
	pins     *account.PinCache
	failover *Failover
}

// New returns a router over the given catalog and pools.
func New(options Options) *Router {
	draw := options.Rand
	if draw == nil {
		draw = defaultRand()
	}
	return &Router{
		catalog: options.Catalog, pools: options.Pools, rand: draw,
		rotation: map[string]int{}, latency: map[string]float64{},
		pins:     account.NewPinCache(options.PinsLimit, 0, nil),
		failover: NewFailover(options.Clock, options.FailoverBackoff),
	}
}

// Plan resolves a request into the candidates it will try, in order. A
// request whose every candidate was refused reports why, so a caller answers
// with the reason rather than an empty list.
func (r *Router) Plan(ctx context.Context, request Request) (Plan, error) {
	return r.resolve(ctx, request, false)
}

// Preview returns the plan a request would get without touching a rotation
// counter, a pin, or a random draw, so a surface can explain routing without
// changing it.
func (r *Router) Preview(ctx context.Context, request Request) (Plan, error) {
	return r.resolve(ctx, request, true)
}

func (r *Router) resolve(ctx context.Context, request Request, preview bool) (Plan, error) {
	snapshot, found := r.current()
	if !found {
		return Plan{}, ErrNoCatalog
	}
	plan := r.plan(snapshot, request, preview)
	plan.Candidates = r.failover.Filter(plan.Candidates)
	if err := planProblem(plan, snapshot); err != nil {
		return plan, err
	}
	return plan, nil
}

func (r *Router) current() (*catalog.Snapshot, bool) {
	if r.catalog == nil {
		return nil, false
	}
	return r.catalog.Snapshot()
}

func (r *Router) plan(snapshot *catalog.Snapshot, request Request, preview bool) Plan {
	requested := strings.TrimSpace(request.Model)
	if requested == "" {
		return Plan{}
	}
	// A public name is matched exactly against the map the snapshot built, so
	// a hyphenated identifier is never split back into a provider and a
	// model. A name that is not published falls through to the spellings an
	// operator stores and the console's own tester uses.
	if target, found := snapshot.ResolveClientID(requested); found {
		if plan, found := r.planClientTarget(snapshot, request, target, preview); found {
			return plan
		}
	}
	request.Model = requested
	return r.planSpelling(snapshot, request, preview)
}

func (r *Router) planClientTarget(snapshot *catalog.Snapshot, request Request, target catalog.ClientTarget, preview bool) (Plan, bool) {
	request.Model = target.ProviderID + "/" + target.ModelID
	if target.GroupID != "" {
		request.Model = target.GroupID
		if plan, found := r.fromGroup(snapshot, request, preview); found {
			applyContextVariant(&plan, snapshot, target)
			return plan, true
		}
		return Plan{}, false
	}
	if plan, found := r.fromQualified(snapshot, request); found {
		applyContextVariant(&plan, snapshot, target)
		return plan, true
	}
	return Plan{}, false
}

func (r *Router) planSpelling(snapshot *catalog.Snapshot, request Request, preview bool) Plan {
	if plan, found := r.fromGroup(snapshot, request, preview); found {
		return plan
	}
	if plan, found := r.fromQualified(snapshot, request); found {
		return plan
	}
	if plan, found := r.fromBare(snapshot, request, preview); found {
		return plan
	}
	if plan, found := r.fromPublicPassthrough(snapshot, request); found {
		return plan
	}
	return r.fromPassthrough(snapshot, request, preview)
}

func applyContextVariant(plan *Plan, snapshot *catalog.Snapshot, target catalog.ClientTarget) {
	plan.LongContext = target.MillionContext
	plan.NativeMillion = target.NativeMillion
	if target.MillionContext {
		return
	}
	if target.GroupID != "" {
		group, found := snapshot.Group(target.GroupID)
		if found && snapshot.OffersLongContextGroup(group) {
			plan.ContextLimit = catalog.DefaultContext
		}
		return
	}
	model, found := snapshot.Model(target.ProviderID, target.ModelID)
	if found && snapshot.OffersLongContext(model) {
		plan.ContextLimit = catalog.DefaultContext
	}
}

func (r *Router) fromPublicPassthrough(snapshot *catalog.Snapshot, request Request) (Plan, bool) {
	name := catalog.StripClaudeAlias(catalog.StripContextSuffix(request.Model))
	if !strings.HasPrefix(name, catalog.ClientPrefix) {
		return Plan{}, false
	}
	matched, modelID, matches := matchPassthroughProvider(snapshot, name)
	if matches != 1 {
		return Plan{}, false
	}
	plan := Plan{Model: name, Kind: KindPassthrough}
	plan.Candidates = []Candidate{withWarnings(&plan, snapshot, catalog.Model{
		ProviderID: matched.ID, ID: modelID, Category: "chat",
		APIFormat: matched.APIFormat, BaseURL: matched.BaseURL, Enabled: true,
	}, request.Needs, 1)}
	return plan, true
}

func matchPassthroughProvider(snapshot *catalog.Snapshot, name string) (catalog.Provider, string, int) {
	var matched catalog.Provider
	modelID := ""
	matches := 0
	for _, host := range snapshot.Providers {
		if !host.Configured || host.APIFormat == "" {
			continue
		}
		prefix := catalog.ClientPrefix + catalog.Slug(host.ID) + catalog.SlugSeparator
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		remainder := strings.TrimPrefix(name, prefix)
		if remainder == "" || catalog.ClientModelID(host.ID, remainder) != name {
			continue
		}
		matched, modelID, matches = host, remainder, matches+1
	}
	return matched, modelID, matches
}

func (r *Router) fromGroup(snapshot *catalog.Snapshot, request Request, preview bool) (Plan, bool) {
	group, found := snapshot.Group(request.Model)
	if !found || !group.Enabled {
		return Plan{}, false
	}
	plan := Plan{
		Model: request.Model, Kind: KindGroup, GroupID: group.ID,
		GroupSwitchOn4xx: new(group.SwitchOn4xx), GroupSwitchOn5xx: new(group.SwitchOn5xx),
	}
	for _, member := range group.Members {
		r.addGroupMember(snapshot, &plan, member, request.Needs)
	}
	plan.Candidates = r.orderGroup(group, plan.Candidates, preview)
	plan.Candidates = r.pinned(request.Conversation, prefixGroup(group.ID), plan.Candidates, preview)
	return plan, true
}

func (r *Router) addAutoMember(snapshot *catalog.Snapshot, plan *Plan, member catalog.Member, needs catalog.Requirements) {
	candidates, skipped := expandAuto(snapshot, member, needs, plan)
	plan.Candidates = append(plan.Candidates, candidates...)
	plan.Skipped = append(plan.Skipped, skipped...)
}

func (r *Router) addGroupMember(snapshot *catalog.Snapshot, plan *Plan, member catalog.Member, needs catalog.Requirements) {
	if !member.Enabled {
		plan.Skipped = append(plan.Skipped, Skipped{
			ProviderID: member.ProviderID, ModelID: member.ModelID,
			Reason: "the member is switched off",
		})
		return
	}
	// A member that names a bare identifier follows every connection that
	// serves it, so a route is not edited when a connection is added.
	if catalog.MemberKindOf(member.Kind) == catalog.MemberKindAuto {
		r.addAutoMember(snapshot, plan, member, needs)
		return
	}
	model, found := snapshot.Model(member.ProviderID, member.ModelID)
	if !found {
		plan.Skipped = append(plan.Skipped, Skipped{
			ProviderID: member.ProviderID, ModelID: member.ModelID,
			Reason: "the model is not in the catalog",
		})
		return
	}
	if reason := snapshot.SkipReason(model, needs); reason != "" {
		plan.Skipped = append(plan.Skipped, Skipped{
			ProviderID: member.ProviderID, ModelID: member.ModelID, Reason: reason,
		})
		return
	}
	plan.Candidates = append(plan.Candidates, withWarnings(plan, snapshot, model, needs, member.Weight))
}

func expandAuto(snapshot *catalog.Snapshot, member catalog.Member, needs catalog.Requirements, plan *Plan) ([]Candidate, []Skipped) {
	models := snapshot.ModelsNamed(member.ModelID)
	if len(models) == 0 {
		return nil, []Skipped{{
			ModelID: member.ModelID, Reason: "the model is not in the catalog",
		}}
	}
	candidates := make([]Candidate, 0, len(models))
	var skipped []Skipped
	for _, model := range models {
		if reason := snapshot.SkipReason(model, needs); reason != "" {
			skipped = append(skipped, Skipped{
				ProviderID: model.ProviderID, ModelID: model.ID, Reason: reason,
			})
			continue
		}
		candidates = append(candidates, withWarnings(plan, snapshot, model, needs, member.Weight))
	}
	return candidates, skipped
}

func (r *Router) fromQualified(snapshot *catalog.Snapshot, request Request) (Plan, bool) {
	providerID, modelID, found := strings.Cut(request.Model, "/")
	if !found {
		return Plan{}, false
	}
	model, found := snapshot.Model(providerID, modelID)
	if !found {
		return Plan{}, false
	}
	plan := Plan{Model: request.Model, Kind: KindQualified}
	if reason := snapshot.SkipReason(model, request.Needs); reason != "" {
		plan.Skipped = append(plan.Skipped, Skipped{
			ProviderID: providerID, ModelID: modelID, Reason: reason,
		})
		return plan, true
	}
	plan.Candidates = []Candidate{withWarnings(&plan, snapshot, model, request.Needs, 1)}
	return plan, true
}

func (r *Router) fromBare(snapshot *catalog.Snapshot, request Request, preview bool) (Plan, bool) {
	models := snapshot.ModelsNamed(request.Model)
	if len(models) == 0 {
		return Plan{}, false
	}
	plan := Plan{Model: request.Model, Kind: KindBare}
	for _, model := range models {
		if reason := snapshot.SkipReason(model, request.Needs); reason != "" {
			plan.Skipped = append(plan.Skipped, Skipped{
				ProviderID: model.ProviderID, ModelID: model.ID, Reason: reason,
			})
			continue
		}
		plan.Candidates = append(plan.Candidates, withWarnings(&plan, snapshot, model, request.Needs, 1))
	}
	plan.Candidates = r.pinned(request.Conversation, prefixBare(request.Model), plan.Candidates, preview)
	return plan, true
}

func (r *Router) fromPassthrough(snapshot *catalog.Snapshot, request Request, _ bool) Plan {
	plan := Plan{Model: request.Model, Kind: KindPassthrough}
	providerID, modelID, found := strings.Cut(request.Model, "/")
	if !found {
		return plan
	}
	host, known := snapshot.Provider(providerID)
	if !known || !host.Configured {
		return plan
	}
	if host.APIFormat == "" {
		return plan
	}
	plan.Candidates = []Candidate{withWarnings(&plan, snapshot, catalog.Model{
		ProviderID: providerID, ID: modelID, Category: "chat",
		APIFormat: host.APIFormat, BaseURL: host.BaseURL, Enabled: true,
	}, request.Needs, 1)}
	return plan
}

func prefixGroup(id string) string {
	return "group " + id
}

func prefixBare(model string) string {
	return "bare " + model
}

func candidateKey(candidate Candidate) string {
	return candidate.ProviderID + " " + candidate.ModelID
}

func defaultRand() func() float64 {
	var state sync.Mutex
	seed := uint64(0x9E3779B97F4A7C15)
	return func() float64 {
		state.Lock()
		defer state.Unlock()
		seed ^= seed << 13
		seed ^= seed >> 7
		seed ^= seed << 17
		return float64(seed>>11) / float64(1<<53)
	}
}

func planProblem(plan Plan, snapshot *catalog.Snapshot) error {
	if len(plan.Candidates) > 0 {
		return nil
	}
	if plan.Kind == KindGroup {
		if reason := firstSkip(plan); reason != "" {
			return fmt.Errorf("%s: %w (%s)", plan.GroupID, ErrNoRoute, reason)
		}
		return fmt.Errorf("%s: %w", plan.GroupID, ErrNoRoute)
	}
	if reason := firstSkip(plan); reason != "" {
		return fmt.Errorf("%s: %w (%s)", plan.Model, ErrNoRoute, reason)
	}
	if plan.Model == "" {
		return ErrModelRequired
	}
	if known := snapshot.ModelsNamed(plan.Model); len(known) > 0 {
		return fmt.Errorf("%s: %w (%s)", plan.Model, ErrModelUnconfigured, providersOf(known))
	}
	return fmt.Errorf("%s: %w", plan.Model, ErrModelNotFound)
}

func firstSkip(plan Plan) string {
	if len(plan.Skipped) == 0 {
		return ""
	}
	return plan.Skipped[0].Reason
}

func providersOf(models []catalog.Model) string {
	ids := make([]string, 0, len(models))
	for _, model := range models {
		ids = append(ids, model.ProviderID)
	}
	return strings.Join(ids, ", ")
}
