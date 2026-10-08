// Usage query predicates: turning filters into SQL.
package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/jonaskahn/relo/internal/activity"
)

var groupExpressions = map[activity.GroupBy]string{
	activity.GroupByDay:        "strftime('%Y-%m-%d', timestamp / 1000, 'unixepoch')",
	activity.GroupByProvider:   "provider",
	activity.GroupByModel:      "model",
	activity.GroupByCredential: "COALESCE(credential_label, '')",
	activity.GroupByAccount:    "COALESCE(credential_id, '')",
	activity.GroupByClient:     "COALESCE(client_key_name, '')",
}

// UsageQuery reads the usage log: grouped rollups and cursor-paginated
// listings. Every predicate is bound, never interpolated.
type UsageQuery struct {
	db      *DB
	archive *Archive
}

// NewUsageQuery returns a query reader over the given database.
func NewUsageQuery(db *DB) *UsageQuery {
	return &UsageQuery{db: db, archive: NewArchive(db)}
}

// RawFromMs reports the first timestamp whose request rows are still
// retained, and whether the archive holds any closed day.
func (q *UsageQuery) RawFromMs(ctx context.Context) (int64, bool, error) {
	return q.archive.RawFromMs(ctx)
}

func predicates(f activity.UsageFilter, cursor *activity.LogCursor) (string, []any) {
	clauses := make([]string, 0, 8)
	args := make([]any, 0, 8)
	clauses, args = appendRange(clauses, args, f)
	clauses, args = appendEquals(clauses, args, f)
	if f.AccessClient != "" {
		clauses = append(clauses, "client_app = ?")
		args = append(args, f.AccessClient)
	}
	if cursor != nil {
		clauses = append(clauses, "(timestamp, id) < (?, ?)")
		args = append(args, cursor.TimestampMs, cursor.ID)
	}
	if len(f.Providers) > 0 {
		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(f.Providers)), ",")
		clauses = append(clauses, "provider IN ("+placeholders+")")
		for _, provider := range f.Providers {
			args = append(args, provider)
		}
	}
	return whereClause(clauses), args
}

func appendRange(clauses []string, args []any, f activity.UsageFilter) ([]string, []any) {
	if f.SinceMs > 0 {
		clauses = append(clauses, "timestamp >= ?")
		args = append(args, f.SinceMs)
	}
	if f.UntilMs > 0 {
		clauses = append(clauses, "timestamp <= ?")
		args = append(args, f.UntilMs)
	}
	return clauses, args
}

func appendEquals(clauses []string, args []any, f activity.UsageFilter) ([]string, []any) {
	filters := []struct {
		column string
		value  string
	}{
		{"provider", f.Provider},
		{"model", f.Model},
		{"credential_label", f.CredentialLabel},
		{"credential_id", f.CredentialID},
		{"surface", f.Surface},
		{"route_provider", f.RouteProvider},
		{"request_id", f.RequestID},
		{"client_key_id", f.ClientKeyID},
		{"origin", f.Origin},
	}
	for _, filter := range filters {
		if filter.value != "" {
			clauses = append(clauses, filter.column+" = ?")
			args = append(args, filter.value)
		}
	}
	if len(f.Status) > 0 {
		clauses = append(clauses, statusPredicate(f.Status))
		args = append(args, statusArguments(f.Status)...)
	}
	return clauses, args
}

func statusPredicate(spans []activity.StatusRange) string {
	parts := make([]string, 0, len(spans))
	for _, span := range spans {
		if span.Low == span.High {
			parts = append(parts, "status = ?")
			continue
		}
		parts = append(parts, "status BETWEEN ? AND ?")
	}
	return "(" + strings.Join(parts, " OR ") + ")"
}

func statusArguments(spans []activity.StatusRange) []any {
	args := make([]any, 0, len(spans)*2)
	for _, span := range spans {
		args = append(args, span.Low)
		if span.Low != span.High {
			args = append(args, span.High)
		}
	}
	return args
}

func whereClause(clauses []string) string {
	if len(clauses) == 0 {
		return ""
	}
	return " WHERE " + strings.Join(clauses, " AND ")
}

func archivePredicates(f activity.UsageFilter, lastDay string) (string, []any, bool) {
	if f.RouteProvider != "" || f.RequestID != "" || f.AccessClient != "" {
		return "", nil, false
	}
	clauses := make([]string, 0, 8)
	args := make([]any, 0, 8)
	if lastDay != "" {
		clauses = append(clauses, "day <= ?")
		args = append(args, lastDay)
	}
	clauses, args = appendDayRange(clauses, args, f)
	clauses, args = appendEqualityFilters(clauses, args, f)
	clauses, args = appendProviderList(clauses, args, f.Providers)
	if len(f.Status) > 0 {
		clauses = append(clauses, statusPredicate(f.Status))
		args = append(args, statusArguments(f.Status)...)
	}
	return whereClause(clauses), args, true
}

func appendDayRange(clauses []string, args []any, f activity.UsageFilter) ([]string, []any) {
	if f.SinceMs > 0 {
		clauses = append(clauses, "day >= ?")
		args = append(args, DayOf(f.SinceMs))
	}
	if f.UntilMs > 0 {
		clauses = append(clauses, "day <= ?")
		args = append(args, DayOf(f.UntilMs))
	}
	return clauses, args
}

func appendEqualityFilters(clauses []string, args []any, f activity.UsageFilter) ([]string, []any) {
	equals := []struct {
		column string
		value  string
	}{
		{"provider", f.Provider},
		{"model", f.Model},
		{"account_label", f.CredentialLabel},
		{"account_id", f.CredentialID},
		{"surface", f.Surface},
		{"client_id", f.ClientKeyID},
		{"origin", f.Origin},
	}
	for _, filter := range equals {
		if filter.value != "" {
			clauses = append(clauses, filter.column+" = ?")
			args = append(args, filter.value)
		}
	}
	return clauses, args
}

func appendProviderList(clauses []string, args []any, providers []string) ([]string, []any) {
	if len(providers) == 0 {
		return clauses, args
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(providers)), ",")
	clauses = append(clauses, "provider IN ("+placeholders+")")
	for _, provider := range providers {
		args = append(args, provider)
	}
	return clauses, args
}

func mergeRollup(into, from activity.UsageRollupRow) activity.UsageRollupRow {
	into.Requests += from.Requests
	into.Errors += from.Errors
	into.InputTokens += from.InputTokens
	into.OutputTokens += from.OutputTokens
	into.CacheReadTokens += from.CacheReadTokens
	into.CacheWriteTokens += from.CacheWriteTokens
	into.CostMicros += from.CostMicros
	into.UnpricedRequests += from.UnpricedRequests
	into.DurationMs += from.DurationMs
	into.Attempts += from.Attempts
	into.RetriedRequests += from.RetriedRequests
	into.DurationMaxMs = max(into.DurationMaxMs, from.DurationMaxMs)
	return into
}

func sortedRollup(groups map[string]activity.UsageRollupRow, limit int) []activity.UsageRollupRow {
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if limit > 0 && len(keys) > limit {
		keys = keys[:limit]
	}
	rows := make([]activity.UsageRollupRow, 0, len(keys))
	for _, key := range keys {
		row := groups[key]
		row.Key = key
		rows = append(rows, row)
	}
	return rows
}

// Logs returns one page of usage events, newest first.
func (q *UsageQuery) Logs(ctx context.Context, query activity.LogQuery) (activity.LogPage, error) {
	if err := query.Validate(); err != nil {
		return activity.LogPage{}, err
	}
	limit := query.Limit
	if limit == 0 {
		limit = activity.DefaultLogLimit
	}
	sqlText, args := logSQL(query.Filter, query.Cursor, limit+1)
	rows, err := q.db.sql.QueryContext(ctx, sqlText, args...)
	if err != nil {
		return activity.LogPage{}, fmt.Errorf("list usage events: %w", err)
	}
	defer func() { _ = rows.Close() }()
	scanned, err := scanLogRows(rows)
	if err != nil {
		return activity.LogPage{}, err
	}
	return logPage(scanned, limit), nil
}

// Rollup returns the usage events of a filter grouped by one dimension. Days
// the archive holds closed are read from their aggregate, and the request
// rows still retained answer the rest of the range.
func (q *UsageQuery) Rollup(ctx context.Context, query activity.RollupQuery) ([]activity.UsageRollupRow, error) {
	if err := query.Validate(); err != nil {
		return nil, err
	}
	limit := query.Limit
	if limit == 0 {
		limit = activity.DefaultRollupLimit
	}
	rawFrom, archived, err := q.archive.RawFromMs(ctx)
	if err != nil {
		return nil, err
	}
	groups := map[string]activity.UsageRollupRow{}
	if archived && query.Filter.ReachesArchive(rawFrom) {
		rows, err := q.archiveRollup(ctx, query.GroupBy, query.Filter, activity.LastClosedDay(rawFrom), limit)
		if err != nil {
			return nil, err
		}
		mergeRollupRows(groups, rows)
	}
	if rawFilter, retained := query.Filter.RawSegment(rawFrom, archived); retained {
		rows, err := q.rollup(ctx, activity.RollupQuery{Filter: rawFilter, GroupBy: query.GroupBy, Limit: limit}, limit)
		if err != nil {
			return nil, err
		}
		mergeRollupRows(groups, rows)
	}
	return sortedRollup(groups, limit), nil
}

func mergeRollupRows(groups map[string]activity.UsageRollupRow, rows []activity.UsageRollupRow) {
	for _, row := range rows {
		groups[row.Key] = mergeRollup(groups[row.Key], row)
	}
}

func (q *UsageQuery) rollup(ctx context.Context, query activity.RollupQuery, limit int) ([]activity.UsageRollupRow, error) {
	sqlText, args := rollupSQL(query, limit)
	rows, err := q.db.sql.QueryContext(ctx, sqlText, args...)
	if err != nil {
		return nil, fmt.Errorf("roll up usage events: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanRollupRows(rows)
}

func (q *UsageQuery) archiveRollup(ctx context.Context, groupBy activity.GroupBy, filter activity.UsageFilter, lastDay string, limit int) ([]activity.UsageRollupRow, error) {
	expression, known := archiveGroupExpressions[groupBy]
	if !known {
		return nil, activity.InvalidFilter("group_by", "must be one of day, provider, model, credential, client")
	}
	where, args, usable := archivePredicates(filter, lastDay)
	if !usable {
		return nil, nil
	}
	sqlText := strings.Replace(archiveRollupSelect, "@group", expression, 1) + where + archiveRollupOrder
	args = append(args, limit)
	rows, err := q.db.sql.QueryContext(ctx, sqlText, args...)
	if err != nil {
		return nil, fmt.Errorf("roll up archived usage: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanRollupRows(rows)
}

// Attempts returns the upstream sends of one logical request, in order.
func (q *UsageQuery) Attempts(ctx context.Context, eventID int64) ([]activity.UsageAttemptRow, error) {
	rows, err := q.db.sql.QueryContext(ctx, attemptsSQL, eventID)
	if err != nil {
		return nil, fmt.Errorf("list usage attempts: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanAttemptRows(rows)
}

// Summary totals a filter into one row, independent of the group limit a
// rollup applies, so a page can show exact totals over every matching event.
// A filter that matches nothing answers a zero row rather than an error.
func (q *UsageQuery) Summary(ctx context.Context, filter activity.UsageFilter) (activity.UsageRollupRow, error) {
	if err := filter.Validate(); err != nil {
		return activity.UsageRollupRow{}, err
	}
	rawFrom, archived, err := q.archive.RawFromMs(ctx)
	if err != nil {
		return activity.UsageRollupRow{}, err
	}
	summary := activity.UsageRollupRow{}
	if archived && filter.ReachesArchive(rawFrom) {
		archivedRow, err := q.archiveSummary(ctx, filter, activity.LastClosedDay(rawFrom))
		if err != nil {
			return activity.UsageRollupRow{}, err
		}
		summary = mergeRollup(summary, archivedRow)
	}
	if rawFilter, retained := filter.RawSegment(rawFrom, archived); retained {
		where, args := predicates(rawFilter, nil)
		row := q.db.sql.QueryRowContext(ctx, summarySQL+where, args...)
		var retainedRow activity.UsageRollupRow
		if err := row.Scan(&retainedRow.Requests, &retainedRow.Errors, &retainedRow.InputTokens,
			&retainedRow.OutputTokens, &retainedRow.CacheReadTokens, &retainedRow.CacheWriteTokens,
			&retainedRow.CostMicros, &retainedRow.UnpricedRequests, &retainedRow.DurationMs,
			&retainedRow.Attempts, &retainedRow.RetriedRequests, &retainedRow.DurationMaxMs); err != nil {
			return activity.UsageRollupRow{}, fmt.Errorf("summarize usage events: %w", err)
		}
		summary = mergeRollup(summary, retainedRow)
	}
	return summary, nil
}

func (q *UsageQuery) archiveSummary(ctx context.Context, filter activity.UsageFilter, lastDay string) (activity.UsageRollupRow, error) {
	where, args, usable := archivePredicates(filter, lastDay)
	if !usable {
		return activity.UsageRollupRow{}, nil
	}
	row := q.db.sql.QueryRowContext(ctx, archiveSummarySQL+where, args...)
	var summary activity.UsageRollupRow
	if err := row.Scan(&summary.Requests, &summary.Errors, &summary.InputTokens,
		&summary.OutputTokens, &summary.CacheReadTokens, &summary.CacheWriteTokens,
		&summary.CostMicros, &summary.UnpricedRequests, &summary.DurationMs,
		&summary.Attempts, &summary.RetriedRequests, &summary.DurationMaxMs); err != nil {
		return activity.UsageRollupRow{}, fmt.Errorf("summarize archived usage: %w", err)
	}
	return summary, nil
}

// PlanLogs returns the SQLite query plan of the listing a request would
// run, so a caller can assert the read stays on an index.
func (q *UsageQuery) PlanLogs(ctx context.Context, query activity.LogQuery) ([]string, error) {
	if err := query.Validate(); err != nil {
		return nil, err
	}
	limit := query.Limit
	if limit == 0 {
		limit = activity.DefaultLogLimit
	}
	sqlText, args := logSQL(query.Filter, query.Cursor, limit)
	return q.plan(ctx, sqlText, args)
}

// PlanRollup returns the SQLite query plan of a rollup.
func (q *UsageQuery) PlanRollup(ctx context.Context, query activity.RollupQuery) ([]string, error) {
	if err := query.Validate(); err != nil {
		return nil, err
	}
	limit := query.Limit
	if limit == 0 {
		limit = activity.DefaultRollupLimit
	}
	sqlText, args := rollupSQL(query, limit)
	return q.plan(ctx, sqlText, args)
}

func (q *UsageQuery) plan(ctx context.Context, statement string, args []any) ([]string, error) {
	rows, err := q.db.sql.QueryContext(ctx, "EXPLAIN QUERY PLAN "+statement, args...)
	if err != nil {
		return nil, fmt.Errorf("explain usage query: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanPlan(rows)
}

func logSQL(filter activity.UsageFilter, cursor *activity.LogCursor, limit int) (string, []any) {
	where, args := predicates(filter, cursor)
	args = append(args, limit)
	return logSelect + where + logOrder, args
}

func rollupSQL(query activity.RollupQuery, limit int) (string, []any) {
	where, args := predicates(query.Filter, nil)
	args = append(args, limit)
	expression := groupExpressions[query.GroupBy]
	return strings.Replace(rollupSelect, "@group", expression, 1) + where + rollupOrder, args
}

func logPage(rows []activity.UsageLogRow, limit int) activity.LogPage {
	if len(rows) <= limit {
		return activity.LogPage{Rows: rows}
	}
	page := activity.LogPage{Rows: rows[:limit]}
	last := page.Rows[len(page.Rows)-1]
	page.Next = &activity.LogCursor{TimestampMs: last.TimestampMs, ID: last.ID}
	return page
}

func scanLogRows(rows *sql.Rows) ([]activity.UsageLogRow, error) {
	events := make([]activity.UsageLogRow, 0, activity.DefaultLogLimit)
	for rows.Next() {
		event, err := scanLogRow(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

func scanLogRow(rows *sql.Rows) (activity.UsageLogRow, error) {
	var (
		event      activity.UsageLogRow
		cost       sql.NullInt64
		request    sql.NullString
		surface    sql.NullString
		label      sql.NullString
		credential sql.NullString
		route      sql.NullString
		reason     sql.NullString
		client     sql.NullString
		clientName sql.NullString
		origin     sql.NullString
		extra      sql.NullString
	)
	err := rows.Scan(&event.ID, &request, &event.TimestampMs, &event.Provider, &event.Model,
		&label, &credential, &surface, &event.Status, &event.DurationMs, &event.InputTokens,
		&event.OutputTokens, &event.CacheReadTokens, &event.CacheWriteTokens,
		&cost, &route, &reason, &client, &clientName, &origin, &extra)
	if err != nil {
		return activity.UsageLogRow{}, fmt.Errorf("scan usage event: %w", err)
	}
	return finishLogRow(event, request, label, credential, surface, route, reason, client, clientName, origin, extra, cost), nil
}

func finishLogRow(event activity.UsageLogRow, request, label, credential, surface, route, reason, client, clientName, origin, extra sql.NullString, cost sql.NullInt64) activity.UsageLogRow {
	event.RequestID = request.String
	event.CredentialLabel = label.String
	event.CredentialID = credential.String
	event.Surface = surface.String
	event.RouteProvider = route.String
	event.RouteReason = reason.String
	event.ClientKeyID = client.String
	event.ClientKeyName = clientName.String
	event.Origin = origin.String
	event.Warnings = warningsOf(extra.String)
	return setCost(event, cost)
}

func setCost(event activity.UsageLogRow, cost sql.NullInt64) activity.UsageLogRow {
	if !cost.Valid {
		return event
	}
	value := cost.Int64
	event.EstimatedCostMicros = &value
	return event
}

func warningsOf(extra string) []string {
	if extra == "" || extra == "{}" {
		return nil
	}
	var held struct {
		Warnings []string `json:"warnings"`
	}
	if err := json.Unmarshal([]byte(extra), &held); err != nil {
		return nil
	}
	return held.Warnings
}

func scanRollupRows(rows *sql.Rows) ([]activity.UsageRollupRow, error) {
	groups := make([]activity.UsageRollupRow, 0, 16)
	for rows.Next() {
		var group activity.UsageRollupRow
		err := rows.Scan(&group.Key, &group.Requests, &group.Errors, &group.InputTokens,
			&group.OutputTokens, &group.CacheReadTokens, &group.CacheWriteTokens,
			&group.CostMicros, &group.UnpricedRequests, &group.DurationMs,
			&group.Attempts, &group.RetriedRequests, &group.DurationMaxMs)
		if err != nil {
			return nil, fmt.Errorf("scan usage rollup: %w", err)
		}
		groups = append(groups, group)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate usage rollups: %w", err)
	}
	return groups, nil
}

func scanAttemptRows(rows *sql.Rows) ([]activity.UsageAttemptRow, error) {
	attempts := make([]activity.UsageAttemptRow, 0, 4)
	for rows.Next() {
		var attempt activity.UsageAttemptRow
		var label, credential sql.NullString
		err := rows.Scan(&attempt.Ordinal, &attempt.Provider, &attempt.Model, &label, &credential,
			&attempt.Status, &attempt.DurationMs, &attempt.InputTokens, &attempt.OutputTokens)
		if err != nil {
			return nil, fmt.Errorf("scan usage attempt: %w", err)
		}
		attempt.CredentialLabel = label.String
		attempt.CredentialID = credential.String
		attempts = append(attempts, attempt)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate usage attempts: %w", err)
	}
	return attempts, nil
}

func scanPlan(rows *sql.Rows) ([]string, error) {
	details := make([]string, 0, 4)
	for rows.Next() {
		var (
			id, parent, unused int64
			detail             string
		)
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			return nil, fmt.Errorf("scan query plan: %w", err)
		}
		details = append(details, detail)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate query plan: %w", err)
	}
	return details, nil
}

const logSelect = `SELECT id, request_id, timestamp, provider, model, credential_label, credential_id, surface, status,
	duration_ms, input_tokens, output_tokens, cache_read_tokens, cache_write_tokens,
	estimated_cost_micros, route_provider, route_reason, client_key_id, client_key_name, origin, extra
FROM usage_events`

const logOrder = `
ORDER BY timestamp DESC, id DESC
LIMIT ?`

const rollupSelect = `SELECT @group AS grp,
	COUNT(*),
	SUM(CASE WHEN status >= 400 THEN 1 ELSE 0 END),
	SUM(input_tokens),
	SUM(output_tokens),
	SUM(cache_read_tokens),
	SUM(cache_write_tokens),
	SUM(COALESCE(estimated_cost_micros, 0)),
	SUM(CASE WHEN estimated_cost_micros IS NULL THEN 1 ELSE 0 END),
	SUM(duration_ms),
	SUM(attempts),
	SUM(retried),
	MAX(duration_ms)
FROM usage_events`

const rollupOrder = `
GROUP BY grp
ORDER BY grp
LIMIT ?`

const summarySQL = `SELECT
	COALESCE(COUNT(*), 0),
	COALESCE(SUM(CASE WHEN status >= 400 THEN 1 ELSE 0 END), 0),
	COALESCE(SUM(input_tokens), 0),
	COALESCE(SUM(output_tokens), 0),
	COALESCE(SUM(cache_read_tokens), 0),
	COALESCE(SUM(cache_write_tokens), 0),
	COALESCE(SUM(COALESCE(estimated_cost_micros, 0)), 0),
	COALESCE(SUM(CASE WHEN estimated_cost_micros IS NULL THEN 1 ELSE 0 END), 0),
	COALESCE(SUM(duration_ms), 0),
	COALESCE(SUM(attempts), 0),
	COALESCE(SUM(retried), 0),
	COALESCE(MAX(duration_ms), 0)
FROM usage_events`

const attemptsSQL = `SELECT ordinal, provider, model, credential_label, credential_id, status, duration_ms, input_tokens, output_tokens
FROM usage_attempts WHERE event_id = ? ORDER BY ordinal`

var archiveGroupExpressions = map[activity.GroupBy]string{
	activity.GroupByDay:        "day",
	activity.GroupByProvider:   "provider",
	activity.GroupByModel:      "model",
	activity.GroupByCredential: "account_label",
	activity.GroupByAccount:    "account_id",
	activity.GroupByClient:     "client_name",
}

const archiveRollupSelect = `SELECT @group AS grp,
	COALESCE(SUM(requests), 0),
	COALESCE(SUM(CASE WHEN status >= 400 THEN requests ELSE 0 END), 0),
	COALESCE(SUM(input_tokens), 0),
	COALESCE(SUM(output_tokens), 0),
	COALESCE(SUM(cache_read_tokens), 0),
	COALESCE(SUM(cache_write_tokens), 0),
	COALESCE(SUM(cost_micros), 0),
	COALESCE(SUM(unpriced_requests), 0),
	COALESCE(SUM(duration_ms), 0),
	COALESCE(SUM(attempts), 0),
	COALESCE(SUM(retried_requests), 0),
	COALESCE(MAX(duration_max_ms), 0)
FROM usage_daily`

const archiveRollupOrder = `
GROUP BY grp
ORDER BY grp
LIMIT ?`

const archiveSummarySQL = `SELECT
	COALESCE(SUM(requests), 0),
	COALESCE(SUM(CASE WHEN status >= 400 THEN requests ELSE 0 END), 0),
	COALESCE(SUM(input_tokens), 0),
	COALESCE(SUM(output_tokens), 0),
	COALESCE(SUM(cache_read_tokens), 0),
	COALESCE(SUM(cache_write_tokens), 0),
	COALESCE(SUM(cost_micros), 0),
	COALESCE(SUM(unpriced_requests), 0),
	COALESCE(SUM(duration_ms), 0),
	COALESCE(SUM(attempts), 0),
	COALESCE(SUM(retried_requests), 0),
	COALESCE(MAX(duration_max_ms), 0)
FROM usage_daily`
