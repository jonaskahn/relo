// Retention settings service: budget, preview, and one run.
package sqlite

import (
	"context"

	"github.com/jonaskahn/relo/internal/activity"
)

// RetentionSettings adapts the retention store to the settings contract, so a
// settings use case never touches SQL.
type RetentionSettings struct {
	store *Retention
}

// NewRetentionSettings returns a retention settings store over the database.
func NewRetentionSettings(db *DB, options RetentionOptions) *RetentionSettings {
	return &RetentionSettings{store: NewRetention(db, options)}
}

// Budget returns the stored budget, and whether an operator ever set one.
func (s *RetentionSettings) Budget(ctx context.Context) (activity.RetentionBudget, bool, error) {
	stored, found, err := s.store.Config(ctx)
	if err != nil {
		return activity.RetentionBudget{}, false, err
	}
	return budgetOf(stored), found, nil
}

// SaveBudget stores one usage-log budget.
func (s *RetentionSettings) SaveBudget(ctx context.Context, budget activity.RetentionBudget) error {
	return s.store.SaveConfig(ctx, configOf(budget))
}

// Preview reports what one budget would delete, without changing a row.
func (s *RetentionSettings) Preview(ctx context.Context, budget activity.RetentionBudget) (activity.RetentionReport, error) {
	report, err := s.store.Preview(ctx, configOf(budget))
	if err != nil {
		return activity.RetentionReport{}, err
	}
	return reportOf(report), nil
}

// RunOnce applies the stored budget now, so a save or a console button
// cleans up without waiting for the maintenance tick.
func (s *RetentionSettings) RunOnce(ctx context.Context) (activity.RetentionReport, error) {
	report, err := s.store.RunOnce(ctx)
	if err != nil {
		return activity.RetentionReport{}, err
	}
	return reportOf(report), nil
}

// Sweep runs the maintenance pass the console waits on: archive, clean,
// purge the captured bodies, and compact.
func (s *RetentionSettings) Sweep(ctx context.Context) (activity.CleanupReport, error) {
	report, err := s.store.Sweep(ctx)
	if err != nil {
		return activity.CleanupReport{}, err
	}
	return report, nil
}

func budgetOf(stored RetentionConfig) activity.RetentionBudget {
	return activity.RetentionBudget{
		UsageDays: stored.UsageDays, MaxEvents: stored.MaxEvents,
		MaxBytes: stored.MaxBytes, UpdatedAtMs: stored.UpdatedAtMs,
	}
}

func configOf(budget activity.RetentionBudget) RetentionConfig {
	return RetentionConfig{
		UsageDays: budget.UsageDays, MaxEvents: budget.MaxEvents,
		MaxBytes: budget.MaxBytes, UpdatedAtMs: budget.UpdatedAtMs,
	}
}

func reportOf(report RetentionReport) activity.RetentionReport {
	return activity.RetentionReport{
		Config: budgetOf(report.Config), AgeCutoffMs: report.AgeCutoffMs,
		RowsBefore: report.RowsBefore, RowsDeleted: report.RowsDeleted,
		LiveBytesBefore: report.LiveBytesBefore, LiveBytesAfter: report.LiveBytesAfter,
		EstimatedBytesFreed: report.EstimatedBytesFreed,
	}
}
