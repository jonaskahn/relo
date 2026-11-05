// Inference capture recording: the request and response rows debugging reads.
package server

import (
	"bytes"
	"context"
	"io"
	"net/http"

	"github.com/jonaskahn/relo/internal/activity"
	capturelog "github.com/jonaskahn/relo/internal/capture"
)

const maxInboundRead = 64 << 20

func captureInbound(redactor CaptureRedactor, r *http.Request, limit int64, tail bool) *CapturedMessage {
	message := &CapturedMessage{
		Method: r.Method, URL: inboundTarget(r), Headers: redactor.RedactHeaders(inboundHeaders(r)),
	}
	if r.Body == nil {
		return message
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxInboundRead+1))
	if err != nil {
		return message
	}
	readTruncated := int64(len(body)) > maxInboundRead
	if readTruncated {
		body = body[:maxInboundRead]
	}
	_ = r.Body.Close()
	r.Body = io.NopCloser(bytes.NewReader(body))
	r.ContentLength = int64(len(body))
	kept, captureTruncated := redactor.CaptureBytes(body, limit, tail)
	message.Body, message.Truncated = redactor.RedactBody(kept), readTruncated || captureTruncated
	return message
}

func inboundTarget(r *http.Request) string {
	if r.URL == nil {
		return ""
	}
	return r.URL.RequestURI()
}

func inboundHeaders(r *http.Request) map[string][]string {
	headers := make(map[string][]string, len(r.Header)+1)
	for name, values := range r.Header {
		headers[name] = append([]string(nil), values...)
	}
	if r.Host != "" {
		headers["Host"] = []string{r.Host}
	}
	return headers
}

func (s *Server) recordCaptures(ctx context.Context, eventID int64, outcome *requestOutcome) {
	if s.opts.Captures == nil {
		return
	}
	captures := make([]activity.CaptureWrite, 0, len(outcome.attempts)*2+2)
	if outcome.inbound != nil {
		captures = append(captures, s.captureRow(nil, activity.CaptureAgentRequest, outcome.inbound))
	}
	captures = s.appendAnswerCapture(captures, outcome)
	for ordinal, attempt := range outcome.attempts {
		captures = s.appendAttemptCaptures(captures, ordinal, attempt)
	}
	if len(captures) == 0 {
		return
	}
	if err := s.opts.Captures.Append(ctx, eventID, captures); err != nil {
		s.opts.Logger.Warn("store request captures", "request_id", outcome.requestID, "error", err)
	}
}

func (s *Server) appendAnswerCapture(captures []activity.CaptureWrite, outcome *requestOutcome) []activity.CaptureWrite {
	// The answer belongs beside the request it answers, so a request whose
	// bytes Relo could not decode still has both halves.
	if outcome.answer != nil {
		if message, ok := outcome.answer.captured(); ok {
			captures = append(captures, s.captureRow(nil, activity.CaptureAgentResponse, message))
		}
	}
	return captures
}

func (s *Server) appendAttemptCaptures(captures []activity.CaptureWrite, ordinal int, attempt attemptRow) []activity.CaptureWrite {
	if attempt.capture == nil {
		return captures
	}
	index := ordinal
	if attempt.capture.Request != nil {
		captures = append(captures, s.captureRow(&index, activity.CaptureProviderRequest, attempt.capture.Request))
	}
	if attempt.capture.Response != nil {
		captures = append(captures, s.captureRow(&index, activity.CaptureProviderResponse, attempt.capture.Response))
	}
	return captures
}

func (s *Server) captureRow(ordinal *int, kind string, message *CapturedMessage) activity.CaptureWrite {
	// The traffic log raises the capture bound, so a body kept whole for the
	// file on disk is cut back to the state cap here: the database keeps the
	// head and tail, which is enough to debug both ends of a large exchange.
	kept, truncated := s.clampCapture(kind, message.Body)
	return activity.CaptureWrite{
		Ordinal: ordinal, Kind: kind, Method: message.Method, URL: message.URL,
		Status: message.Status, Headers: message.Headers, Body: kept,
		Truncated: message.Truncated || truncated,
	}
}

func (s *Server) clampCapture(kind string, body []byte) ([]byte, bool) {
	limit := s.opts.CaptureRedactor.MaxCaptureBytes()
	if kind == activity.CaptureProviderRequest || kind == activity.CaptureProviderResponse {
		limit = capturelog.ProviderMaxBytes
	}
	return s.opts.CaptureRedactor.CaptureBytes(body, limit, true)
}

type agentAnswer struct {
	http.ResponseWriter
	redactor CaptureRedactor
	sink     *capturelog.Sink
	status   int
	headers  map[string][]string
}

var _ http.ResponseWriter = (*agentAnswer)(nil)

func newAgentAnswer(w http.ResponseWriter, redactor CaptureRedactor, limit int64, tail bool) *agentAnswer {
	return &agentAnswer{ResponseWriter: w, redactor: redactor, sink: capturelog.NewSink(limit, tail)}
}

// WriteHeader keeps the headers the reply was written with, once, and hands
// the status on. The headers are read here rather than at the end, because a
// writer that set them before writing is the only place they are still the
// reply's own.
func (a *agentAnswer) WriteHeader(status int) {
	if a.status == 0 {
		a.status = status
		a.headers = a.redactor.RedactHeaders(a.Header())
	}
	a.ResponseWriter.WriteHeader(status)
}

// Write keeps as much of the answer as the capture shape holds, marks the rest
// truncated, and always reports the bytes written, so the cap never cuts a
// reply short.
func (a *agentAnswer) Write(data []byte) (int, error) {
	if a.status == 0 {
		a.WriteHeader(http.StatusOK)
	}
	_, _ = a.sink.Write(data)
	return a.ResponseWriter.Write(data)
}

// Flush forwards the flush a streamed reply depends on, so a wrapped writer
// still reaches the client as the answer is written.
func (a *agentAnswer) Flush() {
	if flusher, ok := a.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (a *agentAnswer) captured() (*CapturedMessage, bool) {
	if a.status == 0 {
		return nil, false
	}
	body, truncated := a.sink.Bytes()
	return &CapturedMessage{
		Status: a.status, Headers: a.headers, Body: a.redactor.RedactBody(body),
		Truncated: truncated,
	}, true
}
