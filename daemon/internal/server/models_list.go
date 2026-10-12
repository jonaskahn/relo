// Data-plane model listing: the catalog rows clients choose from.
package server

import (
	"context"
	"net/http"
	"strings"

	appaccount "github.com/jonaskahn/relo/internal/application/account"
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
		if protocol == inference.ProtocolAnthropic || s.isAnthropicRequest(r) {
			listed := listedModels(snapshot, listView{advertised: s.advertisedContexts(r.Context()), stripClaude: true})
			writeJSON(w, http.StatusOK, anthropicModelList(listed))
			return
		}
		rows := []map[string]any{}
		if s.opts.Integrations != nil {
			rows = s.opts.Integrations.CodexCatalogRows()
		}
		attributeCodexRows(rows, listedModels(snapshot, listView{}))
		writeJSON(w, http.StatusOK, map[string]any{"models": rows})
	}
}

type listedModel struct {
	ID            string
	Name          string
	ProviderID    string
	SourceModelID string
	NativeMillion bool
	Paired        bool
	Suffixed      bool
	ContextWindow *int64
	MaxOutput     *int64
}

func listedModels(snapshot *catalog.Snapshot, view listView) []listedModel {
	entries := make([]listedModel, 0, 64)
	seen := map[string]bool{}
	for _, group := range snapshot.ListedGroups() {
		id := catalog.ClientRouteID(group.ID)
		seen[id] = true
		entries = appendListedVariants(entries, listedGroup(snapshot, view.advertised, group, id), snapshot.OffersLongContextGroup(group))
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
	window := groupSmaller(snapshot, group, func(m catalog.Model) *int64 {
		return advertisedContext(advertised, m)
	})
	return listedModel{
		ID:            id,
		Name:          catalog.ClientModelName(labelOr(group.Label, group.ID), catalog.RouteLabelPrefix, namedWindow(window, snapshot.NativeMillionGroup(group)), false),
		NativeMillion: snapshot.NativeMillionGroup(group),
		Suffixed:      snapshot.OffersSuffixedGroup(group),
		ContextWindow: window,
		MaxOutput:     groupSmaller(snapshot, group, func(m catalog.Model) *int64 { return m.MaxOutput }),
	}
}

func listedStandalone(snapshot *catalog.Snapshot, view listView, model catalog.Model, id string) listedModel {
	window := advertisedContext(view.advertised, model)
	viewModel := model
	viewModel.ContextWindow = window
	label := labelOr(model.Name, model.ID)
	if view.stripClaude && snapshot.ClaudeSubscription(model.ProviderID) {
		label = catalog.StripClaudePrefix(label)
	}
	return listedModel{
		ID: id, Name: catalog.ClientModelName(label, providerLabel(snapshot, model.ProviderID), namedWindow(window, snapshot.MillionAlone(viewModel)), cursorMaxLane(snapshot, model)),
		ProviderID: model.ProviderID, SourceModelID: model.ID,
		NativeMillion: snapshot.MillionAlone(viewModel),
		Suffixed:      snapshot.OffersSuffixed(viewModel),
		ContextWindow: window, MaxOutput: model.MaxOutput,
	}
}

// namedWindow is the window the display name may mark. A model that is a
// million-token entry on its own carries no generated marker on this surface.
func namedWindow(window *int64, alone bool) *int64 {
	if alone {
		return nil
	}
	return window
}

// cursorMaxLane reports whether a model names a max-mode Cursor lane. Native
// lanes carry the cursor-agent format; lanes a Cursor template serves over
// another format name max mode in the id itself. Published identifiers carry
// Relo's own -1m marker after the lane, so ...-fast-max-1m qualifies. A
// non-Cursor model that happens to read that way is never marked.
func cursorMaxLane(snapshot *catalog.Snapshot, model catalog.Model) bool {
	if model.APIFormat == catalog.FormatCursorAgent {
		return catalog.HasMaxLane(model.ID)
	}
	return cursorTemplate(snapshot, model.ProviderID) && catalog.HasMaxLane(model.ID)
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
	if value, found := advertised[appaccount.AdvertisedContextKey(model.ProviderID, model.ID)]; found {
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
			"max_input_tokens": nullableInt(entry.ContextWindow),
			"max_tokens":       nullableInt(entry.MaxOutput),
		}
		if entry.ProviderID != "" {
			row["provider_id"] = entry.ProviderID
			row["source_model_id"] = entry.SourceModelID
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
