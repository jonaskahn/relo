// The data-plane history and the retention window that governs it.
package server

import (
	"context"

	"github.com/jonaskahn/relo/internal/threads"
)

// ThreadHistory rebuilds the conversation a client continues from.
type ThreadHistory interface {
	Resolve(ctx context.Context, clientKey, id string) (threads.Resolved, bool, error)
	Record(ctx context.Context, turn threads.Turn) error
}

// ThreadRetention reads, stores, and applies how long a continued thread
// stays recoverable.
type ThreadRetention interface {
	Days(ctx context.Context) (int, bool, error)
	SaveDays(ctx context.Context, days int) error
	Sweep(ctx context.Context) (int64, error)
	Preview(ctx context.Context, days int) (int64, error)
}
