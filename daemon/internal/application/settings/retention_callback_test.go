package settings

import (
	"context"
	"errors"
	"testing"

	"github.com/jonaskahn/relo/internal/activity"
)

type recordingRetention struct {
	saved activity.RetentionBudget
	err   error
}

func (r *recordingRetention) Budget(context.Context) (activity.RetentionBudget, bool, error) {
	return activity.RetentionBudget{}, false, nil
}

func (r *recordingRetention) SaveBudget(_ context.Context, budget activity.RetentionBudget) error {
	if r.err != nil {
		return r.err
	}
	r.saved = budget
	return nil
}

func (r *recordingRetention) Preview(context.Context, activity.RetentionBudget) (activity.RetentionReport, error) {
	return activity.RetentionReport{}, nil
}

func (r *recordingRetention) RunOnce(context.Context) (activity.RetentionReport, error) {
	return activity.RetentionReport{}, nil
}

func (r *recordingRetention) Sweep(context.Context) (activity.CleanupReport, error) {
	return activity.CleanupReport{}, nil
}

func TestSaveRetentionAppliesTheSavedBudget(t *testing.T) {
	store := &recordingRetention{}
	var applied *activity.RetentionBudget
	service := New(Options{
		Retention: store, ConfigPath: t.TempDir() + "/config.toml",
		OnRetentionSaved: func(budget activity.RetentionBudget) { applied = &budget },
	})
	budget := RetentionSettings{UsageDays: 3, MaxEvents: 10, MaxBytes: 1 << 20}
	if _, err := service.SaveRetention(context.Background(), budget); err != nil {
		t.Fatalf("SaveRetention() error = %v", err)
	}
	if applied == nil || applied.UsageDays != 3 || applied.MaxBytes != 1<<20 {
		t.Fatalf("OnRetentionSaved() = %+v, want the saved budget", applied)
	}
	if store.saved.UsageDays != 3 || store.saved.MaxEvents != 10 {
		t.Fatalf("stored budget = %+v, want the saved values", store.saved)
	}
}

func TestSaveRetentionLeavesTheTrafficBudgetAloneOnFailure(t *testing.T) {
	store := &recordingRetention{err: errors.New("store down")}
	called := false
	service := New(Options{
		Retention: store, ConfigPath: t.TempDir() + "/config.toml",
		OnRetentionSaved: func(activity.RetentionBudget) { called = true },
	})
	if _, err := service.SaveRetention(context.Background(), RetentionSettings{UsageDays: 3}); err == nil {
		t.Fatal("SaveRetention() = nil, want the store failure")
	}
	if called {
		t.Fatal("OnRetentionSaved() ran for a budget the store refused")
	}
}
