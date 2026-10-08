// Retention configuration and defaults.
package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/jonaskahn/relo/internal/activity"
)

const (
	defaultRetentionInterval = time.Hour
	// retentionBatch bounds one delete pass, so a large trim never holds the
	// write lock long enough to starve a request.
	retentionBatch = 1000
	// reclaimBytes is the free space worth a rewrite: below it a VACUUM
	// costs more than it returns.
	reclaimBytes = 1 << 20
	// DefaultUsageDays is how long request details are kept when an operator
	// never set a budget: a few days of detail survive long enough to inspect,
	// while the archive keeps every older total.
	DefaultUsageDays = 3
)

// DefaultRetentionConfig returns the budget an install that never stored one
// is governed by.
func DefaultRetentionConfig() RetentionConfig {
	return RetentionConfig{UsageDays: DefaultUsageDays}
}

var (
	// ErrNotDrained reports a VACUUM that would run while Relo still has
	// work in flight.
	ErrNotDrained = errors.New("refusing to vacuum while requests are in flight")
)

const (
	selectRetentionSQL = `SELECT usage_days, max_events, max_bytes, updated_at
	FROM retention_config WHERE id = 1`
	upsertRetentionSQL = `INSERT INTO retention_config (id, usage_days, max_events, max_bytes, updated_at)
	VALUES (1, ?, ?, ?, ?)
	ON CONFLICT (id) DO UPDATE SET
		usage_days = excluded.usage_days, max_events = excluded.max_events,
		max_bytes = excluded.max_bytes, updated_at = excluded.updated_at`
	countEventsSQL = `SELECT count(*) FROM usage_events`
	oldestBatchSQL = `SELECT id, timestamp FROM usage_events ORDER BY timestamp, id LIMIT ?`
	// deleteBeforeSQL removes the oldest rows before one instant, one batch
	// at a time. Every age, count, and byte rule deletes through it, so no
	// rule can touch a row the floor protects or one whose day is still open.
	deleteBeforeSQL = `DELETE FROM usage_events WHERE id IN (
	SELECT id FROM usage_events WHERE timestamp < ? ORDER BY timestamp, id LIMIT ?
)`
	countBeforeSQL = `SELECT count(*) FROM usage_events WHERE timestamp < ?`
	// freeBytesSQL reads the space the database holds on its free list,
	// which is what a VACUUM returns to the OS.
	freeBytesSQL = `SELECT
	(SELECT freelist_count FROM pragma_freelist_count) * (SELECT page_size FROM pragma_page_size)`
)

// RetentionConfig is the usage-log budget. A zero field leaves that budget
// unlimited; an install that never stored a budget is governed by
// DefaultRetentionConfig instead.
type RetentionConfig struct {
	UsageDays   int
	MaxEvents   int64
	MaxBytes    int64
	UpdatedAtMs int64
}

// RetentionReport is what one retention run did, or what a preview would do.
type RetentionReport struct {
	Config              RetentionConfig
	AgeCutoffMs         int64
	RowsBefore          int64
	RowsDeleted         int64
	LiveBytesBefore     int64
	LiveBytesAfter      int64
	EstimatedBytesFreed int64
}

// DrainProbe reports whether Relo has finished the work a VACUUM would
// interrupt. A nil probe means nothing is in flight.
type DrainProbe interface {
	Drained() bool
}

// RetentionOptions tunes the retention store.
type RetentionOptions struct {
	Logger   *slog.Logger
	Drain    DrainProbe
	Now      func() time.Time
	PageSize int64
}

// Retention enforces the usage-log budget and compacts the database once
// the deletions are done. Retention runs on the WAL checkpoint schedule the
// daemon starts it with.
type Retention struct {
	db      *DB
	opts    RetentionOptions
	archive *Archive
}

// NewRetention returns a retention store over the given database.
func NewRetention(db *DB, options RetentionOptions) *Retention {
	settings := options
	if settings.Logger == nil {
		settings.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	if settings.Now == nil {
		settings.Now = time.Now
	}
	archive := NewArchive(db)
	archive.now = settings.Now
	return &Retention{db: db, opts: settings, archive: archive}
}

// Config returns the stored budget, and whether an operator ever set one.
func (r *Retention) Config(ctx context.Context) (RetentionConfig, bool, error) {
	var config RetentionConfig
	err := r.db.sql.QueryRowContext(ctx, selectRetentionSQL).
		Scan(&config.UsageDays, &config.MaxEvents, &config.MaxBytes, &config.UpdatedAtMs)
	if errors.Is(err, sql.ErrNoRows) {
		return DefaultRetentionConfig(), false, nil
	}
	if err != nil {
		return RetentionConfig{}, false, fmt.Errorf("read retention config: %w", err)
	}
	return config, true, nil
}

// Budget returns the stored budget in the activity vocabulary.
func (r *Retention) Budget(ctx context.Context) (activity.RetentionBudget, bool, error) {
	config, stored, err := r.Config(ctx)
	if err != nil {
		return activity.RetentionBudget{}, false, err
	}
	return activity.RetentionBudget{
		UsageDays: config.UsageDays, MaxEvents: config.MaxEvents,
		MaxBytes: config.MaxBytes, UpdatedAtMs: config.UpdatedAtMs,
	}, stored, nil
}

// SaveConfig stores the budget, refusing one the floor cannot honor before
// the write.
func (r *Retention) SaveConfig(ctx context.Context, config RetentionConfig) error {
	if err := config.Validate(); err != nil {
		return err
	}
	at := config.UpdatedAtMs
	if at == 0 {
		at = r.opts.Now().UnixMilli()
	}
	_, err := r.db.sql.ExecContext(ctx, upsertRetentionSQL,
		config.UsageDays, config.MaxEvents, config.MaxBytes, at)
	if err != nil {
		return fmt.Errorf("store retention config: %w", err)
	}
	return nil
}

// Validate reports a budget the retention rules cannot honor. The ledger
// keeps a floor of detail either way: 0 keeps everything, any other budget
// has to leave at least the floor.
func (c RetentionConfig) Validate() error {
	if c.UsageDays != 0 && c.UsageDays < activity.MinUsageDays {
		return fmt.Errorf("usage_days: %w (keep all with 0, or at least a %d-day window)", activity.ErrInvalidRetention, activity.MinUsageDays)
	}
	if c.MaxEvents < 0 {
		return fmt.Errorf("max_events: %w", activity.ErrInvalidRetention)
	}
	if c.MaxBytes < 0 {
		return fmt.Errorf("max_bytes: %w", activity.ErrInvalidRetention)
	}
	return nil
}

// Preview reports what a retention run would delete, without deleting
// anything.
func (r *Retention) Preview(ctx context.Context, config RetentionConfig) (RetentionReport, error) {
	report, err := r.inspect(ctx, config)
	if err != nil {
		return RetentionReport{}, err
	}
	report.EstimatedBytesFreed = r.estimateFreed(report)
	return report, nil
}

// Enforce applies the budget, oldest rows first, without compacting. Every
// day before today is closed first, and no rule deletes a row inside the
// floor, so a total never loses the rows it was computed from. A pass that
// is already running reports ErrRetentionBusy instead of waiting.
func (r *Retention) Enforce(ctx context.Context, config RetentionConfig) (RetentionReport, error) {
	if err := config.Validate(); err != nil {
		return RetentionReport{}, err
	}
	if !r.db.maintenance.TryLock() {
		return RetentionReport{}, activity.ErrRetentionBusy
	}
	defer r.db.maintenance.Unlock()
	report, _, err := r.enforceLocked(ctx, config)
	return report, err
}

func (r *Retention) enforceLocked(ctx context.Context, config RetentionConfig) (RetentionReport, int, error) {
	finalized, err := r.finalizeAll(ctx)
	if err != nil {
		return RetentionReport{}, 0, err
	}
	report := RetentionReport{AgeCutoffMs: r.effectiveCutoff(config)}
	if err := r.deleteOldestFirst(ctx, config, &report); err != nil {
		return RetentionReport{}, 0, err
	}
	return report, finalized, nil
}

// RunOnce applies the stored budget now. A pass that is already running
// reports ErrRetentionBusy instead of waiting, which is what the maintenance
// tick and a console save both want. It deletes rows only: returning the
// freed space is the reclaim pass the tick runs after it.
func (r *Retention) RunOnce(ctx context.Context) (RetentionReport, error) {
	if !r.db.maintenance.TryLock() {
		return RetentionReport{}, activity.ErrRetentionBusy
	}
	defer r.db.maintenance.Unlock()
	config, err := r.storedConfig(ctx)
	if err != nil {
		return RetentionReport{}, err
	}
	report, _, err := r.enforceLocked(ctx, config)
	if err != nil {
		return report, err
	}
	return report, nil
}

// Sweep is the maintenance action the console waits on: it closes every open
// day, applies the stored budget, purges every captured body, and compacts
// the file once the data plane is idle. It holds the maintenance lock for the
// whole pass, so no tick, save, or second sweep runs beside it.
func (r *Retention) Sweep(ctx context.Context) (activity.CleanupReport, error) {
	if !r.db.maintenance.TryLock() {
		return activity.CleanupReport{}, activity.ErrRetentionBusy
	}
	defer r.db.maintenance.Unlock()
	return r.sweepLocked(ctx)
}

func (r *Retention) sweepLocked(ctx context.Context) (activity.CleanupReport, error) {
	report := activity.CleanupReport{}
	var err error
	if report.LiveBytesBefore, err = r.LiveBytes(ctx); err != nil {
		return report, err
	}
	// Every captured body goes in this pass, whether the budget takes its
	// request row or the purge does, so the count reads what was stored.
	if report.CapturesDeleted, err = r.captureCount(ctx); err != nil {
		return report, err
	}
	config, err := r.storedConfig(ctx)
	if err != nil {
		return report, err
	}
	if err := r.enforceAndPurge(ctx, config, &report); err != nil {
		return report, err
	}
	if report.LiveBytesAfter, err = r.LiveBytes(ctx); err != nil {
		return report, err
	}
	return report, nil
}

func (r *Retention) enforceAndPurge(ctx context.Context, config RetentionConfig, report *activity.CleanupReport) error {
	enforced, finalized, err := r.enforceLocked(ctx, config)
	if err != nil {
		return err
	}
	report.FinalizedDays = finalized
	report.RowsDeleted = enforced.RowsDeleted
	if err := r.purgeCaptures(ctx); err != nil {
		return err
	}
	if err := r.Vacuum(ctx); err != nil {
		if !errors.Is(err, ErrNotDrained) {
			return err
		}
	} else {
		report.Vacuumed = true
	}
	return nil
}

func (r *Retention) storedConfig(ctx context.Context) (RetentionConfig, error) {
	config, stored, err := r.Config(ctx)
	if err != nil {
		return RetentionConfig{}, err
	}
	if !stored {
		config = DefaultRetentionConfig()
	}
	if config.UsageDays != 0 && config.UsageDays < activity.MinUsageDays {
		// A floor that moved above a stored budget leaves a window the ledger
		// can no longer honor. The run uses the floor and stores it, so the
		// console shows the window actually in force and no pass is refused.
		r.opts.Logger.Warn("stored usage window is below the floor",
			"stored_days", config.UsageDays, "floor_days", activity.MinUsageDays)
		config.UsageDays = activity.MinUsageDays
		if err := r.SaveConfig(ctx, config); err != nil {
			return RetentionConfig{}, err
		}
	}
	if err := config.Validate(); err != nil {
		return RetentionConfig{}, err
	}
	return config, nil
}

func (r *Retention) captureCount(ctx context.Context) (int64, error) {
	var stored int64
	if err := r.db.sql.QueryRowContext(ctx, countCapturesSQL).Scan(&stored); err != nil {
		return 0, fmt.Errorf("count request captures: %w", err)
	}
	return stored, nil
}

func (r *Retention) purgeCaptures(ctx context.Context) error {
	for {
		result, err := r.db.sql.ExecContext(ctx, purgeCapturesSQL, retentionBatch)
		if err != nil {
			return fmt.Errorf("purge request captures: %w", err)
		}
		deleted, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("count purged request captures: %w", err)
		}
		if deleted < int64(retentionBatch) {
			return nil
		}
	}
}

// MaintainOnce applies the stored budget now, which is how a process closes
// a backlog at startup instead of waiting for the first tick.
func (r *Retention) MaintainOnce(ctx context.Context) {
	r.maintain(ctx)
}

func (r *Retention) finalizeAll(ctx context.Context) (int, error) {
	closed := 0
	for {
		count, err := r.archive.Finalize(ctx)
		if err != nil {
			return closed, fmt.Errorf("close the days before deleting them: %w", err)
		}
		if count == 0 {
			return closed, nil
		}
		closed += count
	}
}

// Run enforces the stored budget on every tick until ctx is cancelled. Each
// tick is one maintenance pass: deletions, then a compaction of whatever
// space the log no longer needs.
func (r *Retention) Run(ctx context.Context, every time.Duration) error {
	if every <= 0 {
		every = defaultRetentionInterval
	}
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			r.maintain(ctx)
		}
	}
}

func (r *Retention) maintain(ctx context.Context) {
	report, err := r.RunOnce(ctx)
	switch {
	case errors.Is(err, activity.ErrRetentionBusy):
	case err != nil:
		r.opts.Logger.Warn("enforce retention", "error", err)
	case report.RowsDeleted > 0:
		r.opts.Logger.Info("retention pass", "rows_deleted", report.RowsDeleted)
	}
	// A pass that deleted nothing can still leave reclaimable space from an
	// earlier one, so every tick tries to return it.
	r.reclaim(ctx)
}

func (r *Retention) reclaim(ctx context.Context) {
	// A sweep or another pass holds the maintenance lock for its whole run;
	// a compaction must not work beside it.
	if !r.db.maintenance.TryLock() {
		return
	}
	defer r.db.maintenance.Unlock()
	free, err := r.freeBytes(ctx)
	if err != nil {
		r.opts.Logger.Warn("read the free pages", "error", err)
		return
	}
	if free < reclaimBytes {
		return
	}
	r.compact(ctx)
}

func (r *Retention) freeBytes(ctx context.Context) (int64, error) {
	var free int64
	if err := r.db.sql.QueryRowContext(ctx, freeBytesSQL).Scan(&free); err != nil {
		return 0, fmt.Errorf("read the free pages: %w", err)
	}
	return free, nil
}

func (r *Retention) compact(ctx context.Context) {
	switch err := r.Vacuum(ctx); {
	case err == nil:
		r.opts.Logger.Info("compacted the database")
	case errors.Is(err, ErrNotDrained):
		r.opts.Logger.Debug("compaction deferred", "reason", "requests in flight")
	default:
		r.opts.Logger.Warn("compact the database", "error", err)
	}
}

func (r *Retention) inspect(ctx context.Context, config RetentionConfig) (RetentionReport, error) {
	if err := config.Validate(); err != nil {
		return RetentionReport{}, err
	}
	report := RetentionReport{Config: config, AgeCutoffMs: r.effectiveCutoff(config)}
	rows, err := r.eventCount(ctx)
	if err != nil {
		return RetentionReport{}, err
	}
	report.RowsBefore = rows
	planned, err := r.plannedDeletions(ctx, config, report.AgeCutoffMs)
	if err != nil {
		return RetentionReport{}, err
	}
	report.RowsDeleted = planned
	if report.LiveBytesBefore, err = r.LiveBytes(ctx); err != nil {
		return RetentionReport{}, err
	}
	report.LiveBytesAfter = report.LiveBytesBefore
	return report, nil
}

func (r *Retention) plannedDeletions(ctx context.Context, config RetentionConfig, ageCutoffMs int64) (int64, error) {
	planned := int64(0)
	if ageCutoffMs > 0 {
		count, err := r.countWhere(ctx, `timestamp < ?`, ageCutoffMs)
		if err != nil {
			return 0, err
		}
		planned = count
	}
	if config.MaxEvents <= 0 {
		return planned, nil
	}
	eligible, err := r.countBefore(ctx, r.deletionFloorMs())
	if err != nil {
		return 0, err
	}
	if remaining := eligible - planned; remaining > config.MaxEvents {
		planned = eligible - config.MaxEvents
	}
	return planned, nil
}

func (r *Retention) estimateFreed(report RetentionReport) int64 {
	if report.RowsBefore == 0 || report.RowsDeleted == 0 {
		return 0
	}
	perRow := report.LiveBytesBefore / report.RowsBefore
	return perRow * report.RowsDeleted
}

func (r *Retention) deleteOldestFirst(ctx context.Context, config RetentionConfig, report *RetentionReport) error {
	before, err := r.eventCount(ctx)
	if err != nil {
		return err
	}
	report.RowsBefore = before
	if before == 0 {
		return r.finish(ctx, report)
	}
	if report.LiveBytesBefore, err = r.LiveBytes(ctx); err != nil {
		return err
	}
	if err := r.deleteExpired(ctx, config, report); err != nil {
		return err
	}
	if err := r.deleteSurplus(ctx, config, report); err != nil {
		return err
	}
	if err := r.deleteForBytes(ctx, config, report); err != nil {
		return err
	}
	return r.finish(ctx, report)
}

func (r *Retention) finish(ctx context.Context, report *RetentionReport) error {
	live, err := r.LiveBytes(ctx)
	if err != nil {
		return err
	}
	report.LiveBytesAfter = live
	return nil
}

func (r *Retention) deleteExpired(ctx context.Context, config RetentionConfig, report *RetentionReport) error {
	cutoff := r.effectiveCutoff(config)
	report.AgeCutoffMs = cutoff
	if cutoff == 0 {
		return nil
	}
	return r.deleteAllBefore(ctx, cutoff, report)
}

func (r *Retention) deleteSurplus(ctx context.Context, config RetentionConfig, report *RetentionReport) error {
	if config.MaxEvents <= 0 {
		return nil
	}
	floor := r.deletionFloorMs()
	rows, err := r.countBefore(ctx, floor)
	if err != nil {
		return err
	}
	if surplus := rows - config.MaxEvents; surplus > 0 {
		return r.deleteOldest(ctx, surplus, floor, report)
	}
	return nil
}

func (r *Retention) deleteForBytes(ctx context.Context, config RetentionConfig, report *RetentionReport) error {
	if config.MaxBytes <= 0 {
		return nil
	}
	live, err := r.LiveBytes(ctx)
	if err != nil {
		return err
	}
	floor := r.deletionFloorMs()
	for live > config.MaxBytes {
		deleted, err := r.deleteBatch(ctx, floor, retentionBatch, report)
		if err != nil {
			return err
		}
		if deleted == 0 {
			return nil
		}
		if live, err = r.LiveBytes(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (r *Retention) deleteAllBefore(ctx context.Context, beforeMs int64, report *RetentionReport) error {
	for {
		deleted, err := r.deleteBatch(ctx, beforeMs, retentionBatch, report)
		if err != nil {
			return err
		}
		if deleted < int64(retentionBatch) {
			return nil
		}
	}
}

func (r *Retention) deleteOldest(ctx context.Context, count int64, beforeMs int64, report *RetentionReport) error {
	remaining := count
	for remaining > 0 {
		batch := retentionBatch
		if remaining < int64(batch) {
			batch = int(remaining)
		}
		deleted, err := r.deleteBatch(ctx, beforeMs, batch, report)
		if err != nil {
			return err
		}
		if deleted == 0 {
			return nil
		}
		remaining -= deleted
	}
	return nil
}

func (r *Retention) deleteBatch(ctx context.Context, beforeMs int64, batch int, report *RetentionReport) (int64, error) {
	before := report.RowsDeleted
	if err := r.deleteWhere(ctx, deleteBeforeSQL, report, beforeMs, batch); err != nil {
		return 0, err
	}
	return report.RowsDeleted - before, nil
}

func (r *Retention) deleteWhere(ctx context.Context, statement string, report *RetentionReport, args ...any) error {
	result, err := r.db.sql.ExecContext(ctx, statement, args...)
	if err != nil {
		return fmt.Errorf("delete usage events: %w", err)
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count deleted usage events: %w", err)
	}
	report.RowsDeleted += deleted
	return nil
}

// LiveBytes returns the logical size of the database: the pages that hold
// data, not the pages sitting on the freelist or the file on disk.
func (r *Retention) LiveBytes(ctx context.Context) (int64, error) {
	pages, err := r.pageCount(ctx, "page_count")
	if err != nil {
		return 0, err
	}
	free, err := r.pageCount(ctx, "freelist_count")
	if err != nil {
		return 0, err
	}
	size, err := r.pageSize(ctx)
	if err != nil {
		return 0, err
	}
	if pages < free {
		free = pages
	}
	return (pages - free) * size, nil
}

// Vacuum compacts the file, and refuses to run while Relo still has work in
// flight: a VACUUM needs the write lock, and taking it mid-request is what
// makes a maintenance pass look like an outage.
func (r *Retention) Vacuum(ctx context.Context) error {
	if r.opts.Drain != nil && !r.opts.Drain.Drained() {
		return ErrNotDrained
	}
	if _, err := r.db.sql.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		return fmt.Errorf("checkpoint before vacuum: %w", err)
	}
	if _, err := r.db.sql.ExecContext(ctx, "VACUUM"); err != nil {
		return fmt.Errorf("vacuum the database: %w", err)
	}
	// The rewrite writes the compacted pages through the log; truncating it
	// now returns that file space too.
	if _, err := r.db.sql.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		return fmt.Errorf("checkpoint after vacuum: %w", err)
	}
	return nil
}

func (r *Retention) cutoff(config RetentionConfig) int64 {
	if config.UsageDays <= 0 {
		return 0
	}
	kept := time.Duration(config.UsageDays-1) * 24 * time.Hour
	return startOfUTCDay(r.opts.Now()).Add(-kept).UnixMilli()
}

func (r *Retention) effectiveCutoff(config RetentionConfig) int64 {
	cutoff := r.cutoff(config)
	if cutoff == 0 {
		return 0
	}
	if floor := r.deletionFloorMs(); cutoff > floor {
		return floor
	}
	return cutoff
}

func (r *Retention) deletionFloorMs() int64 {
	kept := time.Duration(activity.MinUsageDays-1) * 24 * time.Hour
	return startOfUTCDay(r.opts.Now()).Add(-kept).UnixMilli()
}

func startOfUTCDay(at time.Time) time.Time {
	utc := at.UTC()
	return time.Date(utc.Year(), utc.Month(), utc.Day(), 0, 0, 0, 0, time.UTC)
}

func (r *Retention) eventCount(ctx context.Context) (int64, error) {
	var count int64
	if err := r.db.sql.QueryRowContext(ctx, countEventsSQL).Scan(&count); err != nil {
		return 0, fmt.Errorf("count usage events: %w", err)
	}
	return count, nil
}

func (r *Retention) countBefore(ctx context.Context, beforeMs int64) (int64, error) {
	var count int64
	if err := r.db.sql.QueryRowContext(ctx, countBeforeSQL, beforeMs).Scan(&count); err != nil {
		return 0, fmt.Errorf("count usage events: %w", err)
	}
	return count, nil
}

func (r *Retention) countWhere(ctx context.Context, clause string, args ...any) (int64, error) {
	var count int64
	statement := "SELECT count(*) FROM usage_events WHERE " + clause
	if err := r.db.sql.QueryRowContext(ctx, statement, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("count usage events: %w", err)
	}
	return count, nil
}

func (r *Retention) pageCount(ctx context.Context, pragma string) (int64, error) {
	var count int64
	if err := r.db.sql.QueryRowContext(ctx, "PRAGMA "+pragma).Scan(&count); err != nil {
		return 0, fmt.Errorf("read %s: %w", pragma, err)
	}
	return count, nil
}

func (r *Retention) pageSize(ctx context.Context) (int64, error) {
	if r.opts.PageSize > 0 {
		return r.opts.PageSize, nil
	}
	size, err := r.pageCount(ctx, "page_size")
	if err != nil {
		return 0, err
	}
	return size, nil
}
