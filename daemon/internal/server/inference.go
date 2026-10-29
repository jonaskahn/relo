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
	"strconv"
	"time"

	"github.com/jonaskahn/relo/internal/account"
	"github.com/jonaskahn/relo/internal/activity"
	"github.com/jonaskahn/relo/internal/adapters/wire"
	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/internal/config"
	"github.com/jonaskahn/relo/internal/inference"
	"github.com/jonaskahn/relo/internal/routing"
)

const (
	requestIDBytes = 8
	// maxAttempts is how many sends a request with no pool to ask is costed
	// at, which is the widest walk one member could take.
	maxAttempts = 4
	// maxRequestAttempts ceilings how many sends one request may cost: every
	// account of every member, each with its retries. It keeps a long route
	// answering in a predictable time.
	maxRequestAttempts = 32
	// maxAccountsPerMember caps one member's share of the accounts, so a
	// connection with many accounts cannot consume the whole request.
	maxAccountsPerMember = 4
	// retryWaitLow and retryWaitHigh bound the random wait before an account
	// is retried, and retryWaitHigh is also the longest provider wait one
	// request retries through. A longer wait belongs to the account's cooldown:
	// the request moves to the next account or member instead of sitting on the
	// provider's clock.
	retryWaitLow  = time.Second
	retryWaitHigh = 3 * time.Second
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
		limit := s.captureLimit()
		captured := captureInbound(s.opts.CaptureRedactor, r, limit)
		// The answer is kept beside the request it answers, so the log holds
		// both halves of the exchange an operator debugs.
		answer := newAgentAnswer(w, s.opts.CaptureRedactor, limit)
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
		s.relay(relaySpec{writer: answer, request: r, surface: surface, inbound: inbound, canonical: request, origin: inference.OriginExternal, captured: captured, native: inboundNativeBody(inbound)})
	}
}

// inboundNativeBody returns the client body an inbound codec kept, which a
// routed attempt reuses when it speaks the client's own wire family.
func inboundNativeBody(inbound wire.InboundCodec) []byte {
	carrier, ok := inbound.(wire.NativeBodyCarrier)
	if !ok {
		return nil
	}
	return carrier.NativeBody()
}

type relaySpec struct {
	writer    http.ResponseWriter
	request   *http.Request
	surface   inferenceSurface
	inbound   wire.InboundCodec
	canonical *inference.Request
	origin    string
	captured  *CapturedMessage
	// native is the original client body a same-family surface decoded,
	// kept so a routed attempt can preserve its wire shape.
	native []byte
}

func (s *Server) relay(spec relaySpec) *requestOutcome {
	w, r, request := spec.writer, spec.request, spec.canonical
	outcome := &requestOutcome{
		requested: request.Model, surface: spec.surface.name,
		requestID: newRequestID(), started: time.Now(), request: request, origin: spec.origin,
		inbound: spec.captured, native: spec.native,
	}
	// A writer that keeps the answer belongs to traffic an agent sent, which
	// is exactly the traffic whose exchange the log shows.
	if answer, ok := w.(*agentAnswer); ok {
		outcome.answer = answer
	}
	s.traceAgentRequest(outcome)
	plan, err := s.opts.Router.Plan(r.Context(), routing.Request{
		Model: request.Model, Conversation: s.conversationAnchor(outcome, r), Needs: needsOf(request),
	})
	anthropic := spec.surface.name == inference.SurfaceMessages
	if err != nil {
		return s.relayUnrouted(spec, outcome, request, err, anthropic)
	}
	outcome.kind = plan.Kind
	outcome.groupID = plan.GroupID
	outcome.longContext = plan.LongContext
	outcome.nativeMillion = plan.NativeMillion
	s.send(spec, plan, outcome, anthropic)
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
	s         *Server
	w         http.ResponseWriter
	r         *http.Request
	plan      routing.Plan
	inbound   wire.InboundCodec
	outcome   *requestOutcome
	anthropic bool
	tried     map[string][]string
	budget    int
	attempts  int
	// walked counts the members this request actually asked an account of.
	walked         int
	cooldown       *routing.CoolingError
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
	// maxRetries is how many times one account of this member is retried.
	maxRetries int
	// account is the account being retried, and retries how many retries it
	// has had since the request first reached it.
	account string
	retries int
}

func (d *sendDriver) run() {
	for index, candidate := range d.plan.Candidates {
		if !d.runMember(candidate, index == len(d.plan.Candidates)-1) {
			return
		}
	}
	if d.walked == len(d.plan.Candidates) {
		// Every member answered without serving the request, which is what a
		// route remembers: the next request starts past the whole route.
		d.s.opts.Router.MarkGroupFailed(d.plan)
	}
	if d.attempts == 0 && d.cooldown != nil {
		d.s.failInference(d.w, d.r, d.cooldown, d.anthropic)
		d.s.record(context.WithoutCancel(d.r.Context()), d.outcome, Result{Status: http.StatusServiceUnavailable})
		return
	}
	if d.last == nil {
		d.last = skipCause(d.plan)
	}
	d.s.failInference(d.w, d.r, fmt.Errorf("%w: %v", routing.ErrNoRoute, d.last), d.anthropic)
	d.s.record(context.WithoutCancel(d.r.Context()), d.outcome, Result{Status: http.StatusServiceUnavailable})
}

func (d *sendDriver) runMember(candidate routing.Candidate, lastCandidate bool) bool {
	if until := d.s.connectionCooldown(candidate.ProviderID); !until.IsZero() {
		// A connection that just failed every account is not asked again
		// until its wait is over: the request goes to the next member.
		cooling := &routing.CoolingError{
			Name: candidate.ProviderID, Until: until,
			Reason: account.CoolingDown(candidate.ProviderID, until),
		}
		if d.cooldown == nil || until.Before(d.cooldown.Until) {
			d.cooldown = cooling
		}
		d.last = cooling
		return true
	}
	d.walked++
	// One member may spend one attempt per account it can reach, plus the
	// retries each of them is allowed. A member never spends more than that,
	// however much of the request's budget is left.
	allowance := d.s.candidateAllowance(candidate)
	walk := &memberWalk{maxRetries: d.s.retryAttempts(candidate.ProviderID)}
	limit := allowance * (1 + walk.maxRetries)
	for step := 0; step < limit && d.attempts < d.budget; step++ {
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
	spec := attemptSpec{
		request: d.r, candidate: candidate, inbound: d.inbound, outcome: d.outcome,
		// A retry stays on its account. A failure this account will be asked
		// again for is held back from its health until the retries are spent,
		// while a refusal a retry cannot cure is reported at once.
		prefer: walk.account,
		hold: func(result Result) bool {
			return retriesSameAccount(result) && walk.retries < walk.maxRetries &&
				retryCurable(result.Status)
		},
	}
	result, err := d.s.attempt(d.w, spec, d.tried, final)
	d.keepAttempt(candidate, result, err)
	if result.Status != 0 {
		d.upstreamStatus = result.Status
	}
	if err == nil {
		d.s.opts.Router.Record(candidate, result.HeaderLatency)
		d.s.opts.Router.Pin(d.s.conversationAnchor(d.outcome, d.r), routing.PinPrefix(d.plan), candidate)
		d.s.opts.Router.MarkSucceeded(candidate)
		d.s.opts.Router.MarkGroupSucceeded(d.plan, candidate)
		d.s.markConnectionServed(candidate.ProviderID, result.CredentialID)
		d.s.record(context.WithoutCancel(d.r.Context()), d.outcome, result)
		return walkAnswered
	}
	return d.handleAttemptFailure(failedAttempt{
		candidate: candidate, lastCandidate: lastCandidate, allowance: allowance, walk: walk, result: result,
	}, err)
}

// keepAttempt keeps one attempt's outcome with its request, and hands the
// exchange it had to the traffic log before the walk moves on.
func (d *sendDriver) keepAttempt(candidate routing.Candidate, result Result, err error) {
	attempt := attemptRow{
		providerID: candidate.ProviderID, modelID: candidate.ModelID,
		credential: result.Credential, credentialID: result.CredentialID,
		status: result.Status, code: attemptCode(err), capture: result.Capture,
	}
	d.outcome.attempts = append(d.outcome.attempts, attempt)
	d.s.traceAttempt(d.outcome, len(d.outcome.attempts)-1, attempt)
}

// failedAttempt is one refused attempt and where it sits in the walk, which is
// what decides whether its account is asked again or the request moves on.
type failedAttempt struct {
	candidate     routing.Candidate
	lastCandidate bool
	allowance     int
	walk          *memberWalk
	result        Result
}

func (d *sendDriver) handleAttemptFailure(failed failedAttempt, err error) walkAction {
	d.last = err
	d.rotateWire(failed)
	if retryAnotherCandidate(err) {
		return d.tryAnotherCandidate(failed)
	}
	if !failed.result.Retryable {
		d.recordSpentAccount(failed.candidate, failed.allowance, failed.result)
		return d.answerUnretryable(failed)
	}
	walk, result := failed.walk, failed.result
	walk.pending, walk.pendingErr, walk.attempted = result, err, true
	if d.retriesAccount(walk, result) {
		if !d.s.waitBeforeRetry(d.r.Context(), walk.retries, result.RetryAfter) {
			// The wait was cut short, so no attempt follows. Answer the
			// refusal the request already earned rather than leaving the
			// handler with an empty success when the caller is gone.
			d.s.answerFailed(d, d.outcome, result, err)
			return walkAnswered
		}
		walk.retries++
		return walkContinue
	}
	return d.leaveAccount(failed)
}

func (d *sendDriver) tryAnotherCandidate(failed failedAttempt) walkAction {
	credential := credentialKeyOf(failed.result)
	if credential != "" && !slices.Contains(d.tried[failed.candidate.ProviderID], credential) {
		d.tried[failed.candidate.ProviderID] = append(d.tried[failed.candidate.ProviderID], credential)
	}
	failed.walk.account, failed.walk.retries = "", 0
	if len(d.tried[failed.candidate.ProviderID]) < failed.allowance {
		return walkContinue
	}
	if failed.lastCandidate {
		d.s.answerFailed(d, d.outcome, failed.result, d.last)
		return walkAnswered
	}
	d.s.opts.Router.Unpin(d.s.conversationAnchor(d.outcome, d.r), routing.PinPrefix(d.plan))
	failed.walk.marked = true
	return walkNextMember
}

func retryAnotherCandidate(err error) bool {
	return errors.Is(err, errContextLimitExceeded) || errors.Is(err, wire.ErrUnsupportedFeature)
}

// rotateWire moves one refused conversation onto the next wire generation, so
// the next request rebuilds it. Cursor answers failed_precondition when a
// rebuilt state does not match what the conversation already holds; the same
// canonical history on a fresh conversation is what serves it.
func (d *sendDriver) rotateWire(failed failedAttempt) {
	if !failed.result.RotateWire {
		return
	}
	credential := failed.result.CredentialID
	if credential == "" {
		credential = credentialKeyOf(failed.result)
	}
	d.s.cursorWires.rotate(failed.candidate.ProviderID, credential, d.s.conversationAnchor(d.outcome, d.r))
}

// retriesAccount reports whether the failed account is asked again. A failure
// that says something about the account or the model, or that the connection's
// policy made retryable, is not cured by asking the same account again, and
// neither is a provider wait longer than one request may spend.
func (d *sendDriver) retriesAccount(walk *memberWalk, result Result) bool {
	if !retriesSameAccount(result) {
		return false
	}
	credential := credentialKeyOf(result)
	if walk.account != credential {
		walk.account, walk.retries = credential, 0
	}
	return walk.retries < walk.maxRetries
}

// retriesSameAccount reports whether a failure is one the retry budget would
// spend on the same account: a transient answer the account may survive, and a
// wait the provider named that fits inside one request. A longer wait is left
// to the account's cooldown so the answer is not held open on the provider's
// clock.
func retriesSameAccount(result Result) bool {
	if credentialKeyOf(result) == "" || result.ClassSwitch ||
		sameProviderRetry(result) && result.Status != http.StatusTooManyRequests {
		return false
	}
	return result.RetryAfter <= retryWaitHigh
}

func (d *sendDriver) answerUnretryable(failed failedAttempt) walkAction {
	walk, result := failed.walk, failed.result
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

// leaveAccount moves on from an account that failed its retries: to the next
// account of the connection, and once every account has answered, to the next
// member. The last member answers the failure itself.
func (d *sendDriver) leaveAccount(failed failedAttempt) walkAction {
	candidate, allowance, walk, result := failed.candidate, failed.allowance, failed.walk, failed.result
	credential := credentialKeyOf(result)
	if !slices.Contains(d.tried[candidate.ProviderID], credential) {
		d.tried[candidate.ProviderID] = append(d.tried[candidate.ProviderID], credential)
	}
	walk.account, walk.retries = "", 0
	if len(d.tried[candidate.ProviderID]) < allowance {
		return walkContinue
	}
	if connectionSpent(result) {
		d.s.markConnectionExhausted(candidate.ProviderID, result.RetryAfter)
	}
	d.s.markCandidateFailed(candidate, result)
	if failed.lastCandidate || !memberSwitchAllowed(d.plan, result) {
		d.s.answerFailed(d, d.outcome, result, d.last)
		return walkAnswered
	}
	d.s.opts.Router.Unpin(d.s.conversationAnchor(d.outcome, d.r), routing.PinPrefix(d.plan))
	walk.marked = true
	return walkNextMember
}

// recordSpentAccount records an account whose answer this request cannot
// retry, which is the last attempt it may spend. Without it a request that
// ends on the final attempt would never tell the connection that every
// account of it failed, and the connection would keep being walked.
func (d *sendDriver) recordSpentAccount(candidate routing.Candidate, allowance int, result Result) {
	credential := credentialKeyOf(result)
	if credential == "" || !connectionSpent(result) {
		return
	}
	if !slices.Contains(d.tried[candidate.ProviderID], credential) {
		d.tried[candidate.ProviderID] = append(d.tried[candidate.ProviderID], credential)
	}
	if len(d.tried[candidate.ProviderID]) < allowance {
		return
	}
	d.s.markConnectionExhausted(candidate.ProviderID, result.RetryAfter)
	d.s.markCandidateFailed(candidate, result)
}

// connectionSpent reports a failure that means the connection cannot serve
// right now: a rate limit or a server fault. A refusal about the request or
// the model says nothing about the connection, so it never parks it.
func connectionSpent(result Result) bool {
	return result.Status == 429 || result.Status >= 500
}

func (s *Server) connectionCooldown(providerID string) time.Time {
	if s.opts.Pools == nil {
		return time.Time{}
	}
	return s.opts.Pools.ConnectionCooldownUntil(providerID)
}

func (s *Server) markConnectionServed(providerID, credentialID string) {
	if s.opts.Pools != nil && credentialID != "" {
		s.opts.Pools.MarkServed(providerID, credentialID)
	}
}

func (s *Server) markConnectionExhausted(providerID string, retryAfter time.Duration) {
	if s.opts.Pools != nil {
		s.opts.Pools.MarkExhausted(providerID, retryAfter)
	}
}

func (s *Server) sendBudget(plan routing.Plan) int {
	total := 0
	for _, candidate := range plan.Candidates {
		total += s.candidateAllowance(candidate) * (1 + s.retryAttempts(candidate.ProviderID))
	}
	return min(max(total, 1), maxRequestAttempts)
}

func (s *Server) candidateAllowance(candidate routing.Candidate) int {
	if s.opts.Pools == nil {
		return maxAttempts
	}
	serving, _ := s.opts.Pools.Coverage(candidate.ProviderID, capabilityIDs(candidate.Model))
	return min(max(serving, 1), maxAccountsPerMember)
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

func (s *Server) retryAttempts(providerID string) int {
	global := config.DefaultUpstreamRetryAttempts
	if s.opts.Settings != nil {
		if stored, err := s.opts.Settings.UpstreamRetryAttempts(); err == nil {
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
	return retryAttemptsFor(host, global)
}

func retryAttemptsFor(host catalog.Provider, global int) int {
	if host.RetryAttempts != nil {
		return *host.RetryAttempts
	}
	return global
}

func randomDelay(low, high time.Duration) time.Duration {
	return low + time.Duration(mathrand.Int64N(int64(high-low)))
}

func (s *Server) waitBeforeRetry(ctx context.Context, retry int, retryAfter time.Duration) bool {
	delay := randomDelay(retryWaitLow, retryWaitHigh)
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
	if upstreamStatus == 0 && (errors.Is(cause, errContextLimitExceeded) || errors.Is(cause, wire.ErrUnsupportedFeature)) {
		status = failureStatus(cause)
	}
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
	var cooling *routing.CoolingError
	if errors.As(cause, &cooling) {
		w.Header().Set("Retry-After", strconv.Itoa(max(1, int(time.Until(cooling.Until).Seconds()+0.5))))
	}
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
	case errors.Is(cause, wire.ErrUnsupportedFeature):
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
	case errors.Is(cause, wire.ErrUnsupportedFeature):
		return "unsupported_capability"
	case errors.Is(cause, routing.ErrNoRoute), errors.Is(cause, catalog.ErrNoCatalog):
		return "no_route"
	case errors.Is(cause, routing.ErrModelUnconfigured):
		return "model_unconfigured"
	default:
		return "model_not_found"
	}
}

func (s *Server) estimateRequestTokens(request *inference.Request) int {
	if request == nil {
		return 0
	}
	// Base64 image payloads are never text a tokenizer can measure, so the
	// view drops them and each image is charged what a provider bills for one.
	view, images := inference.TextualView(request)
	body, err := json.Marshal(struct {
		Messages []inference.Message `json:"messages"`
		Tools    []inference.Tool    `json:"tools,omitempty"`
	}{Messages: view.Messages, Tools: view.Tools})
	if err != nil {
		return 0
	}
	tokens := int(catalog.PromptTokensFrom(len(body)))
	if s.contextTokenizer != nil {
		if counted, err := s.contextTokenizer.Count(string(body)); err == nil {
			tokens = counted
		}
	}
	return tokens + images*inference.ImagePromptTokens
}

func failureKind(cause error) routing.Kind {
	if errors.Is(cause, routing.ErrModelRequired) {
		return ""
	}
	return routing.Kind(failureCode(cause))
}

func needsOf(request *inference.Request) catalog.Requirements {
	needs := catalog.Requirements{Tools: len(request.Tools) > 0}
	characters, images := 0, 0
	for _, message := range request.Messages {
		for _, part := range message.Content {
			if part.Type == inference.ContentTypeImage {
				needs.Images = true
				images++
				continue
			}
			characters += len(part.Text)
		}
	}
	needs.PromptTokens = catalog.PromptTokensFrom(characters) + int64(images*inference.ImagePromptTokens)
	return needs
}

func conversationOf(r *http.Request) string {
	if r == nil {
		return ""
	}
	return wire.SessionAnchor(r.Header)
}

// conversationAnchor is the conversation identity one request speaks with. A
// client that named its conversation keeps it; one that did not is given a
// leased session held while it keeps asking, so a provider never sees a fresh
// identity per request. The lease is taken once per request, which is what
// keeps a retry and the attempt it follows on the one session.
func (s *Server) conversationAnchor(outcome *requestOutcome, r *http.Request) string {
	if named := conversationOf(r); named != "" {
		return named
	}
	if s.opts.SessionLeases == nil {
		return ""
	}
	if outcome.sessionLease == "" {
		keyID, _, _ := describeClient(r.Context())
		outcome.sessionLease = s.opts.SessionLeases.Lease(account.SessionKey{
			ClientKind: wire.ClientKind(r.Header), ClientKey: keyID,
		})
	}
	return outcome.sessionLease
}

func (s *Server) deviceID(accountID string) string {
	if accountID == "" || s.opts.DeviceSeed == "" {
		return ""
	}
	return account.ClaudeDeviceID(s.opts.DeviceSeed, accountID)
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
	// sessionLease is the synthetic session a request with no conversation
	// named was given, taken once so a retry stays on the same identity.
	sessionLease string
	// openCodeLane is the session a request with no conversation still sends
	// to OpenCode Go, allocated once so a retry stays on the same lane.
	openCodeLane string
	// origin names where the request came from, which the log shows: external
	// traffic a client sent, or internal traffic the console's tester ran.
	origin string
	// inbound is the request the agent sent, kept so the log can show the
	// bytes a client actually asked with.
	inbound *CapturedMessage
	// native is the original client body a same-family surface decoded. An
	// attempt the route keeps on the requested model reuses its wire shape
	// instead of rebuilding the body from the canonical request.
	native []byte
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
	ctx = s.recordContext(ctx)
	event := s.usageEvent(outcome, result)
	event.ClientKeyID, event.ClientKeyName, event.ClientApp = describeClient(ctx)
	event.Attempts = len(outcome.attempts)
	event.Retried = len(outcome.attempts) > 1
	s.publishUsage(event)
	// The answer is the last half of the exchange to exist, so the log takes it
	// here; the state rows follow behind the response.
	s.traceAgentAnswer(outcome)
	// The rows land behind the response, because a capture as large as the
	// request must not hold the handler open. A full queue runs the job here
	// instead of dropping it: the Logs page and the spend totals read these.
	s.bookkeeping.enqueueOrRun(func() {
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
	})
}

func (s *Server) recordUnreadable(r *http.Request, surface inferenceSurface, captured *CapturedMessage, answer *agentAnswer) {
	outcome := &requestOutcome{
		surface: surface.name, requestID: newRequestID(), started: time.Now(),
		origin: inference.OriginExternal, kind: routing.Kind("bad_request"), inbound: captured,
		answer: answer,
	}
	s.traceAgentRequest(outcome)
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
