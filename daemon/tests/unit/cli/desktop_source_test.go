package cli_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/activity"
	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	"github.com/jonaskahn/relo/internal/cli"
	"github.com/jonaskahn/relo/internal/config"
)

// TestDesktopActivitySnapshotReadsTheStateDatabase covers the tray's reader:
// one stored request folds into the paid provider's row, and a plan-covered
// connection reads as unpaid.
func TestDesktopActivitySnapshotReadsTheStateDatabase(t *testing.T) {
	home := t.TempDir()
	logger := slog.New(slog.DiscardHandler)
	db, err := sqlite.OpenDB(config.DatabasePath(home), logger)
	if err != nil {
		t.Fatalf("open state database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := sqlite.Migrate(db); err != nil {
		t.Fatalf("migrate state database: %v", err)
	}
	seedDesktopProvider(t, db, "openai", "template")
	seedDesktopProvider(t, db, "opencode-go", "template")
	recorder, err := sqlite.NewUsageRecorder(db, logger)
	if err != nil {
		t.Fatalf("NewUsageRecorder() error = %v", err)
	}
	t.Cleanup(func() { _ = recorder.Close() })
	now := time.Now()
	if _, err := recorder.AppendEvent(context.Background(), activity.RequestEvent{
		RequestID: "req-1", Timestamp: now.UnixMilli(),
		Provider: "openai", Model: "gpt-4o", RequestedModel: "openai/gpt-4o",
		Status: 200, InputTokens: 10, OutputTokens: 5,
	}); err != nil {
		t.Fatalf("AppendEvent() error = %v", err)
	}

	snapshot, err := cli.NewDesktopActivitySource(home, logger).Snapshot(
		context.Background(), now.Add(-time.Hour).UnixMilli())
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if len(snapshot.Rows) != 1 || snapshot.Rows[0].Key != "openai" {
		t.Fatalf("rows = %+v, want the one paid provider", snapshot.Rows)
	}
	if snapshot.Rows[0].Requests != 1 {
		t.Fatalf("requests = %v, want 1", snapshot.Rows[0].Requests)
	}
	if !snapshot.UnpaidIDs["opencode-go"] || snapshot.UnpaidIDs["openai"] {
		t.Fatalf("unpaid = %v, want only the plan-covered connection", snapshot.UnpaidIDs)
	}
}

// TestDesktopActivitySnapshotWithoutADatabase covers a state directory the
// tray has not reached yet: the read fails instead of answering an idle day.
func TestDesktopActivitySnapshotWithoutADatabase(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)
	source := cli.NewDesktopActivitySource(t.TempDir(), logger)
	if _, err := source.Snapshot(context.Background(), 0); err == nil {
		t.Fatal("Snapshot() = nil, want the missing database to fail the read")
	}
}

func seedDesktopProvider(t *testing.T, db *sqlite.DB, id, origin string) {
	t.Helper()
	if _, err := db.SQL().Exec(
		"INSERT INTO providers (id, template_id, origin, label, auth, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
		id, id, origin, id, "api_key", 0, 0,
	); err != nil {
		t.Fatalf("seed provider %s: %v", id, err)
	}
}
