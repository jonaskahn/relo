// Upstream attempts: executing one candidate and recording what it cost.
package server

import (
	"context"
	"errors"
	"fmt"
	"github.com/jonaskahn/relo/internal/inference"
	"net/http"
	"strings"
	"time"

	"github.com/jonaskahn/relo/internal/account"
	"github.com/jonaskahn/relo/internal/activity"
	"github.com/jonaskahn/relo/internal/adapters/wire"
	appcatalog "github.com/jonaskahn/relo/internal/application/catalog"
	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/internal/config"
	"github.com/jonaskahn/relo/internal/routing"
)

const warningHeader = "X-Relo-Warning"

func (s *Server) relayReady() bool {
	return s.opts.Relay != nil && s.opts.Router != nil &&
		s.opts.Catalog != nil
}

type attemptSpec struct {
	request    *http.Request
	candidate  routing.Candidate
	inbound    wire.InboundCodec
	outcome    *requestOutcome
	host       catalog.Provider
	authorized catalog.Authorization
}

func (s *Server) attempt(w http.ResponseWriter, spec attemptSpec, tried map[string][]string, final bool) (Result, error) {
	host, authorized, err := s.resolveAttemptCandidate(spec, tried)
	if err != nil {
		return Result{}, err
	}
	spec.host, spec.authorized = host, authorized
	module, supported := s.opts.Formats.Outbound(spec.candidate.Model.APIFormat)
	if !supported {
		return Result{}, fmt.Errorf("%s: %w (%s)",
			spec.candidate.ProviderID, catalog.ErrUnsupportedFormat, spec.candidate.Model.APIFormat)
	}
	announceAttempt(w, spec)
	exchange, err := s.buildAttemptExchange(spec, module, final)
	if err != nil {
		return Result{}, err
	}
	result, err := s.opts.Relay.Execute(s.proxyContext(spec.request, spec.host.UseProxy), exchange, w)
	result.CredentialID = spec.authorized.CredentialID
	if err != nil {
		// Nothing reached the client, so the warning belongs to no answer.
		w.Header().Del(warningHeader)
		spec.outcome.warnings = nil
	}
	s.reportAttemptAsync(spec, result)
	return result, err
}

func (s *Server) reportAttemptAsync(spec attemptSpec, result Result) {
	// What the outcome says about the account, its model access, and its
	// quota is written behind the response, so the answer never waits on
	// those stores.
	candidate, authorized := spec.candidate, spec.authorized
	ctx := context.WithoutCancel(spec.request.Context())
	s.bookkeeping.enqueue(func() {
		s.reportCredential(ctx, candidate, authorized, result)
		s.learnModelAccess(ctx, candidate, authorized, result)
		s.recordQuota(ctx, authorized.CredentialID, result.RateLimits)
	})
}

func (s *Server) resolveAttemptCandidate(spec attemptSpec, tried map[string][]string) (catalog.Provider, catalog.Authorization, error) {
	candidate, outcome := spec.candidate, spec.outcome
	snapshot, found := s.opts.Catalog.Snapshot()
	if !found {
		return catalog.Provider{}, catalog.Authorization{}, catalog.ErrNoCatalog
	}
	host, found := snapshot.Provider(candidate.ProviderID)
	if !found {
		return catalog.Provider{}, catalog.Authorization{}, fmt.Errorf("%s: %w", candidate.ProviderID, appcatalog.ErrProviderNotFound)
	}
	authorized, err := s.opts.Relay.Authorize(spec.request.Context(), catalog.AuthRequest{
		ProviderID: candidate.ProviderID, Auth: host.Auth,
		Selection: account.Selection{
			ConversationID: conversationOf(spec.request), Surface: string(outcome.kind),
			RequestID: outcome.requestID, Exclude: tried[candidate.ProviderID],
			Models: capabilityIDs(candidate.Model),
		},
	})
	if err != nil {
		return catalog.Provider{}, catalog.Authorization{}, withCooldown(s.opts.Pools, candidate.ProviderID, err)
	}
	return host, authorized, nil
}

func announceAttempt(w http.ResponseWriter, spec attemptSpec) {
	outcome, candidate, authorized := spec.outcome, spec.candidate, spec.authorized
	outcome.providerID, outcome.modelID, outcome.credential = candidate.ProviderID, candidate.ModelID, authorized.Label
	// A candidate the catalog doubts is still sent, and the warning travels on
	// the answer so the caller learns what it asked for that the model may not
	// do. A retryable failure wrote nothing and clears the header again.
	if len(candidate.Warnings) > 0 {
		w.Header().Set(warningHeader, strings.Join(candidate.Warnings, "; "))
	}
	outcome.warnings = candidate.Warnings
}

func (s *Server) buildAttemptExchange(spec attemptSpec, module wire.CodecModule, final bool) (Exchange, error) {
	candidate, outcome, host, authorized := spec.candidate, spec.outcome, spec.host, spec.authorized
	template, templated := s.connectionTemplate(host)
	extra, err := s.attemptHeaders(spec, template, templated)
	if err != nil {
		return Exchange{}, err
	}
	exchange := Exchange{
		ProviderID: candidate.ProviderID, Codec: module,
		RequiresStream: templated && template.RequiresStream,
		Options:        s.attemptCodecOptions(spec, extra, template, templated),
		Request:        modelRequest(outcome.request, candidate.Model.UpstreamID),
		Inbound:        spec.inbound, Surface: outcome.surface, RequestID: outcome.requestID,
		CredentialLabel: authorized.Label, Final: final,
		Policy: connectionPolicy(host),
	}
	if host.Auth == catalog.AuthOAuth && authorized.CredentialID != "" {
		providerID, credentialID, rejected := candidate.ProviderID, authorized.CredentialID, authorized.Token
		exchange.OnUnauthorized = func(ctx context.Context) (catalog.Authorization, error) {
			return s.opts.Relay.RefreshRejected(ctx, providerID, credentialID, rejected)
		}
	}
	exchange.CallWait = s.callWait(host)
	return exchange, nil
}

func (s *Server) attemptHeaders(spec attemptSpec, template catalog.Template, templated bool) (map[string]string, error) {
	r, candidate, outcome := spec.request, spec.candidate, spec.outcome
	host, authorized := spec.host, spec.authorized
	base := endpointBase(candidate, authorized)
	mode := s.opts.Formats.ModeFor(candidate.Model.APIFormat)
	extra := wire.WithOpenCodeGoSession(wire.MergeHeaders(host.Headers, authorized.Headers), base, mode, openCodeLane(outcome, r))
	if templated && template.ID == codexTemplateID {
		extra = wire.MergeHeaders(extra, codexIdentityHeaders(r))
	}
	if !templated || template.ID != catalog.OpenCodeFreeTemplate {
		return extra, nil
	}
	inboundSession := ""
	if r != nil {
		inboundSession = r.Header.Get("x-opencode-session")
	}
	free, headerErr := wire.OpenCodeFreeHeaders(openCodeLane(outcome, r), inboundSession)
	if headerErr != nil {
		return nil, headerErr
	}
	return free, nil
}

func (s *Server) attemptCodecOptions(spec attemptSpec, extra map[string]string, template catalog.Template, templated bool) wire.CodecOpts {
	candidate, outcome, host, authorized := spec.candidate, spec.outcome, spec.host, spec.authorized
	return wire.CodecOpts{
		BaseURL:                endpointBase(candidate, authorized),
		CredentialRef:          authorized.Token,
		AuthMethod:             string(authorized.Auth),
		Mode:                   s.opts.Formats.ModeFor(candidate.Model.APIFormat),
		Project:                authorized.Project,
		RequestID:              outcome.requestID,
		SessionAnchor:          conversationOf(spec.request),
		ExtraHeaders:           extra,
		RefusesMaxOutputTokens: templated && template.RefusesMaxOutputTokens,
		MaxOutput:              candidate.Model.MaxOutput,
		ReasoningEfforts:       candidate.Model.ReasoningEfforts,
		ReasoningToggle:        candidate.Model.ReasoningToggle,
		ReasoningBudget:        candidate.Model.ReasoningBudget,
		ReasoningBudgetMin:     candidate.Model.ReasoningBudgetMin,
		ReasoningBudgetMax:     candidate.Model.ReasoningBudgetMax,
		TemplateID:             host.TemplateID,
		LongContext:            outcome.longContext,
		NativeMillion:          outcome.nativeMillion,
	}
}

func codexIdentityHeaders(request *http.Request) map[string]string {
	headers := map[string]string{}
	if request == nil {
		return headers
	}
	for _, name := range []string{
		"thread-id",
		"x-codex-window-id",
		"x-codex-turn-metadata",
		"x-codex-turn-state",
		"x-models-etag",
		"x-openai-subagent",
		"x-codex-parent-thread-id",
		"x-openai-internal-codex-residency",
	} {
		if value := strings.TrimSpace(request.Header.Get(name)); value != "" {
			headers[name] = value
		}
	}
	return headers
}

func (s *Server) proxyContext(r *http.Request, use bool) context.Context {
	ctx := r.Context()
	if !use || s.opts.Settings == nil {
		return ctx
	}
	raw, err := s.opts.Settings.ProxyURL()
	if err != nil || raw == "" {
		return ctx
	}
	return config.WithOutboundProxy(ctx, raw)
}

func (s *Server) callWait(host catalog.Provider) time.Duration {
	seconds := config.DefaultUpstreamTimeoutSeconds
	if s.opts.Settings != nil {
		if stored, err := s.opts.Settings.UpstreamTimeout(); err == nil && stored > 0 {
			seconds = stored
		}
	}
	if host.TimeoutSeconds != nil && *host.TimeoutSeconds > 0 {
		seconds = *host.TimeoutSeconds
	}
	return time.Duration(seconds) * time.Second
}

func (s *Server) connectionTemplate(host catalog.Provider) (catalog.Template, bool) {
	if host.TemplateID == "" || s.opts.Templates == nil {
		return catalog.Template{}, false
	}
	return s.opts.Templates.Curated(host.TemplateID)
}

func (s *Server) recordQuota(ctx context.Context, credentialID string, windows []activity.WindowSample) {
	if s.opts.Quota == nil || credentialID == "" || len(windows) == 0 {
		return
	}
	if err := s.opts.Quota.Record(ctx, credentialID, windows); err != nil {
		s.opts.Logger.Warn("record the quota a response reported", "credential", credentialID, "error", err)
		return
	}
	if s.opts.QuotaFreshness != nil {
		s.opts.QuotaFreshness.NoteFresh(credentialID)
	}
	s.publishQuota(credentialID, windows)
}

func (s *Server) probeAccountQuota(ctx context.Context, credentialID string) {
	if s.opts.QuotaRefresh == nil || credentialID == "" {
		return
	}
	windows, err := s.opts.QuotaRefresh.ProbeCredential(ctx, credentialID)
	if err != nil {
		s.opts.Logger.Warn("probe quota after a login", "credential", credentialID, "error", err)
		return
	}
	s.publishSnapshots(windows)
}

func (s *Server) publishSnapshots(windows []activity.Snapshot) {
	grouped := map[string][]activity.WindowSample{}
	for _, window := range windows {
		grouped[window.CredentialID] = append(grouped[window.CredentialID], activity.WindowSample{
			Window: window.Window, UsedPercent: window.UsedPercent,
			ResetAt: window.ResetAt, Seconds: window.Seconds,
		})
	}
	for credentialID, samples := range grouped {
		s.publishQuota(credentialID, samples)
	}
}

func (s *Server) publishQuota(credentialID string, windows []activity.WindowSample) {
	if s.events == nil {
		return
	}
	_ = s.events.Publish(ChannelQuota, quotaLine{CredentialID: credentialID, Windows: quotaLines(windows)})
}

type quotaLine struct {
	CredentialID string            `json:"credential_id"`
	Windows      []quotaWindowLine `json:"windows"`
}

type quotaWindowLine struct {
	Window      string  `json:"window"`
	UsedPercent float64 `json:"used_percent"`
	ResetAtMs   int64   `json:"reset_at_ms"`
	Seconds     int64   `json:"window_seconds"`
}

func quotaLines(windows []activity.WindowSample) []quotaWindowLine {
	lines := make([]quotaWindowLine, 0, len(windows))
	for _, window := range windows {
		lines = append(lines, quotaWindowLine{
			Window: window.Window, UsedPercent: window.UsedPercent,
			ResetAtMs: window.ResetAt * 1000, Seconds: window.Seconds,
		})
	}
	return lines
}

func (s *Server) learnModelAccess(ctx context.Context, candidate routing.Candidate, authorized catalog.Authorization, result Result) {
	pools := s.opts.Pools
	if pools == nil || authorized.CredentialID == "" {
		return
	}
	if result.ModelAccess {
		if err := pools.LearnUnavailable(ctx, authorized.CredentialID, candidate.ModelID); err != nil {
			s.opts.Logger.Warn("record a model refusal", "credential", authorized.CredentialID,
				"model", candidate.ModelID, "error", err)
		}
		return
	}
	if result.Status < http.StatusOK || result.Status >= http.StatusMultipleChoices {
		return
	}
	if !pools.RosterKnown(authorized.CredentialID) {
		return
	}
	if pools.ModelIndex().CanServe(authorized.CredentialID, capabilityIDs(candidate.Model)) {
		return
	}
	if err := pools.LearnAvailable(ctx, authorized.CredentialID, candidate.ModelID); err != nil {
		s.opts.Logger.Warn("record a served model", "credential", authorized.CredentialID,
			"model", candidate.ModelID, "error", err)
	}
}

func capabilityIDs(model catalog.Model) []string {
	identifiers := make([]string, 0, 3)
	for _, id := range []string{model.ID, model.UpstreamID, model.ClonedFrom} {
		if id != "" {
			identifiers = append(identifiers, id)
		}
	}
	return identifiers
}

func (s *Server) reportCredential(ctx context.Context, candidate routing.Candidate, authorized catalog.Authorization, result Result) {
	// A completion that counted no tokens is the relay's own synthetic rate
	// limit: it says nothing about the account, and counting it would open
	// the breaker in the middle of the short retries.
	if result.ZeroTokens {
		return
	}
	err := s.opts.Relay.Report(ctx, Outcome{
		ProviderID: candidate.ProviderID, CredentialID: authorized.CredentialID,
		Status: result.Status, RetryAfter: result.RetryAfter, RefreshSettled: result.RefreshSettled,
	})
	if err != nil && s.opts.Logger != nil {
		s.opts.Logger.Warn("record a credential outcome", "error", err)
	}
}

func endpointBase(candidate routing.Candidate, authorized catalog.Authorization) string {
	if authorized.BaseURL != "" {
		return authorized.BaseURL
	}
	return candidate.Model.BaseURL
}

func modelRequest(request *inference.Request, modelID string) *inference.Request {
	copied := *request
	copied.Model = modelID
	return &copied
}

func withCooldown(pools *account.Manager, providerID string, err error) error {
	if pools == nil || !errors.Is(err, account.ErrNoCredentials) {
		return err
	}
	until := pools.CooldownUntil(providerID)
	if until.IsZero() {
		return err
	}
	return fmt.Errorf("%s: %w", account.CoolingDown(providerID, until), err)
}
