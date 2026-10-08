package server

import (
	"errors"
	"testing"

	"github.com/jonaskahn/relo/internal/activity"
)

// TestSweepJobRefusesASecondStart covers the gate behind the console's 409:
// one sweep runs at a time, and a finished or failed run releases the next.
func TestSweepJobRefusesASecondStart(t *testing.T) {
	job := sweepJob{state: sweepIdle}
	if !job.start() {
		t.Fatal("the first start was refused")
	}
	if job.start() {
		t.Fatal("a second start claimed a run in flight")
	}
	job.finish(activity.CleanupReport{RowsDeleted: 2}, nil)
	if !job.start() {
		t.Fatal("a finished sweep still held the run")
	}
	if job.start() {
		t.Fatal("a second start claimed a run in flight")
	}
	job.finish(activity.CleanupReport{}, errors.New("boom"))
	if got := job.status(); got.State != sweepFailed || got.Error != "boom" {
		t.Fatalf("status = %+v, want the failure reported", got)
	}
	if !job.start() {
		t.Fatal("a failed sweep still held the run")
	}
}

// TestSweepJobReportsTheReport covers what the console polls: the working
// phase while the run is going, and the report once it is done.
func TestSweepJobReportsTheReport(t *testing.T) {
	job := sweepJob{state: sweepIdle}
	job.start()
	if got := job.status(); got.State != sweepWorking {
		t.Fatalf("status = %+v, want a working run", got)
	}
	job.finish(activity.CleanupReport{
		FinalizedDays: 2, RowsDeleted: 10, CapturesDeleted: 4,
		LiveBytesBefore: 1 << 30, LiveBytesAfter: 1 << 20, Vacuumed: true,
	}, nil)
	got := job.status()
	if got.State != sweepDone || got.Report == nil {
		t.Fatalf("status = %+v, want a done run with its report", got)
	}
	if got.Report.CapturesDeleted != 4 || got.Report.LiveBytesAfter != 1<<20 || !got.Report.Vacuumed {
		t.Fatalf("report = %+v, want the stored figures", got.Report)
	}
	if got.StartedAtMs == 0 || got.EndedAtMs == 0 {
		t.Fatalf("status = %+v, want the run timed", got)
	}
}
