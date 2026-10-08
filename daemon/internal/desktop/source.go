// Tray data source: reading activity for the menu.
package desktop

import (
	"context"
	"time"
)

// ActivitySnapshot is one reading of the last day's usage: the rollup rows
// and the connections whose spend never counts. The command line builds it
// from the state database, so this package never imports the persistence
// adapter.
type ActivitySnapshot struct {
	Rows []UsageRow
	// UnpaidIDs names the connections whose spend never counts: plan-covered
	// and local ones, which the menu shows without money.
	UnpaidIDs map[string]bool
}

// ActivitySource reads one usage snapshot for the tray menu. The command
// line injects the database-backed reader, so the tray renders whatever the
// reader returns.
type ActivitySource interface {
	Snapshot(ctx context.Context, sinceMs int64) (ActivitySnapshot, error)
}

type activityData struct {
	totals usageTotals
	// ok is true once a read has answered, which is what tells an idle day
	// apart from a database the tray has not reached yet.
	ok bool
	// err is why the last read failed, which the log records and an operator
	// reads in the daemon log.
	err string
}

type desktopSource struct {
	source ActivitySource
}

func newDesktopSource(source ActivitySource) *desktopSource {
	return &desktopSource{source: source}
}

// Read totals the last day of usage. Spend follows the console's own rule:
// only pay-as-you-go connections count, so a plan's traffic never reads as
// money.
func (s *desktopSource) Read(ctx context.Context, now time.Time) (activityData, error) {
	snapshot, err := s.source.Snapshot(ctx, rollupSince(now))
	if err != nil {
		return activityData{}, err
	}
	return activityData{
		totals: totalsOf(withoutNonPaidSpend(snapshot.Rows, snapshot.UnpaidIDs)),
		ok:     true,
	}, nil
}

func rollupSince(now time.Time) int64 {
	return now.Add(-24 * time.Hour).UnixMilli()
}
