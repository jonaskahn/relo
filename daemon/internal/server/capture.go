// Inference capture recording: the request and response rows debugging reads.
package server

import (
	"bytes"
	"context"
	"io"
	"net/http"

	"github.com/jonaskahn/relo/internal/activity"
)

const maxInboundRead = 64 << 20

func captureInbound(redactor CaptureRedactor, r *http.Request) *CapturedMessage {
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
	kept, captureTruncated := redactor.CaptureBytes(body, redactor.MaxCaptureBytes())
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
		captures = append(captures, captureRow(nil, activity.CaptureAgentRequest, outcome.inbound))
	}
	captures = appendAnswerCapture(captures, outcome)
	for ordinal, attempt := range outcome.attempts {
		captures = appendAttemptCaptures(captures, ordinal, attempt)
	}
	if len(captures) == 0 {
		return
	}
	if err := s.opts.Captures.Append(ctx, eventID, captures); err != nil {
		s.opts.Logger.Warn("store request captures", "request_id", outcome.requestID, "error", err)
	}
}

func appendAnswerCapture(captures []activity.CaptureWrite, outcome *requestOutcome) []activity.CaptureWrite {
	// The answer belongs beside the request it answers, so a request whose
	// bytes Relo could not decode still has both halves.
	if outcome.answer != nil {
		if message, ok := outcome.answer.captured(); ok {
			captures = append(captures, captureRow(nil, activity.CaptureAgentResponse, message))
		}
	}
	return captures
}

func appendAttemptCaptures(captures []activity.CaptureWrite, ordinal int, attempt attemptRow) []activity.CaptureWrite {
	if attempt.capture == nil {
		return captures
	}
	index := ordinal
	if attempt.capture.Request != nil {
		captures = append(captures, captureRow(&index, activity.CaptureProviderRequest, attempt.capture.Request))
	}
	if attempt.capture.Response != nil {
		captures = append(captures, captureRow(&index, activity.CaptureProviderResponse, attempt.capture.Response))
	}
	return captures
}

func captureRow(ordinal *int, kind string, message *CapturedMessage) activity.CaptureWrite {
	return activity.CaptureWrite{
		Ordinal: ordinal, Kind: kind, Method: message.Method, URL: message.URL,
		Status: message.Status, Headers: message.Headers, Body: message.Body,
		Truncated: message.Truncated,
	}
}

type agentAnswer struct {
	http.ResponseWriter
	redactor  CaptureRedactor
	limit     int64
	status    int
	headers   map[string][]string
	body      []byte
	truncated bool
}

var _ http.ResponseWriter = (*agentAnswer)(nil)

func newAgentAnswer(w http.ResponseWriter, redactor CaptureRedactor) *agentAnswer {
	return &agentAnswer{ResponseWriter: w, redactor: redactor, limit: redactor.MaxCaptureBytes()}
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

// Write keeps as much of the answer as the limit holds, marks the rest
// truncated, and always reports the bytes written, so the cap never cuts a
// reply short.
func (a *agentAnswer) Write(data []byte) (int, error) {
	if a.status == 0 {
		a.WriteHeader(http.StatusOK)
	}
	room := a.limit - int64(len(a.body))
	switch {
	case room <= 0:
		if len(data) > 0 {
			a.truncated = true
		}
	case int64(len(data)) > room:
		a.body = append(a.body, data[:room]...)
		a.truncated = true
	default:
		a.body = append(a.body, data...)
	}
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
	return &CapturedMessage{
		Status: a.status, Headers: a.headers, Body: a.redactor.RedactBody(a.body),
		Truncated: a.truncated,
	}, true
}
