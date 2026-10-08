package storage_test

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/jonaskahn/relo/internal/activity"
	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	"github.com/jonaskahn/relo/tests/testkit"
)

// TestSweepPurgesEveryCaptureAndKeepsTheNumbers covers the promise behind the
// console's cleanup: every captured body goes, the requests that produced
// them stay countable, and the file is compacted afterwards.
func TestSweepPurgesEveryCaptureAndKeepsTheNumbers(t *testing.T) {
	db := testkit.OpenTestDB(t)
	retention := newRetention(t, db, withDrain(true))
	ctx := context.Background()
	ids := seedDays(t, db, 5, 2)
	seedCaptures(t, ctx, db, ids)
	stored := countRows(t, db, "usage_captures")
	if err := retention.SaveConfig(ctx, sqlite.RetentionConfig{UsageDays: 3}); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	report, err := retention.Sweep(ctx)
	if err != nil {
		t.Fatalf("Sweep() error = %v", err)
	}
	if report.CapturesDeleted != int64(stored) {
		t.Fatalf("CapturesDeleted = %d, want every one of the %d stored", report.CapturesDeleted, stored)
	}
	if report.RowsDeleted == 0 {
		t.Fatal("Sweep() removed no requests, want the days outside the budget gone")
	}
	if report.FinalizedDays == 0 {
		t.Fatal("Sweep() closed no days, want the open ones aggregated")
	}
	if !report.Vacuumed {
		t.Fatal("Sweep() left the file uncompacted")
	}
	if report.LiveBytesAfter >= report.LiveBytesBefore {
		t.Fatalf("live bytes %d did not fall below %d", report.LiveBytesAfter, report.LiveBytesBefore)
	}
	if left := countRows(t, db, "usage_captures"); left != 0 {
		t.Fatalf("usage_captures holds %d rows, want none", left)
	}
	// The closed days kept their totals before anything was deleted, so the
	// requests a sweep removes are still counted.
	if archived := archivedRequests(t, db); archived != 8 {
		t.Fatalf("the archive holds %d requests, want the eight of the closed days", archived)
	}
}

// TestSweepRunsOnePassAtATime covers the lock every maintenance trigger
// shares: sweeps work one after another, so the captures they count and
// remove are never counted twice.
func TestSweepRunsOnePassAtATime(t *testing.T) {
	db := testkit.OpenTestDB(t)
	retention := newRetention(t, db, withDrain(true))
	ctx := context.Background()
	ids := seedDays(t, db, 5, 2)
	seedCaptures(t, ctx, db, ids)
	stored := countRows(t, db, "usage_captures")

	const passes = 6
	start := make(chan struct{})
	var wg sync.WaitGroup
	var removed atomic.Int64
	for i := 0; i < passes; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			report, err := retention.Sweep(ctx)
			if err != nil {
				if errors.Is(err, activity.ErrRetentionBusy) {
					return
				}
				t.Errorf("Sweep() error = %v", err)
				return
			}
			removed.Add(report.CapturesDeleted)
		}()
	}
	close(start)
	wg.Wait()
	if removed.Load() != int64(stored) {
		t.Fatalf("concurrent sweeps counted %d captures, want the %d stored removed once", removed.Load(), stored)
	}
	if left := countRows(t, db, "usage_captures"); left != 0 {
		t.Fatalf("usage_captures holds %d rows, want none", left)
	}
}

// TestSweepSkipsCompactionWhileRequestsAreInFlight covers the one step that
// waits for a quiet data plane: the sweep still removes every stored body
// and reports that the free space stays until a quieter moment.
func TestSweepSkipsCompactionWhileRequestsAreInFlight(t *testing.T) {
	db := testkit.OpenTestDB(t)
	retention := newRetention(t, db, withDrain(false))
	ctx := context.Background()
	ids := seedDays(t, db, 5, 2)
	seedCaptures(t, ctx, db, ids)

	report, err := retention.Sweep(ctx)
	if err != nil {
		t.Fatalf("Sweep() error = %v", err)
	}
	if report.Vacuumed {
		t.Fatal("Sweep() compacted while requests were in flight")
	}
	if report.CapturesDeleted != int64(len(ids)) {
		t.Fatalf("CapturesDeleted = %d, want every one of the %d stored", report.CapturesDeleted, len(ids))
	}
	if left := countRows(t, db, "usage_captures"); left != 0 {
		t.Fatalf("usage_captures holds %d rows, want none", left)
	}
}

func seedCaptures(t *testing.T, ctx context.Context, db *sqlite.DB, ids []int64) {
	t.Helper()
	store := sqlite.NewCaptureStore(db)
	for _, id := range ids {
		capture := sqlite.Capture{
			Kind: sqlite.CaptureAgentRequest, Method: http.MethodPost,
			URL: "/v1/chat/completions", Body: bytes.Repeat([]byte("x"), 4096),
		}
		if err := store.Append(ctx, id, []sqlite.Capture{capture}); err != nil {
			t.Fatalf("Append() error = %v", err)
		}
	}
}
