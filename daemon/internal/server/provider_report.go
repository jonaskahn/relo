// Provider reports expose bounded, masked exchanges of failed upstream attempts.
package server

import (
	"context"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/jonaskahn/relo/internal/activity"
	appactivity "github.com/jonaskahn/relo/internal/application/activity"
	"github.com/jonaskahn/relo/internal/capture"
)

type providerReport struct {
	RequestID   string                  `json:"request_id"`
	ReloVersion string                  `json:"relo_version"`
	GeneratedAt string                  `json:"generated_at"`
	Attempts    []providerReportAttempt `json:"attempts"`
}

type providerReportAttempt struct {
	Ordinal    int                    `json:"ordinal"`
	Provider   string                 `json:"provider"`
	Model      string                 `json:"model"`
	Status     int                    `json:"status"`
	DurationMs int64                  `json:"duration_ms"`
	Request    *providerReportMessage `json:"request,omitempty"`
	Response   *providerReportMessage `json:"response,omitempty"`
}

type providerReportMessage struct {
	Method    string              `json:"method,omitempty"`
	URL       string              `json:"url,omitempty"`
	Status    int                 `json:"status,omitempty"`
	Headers   map[string][]string `json:"headers"`
	Body      string              `json:"body"`
	BodyBytes int64               `json:"body_bytes"`
	Truncated bool                `json:"truncated"`
}

func (s *Server) handleProviderReport(w http.ResponseWriter, r *http.Request) {
	reads, ok := s.activityOrUnavailable(w, r)
	if !ok {
		return
	}
	eventID, ok := s.captureEventID(w, r)
	if !ok {
		return
	}
	report, err := s.providerReport(r.Context(), reads, eventID)
	if err != nil {
		s.failCaptureRead(w, r, err)
		return
	}
	if len(report.Attempts) == 0 {
		s.fail(w, r, refusal{Status: http.StatusNotFound, Code: "not_found", Message: "api.logs.provider_error_missing"})
		return
	}
	report.RequestID, err = reads.RequestID(r.Context(), eventID)
	if err != nil {
		s.failCaptureRead(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, report)
}

func (s *Server) providerReport(ctx context.Context, reads *appactivity.Service, eventID int64) (providerReport, error) {
	report := providerReport{ReloVersion: s.opts.Version, GeneratedAt: s.opts.Clock.Now().UTC().Format(time.RFC3339)}
	captures, err := reads.Captures(ctx, eventID)
	if err != nil {
		return report, err
	}
	selected := failedProviderAttempts(captures)
	if len(selected) == 0 {
		return report, nil
	}
	attempts, err := reads.Attempts(ctx, eventID)
	if err != nil {
		return report, err
	}

	fillProviderAttemptFacts(selected, attempts)

	for _, row := range captures {
		if row.Ordinal == nil {
			continue
		}
		if err := s.fillProviderMessage(ctx, reads, providerReportCapture{eventID: eventID, header: row}, selected[*row.Ordinal]); err != nil {
			return report, err
		}
	}

	report.Attempts = orderedProviderAttempts(selected)

	return report, nil
}

func failedProviderAttempts(captures []activity.CaptureHeader) map[int]*providerReportAttempt {
	selected := make(map[int]*providerReportAttempt)
	for _, row := range captures {
		if row.Ordinal != nil && row.Kind == activity.CaptureProviderResponse && row.Status >= 400 {
			selected[*row.Ordinal] = &providerReportAttempt{Ordinal: *row.Ordinal, Status: row.Status}
		}
	}
	return selected
}

type providerReportCapture struct {
	eventID int64
	header  activity.CaptureHeader
}

func (s *Server) fillProviderMessage(ctx context.Context, reads *appactivity.Service, stored providerReportCapture, entry *providerReportAttempt) error {
	row := stored.header
	if entry == nil || (row.Kind != activity.CaptureProviderRequest && row.Kind != activity.CaptureProviderResponse) {
		return nil
	}
	body, err := reads.CaptureBody(ctx, stored.eventID, row.ID)
	if err != nil {
		return err
	}
	message := s.providerReportMessage(row, body)
	if row.Kind == activity.CaptureProviderRequest {
		entry.Request = message
	} else {
		entry.Response = message
	}
	return nil
}

func (s *Server) providerReportMessage(row activity.CaptureHeader, body activity.CaptureBody) *providerReportMessage {
	kept, truncated := s.opts.CaptureRedactor.CaptureBytes(body.Body, capture.ProviderMaxBytes, true)
	clean := strings.ToValidUTF8(string(s.opts.CaptureRedactor.RedactBody(kept)), "�")
	message := &providerReportMessage{
		Headers: s.opts.CaptureRedactor.RedactHeaders(row.Headers), Body: clean,
		BodyBytes: int64(len(clean)), Truncated: row.Truncated || body.Truncated || truncated,
	}
	if row.Kind == activity.CaptureProviderRequest {
		message.Method, message.URL = row.Method, providerReportURL(row.URL)
	} else {
		message.Status = row.Status
	}
	return message
}

func providerReportURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	parsed.RawQuery, parsed.Fragment, parsed.User, parsed.ForceQuery = "", "", nil, false
	return parsed.String()
}

func fillProviderAttemptFacts(selected map[int]*providerReportAttempt, attempts []activity.UsageAttemptRow) {
	for _, attempt := range attempts {
		if entry := selected[attempt.Ordinal]; entry != nil {
			entry.Provider, entry.Model, entry.DurationMs = attempt.Provider, attempt.Model, attempt.DurationMs
		}
	}
}

func orderedProviderAttempts(selected map[int]*providerReportAttempt) []providerReportAttempt {
	attempts := make([]providerReportAttempt, 0, len(selected))
	for _, entry := range selected {
		attempts = append(attempts, *entry)
	}
	sort.Slice(attempts, func(i, j int) bool { return attempts[i].Ordinal < attempts[j].Ordinal })
	return attempts
}
