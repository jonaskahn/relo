// Capture store: persisting inference request and response bodies.
package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jonaskahn/relo/internal/activity"
)

// Capture kinds match the activity vocabulary the data plane writes.
const (
	CaptureAgentRequest     = activity.CaptureAgentRequest
	CaptureAgentResponse    = activity.CaptureAgentResponse
	CaptureProviderRequest  = activity.CaptureProviderRequest
	CaptureProviderResponse = activity.CaptureProviderResponse
)

const (
	insertCaptureSQL = `INSERT INTO usage_captures (
	event_id, ordinal, kind, method, url, status, headers, body, body_bytes, truncated, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	selectCaptureManifestSQL = `SELECT id, ordinal, kind, method, url, status, headers, body_bytes, truncated
	FROM usage_captures WHERE event_id = ?
	ORDER BY CASE kind WHEN 'agent_request' THEN 0 WHEN 'agent_response' THEN 1
		WHEN 'provider_request' THEN 2 ELSE 3 END, ordinal, id`
	selectCaptureBodySQL = `SELECT body, body_bytes, truncated FROM usage_captures WHERE id = ? AND event_id = ?`
	countCapturesSQL     = `SELECT count(*) FROM usage_captures`
	// purgeCapturesSQL drops one batch of captured bodies. Usage is read from
	// the request rows and their daily archive, so the log keeps its numbers
	// when the messages that produced them are gone. The batch mirrors the
	// one the event rules delete in, so a purge of a large log writes a small
	// journal instead of one the size of everything it frees.
	purgeCapturesSQL = `DELETE FROM usage_captures WHERE id IN (
	SELECT id FROM usage_captures LIMIT ?
)`
)

// Capture is one message written beside a usage row.
type Capture = activity.CaptureWrite

// CaptureStore writes and reads the messages captured beside usage rows.
type CaptureStore struct {
	db *DB
}

// NewCaptureStore returns a capture store over the given database.
func NewCaptureStore(db *DB) *CaptureStore {
	return &CaptureStore{db: db}
}

// Append writes every capture of one request in a single transaction. A body
// larger than the caller's capture limit arrives already truncated, which the
// row records.
func (s *CaptureStore) Append(ctx context.Context, eventID int64, captures []Capture) error {
	if len(captures) == 0 {
		return nil
	}
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin request capture: %w", err)
	}
	for _, capture := range captures {
		capture.EventID = eventID
		if _, err := tx.ExecContext(ctx, insertCaptureSQL, captureArgs(capture, s.now())...); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("append request capture: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit request capture: %w", err)
	}
	return nil
}

// Manifest lists what was captured for one request, newest metadata first
// without any body.
func (s *CaptureStore) Manifest(ctx context.Context, eventID int64) ([]activity.CaptureHeader, error) {
	rows, err := s.db.sql.QueryContext(ctx, selectCaptureManifestSQL, eventID)
	if err != nil {
		return nil, fmt.Errorf("list request captures: %w", err)
	}
	defer func() { _ = rows.Close() }()
	captures := make([]activity.CaptureHeader, 0, 4)
	for rows.Next() {
		capture, err := scanCaptureHeader(rows)
		if err != nil {
			return nil, err
		}
		captures = append(captures, capture)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate request captures: %w", err)
	}
	return captures, nil
}

func scanCaptureHeader(rows *sql.Rows) (activity.CaptureHeader, error) {
	var (
		capture   activity.CaptureHeader
		ordinal   sql.NullInt64
		headers   sql.NullString
		truncated int
	)
	if err := rows.Scan(&capture.ID, &ordinal, &capture.Kind, &capture.Method, &capture.URL,
		&capture.Status, &headers, &capture.BodyBytes, &truncated); err != nil {
		return activity.CaptureHeader{}, fmt.Errorf("scan request capture: %w", err)
	}
	if ordinal.Valid {
		value := int(ordinal.Int64)
		capture.Ordinal = &value
	}
	capture.Headers = headersOf(headers.String)
	capture.Truncated = truncated != 0
	return capture, nil
}

// Body returns one stored body, refusing a capture that belongs to another
// request.
func (s *CaptureStore) Body(ctx context.Context, eventID, captureID int64) (activity.CaptureBody, error) {
	var (
		body      []byte
		bodyBytes int64
		truncated int
	)
	err := s.db.sql.QueryRowContext(ctx, selectCaptureBodySQL, captureID, eventID).
		Scan(&body, &bodyBytes, &truncated)
	if errors.Is(err, sql.ErrNoRows) {
		return activity.CaptureBody{}, activity.ErrCaptureNotFound
	}
	if err != nil {
		return activity.CaptureBody{}, fmt.Errorf("read request capture body: %w", err)
	}
	return activity.CaptureBody{Body: body, BodyBytes: bodyBytes, Truncated: truncated != 0}, nil
}

func captureArgs(capture Capture, now int64) []any {
	var ordinal any
	if capture.Ordinal != nil {
		ordinal = *capture.Ordinal
	}
	return []any{
		capture.EventID, ordinal, capture.Kind, capture.Method, capture.URL,
		capture.Status, headersArg(capture.Headers), capture.Body, len(capture.Body),
		boolCount(capture.Truncated), now,
	}
}

func headersArg(headers map[string][]string) string {
	if len(headers) == 0 {
		return "{}"
	}
	body, err := json.Marshal(headers)
	if err != nil {
		return "{}"
	}
	return string(body)
}

func headersOf(raw string) map[string][]string {
	if raw == "" || raw == "{}" {
		return map[string][]string{}
	}
	headers := map[string][]string{}
	if err := json.Unmarshal([]byte(raw), &headers); err != nil {
		return map[string][]string{}
	}
	return headers
}

func (s *CaptureStore) now() int64 {
	return time.Now().UnixMilli()
}
