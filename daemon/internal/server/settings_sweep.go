// Retention sweep job: running cleanup and reporting its status.
package server

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/jonaskahn/relo/internal/activity"
)

const (
	sweepIdle    = "idle"
	sweepWorking = "working"
	sweepDone    = "done"
	sweepFailed  = "failed"
)

type sweepStatus struct {
	State       string       `json:"state"`
	StartedAtMs int64        `json:"started_at_ms,omitempty"`
	EndedAtMs   int64        `json:"ended_at_ms,omitempty"`
	Report      *sweepReport `json:"report,omitempty"`
	Error       string       `json:"error,omitempty"`
}

type sweepReport struct {
	FinalizedDays   int   `json:"finalized_days"`
	RowsDeleted     int64 `json:"rows_deleted"`
	CapturesDeleted int64 `json:"captures_deleted"`
	LiveBytesBefore int64 `json:"live_bytes_before"`
	LiveBytesAfter  int64 `json:"live_bytes_after"`
	Vacuumed        bool  `json:"vacuumed"`
}

type sweepJob struct {
	mu      sync.Mutex
	state   string
	started time.Time
	ended   time.Time
	report  *sweepReport
	failure string
}

func (j *sweepJob) start() bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.state == sweepWorking {
		return false
	}
	j.state = sweepWorking
	j.started = time.Now()
	j.ended = time.Time{}
	j.report = nil
	j.failure = ""
	return true
}

func (j *sweepJob) finish(report activity.CleanupReport, err error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.ended = time.Now()
	if err != nil {
		j.state = sweepFailed
		j.failure = err.Error()
		return
	}
	j.state = sweepDone
	j.report = &sweepReport{
		FinalizedDays: report.FinalizedDays, RowsDeleted: report.RowsDeleted,
		CapturesDeleted: report.CapturesDeleted,
		LiveBytesBefore: report.LiveBytesBefore, LiveBytesAfter: report.LiveBytesAfter,
		Vacuumed: report.Vacuumed,
	}
}

func (j *sweepJob) status() sweepStatus {
	j.mu.Lock()
	defer j.mu.Unlock()
	snapshot := sweepStatus{State: j.state, Error: j.failure}
	if !j.started.IsZero() {
		snapshot.StartedAtMs = j.started.UnixMilli()
	}
	if !j.ended.IsZero() {
		snapshot.EndedAtMs = j.ended.UnixMilli()
	}
	if j.report != nil {
		taken := *j.report
		snapshot.Report = &taken
	}
	return snapshot
}

func (s *Server) handleStartSweep(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.settingsOrUnavailable(w, r); !ok {
		return
	}
	if !s.sweep.start() {
		s.fail(w, r, refusal{
			Status: http.StatusConflict, Code: "action_in_progress", Message: "api.settings.sweep.busy",
		})
		return
	}
	go s.runSweep(context.WithoutCancel(r.Context()))
	writeJSON(w, http.StatusAccepted, s.sweep.status())
}

func (s *Server) handleGetSweep(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.sweep.status())
}

func (s *Server) runSweep(ctx context.Context) {
	report, err := s.opts.Settings.Sweep(ctx)
	s.sweep.finish(report, err)
}
