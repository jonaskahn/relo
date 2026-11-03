package platform

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/activity"
	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	"github.com/jonaskahn/relo/internal/config"
	"github.com/jonaskahn/relo/internal/server"
)

func trafficDay(t *testing.T, dir, day string, size int) string {
	t.Helper()
	path := filepath.Join(dir, trafficLogPrefix+day+trafficLogSuffix)
	if err := os.WriteFile(path, make([]byte, size), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

func newBudgetedTrafficLog(home string, now time.Time, budget activity.RetentionBudget) *trafficLog {
	return &trafficLog{
		dir: logDir(home), now: func() time.Time { return now },
		usageDays: budget.UsageDays, maxBytes: budget.MaxBytes,
	}
}

func TestTrafficLogUsageDaysRemovesWholeDaysAndKeepsToday(t *testing.T) {
	home := t.TempDir()
	now := time.Date(2026, 11, 11, 9, 0, 0, 0, time.UTC)
	dir := logDir(home)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	traffic := newBudgetedTrafficLog(home, now, activity.RetentionBudget{UsageDays: 3})
	old := trafficDay(t, dir, "20261108", 10)
	kept := trafficDay(t, dir, "20261109", 10)
	trafficDay(t, dir, "20261111", 10)

	if err := traffic.AppendHalf(server.TrafficHalf{RequestID: "req-1", Kind: activity.CaptureAgentRequest}); err != nil {
		t.Fatalf("AppendHalf() error = %v", err)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatalf("the file outside the window survived: %v", err)
	}
	if _, err := os.Stat(kept); err != nil {
		t.Fatalf("the file inside the window was removed: %v", err)
	}
}

func TestTrafficLogUsageDaysZeroKeepsEveryDay(t *testing.T) {
	home := t.TempDir()
	now := time.Date(2026, 11, 11, 9, 0, 0, 0, time.UTC)
	dir := logDir(home)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	traffic := newBudgetedTrafficLog(home, now, activity.RetentionBudget{})
	old := trafficDay(t, dir, "20260101", 10)

	if err := traffic.AppendHalf(server.TrafficHalf{RequestID: "req-1", Kind: activity.CaptureAgentRequest}); err != nil {
		t.Fatalf("AppendHalf() error = %v", err)
	}
	if _, err := os.Stat(old); err != nil {
		t.Fatalf("a zero-day budget removed an old file: %v", err)
	}
}

func TestTrafficLogMaxBytesRemovesOldestClosedFiles(t *testing.T) {
	home := t.TempDir()
	now := time.Date(2026, 11, 11, 9, 0, 0, 0, time.UTC)
	dir := logDir(home)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	traffic := newBudgetedTrafficLog(home, now, activity.RetentionBudget{MaxBytes: 200})
	oldest := trafficDay(t, dir, "20261108", 1000)
	newer := trafficDay(t, dir, "20261109", 10)
	today := trafficDay(t, dir, "20261111", 10)

	if err := traffic.AppendHalf(server.TrafficHalf{RequestID: "req-1", Kind: activity.CaptureAgentRequest}); err != nil {
		t.Fatalf("AppendHalf() error = %v", err)
	}
	if _, err := os.Stat(oldest); !os.IsNotExist(err) {
		t.Fatalf("the oldest closed file survived: %v", err)
	}
	for _, path := range []string{newer, today} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("a file under the budget was removed: %v", err)
		}
	}
}

func TestTrafficLogNeverRemovesTodayForTheByteBudget(t *testing.T) {
	home := t.TempDir()
	now := time.Date(2026, 11, 11, 9, 0, 0, 0, time.UTC)
	dir := logDir(home)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	traffic := newBudgetedTrafficLog(home, now, activity.RetentionBudget{MaxBytes: 1})
	today := trafficDay(t, dir, "20261111", 50)

	if err := traffic.AppendHalf(server.TrafficHalf{RequestID: "req-1", Kind: activity.CaptureAgentRequest}); err != nil {
		t.Fatalf("AppendHalf() error = %v", err)
	}
	if _, err := os.Stat(today); err != nil {
		t.Fatalf("today's file was removed: %v", err)
	}
}

func TestTrafficLogSetBudgetPrunesImmediately(t *testing.T) {
	home := t.TempDir()
	now := time.Date(2026, 11, 11, 9, 0, 0, 0, time.UTC)
	dir := logDir(home)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	traffic := newBudgetedTrafficLog(home, now, activity.RetentionBudget{})
	old := trafficDay(t, dir, "20260101", 10)
	if err := traffic.SetBudget(activity.RetentionBudget{MaxBytes: 1}); err != nil {
		t.Fatalf("SetBudget() error = %v", err)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatalf("the old file survived a saved budget: %v", err)
	}
}

func TestTrafficLogStatsReportTodayAndTheFolder(t *testing.T) {
	home := t.TempDir()
	now := time.Date(2026, 11, 11, 9, 0, 0, 0, time.UTC)
	dir := logDir(home)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	traffic := newBudgetedTrafficLog(home, now, activity.RetentionBudget{})
	trafficDay(t, dir, "20261109", 10)
	trafficDay(t, dir, "20261111", 30)

	stats, err := traffic.Stats()
	if err != nil {
		t.Fatalf("Stats() error = %v", err)
	}
	if stats.TodayBytes != 30 || stats.TotalBytes != 40 {
		t.Fatalf("Stats() = %+v, want 30 today and 40 retained", stats)
	}
}

func TestTrafficLogOnReadsTheSwitch(t *testing.T) {
	home := t.TempDir()
	path := config.ConfigPath(home)
	traffic := newBudgetedTrafficLog(home, time.Now(), activity.RetentionBudget{})
	traffic.configPath = path
	if traffic.On() {
		t.Fatal("On() = true, want off without a stored switch")
	}
	if err := os.WriteFile(path, []byte("[system.logging]\ntraffic = true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !traffic.On() {
		t.Fatal("On() = false, want the stored switch")
	}
}

func TestRefreshTrafficBudgetAppliesTheStoredSettings(t *testing.T) {
	home := t.TempDir()
	db, err := sqlite.OpenDB(filepath.Join(home, "relo.db"), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("OpenDB() error = %v", err)
	}
	defer func() { _ = db.Close() }()
	if err := sqlite.Migrate(db); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	now := time.Date(2026, 11, 11, 9, 0, 0, 0, time.UTC)
	retention := sqlite.NewRetention(db, sqlite.RetentionOptions{Now: func() time.Time { return now }})
	if err := retention.SaveConfig(context.Background(), sqlite.RetentionConfig{UsageDays: 3, MaxBytes: 1}); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}
	dir := logDir(home)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	traffic := newBudgetedTrafficLog(home, now, activity.RetentionBudget{})
	old := trafficDay(t, dir, "20260101", 10)

	refreshTrafficBudget(context.Background(), deps{
		traffic: traffic, logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}, retention)

	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatalf("the old file survived the stored budget: %v", err)
	}
	if budget := traffic.Budget(); budget.UsageDays != 3 || budget.MaxBytes != 1 {
		t.Fatalf("Budget() = %+v, want the stored settings", budget)
	}
}
