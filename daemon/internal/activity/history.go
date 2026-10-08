// Usage history bounds and retention windows.
package activity

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"time"
)

// History bounds cap the usage ring, so one credential's traffic never
// crowds out every other's.
const (
	HistoryPerCredential = 200
	HistoryCredentials   = 64
	HistoryTotalRows     = 4096
	HistoryMaxAge        = 30 * 24 * time.Hour
)

// HistoryBounds are the fixed bounds the history ring is trimmed to.
type HistoryBounds struct {
	PerCredential int
	Credentials   int
	Total         int
	MaxAge        time.Duration
}

// DefaultHistoryBounds returns the bounds every Relo installation keeps.
func DefaultHistoryBounds() HistoryBounds {
	return HistoryBounds{
		PerCredential: HistoryPerCredential,
		Credentials:   HistoryCredentials,
		Total:         HistoryTotalRows,
		MaxAge:        HistoryMaxAge,
	}
}

// History appends observations and keeps the ring inside its bounds.
type History struct {
	repo   Repository
	bounds HistoryBounds
	logger *slog.Logger
}

// HistoryOptions tunes a history ring.
type HistoryOptions struct {
	Bounds HistoryBounds
	Logger *slog.Logger
}

// NewHistory returns a history ring over repo.
func NewHistory(repo Repository, options HistoryOptions) *History {
	settings := options
	if settings.Bounds.PerCredential == 0 {
		settings.Bounds = DefaultHistoryBounds()
	}
	if settings.Logger == nil {
		settings.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &History{repo: repo, bounds: settings.Bounds, logger: settings.Logger}
}

// Append stores samples and trims the ring.
func (h *History) Append(ctx context.Context, snapshots []Snapshot) error {
	percentages := make([]Snapshot, 0, len(snapshots))
	for _, snapshot := range snapshots {
		if !monetary(snapshot) {
			percentages = append(percentages, snapshot)
		}
	}
	if len(percentages) == 0 {
		return nil
	}
	if err := h.repo.AppendHistory(ctx, percentages); err != nil {
		return fmt.Errorf("store quota history: %w", err)
	}
	return h.Trim(ctx)
}

// Trim brings the ring back inside its bounds. A trim that fails is
// logged and returned, never fatal to the caller that observed the quota.
func (h *History) Trim(ctx context.Context) error {
	if err := h.repo.TrimHistory(ctx, h.bounds); err != nil {
		h.logger.Warn("quota history trim failed", "error", err)
		return fmt.Errorf("trim quota history: %w", err)
	}
	return nil
}
