// Snapshot lookup: providers and models by id.
package catalog

import (
	"strings"
	"time"
)

// Provider returns one provider by identifier.
func (s *Snapshot) Provider(id string) (Provider, bool) {
	host, found := s.byProvider[id]
	return host, found
}

// Models returns one provider's models, ordered by identifier.
func (s *Snapshot) Models(providerID string) []Model {
	return s.byProviderM[providerID]
}

// Model returns one provider's model.
func (s *Snapshot) Model(providerID, modelID string) (Model, bool) {
	model, found := s.byModelKey[key{providerID: providerID, modelID: modelID}]
	return model, found
}

// ModelsNamed returns every model with this identifier, across providers, in
// the order routing tries them.
func (s *Snapshot) ModelsNamed(modelID string) []Model {
	return s.byModelID[modelID]
}

// Group returns one group by identifier.
func (s *Snapshot) Group(id string) (Group, bool) {
	group, found := s.byGroupID[id]
	return group, found
}

// Counts reports how many providers and models the snapshot holds.
func (s *Snapshot) Counts() (providers, models, routable, configured int) {
	for _, host := range s.Providers {
		providers++
		if host.Routable {
			routable++
		}
		if host.Configured {
			configured++
		}
		models += len(s.byProviderM[host.ID])
	}
	return providers, models, routable, configured
}

// Eligible reports whether a request may go to a model.
func (s *Snapshot) Eligible(model Model, needs Requirements) bool {
	if reason := s.SkipReason(model, needs); reason != "" {
		return false
	}
	return true
}

// Coverage counts the accounts of one model's connection that could take a
// request now, and how many of them can serve the model. It is read from the
// live pools rather than from the snapshot, so a fresh listing or an
// account-level refusal is reflected the moment it happens.
func (s *Snapshot) Coverage(model Model) (serving, active int) {
	if s.accounts == nil {
		return 0, 0
	}
	return s.accounts.Coverage(model.ProviderID, model)
}

// Requirements is what one request needs from a model.
type Requirements struct {
	Images       bool
	Tools        bool
	PromptTokens int64
}

// SkipReason names why a request cannot go to a model, empty when it can.
func (s *Snapshot) SkipReason(model Model, needs Requirements) string {
	host, found := s.byProvider[model.ProviderID]
	if !found {
		return "the provider is not in the catalog"
	}
	if !host.Routable {
		return host.UnroutableReason
	}
	if !host.Enabled {
		return "the provider is switched off"
	}
	if !host.Configured {
		return "the provider has no account"
	}
	if !model.Routable {
		return model.UnroutableReason
	}
	if !model.Enabled {
		return "the model is switched off"
	}
	if model.Available != nil && !*model.Available {
		return "the provider no longer lists the model"
	}
	return s.accountSkipReason(host, model)
}

func (s *Snapshot) accountSkipReason(host Provider, model Model) string {
	// A model one connection lists may still be out of reach for every
	// account behind it, which is what an account-level entitlement means. A
	// keyless connection holds no accounts and is never filtered here.
	if host.Auth != AuthNone && s.accounts != nil && !s.accounts.ServesModel(model.ProviderID, model) {
		if until := s.accounts.CooldownUntil(model.ProviderID); !until.IsZero() {
			return coolingDownReason(model.ProviderID, until)
		}
		return "no account of this connection can serve the model"
	}
	return ""
}

func coolingDownReason(providerID string, until time.Time) string {
	return providerID + ": every account is cooling down after upstream rate limiting until " + until.UTC().Format("15:04 MST")
}

// Warnings names what a request asks of a model the catalog says it cannot
// do. Unlike SkipReason these do not stop the request: a provider is the
// authority on what it serves, so the request is sent and the warning is
// reported beside the answer an upstream gives. A refusal stays visible as the
// upstream's own error.
func (s *Snapshot) Warnings(model Model, needs Requirements) []string {
	warnings := make([]string, 0, 3)
	if needs.Tools && model.SupportsTools != nil && !*model.SupportsTools {
		warnings = append(warnings, "the model may not call tools")
	}
	if needs.Images && model.SupportsVision != nil && !*model.SupportsVision {
		warnings = append(warnings, "the model may not read images")
	}
	if model.ContextWindow != nil && needs.PromptTokens > *model.ContextWindow {
		warnings = append(warnings, "the request is longer than the model's context window")
	}
	return warnings
}

// Listed models are the ones a client's own model picker should offer. Two
// connections that publish the same upstream identifier are two entries under
// two public names, so a picker shows both rather than hiding one behind the
// other's spelling.
func (s *Snapshot) Listed() []Model {
	listed := make([]Model, 0, 64)
	seen := map[string]bool{}
	for _, host := range s.Providers {
		if !host.Configured {
			continue
		}
		for _, model := range s.byProviderM[host.ID] {
			name := ClientModelID(model.ProviderID, model.ID)
			if !listable(model) || seen[name] {
				continue
			}
			seen[name] = true
			listed = append(listed, model)
		}
	}
	return listed
}

func listable(model Model) bool {
	// A catalog entry that labels a model deprecated describes it; the switch
	// the operator owns is what decides whether a model is served.
	if !model.Enabled || !model.Routable {
		return false
	}
	if model.Available != nil && !*model.Available {
		return false
	}
	switch model.Category {
	case CategoryChat, CategoryReasoning, CategoryVision:
		return true
	default:
		return false
	}
}

// ListedGroups returns the groups a client's model picker should offer.
func (s *Snapshot) ListedGroups() []Group {
	listed := make([]Group, 0, 8)
	for _, group := range s.Groups {
		if !group.Enabled || !group.Listed {
			continue
		}
		if len(s.eligibleMembers(group, Requirements{})) > 0 {
			listed = append(listed, group)
		}
	}
	return listed
}

// ClaudeSubscription reports whether a connection is a Claude.ai sign-in.
// Only such a connection takes the native and beta 1M rules below.
func (s *Snapshot) ClaudeSubscription(providerID string) bool {
	host, found := s.byProvider[providerID]
	return found && host.TemplateID == ClaudeSubscriptionTemplate
}

// MillionAlone reports whether a model is a million-token entry on its own,
// so it is published once under its bare name. A natively million-token
// Claude generation qualifies only on a Claude.ai sign-in; anywhere else it
// is published with the generated suffix. An upstream identifier that already
// names itself a million-token model qualifies on any connection.
func (s *Snapshot) MillionAlone(model Model) bool {
	if ProviderMillionMarker(model.ID, model.ContextWindow) {
		return true
	}
	return s.ClaudeSubscription(model.ProviderID) && NativeMillionContext(model.ID, model.ContextWindow)
}

// MillionSuffixed reports whether a model is published once, at its own
// window, under the generated 1M suffix: a million-token model that is
// neither paired nor alone on its connection.
func (s *Snapshot) MillionSuffixed(model Model) bool {
	if model.ContextWindow == nil || *model.ContextWindow < MillionContext {
		return false
	}
	return !s.MillionAlone(model) && !s.pairsWithOneMillion(model)
}

// OffersLongContext reports whether a model was published beside a separate 1M
// entry, so an agent can pick the window that stays in the base rate. The
// window alone does not answer it: a model that is a million-token entry on its
// own is published once, and a derived name is dropped when a model of the
// provider already claims it.
func (s *Snapshot) OffersLongContext(model Model) bool {
	return s.pairsWithOneMillion(model) && s.paired[ClientModelID(model.ProviderID, model.ID)]
}

func (s *Snapshot) pairsWithOneMillion(model Model) bool {
	if !s.ClaudeSubscription(model.ProviderID) {
		return false
	}
	if model.APIFormat != FormatAnthropic {
		return false
	}
	if model.ContextWindow == nil || *model.ContextWindow < MillionContext {
		return false
	}
	return !s.MillionAlone(model)
}

// OffersSuffixed reports whether a model is published once, under the
// generated 1M suffix alone. A derived suffix that collides with a name a
// model of the provider already holds is not published, so the model keeps
// its bare name there.
func (s *Snapshot) OffersSuffixed(model Model) bool {
	return s.MillionSuffixed(model) && s.suffixed[ClientModelID(model.ProviderID, model.ID)]
}

// OffersLongContextGroup reports whether a route was published beside a separate
// 1M entry.
func (s *Snapshot) OffersLongContextGroup(group Group) bool {
	return s.pairsWithOneMillionGroup(group) && s.paired[ClientRouteID(group.ID)]
}

// OffersSuffixedGroup reports whether a route is published once, under the
// generated 1M suffix alone.
func (s *Snapshot) OffersSuffixedGroup(group Group) bool {
	return s.SuffixedMillionGroup(group) && s.suffixed[ClientRouteID(group.ID)]
}

func (s *Snapshot) pairsWithOneMillionGroup(group Group) bool {
	found := false
	for _, member := range group.Members {
		if !member.Enabled {
			continue
		}
		if MemberKindOf(member.Kind) != MemberKindModel {
			return false
		}
		model, ok := s.Model(member.ProviderID, member.ModelID)
		if !ok || !s.pairsWithOneMillion(model) {
			return false
		}
		found = true
	}
	return found
}

// NativeMillionGroup reports whether every enabled member of a route is a
// million-token entry on its own. Such a route is published once, at that
// window, because there is no second entry for a client to spell its way to.
// A route is native only when it has enabled members and every one of them is.
func (s *Snapshot) NativeMillionGroup(group Group) bool {
	found := false
	for _, member := range group.Members {
		if !member.Enabled {
			continue
		}
		model, ok := s.Model(member.ProviderID, member.ModelID)
		if !ok || !s.MillionAlone(model) {
			return false
		}
		found = true
	}
	return found
}

// SuffixedMillionGroup reports whether a route is published once, at its own
// window, under the generated 1M suffix: every enabled member reaches a
// million tokens, and no member takes the native or paired rule.
func (s *Snapshot) SuffixedMillionGroup(group Group) bool {
	found := false
	for _, member := range group.Members {
		if !member.Enabled {
			continue
		}
		if MemberKindOf(member.Kind) != MemberKindModel {
			return false
		}
		model, ok := s.Model(member.ProviderID, member.ModelID)
		if !ok || model.ContextWindow == nil || *model.ContextWindow < MillionContext {
			return false
		}
		if s.MillionAlone(model) || s.pairsWithOneMillion(model) {
			return false
		}
		found = true
	}
	return found
}

// StandardContext names the window a published entry carries, and whether a
// separate million-token entry is published beside it: a model that reaches 1M
// on the long-context beta is published twice so an agent can pick the window
// that stays in the base rate. Every other entry is published once, at its own
// window, which is what a model that is natively 1M needs.
func StandardContext(window *int64, offersAlternate bool) (*int64, bool) {
	if !offersAlternate || window == nil || *window < MillionContext {
		return window, false
	}
	standard := DefaultContext
	return &standard, true
}

func (s *Snapshot) eligibleMembers(group Group, needs Requirements) []Member {
	members := make([]Member, 0, len(group.Members))
	for _, member := range group.Members {
		if !member.Enabled {
			continue
		}
		model, found := s.Model(member.ProviderID, member.ModelID)
		if !found || s.SkipReason(model, needs) != "" {
			continue
		}
		members = append(members, member)
	}
	return members
}

// GroupModels resolves a route's enabled members to the catalog models they
// serve: a named member is one model, a bare identifier is every connection
// serving it, and a member the catalog cannot route is left out. It is the
// set a route draws its published budget, promised capabilities, and
// capability warnings from, the same set the model list reads.
func (s *Snapshot) GroupModels(group Group) []Model {
	models := make([]Model, 0, len(group.Members))
	for _, member := range group.Members {
		if !member.Enabled {
			continue
		}
		if MemberKindOf(member.Kind) == MemberKindAuto {
			for _, model := range s.ModelsNamed(member.ModelID) {
				if s.SkipReason(model, Requirements{}) == "" {
					models = append(models, model)
				}
			}
			continue
		}
		model, found := s.Model(member.ProviderID, member.ModelID)
		if found && s.SkipReason(model, Requirements{}) == "" {
			models = append(models, model)
		}
	}
	return models
}

// The capabilities a route cannot promise to a client that reads them from
// the published model list, because a member reachable under the same name
// lacks them. They are advisory: Relo publishes no reasoning or vision on a
// route entry, so a member that offers them is the one that loses them.
const (
	WarningReasoningOff = "reasoning_off"
	WarningVisionOff    = "vision_off"
	WarningToolsMixed   = "tools_mixed"
)

// CapabilityWarnings names each capability a route cannot promise whichever
// member answers. A null capability states nothing and raises nothing, and a
// uniform capability raises nothing: it is only a mix that loses something.
func (s *Snapshot) CapabilityWarnings(group Group) []string {
	warnings := make([]string, 0, 3)
	seen := capabilitySpread{}
	for _, model := range s.GroupModels(group) {
		seen.observe(model)
	}
	return seen.warnings(warnings)
}

type capabilitySpread struct {
	calling, refusing, reasoning, noReasoning, vision, noVision bool
}

func (c *capabilitySpread) observe(model Model) {
	observeCapability(model.SupportsTools, &c.calling, &c.refusing)
	observeCapability(model.SupportsReasoning, &c.reasoning, &c.noReasoning)
	observeCapability(model.SupportsVision, &c.vision, &c.noVision)
}

func observeCapability(stated *bool, affirmative, negative *bool) {
	if stated == nil {
		return
	}
	if *stated {
		*affirmative = true
	} else {
		*negative = true
	}
}

func (c capabilitySpread) warnings(warnings []string) []string {
	if c.reasoning && c.noReasoning {
		warnings = append(warnings, WarningReasoningOff)
	}
	if c.vision && c.noVision {
		warnings = append(warnings, WarningVisionOff)
	}
	if c.calling && c.refusing {
		warnings = append(warnings, WarningToolsMixed)
	}
	return warnings
}

// PromisedCapabilities reports whether a route can promise tool calling,
// reasoning and vision whichever member answers. A capability is true only
// when at least one reachable model states it and none deny it, false when
// any reachable model denies it, and nil when no reachable model states it.
func (s *Snapshot) PromisedCapabilities(group Group) (tools, reasoning, vision *bool) {
	var statedTools, deniedTools bool
	var statedReasoning, deniedReasoning, statedVision, deniedVision bool
	for _, model := range s.GroupModels(group) {
		if model.SupportsTools != nil {
			if *model.SupportsTools {
				statedTools = true
			} else {
				deniedTools = true
			}
		}
		if model.SupportsReasoning != nil {
			if *model.SupportsReasoning {
				statedReasoning = true
			} else {
				deniedReasoning = true
			}
		}
		if model.SupportsVision != nil {
			if *model.SupportsVision {
				statedVision = true
			} else {
				deniedVision = true
			}
		}
	}
	return promisedCapability(statedTools, deniedTools),
		promisedCapability(statedReasoning, deniedReasoning),
		promisedCapability(statedVision, deniedVision)
}

func promisedCapability(stated, denied bool) *bool {
	if denied {
		value := false
		return &value
	}
	if !stated {
		return nil
	}
	value := true
	return &value
}

// ShadowsModel reports whether a route id is also a served model name, which
// is what decides whether a client asking for it gets the route or the model.
func (s *Snapshot) ShadowsModel(groupID string) bool {
	return len(s.byModelID[groupID]) > 0
}

// PromptTokensFrom estimates the tokens a prompt of this length costs, so
// usage can be recorded where the vendor reported none.
func PromptTokensFrom(characters int) int64 {
	if characters <= 0 {
		return 0
	}
	return int64(characters+3) / 4
}

// NormalizeCategory folds a vendor's category spelling into the one the
// catalog compares, so lookups never miss on case or padding.
func NormalizeCategory(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

// NowMs is the current instant in Unix milliseconds, the clock stored rows use.
func NowMs() int64 {
	return time.Now().UnixMilli()
}
