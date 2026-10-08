// Relay execution: streaming, aggregated, and complete sends.
package upstream

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jonaskahn/relo/internal/inference"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jonaskahn/relo/internal/adapters/wire"
	"github.com/jonaskahn/relo/internal/adapters/wire/anthropic"
	"github.com/jonaskahn/relo/internal/adapters/wire/eventstream"
	"github.com/jonaskahn/relo/internal/adapters/wire/sse"
)

type attempt struct {
	writer   http.ResponseWriter
	exchange Exchange
	started  time.Time
	latency  time.Duration
	capture  *Capture
	state    *dispatchState
}

func (e *Executor) relayStream(att attempt, response *http.Response) (Result, error) {
	decoder, err := streamDecoder(att.exchange.Codec)
	if err != nil {
		return e.fail(att, http.StatusBadGateway, "not_implemented", err)
	}
	collector := newUsageCollector()
	run := openStreamRun(att, response, decoder, collector)
	runErr := sse.Run(run.config)
	att.capture.Response = captureResponse(response, run.sink)
	return finishStreamRun(run, runErr, att)
}

type streamRun struct {
	config       sse.RunConfig
	sink         *captureSink
	tracker      *streamWriter
	collector    *usageCollector
	readFailed   bool
	readErr      error
	decodeFailed bool
	eofRan       bool
}

func openStreamRun(att attempt, response *http.Response, decoder wire.StreamDecoder, collector *usageCollector) *streamRun {
	// The stream is teed into a bounded sink, so the reply is kept while the
	// client reads it and the cap never cuts the stream short.
	sink := newCaptureSink(MaxCaptureBytes)
	upstream := io.TeeReader(response.Body, sink)
	if strings.Contains(response.Header.Get("Content-Type"), "eventstream") {
		upstream = eventstream.NewAdapter(upstream)
	}

	// Frames reach the client as they arrive: a stream the provider stalls or
	// abandons still shows what arrived beside its error frame, and what
	// arrived is never taken back for a retry. A read that failed before the
	// stream ended, and a failure the stream carried inside its frames, each
	// already wrote its own error frame, so both keep that answer and stay
	// out of the routing memory: an ambiguous answer is neither a success
	// nor a refusal to remember.
	prepareStream(att.writer)
	run := &streamRun{
		sink:      sink,
		tracker:   &streamWriter{ResponseWriter: att.writer},
		collector: collector,
	}
	run.config = run.streamConfig(att, decoder, collector, upstream)
	return run
}

func (run *streamRun) streamConfig(att attempt, decoder wire.StreamDecoder, collector *usageCollector, upstream io.Reader) sse.RunConfig {
	transform := transformFrames(att.exchange.Inbound, decoder, collector)
	onError := errorFrames(att.exchange.Inbound)
	return sse.RunConfig{
		Upstream: upstream,
		Writer:   run.tracker,
		Transform: func(event wire.SSEEvent) ([]wire.SSEEvent, error) {
			frames, transformErr := transform(event)
			if transformErr != nil {
				run.decodeFailed = true
			}
			return frames, transformErr
		},
		OnEOF: func() ([]wire.SSEEvent, error) {
			run.eofRan = true
			return closingFrames(att.exchange.Inbound, decoder, collector)()
		},
		OnError: streamRunOnError(run, onError),
	}
}

func streamRunOnError(run *streamRun, onError func(error) []wire.SSEEvent) func(error) []wire.SSEEvent {
	return func(err error) []wire.SSEEvent {
		// A read that failed before the stream ended is the provider
		// stopping mid-answer; a decode or closing failure already has
		// its own error frame and keeps its old path.
		if !run.decodeFailed && !run.eofRan {
			run.readFailed = true
			run.readErr = err
		}
		if onError == nil {
			return nil
		}
		return onError(err)
	}
}

func streamRunResult(run *streamRun, att attempt) Result {
	return Result{
		Status:        http.StatusOK,
		Duration:      time.Since(att.started),
		Model:         att.exchange.Request.Model,
		Credential:    att.exchange.CredentialLabel,
		HeaderLatency: att.latency,
		Usage:         run.collector.report(),
		Wrote:         true,
	}
}
func finishStreamRun(run *streamRun, runErr error, att attempt) (Result, error) {
	result := streamRunResult(run, att)
	failure := run.collector.failed()
	switch {
	case run.tracker.writeErr != nil:
		// The client is gone: what the relay wrote never arrived, so the
		// attempt failed even when the upstream answered in full.
		return result, fmt.Errorf("%w: %v", ErrRelayFailed, run.tracker.writeErr)
	case run.readFailed && errors.Is(run.readErr, context.Canceled):
		// The caller went away mid-stream: the partial answer already went
		// out with it, so the attempt is abandoned rather than reported.
		// The account is left alone, the way any abandoned send leaves it.
		result.Status = StatusClientAborted
		return result, run.readErr
	case run.readFailed:
		return result, fmt.Errorf("%w: read upstream stream: %w", ErrUpstreamFailed, run.readErr)
	case runErr != nil:
		return result, fmt.Errorf("%w: %v", ErrUpstreamFailed, runErr)
	case failure != nil:
		return result, fmt.Errorf("%w: %s", ErrUpstreamFailed, failure.Message)
	default:
		return result, nil
	}
}

func (e *Executor) relayAggregated(att attempt, response *http.Response) (Result, error) {
	decoder, err := streamDecoder(att.exchange.Codec)
	if err != nil {
		return e.fail(att, http.StatusBadGateway, "not_implemented", err)
	}
	// The whole body is read here anyway, so it is captured as it is read.
	sink := newCaptureSink(MaxCaptureBytes)
	upstream := io.TeeReader(response.Body, sink)
	if strings.Contains(response.Header.Get("Content-Type"), "eventstream") {
		upstream = eventstream.NewAdapter(upstream)
	}
	events, failed, err := e.collectAggregatedFrames(att, response, decoder, upstream)
	if err != nil {
		return failed, err
	}
	collector := newUsageCollector()
	collector.observeAll(events)
	att.capture.Response = captureResponse(response, sink)
	if !collector.reported() {
		return e.zeroTokens(att)
	}
	return e.answerRelayEvents(att, response, collector, events)
}

func (e *Executor) collectAggregatedFrames(att attempt, response *http.Response, decoder wire.StreamDecoder, upstream io.Reader) ([]inference.Event, Result, error) {
	events := make([]inference.Event, 0, 16)
	reader := sse.NewReader(upstream)
	for {
		frame, readErr := reader.Next()
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			result, err := e.failRead(att, "read upstream stream", readErr)
			return nil, result, err
		}
		if failed, err, done := e.pushAggregatedFrame(att, decoder, frame, &events); done {
			return nil, failed, err
		}
	}
	return e.finishAggregatedFrames(att, response, decoder, events)
}

func (e *Executor) pushAggregatedFrame(att attempt, decoder wire.StreamDecoder, frame wire.SSEEvent, events *[]inference.Event) (Result, error, bool) {
	canonical, decodeErr := decoder.Push(frame)
	if decodeErr != nil {
		result, err := e.fail(att, http.StatusBadGateway, "bad_gateway", decodeErr)
		return result, err, true
	}
	*events = append(*events, canonical...)
	return Result{}, nil, false
}

func (e *Executor) finishAggregatedFrames(att attempt, response *http.Response, decoder wire.StreamDecoder, events []inference.Event) ([]inference.Event, Result, error) {
	final, err := decoder.Finish()
	if err != nil {
		result, err := e.fail(att, http.StatusBadGateway, "bad_gateway", err)
		return nil, result, err
	}
	events = append(events, final...)
	if failure := failureFrom(events); failure != nil {
		result, err := e.fail(att, failure.Status, failure.Code, &vendorFailure{message: failure.Message})
		return nil, result, err
	}
	return events, Result{}, nil
}

func (e *Executor) answerRelayEvents(att attempt, response *http.Response, collector *usageCollector, events []inference.Event) (Result, error) {
	if !collector.reported() {
		return e.zeroTokens(att)
	}
	payload, err := att.exchange.Inbound.EncodeResponse(events)
	if err != nil {
		return e.fail(att, http.StatusBadGateway, "bad_gateway", err)
	}
	result := Result{
		Status: response.StatusCode, Duration: time.Since(att.started), Model: att.exchange.Request.Model,
		Credential: att.exchange.CredentialLabel, HeaderLatency: att.latency, Wrote: true,
	}
	att.writer.Header().Set("Content-Type", "application/json")
	att.writer.WriteHeader(response.StatusCode)
	if _, err := att.writer.Write(payload); err != nil {
		return result, fmt.Errorf("%w: write response: %v", ErrRelayFailed, err)
	}
	result.Usage = collector.report()
	result.RelayedBody = true
	return result, nil
}

func (e *Executor) relayComplete(att attempt, response *http.Response) (Result, error) {
	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes))
	if err != nil {
		return e.failRead(att, "read upstream response", err)
	}
	att.capture.Response = capturedBody(response, body, MaxCaptureBytes)
	events, err := att.exchange.Codec.DecodeResponse(body)
	if err != nil {
		return e.fail(att, http.StatusBadGateway, "bad_gateway", err)
	}
	if failure := failureFrom(events); failure != nil {
		return e.fail(att, failure.Status, failure.Code, &vendorFailure{message: failure.Message})
	}
	collector := newUsageCollector()
	collector.observeAll(events)
	return e.answerRelayEvents(att, response, collector, events)
}

func (e *Executor) zeroTokens(att attempt) (Result, error) {
	result, err := e.fail(att, http.StatusTooManyRequests, "empty_completion", ErrZeroTokens)
	result.ZeroTokens = true
	return result, err
}

func (e *Executor) failRead(att attempt, action string, cause error) (Result, error) {
	code := "upstream_read_failed"
	if errors.Is(cause, ErrUpstreamStall) {
		code = "upstream_timeout"
	}
	return e.fail(att, http.StatusGatewayTimeout, code,
		fmt.Errorf("%w: %s: %w", ErrUpstreamFailed, action, cause))
}

func (e *Executor) relayRejection(att attempt, response *http.Response) (Result, error) {
	body, err := io.ReadAll(io.LimitReader(response.Body, maxErrorBodyBytes))
	if err != nil {
		return e.fail(att, http.StatusBadGateway, "upstream_unavailable",
			fmt.Errorf("%w: read error body: %v", ErrUpstreamFailed, err))
	}
	att.capture.Response = capturedBody(response, body, MaxCaptureBytes)
	info := decodeRejection(att.exchange.Codec, response.StatusCode, body)
	result := Result{
		Status: response.StatusCode, Duration: time.Since(att.started), Model: att.exchange.Request.Model,
		Credential: att.exchange.CredentialLabel, HeaderLatency: att.latency,
		RetryAfter:  retryAfter(response),
		ModelAccess: modelAccessRefusal(response.StatusCode, info, att.exchange.Request.Model),
	}
	switchable := att.exchange.Policy.Switchable(response.StatusCode)
	if !att.exchange.Final && (Retryable(response.StatusCode) || result.ModelAccess || switchable) {
		result.Retryable = true
		result.ClassSwitch = switchable
		return result, fmt.Errorf("%w: %d %s", ErrUpstreamStatus, response.StatusCode, info.Message)
	}
	writeFailure(att.writer, att.exchange.Inbound, info)
	result.Wrote = true
	return result, fmt.Errorf("%w: %d %s", ErrUpstreamStatus, response.StatusCode, info.Message)
}

func modelAccessRefusal(status int, info *inference.ErrorInfo, model string) bool {
	if model == "" || info == nil {
		return false
	}
	if status == http.StatusNotFound {
		return true
	}
	if status != http.StatusForbidden {
		return false
	}
	return strings.Contains(info.Message, model) || strings.Contains(info.Code, model)
}

func prepareStream(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
}

type streamWriter struct {
	http.ResponseWriter
	writeErr error
}

// Write forwards one frame to the client, keeping the first failure so the
// relay can report what the stream cost.
func (t *streamWriter) Write(data []byte) (int, error) {
	written, err := t.ResponseWriter.Write(data)
	if err != nil && t.writeErr == nil {
		t.writeErr = err
	}
	return written, err
}

// Flush pushes buffered frames to the client, where the listener supports it.
func (t *streamWriter) Flush() {
	if flusher, ok := t.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func writeFailure(w http.ResponseWriter, inbound wire.InboundCodec, info *inference.ErrorInfo) {
	status := info.Status
	if status == 0 {
		status = http.StatusBadGateway
	}
	var payload []byte
	if _, anthropicClient := inbound.(*anthropic.Inbound); anthropicClient {
		payload = anthropic.FailureDocument(status, info.Message)
	} else {
		var err error
		payload, err = json.Marshal(map[string]any{"error": map[string]any{
			"type":    info.Code,
			"message": info.Message,
			"code":    status,
		}})
		if err != nil {
			payload = []byte(`{"error":{"type":"internal_error","message":"encoding the error failed"}}`)
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(payload)
}
