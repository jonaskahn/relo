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
	// cursor is the exact Cursor tuple one cursor-agent attempt sends,
	// resolved after authorization chose the account.
	cursor *wire.CursorRequest
	// antigravity is the exact Cloud Code SKU one attempt sends, resolved
	// after authorization chose the account.
	antigravity *wire.AntigravityRequest
	// prefer is the account a retry stays on. hold reports a failure that
	// another retry of this account follows, so the account is not held to
	// account for it yet: a rate limit or server fault a retry may cure.
	prefer string
	hold   func(Result) bool
}

func (s *Server) attempt(w http.ResponseWriter, spec attemptSpec, tried map[string][]string, final bool) (Result, error) {
	host, authorized, err := s.resolveAttemptCandidate(spec, tried)
	if err != nil {
		return Result{}, err
	}
	spec.host, spec.authorized = host, authorized
	if err := s.resolveAttemptSelections(&spec, tried); err != nil {
		return Result{}, err
	}
	if err := s.resolveAttemptCapabilities(spec.request.Context(), &spec); err != nil {
		return Result{}, err
	}
	module, supported := s.opts.Formats.Outbound(spec.candidate.Model.APIFormat)
	if !supported {
		return Result{}, fmt.Errorf("%s: %w (%s)",
			spec.candidate.ProviderID, catalog.ErrUnsupportedFormat, spec.candidate.Model.APIFormat)
	}
	exchange, err := s.buildAttemptExchange(spec, module, final)
	if err != nil {
		return Result{CredentialID: spec.authorized.CredentialID}, err
	}
	s.announceAttempt(w, spec)
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

// resolveAttemptCapabilities overlays the selected credential's discovered
// facts and the selected provider tuple on the attempt-local model. Nothing
// here mutates the catalog snapshot shared by retries and route members.
func (s *Server) resolveAttemptCapabilities(ctx context.Context, spec *attemptSpec) error {
	if s.opts.Accounts != nil && spec.authorized.CredentialID != "" {
		effective, err := s.opts.Accounts.EffectiveModel(ctx, spec.candidate.ProviderID, spec.candidate.ModelID, spec.authorized.CredentialID)
		if err != nil {
			return err
		}
		spec.candidate.Model = effective
	}
	if spec.antigravity == nil {
		return nil
	}
	if spec.antigravity.Context > 0 {
		contextWindow := spec.antigravity.Context
		spec.candidate.Model.ContextWindow = &contextWindow
		spec.candidate.Model.MaxInput = &contextWindow
		spec.candidate.Model.FactSources.ContextWindow = "account"
		spec.candidate.Model.FactSources.MaxInput = "account"
	}
	if spec.antigravity.MaxOutput > 0 {
		maxOutput := int64(spec.antigravity.MaxOutput)
		spec.candidate.Model.MaxOutput = &maxOutput
		spec.candidate.Model.FactSources.MaxOutput = "account"
	}
	return nil
}

func (s *Server) resolveAttemptSelections(spec *attemptSpec, tried map[string][]string) error {
	if spec.candidate.Model.APIFormat == catalog.FormatCursorAgent {
		selection, err := s.selectCursorAttempt(spec.request.Context(), spec, tried)
		if err != nil {
			return err
		}
		spec.cursor = selection
	}
	if spec.candidate.Model.APIFormat != catalog.FormatCloudCode {
		return nil
	}
	selection, err := s.selectAntigravityAttempt(spec.request.Context(), spec, tried)
	if err != nil {
		return err
	}
	spec.antigravity = selection
	return nil
}

func (s *Server) reportAttemptAsync(spec attemptSpec, result Result) {
	// What the outcome says about the account, its model access, and its
	// quota is written behind the response, so the answer never waits on
	// those stores.
	candidate, authorized := spec.candidate, spec.authorized
	ctx := s.recordContext(spec.request.Context())
	held := spec.hold != nil && spec.hold(result)
	s.bookkeeping.enqueue(func() {
		if !held {
			s.reportCredential(ctx, candidate, authorized, result)
		}
		s.learnModelAccess(ctx, candidate, authorized, result)
		s.recordQuota(ctx, authorized.CredentialID, result.RateLimits)
	})
}

func (s *Server) resolveAttemptCandidate(spec attemptSpec, tried map[string][]string) (catalog.Provider, catalog.Authorization, error) {
	candidate := spec.candidate
	snapshot, found := s.opts.Catalog.Snapshot()
	if !found {
		return catalog.Provider{}, catalog.Authorization{}, catalog.ErrNoCatalog
	}
	host, found := snapshot.Provider(candidate.ProviderID)
	if !found {
		return catalog.Provider{}, catalog.Authorization{}, fmt.Errorf("%s: %w", candidate.ProviderID, appcatalog.ErrProviderNotFound)
	}
	authorized, err := s.opts.Relay.Authorize(spec.request.Context(), s.attemptAuthRequest(spec, host, tried))
	if err != nil {
		return catalog.Provider{}, catalog.Authorization{}, withCooldown(s.opts.Pools, candidate.ProviderID, err)
	}
	return host, authorized, nil
}

// attemptAuthRequest names the credential contract one attempt runs under: the
// connection's auth kind, where its key travels, and the cloud region and
// service a signer needs, beside the account selection routing made. Every
// attempt path builds it here so verification and inference agree. The host is
// the one the catalog read for this attempt, not the caller's copy of the spec,
// which names no connection until this attempt resolved one.
func (s *Server) attemptAuthRequest(spec attemptSpec, host catalog.Provider, tried map[string][]string) catalog.AuthRequest {
	candidate, outcome := spec.candidate, spec.outcome
	return catalog.AuthRequest{
		ProviderID: candidate.ProviderID, Auth: host.Auth,
		KeyHeader: host.KeyHeader, Region: host.Variables["region"], Service: host.Variables["service"],
		Selection: account.Selection{
			ConversationID: s.conversationAnchor(outcome, spec.request), Surface: string(outcome.kind),
			RequestID: outcome.requestID, Exclude: tried[candidate.ProviderID], Prefer: spec.prefer,
			Models: capabilityIDs(candidate.Model),
		},
	}
}

func (s *Server) announceAttempt(w http.ResponseWriter, spec attemptSpec) {
	outcome, candidate, authorized := spec.outcome, spec.candidate, spec.authorized
	outcome.providerID, outcome.modelID, outcome.credential = candidate.ProviderID, candidate.ModelID, authorized.Label
	// A candidate the catalog doubts is still sent, and the warning travels on
	// the answer so the caller learns what it asked for that the model may not
	// do. A retryable failure wrote nothing and clears the header again.
	warnings := candidate.Warnings
	if dropped := s.droppedCeilingWarning(spec); dropped != "" {
		warnings = append(append([]string(nil), warnings...), dropped)
	}
	if len(warnings) > 0 {
		w.Header().Set(warningHeader, strings.Join(warnings, "; "))
	}
	outcome.warnings = warnings
}

// droppedCeilingWarning names the output ceiling the attempt leaves out:
// the request states one and the connection's dialect has no parameter for
// it. The codec drops it so the turn can run; the warning keeps the drop
// from being silent.
func (s *Server) droppedCeilingWarning(spec attemptSpec) string {
	if spec.outcome == nil || spec.outcome.request == nil || spec.outcome.request.MaxTokens == 0 {
		return ""
	}
	template, templated := s.connectionTemplate(spec.host)
	if !templated || !template.RefusesMaxOutputTokens {
		return ""
	}
	return "the model ignores the max-output ceiling"
}

// nolint:funlen
func (s *Server) buildAttemptExchange(spec attemptSpec, module wire.CodecModule, final bool) (Exchange, error) {
	template, templated := s.connectionTemplate(spec.host)
	extra, err := s.attemptHeaders(spec, template, templated)
	if err != nil {
		return Exchange{}, err
	}
	request, originals := s.modelRequest(spec, template, templated)
	if err := s.validateOutboundRequest(spec, request); err != nil {
		return Exchange{}, err
	}
	exchange := s.attemptExchange(spec, attemptExchangeInputs{
		module: module, final: final, request: request, originals: originals, extra: extra,
		template: template, templated: templated,
	})
	s.attachAttemptHooks(&exchange, spec)
	return exchange, nil
}

type attemptExchangeInputs struct {
	module    wire.CodecModule
	final     bool
	request   *inference.Request
	originals map[string]string
	extra     map[string]string
	template  catalog.Template
	templated bool
}

func (s *Server) attemptExchange(spec attemptSpec, in attemptExchangeInputs) Exchange {
	candidate, outcome, host, authorized := spec.candidate, spec.outcome, spec.host, spec.authorized
	return Exchange{
		ProviderID: candidate.ProviderID, Codec: wire.RestoreToolNames(in.module, in.originals),
		CaptureLimit:   s.captureLimit(),
		RequiresStream: in.templated && in.template.RequiresStream,
		Options: s.attemptCodecOptions(spec, attemptCodecInputs{
			request: in.request, extra: in.extra, template: in.template, templated: in.templated,
		}),
		Request: in.request,
		Inbound: spec.inbound, Surface: outcome.surface, RequestID: outcome.requestID,
		CredentialLabel: authorized.Label, Final: in.final,
		Policy: connectionPolicy(host),
	}
}

func (s *Server) attachAttemptHooks(exchange *Exchange, spec attemptSpec) {
	candidate, outcome, host, authorized := spec.candidate, spec.outcome, spec.host, spec.authorized
	if host.Auth == catalog.AuthOAuth && authorized.CredentialID != "" {
		providerID, credentialID, rejected := candidate.ProviderID, authorized.CredentialID, authorized.Token
		exchange.OnUnauthorized = func(ctx context.Context) (catalog.Authorization, error) {
			return s.opts.Relay.RefreshRejected(ctx, providerID, credentialID, rejected)
		}
	}
	exchange.CallWait = s.callWait(host)
	if candidate.Model.APIFormat == catalog.FormatCursorAgent {
		anchor, providerID, credentialID := s.conversationAnchor(outcome, spec.request), candidate.ProviderID, authorized.CredentialID
		exchange.OnHandoff = func() { s.cursorWires.rotate(providerID, credentialID, anchor) }
	}
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
	lane := openCodeLane(outcome, r)
	inbound := s.freeLaneIdentity(r, host)
	// The genuine affinity travels as-is; only a conversation the client
	// never named gets the forged session kept per lane.
	if !wire.ValidSessionID(inbound.Affinity) {
		inbound.Session = s.freeLaneSession(lane)
	}
	return wire.OpenCodeFreeHeaders(lane, inbound), nil
}

// freeLaneIdentity reads what the inbound client said about itself, so the
// free lane relays the client's own identity instead of inventing one. The
// allowlist below is the whole inbound surface: the client key never travels
// upstream. An operator override fills the project only when the client sent
// none; otherwise the rotating forged id does.
func (s *Server) freeLaneIdentity(r *http.Request, host catalog.Provider) wire.InboundIdentity {
	inbound := wire.InboundIdentity{}
	if r == nil {
		return inbound
	}
	inbound.UserAgent = r.Header.Get("User-Agent")
	inbound.Affinity = firstHeader(r.Header,
		"x-session-affinity", "X-Session-Id", "x-opencode-session", "x-opencode-session-id")
	inbound.Project = r.Header.Get("x-opencode-project")
	inbound.ParentSessionID = firstHeader(r.Header,
		"x-opencode-parent-session-id", "x-parent-session-id")
	if inbound.Project == "" {
		inbound.Project = host.Headers["x-opencode-project"]
	}
	if inbound.Project == "" {
		inbound.Project = s.freeProject.Get()
	}
	return inbound
}

// firstHeader returns the first non-blank value among the names, which all
// name the same identity across client generations.
func firstHeader(header http.Header, names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(header.Get(name)); value != "" {
			return value
		}
	}
	return ""
}

// freeSessionCacheCap bounds the forged lane sessions one daemon keeps. A
// conversation holds its session while it is recent; an evicted lane simply
// forges a new one, losing stickiness but never correctness.
const freeSessionCacheCap = 4096

// freeLaneSession returns the forged session bound to one lane, minting and
// remembering it on first sight the way a native conversation start mints
// its session once and speaks on it until the conversation ends.
func (s *Server) freeLaneSession(lane string) string {
	s.freeMu.Lock()
	defer s.freeMu.Unlock()
	if session, ok := s.freeSessions[lane]; ok {
		return session
	}
	if len(s.freeSessions) >= freeSessionCacheCap {
		clear(s.freeSessions)
	}
	if s.freeSessions == nil {
		s.freeSessions = make(map[string]string, freeSessionCacheCap)
	}
	session := wire.NativeSessionID()
	s.freeSessions[lane] = session
	return session
}

type attemptCodecInputs struct {
	request   *inference.Request
	extra     map[string]string
	template  catalog.Template
	templated bool
}

// nolint:funlen
func (s *Server) attemptCodecOptions(spec attemptSpec, in attemptCodecInputs) wire.CodecOpts {
	candidate, outcome, host, authorized := spec.candidate, spec.outcome, spec.host, spec.authorized
	anchor := s.conversationAnchor(outcome, spec.request)
	return wire.CodecOpts{
		BaseURL:                endpointBase(candidate, authorized),
		CredentialRef:          authorized.Token,
		AuthMethod:             string(authorized.Auth),
		KeyHeader:              string(authorized.KeyHeader),
		Signer:                 authorized.Signer,
		Mode:                   s.opts.Formats.ModeFor(candidate.Model.APIFormat),
		Project:                authorized.Project,
		AccountID:              authorized.AccountID,
		DeviceID:               s.deviceID(authorized.AccountID),
		SessionID:              account.ClaudeSessionID(candidate.ProviderID, anchor),
		NativeBody:             s.nativeBody(outcome, candidate),
		RequestID:              outcome.requestID,
		SessionAnchor:          anchor,
		ExtraHeaders:           in.extra,
		RefusesMaxOutputTokens: in.templated && in.template.RefusesMaxOutputTokens,
		MaxOutput:              candidate.Model.MaxOutput,
		ReasoningEfforts:       candidate.Model.ReasoningEfforts,
		ReasoningToggle:        candidate.Model.ReasoningToggle,
		ReasoningBudget:        candidate.Model.ReasoningBudget,
		ReasoningBudgetMin:     candidate.Model.ReasoningBudgetMin,
		ReasoningBudgetMax:     candidate.Model.ReasoningBudgetMax,
		TemplateID:             host.TemplateID,
		ToolCallDialect:        host.ToolCallDialect,
		LongContext:            outcome.longContext,
		NativeMillion:          outcome.nativeMillion,
		UpstreamModel:          candidate.Model.UpstreamID,
		PromptTokens:           s.attemptPromptTokens(in.request, candidate.Model, in.templated && in.template.RefusesMaxOutputTokens),
		Cursor:                 spec.cursor,
		Antigravity:            spec.antigravity,
	}
}

func (s *Server) attemptPromptTokens(request *inference.Request, model catalog.Model, refusesOutput bool) int64 {
	if request == nil {
		return 0
	}
	reserved := request.MaxTokens
	if refusesOutput {
		reserved = 0
	} else if model.MaxOutput != nil && *model.MaxOutput > 0 {
		reserved = int(*model.MaxOutput)
	}
	return int64(s.estimateRequestTokens(request) + max(reserved, 0))
}

// nativeBody returns the client's own body when the attempt speaks the client's
// wire family, so an alias or a route member keeps the shape the client built
// instead of a rebuild that drops its native controls. Routing already decided
// that the candidate serves the requested model, so the two spellings never
// have to match.
func (s *Server) nativeBody(outcome *requestOutcome, candidate routing.Candidate) []byte {
	if outcome == nil || len(outcome.native) == 0 {
		return nil
	}
	if candidate.Model.APIFormat != catalog.FormatAnthropic {
		return nil
	}
	return outcome.native
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

// retryCurable reports a status a retry of the same account could turn into a
// completion: a rate limit, a server fault, or an attempt that never reached
// the upstream. A refusal about the account or the request is not one.
func retryCurable(status int) bool {
	return status == 0 || status == http.StatusTooManyRequests || status >= 500
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

// modelRequest returns the request one attempt sends, with the map that names
// each shortened tool again on the answer. The signed-out Zen lane corrects
// everything its gateway refuses; a paid OpenCode lane shares only the
// gateway's tool-name limit, so its request stays otherwise whole.
// nolint:funlen
func (s *Server) modelRequest(spec attemptSpec, template catalog.Template, templated bool) (*inference.Request, map[string]string) {
	copied := modelRequest(spec.outcome.request, spec.candidate.Model.UpstreamID)
	// A client that names no output ceiling leaves the upstream gateway its
	// own default — often a silent 32K. When no fact layer detected a ceiling
	// either, the wire carries the catalog default, so the turn matches the
	// ceiling /v1/models and the generated client configs publish. A model a
	// layer described keeps today's pass-through.
	if templated && template.RefusesMaxOutputTokens {
		copied.MaxTokens = 0
	} else if maxOutput := spec.candidate.Model.MaxOutput; maxOutput != nil && *maxOutput > 0 {
		copied.MaxTokens = int(*maxOutput)
	} else if copied.MaxTokens <= 0 && spec.candidate.Model.FactSources.MaxOutput == "" {
		copied.MaxTokens = int(catalog.DefaultMaxOutput)
	}
	if s.opts.Logger != nil && spec.outcome != nil && spec.outcome.request != nil &&
		spec.outcome.request.MaxTokens > 0 && copied.MaxTokens > 0 &&
		spec.outcome.request.MaxTokens != copied.MaxTokens {
		s.opts.Logger.Debug("normalized the output budget",
			"provider", spec.candidate.ProviderID, "model", spec.candidate.Model.ID,
			"requested", spec.outcome.request.MaxTokens, "effective", copied.MaxTokens,
			"source", spec.candidate.Model.FactSources.MaxOutput)
	}
	if spec.candidate.Model.APIFormat == catalog.FormatCloudCode && isClaudeAntigravityPassthrough(spec.candidate) {
		// Claude thinks through nothing on this transport, so a stated
		// effort never reaches the codec.
		copied.Reasoning = nil
	}
	if templated && template.ID == catalog.OpenCodeFreeTemplate {
		if s.openCodeFreeToolCutoff(spec) {
			seed := conversationOf(spec.request)
			if seed == "" && spec.outcome != nil {
				seed = spec.outcome.requestID
			}
			trimmed, originals := wire.OpenCodeFreeRequestWithToolCutoff(copied, spec.candidate.Model.APIFormat, seed)
			s.logToolCutoff(spec, copied, trimmed)
			return trimmed, originals
		}
		return wire.OpenCodeFreeRequest(copied, spec.candidate.Model.APIFormat)
	}
	if openCodeGatewayLane(spec.candidate, spec.authorized) {
		return wire.OpenCodeGatewayRequest(copied)
	}
	return copied, nil
}

// validateOutboundRequest rejects a request the selected account explicitly
// cannot serve. It never drops tools, images, or reasoning to make a turn fit.
// nolint:funlen
func (s *Server) validateOutboundRequest(spec attemptSpec, request *inference.Request) error {
	if request == nil {
		return nil
	}
	model := spec.candidate.Model
	if len(request.Tools) > 0 && model.SupportsTools != nil && !*model.SupportsTools {
		return fmt.Errorf("tools are not supported by %s: %w", model.ID, wire.ErrUnsupportedFeature)
	}
	if needsImages(request) && model.SupportsVision != nil && !*model.SupportsVision {
		return fmt.Errorf("images are not supported by %s: %w", model.ID, wire.ErrUnsupportedFeature)
	}
	if effort, stated := catalog.RequestEffort(request.Reasoning); stated && effort != "" {
		if model.SupportsReasoning != nil && !*model.SupportsReasoning {
			return fmt.Errorf("reasoning is not supported by %s: %w", model.ID, wire.ErrUnsupportedFeature)
		}
		if !effortKnown(model.ReasoningEfforts, effort) {
			return fmt.Errorf("reasoning effort %s is not supported by %s: %w", effort, model.ID, wire.ErrUnsupportedFeature)
		}
	}
	// The estimate is character-derived, so keep a conservative allowance for
	// multi-byte text while never using it to enlarge the provider's capacity.
	promptTokens := int64(s.estimateRequestTokens(request))
	if model.MaxInput != nil && promptTokens > *model.MaxInput {
		return &contextLimitError{Model: model.ID, Estimated: promptTokens, Limit: *model.MaxInput}
	}
	if model.MaxOutput != nil && *model.MaxOutput > 0 && request.MaxTokens > int(*model.MaxOutput) {
		request.MaxTokens = int(*model.MaxOutput)
	}
	if model.ContextWindow != nil && promptTokens > *model.ContextWindow {
		return &contextLimitError{Model: model.ID, Estimated: promptTokens, Limit: *model.ContextWindow}
	}
	if model.ContextWindow != nil && request.MaxTokens > 0 {
		available := *model.ContextWindow - promptTokens
		if available <= 0 {
			return &contextLimitError{Model: model.ID, Estimated: promptTokens, Limit: *model.ContextWindow}
		}
		if int64(request.MaxTokens) > available {
			request.MaxTokens = int(available)
		}
	}
	return nil
}

func needsImages(request *inference.Request) bool {
	for _, message := range request.Messages {
		for _, part := range message.Content {
			if part.Type == inference.ContentTypeImage {
				return true
			}
		}
	}
	return false
}

func effortKnown(ladder []string, effort string) bool {
	if ladder == nil {
		return true
	}
	if len(ladder) == 0 {
		return false
	}
	normalized, ok := inference.NormalizeEffort(effort)
	if !ok {
		return false
	}
	for _, listed := range ladder {
		if listed == normalized {
			return true
		}
	}
	return false
}

func (s *Server) openCodeFreeToolCutoff(spec attemptSpec) bool {
	if s.opts.TemplateSettings == nil || spec.request == nil {
		return true
	}
	setting, err := s.opts.TemplateSettings.Read(spec.request.Context(), spec.candidate.ProviderID)
	if err != nil {
		if s.opts.Logger != nil {
			s.opts.Logger.Warn("read the OpenCode free tool cutoff setting",
				"provider", spec.candidate.ProviderID, "error", err)
		}
		return true
	}
	return setting.ToolCutoff
}

func (s *Server) logToolCutoff(spec attemptSpec, before, after *inference.Request) {
	if s.opts.Logger == nil || before == nil || after == nil || len(after.Tools) >= len(before.Tools) {
		return
	}
	client := ""
	if spec.request != nil {
		client = wire.ClientKind(spec.request.Header)
	}
	s.opts.Logger.Info("trimmed tools for the OpenCode free gateway",
		"provider", spec.candidate.ProviderID, "client", client,
		"offered", len(before.Tools), "sent", len(after.Tools))
}

func openCodeGatewayLane(candidate routing.Candidate, authorized catalog.Authorization) bool {
	if candidate.ProviderID == catalog.OpenCodeGoTemplate {
		return true
	}
	return wire.OpenCodeGateway(endpointBase(candidate, authorized))
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
