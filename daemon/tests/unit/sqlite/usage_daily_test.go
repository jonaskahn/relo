package storage_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/activity"
	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	"github.com/jonaskahn/relo/tests/testkit"
)

// archivedDays seeds two closed days and one open day, each with the
// dimensions the console filters and groups by, and returns the recorder.
func archivedDays(t *testing.T, db *sqlite.DB) *sqlite.UsageRecorder {
	t.Helper()
	ctx := context.Background()
	recorder := newRecorder(t, db)
	cost := int64(2_500)
	events := []sqlite.UsageEvent{
		// Two days ago: two providers, two models, two accounts, both origins.
		{RequestID: "old-1", Timestamp: day(2026, 1, 1) + 1_000, Provider: "openai", Model: "gpt-4o",
			CredentialLabel: "work", CredentialID: "cred-1", ClientKeyID: "key-1", ClientKeyName: "codex",
			Surface: "chat-completions", Origin: "external", Status: 200, DurationMs: 40,
			InputTokens: 100, OutputTokens: 20, CacheReadTokens: 10, CacheWriteTokens: 5,
			EstimatedCostMicros: &cost, Attempts: 1},
		{RequestID: "old-2", Timestamp: day(2026, 1, 1) + 2_000, Provider: "anthropic", Model: "claude",
			CredentialLabel: "work", CredentialID: "cred-2", ClientKeyID: "key-2", ClientKeyName: "claude-code",
			Surface: "messages", Origin: "internal", Status: 500, DurationMs: 60,
			InputTokens: 30, OutputTokens: 0, Attempts: 2, Retried: true},
		// Yesterday: a rate limit, which the status class filter has to keep.
		{RequestID: "old-3", Timestamp: day(2026, 1, 2) + 1_000, Provider: "openai", Model: "gpt-4o",
			CredentialLabel: "work", CredentialID: "cred-1", ClientKeyID: "key-1", ClientKeyName: "codex",
			Surface: "chat-completions", Origin: "external", Status: 429, DurationMs: 15,
			InputTokens: 5, Attempts: 3, Retried: true},
		// Today: still open, and never read from the archive.
		{RequestID: "today-1", Timestamp: time.Date(2026, 1, 4, 9, 0, 0, 0, time.UTC).UnixMilli(),
			Provider: "openai", Model: "gpt-4o", CredentialLabel: "work", CredentialID: "cred-1",
			ClientKeyID: "key-1", ClientKeyName: "codex", Surface: "chat-completions",
			Origin: "external", Status: 200, DurationMs: 90, InputTokens: 7, OutputTokens: 3, Attempts: 1},
	}
	for _, event := range events {
		if _, err := recorder.AppendEvent(ctx, event); err != nil {
			t.Fatalf("AppendEvent(%s) error = %v", event.RequestID, err)
		}
	}
	return recorder
}

// archiveNow is the moment the seeded days are read from: the day the events
// call today is still open, and the three before it have closed.
func archiveNow() time.Time {
	return time.Date(2026, 1, 4, 12, 0, 0, 0, time.UTC)
}

// archiveFinalize closes every day before today against the fixed clock the
// retention tests use, so a day boundary is deterministic.
func archiveFinalize(t *testing.T, db *sqlite.DB) {
	t.Helper()
	archive := sqlite.NewArchive(db)
	archive.SetClock(archiveNow)
	if _, err := archive.Finalize(context.Background()); err != nil {
		t.Fatalf("Finalize() error = %v", err)
	}
}

// archiveRetention enforces a budget against the same day the days were
// seeded for, so the window covers the seeded events.
func archiveRetention(t *testing.T, db *sqlite.DB, days int) {
	t.Helper()
	retention := newRetention(t, db, func(options *sqlite.RetentionOptions) {
		options.Now = archiveNow
	})
	if _, err := retention.Enforce(context.Background(), sqlite.RetentionConfig{UsageDays: days}); err != nil {
		t.Fatalf("Enforce() error = %v", err)
	}
}

// TestArchiveTotalsSurviveDeletingTheRows covers the promise of the daily
// archive: once a day is closed, its totals are answered without the request
// rows, and the page's reads do not change when those rows go.
func TestArchiveTotalsSurviveDeletingTheRows(t *testing.T) {
	db := testkit.OpenTestDB(t)
	archivedDays(t, db)
	query := sqlite.NewUsageQuery(db)
	ctx := context.Background()

	before, err := query.Summary(ctx, activity.UsageFilter{})
	if err != nil {
		t.Fatalf("Summary() error = %v", err)
	}
	if before.Requests != 4 {
		t.Fatalf("summary before = %+v, want the four seeded requests", before)
	}

	archiveFinalize(t, db)
	after, err := query.Summary(ctx, activity.UsageFilter{})
	if err != nil {
		t.Fatalf("Summary() error = %v", err)
	}
	// Sealing a closed day raises it to what its rows prove and never lowers
	// it, so the live aggregate and the seal cannot add up twice.
	assertRollup(t, "after closing the days", after, before)

	// A budget of three days keeps today and the two days before it, so the
	// first day loses the two rows its totals were computed from.
	archiveRetention(t, db, 3)
	if left := countRows(t, db, "usage_events"); left != 2 {
		t.Fatalf("usage_events holds %d rows, want the two of the kept days", left)
	}
	deleted, err := query.Summary(ctx, activity.UsageFilter{})
	if err != nil {
		t.Fatalf("Summary() error = %v", err)
	}
	assertRollup(t, "after deleting the rows of a closed day", deleted, before)
}

// TestArchiveKeepsEveryFilterExact covers the read the console makes after
// the rows are gone: each filter and each grouping still answers exactly what
// it answered while the rows were there.
func TestArchiveKeepsEveryFilterExact(t *testing.T) {
	db := testkit.OpenTestDB(t)
	archivedDays(t, db)
	query := sqlite.NewUsageQuery(db)
	ctx := context.Background()
	filters := []struct {
		name   string
		filter activity.UsageFilter
	}{
		{"provider", activity.UsageFilter{Provider: "openai"}},
		{"model", activity.UsageFilter{Model: "claude"}},
		{"account", activity.UsageFilter{CredentialID: "cred-1"}},
		{"account label", activity.UsageFilter{CredentialLabel: "work"}},
		{"client key", activity.UsageFilter{ClientKeyID: "key-2"}},
		{"origin", activity.UsageFilter{Origin: "internal"}},
		{"surface", activity.UsageFilter{Surface: "messages"}},
		{"status class", activity.UsageFilter{Status: statusSpans(t, "4xx,5xx")}},
		{"status code", activity.UsageFilter{Status: statusSpans(t, "429")}},
		{"everything at once", activity.UsageFilter{Provider: "openai", Model: "gpt-4o",
			CredentialID: "cred-1", ClientKeyID: "key-1", Origin: "external", Surface: "chat-completions"}},
	}
	groups := []activity.GroupBy{activity.GroupByDay, activity.GroupByProvider, activity.GroupByModel,
		activity.GroupByAccount, activity.GroupByClient, activity.GroupByCredential}

	type snapshot struct {
		summaries map[string]activity.UsageRollupRow
		rollups   map[string][]activity.UsageRollupRow
	}
	take := func(label string) snapshot {
		t.Helper()
		shot := snapshot{summaries: map[string]activity.UsageRollupRow{}, rollups: map[string][]activity.UsageRollupRow{}}
		for _, filter := range filters {
			summary, err := query.Summary(ctx, filter.filter)
			if err != nil {
				t.Fatalf("Summary(%s) error = %v", filter.name, err)
			}
			shot.summaries[filter.name] = summary
		}
		for _, group := range groups {
			rows, err := query.Rollup(ctx, activity.RollupQuery{GroupBy: group})
			if err != nil {
				t.Fatalf("Rollup(%s) error = %v", group, err)
			}
			shot.rollups[string(group)] = rows
		}
		t.Logf("took the %s snapshot", label)
		return shot
	}

	before := take("request-log")
	archiveFinalize(t, db)
	archiveRetention(t, db, 3)
	after := take("archived")

	for name, want := range before.summaries {
		assertRollup(t, "summary of "+name, after.summaries[name], want)
	}
	for group, want := range before.rollups {
		got := after.rollups[group]
		if len(got) != len(want) {
			t.Fatalf("%s rollup = %+v, want %+v", group, got, want)
		}
		for index := range want {
			if got[index] != want[index] {
				t.Fatalf("%s rollup row %d = %+v, want %+v", group, index, got[index], want[index])
			}
		}
	}
}

// TestArchiveDoesNotAnswerWhatItKeeps covers the boundary the archive
// reports: a read that reaches past the retained rows says so, and a filter
// only the request log can answer stays with the log.
func TestArchiveDoesNotAnswerWhatItKeeps(t *testing.T) {
	db := testkit.OpenTestDB(t)
	archivedDays(t, db)
	query := sqlite.NewUsageQuery(db)
	archive := sqlite.NewArchive(db)
	ctx := context.Background()

	if _, found, err := archive.RawFromMs(ctx); err != nil || found {
		t.Fatalf("RawFromMs() before finalizing = found %v, error %v", found, err)
	}
	archiveFinalize(t, db)
	floor, found, err := archive.RawFromMs(ctx)
	if err != nil || !found {
		t.Fatalf("RawFromMs() = found %v, error %v", found, err)
	}
	want := day(2026, 1, 4)
	if floor != want {
		t.Fatalf("RawFromMs() = %d, want the start of the open day %d", floor, want)
	}
	// A range that opens before the floor reaches the archive; one that opens
	// inside the retained window does not.
	recent, err := query.Summary(ctx, activity.UsageFilter{SinceMs: floor})
	if err != nil {
		t.Fatalf("Summary() error = %v", err)
	}
	if recent.Requests != 1 {
		t.Fatalf("summary of the retained day = %+v, want the one open-day request", recent)
	}
	all, err := query.Summary(ctx, activity.UsageFilter{})
	if err != nil {
		t.Fatalf("Summary() error = %v", err)
	}
	if all.Requests != 4 {
		t.Fatalf("summary of everything = %+v, want all four requests", all)
	}
	// A request identifier is not something the archive keeps, so it answers
	// only the rows the log still holds.
	archiveRetention(t, db, 3)
	gone, err := query.Summary(ctx, activity.UsageFilter{RequestID: "old-1"})
	if err != nil {
		t.Fatalf("Summary() error = %v", err)
	}
	if gone.Requests != 0 {
		t.Fatalf("summary by a deleted request id = %+v, want nothing", gone)
	}
	kept, err := query.Summary(ctx, activity.UsageFilter{RequestID: "today-1"})
	if err != nil {
		t.Fatalf("Summary() error = %v", err)
	}
	if kept.Requests != 1 {
		t.Fatalf("summary by a retained request id = %+v, want the one request", kept)
	}
}

// TestFinalizeIsIdempotent pins that closing the days twice changes nothing:
// the second pass finds every day closed and leaves the totals alone.
func TestFinalizeIsIdempotent(t *testing.T) {
	db := testkit.OpenTestDB(t)
	archivedDays(t, db)
	archive := sqlite.NewArchive(db)
	archive.SetClock(archiveNow)
	ctx := context.Background()

	closed, err := archive.Finalize(ctx)
	if err != nil {
		t.Fatalf("Finalize() error = %v", err)
	}
	if closed != 3 {
		t.Fatalf("first Finalize() closed %d days, want the three before today", closed)
	}
	before := dailyTotals(t, db)

	closed, err = archive.Finalize(ctx)
	if err != nil {
		t.Fatalf("second Finalize() error = %v", err)
	}
	if closed != 0 {
		t.Fatalf("second Finalize() closed %d days, want none", closed)
	}
	after := dailyTotals(t, db)
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("daily totals changed on a second pass: %v, want %v", after, before)
	}
}

// TestFinalizeNeverLowersATotal covers what a seal must never do: shrink a
// day. The totals a request writes at its end are the truth, so a seal can
// only raise what its rows prove and leaves a higher total alone.
func TestFinalizeNeverLowersATotal(t *testing.T) {
	db := testkit.OpenTestDB(t)
	recorder := archivedDays(t, db)
	archiveFinalize(t, db)
	ctx := context.Background()

	// A live write went missing while the day was open: the rows prove one
	// more request than the aggregate holds, and the seal raises it.
	if _, err := db.SQL().ExecContext(ctx,
		"UPDATE usage_daily SET requests = requests - 1 WHERE day = '2026-01-01'"); err != nil {
		t.Fatalf("lower the aggregate: %v", err)
	}
	if _, err := db.SQL().ExecContext(ctx,
		"UPDATE usage_days SET finalized = 0 WHERE day = '2026-01-01'"); err != nil {
		t.Fatalf("reopen the day: %v", err)
	}
	archiveFinalize(t, db)
	if got := dailyTotals(t, db)["2026-01-01"]; got != 2 {
		t.Fatalf("sealed total = %d, want the two requests the rows hold", got)
	}

	// A request that started before midnight arrives while the day is closed
	// again; the next seal counts it exactly once.
	appendEvent(t, recorder, sqlite.UsageEvent{
		RequestID: "late-1", Timestamp: day(2026, 1, 1) + 3_000,
		Provider: "openai", Model: "gpt-4o", Status: 200, Attempts: 1,
	})
	if _, err := db.SQL().ExecContext(ctx,
		"UPDATE usage_days SET finalized = 0 WHERE day = '2026-01-01'"); err != nil {
		t.Fatalf("reopen the day: %v", err)
	}
	archiveFinalize(t, db)
	if got := dailyTotals(t, db)["2026-01-01"]; got != 3 {
		t.Fatalf("sealed total = %d, want the late request counted once", got)
	}

	// A day that already reports more than its rows is left alone: lowering
	// it to the rows is exactly the rebuild that loses data once a cleanup
	// has shortened the log.
	if _, err := db.SQL().ExecContext(ctx,
		"UPDATE usage_daily SET requests = requests * 2 WHERE day = '2026-01-01'"); err != nil {
		t.Fatalf("raise the aggregate: %v", err)
	}
	if _, err := db.SQL().ExecContext(ctx,
		"UPDATE usage_days SET finalized = 0 WHERE day = '2026-01-01'"); err != nil {
		t.Fatalf("reopen the day: %v", err)
	}
	archiveFinalize(t, db)
	if got := dailyTotals(t, db)["2026-01-01"]; got != 6 {
		t.Fatalf("sealed total = %d, want the six it already reported left untouched", got)
	}
}

// dailyTotals reads one total per day of the aggregate, which is what an
// idempotent pass must leave untouched.
func dailyTotals(t *testing.T, db *sqlite.DB) map[string]int64 {
	t.Helper()
	rows, err := db.SQL().Query("SELECT day, sum(requests) FROM usage_daily GROUP BY day ORDER BY day")
	if err != nil {
		t.Fatalf("read daily totals: %v", err)
	}
	defer func() { _ = rows.Close() }()
	totals := map[string]int64{}
	for rows.Next() {
		var day string
		var requests int64
		if err := rows.Scan(&day, &requests); err != nil {
			t.Fatalf("scan daily total: %v", err)
		}
		totals[day] = requests
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate daily totals: %v", err)
	}
	return totals
}

// assertRollup compares two rollups field by field, so a difference names the
// counter that moved rather than printing two whole rows.
func assertRollup(t *testing.T, label string, got, want activity.UsageRollupRow) {
	t.Helper()
	if got == want {
		return
	}
	t.Fatalf("%s = %+v, want %+v", label, got, want)
}

// statusSpans reads a status filter the way the service does, so a test can
// filter by a class or a code.
func statusSpans(t *testing.T, raw string) []activity.StatusRange {
	t.Helper()
	spans, err := activity.ParseStatusFilter(raw)
	if err != nil {
		t.Fatalf("ParseStatusSpans(%q) error = %v", raw, err)
	}
	return spans
}
