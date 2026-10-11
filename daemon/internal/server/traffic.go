// The traffic log keeps whole agent and provider exchanges on disk when the
// operator turns it on: one JSON line per request, one file per day, aged out
// with the daemon log's own retention. The shapes here are what the relay
// hands the writer and what the request detail page reads back.
package server

import (
	"context"

	"github.com/jonaskahn/relo/internal/activity"
)

// captureReader lists the messages stored beside one request, which the
// detail page resolves a capture id against.
type captureReader interface {
	Captures(ctx context.Context, eventID int64) ([]activity.CaptureHeader, error)
}

// TrafficRecord is one request's exchange as the traffic log keeps it.
type TrafficRecord struct {
	Timestamp     string               `json:"timestamp"`
	EventID       int64                `json:"event_id"`
	RequestID     string               `json:"request_id"`
	Surface       string               `json:"surface"`
	Origin        string               `json:"origin"`
	Provider      string               `json:"provider"`
	Model         string               `json:"model"`
	Status        int                  `json:"status"`
	AgentRequest  *TrafficMessage      `json:"agent_request,omitempty"`
	AgentResponse *TrafficMessage      `json:"agent_response,omitempty"`
	Attempts      []TrafficAttemptLine `json:"attempts,omitempty"`
}

// TrafficAttemptLine is what one attempt sent a provider and what came back.
type TrafficAttemptLine struct {
	Ordinal    int             `json:"ordinal"`
	Provider   string          `json:"provider"`
	Model      string          `json:"model"`
	Credential string          `json:"credential,omitempty"`
	Status     int             `json:"status"`
	Request    *TrafficMessage `json:"request,omitempty"`
	Response   *TrafficMessage `json:"response,omitempty"`
}

// TrafficMessage is one HTTP message: the method or the status, the target,
// every header, and the body bytes. The bodies are already redacted.
type TrafficMessage struct {
	Method    string              `json:"method,omitempty"`
	URL       string              `json:"url,omitempty"`
	Status    int                 `json:"status,omitempty"`
	Headers   map[string][]string `json:"headers,omitempty"`
	Body      []byte              `json:"body,omitempty"`
	Truncated bool                `json:"truncated,omitempty"`
}

// TrafficLog is the daily JSONL file the finished request is written to, and
// the place the request detail page reads a whole body back from.
type TrafficLog interface {
	Append(record TrafficRecord) error
	Find(eventID int64) (TrafficRecord, bool, error)
}

// trafficMessageFor names the message of one stored capture inside a whole
// record, so the detail page can show the body the traffic log kept rather
// than the state-sized one it fell back to.
func trafficMessageFor(record TrafficRecord, kind string, ordinal *int) *TrafficMessage {
	switch kind {
	case activity.CaptureAgentRequest:
		return record.AgentRequest
	case activity.CaptureAgentResponse:
		return record.AgentResponse
	case activity.CaptureProviderRequest, activity.CaptureProviderResponse:
		if ordinal == nil {
			return nil
		}
		if *ordinal < 0 || *ordinal >= len(record.Attempts) {
			return nil
		}
		attempt := record.Attempts[*ordinal]
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

// wholeBodies names the captures of one request whose whole body the traffic
// log kept, so the manifest does not show them as cut short.
func (s *Server) wholeBodies(ctx context.Context, reads captureReader, eventID int64, captures []activity.CaptureHeader) map[int64]bool {
	record, found, err := s.opts.Traffic.Find(eventID)
	if err != nil || !found {
		return nil
	}
	whole := make(map[int64]bool, len(captures))
	for _, capture := range captures {
		if !capture.Truncated {
			continue
		}
		if message := trafficMessageFor(record, capture.Kind, capture.Ordinal); message != nil && !message.Truncated {
			whole[capture.ID] = true
		}
	}
	return whole
}

// wholeCaptureBody returns the body the traffic log kept for one capture, and
// whether it is there. A request the traffic log never wrote, or whose day has
// aged out, leaves the state-sized body to stand on its own.
func (s *Server) wholeCaptureBody(ctx context.Context, reads captureReader, eventID, captureID int64) ([]byte, bool) {
	if s.opts.Traffic == nil {
		return nil, false
	}
	captures, err := reads.Captures(ctx, eventID)
	if err != nil {
		return nil, false
	}
	var header *activity.CaptureHeader
	for i := range captures {
		if captures[i].ID == captureID {
			header = &captures[i]
			break
		}
	}
	if header == nil {
		return nil, false
	}
	record, found, err := s.opts.Traffic.Find(eventID)
	if err != nil || !found {
		return nil, false
	}
	message := trafficMessageFor(record, header.Kind, header.Ordinal)
	if message == nil {
		return nil, false
	}
	return message.Body, true
}
