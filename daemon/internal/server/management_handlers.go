// Management API handlers and the response shapes they render.
package server

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/jonaskahn/relo/internal/account"
	"github.com/jonaskahn/relo/internal/activity"
	appaccount "github.com/jonaskahn/relo/internal/application/account"
	appactivity "github.com/jonaskahn/relo/internal/application/activity"
	appcatalog "github.com/jonaskahn/relo/internal/application/catalog"
	approuting "github.com/jonaskahn/relo/internal/application/routing"
	appsettings "github.com/jonaskahn/relo/internal/application/settings"
	appstatus "github.com/jonaskahn/relo/internal/application/status"
	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/internal/config"
	"github.com/jonaskahn/relo/internal/routing"
)

type accountsResponse struct {
	Items []accountResponse `json:"items"`
}

type providersResponse struct {
	Items []providerResponse `json:"items"`
	Total int                `json:"total"`
}

type modelsResponse struct {
	Items []modelResponse `json:"items"`
	Total int             `json:"total"`
}

type modelsDevSearchResponse struct {
	Items     []modelsDevSearchHitResponse `json:"items"`
	ModelsDev modelsDevStateResponse       `json:"modelsdev"`
}

type groupsResponse struct {
	Items []groupResponse `json:"items"`
}

type usageResponse struct {
	Items []activity.UsageRollupRow `json:"items"`
	// Archive reports that the read reached past the request rows retention
	// keeps, so older days come from their aggregate and the time bounds in
	// that part are whole UTC days.
	Archive       bool  `json:"archive"`
	ArchiveFromMs int64 `json:"archive_from_ms,omitempty"`
}

type logsResponse struct {
	Items      []activity.UsageLogRow `json:"items"`
	NextCursor string                 `json:"next_cursor,omitempty"`
}

type attemptsResponse struct {
	Items []activity.UsageAttemptRow `json:"items"`
}

type doctorResponse struct {
	Checks []doctorCheckResponse `json:"checks"`
}

type createAccountRequest struct {
	ProviderID string `json:"provider_id"`
	Kind       string `json:"kind"`
	Label      string `json:"label"`
	Secret     string `json:"secret"`
	Priority   int    `json:"priority"`
}

type patchAccountRequest struct {
	Status   *string `json:"status"`
	Priority *int    `json:"priority"`
	Label    *string `json:"label"`
}

type patchProviderRequest struct {
	Enabled      *bool             `json:"enabled"`
	Rank         *int              `json:"rank"`
	PoolStrategy *string           `json:"pool_strategy"`
	BaseURL      *string           `json:"base_url"`
	Variables    map[string]string `json:"variables"`
	Label        *string           `json:"label"`
	APIFormat    *string           `json:"api_format"`
	KeyHeader    *string           `json:"key_header"`
	Headers      map[string]string `json:"headers"`
	UseProxy     *bool             `json:"use_proxy"`
	// TimeoutSeconds is present in the body when this connection sets its own
	// call wait, and null when it goes back to the global value.
	TimeoutSeconds appcatalog.OptionalInt `json:"timeout_seconds"`
	// RetryBackoff is present when this connection sets its own retry
	// windows, and null when it goes back to the global ones.
	RetryBackoff appcatalog.OptionalRetryBackoff `json:"retry_backoff"`
	// SwitchOn4xx and SwitchOn5xx fail this connection over on an unlisted
	// status of that class; absent leaves the stored choice alone.
	SwitchOn4xx *bool `json:"switch_on_4xx"`
	SwitchOn5xx *bool `json:"switch_on_5xx"`
}

type patchModelRequest struct {
	Enabled         *bool                   `json:"enabled"`
	PricedAs        *string                 `json:"priced_as"`
	Override        *modelOverrideRequest   `json:"override"`
	UpstreamModelID *string                 `json:"upstream_model_id"`
	ContextWindow   appcatalog.OptionalInt  `json:"context_window"`
	MaxOutput       appcatalog.OptionalInt  `json:"max_output"`
	Tools           appcatalog.OptionalBool `json:"tools"`
	Reasoning       appcatalog.OptionalBool `json:"reasoning"`
	Vision          appcatalog.OptionalBool `json:"vision"`
	Prices          *pricesRequest          `json:"prices"`
}

type addModelRequest struct {
	ModelID  string  `json:"model_id"`
	PricedAs *string `json:"priced_as"`
}

type cloneModelRequest struct {
	SourceModelID   string                `json:"source_model_id"`
	ModelID         string                `json:"model_id"`
	UpstreamModelID *string               `json:"upstream_model_id"`
	Override        *modelOverrideRequest `json:"override"`
}

type putGroupRequest struct {
	Label    string                    `json:"label"`
	Strategy string                    `json:"strategy"`
	Enabled  *bool                     `json:"enabled"`
	Listed   *bool                     `json:"listed"`
	Members  []groupMemberWriteRequest `json:"members"`
	// SwitchOn4xx and SwitchOn5xx move this route to its next member on an
	// unlisted status of that class; null is the shipped default, which does.
	SwitchOn4xx *bool `json:"switch_on_4xx"`
	SwitchOn5xx *bool `json:"switch_on_5xx"`
}

type groupMemberWriteRequest struct {
	ProviderID string `json:"provider_id"`
	ModelID    string `json:"model_id"`
	Kind       string `json:"kind"`
	Weight     int    `json:"weight"`
	Enabled    bool   `json:"enabled"`
}

type routeResponse struct {
	Model      string           `json:"model"`
	Kind       string           `json:"kind"`
	GroupID    string           `json:"group_id,omitempty"`
	Candidates []routeCandidate `json:"candidates"`
	Skipped    []routeCandidate `json:"skipped,omitempty"`
	// Warnings are the advisory mismatches the route accepted anyway, so a
	// preview says what the request asked for that the catalog doubts without
	// pretending the request cannot be served.
	Warnings []routeCandidate `json:"warnings,omitempty"`
}

type routeCandidate struct {
	ProviderID string          `json:"provider_id"`
	ModelID    string          `json:"model_id"`
	Eligible   bool            `json:"eligible"`
	Reason     string          `json:"reason,omitempty"`
	Warnings   []string        `json:"warnings,omitempty"`
	Prices     *pricesResponse `json:"prices,omitempty"`
}

func (s *Server) handleListAccounts(w http.ResponseWriter, r *http.Request) {
	accounts, ok := s.accountsOrUnavailable(w, r)
	if !ok {
		return
	}
	listed, err := accounts.Accounts(r.Context(), r.URL.Query().Get("provider"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, accountsResponse{Items: toAccountList(listed)})
}

func (s *Server) handleCreateAccount(w http.ResponseWriter, r *http.Request) {
	accounts, ok := s.accountsOrUnavailable(w, r)
	if !ok {
		return
	}
	manager, ok := s.catalogAPIOrUnavailable(w, r)
	if !ok {
		return
	}
	var request createAccountRequest
	if !s.decodeJSON(w, r, &request) {
		return
	}
	created, err := accounts.AddAccount(r.Context(), appaccount.NewAccount{
		ProviderID: request.ProviderID, Kind: request.Kind, Label: request.Label,
		SecretValue: request.Secret, Priority: request.Priority,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	s.fetchModels(manager, created.ProviderID)
	writeJSON(w, http.StatusCreated, toAccountResponse(created))
}

func (s *Server) handleGetAccount(w http.ResponseWriter, r *http.Request) {
	accounts, ok := s.accountsOrUnavailable(w, r)
	if !ok {
		return
	}
	found, err := accounts.Account(r.Context(), r.PathValue("id"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toAccountResponse(found))
}

func (s *Server) handlePatchAccount(w http.ResponseWriter, r *http.Request) {
	accounts, ok := s.accountsOrUnavailable(w, r)
	if !ok {
		return
	}
	var request patchAccountRequest
	if !s.decodeJSON(w, r, &request) {
		return
	}
	if err := patchAccount(r, accounts, r.PathValue("id"), request); err != nil {
		writeServiceError(w, err)
		return
	}
	updated, err := accounts.Account(r.Context(), r.PathValue("id"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toAccountResponse(updated))
}

func patchAccount(r *http.Request, accounts *appaccount.Service, id string, request patchAccountRequest) error {
	if request.Label != nil {
		if err := accounts.RenameAccount(r.Context(), id, *request.Label); err != nil {
			return err
		}
	}
	if request.Status == nil {
		return nil
	}
	switch *request.Status {
	case account.StatusActive:
		return accounts.ResumeAccount(r.Context(), id)
	case account.StatusPaused:
		return accounts.PauseAccount(r.Context(), id)
	default:
		return ErrUnknownStatus
	}
}

func (s *Server) handleDeleteAccount(w http.ResponseWriter, r *http.Request) {
	accounts, ok := s.accountsOrUnavailable(w, r)
	if !ok {
		return
	}
	if err := accounts.RemoveAccount(r.Context(), r.PathValue("id")); err != nil {
		writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListProviders(w http.ResponseWriter, r *http.Request) {
	manager, ok := s.catalogAPIOrUnavailable(w, r)
	if !ok {
		return
	}
	providers, err := manager.Providers(r.Context(), providerQuery(r))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, providersResponse{Items: toProviderList(providers), Total: len(providers)})
}

func providerQuery(r *http.Request) appcatalog.ProviderQuery {
	query := r.URL.Query()
	return appcatalog.ProviderQuery{
		Origin: query.Get("origin"), Auth: query.Get("auth"),
		Search: query.Get("q"), Configured: query.Get("configured") == "1",
	}
}

func (s *Server) handleGetProvider(w http.ResponseWriter, r *http.Request) {
	manager, ok := s.catalogAPIOrUnavailable(w, r)
	if !ok {
		return
	}
	provider, err := manager.Provider(r.Context(), r.PathValue("id"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toProviderResponse(provider))
}

func (s *Server) handlePatchProvider(w http.ResponseWriter, r *http.Request) {
	manager, ok := s.catalogAPIOrUnavailable(w, r)
	if !ok {
		return
	}
	var request patchProviderRequest
	if !s.decodeJSON(w, r, &request) {
		return
	}
	provider, err := manager.PatchProvider(r.Context(), r.PathValue("id"), appcatalog.ProviderPatch{
		Enabled: request.Enabled, Rank: request.Rank, PoolStrategy: request.PoolStrategy,
		BaseURL: request.BaseURL, Variables: request.Variables,
		Label: request.Label, APIFormat: request.APIFormat, KeyHeader: request.KeyHeader, Headers: request.Headers,
		UseProxy: request.UseProxy, TimeoutSeconds: request.TimeoutSeconds, RetryBackoff: request.RetryBackoff,
		SwitchOn4xx: request.SwitchOn4xx, SwitchOn5xx: request.SwitchOn5xx,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toProviderResponse(provider))
}

func (s *Server) handleDeleteProvider(w http.ResponseWriter, r *http.Request) {
	manager, ok := s.catalogAPIOrUnavailable(w, r)
	if !ok {
		return
	}
	if err := manager.DeleteProvider(r.Context(), r.PathValue("id")); err != nil {
		writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListCatalogModels(w http.ResponseWriter, r *http.Request) {
	manager, ok := s.catalogAPIOrUnavailable(w, r)
	if !ok {
		return
	}
	models, total, err := manager.Models(r.Context(), modelQuery(r))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, modelsResponse{Items: toModelList(models), Total: total})
}

func modelQuery(r *http.Request) appcatalog.ModelQuery {
	query := r.URL.Query()
	limit, _ := queryInt(r, "limit", defaultModelLimit)
	offset, _ := queryInt(r, "offset", 0)
	return appcatalog.ModelQuery{
		Provider: query.Get("provider"), Category: query.Get("category"),
		Origin: query.Get("origin"), Search: query.Get("q"),
		Available: query.Get("available"), Enabled: queryBool(query.Get("enabled")),
		Overridden: query.Get("overridden") == "true",
		Unpriced:   query.Get("unpriced") == "true",
		Configured: query.Get("configured") == "true",
		Limit:      limit, Offset: offset,
	}
}

func queryBool(raw string) *bool {
	if raw == "" {
		return nil
	}
	value := raw == "true" || raw == "1"
	return &value
}

func (s *Server) handleGetModel(w http.ResponseWriter, r *http.Request) {
	manager, ok := s.catalogAPIOrUnavailable(w, r)
	if !ok {
		return
	}
	model, err := manager.Model(r.Context(), r.PathValue("provider"), r.PathValue("model"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toModelResponse(model))
}

func (s *Server) handlePatchModel(w http.ResponseWriter, r *http.Request) {
	manager, ok := s.catalogAPIOrUnavailable(w, r)
	if !ok {
		return
	}
	var request patchModelRequest
	if !s.decodeJSON(w, r, &request) {
		return
	}
	model, err := manager.PatchModel(r.Context(), r.PathValue("provider"), r.PathValue("model"),
		appcatalog.ModelPatch{
			Enabled: request.Enabled, PricedAs: request.PricedAs,
			Override:        toModelOverrideInput(request.Override),
			UpstreamModelID: request.UpstreamModelID, ContextWindow: request.ContextWindow,
			MaxOutput: request.MaxOutput,
			Tools:     request.Tools, Reasoning: request.Reasoning, Vision: request.Vision,
			Prices: toPricesInput(request.Prices),
		})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toModelResponse(model))
}

func (s *Server) handleAddModel(w http.ResponseWriter, r *http.Request) {
	manager, ok := s.catalogAPIOrUnavailable(w, r)
	if !ok {
		return
	}
	var request addModelRequest
	if !s.decodeJSON(w, r, &request) {
		return
	}
	pricedAs := ""
	if request.PricedAs != nil {
		pricedAs = *request.PricedAs
	}
	model, err := manager.AddModel(r.Context(), r.PathValue("id"), request.ModelID, pricedAs)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toModelResponse(model))
}

func (s *Server) handleCloneModel(w http.ResponseWriter, r *http.Request) {
	manager, ok := s.catalogAPIOrUnavailable(w, r)
	if !ok {
		return
	}
	var request cloneModelRequest
	if !s.decodeJSON(w, r, &request) {
		return
	}
	model, err := manager.CloneModel(r.Context(), r.PathValue("id"), appcatalog.CloneRequest{
		SourceModelID:   request.SourceModelID,
		ModelID:         request.ModelID,
		UpstreamModelID: request.UpstreamModelID,
		Override:        toModelOverrideInput(request.Override),
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toModelResponse(model))
}

func (s *Server) handleDeleteModel(w http.ResponseWriter, r *http.Request) {
	manager, ok := s.catalogAPIOrUnavailable(w, r)
	if !ok {
		return
	}
	if err := manager.DeleteModel(r.Context(), r.PathValue("provider"), r.PathValue("model")); err != nil {
		writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListGroups(w http.ResponseWriter, r *http.Request) {
	manager, ok := s.catalogAPIOrUnavailable(w, r)
	if !ok {
		return
	}
	groups, err := manager.Groups(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, groupsResponse{Items: toGroupList(groups)})
}

func (s *Server) handleGetGroup(w http.ResponseWriter, r *http.Request) {
	manager, ok := s.catalogAPIOrUnavailable(w, r)
	if !ok {
		return
	}
	group, err := manager.Group(r.Context(), r.PathValue("id"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toGroupResponse(group))
}

func putGroupWrite(request putGroupRequest) approuting.Write {
	members := make([]approuting.MemberWrite, 0, len(request.Members))
	for _, member := range request.Members {
		members = append(members, approuting.MemberWrite{
			ProviderID: member.ProviderID, ModelID: member.ModelID, Kind: member.Kind,
			Weight: member.Weight, Enabled: member.Enabled,
		})
	}
	return approuting.Write{
		Label: request.Label, Strategy: request.Strategy,
		Enabled: boolOr(request.Enabled, true), Listed: boolOr(request.Listed, true),
		Members:     members,
		SwitchOn4xx: boolOr(request.SwitchOn4xx, true), SwitchOn5xx: boolOr(request.SwitchOn5xx, true),
	}
}

func (s *Server) handlePutGroup(w http.ResponseWriter, r *http.Request) {
	routes, ok := s.routesOrUnavailable(w, r)
	if !ok {
		return
	}
	var request putGroupRequest
	if !s.decodeJSON(w, r, &request) {
		return
	}
	if err := routes.Save(r.Context(), r.PathValue("id"), putGroupWrite(request)); err != nil {
		writeServiceError(w, err)
		return
	}
	manager, ok := s.catalogAPIOrUnavailable(w, r)
	if !ok {
		return
	}
	group, err := manager.Group(r.Context(), r.PathValue("id"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toGroupResponse(group))
}

func (s *Server) handleDeleteGroup(w http.ResponseWriter, r *http.Request) {
	routes, ok := s.routesOrUnavailable(w, r)
	if !ok {
		return
	}
	if err := routes.Delete(r.Context(), r.PathValue("id")); err != nil {
		writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func previewCandidate(candidate routing.Candidate) routeCandidate {
	prices := toPricesResponse(pricesFrom(candidate.Model.Prices))
	return routeCandidate{
		ProviderID: candidate.ProviderID, ModelID: candidate.ModelID,
		Eligible: true, Warnings: candidate.Warnings, Prices: &prices,
	}
}

func (s *Server) handleRoutePreview(w http.ResponseWriter, r *http.Request) {
	if s.opts.Router == nil {
		s.fail(w, r, refusal{Status: http.StatusServiceUnavailable,
			Code: "service_unavailable", Message: "api.service.unavailable"})
		return
	}
	plan, err := s.opts.Router.Plan(r.Context(), routing.Request{
		Model: r.URL.Query().Get("model"),
		Needs: catalog.Requirements{
			Images:       r.URL.Query().Get("images") == "true",
			Tools:        r.URL.Query().Get("tools") == "true",
			PromptTokens: queryInt64Value(r, "prompt_tokens"),
		},
	})
	response := routeResponse{Model: plan.Model, Kind: string(plan.Kind), GroupID: plan.GroupID}
	if err != nil {
		response.Skipped = routeSkipped(plan)
		writeJSON(w, http.StatusNotFound, response)
		return
	}
	for _, candidate := range plan.Candidates {
		response.Candidates = append(response.Candidates, previewCandidate(candidate))
	}
	response.Skipped = routeSkipped(plan)
	response.Warnings = routeWarnings(plan)
	writeJSON(w, http.StatusOK, response)
}

func routeSkipped(plan routing.Plan) []routeCandidate {
	skipped := make([]routeCandidate, 0, len(plan.Skipped))
	for _, entry := range plan.Skipped {
		skipped = append(skipped, routeCandidate{
			ProviderID: entry.ProviderID, ModelID: entry.ModelID, Reason: entry.Reason,
		})
	}
	return skipped
}

func routeWarnings(plan routing.Plan) []routeCandidate {
	warnings := make([]routeCandidate, 0, len(plan.Warnings))
	for _, entry := range plan.Warnings {
		warnings = append(warnings, routeCandidate{
			ProviderID: entry.ProviderID, ModelID: entry.ModelID, Reason: entry.Reason,
		})
	}
	return warnings
}

func (s *Server) handleUsage(w http.ResponseWriter, r *http.Request) {
	reads, ok := s.activityOrUnavailable(w, r)
	if !ok {
		return
	}
	query, err := usageQuery(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	groups, err := reads.Usage(r.Context(), query)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	archive, archiveFrom := s.usageArchive(r.Context(), reads, query)
	writeJSON(w, http.StatusOK, usageResponse{Items: groups, Archive: archive, ArchiveFromMs: archiveFrom})
}

func (s *Server) handleUsageSummary(w http.ResponseWriter, r *http.Request) {
	reads, ok := s.activityOrUnavailable(w, r)
	if !ok {
		return
	}
	query, err := usageQuery(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	summary, err := reads.UsageSummary(r.Context(), query)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	archive, archiveFrom := s.usageArchive(r.Context(), reads, query)
	writeJSON(w, http.StatusOK, usageSummaryResponse{
		Requests: summary.Requests, Errors: summary.Errors,
		InputTokens: summary.InputTokens, OutputTokens: summary.OutputTokens,
		CacheReadTokens: summary.CacheReadTokens, CacheWriteTokens: summary.CacheWriteTokens,
		CostMicros: summary.CostMicros, UnpricedRequests: summary.UnpricedRequests,
		DurationMs: summary.DurationMs, DurationMaxMs: summary.DurationMaxMs,
		Attempts: summary.Attempts, RetriedRequests: summary.RetriedRequests,
		Archive: archive, ArchiveFromMs: archiveFrom,
	})
}

func (s *Server) usageArchive(ctx context.Context, reads *appactivity.Service, query appactivity.UsageQuery) (bool, int64) {
	floor, found, err := reads.ArchiveFloor(ctx)
	if err != nil || !found || floor <= 0 {
		return false, 0
	}
	if query.SinceMs == 0 || query.SinceMs < floor {
		return true, floor
	}
	return false, 0
}

type usageSummaryResponse struct {
	Requests         int64 `json:"requests"`
	Errors           int64 `json:"errors"`
	InputTokens      int64 `json:"input_tokens"`
	OutputTokens     int64 `json:"output_tokens"`
	CacheReadTokens  int64 `json:"cache_read_tokens"`
	CacheWriteTokens int64 `json:"cache_write_tokens"`
	CostMicros       int64 `json:"cost_micros"`
	UnpricedRequests int64 `json:"unpriced_requests"`
	DurationMs       int64 `json:"duration_ms"`
	DurationMaxMs    int64 `json:"duration_max_ms"`
	Attempts         int64 `json:"attempts"`
	RetriedRequests  int64 `json:"retried_requests"`
	// Archive reports that the totals include whole days read from their
	// aggregate rather than from the request log.
	Archive       bool  `json:"archive"`
	ArchiveFromMs int64 `json:"archive_from_ms,omitempty"`
}

func (s *Server) handleQuotas(w http.ResponseWriter, r *http.Request) {
	report, ok := s.statusOrUnavailable(w, r)
	if !ok {
		return
	}
	windows, err := report.Quotas(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": s.quotaWindows(windows)})
}

func (s *Server) quotaWindows(windows []appstatus.QuotaWindow) []quotaWindowResponse {
	rendered := make([]quotaWindowResponse, 0, len(windows))
	for _, window := range windows {
		rendered = append(rendered, quotaWindowResponse{
			CredentialID: window.CredentialID, ConnectionID: window.ProviderID,
			Label: window.Label, Window: window.Window, Seconds: window.Seconds,
			UsedPercent: window.UsedPercent, ResetAtMs: window.ResetAtMs,
			Amount: window.Amount, Currency: window.Currency,
			Source: window.Source, ObservedAtMs: window.ObservedAtMs,
			Stale: s.quotaStale(window.CredentialID),
		})
	}
	return rendered
}

func (s *Server) quotaStale(credentialID string) bool {
	return s.opts.QuotaFreshness != nil && s.opts.QuotaFreshness.Stale(credentialID)
}

type quotaWindowResponse struct {
	CredentialID string   `json:"credential_id"`
	ConnectionID string   `json:"connection_id"`
	Label        string   `json:"label"`
	Window       string   `json:"window"`
	Seconds      int64    `json:"window_seconds"`
	UsedPercent  float64  `json:"used_percent"`
	Amount       *float64 `json:"amount,omitempty"`
	Currency     string   `json:"currency,omitempty"`
	ResetAtMs    int64    `json:"reset_at_ms"`
	Source       string   `json:"source"`
	ObservedAtMs int64    `json:"observed_at_ms"`
	Stale        bool     `json:"stale"`
}

func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	reads, ok := s.activityOrUnavailable(w, r)
	if !ok {
		return
	}
	query, err := logsQuery(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	page, err := reads.Logs(r.Context(), query)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	response := logsResponse{Items: page.Rows}
	if page.Next != nil {
		response.NextCursor = page.Next.Encode()
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) handleAttempts(w http.ResponseWriter, r *http.Request) {
	reads, ok := s.activityOrUnavailable(w, r)
	if !ok {
		return
	}
	eventID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		s.fail(w, r, refusal{
			Status: http.StatusBadRequest, Code: "bad_request", Message: "api.logs.event_id_number",
		})
		return
	}
	attempts, err := reads.Attempts(r.Context(), eventID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, attemptsResponse{Items: attempts})
}

func (s *Server) handleCaptures(w http.ResponseWriter, r *http.Request) {
	reads, ok := s.activityOrUnavailable(w, r)
	if !ok {
		return
	}
	eventID, ok := s.captureEventID(w, r)
	if !ok {
		return
	}
	captures, err := reads.Captures(r.Context(), eventID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	items := make([]captureResponse, 0, len(captures))
	for _, capture := range captures {
		items = append(items, captureResponse{
			ID: capture.ID, Kind: capture.Kind, Ordinal: capture.Ordinal,
			Method: capture.Method, URL: capture.URL, Status: capture.Status,
			Headers: capture.Headers, BodyBytes: capture.BodyBytes, Truncated: capture.Truncated,
		})
	}
	writeJSON(w, http.StatusOK, capturesResponse{Items: items})
}

func (s *Server) parseCaptureID(w http.ResponseWriter, r *http.Request) (int64, error) {
	captureID, err := strconv.ParseInt(r.PathValue("capture"), 10, 64)
	if err != nil {
		s.fail(w, r, refusal{
			Status: http.StatusBadRequest, Code: "bad_request", Message: "api.logs.capture_id_number",
		})
		return 0, err
	}
	return captureID, nil
}

func (s *Server) handleCaptureBody(w http.ResponseWriter, r *http.Request) {
	reads, ok := s.activityOrUnavailable(w, r)
	if !ok {
		return
	}
	eventID, ok := s.captureEventID(w, r)
	if !ok {
		return
	}
	captureID, err := s.parseCaptureID(w, r)
	if err != nil {
		return
	}
	body, err := reads.CaptureBody(r.Context(), eventID, captureID)
	if err != nil {
		s.failCaptureRead(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Relo-Truncated", strconv.FormatBool(body.Truncated))
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(body.Body); err != nil {
		s.opts.Logger.Warn("write a captured body", "error", err)
	}
}

func (s *Server) failCaptureRead(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, activity.ErrCaptureNotFound) {
		s.fail(w, r, refusal{
			Status: http.StatusNotFound, Code: "not_found", Message: "api.logs.capture_missing",
		})
		return
	}
	writeServiceError(w, err)
}

func (s *Server) captureEventID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	eventID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		s.fail(w, r, refusal{
			Status: http.StatusBadRequest, Code: "bad_request", Message: "api.logs.event_id_number",
		})
		return 0, false
	}
	return eventID, true
}

type capturesResponse struct {
	Items []captureResponse `json:"items"`
}

type captureResponse struct {
	ID        int64               `json:"id"`
	Kind      string              `json:"kind"`
	Ordinal   *int                `json:"ordinal,omitempty"`
	Method    string              `json:"method"`
	URL       string              `json:"url"`
	Status    int                 `json:"status"`
	Headers   map[string][]string `json:"headers"`
	BodyBytes int64               `json:"body_bytes"`
	Truncated bool                `json:"truncated"`
}

func (s *Server) handleUsageMetrics(w http.ResponseWriter, r *http.Request) {
	settings, ok := s.settingsOrUnavailable(w, r)
	if !ok {
		return
	}
	metrics, err := settings.UsageMetrics()
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, usageMetricsBody(metrics))
}

func (s *Server) handleSaveUsageMetrics(w http.ResponseWriter, r *http.Request) {
	settings, ok := s.settingsOrUnavailable(w, r)
	if !ok {
		return
	}
	var request struct {
		Metrics []string `json:"metrics"`
	}
	if !s.decodeJSON(w, r, &request) {
		return
	}
	metrics, err := settings.SaveUsageMetrics(request.Metrics)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, usageMetricsBody(metrics))
}

type usageMetricsResponse struct {
	Metrics   []string `json:"metrics"`
	Available []string `json:"available"`
	Min       int      `json:"min"`
	Max       int      `json:"max"`
}

func usageMetricsBody(metrics []string) usageMetricsResponse {
	return usageMetricsResponse{
		Metrics: metrics, Available: config.UsageMetricIDs(),
		Min: config.MinOverviewMetrics, Max: config.MaxOverviewMetrics,
	}
}

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	settings, ok := s.settingsOrUnavailable(w, r)
	if !ok {
		return
	}
	stored, err := settings.Read(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	pending, err := settings.RestartPending(*s.opts.Config)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	stored.RestartPending = pending
	writeJSON(w, http.StatusOK, toSettingsResponse(stored))
}

func (s *Server) handlePutRetention(w http.ResponseWriter, r *http.Request) {
	settings, ok := s.settingsOrUnavailable(w, r)
	if !ok {
		return
	}
	var request retentionSettingsRequest
	if !s.decodeJSON(w, r, &request) {
		return
	}
	saved, err := settings.SaveRetention(r.Context(), toRetentionSettings(request))
	if err != nil {
		s.writeRetentionError(w, r, err)
		return
	}
	s.cleanAfterSettingsSave(r.Context(), settings)
	writeJSON(w, http.StatusOK, toRetentionSettingsResponse(saved))
}

func (s *Server) handlePatchAccess(w http.ResponseWriter, r *http.Request) {
	settings, ok := s.settingsOrUnavailable(w, r)
	if !ok {
		return
	}
	var body struct {
		AllowExternal *bool `json:"allow_external"`
	}
	if !s.decodeJSON(w, r, &body) {
		return
	}
	if body.AllowExternal == nil {
		s.fail(w, r, refusal{
			Status: http.StatusBadRequest, Code: "bad_request", Message: "api.body.invalid",
		})
		return
	}
	if err := settings.SaveAccess(*body.AllowExternal); err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, body)
}

func (s *Server) handlePatchNetwork(w http.ResponseWriter, r *http.Request) {
	settings, ok := s.settingsOrUnavailable(w, r)
	if !ok {
		return
	}
	var request serverSettingsRequest
	if !s.decodeJSON(w, r, &request) {
		return
	}
	saved, err := settings.SaveNetwork(toServerSettings(request))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toServerSettingsResponse(saved))
}

func (s *Server) handlePatchProviders(w http.ResponseWriter, r *http.Request) {
	settings, ok := s.settingsOrUnavailable(w, r)
	if !ok {
		return
	}
	var request providersSettingsRequest
	if !s.decodeJSON(w, r, &request) {
		return
	}
	saved, err := settings.SaveProviders(toProvidersSettings(request))
	if err == nil {
		s.applyFailoverBackoff(saved.FailoverCooldownSeconds)
	}
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toProvidersSettingsResponse(saved))
}

func (s *Server) handlePatchSystem(w http.ResponseWriter, r *http.Request) {
	settings, ok := s.settingsOrUnavailable(w, r)
	if !ok {
		return
	}
	var request systemSettingsRequest
	if !s.decodeJSON(w, r, &request) {
		return
	}
	saved, err := settings.SaveSystem(toSystemSettings(request))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toSystemSettingsResponse(saved))
}

func (s *Server) cleanAfterSettingsSave(ctx context.Context, settings *appsettings.Service) {
	go func() {
		if _, err := settings.RunRetention(context.WithoutCancel(ctx)); err != nil && !errors.Is(err, activity.ErrRetentionBusy) {
			s.opts.Logger.Warn("clean the usage log after a settings save", "error", err)
		}
	}()
}

func (s *Server) handlePatchLanguage(w http.ResponseWriter, r *http.Request) {
	settings, ok := s.settingsOrUnavailable(w, r)
	if !ok {
		return
	}
	var body struct {
		Language string `json:"language"`
	}
	if !s.decodeJSON(w, r, &body) {
		return
	}
	if err := settings.SaveLanguage(body.Language); err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, body)
}

func (s *Server) handlePatchAppearance(w http.ResponseWriter, r *http.Request) {
	settings, ok := s.settingsOrUnavailable(w, r)
	if !ok {
		return
	}
	var patch appearancePatchRequest
	if !s.decodeJSON(w, r, &patch) {
		return
	}
	saved, err := settings.SaveAppearance(r.Context(), toAppearancePatch(patch))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toAppearanceSettingsResponse(saved))
}

func (s *Server) handlePreviewRetention(w http.ResponseWriter, r *http.Request) {
	settings, ok := s.settingsOrUnavailable(w, r)
	if !ok {
		return
	}
	var request retentionSettingsRequest
	if !s.decodeJSON(w, r, &request) {
		return
	}
	preview, err := settings.PreviewRetention(r.Context(), toRetentionSettings(request))
	if err != nil {
		s.writeRetentionError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, preview)
}

func (s *Server) handleRunRetention(w http.ResponseWriter, r *http.Request) {
	settings, ok := s.settingsOrUnavailable(w, r)
	if !ok {
		return
	}
	report, err := settings.RunRetention(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func (s *Server) handleDoctor(w http.ResponseWriter, r *http.Request) {
	report, ok := s.statusOrUnavailable(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, doctorResponse{Checks: toDoctorCheckList(report.Doctor(r.Context()))})
}

func (s *Server) fetchModels(manager *appcatalog.Service, providerID string) {
	if s.opts.Logger == nil {
		return
	}
	if _, err := manager.RefreshProviderModels(context.WithoutCancel(context.Background()), providerID); err != nil {
		s.opts.Logger.Debug("skip model refresh", "provider", providerID, "error", err)
	}
}

func (s *Server) handleListProviderTemplates(w http.ResponseWriter, r *http.Request) {
	manager, ok := s.catalogAPIOrUnavailable(w, r)
	if !ok {
		return
	}
	templatesList, mdState, err := manager.Templates(r.Context())
	if err != nil && s.opts.Logger != nil {
		s.opts.Logger.Debug("modelsdev fetch notice", "error", err)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":     toTemplateList(templatesList),
		"modelsdev": toModelsDevStateResponse(mdState),
	})
}

func (s *Server) handleProbeProvider(w http.ResponseWriter, r *http.Request) {
	manager, ok := s.catalogAPIOrUnavailable(w, r)
	if !ok {
		return
	}
	var req probeRequest
	if !s.decodeJSON(w, r, &req) {
		return
	}
	res, err := manager.ProbeProvider(r.Context(), toProbeInput(req))
	if err != nil {
		if errors.Is(err, appcatalog.ErrCredentialRejected) {
			writeJSON(w, http.StatusUnprocessableEntity, toProbeResultResponse(res))
			return
		}
		if errors.Is(err, appcatalog.ErrListingFailed) {
			writeJSON(w, http.StatusBadGateway, toProbeResultResponse(res))
			return
		}
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toProbeResultResponse(res))
}

func (s *Server) handleCommitProbe(w http.ResponseWriter, r *http.Request) {
	manager, ok := s.catalogAPIOrUnavailable(w, r)
	if !ok {
		return
	}
	var req commitProbeRequest
	if !s.decodeJSON(w, r, &req) {
		return
	}
	provider, err := manager.CommitProbe(r.Context(), r.PathValue("id"), toCommitProbeInput(req))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toProviderResponse(provider))
}

func (s *Server) handleDiscardProbe(w http.ResponseWriter, r *http.Request) {
	manager, ok := s.catalogAPIOrUnavailable(w, r)
	if !ok {
		return
	}
	manager.DiscardProbe(r.PathValue("id"))
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleRefreshQuota(w http.ResponseWriter, r *http.Request) {
	if s.opts.QuotaRefresh == nil {
		s.fail(w, r, refusal{
			Status: http.StatusServiceUnavailable, Code: "service_unavailable", Message: "api.service.unavailable",
		})
		return
	}
	windows, err := s.opts.QuotaRefresh.ProbeProvider(r.Context(), r.PathValue("id"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	s.publishSnapshots(windows)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleRefreshModels(w http.ResponseWriter, r *http.Request) {
	manager, ok := s.catalogAPIOrUnavailable(w, r)
	if !ok {
		return
	}
	res, err := manager.RefreshProviderModels(r.Context(), r.PathValue("id"))
	if err != nil {
		// A console reads the same two codes a probe reports: a refused
		// credential is the operator's to correct, anything else is upstream.
		switch {
		case errors.Is(err, appcatalog.ErrCredentialRejected):
			writeJSON(w, http.StatusUnprocessableEntity, toRefreshResultResponse(res))
			return
		case errors.Is(err, appcatalog.ErrListingFailed):
			writeJSON(w, http.StatusBadGateway, toRefreshResultResponse(res))
			return
		default:
			writeServiceError(w, err)
			return
		}
	}
	s.refreshConnectionQuota(r.Context(), r.PathValue("id"))
	writeJSON(w, http.StatusOK, toRefreshResultResponse(res))
}

func (s *Server) refreshConnectionQuota(ctx context.Context, providerID string) {
	if s.opts.QuotaRefresh == nil || providerID == "" {
		return
	}
	windows, err := s.opts.QuotaRefresh.ProbeProvider(ctx, providerID)
	if err != nil {
		s.opts.Logger.Warn("probe quota after a model refresh", "provider", providerID, "error", err)
		return
	}
	s.publishSnapshots(windows)
}

func (s *Server) handleToggleModels(w http.ResponseWriter, r *http.Request) {
	manager, ok := s.catalogAPIOrUnavailable(w, r)
	if !ok {
		return
	}
	var req struct {
		ModelIDs []string `json:"model_ids"`
		Enabled  bool     `json:"enabled"`
	}
	if !s.decodeJSON(w, r, &req) {
		return
	}
	if err := manager.ToggleModels(r.Context(), r.PathValue("id"), req.ModelIDs, req.Enabled); err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleSetModelsContext(w http.ResponseWriter, r *http.Request) {
	manager, ok := s.catalogAPIOrUnavailable(w, r)
	if !ok {
		return
	}
	var req struct {
		ModelIDs      []string `json:"model_ids"`
		ContextWindow *int64   `json:"context_window"`
	}
	if !s.decodeJSON(w, r, &req) {
		return
	}
	if err := manager.SetModelsContextWindow(r.Context(), r.PathValue("id"), req.ModelIDs, req.ContextWindow); err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleSetModelsCapabilities(w http.ResponseWriter, r *http.Request) {
	manager, ok := s.catalogAPIOrUnavailable(w, r)
	if !ok {
		return
	}
	var req struct {
		ModelIDs  []string                `json:"model_ids"`
		Tools     appcatalog.OptionalBool `json:"tools"`
		Reasoning appcatalog.OptionalBool `json:"reasoning"`
		Vision    appcatalog.OptionalBool `json:"vision"`
	}
	if !s.decodeJSON(w, r, &req) {
		return
	}
	if err := manager.SetModelsCapabilities(r.Context(), r.PathValue("id"), req.ModelIDs, appcatalog.CapabilityFlags{Tools: req.Tools, Reasoning: req.Reasoning, Vision: req.Vision}); err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleGetModelsDevState(w http.ResponseWriter, r *http.Request) {
	manager, ok := s.catalogAPIOrUnavailable(w, r)
	if !ok {
		return
	}
	st, _ := manager.ModelsDevState(r.Context())
	writeJSON(w, http.StatusOK, toModelsDevStateResponse(st))
}

func (s *Server) handleRefreshModelsDev(w http.ResponseWriter, r *http.Request) {
	manager, ok := s.catalogAPIOrUnavailable(w, r)
	if !ok {
		return
	}
	st, err := manager.RefreshModelsDev(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toPricesReportResponse(st))
}

func (s *Server) handleRefreshCatalog(w http.ResponseWriter, r *http.Request) {
	manager, ok := s.catalogAPIOrUnavailable(w, r)
	if !ok {
		return
	}
	result, err := manager.RefreshCatalog(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toCatalogRefreshResultResponse(result))
}

func (s *Server) handleSearchModelsDev(w http.ResponseWriter, r *http.Request) {
	manager, ok := s.catalogAPIOrUnavailable(w, r)
	if !ok {
		return
	}
	limit, err := queryInt(r, "limit", defaultModelsDevSearchLimit)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "limit must be a number")
		return
	}
	if limit < 1 {
		limit = defaultModelsDevSearchLimit
	}
	if limit > maxModelsDevSearchLimit {
		limit = maxModelsDevSearchLimit
	}
	rows, state := manager.SearchModelsDev(r.Context(), r.URL.Query().Get("q"), limit)
	writeJSON(w, http.StatusOK, modelsDevSearchResponse{Items: toModelsDevSearchHitList(rows), ModelsDev: toModelsDevStateResponse(state)})
}
