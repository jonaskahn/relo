package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	appactivity "github.com/jonaskahn/relo/internal/application/activity"
)

// seedUsage writes events spread over two days, so a rollup has groups to
// return and a log listing has pages.
func (h *harness) seedUsage(t *testing.T, count int) {
	t.Helper()
	h.seedUsageOn(t, count, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
}

// seedUsageOn writes the same events on one chosen day, which is how a test
// about retention places rows before the cleanup floor.
func (h *harness) seedUsageOn(t *testing.T, count int, day time.Time) {
	t.Helper()
	recorder, err := sqlite.NewUsageRecorder(h.db, nil)
	if err != nil {
		t.Fatalf("NewUsageRecorder() error = %v", err)
	}
	defer func() { _ = recorder.Close() }()
	for index := 0; index < count; index++ {
		cost := int64(10 * (index + 1))
		event := sqlite.UsageEvent{
			RequestID: requestID(index), Timestamp: day.UnixMilli() + int64(index)*60_000,
			Provider: "openai", Model: "gpt-4o", CredentialLabel: "default",
			Surface: "chat-completions", Status: 200, DurationMs: 25,
			InputTokens: 100, OutputTokens: 20, EstimatedCostMicros: &cost,
		}
		if index%3 == 0 {
			event.Provider = "anthropic"
			event.Model = "claude-sonnet-5"
			event.Status = 500
		}
		if _, err := recorder.AppendEvent(context.Background(), event); err != nil {
			t.Fatalf("AppendEvent() error = %v", err)
		}
	}
}

func requestID(index int) string {
	return "req-" + string(rune('a'+index%26)) + "-" + itoa(index)
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	digits := ""
	for value > 0 {
		digits = string(rune('0'+value%10)) + digits
		value /= 10
	}
	return digits
}

func (h *harness) reads() *appactivity.Service {
	return appactivity.New(appactivity.Options{Usage: sqlite.NewUsageQuery(h.db)})
}

func TestUsageRollupThroughTheService(t *testing.T) {
	harness := newHarness(t)
	ctx := context.Background()
	harness.seedUsage(t, 20)

	rows, err := harness.reads().Usage(ctx, appactivity.UsageQuery{GroupBy: "provider"})
	if err != nil {
		t.Fatalf("Usage() error = %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("Usage() returned %d groups, want 2: %+v", len(rows), rows)
	}
	total := int64(0)
	for _, row := range rows {
		total += row.Requests
	}
	if total != 20 {
		t.Fatalf("Usage() counted %d requests, want 20", total)
	}

	t.Run("by day", func(t *testing.T) {
		rows, err := harness.reads().Usage(ctx, appactivity.UsageQuery{GroupBy: "day", Provider: "openai"})
		if err != nil {
			t.Fatalf("Usage(day) error = %v", err)
		}
		if len(rows) != 1 || rows[0].Key != "2026-01-01" {
			t.Fatalf("Usage(day) = %+v, want the seeded day", rows)
		}
	})

	t.Run("status filter", func(t *testing.T) {
		rows, err := harness.reads().Usage(ctx, appactivity.UsageQuery{GroupBy: "model", Status: "500"})
		if err != nil {
			t.Fatalf("Usage(status) error = %v", err)
		}
		if len(rows) != 1 || rows[0].Key != "claude-sonnet-5" {
			t.Fatalf("Usage(status=500) = %+v, want the failing model", rows)
		}
	})

	t.Run("status class filter", func(t *testing.T) {
		rows, err := harness.reads().Usage(ctx, appactivity.UsageQuery{GroupBy: "model", Status: "4xx,5xx"})
		if err != nil {
			t.Fatalf("Usage(status=4xx,5xx) error = %v", err)
		}
		if len(rows) != 1 || rows[0].Key != "claude-sonnet-5" {
			t.Fatalf("Usage(status=4xx,5xx) = %+v, want the failing model", rows)
		}
	})

	t.Run("refused status", func(t *testing.T) {
		_, err := harness.reads().Usage(ctx, appactivity.UsageQuery{GroupBy: "model", Status: "6xx"})
		if !errors.Is(err, appactivity.ErrInvalidQuery) {
			t.Fatalf("Usage(status=6xx) error = %v, want ErrInvalidQuery", err)
		}
	})

	t.Run("unknown group", func(t *testing.T) {
		_, err := harness.reads().Usage(ctx, appactivity.UsageQuery{GroupBy: "nothing"})
		if !errors.Is(err, appactivity.ErrInvalidQuery) {
			t.Fatalf("Usage() error = %v, want ErrInvalidQuery", err)
		}
	})
}

func TestUsageRollupWithSeveralProviders(t *testing.T) {
	harness := newHarness(t)
	ctx := context.Background()
	harness.seedUsage(t, 20)

	t.Run("the list matches every named connection", func(t *testing.T) {
		rows, err := harness.reads().Usage(ctx, appactivity.UsageQuery{
			GroupBy: "provider", Providers: []string{"openai", "anthropic"},
		})
		if err != nil {
			t.Fatalf("Usage(providers) error = %v", err)
		}
		total := int64(0)
		for _, row := range rows {
			total += row.Requests
		}
		if total != 20 {
			t.Fatalf("Usage(providers) counted %d requests, want all 20", total)
		}
	})

	t.Run("the list keeps a named connection out", func(t *testing.T) {
		rows, err := harness.reads().Usage(ctx, appactivity.UsageQuery{
			GroupBy: "provider", Providers: []string{"openai"},
		})
		if err != nil {
			t.Fatalf("Usage(providers) error = %v", err)
		}
		if len(rows) != 1 || rows[0].Key != "openai" {
			t.Fatalf("Usage(providers) = %+v, want openai only", rows)
		}
	})

	t.Run("one provider still filters by name alone", func(t *testing.T) {
		rows, err := harness.reads().Usage(ctx, appactivity.UsageQuery{GroupBy: "provider", Provider: "openai"})
		if err != nil {
			t.Fatalf("Usage(provider) error = %v", err)
		}
		if len(rows) != 1 || rows[0].Key != "openai" {
			t.Fatalf("Usage(provider) = %+v, want openai only", rows)
		}
	})
}

func TestLogsThroughTheService(t *testing.T) {
	harness := newHarness(t)
	ctx := context.Background()
	harness.seedUsage(t, 25)

	seen := map[int64]int{}
	cursor := ""
	pages := 0
	for {
		page, err := harness.reads().Logs(ctx, appactivity.LogQuery{Cursor: cursor, Limit: 7})
		if err != nil {
			t.Fatalf("Logs() error = %v", err)
		}
		pages++
		for _, row := range page.Rows {
			seen[row.ID]++
		}
		if page.Next == nil {
			break
		}
		cursor = page.Next.Encode()
		if pages > 10 {
			t.Fatal("paging never reached the end of the log")
		}
	}
	if len(seen) != 25 {
		t.Fatalf("paged over %d rows, want 25", len(seen))
	}
	for id, count := range seen {
		if count != 1 {
			t.Fatalf("row %d delivered %d times, want once", id, count)
		}
	}

	t.Run("provider filter", func(t *testing.T) {
		page, err := harness.reads().Logs(ctx, appactivity.LogQuery{Provider: "anthropic"})
		if err != nil {
			t.Fatalf("Logs() error = %v", err)
		}
		for _, row := range page.Rows {
			if row.Provider != "anthropic" {
				t.Fatalf("Logs(anthropic) returned %+v", row)
			}
		}
		if len(page.Rows) == 0 {
			t.Fatal("Logs(anthropic) returned no rows")
		}
	})

	t.Run("malformed cursor", func(t *testing.T) {
		_, err := harness.reads().Logs(ctx, appactivity.LogQuery{Cursor: "not-a-cursor"})
		if !errors.Is(err, appactivity.ErrInvalidQuery) {
			t.Fatalf("Logs() error = %v, want ErrInvalidQuery", err)
		}
	})

	t.Run("invalid filter", func(t *testing.T) {
		_, err := harness.reads().Logs(ctx, appactivity.LogQuery{SinceMs: -1})
		if !errors.Is(err, appactivity.ErrInvalidQuery) {
			t.Fatalf("Logs() error = %v, want ErrInvalidQuery", err)
		}
	})
}

func TestAttemptsThroughTheService(t *testing.T) {
	harness := newHarness(t)
	ctx := context.Background()
	recorder, err := sqlite.NewUsageRecorder(harness.db, nil)
	if err != nil {
		t.Fatalf("NewUsageRecorder() error = %v", err)
	}
	defer func() { _ = recorder.Close() }()
	eventID, err := recorder.AppendEvent(ctx, sqlite.UsageEvent{
		RequestID: "req-1", Timestamp: 1_000, Provider: "openai", Model: "gpt-4o", Status: 200,
	})
	if err != nil {
		t.Fatalf("AppendEvent() error = %v", err)
	}
	if err := recorder.AppendAttempt(ctx, sqlite.UsageAttempt{
		EventID: eventID, Ordinal: 1, Provider: "openai", Model: "gpt-4o", Status: 429,
	}); err != nil {
		t.Fatalf("AppendAttempt() error = %v", err)
	}
	attempts, err := harness.reads().Attempts(ctx, eventID)
	if err != nil {
		t.Fatalf("Attempts() error = %v", err)
	}
	if len(attempts) != 1 || attempts[0].Status != 429 {
		t.Fatalf("Attempts() = %+v, want the retried send", attempts)
	}
}
