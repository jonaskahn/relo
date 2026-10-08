// Process log reads exposes the daemon's own log files to the console, so
// the logs page shows what the daemon wrote without opening the state
// directory.
package server

import (
	"errors"
	"net/http"

	appstatus "github.com/jonaskahn/relo/internal/application/status"
)

type daemonLineDTO struct {
	Offset      int64  `json:"offset"`
	TimestampMs int64  `json:"timestamp_ms"`
	Level       string `json:"level"`
	Message     string `json:"message"`
	Detail      string `json:"detail,omitempty"`
}

type daemonLogsResponse struct {
	Items      []daemonLineDTO `json:"items"`
	NextCursor string          `json:"next_cursor,omitempty"`
	End        string          `json:"end"`
}

type startupEntryDTO struct {
	Name      string `json:"name"`
	StartedAt string `json:"started_at"`
	Outcome   string `json:"outcome"`
	Error     string `json:"error,omitempty"`
	Current   bool   `json:"current"`
	Size      int64  `json:"size"`
}

type startupListResponse struct {
	Items []startupEntryDTO `json:"items"`
}

type startupTranscriptResponse struct {
	Entry     startupEntryDTO `json:"entry"`
	Lines     []daemonLineDTO `json:"lines"`
	Truncated bool            `json:"truncated"`
}

func (s *Server) handleDaemonLogs(w http.ResponseWriter, r *http.Request) {
	reads, ok := s.statusOrUnavailable(w, r)
	if !ok {
		return
	}
	limit, err := queryInt(r, "limit", 200)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	query := r.URL.Query()
	page, err := reads.DaemonLogs(r.Context(), appstatus.DaemonLogQuery{
		Limit: limit, Before: query.Get("before"), After: query.Get("after"),
		Level: query.Get("level"), HideRequests: hideRequests(query.Get("hide_requests")),
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	items := make([]daemonLineDTO, 0, len(page.Lines))
	for _, line := range page.Lines {
		items = append(items, daemonLineDTO{
			Offset: line.Offset, TimestampMs: line.TimestampMs,
			Level: line.Level, Message: line.Message, Detail: line.Detail,
		})
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, daemonLogsResponse{Items: items, NextCursor: page.NextCursor, End: page.End})
}

func (s *Server) handleStartupList(w http.ResponseWriter, r *http.Request) {
	reads, ok := s.statusOrUnavailable(w, r)
	if !ok {
		return
	}
	entries, err := reads.Startups(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	items := make([]startupEntryDTO, 0, len(entries))
	for _, entry := range entries {
		items = append(items, startupEntry(entry))
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, startupListResponse{Items: items})
}

func (s *Server) handleStartupTranscript(w http.ResponseWriter, r *http.Request) {
	reads, ok := s.statusOrUnavailable(w, r)
	if !ok {
		return
	}
	transcript, err := reads.StartupTranscript(r.Context(), r.PathValue("name"))
	if err != nil {
		if errors.Is(err, appstatus.ErrLogNotFound) {
			s.fail(w, r, refusal{Status: http.StatusNotFound, Code: "not_found", Message: "api.logs.startup_missing"})
			return
		}
		writeServiceError(w, err)
		return
	}
	lines := make([]daemonLineDTO, 0, len(transcript.Lines))
	for _, line := range transcript.Lines {
		lines = append(lines, daemonLineDTO{
			Offset: line.Offset, TimestampMs: line.TimestampMs,
			Level: line.Level, Message: line.Message, Detail: line.Detail,
		})
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, startupTranscriptResponse{
		Entry: startupEntry(transcript.Entry), Lines: lines, Truncated: transcript.Truncated,
	})
}

func startupEntry(entry appstatus.StartupEntry) startupEntryDTO {
	return startupEntryDTO{
		Name: entry.Name, StartedAt: entry.StartedAt.UTC().Format("2006-01-02T15:04:05Z"),
		Outcome: entry.Outcome, Error: entry.Error, Current: entry.Current, Size: entry.Size,
	}
}

func hideRequests(raw string) bool {
	return raw == "1" || raw == "true"
}
