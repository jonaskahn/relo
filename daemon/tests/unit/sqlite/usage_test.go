package storage_test

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	"github.com/jonaskahn/relo/tests/testkit"
)

func TestUsageRecorderAppendEvent(t *testing.T) {
	t.Run("append event round-trip", func(t *testing.T) {
		db := testkit.OpenTestDB(t)
		recorder := newRecorder(t, db)
		cost := int64(42)
		event := sqlite.UsageEvent{
			RequestID:           "req-1",
			Timestamp:           1700000000,
			Provider:            "openai",
			Model:               "gpt-4o",
			CredentialLabel:     "default",
			Surface:             "chat-completions",
			Status:              200,
			DurationMs:          1234,
			InputTokens:         120,
			OutputTokens:        45,
			CacheReadTokens:     20,
			CacheWriteTokens:    5,
			EstimatedCostMicros: &cost,
			RouteProvider:       "openai",
			RouteReason:         "pinned-provider",
		}
		id, err := recorder.AppendEvent(context.Background(), event)
		if err != nil {
			t.Fatalf("AppendEvent() error = %v", err)
		}
		stored := readEvent(t, db, id)
		if stored.RequestID != event.RequestID || stored.Timestamp != event.Timestamp {
			t.Fatalf("stored identity = %+v, want %+v", stored, event)
		}
		if stored.Status != event.Status || stored.DurationMs != event.DurationMs {
			t.Fatalf("stored outcome = %+v, want %+v", stored, event)
		}
		if stored.InputTokens != 120 || stored.OutputTokens != 45 ||
			stored.CacheReadTokens != 20 || stored.CacheWriteTokens != 5 {
			t.Fatalf("stored tokens = %+v, want the recorded counts", stored)
		}
		if stored.EstimatedCostMicros == nil || *stored.EstimatedCostMicros != cost {
			t.Fatalf("stored cost = %v, want %d", stored.EstimatedCostMicros, cost)
		}
		if stored.RouteReason != event.RouteReason || stored.Surface != event.Surface {
			t.Fatalf("stored routing = %+v, want %+v", stored, event)
		}
	})

	t.Run("append event with nil cost", func(t *testing.T) {
		db := testkit.OpenTestDB(t)
		recorder := newRecorder(t, db)
		id, err := recorder.AppendEvent(context.Background(), sqlite.UsageEvent{Provider: "openai", Model: "gpt-4o", Status: 200})
		if err != nil {
			t.Fatalf("AppendEvent() error = %v", err)
		}
		if cost := readEvent(t, db, id).EstimatedCostMicros; cost != nil {
			t.Fatalf("EstimatedCostMicros = %v, want nil", cost)
		}
	})

	t.Run("event ID auto-increments", func(t *testing.T) {
		db := testkit.OpenTestDB(t)
		recorder := newRecorder(t, db)
		first, err := recorder.AppendEvent(context.Background(), sqlite.UsageEvent{Provider: "openai", Model: "gpt-4o", Status: 200})
		if err != nil {
			t.Fatalf("AppendEvent() error = %v", err)
		}
		second, err := recorder.AppendEvent(context.Background(), sqlite.UsageEvent{Provider: "openai", Model: "gpt-4o", Status: 200})
		if err != nil {
			t.Fatalf("AppendEvent() error = %v", err)
		}
		if second <= first {
			t.Fatalf("second ID = %d, want greater than %d", second, first)
		}
	})

	t.Run("append without the usage tables reports an error", func(t *testing.T) {
		logger, _ := testkit.TestLogger(t)
		db, err := sqlite.OpenDB(filepath.Join(t.TempDir(), "state.sqlite"), logger)
		if err != nil {
			t.Fatalf("OpenDB() error = %v", err)
		}
		defer func() { _ = db.Close() }()
		recorder, err := sqlite.NewUsageRecorder(db, logger)
		if err != nil {
			assertMissingTable(t, "NewUsageRecorder", err)
			return
		}
		defer func() { _ = recorder.Close() }()
		_, err = recorder.AppendEvent(context.Background(), sqlite.UsageEvent{Provider: "openai", Model: "gpt-4o"})
		assertMissingTable(t, "AppendEvent", err)
		assertMissingTable(t, "AppendAttempt", recorder.AppendAttempt(context.Background(), sqlite.UsageAttempt{EventID: 1}))
	})

	t.Run("close is idempotent", func(t *testing.T) {
		db := testkit.OpenTestDB(t)
		recorder := newRecorder(t, db)
		if err := recorder.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
		if err := recorder.Close(); err != nil {
			t.Fatalf("second Close() error = %v, want nil", err)
		}
	})

}

func TestUsageRecorderAppendAttempt(t *testing.T) {
	t.Run("append attempt linked to event", func(t *testing.T) {
		db := testkit.OpenTestDB(t)
		recorder := newRecorder(t, db)
		ctx := context.Background()
		eventID, err := recorder.AppendEvent(ctx, sqlite.UsageEvent{Provider: "openai", Model: "gpt-4o", Status: 200})
		if err != nil {
			t.Fatalf("AppendEvent() error = %v", err)
		}
		attempt := sqlite.UsageAttempt{
			EventID: eventID, Ordinal: 0, Provider: "openai", Model: "gpt-4o",
			Status: 200, DurationMs: 900, InputTokens: 10, OutputTokens: 4,
		}
		if err := recorder.AppendAttempt(ctx, attempt); err != nil {
			t.Fatalf("AppendAttempt() error = %v", err)
		}
		if got := countRows(t, db, "usage_attempts"); got != 1 {
			t.Fatalf("usage_attempts rows = %d, want 1", got)
		}
		var ordinal, inputTokens int
		var parent int64
		row := db.SQL().QueryRow("SELECT event_id, ordinal, input_tokens FROM usage_attempts")
		if err := row.Scan(&parent, &ordinal, &inputTokens); err != nil {
			t.Fatalf("read attempt: %v", err)
		}
		if parent != eventID || ordinal != 0 || inputTokens != 10 {
			t.Fatalf("attempt = (%d, %d, %d), want (%d, 0, 10)", parent, ordinal, inputTokens, eventID)
		}
	})

	t.Run("multiple attempts with ordinals", func(t *testing.T) {
		db := testkit.OpenTestDB(t)
		recorder := newRecorder(t, db)
		ctx := context.Background()
		eventID, err := recorder.AppendEvent(ctx, sqlite.UsageEvent{Provider: "openai", Model: "gpt-4o", Status: 200})
		if err != nil {
			t.Fatalf("AppendEvent() error = %v", err)
		}
		for ordinal := 0; ordinal < 3; ordinal++ {
			attempt := sqlite.UsageAttempt{EventID: eventID, Ordinal: ordinal, Provider: "openai", Model: "gpt-4o", Status: 429}
			if err := recorder.AppendAttempt(ctx, attempt); err != nil {
				t.Fatalf("AppendAttempt(%d) error = %v", ordinal, err)
			}
		}
		if got := countRows(t, db, "usage_attempts"); got != 3 {
			t.Fatalf("usage_attempts rows = %d, want 3", got)
		}
	})

	t.Run("attempt without a parent event is rejected", func(t *testing.T) {
		db := testkit.OpenTestDB(t)
		recorder := newRecorder(t, db)
		err := recorder.AppendAttempt(context.Background(), sqlite.UsageAttempt{EventID: 4242, Provider: "openai", Model: "gpt-4o"})
		if err == nil {
			t.Fatal("AppendAttempt() error = nil, want a foreign key failure")
		}
	})

	t.Run("concurrent appends no conflict", func(t *testing.T) {
		db := testkit.OpenTestDB(t)
		recorder := newRecorder(t, db)
		ctx := context.Background()
		errs := make(chan error, 100)
		var wg sync.WaitGroup
		for worker := 0; worker < 10; worker++ {
			wg.Add(1)
			go func(worker int) {
				defer wg.Done()
				for index := 0; index < 10; index++ {
					_, err := recorder.AppendEvent(ctx, sqlite.UsageEvent{
						RequestID: fmt.Sprintf("req-%d-%d", worker, index),
						Provider:  "openai", Model: "gpt-4o", Status: 200,
					})
					errs <- err
				}
			}(worker)
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatalf("AppendEvent() error = %v", err)
			}
		}
		if got := countRows(t, db, "usage_events"); got != 100 {
			t.Fatalf("usage_events rows = %d, want 100", got)
		}
	})
}

// assertMissingTable requires a call to fail because a table it needs is absent.
func assertMissingTable(t *testing.T, call string, err error) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s() error = nil, want a missing-table failure", call)
	}
	if !strings.Contains(err.Error(), "no such table") {
		t.Fatalf("%s() error = %v, want a missing-table failure", call, err)
	}
}

func newRecorder(t *testing.T, db *sqlite.DB) *sqlite.UsageRecorder {
	t.Helper()
	logger, _ := testkit.TestLogger(t)
	recorder, err := sqlite.NewUsageRecorder(db, logger)
	if err != nil {
		t.Fatalf("NewUsageRecorder() error = %v", err)
	}
	t.Cleanup(func() { _ = recorder.Close() })
	return recorder
}

func readEvent(t *testing.T, db *sqlite.DB, id int64) sqlite.UsageEvent {
	t.Helper()
	const query = "SELECT request_id, timestamp, provider, model, credential_label, surface, status, duration_ms," +
		" input_tokens, output_tokens, cache_read_tokens, cache_write_tokens, estimated_cost_micros, route_provider, route_reason" +
		" FROM usage_events WHERE id = ?"
	var (
		event       sqlite.UsageEvent
		label       sql.NullString
		surface     sql.NullString
		requestID   sql.NullString
		cost        sql.NullInt64
		routeProv   sql.NullString
		routeReason sql.NullString
	)
	row := db.SQL().QueryRow(query, id)
	if err := row.Scan(&requestID, &event.Timestamp, &event.Provider, &event.Model, &label, &surface,
		&event.Status, &event.DurationMs, &event.InputTokens, &event.OutputTokens,
		&event.CacheReadTokens, &event.CacheWriteTokens, &cost, &routeProv, &routeReason); err != nil {
		t.Fatalf("read usage event %d: %v", id, err)
	}
	event.RequestID = requestID.String
	event.CredentialLabel = label.String
	event.Surface = surface.String
	event.RouteProvider = routeProv.String
	event.RouteReason = routeReason.String
	if cost.Valid {
		value := cost.Int64
		event.EstimatedCostMicros = &value
	}
	return event
}

func countRows(t *testing.T, db *sqlite.DB, table string) int {
	t.Helper()
	var count int
	if err := db.SQL().QueryRow("SELECT count(*) FROM " + table).Scan(&count); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return count
}

func TestUsageRouteColumns(t *testing.T) {
	t.Run("migration adds columns", func(t *testing.T) {
		db := testkit.OpenTestDB(t)
		for _, column := range []string{"estimated_cost_micros", "route_provider", "route_reason"} {
			if !columnExists(t, db, "usage_events", column) {
				t.Fatalf("usage_events is missing %s", column)
			}
		}
	})

	t.Run("direct route -> reason recorded", func(t *testing.T) {
		db := testkit.OpenTestDB(t)
		recorder := newRecorder(t, db)
		cost := int64(12_500)
		id, err := recorder.AppendEvent(context.Background(), sqlite.UsageEvent{
			RequestID: "request-1", Provider: "openai", Model: "gpt-4o", Status: 200,
			EstimatedCostMicros: &cost, RouteProvider: "openai", RouteReason: "catalog",
		})
		if err != nil {
			t.Fatalf("AppendEvent() error = %v", err)
		}
		stored := readEvent(t, db, id)
		if stored.RouteProvider != "openai" || stored.RouteReason != "catalog" {
			t.Fatalf("stored route = %+v, want the decision recorded", stored)
		}
		if stored.EstimatedCostMicros == nil || *stored.EstimatedCostMicros != cost {
			t.Fatalf("stored cost = %v, want %d micros", stored.EstimatedCostMicros, cost)
		}
	})

	t.Run("combo failover -> each attempt distinct", func(t *testing.T) {
		db := testkit.OpenTestDB(t)
		recorder := newRecorder(t, db)
		id, err := recorder.AppendEvent(context.Background(), sqlite.UsageEvent{
			RequestID: "request-1", Provider: "tencent-coding-plan", Model: "tc-code-latest", Status: 200,
			RouteProvider: "tencent-coding-plan", RouteReason: "combo-failover",
		})
		if err != nil {
			t.Fatalf("AppendEvent() error = %v", err)
		}
		attempts := []sqlite.UsageAttempt{
			{EventID: id, Ordinal: 0, Provider: "anthropic", Model: "claude-sonnet-5", Status: 500},
			{EventID: id, Ordinal: 1, Provider: "tencent-coding-plan", Model: "tc-code-latest", Status: 200},
		}
		for _, attempt := range attempts {
			if err := recorder.AppendAttempt(context.Background(), attempt); err != nil {
				t.Fatalf("AppendAttempt() error = %v", err)
			}
		}
		if events := countRows(t, db, "usage_events"); events != 1 {
			t.Fatalf("usage_events rows = %d, want one per logical request", events)
		}
		if attempts := countRows(t, db, "usage_attempts"); attempts != 2 {
			t.Fatalf("usage_attempts rows = %d, want one per physical send", attempts)
		}
		rows, err := db.SQL().Query("SELECT provider, status FROM usage_attempts WHERE event_id = ? ORDER BY ordinal", id)
		if err != nil {
			t.Fatalf("read attempts: %v", err)
		}
		defer func() { _ = rows.Close() }()
		order := make([]string, 0, 2)
		for rows.Next() {
			var provider string
			var status int
			if err := rows.Scan(&provider, &status); err != nil {
				t.Fatalf("scan attempt: %v", err)
			}
			order = append(order, fmt.Sprintf("%s:%d", provider, status))
		}
		if len(order) != 2 || order[0] != "anthropic:500" || order[1] != "tencent-coding-plan:200" {
			t.Fatalf("attempts = %v, want each physical send recorded", order)
		}
	})

	t.Run("route_reason queryable", func(t *testing.T) {
		db := testkit.OpenTestDB(t)
		recorder := newRecorder(t, db)
		for _, reason := range []string{"catalog", "combo-failover"} {
			if _, err := recorder.AppendEvent(context.Background(), sqlite.UsageEvent{
				Provider: "openai", Model: "gpt-4o", Status: 200, RouteProvider: "openai", RouteReason: reason,
			}); err != nil {
				t.Fatalf("AppendEvent() error = %v", err)
			}
		}
		var count int
		if err := db.SQL().QueryRow("SELECT count(*) FROM usage_events WHERE route_reason = ?", "combo-failover").Scan(&count); err != nil {
			t.Fatalf("query route_reason: %v", err)
		}
		if count != 1 {
			t.Fatalf("rows = %d, want the failover attempt only", count)
		}
	})
}

func columnExists(t *testing.T, db *sqlite.DB, table, column string) bool {
	t.Helper()
	rows, err := db.SQL().Query("SELECT name FROM pragma_table_info(?)", table)
	if err != nil {
		t.Fatalf("read columns of %s: %v", table, err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan column: %v", err)
		}
		if name == column {
			return true
		}
	}
	return false
}
