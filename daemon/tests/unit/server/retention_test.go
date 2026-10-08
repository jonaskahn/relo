package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/activity"
	"github.com/jonaskahn/relo/internal/adapters/sqlite"
)

// TestRetentionRefusesABudgetBelowTheFloor pins the floor: the console can
// store 0 or the floor and up, and a shorter window is answered with the
// floor named.
func TestRetentionRefusesABudgetBelowTheFloor(t *testing.T) {
	h := newHarness(t)

	refused := h.management(http.MethodPut, "/api/v1/settings/retention", adminToken,
		strings.NewReader(`{"usage_days":2,"max_events":0,"max_bytes":0}`))
	if refused.Code != http.StatusBadRequest {
		t.Fatalf("short retention status = %d, body = %s", refused.Code, refused.Body.String())
	}
	var body struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(refused.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode the refusal: %v", err)
	}
	if !strings.Contains(body.Error.Message, strconv.Itoa(activity.MinUsageDays)) {
		t.Fatalf("refusal message = %q, want the floor named", body.Error.Message)
	}

	allowed := h.management(http.MethodPut, "/api/v1/settings/retention", adminToken,
		strings.NewReader(`{"usage_days":`+strconv.Itoa(activity.MinUsageDays)+`,"max_events":0,"max_bytes":0}`))
	if allowed.Code != http.StatusOK {
		t.Fatalf("floor retention status = %d, body = %s", allowed.Code, allowed.Body.String())
	}
}

// TestRunRetentionAppliesTheStoredBudget answers the console's cleanup
// action with what the run removed.
func TestRunRetentionAppliesTheStoredBudget(t *testing.T) {
	h := newHarness(t)
	h.seedOldUsageEvent(t)
	store := sqlite.NewRetention(h.db, sqlite.RetentionOptions{Now: h.clock.Now})
	if err := store.SaveConfig(context.Background(), sqlite.RetentionConfig{UsageDays: 3}); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	response := h.management(http.MethodPost, "/api/v1/settings/retention/run", adminToken, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("run retention status = %d, body = %s", response.Code, response.Body.String())
	}
	var report struct {
		RowsDeleted int64 `json:"RowsDeleted"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &report); err != nil {
		t.Fatalf("decode the report: %v", err)
	}
	if report.RowsDeleted != 1 {
		t.Fatalf("RowsDeleted = %d, want the one seeded row", report.RowsDeleted)
	}
	if left := countUsageEvents(t, h); left != 0 {
		t.Fatalf("usage_events holds %d rows, want 0", left)
	}
}

// TestSavingRetentionCleansTheLog covers the trigger behind the save: the
// budget written by the console is applied without waiting for a tick.
func TestSavingRetentionCleansTheLog(t *testing.T) {
	h := newHarness(t)
	h.seedOldUsageEvent(t)

	saved := h.management(http.MethodPut, "/api/v1/settings/retention", adminToken,
		strings.NewReader(`{"usage_days":3,"max_events":0,"max_bytes":0}`))
	if saved.Code != http.StatusOK {
		t.Fatalf("save settings status = %d, body = %s", saved.Code, saved.Body.String())
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if countUsageEvents(t, h) == 0 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("usage_events still holds %d rows after a save, want the cleanup", countUsageEvents(t, h))
}

// seedOldUsageEvent writes one event far outside the three-day floor the
// harness clock anchors.
func (h *harness) seedOldUsageEvent(t *testing.T) {
	t.Helper()
	recorder, err := sqlite.NewUsageRecorder(h.db, nil)
	if err != nil {
		t.Fatalf("NewUsageRecorder() error = %v", err)
	}
	defer func() { _ = recorder.Close() }()
	if _, err := recorder.AppendEvent(context.Background(), sqlite.UsageEvent{
		RequestID: "req-old", Timestamp: time.Date(2023, 10, 1, 0, 0, 0, 0, time.UTC).UnixMilli(),
		Provider: "openai", Model: "gpt-4o", Status: 200,
	}); err != nil {
		t.Fatalf("AppendEvent() error = %v", err)
	}
}

func countUsageEvents(t *testing.T, h *harness) int {
	t.Helper()
	var count int
	if err := h.db.SQL().QueryRowContext(context.Background(), "SELECT count(*) FROM usage_events").Scan(&count); err != nil {
		t.Fatalf("count usage events: %v", err)
	}
	return count
}
