// Package dispatch sends canonical requests upstream. It is the only package
// that holds a resolved credential secret while a request runs.
package upstream

import (
	"context"
	"errors"
	"fmt"
	"github.com/jonaskahn/relo/internal/inference"
	"log/slog"
	"maps"
	"net/http"
	"strconv"
	"time"

	"github.com/jonaskahn/relo/internal/activity"
	"github.com/jonaskahn/relo/internal/adapters/quota"
	"github.com/jonaskahn/relo/internal/adapters/wire"
	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/internal/contentencoding"
)

const (
	maxErrorBodyBytes = 64 << 10
	maxResponseBytes  = 64 << 20
)

// Relay errors name why an upstream attempt failed: the send, the vendor's
// refusal, the response read, and an answer without usage to bill.
var (
	ErrUpstreamFailed = errors.New("upstream request failed")
	ErrUpstreamStatus = errors.New("upstream rejected the request")
	ErrRelayFailed    = errors.New("relaying the upstream response failed")
	// ErrCodecStreamless reports a codec that cannot stream, naming the type
	// that was asked to open a stream it does not implement.
	ErrCodecStreamless = errors.New("provider codec")
	// ErrZeroTokens reports a 2xx whose collected usage was all zeros: the
	// provider ended the answer before reporting a completion.
	ErrZeroTokens = errors.New("the provider returned a completion with no token usage")
)

// vendorFailure carries a failure the vendor itself reported, keeping its
// words so the client reads what actually refused the request. The message is
// runtime data, so no static sentinel can name it; T-18 owns the typed-error
// systematization this anticipates.
type vendorFailure struct {
	message string
}

func (e *vendorFailure) Error() string {
	return e.message
}

// StatusClientAborted is answered when the caller went away before its
// attempt finished. It is neither retryable nor a verdict on the account,
// so the next request still starts on it.
const StatusClientAborted = 499

// Exchange is one upstream attempt: what to send, to whom, and how the client
// wants the answer rendered. The codec and the endpoint arrive with routing,
// so the executor never reads the catalog.
type Exchange struct {
	ProviderID string
	Codec      wire.CodecModule
	Options    wire.CodecOpts
	Request    *inference.Request
	Inbound    wire.InboundCodec
	Surface    string
	RequestID  string
	// RequiresStream reports that the connection's upstream only answers a
	// streamed request, so a client that asked for one complete body is
	// served by reassembling the stream rather than by a one-shot request the
	// upstream would refuse.
	RequiresStream bool
	// CredentialLabel names the account in a log, never the secret.
	CredentialLabel string
	// CallWait is how long one attempt waits for response headers, and then
	// how long the body may stay silent between bytes. A stream that keeps
	// sending is read until the provider ends it. Zero means no wait.
	CallWait time.Duration
	// Final reports that no further attempt follows, so a failure has to be
	// written to the client rather than reported to the caller.
	Final bool
	// Policy is the connection's own failover choice: which statuses the relay
	// would otherwise hand back to the client are worth another account.
	Policy Policy
	// OnUnauthorized renews a token a Claude request just refused. The
	// executor calls it once, before anything is written to the client, and
	// resends with what it returns.
	OnUnauthorized func(ctx context.Context) (catalog.Authorization, error)
}

// Policy is how one connection asked the relay to fail over.
type Policy struct {
	// SwitchOn4xx and SwitchOn5xx move a request to another account when the
	// upstream refuses with a status of that class the relay does not always
	// retry, such as a 400 or a 501.
	SwitchOn4xx bool
	SwitchOn5xx bool
}

// AllowsClass reports whether the policy admits a status of this class, apart
// from whether the relay always retries that particular status.
func (p Policy) AllowsClass(status int) bool {
	switch {
	case status >= 400 && status < 500:
		return p.SwitchOn4xx
	case status >= 500 && status < 600:
		return p.SwitchOn5xx
	default:
		return false
	}
}

// Switchable reports whether an unlisted 4xx or 5xx may be retried elsewhere.
// A status the relay always retries is not this decision's to make, so it
// answers false for one.
func (p Policy) Switchable(status int) bool {
	return !Retryable(status) && p.AllowsClass(status)
}

// Result reports how one attempt ended, for logging, routing feedback, and
// usage recording.
type Result struct {
	Status     int
	Usage      inference.UsageReport
	Duration   time.Duration
	Model      string
	Credential string
	// CredentialID is the stored account the attempt ran on, which is what a
	// retry excludes and what a model refusal is remembered against. The label
	// beside it is for humans, and the two are not interchangeable.
	CredentialID string
	// HeaderLatency is how long the upstream took to answer at all, which is
	// what a fastest-first group orders by.
	HeaderLatency time.Duration
	// Retryable reports a failure this attempt did not write to the client, so
	// the caller may try the next candidate.
	Retryable bool
	// ClassSwitch reports a failure the connection's own policy made
	// retryable although the relay does not always retry its status. The
	// walk treats it like any other retryable failure: each attempt waits
	// its retry window, and every account of the connection answers once
	// before the request leaves it.
	ClassSwitch bool
	// Wrote reports that this attempt already sent the client its answer, so
	// a later failure must not write a second one.
	Wrote bool
	// ModelAccess reports a refusal that is about the model rather than the
	// account: the account may not use this model, so the next attempt belongs
	// to another account of the same connection rather than to another
	// provider, and the account itself stays in rotation.
	ModelAccess bool
	// ZeroTokens reports a 2xx that carried no token usage at all, which the
	// relay turns into a retryable rate limit. The account's own health is
	// not blamed for it.
	ZeroTokens bool
	// RetryAfter is how long the upstream asked the caller to wait.
	RetryAfter  time.Duration
	RelayedBody bool
	// RateLimits are the quota windows the upstream reported beside its
	// response, which is the freshest reading Relo gets without asking.
	RateLimits []activity.WindowSample
	// Capture holds what this attempt sent and what its provider answered,
	// which the caller stores beside the usage row of the request.
	Capture *Capture
	// RefreshSettled reports that a 401 was already handled by OnUnauthorized
	// failing, so the caller must not treat the status as a fresh refusal.
	RefreshSettled bool
}

// Executor sends canonical requests upstream and relays their responses.
type Executor struct {
	client *http.Client
	logger *slog.Logger
}

// NewExecutor returns an executor that uses the given HTTP client.
func NewExecutor(client *http.Client, logger *slog.Logger) *Executor {
	return &Executor{client: client, logger: logger}
}

// Execute sends one exchange upstream and writes the client response. An
// attempt that may still be replaced by another candidate returns before
// writing anything, so the caller keeps its choice of what the client sees.
func (e *Executor) Execute(ctx context.Context, ex Exchange, w http.ResponseWriter) (Result, error) {
	started := time.Now()
	// A client that asked for one complete answer still gets one: when the
	// upstream only speaks streams, the request goes out as a stream on a copy
	// and the answer is reassembled. The canonical request is shared between
	// candidates, so it is never mutated.
	request, aggregate := aggregatedUpstreamRequest(ex)
	if binder, ok := ex.Codec.(wire.RequestBinder); ok {
		ex.Codec = binder.Bind(request, ex.Options)
	}
	st := &dispatchState{capture: &Capture{}}
	if failed, err, done := e.dispatchPasses(ctx, dispatchPass{ex: &ex, writer: w, request: request, started: started, state: st}); done {
		return failed, err
	}
	defer func() { _ = st.response.Body.Close() }()
	att := attempt{writer: w, exchange: ex, started: started, latency: st.latency, capture: st.capture, state: st}
	result, err := e.relay(att, st.response, aggregate)
	result.RateLimits = quota.WindowsFromHeaders(st.response.Header)
	result.Capture = st.capture
	result.RefreshSettled = st.refreshSettled
	return result, err
}

type dispatchState struct {
	capture        *Capture
	response       *http.Response
	latency        time.Duration
	refreshSettled bool
}

type dispatchPass struct {
	ex      *Exchange
	writer  http.ResponseWriter
	request *inference.Request
	started time.Time
	state   *dispatchState
}

func (p dispatchPass) attempt(latency time.Duration) attempt {
	return attempt{writer: p.writer, exchange: *p.ex, started: p.started, latency: latency, capture: p.state.capture, state: p.state}
}

func aggregatedUpstreamRequest(ex Exchange) (*inference.Request, bool) {
	request := ex.Request
	if !ex.RequiresStream || ex.Request.Stream {
		return request, false
	}
	streamed := *ex.Request
	streamed.Stream = true
	return &streamed, true
}

func (e *Executor) failDispatch(att attempt, status int, code string, err error) (Result, error) {
	result, err := e.fail(att, status, code, err)
	result.Capture = att.state.capture
	result.RefreshSettled = att.state.refreshSettled
	return result, err
}

func (e *Executor) dispatchPasses(ctx context.Context, pass dispatchPass) (Result, error, bool) {
	for num := 0; num < 2; num++ {
		retry, failed, result, err := e.dispatchOnce(ctx, pass, num)
		if failed {
			return result, err, true
		}
		if !retry {
			return Result{}, nil, false
		}
	}
	return Result{}, nil, false
}

func (e *Executor) dispatchOnce(ctx context.Context, pass dispatchPass, num int) (bool, bool, Result, error) {
	ex := pass.ex
	outbound, err := ex.Codec.EncodeRequest(pass.request, ex.Options)
	if err != nil {
		result, err := e.failDispatch(pass.attempt(0), http.StatusBadGateway, "bad_request", err)
		return false, true, result, err
	}
	if num == 0 {
		if message, captureErr := captureRequest(outbound, MaxCaptureBytes); captureErr == nil {
			pass.state.capture.Request = message
		}
	}
	if ex.Options.Signer != nil {
		if err := ex.Options.Signer(outbound); err != nil {
			result, err := e.failDispatch(pass.attempt(0), http.StatusBadGateway, "signing_failed", err)
			return false, true, result, err
		}
	}
	return e.sendDispatchRequest(ctx, pass, outbound, num)
}

func (e *Executor) sendDispatchRequest(ctx context.Context, pass dispatchPass, outbound *http.Request, num int) (bool, bool, Result, error) {
	ex := pass.ex
	clock, callCtx := startHeaderClock(ctx, ex.CallWait)
	response, err := e.client.Do(outbound.WithContext(callCtx))
	pass.state.latency = time.Since(pass.started)
	if err != nil {
		result, err := e.failDispatchSend(pass.attempt(pass.state.latency), clock, ctx, err)
		return false, true, result, err
	}
	if clock.defused() {
		_ = response.Body.Close()
		clock.release()
		result, err := e.failDispatch(pass.attempt(pass.state.latency), http.StatusGatewayTimeout, "header_timeout",
			fmt.Errorf("%w: no response headers within %s", ErrUpstreamFailed, ex.CallWait))
		return false, true, result, err
	}
	// The body owns the call context from here: closing it, whichever
	// path closes it, releases the request the way its cancel would.
	response.Body = newIdleBody(ctx, response.Body, ex.CallWait, clock.release)
	pass.state.response = response
	return e.settleDispatchPass(ctx, pass, num)
}

func (e *Executor) failDispatchSend(att attempt, clock *headerClock, ctx context.Context, err error) (Result, error) {
	headerExpired := clock.defused()
	clock.release()
	if headerExpired && ctx.Err() == nil {
		return e.failDispatch(att, http.StatusGatewayTimeout, "header_timeout",
			fmt.Errorf("%w: no response headers within %s", ErrUpstreamFailed, att.exchange.CallWait))
	}
	return e.failDispatch(att, http.StatusBadGateway, "upstream_unavailable",
		fmt.Errorf("%w: %w", ErrUpstreamFailed, err))
}

func (e *Executor) settleDispatchPass(ctx context.Context, pass dispatchPass, num int) (bool, bool, Result, error) {
	ex := pass.ex
	response := pass.state.response
	if num == 0 && response.StatusCode == http.StatusUnauthorized && ex.OnUnauthorized != nil {
		next, refreshErr := ex.OnUnauthorized(ctx)
		if refreshErr != nil {
			pass.state.refreshSettled = true
			return false, false, Result{}, nil
		}
		_ = response.Body.Close()
		applyRefreshedAuth(&ex.Options, next)
		return true, false, Result{}, nil
	}
	if decodeErr := contentencoding.Decode(response); decodeErr != nil {
		_ = response.Body.Close()
		result, err := e.failDispatch(pass.attempt(pass.state.latency), http.StatusBadGateway, "upstream_unavailable",
			fmt.Errorf("%w: %v", ErrUpstreamFailed, decodeErr))
		return false, true, result, err
	}
	return false, false, Result{}, nil
}

func applyRefreshedAuth(options *wire.CodecOpts, next catalog.Authorization) {
	options.CredentialRef = next.Token
	if next.Project != "" {
		options.Project = next.Project
	}
	if next.BaseURL != "" {
		options.BaseURL = next.BaseURL
	}
	if len(next.Headers) == 0 {
		return
	}
	merged := maps.Clone(options.ExtraHeaders)
	if merged == nil {
		merged = map[string]string{}
	}
	maps.Copy(merged, next.Headers)
	options.ExtraHeaders = merged
}

func (e *Executor) relay(att attempt, response *http.Response, aggregate bool) (Result, error) {
	if response.StatusCode >= http.StatusBadRequest {
		return e.relayRejection(att, response)
	}
	if att.exchange.Request.Stream {
		return e.relayStream(att, response)
	}
	if aggregate {
		return e.relayAggregated(att, response)
	}
	return e.relayComplete(att, response)
}

func (e *Executor) fail(att attempt, status int, code string, cause error) (Result, error) {
	ex := att.exchange
	if errors.Is(cause, context.Canceled) {
		if e.logger != nil {
			e.logger.Warn("upstream attempt abandoned", "provider", ex.ProviderID,
				"code", "client_aborted", "error", cause)
		}
		return Result{
			Status: StatusClientAborted, Duration: time.Since(att.started), Model: ex.Request.Model,
			Credential: ex.CredentialLabel, HeaderLatency: att.latency,
		}, cause
	}
	if e.logger != nil {
		e.logger.Warn("upstream attempt failed", "provider", ex.ProviderID,
			"code", code, "error", cause)
	}
	result := Result{
		Status: status, Duration: time.Since(att.started), Model: ex.Request.Model,
		Credential: ex.CredentialLabel, HeaderLatency: att.latency,
	}
	switchable := ex.Policy.Switchable(status)
	if !ex.Final && (Retryable(status) || switchable) {
		result.Retryable = true
		result.ClassSwitch = switchable
		return result, cause
	}
	writeFailure(att.writer, ex.Inbound, &inference.ErrorInfo{Code: code, Message: cause.Error(), Status: status})
	result.Wrote = true
	return result, cause
}

// Retryable reports whether another candidate may still serve a request that
// failed with this status: a transient upstream fault or a rate limit is worth
// trying elsewhere, and anything else is the caller's to fix.
func Retryable(status int) bool {
	switch status {
	case http.StatusRequestTimeout, http.StatusTooEarly, http.StatusTooManyRequests,
		http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable,
		http.StatusGatewayTimeout, 529:
		return true
	default:
		return false
	}
}

func retryAfter(response *http.Response) time.Duration {
	raw := response.Header.Get("Retry-After")
	if raw == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(raw); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	when, err := http.ParseTime(raw)
	if err != nil {
		return 0
	}
	if delay := time.Until(when); delay > 0 {
		return delay
	}
	return 0
}

func streamDecoder(codecImpl wire.Codec) (wire.StreamDecoder, error) {
	streaming, ok := codecImpl.(wire.StreamCodec)
	if !ok {
		return nil, fmt.Errorf("%w %T has no stream decoder", ErrCodecStreamless, codecImpl)
	}
	return streaming.NewStreamDecoder(), nil
}

func decodeRejection(codecImpl wire.Codec, status int, body []byte) *inference.ErrorInfo {
	decoder, ok := codecImpl.(wire.ErrorDecoder)
	if !ok {
		return &inference.ErrorInfo{Code: http.StatusText(status), Message: string(body), Status: status}
	}
	return decoder.DecodeError(status, body)
}

func failureFrom(events []inference.Event) *inference.ErrorInfo {
	for _, event := range events {
		if event.Kind == inference.EventError {
			return event.Error
		}
	}
	return nil
}
