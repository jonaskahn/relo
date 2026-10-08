// Inference surfaces and relay dispatch to the chosen provider.
package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	mathrand "math/rand/v2"
	"net/http"
	"slices"
	"time"

	"github.com/jonaskahn/relo/internal/activity"
	"github.com/jonaskahn/relo/internal/adapters/wire"
	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/internal/config"
	"github.com/jonaskahn/relo/internal/inference"
	"github.com/jonaskahn/relo/internal/routing"
)

const (
	requestIDBytes = 8
	// maxAttempts floors how many sends one client request may cost, so a
	// lone candidate still gets its full retry schedule.
	maxAttempts = 4
	// maxRequestAttempts ceilings how many sends one request may cost when
	// its route has several members with accounts of their own, so a long
	// route still answers in a predictable time.
	maxRequestAttempts = 8
	// maxAccountsPerMember caps one member's share of the sends, so a
	// connection with many accounts cannot consume the whole request.
	maxAccountsPerMember  = 4
	contextLimitTolerance = 2_560
	// anthropicVersionHeader is the header an Anthropic client sends. It
	// repeats the codec's constant so a server built without a format
	// registry still answers the model list in the shape the client reads.
	anthropicVersionHeader = "anthropic-version"
)

var errContextLimitExceeded = errors.New("context length exceeded")

type inferenceSurface struct {
	name string
	path string
}

func chatCompletionsSurface() inferenceSurface {
	return inferenceSurface{name: inference.SurfaceChatCompletions, path: chatCompletionsPath}
}

func responsesSurface() inferenceSurface {
	return inferenceSurface{name: inference.SurfaceResponses, path: responsesPath}
}

func messagesSurface() inferenceSurface {
	return inferenceSurface{name: inference.SurfaceMessages, path: messagesPath}
}

func (s *Server) inboundFor(surface inferenceSurface) (wire.InboundCodec, bool) {
	if s.opts.Formats == nil {
		return nil, false
	}
	return s.opts.Formats.Inbound(surface.name)
}

func (s *Server) isAnthropicRequest(r *http.Request) bool {
	if r == nil {
		return false
	}
	if s.opts.Formats == nil {
		return r.Header.Get(anthropicVersionHeader) != ""
	}
	return s.opts.Formats.IsAnthropicRequest(r.Header)
}

func (s *Server) handleInference(surface inferenceSurface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.relayReady() {
			s.handleNotImplemented(w, r)
			return
		}
		if claudeLoginFrom(r.Context()) {
			s.serveClaudeLogin(w, r, surface)
			return
		}
		// The agent's own request is read before the codec consumes it, which
		// is what keeps a body Relo cannot decode readable in the log.
		captured := captureInbound(s.opts.CaptureRedactor, r)
		// The answer is kept beside the request it answers, so the log holds
		// both halves of the exchange an operator debugs.
		answer := newAgentAnswer(w, s.opts.CaptureRedactor)
		inbound, found := s.inboundFor(surface)
		if !found {
			s.handleNotImplemented(w, r)
			return
		}
		request, err := inbound.DecodeRequest(r)
		if err != nil {
			writeError(answer, http.StatusBadRequest, "bad_request", err.Error())
			s.recordUnreadable(r, surface, captured, answer)
			return
		}
		s.relay(relaySpec{writer: answer, request: r, surface: surface, inbound: inbound, canonical: request, origin: inference.OriginExternal, captured: captured})
	}
}

type relaySpec struct {
	writer    http.ResponseWriter
	request   *http.Request
	surface   inferenceSurface
	inbound   wire.InboundCodec
	canonical *inference.Request
	origin    string
	captured  *CapturedMessage
}

func (s *Server) relay(spec relaySpec) *requestOutcome {
	w, r, request := spec.writer, spec.request, spec.canonical
	outcome := &requestOutcome{
		requested: request.Model, surface: spec.surface.name,
		requestID: newRequestID(), started: time.Now(), request: request, origin: spec.origin,
		inbound: spec.captured,
	}
	// A writer that keeps the answer belongs to traffic an agent sent, which
	// is exactly the traffic whose exchange the log shows.
	if answer, ok := w.(*agentAnswer); ok {
		outcome.answer = answer
	}
	plan, err := s.opts.Router.Plan(r.Context(), routing.Request{
		Model: request.Model, Conversation: conversationOf(r), Needs: needsOf(request),
	})
	anthropic := spec.surface.name == inference.SurfaceMessages
	if err != nil {
		return s.relayUnrouted(spec, outcome, request, err, anthropic)
	}
	if s.contextLimitExceeded(plan, request) {
		return s.relayOverLimit(spec, outcome, request, plan, anthropic)
	}
	outcome.kind = plan.Kind
	outcome.groupID = plan.GroupID
	outcome.longContext = plan.LongContext
	outcome.nativeMillion = plan.NativeMillion
	s.send(spec, plan, outcome, anthropic)
	return outcome
}

func (s *Server) relayOverLimit(spec relaySpec, outcome *requestOutcome, request *inference.Request, plan routing.Plan, anthropic bool) *requestOutcome {
	w, r := spec.writer, spec.request
	err := fmt.Errorf("%w: the default model accepts at most %d tokens; use its 1M entry for a larger request",
		errContextLimitExceeded, plan.ContextLimit)
	outcome.kind = failureKind(err)
	s.failInference(w, r, err, anthropic)
	s.record(context.WithoutCancel(r.Context()), outcome, Result{Status: failureStatus(err)})
	return outcome
}

func (s *Server) relayUnrouted(spec relaySpec, outcome *requestOutcome, request *inference.Request, err error, anthropic bool) *requestOutcome {
	w, r := spec.writer, spec.request
	outcome.kind = failureKind(err)
	if s.opts.Logger != nil {
		s.opts.Logger.Warn("request has no route", "model", request.Model, "error", err)
	}
	s.failInference(w, r, err, anthropic)
	s.record(context.WithoutCancel(r.Context()), outcome, Result{Status: failureStatus(err)})
	return outcome
}

func (s *Server) send(spec relaySpec, plan routing.Plan, outcome *requestOutcome, anthropic bool) {
	driver := &sendDriver{
		s: s, w: spec.writer, r: spec.request, plan: plan, inbound: spec.inbound,
		outcome: outcome, anthropic: anthropic,
		tried:  map[string][]string{},
		budget: s.sendBudget(plan),
	}
	driver.run()
}

type sendDriver struct {
	s              *Server
	w              http.ResponseWriter
	r              *http.Request
	plan           routing.Plan
	inbound        wire.InboundCodec
	outcome        *requestOutcome
	anthropic      bool
	tried          map[string][]string
	budget         int
	attempts       int
	last           error
	upstreamStatus int
}

type walkAction int

const (
	walkContinue walkAction = iota
	walkNextMember
	walkAnswered
)

type memberWalk struct {
	pending    Result
	pendingErr error
	attempted  bool
	marked     bool
}

func (d *sendDriver) run() {
	for index, candidate := range d.plan.Candidates {
		if !d.runMember(candidate, index == len(d.plan.Candidates)-1) {
			return
		}
	}
	if d.last == nil {
		d.last = skipCause(d.plan)
	}
	d.s.failInference(d.w, d.r, fmt.Errorf("%w: %v", routing.ErrNoRoute, d.last), d.anthropic)
	d.s.record(context.WithoutCancel(d.r.Context()), d.outcome, Result{Status: http.StatusServiceUnavailable})
}

func (d *sendDriver) runMember(candidate routing.Candidate, lastCandidate bool) bool {
	allowance := d.s.candidateAllowance(candidate)
	limit := maxAccountsPerMember
	if lastCandidate {
		// The last member spends whatever is left, which is how a lone
		// candidate keeps its full retry schedule.
		limit = d.budget - d.attempts
	}
	walk := &memberWalk{}
	for account := 0; account < limit && d.attempts < d.budget; account++ {
		action := d.stepAttempt(candidate, lastCandidate, allowance, walk)
		if action == walkAnswered {
			return false
		}
		if action == walkNextMember {
			break
		}
	}
	if walk.attempted && !walk.marked {
		d.s.markCandidateFailed(candidate, walk.pending)
	}
	return true
}

func (d *sendDriver) stepAttempt(candidate routing.Candidate, lastCandidate bool, allowance int, walk *memberWalk) walkAction {
	// An attempt is final only when no attempt follows it: the last
	// send this request may spend. Anything earlier reports its
	// failure only to the walk, so the next account or member still
	// gets a chance to serve the request.
	final := d.attempts == d.budget-1
	d.attempts++
	result, err := d.s.attempt(d.w, attemptSpec{request: d.r, candidate: candidate, inbound: d.inbound, outcome: d.outcome}, d.tried, final)
	d.outcome.attempts = append(d.outcome.attempts, attemptRow{
		providerID: candidate.ProviderID, modelID: candidate.ModelID,
		credential: result.Credential, credentialID: result.CredentialID,
		status: result.Status, code: attemptCode(err), capture: result.Capture,
	})
	if result.Status != 0 {
		d.upstreamStatus = result.Status
	}
	if err == nil {
		d.s.opts.Router.Record(candidate, result.HeaderLatency)
		d.s.opts.Router.Pin(conversationOf(d.r), routing.PinPrefix(d.plan), candidate)
		d.s.opts.Router.MarkSucceeded(candidate)
		d.s.record(context.WithoutCancel(d.r.Context()), d.outcome, result)
		return walkAnswered
	}
	return d.handleAttemptFailure(candidate, lastCandidate, allowance, walk, result, err)
}

func (d *sendDriver) handleAttemptFailure(candidate routing.Candidate, lastCandidate bool, allowance int, walk *memberWalk, result Result, err error) walkAction {
	d.last = err
	if !result.Retryable {
		return d.answerUnretryable(walk, result)
	}
	walk.pending, walk.pendingErr, walk.attempted = result, err, true
	if !d.s.waitBeforeRetry(d.r.Context(), d.s.retryWindows(candidate.ProviderID), d.attempts-1, result.RetryAfter) {
		d.s.record(context.WithoutCancel(d.r.Context()), d.outcome, result)
		return walkAnswered
	}
	return d.routeRetryableFailure(candidate, lastCandidate, allowance, walk, result)
}

func (d *sendDriver) answerUnretryable(walk *memberWalk, result Result) walkAction {
	// An attempt that never reached the upstream has no failure
	// of its own: when an earlier attempt did, the client reads
	// that failure instead of the selection error. A retry that
	// lands back on the account it just cooled would otherwise
	// answer with the cooldown rather than the refusal that
	// caused it.
	if result.Status == 0 && walk.attempted && walk.pending.Status != 0 {
		d.s.answerFailed(d, d.outcome, walk.pending, walk.pendingErr)
		return walkAnswered
	}
	d.s.answerFailed(d, d.outcome, result, d.last)
	return walkAnswered
}

func (d *sendDriver) routeRetryableFailure(candidate routing.Candidate, lastCandidate bool, allowance int, walk *memberWalk, result Result) walkAction {
	if result.ClassSwitch {
		return d.handleClassSwitch(candidate, lastCandidate, walk, result)
	}
	if sameProviderRetry(result) {
		return d.handleSameProviderRetry(candidate, lastCandidate, allowance, walk, result)
	}
	return d.handleMemberFallback(candidate, lastCandidate, allowance, walk, result)
}

func (d *sendDriver) handleMemberFallback(candidate routing.Candidate, lastCandidate bool, allowance int, walk *memberWalk, result Result) walkAction {
	d.s.opts.Router.Unpin(conversationOf(d.r), routing.PinPrefix(d.plan))
	if lastCandidate {
		// The last member answers once every account it could serve
		// has answered, the way the same-provider path does: asking
		// a tried account again only walks it further down its
		// denylist.
		credential := credentialKeyOf(result)
		if !slices.Contains(d.tried[candidate.ProviderID], credential) {
			d.tried[candidate.ProviderID] = append(d.tried[candidate.ProviderID], credential)
		}
		if len(d.tried[candidate.ProviderID]) >= allowance {
			d.s.answerFailed(d, d.outcome, result, d.last)
			return walkAnswered
		}
		return walkContinue
	}
	d.s.markCandidateFailed(candidate, result)
	walk.marked = true
	return walkNextMember
}

func (d *sendDriver) handleClassSwitch(candidate routing.Candidate, lastCandidate bool, walk *memberWalk, result Result) walkAction {
	// A status the operator asked Relo to switch on reaches another
	// account of the same connection first: every account answers once,
	// and the request leaves the connection only when none of them can
	// serve it.
	credential := credentialKeyOf(result)
	if !slices.Contains(d.tried[candidate.ProviderID], credential) {
		d.tried[candidate.ProviderID] = append(d.tried[candidate.ProviderID], credential)
		return walkContinue
	}
	if lastCandidate || !memberSwitchAllowed(d.plan, result) {
		d.s.markCandidateFailed(candidate, result)
		d.s.answerFailed(d, d.outcome, result, d.last)
		return walkAnswered
	}
	d.s.opts.Router.Unpin(conversationOf(d.r), routing.PinPrefix(d.plan))
	d.s.markCandidateFailed(candidate, result)
	walk.marked = true
	return walkNextMember
}

func (d *sendDriver) handleSameProviderRetry(candidate routing.Candidate, lastCandidate bool, allowance int, walk *memberWalk, result Result) walkAction {
	credential := credentialKeyOf(result)
	if !slices.Contains(d.tried[candidate.ProviderID], credential) {
		d.tried[candidate.ProviderID] = append(d.tried[candidate.ProviderID], credential)
	}
	// Every account that could serve this member has answered:
	// the next member gets the send instead of one that just
	// failed again, and the last member answers the failure
	// instead of asking a tried account again. A second refusal
	// only walks the same account further down its denylist, so
	// the request after this one would meet the cooldown rather
	// than the account.
	if len(d.tried[candidate.ProviderID]) >= allowance {
		if lastCandidate {
			d.s.answerFailed(d, d.outcome, result, d.last)
			return walkAnswered
		}
		d.s.opts.Router.Unpin(conversationOf(d.r), routing.PinPrefix(d.plan))
		d.s.markCandidateFailed(candidate, result)
		walk.marked = true
		return walkNextMember
	}
	return walkContinue
}

func (s *Server) sendBudget(plan routing.Plan) int {
	total := 0
	for _, candidate := range plan.Candidates {
		total += s.candidateAllowance(candidate)
	}
	return min(max(total, maxAttempts), maxRequestAttempts)
}

func (s *Server) candidateAllowance(candidate routing.Candidate) int {
	if s.opts.Pools == nil {
		return maxAttempts
	}
	serving, _ := s.opts.Pools.Coverage(candidate.ProviderID, capabilityIDs(candidate.Model))
	return min(max(serving, 1), maxAccountsPerMember)
}

func retryDelay(windows [][2]int, retry int) time.Duration {
	if retry < 0 || retry >= len(windows) {
		return 0
	}
	window := windows[retry]
	low := time.Duration(window[0]) * time.Second
	high := time.Duration(window[1]) * time.Second
	if high <= low {
		return low
	}
	return randomDelay(low, high)
}

func (s *Server) applyFailoverBackoff(seconds int) {
	if seconds < 0 {
		return
	}
	steps := config.FailoverBackoff(seconds)
	if s.opts.Pools != nil {
		s.opts.Pools.SetFailoverBackoff(steps)
	}
	if s.opts.Router != nil {
		s.opts.Router.SetFailoverBackoff(steps)
	}
}

func (s *Server) retryWindows(providerID string) [][2]int {
	global := append([][2]int(nil), config.DefaultUpstreamRetryBackoff()...)
	if s.opts.Settings != nil {
		if stored, err := s.opts.Settings.UpstreamRetryBackoff(); err == nil && len(stored) > 0 {
			global = stored
		}
	}
	if s.opts.Catalog == nil {
		return global
	}
	snapshot, found := s.opts.Catalog.Snapshot()
	if !found {
		return global
	}
	host, ok := snapshot.Provider(providerID)
	if !ok {
		return global
	}
	return retryWindowsFor(host, global)
}

func retryWindowsFor(host catalog.Provider, global [][2]int) [][2]int {
	if len(host.RetryBackoff) > 0 {
		return host.RetryBackoff
	}
	return global
}

func randomDelay(low, high time.Duration) time.Duration {
	return low + time.Duration(mathrand.Int64N(int64(high-low)))
}

func (s *Server) waitBeforeRetry(ctx context.Context, windows [][2]int, retry int, retryAfter time.Duration) bool {
	delay := retryDelay(windows, retry)
	if s.opts.RetryDelay != nil {
		delay = s.opts.RetryDelay(retry)
	}
	if retryAfter > delay {
		delay = retryAfter
	}
	if delay <= 0 {
		return ctx.Err() == nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}

func (s *Server) writeUnwritten(d *sendDriver, upstreamStatus int, cause error) {
	// Leaving the handler without a write is an empty HTTP 200, which a
	// streaming client reads as a stream that ended before any event.
	status := answeredStatus(upstreamStatus)
	if d.anthropic {
		s.writeAnthropicError(d.w, status, cause.Error())
		return
	}
	s.fail(d.w, d.r, refusal{Status: status, Code: "upstream_error", Detail: cause})
}

func answeredStatus(upstreamStatus int) int {
	if upstreamStatus != 0 {
		return upstreamStatus
	}
	return http.StatusServiceUnavailable
}

func (s *Server) answerFailed(d *sendDriver, outcome *requestOutcome, result Result, cause error) {
	if !result.Wrote {
		s.writeUnwritten(d, d.upstreamStatus, cause)
		result.Status = answeredStatus(d.upstreamStatus)
	}
	s.record(context.WithoutCancel(d.r.Context()), outcome, result)
}

func sameProviderRetry(result Result) bool {
	// A model an account may not use is worth another account of the same
	// connection: the provider still serves the model, just not to this one.
	if result.ModelAccess {
		return true
	}
	switch result.Status {
	case http.StatusUnauthorized, http.StatusForbidden, http.StatusTooManyRequests:
		return true
	default:
		return false
	}
}

func (s *Server) markCandidateFailed(candidate routing.Candidate, result Result) {
	if s == nil || s.opts.Router == nil {
		return
	}
	if result.Status != 0 && result.Status != 429 && result.Status < 500 {
		return
	}
	s.opts.Router.MarkFailed(candidate)
}

func memberSwitchAllowed(plan routing.Plan, result Result) bool {
	if !result.ClassSwitch {
		return true
	}
	return routePolicy(plan).AllowsClass(result.Status)
}

func connectionPolicy(host catalog.Provider) Policy {
	return Policy{SwitchOn4xx: host.SwitchOn4xx, SwitchOn5xx: host.SwitchOn5xx}
}

func routePolicy(plan routing.Plan) Policy {
	policy := Policy{SwitchOn4xx: true, SwitchOn5xx: true}
	if plan.GroupSwitchOn4xx != nil {
		policy.SwitchOn4xx = *plan.GroupSwitchOn4xx
	}
	if plan.GroupSwitchOn5xx != nil {
		policy.SwitchOn5xx = *plan.GroupSwitchOn5xx
	}
	return policy
}

func credentialKeyOf(result Result) string {
	if result.CredentialID != "" {
		return result.CredentialID
	}
	return result.Credential
}

func (s *Server) failInference(w http.ResponseWriter, r *http.Request, cause error, anthropic bool) {
	status := failureStatus(cause)
	if anthropic {
		s.writeAnthropicError(w, status, cause.Error())
		return
	}
	s.fail(w, r, refusal{Status: status, Code: failureCode(cause), Detail: cause})
}

func (s *Server) writeAnthropicError(w http.ResponseWriter, status int, message string) {
	if status == 0 {
		status = http.StatusServiceUnavailable
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(s.anthropicFailure(status, message))
}

func (s *Server) anthropicFailure(status int, message string) []byte {
	if s.opts.Formats == nil {
		return nil
	}
	return s.opts.Formats.FailureDocument(status, message)
}

func skipCause(plan routing.Plan) error {
	if len(plan.Skipped) == 0 {
		return nil
	}
	return &skipFailure{reason: plan.Skipped[0].Reason}
}

// skipFailure carries a routing skip reason the vendor or the pools reported,
// keeping its words so the console reads what actually refused the request.
type skipFailure struct {
	reason string
}

func (e *skipFailure) Error() string {
	return e.reason
}

func failureStatus(cause error) int {
	switch {
	case errors.Is(cause, routing.ErrModelRequired):
		return http.StatusBadRequest
	case errors.Is(cause, errContextLimitExceeded):
		return http.StatusBadRequest
	case errors.Is(cause, routing.ErrNoRoute), errors.Is(cause, catalog.ErrNoCatalog):
		return http.StatusServiceUnavailable
	default:
		return http.StatusNotFound
	}
}

func failureCode(cause error) string {
	switch {
	case errors.Is(cause, routing.ErrModelRequired):
		return "model_required"
	case errors.Is(cause, errContextLimitExceeded):
		return "context_length_exceeded"
	case errors.Is(cause, routing.ErrNoRoute), errors.Is(cause, catalog.ErrNoCatalog):
		return "no_route"
	case errors.Is(cause, routing.ErrModelUnconfigured):
		return "model_unconfigured"
	default:
		return "model_not_found"
	}
}

func (s *Server) contextLimitExceeded(plan routing.Plan, request *inference.Request) bool {
	if plan.ContextLimit <= 0 || request == nil {
		return false
	}
	body, err := json.Marshal(struct {
		Messages []inference.Message `json:"messages"`
		Tools    []inference.Tool    `json:"tools,omitempty"`
	}{Messages: request.Messages, Tools: request.Tools})
	if err != nil {
		return false
	}
	tokens := int(catalog.PromptTokensFrom(len(body)))
	if s.contextTokenizer != nil {
		if counted, err := s.contextTokenizer.Count(string(body)); err == nil {
			tokens = counted
		}
	}
	return int64(tokens+max(request.MaxTokens, 0)) > plan.ContextLimit+contextLimitTolerance
}

func failureKind(cause error) routing.Kind {
	if errors.Is(cause, routing.ErrModelRequired) {
		return ""
	}
	return routing.Kind(failureCode(cause))
}

func needsOf(request *inference.Request) catalog.Requirements {
	needs := catalog.Requirements{Tools: len(request.Tools) > 0}
	characters := 0
	for _, message := range request.Messages {
		for _, part := range message.Content {
			if part.Type == inference.ContentTypeImage {
				needs.Images = true
				continue
			}
			characters += len(part.Text)
		}
	}
	needs.PromptTokens = catalog.PromptTokensFrom(characters)
	return needs
}

func conversationOf(r *http.Request) string {
	if r == nil {
		return ""
	}
	return wire.SessionAnchor(r.Header)
}

func openCodeLane(outcome *requestOutcome, r *http.Request) string {
	if lane := conversationOf(r); lane != "" {
		return lane
	}
	if outcome.openCodeLane != "" {
		return outcome.openCodeLane
	}
	outcome.openCodeLane = outcome.requestID
	if outcome.openCodeLane == "" {
		outcome.openCodeLane = newRequestID()
	}
	return outcome.openCodeLane
}

type requestOutcome struct {
	requested  string
	surface    string
	requestID  string
	kind       routing.Kind
	groupID    string
	providerID string
	modelID    string
	credential string
	started    time.Time
	request    *inference.Request
	attempts   []attemptRow
	// longContext reports that the caller named the million-token entry,
	// which the Claude subscription profile marks with its own beta.
	longContext bool
	// nativeMillion reports that the named entry is a million tokens on its own,
	// so the long-context beta is left off the upstream request.
	nativeMillion bool
	// warnings names what this request asked of the model that served it and
	// the catalog doubts, stored with the event so a log keeps the note.
	warnings []string
	// openCodeLane is the session a request with no conversation still sends
	// to OpenCode Go, allocated once so a retry stays on the same lane.
	openCodeLane string
	// origin names where the request came from, which the log shows: external
	// traffic a client sent, or internal traffic the console's tester ran.
	origin string
	// inbound is the request the agent sent, kept so the log can show the
	// bytes a client actually asked with.
	inbound *CapturedMessage
	// answer is the reply the agent read, kept so the log can show what Relo
	// answered the bytes it was asked with.
	answer *agentAnswer
}

type attemptRow struct {
	providerID string
	modelID    string
	credential string
	// credentialID names the stored account the attempt ran on, and capture
	// holds the request it sent and the reply it got.
	credentialID string
	status       int
	code         string
	capture      *Capture
}

func (s *Server) record(ctx context.Context, outcome *requestOutcome, result Result) {
	if s.opts.Usage == nil {
		return
	}
	event := s.usageEvent(outcome, result)
	event.ClientKeyID, event.ClientKeyName, event.ClientApp = describeClient(ctx)
	event.Attempts = len(outcome.attempts)
	event.Retried = len(outcome.attempts) > 1
	s.publishUsage(event)
	// The event and its day aggregate commit together: a finalize pass that
	// runs while a request finishes either recomputes the day from the row or
	// sees it arrive afterwards, never both.
	eventID, err := s.opts.Usage.AppendEvent(ctx, event)
	if err != nil {
		s.opts.Logger.Error("record usage event", "error", err)
		return
	}
	for ordinal, attempt := range outcome.attempts {
		if err := s.opts.Usage.AppendAttempt(ctx, activity.RequestAttempt{
			EventID: eventID, Ordinal: ordinal,
			Provider: attempt.providerID, Model: attempt.modelID,
			CredentialLabel: attempt.credential, CredentialID: attempt.credentialID,
			Status: attempt.status, ErrorCode: attempt.code,
		}); err != nil {
			s.opts.Logger.Error("record usage attempt", "error", err)
			return
		}
	}
	s.recordCaptures(ctx, eventID, outcome)
}

func (s *Server) recordUnreadable(r *http.Request, surface inferenceSurface, captured *CapturedMessage, answer *agentAnswer) {
	outcome := &requestOutcome{
		surface: surface.name, requestID: newRequestID(), started: time.Now(),
		origin: inference.OriginExternal, kind: routing.Kind("bad_request"), inbound: captured,
		answer: answer,
	}
	s.record(context.WithoutCancel(r.Context()), outcome, Result{Status: http.StatusBadRequest})
}

func (s *Server) usageEvent(outcome *requestOutcome, result Result) activity.RequestEvent {
	status := result.Status
	if status == 0 {
		status = http.StatusServiceUnavailable
	}
	cost := s.priceOf(outcome, result.Usage)
	origin := outcome.origin
	if !inference.OriginKnown(origin) {
		origin = inference.OriginExternal
	}
	return activity.RequestEvent{
		// The column is milliseconds everywhere it is read: the age cutoff,
		// the since/until filters, and the per-day grouping. Writing seconds
		// here made a date filter match nothing.
		RequestID: outcome.requestID, Timestamp: outcome.started.UnixMilli(),
		Provider: outcome.providerID, Model: outcome.modelID,
		RequestedModel: outcome.requested, GroupID: outcome.groupID,
		CredentialLabel: outcome.credential, CredentialID: result.CredentialID,
		Surface: outcome.surface, Status: status,
		Warnings:    outcome.warnings,
		Origin:      origin,
		DurationMs:  result.Duration.Milliseconds(),
		InputTokens: result.Usage.InputTokens, OutputTokens: result.Usage.OutputTokens,
		CacheReadTokens: result.Usage.CacheReadTokens, CacheWriteTokens: result.Usage.CacheWriteTokens,
		EstimatedCostMicros: cost.Micros,
		RouteProvider:       outcome.providerID, RouteReason: string(outcome.kind),
	}
}

func (s *Server) priceOf(outcome *requestOutcome, usage inference.UsageReport) catalog.Cost {
	snapshot, found := s.opts.Catalog.Snapshot()
	if !found {
		return catalog.Cost{Source: catalog.CostUnknown}
	}
	model, found := snapshot.Model(outcome.providerID, outcome.modelID)
	if !found {
		return catalog.Cost{Source: catalog.CostUnknown}
	}
	return catalog.CostOf(model.Prices, catalog.TokenCounts{
		Input:      int64(usage.InputTokens),
		Output:     int64(usage.OutputTokens),
		CacheRead:  int64(usage.CacheReadTokens),
		CacheWrite: int64(usage.CacheWriteTokens),
	})
}

func attemptCode(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func newRequestID() string {
	raw := make([]byte, requestIDBytes)
	if _, err := rand.Read(raw); err != nil {
		return "unknown"
	}
	return "req-" + hex.EncodeToString(raw)
}

func (s *Server) publishUsage(event activity.RequestEvent) {
	if s.events == nil {
		return
	}
	_ = s.events.Publish(ChannelLogs, logLine{
		RequestID: event.RequestID, Provider: event.Provider, Model: event.Model,
		Credential: event.CredentialLabel, Status: event.Status,
		InputTokens: event.InputTokens, OutputTokens: event.OutputTokens,
		Cost: activity.FormatMicrosPtr(event.EstimatedCostMicros), Client: event.ClientKeyName,
	})
}

type logLine struct {
	RequestID    string `json:"request_id"`
	Provider     string `json:"provider"`
	Model        string `json:"model"`
	Credential   string `json:"credential"`
	Status       int    `json:"status"`
	InputTokens  int    `json:"input_tokens"`
	OutputTokens int    `json:"output_tokens"`
	Cost         string `json:"cost"`
	Client       string `json:"client,omitempty"`
}
