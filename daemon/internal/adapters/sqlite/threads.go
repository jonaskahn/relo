// Persistence for the conversations a client continues on the server.
package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jonaskahn/relo/internal/inference"
	"github.com/jonaskahn/relo/internal/threads"
)

const (
	selectThreadTurnSQL = `SELECT parent_id, system, tools, delta FROM thread_turns WHERE client_key = ? AND id = ?`
	upsertThreadTurnSQL = `INSERT INTO thread_turns (client_key, id, parent_id, system, tools, delta, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (client_key, id) DO UPDATE SET
	parent_id = excluded.parent_id, system = excluded.system,
	tools = excluded.tools, delta = excluded.delta, updated_at = excluded.updated_at`
	deleteThreadTurnsBeforeSQL = `DELETE FROM thread_turns WHERE updated_at < ?`
	countThreadTurnsBeforeSQL  = `SELECT count(*) FROM thread_turns WHERE updated_at < ?`
)

// ThreadTurns stores one finished turn per row. A parent stays a row of its
// own, so a long conversation stores each turn once and a lookup walks the
// chain it names.
type ThreadTurns struct {
	db *DB
}

// NewThreadTurns returns the thread store over the database.
func NewThreadTurns(db *DB) *ThreadTurns {
	return &ThreadTurns{db: db}
}

// Put stores one turn under the identity of the reply it was answered with.
func (s *ThreadTurns) Put(ctx context.Context, turn threads.Turn) error {
	system, err := encodeOptionalMessage(turn.System)
	if err != nil {
		return err
	}
	tools, err := encodeValue(turn.Tools)
	if err != nil {
		return err
	}
	delta, err := encodeValue(turn.Delta)
	if err != nil {
		return err
	}
	if _, err := s.db.sql.ExecContext(ctx, upsertThreadTurnSQL,
		turn.ClientKey, turn.ID, turn.ParentID, system, tools, delta, turn.UpdatedAtMs); err != nil {
		return fmt.Errorf("store thread turn: %w", err)
	}
	return nil
}

// Get returns one turn, and whether it exists.
func (s *ThreadTurns) Get(ctx context.Context, clientKey, id string) (threads.Turn, bool, error) {
	turn := threads.Turn{ClientKey: clientKey, ID: id}
	var system, tools, delta string
	err := s.db.sql.QueryRowContext(ctx, selectThreadTurnSQL, clientKey, id).
		Scan(&turn.ParentID, &system, &tools, &delta)
	if errors.Is(err, sql.ErrNoRows) {
		return threads.Turn{}, false, nil
	}
	if err != nil {
		return threads.Turn{}, false, fmt.Errorf("read thread turn: %w", err)
	}
	if err := decodeOptionalMessage(system, &turn.System); err != nil {
		return threads.Turn{}, false, err
	}
	if err := decodeValue(tools, &turn.Tools); err != nil {
		return threads.Turn{}, false, err
	}
	if err := decodeValue(delta, &turn.Delta); err != nil {
		return threads.Turn{}, false, err
	}
	return turn, true, nil
}

// DeleteBefore removes every turn last written before one instant, which is
// the retention pass the daemon runs on its maintenance tick.
func (s *ThreadTurns) DeleteBefore(ctx context.Context, beforeMs int64) (int64, error) {
	result, err := s.db.sql.ExecContext(ctx, deleteThreadTurnsBeforeSQL, beforeMs)
	if err != nil {
		return 0, fmt.Errorf("delete thread turns: %w", err)
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("count deleted thread turns: %w", err)
	}
	return deleted, nil
}

// CountBefore counts the turns a retention pass would delete.
func (s *ThreadTurns) CountBefore(ctx context.Context, beforeMs int64) (int64, error) {
	var count int64
	if err := s.db.sql.QueryRowContext(ctx, countThreadTurnsBeforeSQL, beforeMs).Scan(&count); err != nil {
		return 0, fmt.Errorf("count thread turns: %w", err)
	}
	return count, nil
}

func encodeOptionalMessage(message *inference.Message) (string, error) {
	if message == nil {
		return "", nil
	}
	return encodeValue(*message)
}

func decodeOptionalMessage(raw string, message **inference.Message) error {
	if raw == "" {
		return nil
	}
	var decoded inference.Message
	if err := decodeValue(raw, &decoded); err != nil {
		return err
	}
	*message = &decoded
	return nil
}

func encodeValue(value any) (string, error) {
	switch typed := value.(type) {
	case nil:
		return "", nil
	case []inference.Message:
		if len(typed) == 0 {
			return "", nil
		}
	case []inference.Tool:
		if len(typed) == 0 {
			return "", nil
		}
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("encode thread turn: %w", err)
	}
	return string(encoded), nil
}

func decodeValue(raw string, target any) error {
	if raw == "" {
		return nil
	}
	if err := json.Unmarshal([]byte(raw), target); err != nil {
		return fmt.Errorf("decode thread turn: %w", err)
	}
	return nil
}
