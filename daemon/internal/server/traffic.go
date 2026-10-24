// The traffic log keeps whole agent and provider exchanges on disk when the
// operator turns it on: one JSON line per half of an exchange, appended the
// moment that half exists, one file per day, aged out with the daemon log's own
// retention. The shapes here are what the relay hands the writer and what the
// request detail page reads back.
package server

import (
	"context"
	"time"

	"github.com/jonaskahn/relo/internal/activity"
)

// captureReader lists the messages stored beside one request, which the detail
// page resolves a capture id against, and names the request id an event id
// stands for.
type captureReader interface {
	Captures(ctx context.Context, eventID int64) ([]activity.CaptureHeader, error)
	RequestID(ctx context.Context, eventID int64) (string, error)
}

// TrafficHalf is one message of one exchange as the traffic log keeps it. A
// half is appended the moment it exists, so what the file holds is as whole as
// the exchange had become. RequestID leads the line on purpose: the log finds
// one request by that prefix, without decoding the lines it skips.
type TrafficHalf struct {
	RequestID  string          `json:"request_id"`
	Timestamp  string          `json:"timestamp"`
	Kind       string          `json:"kind"`
	Ordinal    *int            `json:"ordinal,omitempty"`
	Surface    string          `json:"surface,omitempty"`
	Origin     string          `json:"origin,omitempty"`
	Provider   string          `json:"provider,omitempty"`
	Model      string          `json:"model,omitempty"`
	Credential string          `json:"credential,omitempty"`
	Status     int             `json:"status,omitempty"`
	Message    *TrafficMessage `json:"message,omitempty"`
}

// TrafficMessage is one HTTP message: the method or the status, the target,
// every header, and the body as text. The bodies are already redacted. A body
// that is not UTF-8 reads lossy here; the state rows keep its raw bytes.
type TrafficMessage struct {
	Method    string              `json:"method,omitempty"`
	URL       string              `json:"url,omitempty"`
	Status    int                 `json:"status,omitempty"`
	Headers   map[string][]string `json:"headers,omitempty"`
	Body      string              `json:"body,omitempty"`
	Truncated bool                `json:"truncated,omitempty"`
}

// trafficTrace is what the traffic log kept of one request, folded back out of
// the halves it appended: the two halves of the agent's exchange, and one pair
// per upstream attempt.
type trafficTrace struct {
	AgentRequest  *TrafficMessage
	AgentResponse *TrafficMessage
	Attempts      []trafficAttemptHalf
}

// trafficAttemptHalf is what one attempt sent a provider and what came back.
type trafficAttemptHalf struct {
	Request  *TrafficMessage
	Response *TrafficMessage
}

// TrafficLog is the daily file of JSON lines each half of an exchange is
// appended to, and the place the request detail page reads whole bodies from.
type TrafficLog interface {
	AppendHalf(half TrafficHalf) error
	Halves(ctx context.Context, requestID string) ([]TrafficHalf, error)
}

// trafficMessageFor names the message of one stored capture inside a trace, so
// the detail page can show the body the traffic log kept rather than the
// state-sized one it fell back to.
func trafficMessageFor(trace trafficTrace, kind string, ordinal *int) *TrafficMessage {
	switch kind {
	case activity.CaptureAgentRequest:
		return trace.AgentRequest
	case activity.CaptureAgentResponse:
		return trace.AgentResponse
	case activity.CaptureProviderRequest, activity.CaptureProviderResponse:
		if ordinal == nil || *ordinal < 0 || *ordinal >= len(trace.Attempts) {
			return nil
		}
		attempt := trace.Attempts[*ordinal]
		if kind == activity.CaptureProviderRequest {
			return attempt.Request
		}
		return attempt.Response
	default:
		return nil
	}
}

// trafficOn reports whether the operator asked for the traffic log. It is read
// from the startup file per request, the same way the retry count is, so the
// switch takes effect on save without a daemon restart.
func (s *Server) trafficOn() bool {
	return s.opts.Traffic != nil && s.opts.Settings != nil && s.opts.Settings.LogTraffic()
}

// captureLimit is how much of one body this request keeps: the whole body
// while the traffic log is on, and the state-sized cap otherwise.
func (s *Server) captureLimit() int64 {
	if s.trafficOn() {
		return trafficCaptureBytes
	}
	return s.opts.CaptureRedactor.MaxCaptureBytes()
}

// trafficCaptureBytes bounds one body kept for the traffic log. It matches the
// read the agent's own request is allowed, so an exchange on disk is as
// whole as the request Relo already holds in memory.
const trafficCaptureBytes = maxInboundRead

// traceHalf hands one half of an exchange to the traffic log. The write leaves
// the request path, and the body becomes text on the worker: a copy as large as
// the request itself must never sit in front of the response. A full queue
// drops the half, because a trace is a debugging aid rather than a record.
func (s *Server) traceHalf(half TrafficHalf, message *CapturedMessage) {
	if message == nil || !s.trafficOn() {
		return
	}
	traffic, logger := s.opts.Traffic, s.opts.Logger
	queued := s.bookkeeping.enqueue(func() {
		half.Message = trafficMessage(message)
		if err := traffic.AppendHalf(half); err != nil {
			logger.Warn("write the traffic log", "request_id", half.RequestID, "error", err)
		}
	})
	if !queued {
		logger.Warn("traffic log is behind; dropped a half of the exchange",
			"request_id", half.RequestID, "kind", half.Kind)
	}
}

// traceAgentRequest writes the request the agent itself sent. It reaches the
// file as the request arrives, not when the answer finishes, so a request that
// hangs or dies is still there to read.
func (s *Server) traceAgentRequest(outcome *requestOutcome) {
	s.traceHalf(TrafficHalf{
		RequestID: outcome.requestID, Timestamp: traceStamp(), Kind: activity.CaptureAgentRequest,
		Surface: outcome.surface, Origin: outcome.origin,
	}, outcome.inbound)
}

// traceAttempt writes what one attempt sent a provider and what came back. Both
// halves are complete when the attempt ends, and they reach the file before the
// next attempt is made.
func (s *Server) traceAttempt(outcome *requestOutcome, ordinal int, attempt attemptRow) {
	if attempt.capture == nil {
		return
	}
	position := ordinal
	half := TrafficHalf{
		RequestID: outcome.requestID, Timestamp: traceStamp(), Ordinal: &position,
		Surface: outcome.surface, Origin: outcome.origin,
		Provider: attempt.providerID, Model: attempt.modelID,
		Credential: attempt.credential, Status: attempt.status,
	}
	request := half
	request.Kind = activity.CaptureProviderRequest
	s.traceHalf(request, attempt.capture.Request)
	response := half
	response.Kind = activity.CaptureProviderResponse
	s.traceHalf(response, attempt.capture.Response)
}

// traceAgentAnswer writes the answer the agent was given, which is the last
// half of the exchange to exist.
func (s *Server) traceAgentAnswer(outcome *requestOutcome) {
	if outcome.answer == nil {
		return
	}
	answer, ok := outcome.answer.captured()
	if !ok {
		return
	}
	s.traceHalf(TrafficHalf{
		RequestID: outcome.requestID, Timestamp: traceStamp(), Kind: activity.CaptureAgentResponse,
		Surface: outcome.surface, Origin: outcome.origin, Status: answer.Status,
	}, answer)
}

// trafficMessage turns one captured message into the shape the file holds.
func trafficMessage(message *CapturedMessage) *TrafficMessage {
	if message == nil {
		return nil
	}
	return &TrafficMessage{
		Method: message.Method, URL: message.URL, Status: message.Status,
		Headers: message.Headers, Body: string(message.Body), Truncated: message.Truncated,
	}
}

// traceStamp is the moment one half was written.
func traceStamp() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

// trafficTrace reads what the log kept of one request, resolving the event id
// the detail page holds to the request id the lines are keyed by. A request the
// log never wrote, or whose days have aged out, reports none rather than an
// error: the state-sized bodies still stand.
func (s *Server) trafficTrace(ctx context.Context, reads captureReader, eventID int64) (trafficTrace, bool) {
	if s.opts.Traffic == nil {
		return trafficTrace{}, false
	}
	requestID, err := reads.RequestID(ctx, eventID)
	if err != nil || requestID == "" {
		return trafficTrace{}, false
	}
	halves, err := s.opts.Traffic.Halves(ctx, requestID)
	if err != nil || len(halves) == 0 {
		return trafficTrace{}, false
	}
	return assembleTraffic(halves), true
}

// assembleTraffic folds the halves of one request back into its exchange.
func assembleTraffic(halves []TrafficHalf) trafficTrace {
	trace := trafficTrace{}
	for _, half := range halves {
		switch half.Kind {
		case activity.CaptureAgentRequest:
			trace.AgentRequest = half.Message
		case activity.CaptureAgentResponse:
			trace.AgentResponse = half.Message
		case activity.CaptureProviderRequest, activity.CaptureProviderResponse:
			if half.Ordinal == nil {
				continue
			}
			for ordinal := len(trace.Attempts); ordinal <= *half.Ordinal; ordinal++ {
				trace.Attempts = append(trace.Attempts, trafficAttemptHalf{})
			}
			if half.Kind == activity.CaptureProviderRequest {
				trace.Attempts[*half.Ordinal].Request = half.Message
			} else {
				trace.Attempts[*half.Ordinal].Response = half.Message
			}
		}
	}
	return trace
}

// wholeCaptureBody returns the body the traffic log kept for one capture, and
// whether it is there. A request the traffic log never wrote, or whose day has
// aged out, leaves the state-sized body to stand on its own.
func (s *Server) wholeCaptureBody(ctx context.Context, reads captureReader, eventID, captureID int64) ([]byte, bool) {
	trace, found := s.trafficTrace(ctx, reads, eventID)
	if !found {
		return nil, false
	}
	captures, err := reads.Captures(ctx, eventID)
	if err != nil {
		return nil, false
	}
	for _, capture := range captures {
		if capture.ID != captureID {
			continue
		}
		message := trafficMessageFor(trace, capture.Kind, capture.Ordinal)
		if message == nil {
			return nil, false
		}
		return []byte(message.Body), true
	}
	return nil, false
}
