package storage_test

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/activity"
	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	"github.com/jonaskahn/relo/tests/testkit"
)

var fixedNow = time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

type drainProbe bool

func (d drainProbe) Drained() bool { return bool(d) }

func newRetention(t *testing.T, db *sqlite.DB, options ...func(*sqlite.RetentionOptions)) *sqlite.Retention {
	t.Helper()
	settings := sqlite.RetentionOptions{Now: func() time.Time { return fixedNow }}
	for _, option := range options {
		option(&settings)
	}
	return sqlite.NewRetention(db, settings)
}

func withDrain(drained bool) func(*sqlite.RetentionOptions) {
	return func(options *sqlite.RetentionOptions) { options.Drain = drainProbe(drained) }
}

// seedDays writes count events per day, oldest first, for the days ending
// yesterday relative to fixedNow.
func seedDays(t *testing.T, db *sqlite.DB, days int, perDay int) []int64 {
	t.Helper()
	recorder := newRecorder(t, db)
	ids := make([]int64, 0, days*perDay)
	for offset := days - 1; offset >= 0; offset-- {
		at := fixedNow.Add(-time.Duration(offset) * 24 * time.Hour)
		for index := 0; index < perDay; index++ {
			ids = append(ids, appendEvent(t, recorder, sqlite.UsageEvent{
				RequestID: "req-" + strconv.Itoa(offset) + "-" + strconv.Itoa(index),
				Timestamp: at.UnixMilli() + int64(index),
				Provider:  "openai", Model: "gpt-4o", Status: 200,
			}))
		}
	}
	return ids
}

func timestamps(t *testing.T, db *sqlite.DB) []int64 {
	t.Helper()
	rows, err := db.SQL().QueryContext(context.Background(), "SELECT timestamp FROM usage_events ORDER BY timestamp, id")
	if err != nil {
		t.Fatalf("read timestamps: %v", err)
	}
	defer func() { _ = rows.Close() }()
	stamps := make([]int64, 0, 16)
	for rows.Next() {
		var stamp int64
		if err := rows.Scan(&stamp); err != nil {
			t.Fatalf("scan timestamp: %v", err)
		}
		stamps = append(stamps, stamp)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate timestamps: %v", err)
	}
	return stamps
}

func TestRetentionConfig(t *testing.T) {
	db := testkit.OpenTestDB(t)
	retention := newRetention(t, db)
	ctx := context.Background()

	t.Run("absent config", func(t *testing.T) {
		config, stored, err := retention.Config(ctx)
		if err != nil || stored {
			t.Fatalf("Config() = %+v, %v, %v, want no stored budget", config, stored, err)
		}
	})

	t.Run("round trip", func(t *testing.T) {
		want := sqlite.RetentionConfig{UsageDays: 30, MaxEvents: 1000, MaxBytes: 1 << 20}
		if err := retention.SaveConfig(ctx, want); err != nil {
			t.Fatalf("SaveConfig() error = %v", err)
		}
		stored, found, err := retention.Config(ctx)
		if err != nil || !found {
			t.Fatalf("Config() found = %v, error = %v", found, err)
		}
		if stored.UsageDays != want.UsageDays || stored.MaxEvents != want.MaxEvents || stored.MaxBytes != want.MaxBytes {
			t.Fatalf("Config() = %+v, want %+v", stored, want)
		}
		if stored.UpdatedAtMs != fixedNow.UnixMilli() {
			t.Fatalf("UpdatedAtMs = %d, want %d", stored.UpdatedAtMs, fixedNow.UnixMilli())
		}
	})

	t.Run("negative budgets are refused", func(t *testing.T) {
		cases := []sqlite.RetentionConfig{
			{UsageDays: -1}, {MaxEvents: -1}, {MaxBytes: -1},
		}
		for _, config := range cases {
			if err := retention.SaveConfig(ctx, config); !errors.Is(err, activity.ErrInvalidRetention) {
				t.Fatalf("SaveConfig(%+v) error = %v, want ErrInvalidRetention", config, err)
			}
			if _, err := retention.Enforce(ctx, config); !errors.Is(err, activity.ErrInvalidRetention) {
				t.Fatalf("Enforce(%+v) error = %v, want ErrInvalidRetention", config, err)
			}
			if _, err := retention.Preview(ctx, config); !errors.Is(err, activity.ErrInvalidRetention) {
				t.Fatalf("Preview(%+v) error = %v, want ErrInvalidRetention", config, err)
			}
		}
	})

	t.Run("a usage budget below the floor is refused", func(t *testing.T) {
		for _, days := range []int{1, 2} {
			config := sqlite.RetentionConfig{UsageDays: days}
			if err := retention.SaveConfig(ctx, config); !errors.Is(err, activity.ErrInvalidRetention) {
				t.Fatalf("SaveConfig(usage_days=%d) error = %v, want ErrInvalidRetention", days, err)
			}
			if _, err := retention.Enforce(ctx, config); !errors.Is(err, activity.ErrInvalidRetention) {
				t.Fatalf("Enforce(usage_days=%d) error = %v, want ErrInvalidRetention", days, err)
			}
		}
		for _, days := range []int{0, activity.MinUsageDays, sqlite.DefaultUsageDays} {
			if err := retention.SaveConfig(ctx, sqlite.RetentionConfig{UsageDays: days}); err != nil {
				t.Fatalf("SaveConfig(usage_days=%d) error = %v, want the budget stored", days, err)
			}
		}
	})
}

// TestStoredWindowBelowTheFloorIsLifted covers a floor that moved above a
// stored budget, the way an older build that allowed one day left it: the
// pass honors the floor, stores it, and refuses no run.
func TestStoredWindowBelowTheFloorIsLifted(t *testing.T) {
	db := testkit.OpenTestDB(t)
	retention := newRetention(t, db)
	ctx := context.Background()
	if _, err := db.SQL().ExecContext(ctx,
		`INSERT INTO retention_config (id, usage_days, max_events, max_bytes, updated_at) VALUES (1, 1, 0, 0, ?)`,
		fixedNow.UnixMilli()); err != nil {
		t.Fatalf("store a short window: %v", err)
	}

	if _, err := retention.RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce() error = %v, want the floor honored", err)
	}
	stored, found, err := retention.Config(ctx)
	if err != nil || !found {
		t.Fatalf("Config() found = %v, error = %v", found, err)
	}
	if stored.UsageDays != activity.MinUsageDays {
		t.Fatalf("stored window = %d, want the floor %d", stored.UsageDays, activity.MinUsageDays)
	}
}

func TestRetentionPreviewNeverMutates(t *testing.T) {
	db := testkit.OpenTestDB(t)
	retention := newRetention(t, db)
	seedDays(t, db, 10, 3)
	before := countRows(t, db, "usage_events")

	report, err := retention.Preview(context.Background(), sqlite.RetentionConfig{UsageDays: 5})
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}
	if after := countRows(t, db, "usage_events"); after != before {
		t.Fatalf("usage_events holds %d rows after a preview, want %d", after, before)
	}
	if report.RowsBefore != int64(before) {
		t.Fatalf("RowsBefore = %d, want %d", report.RowsBefore, before)
	}
	if report.RowsDeleted != 15 {
		t.Fatalf("RowsDeleted = %d, want the five days older than the budget", report.RowsDeleted)
	}
	// The cutoff falls on a UTC day boundary, so the closed days the archive
	// holds cover exactly the rows a deletion removes.
	wantCutoff := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC).Add(-4 * 24 * time.Hour).UnixMilli()
	if report.AgeCutoffMs != wantCutoff {
		t.Fatalf("AgeCutoffMs = %d, want the start of %d", report.AgeCutoffMs, wantCutoff)
	}
	if report.EstimatedBytesFreed <= 0 {
		t.Fatalf("EstimatedBytesFreed = %d, want a positive estimate", report.EstimatedBytesFreed)
	}
}

func TestRetentionDeletesOldestFirst(t *testing.T) {
	db := testkit.OpenTestDB(t)
	retention := newRetention(t, db)
	seedDays(t, db, 10, 3)

	report, err := retention.Enforce(context.Background(), sqlite.RetentionConfig{MaxEvents: 12})
	if err != nil {
		t.Fatalf("Enforce() error = %v", err)
	}
	// The floor keeps the three newest days out of the count rule's reach:
	// only the rows before it are eligible, so nine of their twenty-one go.
	if report.RowsDeleted != 9 {
		t.Fatalf("RowsDeleted = %d, want 9", report.RowsDeleted)
	}
	if left := countRows(t, db, "usage_events"); left != 21 {
		t.Fatalf("usage_events holds %d rows, want 21", left)
	}
	stamps := timestamps(t, db)
	if len(stamps) != 21 {
		t.Fatalf("read %d timestamps, want 21", len(stamps))
	}
	// The three oldest days were the ones the count rule removed.
	oldestKept := fixedNow.Add(-6 * 24 * time.Hour).UnixMilli()
	if stamps[0] != oldestKept {
		t.Fatalf("oldest remaining timestamp = %d, want %d", stamps[0], oldestKept)
	}
	if report.LiveBytesAfter > report.LiveBytesBefore {
		t.Fatalf("live bytes grew from %d to %d", report.LiveBytesBefore, report.LiveBytesAfter)
	}
}

func TestRetentionAgeRule(t *testing.T) {
	db := testkit.OpenTestDB(t)
	retention := newRetention(t, db)
	seedDays(t, db, 4, 2)

	report, err := retention.Enforce(context.Background(), sqlite.RetentionConfig{UsageDays: 3})
	if err != nil {
		t.Fatalf("Enforce() error = %v", err)
	}
	// A budget of three days keeps today and the two days before it, so the
	// day before those is the one that goes.
	if report.RowsDeleted != 2 {
		t.Fatalf("RowsDeleted = %d, want the day outside the budget", report.RowsDeleted)
	}
	if left := countRows(t, db, "usage_events"); left != 6 {
		t.Fatalf("usage_events holds %d rows, want 6", left)
	}
	if report.AgeCutoffMs != time.Date(2026, 8, 30, 0, 0, 0, 0, time.UTC).UnixMilli() {
		t.Fatalf("AgeCutoffMs = %d, want the start of 2026-08-30", report.AgeCutoffMs)
	}
	// The deleted day stays readable: every closed day was aggregated before
	// its rows were removed.
	days := finalizedDays(t, db)
	for _, day := range []string{"2026-08-29", "2026-08-30", "2026-08-31"} {
		if !days[day] {
			t.Fatalf("day %s was deleted without being closed: %v", day, days)
		}
	}
	rows := archivedRequests(t, db)
	if rows != 6 {
		t.Fatalf("the archive holds %d requests, want the six closed ones", rows)
	}
}

// TestRetentionKeepsTheFloorAcrossRules covers the promise the console
// makes: no budget, however aggressive, deletes a row inside the floor, so
// every day the ledger promises keeps the rows its totals were written from.
func TestRetentionKeepsTheFloorAcrossRules(t *testing.T) {
	db := testkit.OpenTestDB(t)
	retention := newRetention(t, db)
	seedDays(t, db, 5, 2)

	report, err := retention.Enforce(context.Background(), sqlite.RetentionConfig{MaxEvents: 1, MaxBytes: 1})
	if err != nil {
		t.Fatalf("Enforce() error = %v", err)
	}
	// Four rows sit before the floor and every one of them is eligible; the
	// six inside it stay even under a one-byte budget.
	if report.RowsDeleted != 4 {
		t.Fatalf("RowsDeleted = %d, want the four rows before the floor", report.RowsDeleted)
	}
	if left := countRows(t, db, "usage_events"); left != 6 {
		t.Fatalf("usage_events holds %d rows, want the six inside the floor", left)
	}
	floor := time.Date(2026, 8, 30, 0, 0, 0, 0, time.UTC).UnixMilli()
	for _, stamp := range timestamps(t, db) {
		if stamp < floor {
			t.Fatalf("a row at %d survived inside the floor's reach", stamp)
		}
	}
}

func TestRetentionRunOnceAppliesTheStoredBudget(t *testing.T) {
	db := testkit.OpenTestDB(t)
	retention := newRetention(t, db, withDrain(true))
	seedDays(t, db, 5, 2)
	ctx := context.Background()
	if err := retention.SaveConfig(ctx, sqlite.RetentionConfig{UsageDays: 3}); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	report, err := retention.RunOnce(ctx)
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if report.RowsDeleted != 4 {
		t.Fatalf("RowsDeleted = %d, want the four rows outside the budget", report.RowsDeleted)
	}
	if left := countRows(t, db, "usage_events"); left != 6 {
		t.Fatalf("usage_events holds %d rows, want 6", left)
	}
}

// TestRunOnceRunsOnePassAtATime covers the lock every maintenance trigger
// shares: concurrent saves, the worker tick, and the console button never
// delete the same rows twice.
func TestRunOnceRunsOnePassAtATime(t *testing.T) {
	db := testkit.OpenTestDB(t)
	retention := newRetention(t, db, withDrain(true))
	seedDays(t, db, 5, 2)
	ctx := context.Background()
	if err := retention.SaveConfig(ctx, sqlite.RetentionConfig{UsageDays: 3}); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	const passes = 6
	start := make(chan struct{})
	var wg sync.WaitGroup
	var deleted atomic.Int64
	for index := 0; index < passes; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			report, err := retention.RunOnce(ctx)
			if err != nil {
				if errors.Is(err, activity.ErrRetentionBusy) {
					return
				}
				t.Errorf("RunOnce() error = %v", err)
				return
			}
			deleted.Add(report.RowsDeleted)
		}()
	}
	close(start)
	wg.Wait()
	if got := deleted.Load(); got != 4 {
		t.Fatalf("concurrent passes deleted %d rows, want the four outside the budget once", got)
	}
	if left := countRows(t, db, "usage_events"); left != 6 {
		t.Fatalf("usage_events holds %d rows, want 6", left)
	}
}

// TestLateMidnightRequestCountsOnce is the race between recording and
// finalizing: a request that started before midnight lands its row and its
// day total while a pass is closing that day, and the archive counts it
// exactly once.
func TestLateMidnightRequestCountsOnce(t *testing.T) {
	db := testkit.OpenTestDB(t)
	ctx := context.Background()
	// The pass runs thirty seconds after midnight.
	midnight := time.Date(2026, 9, 1, 0, 0, 30, 0, time.UTC)
	retention := newRetention(t, db, withDrain(true), func(options *sqlite.RetentionOptions) {
		options.Now = func() time.Time { return midnight }
	})
	recorder := newRecorder(t, db)

	const events = 200
	var wg sync.WaitGroup
	stop := make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(stop)
		for index := 0; index < events; index++ {
			_, err := recorder.AppendEvent(ctx, sqlite.UsageEvent{
				RequestID: fmt.Sprintf("late-%d", index),
				Timestamp: time.Date(2026, 8, 31, 23, 59, 0, 0, time.UTC).UnixMilli() + int64(index),
				Provider:  "openai", Model: "gpt-4o", Status: 200, Attempts: 1,
			})
			if err != nil {
				t.Errorf("AppendEvent(%d) error = %v", index, err)
				return
			}
		}
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			if _, err := retention.Enforce(ctx, sqlite.RetentionConfig{UsageDays: 3}); err != nil {
				t.Errorf("Enforce() error = %v", err)
				return
			}
		}
	}()
	wg.Wait()
	if _, err := retention.Enforce(ctx, sqlite.RetentionConfig{UsageDays: 3}); err != nil {
		t.Fatalf("final Enforce() error = %v", err)
	}

	var requests int64
	if err := db.SQL().QueryRowContext(ctx, "SELECT sum(requests) FROM usage_daily WHERE day = '2026-08-31'").Scan(&requests); err != nil {
		t.Fatalf("read the day total: %v", err)
	}
	if requests != events {
		t.Fatalf("the day holds %d requests, want each of the %d recorded once", requests, events)
	}
	if left := countRows(t, db, "usage_events"); left != events {
		t.Fatalf("usage_events holds %d rows, want %d", left, events)
	}
}

// TestTwoHandlesCloseEachDayOnce runs the same pass on two database handles,
// the way a second process would: a day is still closed exactly once, and
// neither pass loses the other's totals.
func TestTwoHandlesCloseEachDayOnce(t *testing.T) {
	first := testkit.OpenTestDB(t)
	seedDays(t, first, 4, 3)
	logger, _ := testkit.TestLogger(t)
	second, err := sqlite.OpenDB(first.Path(), logger)
	if err != nil {
		t.Fatalf("OpenDB(second) error = %v", err)
	}
	t.Cleanup(func() { _ = second.Close() })

	ctx := context.Background()
	passes := []*sqlite.Retention{
		newRetention(t, first, withDrain(true)),
		newRetention(t, second, withDrain(true)),
	}
	start := make(chan struct{})
	var wg sync.WaitGroup
	for _, retention := range passes {
		wg.Add(1)
		go func(retention *sqlite.Retention) {
			defer wg.Done()
			<-start
			if _, err := retention.Enforce(ctx, sqlite.RetentionConfig{UsageDays: 3}); err != nil {
				t.Errorf("Enforce() error = %v", err)
			}
		}(retention)
	}
	close(start)
	wg.Wait()

	// Every seeded request is archived exactly once, and only the rows
	// outside the budget are gone. Closed days total nine; today's live
	// aggregate adds the other three without doubling any of them.
	var live int64
	if err := first.SQL().QueryRowContext(ctx, "SELECT COALESCE(SUM(requests), 0) FROM usage_daily").Scan(&live); err != nil {
		t.Fatalf("read the live aggregate: %v", err)
	}
	if live != 12 {
		t.Fatalf("the response ledger holds %d requests, want all twelve", live)
	}
	if total := archivedRequests(t, first); total != 9 {
		t.Fatalf("the closed days hold %d requests, want the nine of the kept closed days", total)
	}
	if left := countRows(t, first, "usage_events"); left != 9 {
		t.Fatalf("usage_events holds %d rows, want the nine kept days", left)
	}
}

// finalizedDays names every day the archive closed.
func finalizedDays(t *testing.T, db *sqlite.DB) map[string]bool {
	t.Helper()
	rows, err := db.SQL().QueryContext(context.Background(), "SELECT day, finalized FROM usage_days")
	if err != nil {
		t.Fatalf("read the archived days: %v", err)
	}
	defer func() { _ = rows.Close() }()
	days := map[string]bool{}
	for rows.Next() {
		var day string
		var finalized int
		if err := rows.Scan(&day, &finalized); err != nil {
			t.Fatalf("scan an archived day: %v", err)
		}
		days[day] = finalized != 0
	}
	return days
}

// archivedRequests totals the requests the closed days of the daily archive
// hold. A day that is still open contributes nothing here: its live total is
// read from the request log.
func archivedRequests(t *testing.T, db *sqlite.DB) int64 {
	t.Helper()
	const query = `SELECT COALESCE(SUM(requests), 0) FROM usage_daily
	WHERE day <= (SELECT max(day) FROM usage_days WHERE finalized = 1)`
	var total int64
	if err := db.SQL().QueryRowContext(context.Background(), query).Scan(&total); err != nil {
		t.Fatalf("read the archived requests: %v", err)
	}
	return total
}

func TestRetentionByteBudget(t *testing.T) {
	db := testkit.OpenTestDB(t)
	retention := newRetention(t, db)
	// Twenty days leave seventeen eligible above the three-day floor, enough
	// for the byte rule to reach half the database.
	seedDays(t, db, 20, 400)
	live, err := retention.LiveBytes(context.Background())
	if err != nil {
		t.Fatalf("LiveBytes() error = %v", err)
	}
	target := live / 2

	report, err := retention.Enforce(context.Background(), sqlite.RetentionConfig{MaxBytes: target})
	if err != nil {
		t.Fatalf("Enforce() error = %v", err)
	}
	if report.RowsDeleted == 0 {
		t.Fatal("Enforce() deleted nothing while the database was over the byte budget")
	}
	if report.LiveBytesAfter > target {
		t.Fatalf("LiveBytesAfter = %d, want at most %d", report.LiveBytesAfter, target)
	}
	if left := countRows(t, db, "usage_events"); int64(left) == int64(20*400) {
		t.Fatal("usage_events still holds every row after a byte trim")
	}
}

func TestRetentionEmptyLog(t *testing.T) {
	db := testkit.OpenTestDB(t)
	retention := newRetention(t, db)
	report, err := retention.Enforce(context.Background(), sqlite.RetentionConfig{UsageDays: 3, MaxEvents: 1, MaxBytes: 1})
	if err != nil {
		t.Fatalf("Enforce() error = %v", err)
	}
	if report.RowsBefore != 0 || report.RowsDeleted != 0 {
		t.Fatalf("Enforce() on an empty log = %+v, want no rows touched", report)
	}
}

func TestRetentionVacuum(t *testing.T) {
	ctx := context.Background()

	t.Run("refused while requests are in flight", func(t *testing.T) {
		db := testkit.OpenTestDB(t)
		retention := newRetention(t, db, withDrain(false))
		seedDays(t, db, 2, 2)
		if _, err := retention.Enforce(ctx, sqlite.RetentionConfig{MaxEvents: 1}); err != nil {
			t.Fatalf("Enforce() error = %v", err)
		}
		if err := retention.Vacuum(ctx); !errors.Is(err, sqlite.ErrNotDrained) {
			t.Fatalf("Vacuum() error = %v, want ErrNotDrained", err)
		}
	})

	t.Run("compacts once drained", func(t *testing.T) {
		db := testkit.OpenTestDB(t)
		retention := newRetention(t, db, withDrain(true))
		seedDays(t, db, 4, 300)
		// The seeded size is the reference a compaction must beat. Comparing
		// against the size right after the trim would move with any schema
		// the build adds, which is not what this guards.
		seeded, err := retention.LiveBytes(ctx)
		if err != nil {
			t.Fatalf("LiveBytes() error = %v", err)
		}
		if _, err := retention.Enforce(ctx, sqlite.RetentionConfig{MaxEvents: 50}); err != nil {
			t.Fatalf("Enforce() error = %v", err)
		}
		if err := retention.Vacuum(ctx); err != nil {
			t.Fatalf("Vacuum() error = %v", err)
		}
		after, err := retention.LiveBytes(ctx)
		if err != nil {
			t.Fatalf("LiveBytes() error = %v", err)
		}
		if after >= seeded {
			t.Fatalf("live bytes %d did not shrink below the seeded %d across a trim and vacuum", after, seeded)
		}
	})
}

// TestMaintenanceReclaimsLeftoverSpace covers the tick that returns space an
// earlier pass freed: a pass that deletes nothing still compacts once the
// free pages are worth a rewrite.
func TestMaintenanceReclaimsLeftoverSpace(t *testing.T) {
	db := testkit.OpenTestDB(t)
	ctx := context.Background()
	seedDays(t, db, 20, 800)
	busy := newRetention(t, db, withDrain(false))
	if err := busy.SaveConfig(ctx, sqlite.RetentionConfig{MaxEvents: 100}); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}
	// The first tick deletes the surplus; its compaction waits for a quiet
	// data plane, which this store never sees.
	busy.MaintainOnce(ctx)
	before, err := busy.LiveBytes(ctx)
	if err != nil {
		t.Fatalf("LiveBytes() error = %v", err)
	}

	// The next tick has nothing to delete and still returns the free pages.
	quiet := newRetention(t, db, withDrain(true))
	quiet.MaintainOnce(ctx)
	after, err := quiet.LiveBytes(ctx)
	if err != nil {
		t.Fatalf("LiveBytes() error = %v", err)
	}
	if after >= before {
		t.Fatalf("live bytes %d did not fall below %d across a quiet tick", after, before)
	}
}

func TestRetentionLiveBytes(t *testing.T) {
	db := testkit.OpenTestDB(t)
	retention := newRetention(t, db)
	empty, err := retention.LiveBytes(context.Background())
	if err != nil {
		t.Fatalf("LiveBytes() error = %v", err)
	}
	if empty <= 0 {
		t.Fatalf("LiveBytes() = %d, want a positive page count", empty)
	}
	seedDays(t, db, 3, 200)
	seeded, err := retention.LiveBytes(context.Background())
	if err != nil {
		t.Fatalf("LiveBytes() error = %v", err)
	}
	if seeded <= empty {
		t.Fatalf("LiveBytes() = %d after seeding, want more than %d", seeded, empty)
	}
	pageSize := pragmaInt(t, db, "page_size")
	if seeded%int64(pageSize) != 0 {
		t.Fatalf("LiveBytes() = %d, want a whole number of %d-byte pages", seeded, pageSize)
	}
}

func TestRetentionRun(t *testing.T) {
	db := testkit.OpenTestDB(t)
	retention := newRetention(t, db, withDrain(true))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	seedDays(t, db, 5, 2)
	if err := retention.SaveConfig(ctx, sqlite.RetentionConfig{MaxEvents: 2}); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}
	inMaintenance := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		close(inMaintenance)
		done <- retention.Run(ctx, 5*time.Millisecond)
	}()
	<-inMaintenance
	waitForRows(t, db, 8)
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run() never returned after its context was cancelled")
	}

	t.Run("no stored budget is a no-op", func(t *testing.T) {
		fresh := testkit.OpenTestDB(t)
		seedDays(t, fresh, 2, 2)
		quiet := newRetention(t, fresh)
		ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
		defer cancel()
		if err := quiet.Run(ctx, 5*time.Millisecond); err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if left := countRows(t, fresh, "usage_events"); left != 4 {
			t.Fatalf("usage_events holds %d rows, want 4", left)
		}
	})
}

func waitForRows(t *testing.T, db *sqlite.DB, want int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if countRows(t, db, "usage_events") == want {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("usage_events still holds %d rows, want %d", countRows(t, db, "usage_events"), want)
}

func TestRetentionDefaults(t *testing.T) {
	db := testkit.OpenTestDB(t)
	retention := sqlite.NewRetention(db, sqlite.RetentionOptions{PageSize: 4096})
	seedDays(t, db, 3, 4)
	ctx := context.Background()

	live, err := retention.LiveBytes(ctx)
	if err != nil {
		t.Fatalf("LiveBytes() error = %v", err)
	}
	if live%4096 != 0 {
		t.Fatalf("LiveBytes() = %d, want a whole number of 4096-byte pages", live)
	}
	if err := retention.SaveConfig(ctx, sqlite.RetentionConfig{MaxEvents: 1000}); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}
	stored, found, err := retention.Config(ctx)
	if err != nil || !found {
		t.Fatalf("Config() found = %v, error = %v", found, err)
	}
	if stored.UpdatedAtMs == 0 {
		t.Fatal("SaveConfig() stored no timestamp")
	}
	report, err := retention.Enforce(ctx, sqlite.RetentionConfig{})
	if err != nil {
		t.Fatalf("Enforce() with no budget error = %v", err)
	}
	if report.RowsDeleted != 0 || countRows(t, db, "usage_events") != 12 {
		t.Fatalf("Enforce() with no budget = %+v, want no deletions", report)
	}
	report, err = retention.Enforce(ctx, sqlite.RetentionConfig{MaxEvents: 1000})
	if err != nil {
		t.Fatalf("Enforce() error = %v", err)
	}
	if report.RowsDeleted != 0 {
		t.Fatalf("Enforce() under the byte and count budgets deleted %d rows", report.RowsDeleted)
	}
	if preview, err := retention.Preview(ctx, sqlite.RetentionConfig{}); err != nil {
		t.Fatalf("Preview() error = %v", err)
	} else if preview.EstimatedBytesFreed != 0 {
		t.Fatalf("Preview() estimated %d freed bytes for a budget that deletes nothing", preview.EstimatedBytesFreed)
	}
}

func TestRetentionReportsClosedDatabase(t *testing.T) {
	db := testkit.OpenTestDB(t)
	retention := newRetention(t, db)
	seedDays(t, db, 2, 2)
	ctx := context.Background()
	if err := db.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if _, _, err := retention.Config(ctx); err == nil {
		t.Fatal("Config() on a closed database returned no error")
	}
	if err := retention.SaveConfig(ctx, sqlite.RetentionConfig{UsageDays: 3}); err == nil {
		t.Fatal("SaveConfig() on a closed database returned no error")
	}
	if _, err := retention.Preview(ctx, sqlite.RetentionConfig{UsageDays: 3}); err == nil {
		t.Fatal("Preview() on a closed database returned no error")
	}
	if _, err := retention.Enforce(ctx, sqlite.RetentionConfig{UsageDays: 3}); err == nil {
		t.Fatal("Enforce() on a closed database returned no error")
	}
	if _, err := retention.LiveBytes(ctx); err == nil {
		t.Fatal("LiveBytes() on a closed database returned no error")
	}
	if err := retention.Vacuum(ctx); err == nil {
		t.Fatal("Vacuum() on a closed database returned no error")
	}
}

func TestRetentionRunSurvivesFailures(t *testing.T) {
	db := testkit.OpenTestDB(t)
	retention := newRetention(t, db)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := retention.SaveConfig(ctx, sqlite.RetentionConfig{UsageDays: 3}); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- retention.Run(ctx, time.Millisecond) }()
	time.Sleep(30 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run() error = %v, want a cancelled pass to log and return", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run() never returned")
	}
}

func TestRetentionRefusesVacuumOnlyWhenNotDrained(t *testing.T) {
	db := testkit.OpenTestDB(t)
	retention := sqlite.NewRetention(db, sqlite.RetentionOptions{
		Now:      func() time.Time { return fixedNow },
		PageSize: 4096,
		Drain:    drainProbe(true),
	})
	if err := retention.Vacuum(context.Background()); err != nil {
		t.Fatalf("Vacuum() while drained error = %v", err)
	}
}
