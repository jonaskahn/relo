// Thread history a client leaves on the server.
//
// Claude Desktop does not send its conversation again once a thread starts: it
// sends the new turn plus the id of the reply it answers. Relo answers that
// turn, so only Relo can rebuild the history the reply belongs to. This
// package owns what is stored, and the store it is kept in.
package threads

import (
	"context"
	"errors"
	"time"

	"github.com/jonaskahn/relo/internal/inference"
)

// DefaultRetentionDays is how long a finished turn stays continuable when an
// operator never picked a window.
const DefaultRetentionDays = 30

// MinRetentionDays is the shortest window a stored thread may keep. Zero is
// the unlimited choice, like the usage-log budget it sits beside.
const MinRetentionDays = 1

// ErrInvalidRetention reports a window the store cannot honour.
var ErrInvalidRetention = errors.New("thread retention: keep every turn with 0 days, or at least one day")

// RetentionWindow reports a valid stored window, defaulting when an operator
// never picked one.
func RetentionWindow(days int) (int, error) {
	if days == 0 {
		return DefaultRetentionDays, nil
	}
	if days < MinRetentionDays {
		return 0, ErrInvalidRetention
	}
	return days, nil
}

// CutoffMs is the instant retention deletes before, measured from the start
// of the day so a window keeps whole days.
func CutoffMs(days int, now time.Time) int64 {
	if days <= 0 {
		return 0
	}
	utc := now.UTC()
	start := time.Date(utc.Year(), utc.Month(), utc.Day(), 0, 0, 0, 0, time.UTC)
	return start.Add(-time.Duration(days-1) * 24 * time.Hour).UnixMilli()
}

// Turn is one finished reply a client may continue from.
type Turn struct {
	ClientKey string
	ID        string
	ParentID  string
	// Delta is what this turn added: the client's new messages and the
	// assistant reply. A parent keeps its own row, so no turn is written twice.
	Delta []inference.Message
	// System and Tools are only stored on the turn that opened the thread.
	System *inference.Message
	Tools  []inference.Tool
	// UpdatedAtMs is what retention reads: a continued thread refreshes its
	// own deadline, and one nobody continues ages out.
	UpdatedAtMs int64
}

// Resolved is one thread rebuilt for a request.
type Resolved struct {
	// Messages is every turn in order, oldest first, without the system prompt.
	Messages []inference.Message
	System   *inference.Message
	Tools    []inference.Tool
	// Prefix is how many messages the stored history contributed, which is
	// where this request's own messages start.
	Prefix int
	NodeID string
}

// Store persists the turns a client continues from, and answers the windows
// retention applies.
type Store interface {
	Put(ctx context.Context, turn Turn) error
	Get(ctx context.Context, clientKey, id string) (Turn, bool, error)
	DeleteBefore(ctx context.Context, beforeMs int64) (int64, error)
	CountBefore(ctx context.Context, beforeMs int64) (int64, error)
}

// History rebuilds a conversation from the turn store.
type History struct {
	store Store
	now   func() time.Time
}

// NewHistory returns the history over the given store.
func NewHistory(store Store, now func() time.Time) *History {
	if now == nil {
		now = time.Now
	}
	return &History{store: store, now: now}
}

// Resolve rebuilds the thread one reply belongs to.
func (h *History) Resolve(ctx context.Context, clientKey, id string) (Resolved, bool, error) {
	if h == nil || h.store == nil || id == "" {
		return Resolved{}, false, nil
	}
	chain, found, err := h.chain(ctx, clientKey, id)
	if err != nil || !found {
		return Resolved{}, found, err
	}
	return assemble(chain, id), true, nil
}

// chain reads the turns one reply stands on, newest first.
func (h *History) chain(ctx context.Context, clientKey, id string) ([]Turn, bool, error) {
	var chain []Turn
	seen := map[string]bool{}
	for at := id; at != ""; {
		if seen[at] {
			return nil, false, nil
		}
		seen[at] = true
		turn, found, err := h.store.Get(ctx, clientKey, at)
		if err != nil || !found {
			return nil, found, err
		}
		chain = append(chain, turn)
		if len(chain) > maxChain {
			return nil, false, nil
		}
		at = turn.ParentID
	}
	return chain, true, nil
}

// assemble turns a newest-first chain into the conversation it names.
func assemble(chain []Turn, id string) Resolved {
	resolved := Resolved{NodeID: id}
	for index := len(chain) - 1; index >= 0; index-- {
		turn := chain[index]
		if turn.System != nil && resolved.System == nil {
			resolved.System = turn.System
		}
		if turn.Tools != nil && resolved.Tools == nil {
			resolved.Tools = turn.Tools
		}
		resolved.Messages = append(resolved.Messages, turn.Delta...)
	}
	resolved.Prefix = len(resolved.Messages)
	return resolved
}

// Record stores one finished turn, stamping it with the time retention reads.
func (h *History) Record(ctx context.Context, turn Turn) error {
	if h == nil || h.store == nil || turn.ID == "" {
		return nil
	}
	turn.UpdatedAtMs = h.now().UnixMilli()
	return h.store.Put(ctx, turn)
}

// maxChain bounds how many rows one lookup walks. A thread longer than this
// is no longer continuable, which is cheaper than a walk that never ends.
const maxChain = 512
