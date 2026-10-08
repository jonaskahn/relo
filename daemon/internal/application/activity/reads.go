// Package activity owns the usage reads an operator asks for: grouped
// rollups, the request log, the upstream attempts of one request, and the
// messages captured beside it.
package activity

import (
	"context"
	"errors"
	"fmt"
	"strings"

	act "github.com/jonaskahn/relo/internal/activity"
)

// ErrInvalidQuery reports a usage or log query a caller cannot run.
var ErrInvalidQuery = errors.New("invalid usage query")

// UsageReader reads the usage log.
type UsageReader interface {
	Rollup(ctx context.Context, query act.RollupQuery) ([]act.UsageRollupRow, error)
	Summary(ctx context.Context, filter act.UsageFilter) (act.UsageRollupRow, error)
	Logs(ctx context.Context, query act.LogQuery) (act.LogPage, error)
	Attempts(ctx context.Context, eventID int64) ([]act.UsageAttemptRow, error)
	RawFromMs(ctx context.Context) (int64, bool, error)
}

// CaptureReader reads the messages stored beside one request.
type CaptureReader interface {
	Manifest(ctx context.Context, eventID int64) ([]act.CaptureHeader, error)
	Body(ctx context.Context, eventID, captureID int64) (act.CaptureBody, error)
}

// UsageQuery is one grouped read of the usage log. A zero field means no
// constraint, except GroupBy, which a rollup always needs.
type UsageQuery struct {
	SinceMs  int64
	UntilMs  int64
	Provider string
	// Providers narrows the read to several connections at once, which is
	// how one query can total the paid connections together. When it is
	// set, Provider stays empty.
	Providers  []string
	Model      string
	Credential string
	// Account names one stored account identifier, which keeps two accounts
	// that share a label apart in a report.
	Account       string
	Surface       string
	RouteProvider string
	RequestID     string
	Client        string
	// AccessClient names the coding client a key was issued for, which the
	// event stores so a filter still finds the row after the key is gone.
	AccessClient string
	// Origin narrows the read to traffic a console started (internal) or
	// traffic a client sent through the data plane (external).
	Origin string
	// Status names the HTTP codes to match, as a comma-separated list of codes
	// (429) and classes (4xx). An empty value matches every row.
	Status  string
	GroupBy string
	Limit   int
}

// LogQuery is one page request against the request log.
type LogQuery struct {
	SinceMs       int64
	UntilMs       int64
	Provider      string
	Providers     []string
	Model         string
	Credential    string
	Account       string
	Surface       string
	RouteProvider string
	RequestID     string
	Client        string
	// AccessClient names the coding client a key was issued for, which the
	// event stores so a filter still finds the row after the key is gone.
	AccessClient string
	// Origin narrows the read to traffic a console started (internal) or
	// traffic a client sent through the data plane (external).
	Origin string
	// Status names the HTTP codes to match, as a comma-separated list of codes
	// (429) and classes (4xx). An empty value matches every row.
	Status string
	Cursor string
	Limit  int
}

// Options configure the usage reads.
type Options struct {
	Usage    UsageReader
	Captures CaptureReader
}

// Service is the usage reads.
type Service struct {
	usage    UsageReader
	captures CaptureReader
}

// New returns the usage reads over the given stores.
func New(options Options) *Service {
	return &Service{usage: options.Usage, captures: options.Captures}
}

// Usage returns the usage log of a filter, grouped by one dimension.
func (s *Service) Usage(ctx context.Context, query UsageQuery) ([]act.UsageRollupRow, error) {
	filter, err := query.filter()
	if err != nil {
		return nil, err
	}
	rows, err := s.usage.Rollup(ctx, act.RollupQuery{
		Filter:  filter,
		GroupBy: act.GroupBy(strings.TrimSpace(query.GroupBy)),
		Limit:   query.Limit,
	})
	if err != nil {
		return nil, wrapQueryError(err)
	}
	return rows, nil
}

// UsageSummary totals a filter into one row, independent of the group limit a
// rollup applies, so a page reports exact totals over every matching event.
func (s *Service) UsageSummary(ctx context.Context, query UsageQuery) (act.UsageRollupRow, error) {
	filter, err := query.filter()
	if err != nil {
		return act.UsageRollupRow{}, err
	}
	row, err := s.usage.Summary(ctx, filter)
	if err != nil {
		return act.UsageRollupRow{}, wrapQueryError(err)
	}
	return row, nil
}

// Logs returns one page of the request log, newest first.
func (s *Service) Logs(ctx context.Context, query LogQuery) (act.LogPage, error) {
	cursor, err := query.cursor()
	if err != nil {
		return act.LogPage{}, err
	}
	filter, err := query.filter()
	if err != nil {
		return act.LogPage{}, err
	}
	page, err := s.usage.Logs(ctx, act.LogQuery{Filter: filter, Cursor: cursor, Limit: query.Limit})
	if err != nil {
		return act.LogPage{}, wrapQueryError(err)
	}
	return page, nil
}

// Attempts returns the upstream sends of one logical request, in order.
func (s *Service) Attempts(ctx context.Context, eventID int64) ([]act.UsageAttemptRow, error) {
	return s.usage.Attempts(ctx, eventID)
}

// ArchiveFloor reports the first timestamp whose request rows are still
// retained, and whether the archive holds any closed day. A range that starts
// before it is answered at UTC day precision, because those rows are gone and
// their totals live in the daily archive.
func (s *Service) ArchiveFloor(ctx context.Context) (int64, bool, error) {
	return s.usage.RawFromMs(ctx)
}

// Captures lists what was captured beside one request.
func (s *Service) Captures(ctx context.Context, eventID int64) ([]act.CaptureHeader, error) {
	return s.captures.Manifest(ctx, eventID)
}

// CaptureBody returns the stored body of one capture of one request.
func (s *Service) CaptureBody(ctx context.Context, eventID, captureID int64) (act.CaptureBody, error) {
	return s.captures.Body(ctx, eventID, captureID)
}

func (q UsageQuery) filter() (act.UsageFilter, error) {
	status, err := act.ParseStatusFilter(q.Status)
	if err != nil {
		return act.UsageFilter{}, fmt.Errorf("%w: %s", ErrInvalidQuery, err.Error())
	}
	return act.UsageFilter{
		SinceMs: q.SinceMs, UntilMs: q.UntilMs, Provider: q.Provider, Providers: q.Providers, Model: q.Model,
		CredentialLabel: q.Credential, CredentialID: q.Account, Surface: q.Surface, RouteProvider: q.RouteProvider,
		RequestID: q.RequestID, ClientKeyID: q.Client, Status: status,
		AccessClient: q.AccessClient, Origin: q.Origin,
	}, nil
}

func (q LogQuery) filter() (act.UsageFilter, error) {
	return UsageQuery{
		SinceMs: q.SinceMs, UntilMs: q.UntilMs, Provider: q.Provider, Providers: q.Providers, Model: q.Model,
		Credential: q.Credential, Account: q.Account, Surface: q.Surface, RouteProvider: q.RouteProvider,
		RequestID: q.RequestID, Client: q.Client, AccessClient: q.AccessClient, Status: q.Status,
		Origin: q.Origin,
	}.filter()
}

func (q LogQuery) cursor() (*act.LogCursor, error) {
	if q.Cursor == "" {
		return nil, nil
	}
	cursor, err := act.DecodeLogCursor(q.Cursor)
	if err != nil {
		return nil, fmt.Errorf("cursor: %w", ErrInvalidQuery)
	}
	return &cursor, nil
}

func wrapQueryError(err error) error {
	if errors.Is(err, act.ErrInvalidFilter) || errors.Is(err, act.ErrInvalidCursor) {
		return fmt.Errorf("%w: %s", ErrInvalidQuery, err.Error())
	}
	return err
}
