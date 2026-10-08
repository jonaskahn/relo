package server_test

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

// sweepStatus is the shape the console reads one maintenance sweep in.
type sweepStatus struct {
	State  string `json:"state"`
	Error  string `json:"error"`
	Report *struct {
		FinalizedDays   int   `json:"finalized_days"`
		RowsDeleted     int64 `json:"rows_deleted"`
		CapturesDeleted int64 `json:"captures_deleted"`
		LiveBytesBefore int64 `json:"live_bytes_before"`
		LiveBytesAfter  int64 `json:"live_bytes_after"`
		Vacuumed        bool  `json:"vacuumed"`
	} `json:"report"`
}

// TestSweepPurgesBodiesAndCompacts covers the whole action: the sweep purges
// every stored message body and compacts the database, and serving answers
// while it runs.
func TestSweepPurgesBodiesAndCompacts(t *testing.T) {
	h := newHarness(t)
	eventID, _ := seedCapturedRequest(t, h)

	started := h.management(http.MethodPost, "/api/v1/settings/retention/sweep", adminToken, nil)
	if started.Code != http.StatusAccepted {
		t.Fatalf("sweep status = %d, body = %s", started.Code, started.Body.String())
	}
	// Serving never stops for a sweep: the data plane answers while one runs.
	if served := h.dataPlane(http.MethodPost, "/v1/chat/completions", dataPlaneToken, strings.NewReader(streamingBody())); served.Code != http.StatusOK {
		t.Fatalf("inference during a sweep status = %d, body = %s", served.Code, served.Body.String())
	}

	finished := waitForSweep(t, h)
	if finished.State != "done" {
		t.Fatalf("sweep = %+v, want done", finished)
	}
	// The seeded captures and the messages of the request that overlapped it
	// go together: no body a capture held survives a sweep.
	if finished.Report == nil || finished.Report.CapturesDeleted < 3 {
		t.Fatalf("sweep report = %+v, want at least the three seeded captures removed", finished.Report)
	}
	if !finished.Report.Vacuumed {
		t.Fatalf("sweep report = %+v, want a compacted database", finished.Report)
	}
	purged := h.management(http.MethodGet, "/api/v1/activity/requests/"+strconv.FormatInt(eventID, 10)+"/captures", adminToken, nil)
	if purged.Code != http.StatusOK {
		t.Fatalf("capture manifest after a sweep status = %d, body = %s", purged.Code, purged.Body.String())
	}
	var manifest struct {
		Items []json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(purged.Body.Bytes(), &manifest); err != nil {
		t.Fatalf("decode the capture manifest: %v", err)
	}
	if len(manifest.Items) != 0 {
		t.Fatalf("capture manifest after a sweep = %s, want none left", purged.Body.String())
	}
}

// TestFailedSweepReportsItsErrorAndReleasesTheRun covers the failure path: a
// sweep that cannot reach the database says so, and the next one may start.
func TestFailedSweepReportsItsErrorAndReleasesTheRun(t *testing.T) {
	h := newHarness(t)
	if err := h.db.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	started := h.management(http.MethodPost, "/api/v1/settings/retention/sweep", adminToken, nil)
	if started.Code != http.StatusAccepted {
		t.Fatalf("sweep status = %d, body = %s", started.Code, started.Body.String())
	}
	failed := waitForSweep(t, h)
	if failed.State != "failed" || failed.Error == "" {
		t.Fatalf("sweep = %+v, want a failed state carrying its error", failed)
	}
	if retry := h.management(http.MethodPost, "/api/v1/settings/retention/sweep", adminToken, nil); retry.Code != http.StatusAccepted {
		t.Fatalf("sweep after a failure status = %d, body = %s", retry.Code, retry.Body.String())
	}
}

func readSweep(t *testing.T, h *harness) sweepStatus {
	t.Helper()
	response := h.management(http.MethodGet, "/api/v1/settings/retention/sweep", adminToken, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("sweep status = %d, body = %s", response.Code, response.Body.String())
	}
	var status sweepStatus
	if err := json.Unmarshal(response.Body.Bytes(), &status); err != nil {
		t.Fatalf("decode the sweep status: %v", err)
	}
	return status
}

func waitForSweep(t *testing.T, h *harness) sweepStatus {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		status := readSweep(t, h)
		if status.State == "done" || status.State == "failed" {
			return status
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the sweep never finished")
	return sweepStatus{}
}
