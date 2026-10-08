// Usage filters: the queries activity pages read.
package activity

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultLogLimit is the page size a log listing uses when the caller
	// asks for no particular one.
	DefaultLogLimit = 100
	// MaxLogLimit caps one log page, so a single request can never ask the
	// database for the whole history.
	MaxLogLimit = 1000
	// DefaultRollupLimit caps the groups one rollup returns.
	DefaultRollupLimit = 1000
	// MaxFilterTextLength bounds every free-text filter value, keeping a
	// hostile request from binding a megabyte into a predicate.
	MaxFilterTextLength = 256
	// MaxProvidersFilter bounds how many connections one query may name.
	MaxProvidersFilter = 32
	// MaxCursorLength bounds an encoded cursor before it is decoded.
	MaxCursorLength = 96
	dayLayout       = "2006-01-02"
)

var (
	// ErrInvalidFilter reports a filter field a query cannot use.
	ErrInvalidFilter = errors.New("invalid usage filter")
	// ErrInvalidCursor reports a paging token this package did not issue.
	ErrInvalidCursor = errors.New("invalid usage cursor")
)

// GroupBy names the dimension a rollup groups usage events by.
type GroupBy string

// The grouping dimensions a rollup accepts.
const (
	GroupByDay        GroupBy = "day"
	GroupByProvider   GroupBy = "provider"
	GroupByModel      GroupBy = "model"
	GroupByCredential GroupBy = "credential"
	// GroupByAccount groups by the stored account identifier rather than the
	// display label, so two accounts that share a label stay apart.
	GroupByAccount GroupBy = "account"
	GroupByClient  GroupBy = "client"
)

// StatusRange is one span of HTTP status codes a log read may match, so a
// caller can name one code (429) and a whole class (4xx) alike.
type StatusRange struct {
	Low  int
	High int
}

// UsageFilter narrows the usage rows a query reads. A zero value matches
// every row; Status is applied only when a caller names a code or a class.
type UsageFilter struct {
	SinceMs  int64
	UntilMs  int64
	Provider string
	// Providers lists several connections one query may total. When it is
	// set, Provider is empty and the read matches any name in the list.
	Providers       []string
	Model           string
	CredentialLabel string
	// CredentialID narrows the read to one stored account, which is what
	// keeps two accounts of one connection apart in a report.
	CredentialID  string
	Surface       string
	RouteProvider string
	RequestID     string
	ClientKeyID   string
	// AccessClient narrows the read to the requests made with a key an
	// operator issued for one coding client, such as codex or claude-code.
	AccessClient string
	// Origin narrows the read to traffic a console started (internal) or
	// traffic a client sent through the data plane (external).
	Origin string
	Status []StatusRange
}

// LogQuery is one page request against the usage log.
type LogQuery struct {
	Filter UsageFilter
	Cursor *LogCursor
	Limit  int
}

// RollupQuery is one grouped read of the usage log.
type RollupQuery struct {
	Filter  UsageFilter
	GroupBy GroupBy
	Limit   int
}

// UsageLogRow is one usage event as the log listing reports it. The field
// names are the JSON the console reads.
type UsageLogRow struct {
	ID                  int64
	RequestID           string
	TimestampMs         int64
	Provider            string
	Model               string
	CredentialLabel     string
	CredentialID        string
	Surface             string
	Status              int
	DurationMs          int64
	InputTokens         int
	OutputTokens        int
	CacheReadTokens     int
	CacheWriteTokens    int
	EstimatedCostMicros *int64
	RouteProvider       string
	RouteReason         string
	Origin              string
	ClientKeyID         string
	ClientKeyName       string
	// Warnings are the advisory notes this request carried.
	Warnings []string
}

// LogPage is one page of usage events, oldest page last.
type LogPage struct {
	Rows []UsageLogRow
	Next *LogCursor
}

// UsageRollupRow is one group of usage events. A group carries the counts
// that are meaningful for the chosen dimension. The field names are the JSON
// the console reads.
type UsageRollupRow struct {
	Key              string
	Requests         int64
	Errors           int64
	InputTokens      int64
	OutputTokens     int64
	CacheReadTokens  int64
	CacheWriteTokens int64
	CostMicros       int64
	UnpricedRequests int64
	DurationMs       int64
	// Attempts is how many upstream sends the requests of this group made,
	// and RetriedRequests how many of them needed more than one.
	Attempts        int64
	RetriedRequests int64
	// DurationMaxMs is the slowest single request in the group, which is what
	// the average would hide.
	DurationMaxMs int64
}

// UsageAttemptRow is one upstream send of a logical request. The field names
// are the JSON the console reads.
type UsageAttemptRow struct {
	Ordinal  int
	Provider string
	Model    string
	// CredentialLabel and CredentialID name the account the attempt ran on,
	// which is what tells two accounts of one connection apart.
	CredentialLabel string
	CredentialID    string
	Status          int
	DurationMs      int64
	InputTokens     int
	OutputTokens    int
}

// LogCursor marks the last row a page returned: an event timestamp and an
// id, which together are unique and stable while rows are appended.
type LogCursor struct {
	TimestampMs int64
	ID          int64
}

// Encode renders the cursor as the opaque token a client hands back.
func (c LogCursor) Encode() string {
	raw := strconv.FormatInt(c.TimestampMs, 10) + ":" + strconv.FormatInt(c.ID, 10)
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

// DecodeLogCursor parses a cursor this package issued, reporting
// ErrInvalidCursor for every other input.
func DecodeLogCursor(raw string) (LogCursor, error) {
	if raw == "" || len(raw) > MaxCursorLength {
		return LogCursor{}, fmt.Errorf("cursor: %w", ErrInvalidCursor)
	}
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return LogCursor{}, fmt.Errorf("cursor: %w", ErrInvalidCursor)
	}
	return parseCursorParts(string(decoded))
}

func parseCursorParts(decoded string) (LogCursor, error) {
	stamp, id, found := strings.Cut(decoded, ":")
	if !found {
		return LogCursor{}, fmt.Errorf("cursor: %w", ErrInvalidCursor)
	}
	at, err := strconv.ParseInt(stamp, 10, 64)
	if err != nil || at < 0 {
		return LogCursor{}, fmt.Errorf("cursor: %w", ErrInvalidCursor)
	}
	sequence, err := strconv.ParseInt(id, 10, 64)
	if err != nil || sequence < 0 {
		return LogCursor{}, fmt.Errorf("cursor: %w", ErrInvalidCursor)
	}
	return LogCursor{TimestampMs: at, ID: sequence}, nil
}

// Validate reports the first field that makes the filter unusable, naming
// the field so a caller can point the operator at it.
func (f UsageFilter) Validate() error {
	if err := validateFilterRange(f); err != nil {
		return err
	}
	if err := validateFilterStatus(f.Status); err != nil {
		return err
	}
	return validateFilterText(f)
}

func validateFilterRange(f UsageFilter) error {
	if f.SinceMs < 0 {
		return InvalidFilter("filter.since_ms", "must not be negative")
	}
	if f.UntilMs < 0 {
		return InvalidFilter("filter.until_ms", "must not be negative")
	}
	if f.SinceMs > 0 && f.UntilMs > 0 && f.SinceMs > f.UntilMs {
		return InvalidFilter("filter.range", "since must not be after until")
	}
	return nil
}

func validateFilterStatus(spans []StatusRange) error {
	for _, span := range spans {
		if span.Low < 100 || span.High > 599 || span.Low > span.High {
			return InvalidFilter("filter.status", "must be an HTTP status code or class")
		}
	}
	return nil
}

func validateFilterText(f UsageFilter) error {
	texts := []struct {
		field string
		value string
	}{
		{"filter.provider", f.Provider},
		{"filter.model", f.Model},
		{"filter.credential", f.CredentialLabel},
		{"filter.account", f.CredentialID},
		{"filter.surface", f.Surface},
		{"filter.route_provider", f.RouteProvider},
		{"filter.request_id", f.RequestID},
		{"filter.client", f.ClientKeyID},
		{"filter.client_app", f.AccessClient},
	}
	for _, text := range texts {
		if len(text.value) > MaxFilterTextLength {
			return InvalidFilter(text.field, "is too long")
		}
	}
	if len(f.Providers) > MaxProvidersFilter {
		return InvalidFilter("filter.provider", "lists too many connections")
	}
	for _, provider := range f.Providers {
		if len(provider) > MaxFilterTextLength {
			return InvalidFilter("filter.provider", "is too long")
		}
	}
	return nil
}

// Validate reports the first field that makes the paging request unusable.
func (q LogQuery) Validate() error {
	if err := q.Filter.Validate(); err != nil {
		return err
	}
	if q.Limit < 0 || q.Limit > MaxLogLimit {
		return InvalidFilter("limit", "must be between 0 and "+strconv.Itoa(MaxLogLimit))
	}
	return nil
}

// Validate reports the first field that makes the rollup request unusable.
func (q RollupQuery) Validate() error {
	if err := q.Filter.Validate(); err != nil {
		return err
	}
	if !q.GroupBy.known() {
		return InvalidFilter("group_by", "must be one of day, provider, model, credential, client")
	}
	if q.Limit < 0 {
		return InvalidFilter("limit", "must not be negative")
	}
	return nil
}

// InvalidFilter reports one field a usage query cannot use.
func InvalidFilter(field, detail string) error {
	return fmt.Errorf("%s %s: %w", field, detail, ErrInvalidFilter)
}

func (g GroupBy) known() bool {
	switch g {
	case GroupByDay, GroupByProvider, GroupByModel, GroupByCredential, GroupByAccount, GroupByClient:
		return true
	default:
		return false
	}
}

// ReachesArchive reports whether a filter reads anything older than the
// retained request rows, which is the part the archive answers.
func (f UsageFilter) ReachesArchive(rawFromMs int64) bool {
	if rawFromMs <= 0 {
		return false
	}
	return f.SinceMs == 0 || f.SinceMs < rawFromMs
}

// RawSegment returns the filter narrowed to the retained request rows, and
// whether any of them fall inside the range at all.
func (f UsageFilter) RawSegment(rawFromMs int64, archived bool) (UsageFilter, bool) {
	if !archived {
		return f, true
	}
	retained := f
	if retained.SinceMs < rawFromMs {
		retained.SinceMs = rawFromMs
	}
	if retained.UntilMs > 0 && retained.UntilMs <= rawFromMs {
		return retained, false
	}
	return retained, true
}

// LastClosedDay names the newest day the archive holds closed, which is the
// day before the retained request rows begin. A read over the archive never
// crosses it, so a day that is still open is read from its rows alone.
func LastClosedDay(rawFromMs int64) string {
	if rawFromMs <= 0 {
		return ""
	}
	return time.UnixMilli(rawFromMs - 1).UTC().Format(dayLayout)
}

// ParseStatusFilter reads the status a caller asked a log read to match: a
// comma-separated list of exact codes (429) and classes (4xx).
func ParseStatusFilter(raw string) ([]StatusRange, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, nil
	}
	spans := make([]StatusRange, 0, 2)
	for _, item := range strings.Split(trimmed, ",") {
		token := strings.ToLower(strings.TrimSpace(item))
		if span, found := statusClass(token); found {
			spans = append(spans, span)
			continue
		}
		code, err := strconv.Atoi(token)
		if err != nil || code < 100 || code > 599 {
			return nil, fmt.Errorf("status %q: %w (a code or class such as 4xx)", item, ErrInvalidFilter)
		}
		spans = append(spans, StatusRange{Low: code, High: code})
	}
	return spans, nil
}

func statusClass(token string) (StatusRange, bool) {
	if len(token) != 3 || token[1:] != "xx" {
		return StatusRange{}, false
	}
	if token[0] < '1' || token[0] > '5' {
		return StatusRange{}, false
	}
	low := int(token[0]-'0') * 100
	return StatusRange{Low: low, High: low + 99}, true
}
