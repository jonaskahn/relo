package storage_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/activity"
	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	"github.com/jonaskahn/relo/tests/testkit"
)

// day returns the millisecond timestamp of a UTC midnight, which is the
// bucket a day rollup groups by.
func day(year int, month int, monthDay int) int64 {
	return time.Date(year, time.Month(month), monthDay, 0, 0, 0, 0, time.UTC).UnixMilli()
}

func newQuery(t *testing.T, db *sqlite.DB) (*sqlite.UsageQuery, *sqlite.UsageRecorder) {
	t.Helper()
	recorder := newRecorder(t, db)
	return sqlite.NewUsageQuery(db), recorder
}

func appendEvent(t *testing.T, recorder *sqlite.UsageRecorder, event sqlite.UsageEvent) int64 {
	t.Helper()
	id, err := recorder.AppendEvent(context.Background(), event)
	if err != nil {
		t.Fatalf("AppendEvent(%+v) error = %v", event, err)
	}
	return id
}

func seedEvents(t *testing.T, db *sqlite.DB, count int, options ...func(int, *sqlite.UsageEvent)) []int64 {
	t.Helper()
	recorder := newRecorder(t, db)
	ids := make([]int64, 0, count)
	for index := 0; index < count; index++ {
		event := sqlite.UsageEvent{
			RequestID:   fmt.Sprintf("req-%d", index),
			Timestamp:   day(2026, 1, 1) + int64(index),
			Provider:    "openai",
			Model:       "gpt-4o",
			Surface:     "chat-completions",
			Status:      200,
			DurationMs:  10,
			InputTokens: 3,
		}
		for _, option := range options {
			option(index, &event)
		}
		ids = append(ids, appendEvent(t, recorder, event))
	}
	return ids
}

func TestUsageQueryFilterValidation(t *testing.T) {
	db := testkit.OpenTestDB(t)
	query, _ := newQuery(t, db)
	cases := []struct {
		name  string
		query activity.LogQuery
		field string
	}{
		{"negative since", activity.LogQuery{Filter: activity.UsageFilter{SinceMs: -1}}, "filter.since_ms"},
		{"negative until", activity.LogQuery{Filter: activity.UsageFilter{UntilMs: -1}}, "filter.until_ms"},
		{"reversed range", activity.LogQuery{Filter: activity.UsageFilter{SinceMs: 2, UntilMs: 1}}, "filter.range"},
		{"status out of range", activity.LogQuery{Filter: activity.UsageFilter{Status: []activity.StatusRange{{Low: 99, High: 99}}}}, "filter.status"},
		{"status class out of range", activity.LogQuery{Filter: activity.UsageFilter{Status: []activity.StatusRange{{Low: 600, High: 699}}}}, "filter.status"},
		{"reversed status span", activity.LogQuery{Filter: activity.UsageFilter{Status: []activity.StatusRange{{Low: 500, High: 400}}}}, "filter.status"},
		{"long provider", activity.LogQuery{Filter: activity.UsageFilter{Provider: strings.Repeat("x", 257)}}, "filter.provider"},
		{"long model", activity.LogQuery{Filter: activity.UsageFilter{Model: strings.Repeat("x", 257)}}, "filter.model"},
		{"long credential", activity.LogQuery{Filter: activity.UsageFilter{CredentialLabel: strings.Repeat("x", 257)}}, "filter.credential"},
		{"long surface", activity.LogQuery{Filter: activity.UsageFilter{Surface: strings.Repeat("x", 257)}}, "filter.surface"},
		{"long route provider", activity.LogQuery{Filter: activity.UsageFilter{RouteProvider: strings.Repeat("x", 257)}}, "filter.route_provider"},
		{"long client app", activity.LogQuery{Filter: activity.UsageFilter{AccessClient: strings.Repeat("x", 257)}}, "filter.client_app"},
		{"limit too large", activity.LogQuery{Limit: activity.MaxLogLimit + 1}, "limit"},
		{"negative limit", activity.LogQuery{Limit: -1}, "limit"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := query.Logs(context.Background(), testCase.query)
			if !errors.Is(err, activity.ErrInvalidFilter) {
				t.Fatalf("Logs() error = %v, want ErrInvalidFilter", err)
			}
			if !strings.Contains(err.Error(), testCase.field) {
				t.Fatalf("Logs() error = %v, want the %s field path", err, testCase.field)
			}
		})
	}
}

func TestUsageQueryRollupValidation(t *testing.T) {
	db := testkit.OpenTestDB(t)
	query, _ := newQuery(t, db)
	cases := []struct {
		name  string
		query activity.RollupQuery
		field string
	}{
		{"unknown group", activity.RollupQuery{GroupBy: "nothing"}, "group_by"},
		{"empty group", activity.RollupQuery{}, "group_by"},
		{"negative limit", activity.RollupQuery{GroupBy: activity.GroupByDay, Limit: -1}, "limit"},
		{"invalid filter", activity.RollupQuery{GroupBy: activity.GroupByDay, Filter: activity.UsageFilter{SinceMs: -1}}, "filter.since_ms"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := query.Rollup(context.Background(), testCase.query)
			if !errors.Is(err, activity.ErrInvalidFilter) {
				t.Fatalf("Rollup() error = %v, want ErrInvalidFilter", err)
			}
			if !strings.Contains(err.Error(), testCase.field) {
				t.Fatalf("Rollup() error = %v, want the %s field path", err, testCase.field)
			}
			if _, err := query.PlanRollup(context.Background(), testCase.query); !errors.Is(err, activity.ErrInvalidFilter) {
				t.Fatalf("PlanRollup() error = %v, want ErrInvalidFilter", err)
			}
		})
	}
	if _, err := query.PlanLogs(context.Background(), activity.LogQuery{Limit: -1}); !errors.Is(err, activity.ErrInvalidFilter) {
		t.Fatalf("PlanLogs() error = %v, want ErrInvalidFilter", err)
	}
}

func TestLogCursorRoundTrip(t *testing.T) {
	t.Run("encode then decode", func(t *testing.T) {
		cursor := activity.LogCursor{TimestampMs: 1767225600123, ID: 42}
		decoded, err := activity.DecodeLogCursor(cursor.Encode())
		if err != nil {
			t.Fatalf("DecodeLogCursor() error = %v", err)
		}
		if decoded != cursor {
			t.Fatalf("DecodeLogCursor() = %+v, want %+v", decoded, cursor)
		}
	})

	for _, raw := range []string{"", "not-base64!", "!!!!", "MQ", "MTph", "MTphOmI", "LToxOjE", "MTo", "MTrL"} {
		t.Run("reject "+raw, func(t *testing.T) {
			if _, err := activity.DecodeLogCursor(raw); !errors.Is(err, activity.ErrInvalidCursor) {
				t.Fatalf("DecodeLogCursor(%q) error = %v, want ErrInvalidCursor", raw, err)
			}
		})
	}

	t.Run("reject an oversized token", func(t *testing.T) {
		raw := strings.Repeat("A", activity.MaxCursorLength+1)
		if _, err := activity.DecodeLogCursor(raw); !errors.Is(err, activity.ErrInvalidCursor) {
			t.Fatalf("DecodeLogCursor(long) error = %v, want ErrInvalidCursor", err)
		}
	})
}

func TestUsageQueryLogsMalformedCursor(t *testing.T) {
	db := testkit.OpenTestDB(t)
	query, _ := newQuery(t, db)
	_, err := query.Logs(context.Background(), activity.LogQuery{Cursor: &activity.LogCursor{TimestampMs: 1, ID: 1}})
	if err != nil {
		t.Fatalf("Logs() with a well-formed cursor error = %v", err)
	}
	if _, err := activity.DecodeLogCursor("garbage"); !errors.Is(err, activity.ErrInvalidCursor) {
		t.Fatalf("DecodeLogCursor() error = %v, want ErrInvalidCursor", err)
	}
}

func TestUsageQueryLogsFilters(t *testing.T) {
	db := testkit.OpenTestDB(t)
	query, recorder := newQuery(t, db)
	events := []sqlite.UsageEvent{
		{RequestID: "a", Timestamp: 1_000, Provider: "openai", Model: "gpt-4o", CredentialLabel: "default", Surface: "chat-completions", Status: 200, DurationMs: 10},
		{RequestID: "b", Timestamp: 2_000, Provider: "anthropic", Model: "claude", CredentialLabel: "work", Surface: "messages", Status: 500, DurationMs: 20},
		{RequestID: "c", Timestamp: 3_000, Provider: "openai", Model: "gpt-4o-mini", CredentialLabel: "work", Surface: "responses", Status: 200, DurationMs: 30, RouteProvider: "openai", RouteReason: "alias"},
		{RequestID: "d", Timestamp: 4_000, Provider: "openai", Model: "gpt-4o", CredentialLabel: "work", Surface: "chat-completions", Status: 429, DurationMs: 40},
	}
	for _, event := range events {
		appendEvent(t, recorder, event)
	}
	cases := []struct {
		name   string
		filter activity.UsageFilter
		want   []string
	}{
		{"everything", activity.UsageFilter{}, []string{"d", "c", "b", "a"}},
		{"provider", activity.UsageFilter{Provider: "openai"}, []string{"d", "c", "a"}},
		{"model", activity.UsageFilter{Model: "gpt-4o"}, []string{"d", "a"}},
		{"credential", activity.UsageFilter{CredentialLabel: "work"}, []string{"d", "c", "b"}},
		{"surface", activity.UsageFilter{Surface: "responses"}, []string{"c"}},
		{"route provider", activity.UsageFilter{RouteProvider: "openai"}, []string{"c"}},
		{"one code", activity.UsageFilter{Status: []activity.StatusRange{{Low: 500, High: 500}}}, []string{"b"}},
		{"one class", activity.UsageFilter{Status: []activity.StatusRange{{Low: 500, High: 599}}}, []string{"b"}},
		{"two classes", activity.UsageFilter{Status: []activity.StatusRange{{Low: 400, High: 499}, {Low: 500, High: 599}}}, []string{"d", "b"}},
		{"since", activity.UsageFilter{SinceMs: 2_000}, []string{"d", "c", "b"}},
		{"until", activity.UsageFilter{UntilMs: 2_000}, []string{"b", "a"}},
		{"range", activity.UsageFilter{SinceMs: 2_000, UntilMs: 2_500}, []string{"b"}},
		{"combined", activity.UsageFilter{Provider: "openai", CredentialLabel: "work"}, []string{"d", "c"}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			page, err := query.Logs(context.Background(), activity.LogQuery{Filter: testCase.filter})
			if err != nil {
				t.Fatalf("Logs() error = %v", err)
			}
			got := requestIDs(page.Rows)
			if strings.Join(got, ",") != strings.Join(testCase.want, ",") {
				t.Fatalf("Logs() returned %v, want %v", got, testCase.want)
			}
			if page.Next != nil {
				t.Fatalf("Logs() Next = %+v, want nil on a short page", page.Next)
			}
		})
	}
}

func requestIDs(rows []activity.UsageLogRow) []string {
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.RequestID)
	}
	return ids
}

// TestUsageQueryLogsAccessClientFilter covers the filter the console offers
// for the coding client a key was issued for: the event stores that client,
// so a request that names one keeps only the rows written with it.
func TestUsageQueryLogsAccessClientFilter(t *testing.T) {
	db := testkit.OpenTestDB(t)
	query, recorder := newQuery(t, db)
	ctx := context.Background()
	events := []sqlite.UsageEvent{
		{RequestID: "a", Timestamp: 1_000, Provider: "openai", Model: "gpt-4o", Status: 200, ClientKeyID: "key-codex", ClientKeyName: "codex", ClientApp: "codex"},
		{RequestID: "b", Timestamp: 2_000, Provider: "openai", Model: "gpt-4o", Status: 200, ClientKeyID: "key-cursor", ClientKeyName: "cursor", ClientApp: "cursor"},
		{RequestID: "c", Timestamp: 3_000, Provider: "openai", Model: "gpt-4o", Status: 200, ClientKeyID: "key-shared", ClientKeyName: "shared"},
	}
	for _, event := range events {
		appendEvent(t, recorder, event)
	}
	cases := []struct {
		name   string
		filter activity.UsageFilter
		want   []string
	}{
		{"one client", activity.UsageFilter{AccessClient: "codex"}, []string{"a"}},
		{"another client", activity.UsageFilter{AccessClient: "cursor"}, []string{"b"}},
		{"a client no key was issued for", activity.UsageFilter{AccessClient: "grok-build"}, nil},
		{"one key", activity.UsageFilter{ClientKeyID: "key-cursor"}, []string{"b"}},
		{"client and key together", activity.UsageFilter{AccessClient: "codex", ClientKeyID: "key-cursor"}, nil},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			page, err := query.Logs(ctx, activity.LogQuery{Filter: testCase.filter})
			if err != nil {
				t.Fatalf("Logs() error = %v", err)
			}
			got := requestIDs(page.Rows)
			if strings.Join(got, ",") != strings.Join(testCase.want, ",") {
				t.Fatalf("Logs() returned %v, want %v", got, testCase.want)
			}
		})
	}
}

func TestExpiredKeyDeleteLeavesUsage(t *testing.T) {
	db := testkit.OpenTestDB(t)
	query, recorder := newQuery(t, db)
	repo := sqlite.NewAccessKeyRepo(db)
	ctx := context.Background()
	now := int64(1_700_000_000_000)
	expired := accessKeyRow("key-lapsed")
	expired.ExpiresAtMs = now - 1
	revoked := accessKeyRow("key-retired")
	if err := repo.Insert(ctx, expired); err != nil {
		t.Fatalf("Insert(expired) error = %v", err)
	}
	if err := repo.Insert(ctx, revoked); err != nil {
		t.Fatalf("Insert(revoked) error = %v", err)
	}
	if err := repo.Revoke(ctx, revoked.ID, now); err != nil {
		t.Fatalf("Revoke() error = %v", err)
	}
	appendEvent(t, recorder, sqlite.UsageEvent{
		RequestID: "kept", Timestamp: 1_000, Provider: "openai", Model: "gpt-4o", Status: 200,
		ClientKeyID: expired.ID, ClientKeyName: "lapsed", ClientApp: "codex",
	})

	deleted, err := repo.DeleteExpired(ctx, now, nil)
	if err != nil {
		t.Fatalf("DeleteExpired() error = %v", err)
	}
	if deleted != 1 {
		t.Fatalf("DeleteExpired() = %d, want the expired key", deleted)
	}
	if _, found, err := repo.Get(ctx, revoked.ID); err != nil || !found {
		t.Fatalf("Get(revoked) found = %v, err = %v, want the revoked key kept", found, err)
	}

	page, err := query.Logs(ctx, activity.LogQuery{Filter: activity.UsageFilter{AccessClient: "codex"}})
	if err != nil {
		t.Fatalf("Logs() error = %v", err)
	}
	if len(page.Rows) != 1 || page.Rows[0].RequestID != "kept" || page.Rows[0].ClientKeyName != "lapsed" {
		t.Fatalf("rows = %+v, want the request and the name it stored", page.Rows)
	}
}

func TestUsageQueryLogsRow(t *testing.T) {
	db := testkit.OpenTestDB(t)
	query, recorder := newQuery(t, db)
	cost := int64(1234)
	appendEvent(t, recorder, sqlite.UsageEvent{
		RequestID: "req-1", Timestamp: 1_000, Provider: "openai", Model: "gpt-4o",
		CredentialLabel: "default", Surface: "chat-completions", Status: 200,
		DurationMs: 10, InputTokens: 100, OutputTokens: 40, CacheReadTokens: 5,
		CacheWriteTokens: 2, EstimatedCostMicros: &cost, RouteProvider: "openai",
		RouteReason: "catalog",
	})
	page, err := query.Logs(context.Background(), activity.LogQuery{})
	if err != nil {
		t.Fatalf("Logs() error = %v", err)
	}
	if len(page.Rows) != 1 {
		t.Fatalf("Logs() returned %d rows, want 1", len(page.Rows))
	}
	row := page.Rows[0]
	if row.RequestID != "req-1" || row.Provider != "openai" || row.Model != "gpt-4o" {
		t.Fatalf("row identity = %+v", row)
	}
	if row.CredentialLabel != "default" || row.Surface != "chat-completions" || row.RouteReason != "catalog" {
		t.Fatalf("row routing = %+v", row)
	}
	if row.InputTokens != 100 || row.OutputTokens != 40 || row.CacheReadTokens != 5 || row.CacheWriteTokens != 2 {
		t.Fatalf("row tokens = %+v", row)
	}
	if row.TimestampMs != 1_000 || row.DurationMs != 10 || row.Status != 200 {
		t.Fatalf("row timing = %+v", row)
	}
	if row.EstimatedCostMicros == nil || *row.EstimatedCostMicros != cost {
		t.Fatalf("row cost = %v, want %d", row.EstimatedCostMicros, cost)
	}

	t.Run("absent cost stays nil", func(t *testing.T) {
		appendEvent(t, recorder, sqlite.UsageEvent{
			RequestID: "req-2", Timestamp: 2_000, Provider: "openai", Model: "gpt-4o", Status: 200,
		})
		unpriced, err := query.Logs(context.Background(), activity.LogQuery{Filter: activity.UsageFilter{SinceMs: 1_500}})
		if err != nil {
			t.Fatalf("Logs() error = %v", err)
		}
		if len(unpriced.Rows) != 1 || unpriced.Rows[0].EstimatedCostMicros != nil {
			t.Fatalf("unpriced row = %+v, want a nil cost", unpriced.Rows)
		}
	})
}

func TestUsageQueryPaging(t *testing.T) {
	db := testkit.OpenTestDB(t)
	query, _ := newQuery(t, db)
	ids := seedEvents(t, db, 1000)
	seen := map[int64]int{}
	var cursor *activity.LogCursor
	pages := 0
	for {
		page, err := query.Logs(context.Background(), activity.LogQuery{Cursor: cursor, Limit: 37})
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
		cursor = page.Next
		if pages > 40 {
			t.Fatal("paging never reached the end of the log")
		}
	}
	if len(seen) != len(ids) {
		t.Fatalf("paged over %d rows, want %d", len(seen), len(ids))
	}
	for _, id := range ids {
		if seen[id] != 1 {
			t.Fatalf("row %d delivered %d times, want exactly once", id, seen[id])
		}
	}
}

func TestUsageQueryPagingWithinOneTimestamp(t *testing.T) {
	db := testkit.OpenTestDB(t)
	query, _ := newQuery(t, db)
	ids := seedEvents(t, db, 250, func(_ int, event *sqlite.UsageEvent) {
		event.Timestamp = day(2026, 3, 1)
	})
	seen := map[int64]int{}
	var cursor *activity.LogCursor
	for step := 0; step < 10; step++ {
		page, err := query.Logs(context.Background(), activity.LogQuery{Cursor: cursor, Limit: 50})
		if err != nil {
			t.Fatalf("Logs() error = %v", err)
		}
		for _, row := range page.Rows {
			seen[row.ID]++
		}
		cursor = page.Next
		if cursor == nil {
			break
		}
	}
	if len(seen) != len(ids) {
		t.Fatalf("paged over %d rows, want %d", len(seen), len(ids))
	}
	for _, id := range ids {
		if seen[id] != 1 {
			t.Fatalf("row %d delivered %d times, want exactly once", id, seen[id])
		}
	}
}

func TestUsageQueryPagingStableUnderInserts(t *testing.T) {
	db := testkit.OpenTestDB(t)
	query, recorder := newQuery(t, db)
	ids := seedEvents(t, db, 300)
	seen := map[int64]int{}
	var cursor *activity.LogCursor
	appended := int64(0)
	for {
		page, err := query.Logs(context.Background(), activity.LogQuery{Cursor: cursor, Limit: 50})
		if err != nil {
			t.Fatalf("Logs() error = %v", err)
		}
		for _, row := range page.Rows {
			seen[row.ID]++
		}
		if page.Next == nil {
			break
		}
		cursor = page.Next
		appended++
		appendEvent(t, recorder, sqlite.UsageEvent{
			RequestID: fmt.Sprintf("new-%d", appended), Timestamp: day(2026, 6, 1) + appended,
			Provider: "openai", Model: "gpt-4o", Status: 200,
		})
	}
	for _, id := range ids {
		if seen[id] != 1 {
			t.Fatalf("row %d delivered %d times while rows were being appended, want exactly once", id, seen[id])
		}
	}
	if appended == 0 {
		t.Fatal("the test never appended a row mid-paging")
	}
}

func TestUsageQueryRollup(t *testing.T) {
	db := testkit.OpenTestDB(t)
	query, recorder := newQuery(t, db)
	cost := int64(1_000)
	events := []sqlite.UsageEvent{
		{RequestID: "a", Timestamp: day(2026, 1, 1) + 1, Provider: "openai", Model: "gpt-4o",
			CredentialLabel: "default", Status: 200, DurationMs: 10,
			InputTokens: 100, OutputTokens: 50, CacheReadTokens: 10, CacheWriteTokens: 5,
			EstimatedCostMicros: &cost},
		{RequestID: "b", Timestamp: day(2026, 1, 1) + 2, Provider: "openai", Model: "gpt-4o-mini",
			CredentialLabel: "default", Status: 200, DurationMs: 20, InputTokens: 20, OutputTokens: 10},
		{RequestID: "c", Timestamp: day(2026, 1, 2) + 1, Provider: "anthropic", Model: "claude",
			CredentialLabel: "work", Status: 500, DurationMs: 30, InputTokens: 5, OutputTokens: 0,
			EstimatedCostMicros: &cost},
	}
	for _, event := range events {
		appendEvent(t, recorder, event)
	}
	cases := []struct {
		name    string
		groupBy activity.GroupBy
		want    []activity.UsageRollupRow
	}{
		{"day", activity.GroupByDay, []activity.UsageRollupRow{
			{Key: "2026-01-01", Requests: 2, InputTokens: 120, OutputTokens: 60, CacheReadTokens: 10,
				CacheWriteTokens: 5, CostMicros: 1_000, UnpricedRequests: 1, DurationMs: 30, DurationMaxMs: 20},
			{Key: "2026-01-02", Requests: 1, Errors: 1, InputTokens: 5, CostMicros: 1_000, DurationMs: 30, DurationMaxMs: 30},
		}},
		{"provider", activity.GroupByProvider, []activity.UsageRollupRow{
			{Key: "anthropic", Requests: 1, Errors: 1, InputTokens: 5, CostMicros: 1_000, DurationMs: 30, DurationMaxMs: 30},
			{Key: "openai", Requests: 2, InputTokens: 120, OutputTokens: 60, CacheReadTokens: 10,
				CacheWriteTokens: 5, CostMicros: 1_000, UnpricedRequests: 1, DurationMs: 30, DurationMaxMs: 20},
		}},
		{"model", activity.GroupByModel, []activity.UsageRollupRow{
			{Key: "claude", Requests: 1, Errors: 1, InputTokens: 5, CostMicros: 1_000, DurationMs: 30, DurationMaxMs: 30},
			{Key: "gpt-4o", Requests: 1, InputTokens: 100, OutputTokens: 50, CacheReadTokens: 10,
				CacheWriteTokens: 5, CostMicros: 1_000, DurationMs: 10, DurationMaxMs: 10},
			{Key: "gpt-4o-mini", Requests: 1, InputTokens: 20, OutputTokens: 10,
				UnpricedRequests: 1, DurationMs: 20, DurationMaxMs: 20},
		}},
		{"credential", activity.GroupByCredential, []activity.UsageRollupRow{
			{Key: "default", Requests: 2, InputTokens: 120, OutputTokens: 60, CacheReadTokens: 10,
				CacheWriteTokens: 5, CostMicros: 1_000, UnpricedRequests: 1, DurationMs: 30, DurationMaxMs: 20},
			{Key: "work", Requests: 1, Errors: 1, InputTokens: 5, CostMicros: 1_000, DurationMs: 30, DurationMaxMs: 30},
		}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			groups, err := query.Rollup(context.Background(), activity.RollupQuery{GroupBy: testCase.groupBy})
			if err != nil {
				t.Fatalf("Rollup() error = %v", err)
			}
			if len(groups) != len(testCase.want) {
				t.Fatalf("Rollup() returned %d groups, want %d: %+v", len(groups), len(testCase.want), groups)
			}
			for index, group := range groups {
				if group != testCase.want[index] {
					t.Fatalf("group %d = %+v, want %+v", index, group, testCase.want[index])
				}
			}
		})
	}

	t.Run("filtered and limited", func(t *testing.T) {
		groups, err := query.Rollup(context.Background(), activity.RollupQuery{
			Filter:  activity.UsageFilter{Provider: "openai"},
			GroupBy: activity.GroupByModel, Limit: 1,
		})
		if err != nil {
			t.Fatalf("Rollup() error = %v", err)
		}
		if len(groups) != 1 || groups[0].Key != "gpt-4o" {
			t.Fatalf("Rollup() = %+v, want the first openai model group", groups)
		}
	})

	t.Run("day rollup of an empty range", func(t *testing.T) {
		groups, err := query.Rollup(context.Background(), activity.RollupQuery{
			Filter:  activity.UsageFilter{SinceMs: day(2030, 1, 1)},
			GroupBy: activity.GroupByDay,
		})
		if err != nil {
			t.Fatalf("Rollup() error = %v", err)
		}
		if len(groups) != 0 {
			t.Fatalf("Rollup() = %+v, want no groups", groups)
		}
	})
}

func TestUsageQueryAttempts(t *testing.T) {
	db := testkit.OpenTestDB(t)
	query, recorder := newQuery(t, db)
	eventID := appendEvent(t, recorder, sqlite.UsageEvent{
		RequestID: "req-1", Timestamp: 1_000, Provider: "openai", Model: "gpt-4o", Status: 200,
	})
	attempts := []sqlite.UsageAttempt{
		{EventID: eventID, Ordinal: 1, Provider: "openai", Model: "gpt-4o", Status: 429, DurationMs: 5},
		{EventID: eventID, Ordinal: 2, Provider: "openai", Model: "gpt-4o", Status: 200, DurationMs: 9,
			InputTokens: 11, OutputTokens: 22},
	}
	for _, attempt := range attempts {
		if err := recorder.AppendAttempt(context.Background(), attempt); err != nil {
			t.Fatalf("AppendAttempt() error = %v", err)
		}
	}
	rows, err := query.Attempts(context.Background(), eventID)
	if err != nil {
		t.Fatalf("Attempts() error = %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("Attempts() returned %d rows, want 2", len(rows))
	}
	if rows[0].Status != 429 || rows[1].Status != 200 || rows[1].OutputTokens != 22 {
		t.Fatalf("Attempts() = %+v, want both sends in order", rows)
	}
	if empty, err := query.Attempts(context.Background(), eventID+99); err != nil || len(empty) != 0 {
		t.Fatalf("Attempts(unknown) = %+v, %v, want no rows", empty, err)
	}
}

func TestUsageQueryIndexUsage(t *testing.T) {
	db := testkit.OpenTestDB(t)
	query, _ := newQuery(t, db)
	cases := []struct {
		name  string
		plan  func() ([]string, error)
		wants []string
	}{
		{"plain listing walks the timestamp index", func() ([]string, error) {
			return query.PlanLogs(context.Background(), activity.LogQuery{})
		}, []string{"USING INDEX idx_usage_ts"}},
		{"provider listing walks the provider index", func() ([]string, error) {
			return query.PlanLogs(context.Background(), activity.LogQuery{Filter: activity.UsageFilter{Provider: "openai"}})
		}, []string{"USING INDEX idx_usage_provider"}},
		{"provider listing also walks it when paging", func() ([]string, error) {
			cursor := activity.LogCursor{TimestampMs: 10, ID: 10}
			return query.PlanLogs(context.Background(), activity.LogQuery{
				Filter: activity.UsageFilter{Provider: "openai"}, Cursor: &cursor,
			})
		}, []string{"USING INDEX idx_usage_provider"}},
		{"a bounded listing seeks instead of scanning", func() ([]string, error) {
			return query.PlanLogs(context.Background(), activity.LogQuery{
				Filter: activity.UsageFilter{SinceMs: day(2026, 1, 1), UntilMs: day(2026, 2, 1)},
			})
		}, []string{"SEARCH usage_events USING INDEX idx_usage_ts"}},
		{"a provider rollup walks the provider index", func() ([]string, error) {
			return query.PlanRollup(context.Background(), activity.RollupQuery{
				Filter: activity.UsageFilter{Provider: "openai"}, GroupBy: activity.GroupByProvider,
			})
		}, []string{"USING INDEX idx_usage_provider"}},
		{"a bounded day rollup seeks the timestamp index", func() ([]string, error) {
			return query.PlanRollup(context.Background(), activity.RollupQuery{
				Filter: activity.UsageFilter{SinceMs: day(2026, 1, 1)}, GroupBy: activity.GroupByDay,
			})
		}, []string{"SEARCH usage_events USING INDEX idx_usage_ts"}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			plan, err := testCase.plan()
			if err != nil {
				t.Fatalf("plan() error = %v", err)
			}
			joined := strings.Join(plan, " | ")
			for _, want := range testCase.wants {
				if !strings.Contains(joined, want) {
					t.Fatalf("plan = %q, want it to contain %q", joined, want)
				}
			}
			for _, detail := range plan {
				if strings.Contains(detail, "usage_events") && !strings.Contains(detail, "USING INDEX") {
					t.Fatalf("plan = %q, want every usage_events step backed by an index", joined)
				}
			}
		})
	}
}

func TestUsageQueryParameterizesFilters(t *testing.T) {
	db := testkit.OpenTestDB(t)
	query, recorder := newQuery(t, db)
	appendEvent(t, recorder, sqlite.UsageEvent{
		RequestID: "req-1", Timestamp: 1_000, Provider: "openai", Model: "gpt-4o", Status: 200,
	})
	hostile := "openai' OR 1=1 --"
	page, err := query.Logs(context.Background(), activity.LogQuery{Filter: activity.UsageFilter{Provider: hostile}})
	if err != nil {
		t.Fatalf("Logs() error = %v", err)
	}
	if len(page.Rows) != 0 {
		t.Fatalf("Logs() returned %d rows for a filter that matches nothing, want 0", len(page.Rows))
	}
	groups, err := query.Rollup(context.Background(), activity.RollupQuery{
		Filter: activity.UsageFilter{Model: hostile}, GroupBy: activity.GroupByModel,
	})
	if err != nil {
		t.Fatalf("Rollup() error = %v", err)
	}
	if len(groups) != 0 {
		t.Fatalf("Rollup() returned %d groups for a filter that matches nothing, want 0", len(groups))
	}
}

func TestUsageQueryReportsClosedDatabase(t *testing.T) {
	db := testkit.OpenTestDB(t)
	query, _ := newQuery(t, db)
	if err := db.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if _, err := query.Logs(context.Background(), activity.LogQuery{}); err == nil {
		t.Fatal("Logs() on a closed database returned no error")
	}
	if _, err := query.Rollup(context.Background(), activity.RollupQuery{GroupBy: activity.GroupByDay}); err == nil {
		t.Fatal("Rollup() on a closed database returned no error")
	}
	if _, err := query.Attempts(context.Background(), 1); err == nil {
		t.Fatal("Attempts() on a closed database returned no error")
	}
	if _, err := query.PlanLogs(context.Background(), activity.LogQuery{}); err == nil {
		t.Fatal("PlanLogs() on a closed database returned no error")
	}
	if _, err := query.PlanRollup(context.Background(), activity.RollupQuery{GroupBy: activity.GroupByDay}); err == nil {
		t.Fatal("PlanRollup() on a closed database returned no error")
	}
}
