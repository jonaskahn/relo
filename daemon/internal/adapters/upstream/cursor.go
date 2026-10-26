// Cursor duplex attempts: conversing instead of sending one request.
package upstream

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	cursorproto "github.com/jonaskahn/relo/internal/adapters/cursor"
	"github.com/jonaskahn/relo/internal/adapters/wire"
	"github.com/jonaskahn/relo/internal/adapters/wire/cursor"
	"github.com/jonaskahn/relo/internal/adapters/wire/sse"
	"github.com/jonaskahn/relo/internal/inference"
)

// bidirectionalRun is one duplex attempt: the attempt, its usage, and
// what the client already read.
type bidirectionalRun struct {
	attempt    attempt
	started    time.Time
	collector  *usageCollector
	tracker    *streamWriter
	wrote      bool
	emitFailed bool
}

// runBidirectional converses with a bidirectional codec until the turn
// ends, streaming canonical events to the client as they arrive. A turn
// that may still be replaced by another candidate returns before writing
// anything, like every other attempt.
func (e *Executor) runBidirectional(ctx context.Context, att attempt, started time.Time) (Result, error) {
	codec, ok := att.exchange.Codec.(wire.BidirectionalCodec)
	if !ok {
		return e.fail(att, http.StatusBadGateway, "not_implemented",
			fmt.Errorf("%w: no bidirectional run", ErrCodecStreamless))
	}
	run := &bidirectionalRun{attempt: att, started: started, collector: newUsageCollector()}
	// The duplex codec builds its own request, so the exchange is captured
	// here rather than by the dispatcher: without it the traffic log is
	// blind to every Cursor turn.
	att.capture.Request = captureCursorExchange(att.exchange)
	if att.exchange.Request.Stream {
		prepareStream(att.writer)
		run.tracker = &streamWriter{ResponseWriter: att.writer}
	}
	events, err := e.converse(ctx, att, codec, run.emit)
	if run.emitFailed && err == nil {
		err = fmt.Errorf("encode the upstream events: %w", wire.ErrUnknownEvent)
	}
	att.capture.Response = captureCursorRefusal(err)
	if err != nil {
		return e.failBidirectional(run, events, err)
	}
	run.collector.observeAll(events)
	if failure := run.collector.failed(); failure != nil {
		return e.fail(att, http.StatusBadGateway, "bad_gateway", &vendorFailure{message: failure.Message})
	}
	if run.tracker != nil {
		return bidirectionalStreamResult(att, started, run.collector, run.tracker), nil
	}
	return e.answerBidirectional(run, events)
}

func (r *bidirectionalRun) emit(events []inference.Event) {
	frames, err := encodeFrames(r.attempt.exchange.Inbound, events, r.collector)
	if err != nil {
		r.emitFailed = true
		return
	}
	if r.tracker == nil || len(frames) == 0 {
		return
	}
	_ = sse.WriteEvents(r.tracker, frames)
	if r.tracker.writeErr == nil {
		r.wrote = true
	}
}

func (e *Executor) converse(ctx context.Context, att attempt, codec wire.BidirectionalCodec, emit func([]inference.Event)) ([]inference.Event, error) {
	for attempt := 0; attempt < 2; attempt++ {
		events, err := codec.RunBidirectional(ctx, wire.BidirectionalCall{
			Client: e.client, Request: att.exchange.Request, Opts: att.exchange.Options, Emit: emit,
			Record: recordCursorFrame(att),
		})
		if !errors.Is(err, cursor.ErrUnauthorized) || att.exchange.OnUnauthorized == nil || attempt == 1 {
			return events, err
		}
		next, refreshErr := att.exchange.OnUnauthorized(ctx)
		if refreshErr != nil {
			att.state.refreshSettled = true
			return events, err
		}
		applyRefreshedAuth(&att.exchange.Options, next)
	}
	return nil, nil
}

func (e *Executor) failBidirectional(run *bidirectionalRun, events []inference.Event, runErr error) (Result, error) {
	att, started, collector, tracker := run.attempt, run.started, run.collector, run.tracker
	collector.observeAll(events)
	if tracker != nil && (tracker.writeErr != nil || run.wrote) {
		result := bidirectionalStreamResult(att, started, collector, tracker)
		if tracker.writeErr != nil {
			return result, fmt.Errorf("%w: %v", ErrRelayFailed, tracker.writeErr)
		}
		return result, runErr
	}
	if errors.Is(runErr, context.Canceled) {
		return Result{
			Status: StatusClientAborted, Duration: time.Since(started), Model: att.exchange.Request.Model,
			Credential: att.exchange.CredentialLabel, HeaderLatency: att.latency,
		}, runErr
	}
	status := refusalStatus(runErr)
	cause := fmt.Errorf("%w: %v", ErrUpstreamFailed, runErr)
	code := "upstream_unavailable"
	if errors.Is(runErr, wire.ErrUnsupportedFeature) || errors.Is(runErr, wire.ErrInvalidRequest) {
		// A refusal the codec decided before dispatch is the caller's to fix,
		// not a network fault: wrapping it as an upstream failure misnames a
		// local verdict and sends the caller looking at the wrong end.
		cause = runErr
		code = "bad_request"
	}
	result, err := e.fail(att, status, code, cause)
	result.RefreshSettled = att.state.refreshSettled
	return result, err
}

// refusalStatus answers what a failed turn means to the caller. A refusal
// Cursor named classifies as itself, so a rejected model or an expired sign-in
// stops after one attempt instead of burning the failover ladder and cooling
// the connection down; anything unnamed stays a retryable upstream fault. A
// request the codec refused before sending is the caller's to fix, never a
// reason to try another account.
func refusalStatus(runErr error) int {
	var refusal cursorproto.EndStream
	if errors.As(runErr, &refusal) && refusal.Status != 0 {
		return refusal.Status
	}
	if errors.Is(runErr, cursor.ErrUnauthorized) {
		return http.StatusUnauthorized
	}
	if errors.Is(runErr, wire.ErrUnsupportedFeature) || errors.Is(runErr, wire.ErrInvalidRequest) {
		return http.StatusBadRequest
	}
	return http.StatusBadGateway
}

func bidirectionalStreamResult(att attempt, started time.Time, collector *usageCollector, tracker *streamWriter) Result {
	return Result{
		Status: http.StatusOK, Duration: time.Since(started), Model: att.exchange.Request.Model,
		Credential: att.exchange.CredentialLabel, HeaderLatency: att.latency,
		Usage: collector.report(), Wrote: true,
	}
}

func (e *Executor) answerBidirectional(run *bidirectionalRun, events []inference.Event) (Result, error) {
	att, started, collector := run.attempt, run.started, run.collector
	if !collector.reported() && !hasToolUse(events) {
		return e.zeroTokens(att)
	}
	payload, err := att.exchange.Inbound.EncodeResponse(events)
	if err != nil {
		return e.fail(att, http.StatusBadGateway, "bad_gateway", err)
	}
	result := Result{
		Status: http.StatusOK, Duration: time.Since(started), Model: att.exchange.Request.Model,
		Credential: att.exchange.CredentialLabel, HeaderLatency: att.latency, Wrote: true,
	}
	att.writer.Header().Set("Content-Type", "application/json")
	att.writer.WriteHeader(http.StatusOK)
	if _, err := att.writer.Write(payload); err != nil {
		return result, fmt.Errorf("%w: write response: %v", ErrRelayFailed, err)
	}
	result.Usage = collector.report()
	result.RelayedBody = true
	return result, nil
}

// hasToolUse reports a turn that handed tools to the client: usage may be
// absent on such turns, which is not an empty completion.
func hasToolUse(events []inference.Event) bool {
	for _, event := range events {
		if event.Kind == inference.EventTerminal && event.Terminal != nil &&
			event.Terminal.Reason == inference.EventReasonToolUse {
			return true
		}
	}
	return false
}
