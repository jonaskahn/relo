// Data-plane model listing: the catalog rows clients choose from.
package server

import (
	"context"
	"net/http"
	"strings"

	"github.com/jonaskahn/relo/internal/adapters/wire"
	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/internal/inference"
)

func (s *Server) handleListModels(protocol string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		snapshot, found := s.opts.Catalog.Snapshot()
		if !found {
			s.fail(w, r, refusal{
				Status: http.StatusServiceUnavailable, Code: "service_unavailable",
				Message: "api.service.unavailable",
			})
			return
		}
		profile := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("profile")))
		anthropic := protocol == inference.ProtocolAnthropic || s.isAnthropicRequest(r)
		if profile == "anthropic" && protocol != inference.ProtocolAnthropic {
			writeError(w, http.StatusBadRequest, "incompatible_profile", "the anthropic profile requires the Anthropic protocol")
			return
		}
		if profile != "" && anthropic && profile != "anthropic" {
			writeError(w, http.StatusBadRequest, "incompatible_profile", "the Anthropic protocol does not serve the "+profile+" profile")
			return
		}
		if anthropicProfile(profile, anthropic) {
			listed := listedModels(snapshot, listView{advertised: s.advertisedContexts(r.Context()), stripClaude: true})
			writeJSON(w, http.StatusOK, anthropicModelList(listed))
			return
		}
		if profile == "" && wireClientKind(r) == "codex" {
			profile = "codex"
		}
		if profile != "" && profile != "openai" && profile != "codex" {
			writeError(w, http.StatusBadRequest, "unknown_profile", "unknown model-list profile")
			return
		}
		if profile == "codex" {
			rows := []map[string]any{}
			if s.opts.Integrations != nil {
				rows = s.opts.Integrations.CodexCatalogRows()
			}
			attributeCodexRows(rows, listedModels(snapshot, listView{}))
			writeJSON(w, http.StatusOK, map[string]any{"models": rows})
			return
		}
		writeJSON(w, http.StatusOK, openAIModelList(listedModels(snapshot, listView{})))
	}
}

func anthropicProfile(profile string, anthropic bool) bool {
	if anthropic {
		return true
	}
	return profile == "anthropic"
}

func wireClientKind(r *http.Request) string {
	if r == nil {
		return ""
	}
	return wire.ClientKind(r.Header)
}

type listedModel struct {
	ID               string
	Name             string
	ProviderID       string
	SourceModelID    string
	NativeMillion    bool
	Paired           bool
	Suffixed         bool
	ContextWindow    *int64
	MaxInput         *int64
	MaxOutput        *int64
	Tools            *bool
	Reasoning        *bool
	Vision           *bool
	ReasoningEfforts []string
	ReasoningToggle  *bool
	Sources          catalog.FactSource
}

func listedModels(snapshot *catalog.Snapshot, view listView) []listedModel {
	entries := make([]listedModel, 0, 64)
	seen := map[string]bool{}
	for _, group := range snapshot.ListedGroups() {
		id := catalog.ClientRouteID(group.ID)
		seen[id] = true
		entries = appendListedVariants(entries, listedGroup(snapshot, view.advertised, group, id),
			snapshot.OffersLongContextGroup(group) && groupAdvertisedAtMillion(snapshot, group))
	}
	for _, model := range snapshot.Listed() {
		id := catalog.ClientModelID(model.ProviderID, model.ID)
		if seen[id] {
			continue
		}
		seen[id] = true
		entries = appendListedVariants(entries, listedStandalone(snapshot, view, model, id), snapshot.OffersLongContext(model))
	}
	return entries
}

type listView struct {
	advertised  map[string]*int64
	stripClaude bool
}

func listedGroup(snapshot *catalog.Snapshot, advertised map[string]*int64, group catalog.Group, id string) listedModel {
	members := advertisedGroupModels(snapshot, group)
	capabilities := catalog.ConservativeCapabilities(members)
	window := capabilities.ContextWindow
	atMillion := groupAdvertisedAtMillion(snapshot, group)
	return listedModel{
		ID:               id,
		Name:             catalog.ClientModelName(labelOr(group.Label, group.ID), catalog.RouteLabelPrefix, namedWindow(window, snapshot.NativeMillionGroup(group) && atMillion), ""),
		NativeMillion:    snapshot.NativeMillionGroup(group) && atMillion,
		Suffixed:         snapshot.OffersSuffixedGroup(group) && atMillion,
		ContextWindow:    window,
		MaxInput:         capabilities.MaxInput,
		MaxOutput:        capabilities.MaxOutput,
		Tools:            capabilities.SupportsTools,
		Reasoning:        capabilities.SupportsReasoning,
		Vision:           capabilities.SupportsVision,
		ReasoningEfforts: append([]string(nil), capabilities.ReasoningEfforts...),
		ReasoningToggle:  capabilities.ReasoningToggle,
	}
}

func groupAdvertisedAtMillion(snapshot *catalog.Snapshot, group catalog.Group) bool {
	members := advertisedGroupModels(snapshot, group)
	if len(members) == 0 {
		return false
	}
	for _, model := range members {
		if model.ContextWindow == nil || *model.ContextWindow < catalog.MillionContext {
			return false
		}
	}
	return true
}

func listedStandalone(snapshot *catalog.Snapshot, view listView, model catalog.Model, id string) listedModel {
	viewModel := snapshot.AdvertisedModel(model)
	window := advertisedContext(view.advertised, viewModel)
	viewModel.ContextWindow = window
	label := labelOr(model.Name, model.ID)
	if view.stripClaude && snapshot.ClaudeSubscription(model.ProviderID) {
		label = catalog.StripClaudePrefix(label)
	}
	return listedModel{
		ID: id, Name: catalog.ClientModelName(label, providerLabel(snapshot, model.ProviderID), namedWindow(window, snapshot.MillionAlone(viewModel)), cursorLaneMarker(snapshot, model)),
		ProviderID: model.ProviderID, SourceModelID: model.ID,
		NativeMillion: snapshot.MillionAlone(viewModel),
		Suffixed:      snapshot.OffersSuffixed(viewModel),
		ContextWindow: window, MaxInput: viewModel.MaxInput, MaxOutput: viewModel.MaxOutput,
		Tools: viewModel.SupportsTools, Reasoning: viewModel.SupportsReasoning, Vision: viewModel.SupportsVision,
		ReasoningEfforts: append([]string(nil), viewModel.ReasoningEfforts...),
		ReasoningToggle:  knownToggle(viewModel),
		Sources:          viewModel.FactSources,
	}
}

func knownToggle(model catalog.Model) *bool {
	if !model.ReasoningToggleKnown {
		return nil
	}
	value := model.ReasoningToggle
	return &value
}

func advertisedGroupModels(snapshot *catalog.Snapshot, group catalog.Group) []catalog.Model {
	models := snapshot.GroupModels(group)
	advertised := make([]catalog.Model, 0, len(models))
	for _, model := range models {
		advertised = append(advertised, snapshot.AdvertisedModel(model))
	}
	return advertised
}

// namedWindow is the window the display name may mark. A model that is a
// million-token entry on its own carries no generated marker on this surface.
func namedWindow(window *int64, alone bool) *int64 {
	if alone {
		return nil
	}
	return window
}

// cursorLaneMarker names the variant lanes a Cursor model runs: fast, max,
// or both. Native lanes carry the cursor-agent format; lanes a Cursor
// template serves over another format name the lanes in the id itself.
// Published identifiers carry Relo's own -1m marker after the lanes, so
// ...-fast-max-1m qualifies. A non-Cursor model that happens to read that
// way is never marked.
func cursorLaneMarker(snapshot *catalog.Snapshot, model catalog.Model) string {
	if model.APIFormat == catalog.FormatCursorAgent {
		return catalog.LaneMarker(model.ID)
	}
	if !cursorTemplate(snapshot, model.ProviderID) {
		return ""
	}
	return catalog.LaneMarker(model.ID)
}

// cursorTemplate reports whether a provider runs the Cursor sign-in
// template, so its lanes are Cursor's even when another format serves them.
func cursorTemplate(snapshot *catalog.Snapshot, providerID string) bool {
	host, found := snapshot.Provider(providerID)
	return found && host.TemplateID == "cursor"
}

func appendListedVariants(entries []listedModel, entry listedModel, enabled bool) []listedModel {
	standard, paired := catalog.StandardContext(entry.ContextWindow, enabled)
	if !paired {
		if entry.Suffixed {
			// The model is published once, under the suffix alone, at its
			// own window.
			entry.ID = catalog.MillionAlias(entry.ID)
		}
		return append(entries, entry)
	}
	// A paired Claude model is published once, as its 1M entry alone. The
	// bare 200K spelling still resolves for saved clients, but it is no
	// longer advertised.
	_ = standard
	entry.Paired = true
	entry.ID = catalog.MillionAlias(entry.ID)
	return append(entries, entry)
}

func attributeCodexRows(rows []map[string]any, listed []listedModel) {
	bySlug := make(map[string]listedModel, len(listed))
	for _, entry := range listed {
		if entry.ProviderID != "" {
			bySlug[entry.ID] = entry
		}
	}
	for _, row := range rows {
		slug, _ := row["slug"].(string)
		if entry, found := bySlug[slug]; found {
			row["provider_id"] = entry.ProviderID
			row["source_model_id"] = entry.SourceModelID
		}
	}
}

func providerLabel(snapshot *catalog.Snapshot, providerID string) string {
	if host, found := snapshot.Provider(providerID); found {
		return labelOr(host.Label, providerID)
	}
	return providerID
}

func advertisedContext(advertised map[string]*int64, model catalog.Model) *int64 {
	if model.FactSources.ContextWindow == "account" {
		return model.ContextWindow
	}
	if value, found := advertised[model.ProviderID+"\x00"+model.ID]; found {
		return value
	}
	return model.ContextWindow
}

func (s *Server) advertisedContexts(ctx context.Context) map[string]*int64 {
	accounts := s.opts.Accounts
	if accounts == nil {
		return nil
	}
	return accounts.AdvertisedContexts(ctx)
}

func groupSmaller(snapshot *catalog.Snapshot, group catalog.Group, pick func(catalog.Model) *int64) *int64 {
	var smallest *int64
	for _, model := range routableMembers(snapshot, group) {
		value := pick(model)
		if value == nil {
			continue
		}
		if smallest == nil || *value < *smallest {
			smallest = value
		}
	}
	return smallest
}

func routableMembers(snapshot *catalog.Snapshot, group catalog.Group) []catalog.Model {
	members := make([]catalog.Model, 0, len(group.Members))
	for _, member := range group.Members {
		if !member.Enabled {
			continue
		}
		model, found := snapshot.Model(member.ProviderID, member.ModelID)
		if !found || snapshot.SkipReason(model, catalog.Requirements{}) != "" {
			continue
		}
		members = append(members, model)
	}
	return members
}

func anthropicModelList(entries []listedModel) map[string]any {
	data := make([]map[string]any, 0, len(entries))
	for _, entry := range entries {
		row := map[string]any{
			"type": "model", "id": anthropicModelID(entry), "display_name": entry.Name,
			"created_at":       "1970-01-01T00:00:00Z",
			"max_input_tokens": nullableInt(entry.MaxInput),
			"max_tokens":       nullableInt(entry.MaxOutput),
			"relo":             capabilityExtension(entry),
		}
		data = append(data, row)
	}
	response := map[string]any{"data": data, "has_more": false}
	if len(entries) > 0 {
		response["first_id"] = data[0]["id"]
		response["last_id"] = data[len(data)-1]["id"]
	}
	return response
}

func openAIModelList(entries []listedModel) map[string]any {
	data := make([]map[string]any, 0, len(entries))
	for _, entry := range entries {
		owner := entry.ProviderID
		if owner == "" {
			owner = "relo"
		}
		data = append(data, map[string]any{
			"id": entry.ID, "object": "model", "created": 0, "owned_by": owner,
			"relo": capabilityExtension(entry),
		})
	}
	return map[string]any{"object": "list", "data": data}
}

func capabilityExtension(entry listedModel) map[string]any {
	extension := map[string]any{
		"display_name":       entry.Name,
		"context_window":     nullableInt(entry.ContextWindow),
		"max_input_tokens":   nullableInt(entry.MaxInput),
		"max_output_tokens":  nullableInt(entry.MaxOutput),
		"supports_tools":     nullableBool(entry.Tools),
		"supports_reasoning": nullableBool(entry.Reasoning),
		"supports_vision":    nullableBool(entry.Vision),
		"reasoning_efforts":  nullableStrings(entry.ReasoningEfforts),
		"reasoning_toggle":   nullableBool(entry.ReasoningToggle),
	}
	if entry.ProviderID != "" {
		extension["provider_id"] = entry.ProviderID
		extension["source_model_id"] = entry.SourceModelID
	}
	sources := map[string]string{}
	for name, source := range map[string]string{
		"context_window": entry.Sources.ContextWindow, "max_input_tokens": entry.Sources.MaxInput,
		"max_output_tokens": entry.Sources.MaxOutput, "tools": entry.Sources.Tools,
		"reasoning": entry.Sources.Reasoning, "vision": entry.Sources.Vision,
	} {
		if source != "" {
			sources[name] = source
		}
	}
	if len(sources) > 0 {
		extension["sources"] = sources
	}
	return extension
}

func nullableBool(value *bool) any {
	if value == nil {
		return nil
	}
	return *value
}

func nullableStrings(value []string) any {
	if value == nil {
		return nil
	}
	return value
}

func anthropicModelID(entry listedModel) string {
	window := entry.ContextWindow
	// The marker means the long-context variant of a model, so it is spelled
	// only where that variant exists: an entry that is a million tokens on its
	// own has none, and neither has one that lost its twin to a name a model
	// of the provider already holds.
	if entry.NativeMillion || (!entry.Paired && !entry.Suffixed) {
		window = nil
	}
	if entry.ProviderID != "" && entry.SourceModelID != "" {
		// A provider that spells its own marker into the name, kimi-k3[1M], keeps
		// that name whether or not we add one: stripping it would make the
		// Anthropic identifier name the other model. It is only dropped when ours
		// goes on beside it, where the two together name nothing that resolves.
		model := entry.SourceModelID
		if window != nil {
			model = catalog.StripContextMarker(model)
		}
		return catalog.AnthropicModelAlias(entry.ProviderID, model, window)
	}
	// A route's 1M entry is listed under the -1m name other clients read, so the
	// marker it already carries is dropped before Claude Code's own is applied.
	return catalog.AnthropicClientID(catalog.StripMillionAlias(entry.ID), window)
}

func nullableInt(value *int64) any {
	if value == nil {
		return nil
	}
	return *value
}

func labelOr(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}
