// Quota cache: recording vendor-reported windows.
package activity

import (
	"context"
	"fmt"
	"sort"

	"github.com/jonaskahn/relo/internal/clock"
)

// Cache keeps the current quota of every credential.
type Cache struct {
	repo   Repository
	clock  clock.Clock
	source string
}

// CacheOptions tunes a cache.
type CacheOptions struct {
	Clock  clock.Clock
	Source string
}

// NewCache returns a cache that writes snapshots through repo.
func NewCache(repo Repository, options CacheOptions) *Cache {
	settings := options
	if settings.Clock == nil {
		settings.Clock = clock.New()
	}
	return &Cache{repo: repo, clock: settings.Clock, source: settings.Source}
}

// Record stores the windows a probe returned.
func (c *Cache) Record(ctx context.Context, credentialID string, samples []WindowSample) error {
	snapshots := c.snapshots(credentialID, samples)
	if len(snapshots) == 0 {
		return nil
	}
	if err := c.repo.UpsertSnapshots(ctx, snapshots); err != nil {
		return fmt.Errorf("store quota snapshots: %w", err)
	}
	return nil
}

// Windows returns the stored windows of one credential, most constrained
// first.
func (c *Cache) Windows(ctx context.Context, credentialID string) ([]Snapshot, error) {
	snapshots, err := c.repo.ListSnapshots(ctx, credentialID)
	if err != nil {
		return nil, fmt.Errorf("read quota snapshots: %w", err)
	}
	sort.SliceStable(snapshots, func(i, j int) bool {
		left, right := snapshots[i], snapshots[j]
		if monetary(left) != monetary(right) {
			return !monetary(left)
		}
		if monetary(left) {
			return false
		}
		return left.Headroom() < right.Headroom()
	})
	return snapshots, nil
}

// Headroom returns the remaining percentage of one window, and whether a
// snapshot exists for it at all.
func (c *Cache) Headroom(ctx context.Context, credentialID, window string) (float64, bool) {
	snapshots, err := c.repo.ListSnapshots(ctx, credentialID)
	if err != nil {
		return 0, false
	}
	for _, snapshot := range snapshots {
		if snapshot.Window == window && !monetary(snapshot) {
			return snapshot.Headroom(), true
		}
	}
	return 0, false
}

// MostConstrained returns the smallest headroom across a credential's
// windows, which is what a request actually has to fit in.
func (c *Cache) MostConstrained(ctx context.Context, credentialID string) (float64, bool) {
	snapshots, err := c.Windows(ctx, credentialID)
	if err != nil {
		return 0, false
	}
	for _, snapshot := range snapshots {
		if !monetary(snapshot) {
			return snapshot.Headroom(), true
		}
	}
	return 0, false
}

// Lookup returns the headroom lookup of one credential, which is the seam
// the pool strategy reads.
func (c *Cache) Lookup(ctx context.Context, credentialID string) func() (float64, bool) {
	return func() (float64, bool) { return c.MostConstrained(ctx, credentialID) }
}

func (c *Cache) snapshots(credentialID string, samples []WindowSample) []Snapshot {
	now := c.clock.Now()
	snapshots := make([]Snapshot, 0, len(samples))
	for _, sample := range samples {
		if sample.Window == "" {
			continue
		}
		snapshots = append(snapshots, Snapshot{
			CredentialID: credentialID,
			Window:       sample.Window,
			UsedPercent:  sample.UsedPercent,
			Amount:       sample.Amount,
			Currency:     sample.Currency,
			ResetAt:      sample.ResetAt,
			Seconds:      sample.Seconds,
			Source:       c.source,
			UpdatedAt:    now,
		})
	}
	return snapshots
}

func monetary(snapshot Snapshot) bool {
	return snapshot.Amount != nil || snapshot.Currency != ""
}
