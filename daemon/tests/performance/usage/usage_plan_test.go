// Package usage_test holds the scale checks for the usage queries: the plan
// assertions the unit tests make at toy size, repeated over a log the size
// the storage design targets.
package usage_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/activity"
	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	"github.com/jonaskahn/relo/tests/testkit"
)

const (
	// scaledRows is the event count the storage design targets.
	scaledRows = 500_000
	// benchRows keeps a benchmark run short enough to finish in CI.
	benchRows = 20_000
	// dayMs is one UTC day in milliseconds.
	dayMs = 86_400_000
	// baseTimestamp is 2026-01-01T00:00:00Z, so seeded rows span real days.
	baseTimestamp = int64(1_767_225_600_000)
)

// seedSQL expands to one row per sequence number, spread over providers,
// models, credentials, and statuses the way a real log looks.
const seedSQL = `INSERT INTO usage_events (
	request_id, timestamp, provider, model, credential_label, surface, status, duration_ms,
	input_tokens, output_tokens, cache_read_tokens, cache_write_tokens, estimated_cost_micros
)
WITH RECURSIVE seq(n) AS (SELECT 1 UNION ALL SELECT n + 1 FROM seq WHERE n < ?)
SELECT 'req-' || n,
       ? + (n / 1000) * 60000,
       CASE n % 5 WHEN 0 THEN 'anthropic' ELSE 'openai' END,
       CASE n % 7 WHEN 0 THEN 'claude-sonnet-4' WHEN 1 THEN 'gpt-4o-mini' ELSE 'gpt-4o' END,
       CASE n % 3 WHEN 0 THEN 'work' ELSE 'default' END,
       'chat-completions',
       CASE n % 11 WHEN 0 THEN 429 WHEN 1 THEN 500 ELSE 200 END,
       100 + (n % 5000),
       100 + (n % 900),
       20 + (n % 300),
       n % 50,
       n % 10,
       1000 + (n % 100000)
FROM seq`

func seedScaled(t *testing.T, db *sqlite.DB, rows int) {
	t.Helper()
	started := time.Now()
	if _, err := db.SQL().Exec(seedSQL, rows, baseTimestamp); err != nil {
		t.Fatalf("seed %d usage events: %v", rows, err)
	}
	t.Logf("seeded %d usage events in %s", rows, time.Since(started).Round(time.Millisecond))
}

func planText(t *testing.T, plan []string, err error) string {
	t.Helper()
	if err != nil {
		t.Fatalf("plan error = %v", err)
	}
	return strings.Join(plan, " | ")
}

func TestUsageQueriesStayIndexedAtScale(t *testing.T) {
	if testing.Short() {
		t.Skip("run without -short to seed the scaled usage log")
	}
	db := testkit.OpenTestDB(t)
	seedScaled(t, db, scaledRows)
	query := sqlite.NewUsageQuery(db)
	ctx := context.Background()

	t.Run("paged listing stays on the timestamp index", func(t *testing.T) {
		cursor := activity.LogCursor{TimestampMs: baseTimestamp + dayMs, ID: 1}
		explained, err := query.PlanLogs(ctx, activity.LogQuery{Cursor: &cursor, Limit: 100})
		plan := planText(t, explained, err)
		if !strings.Contains(plan, "USING INDEX idx_usage_ts") {
			t.Fatalf("plan = %q, want the timestamp index", plan)
		}
	})

	t.Run("provider listing stays on the provider index", func(t *testing.T) {
		explained, err := query.PlanLogs(ctx, activity.LogQuery{
			Filter: activity.UsageFilter{Provider: "openai"}, Limit: 100,
		})
		plan := planText(t, explained, err)
		if !strings.Contains(plan, "USING INDEX idx_usage_provider") {
			t.Fatalf("plan = %q, want the provider index", plan)
		}
	})

	t.Run("rollups stay on an index", func(t *testing.T) {
		cases := []struct {
			name  string
			query activity.RollupQuery
			index string
		}{
			{"day", activity.RollupQuery{Filter: activity.UsageFilter{SinceMs: baseTimestamp}, GroupBy: activity.GroupByDay}, "USING INDEX idx_usage_ts"},
			{"provider", activity.RollupQuery{GroupBy: activity.GroupByProvider}, "USING INDEX idx_usage_provider"},
			{"model", activity.RollupQuery{Filter: activity.UsageFilter{Provider: "openai"}, GroupBy: activity.GroupByModel}, "USING INDEX idx_usage_provider"},
			{"credential", activity.RollupQuery{Filter: activity.UsageFilter{SinceMs: baseTimestamp}, GroupBy: activity.GroupByCredential}, "USING INDEX idx_usage_ts"},
		}
		for _, testCase := range cases {
			t.Run(testCase.name, func(t *testing.T) {
				explained, err := query.PlanRollup(ctx, testCase.query)
				plan := planText(t, explained, err)
				if !strings.Contains(plan, testCase.index) {
					t.Fatalf("plan = %q, want %s", plan, testCase.index)
				}
				started := time.Now()
				if _, err := query.Rollup(ctx, testCase.query); err != nil {
					t.Fatalf("Rollup() error = %v", err)
				}
				t.Logf("%s rollup took %s", testCase.name, time.Since(started).Round(time.Millisecond))
			})
		}
	})

	t.Run("page walk", func(t *testing.T) {
		started := time.Now()
		cursor := (*activity.LogCursor)(nil)
		pages := 0
		for {
			page, err := query.Logs(ctx, activity.LogQuery{Cursor: cursor, Limit: activity.MaxLogLimit})
			if err != nil {
				t.Fatalf("Logs() error = %v", err)
			}
			pages++
			cursor = page.Next
			if cursor == nil || pages >= 20 {
				break
			}
		}
		t.Logf("%d pages in %s", pages, time.Since(started).Round(time.Millisecond))
	})
}

func benchmarkQuery(b *testing.B) *sqlite.UsageQuery {
	b.Helper()
	db := testkit.OpenTestDB(b)
	if _, err := db.SQL().Exec(seedSQL, benchRows, baseTimestamp); err != nil {
		b.Fatalf("seed usage events: %v", err)
	}
	return sqlite.NewUsageQuery(db)
}

func BenchmarkUsageLogPage(b *testing.B) {
	query := benchmarkQuery(b)
	cursor := activity.LogCursor{TimestampMs: baseTimestamp + dayMs, ID: 1}
	request := activity.LogQuery{Cursor: &cursor, Limit: activity.DefaultLogLimit}
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		if _, err := query.Logs(context.Background(), request); err != nil {
			b.Fatalf("Logs() error = %v", err)
		}
	}
}

func BenchmarkUsageRollupByDay(b *testing.B) {
	query := benchmarkQuery(b)
	request := activity.RollupQuery{
		Filter:  activity.UsageFilter{SinceMs: baseTimestamp},
		GroupBy: activity.GroupByDay,
	}
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		if _, err := query.Rollup(context.Background(), request); err != nil {
			b.Fatalf("Rollup() error = %v", err)
		}
	}
}

func BenchmarkUsageRollupByProvider(b *testing.B) {
	query := benchmarkQuery(b)
	request := activity.RollupQuery{GroupBy: activity.GroupByProvider}
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		if _, err := query.Rollup(context.Background(), request); err != nil {
			b.Fatalf("Rollup() error = %v", err)
		}
	}
}
