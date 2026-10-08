package storage_test

import (
	"context"
	"testing"

	"github.com/jonaskahn/relo/internal/activity"
	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	"github.com/jonaskahn/relo/internal/config"
	"github.com/jonaskahn/relo/internal/inference"
	"github.com/jonaskahn/relo/tests/testkit"
)

// schemaMigrationsDDL creates the bookkeeping table a database written by an
// earlier build carries, so the baseline migration can be applied alone.
const schemaMigrationsDDL = "CREATE TABLE schema_migrations (" +
	"version INTEGER NOT NULL PRIMARY KEY, applied_at INTEGER NOT NULL, checksum TEXT NOT NULL) STRICT"

// TestUsageOriginUpgrade covers a state database written before a request
// carried its origin: the baseline still opens, every stored row reads as
// external traffic, and the filter tells the two origins apart.
func TestUsageOriginUpgrade(t *testing.T) {
	logger, _ := testkit.TestLogger(t)
	db, err := sqlite.OpenDB(config.DatabasePath(testkit.TempHome(t)), logger)
	if err != nil {
		t.Fatalf("OpenDB() error = %v", err)
	}
	defer func() { _ = db.Close() }()
	migrations, err := sqlite.LoadMigrations()
	if err != nil {
		t.Fatalf("LoadMigrations() error = %v", err)
	}
	if _, err := db.SQL().Exec(schemaMigrationsDDL); err != nil {
		t.Fatalf("create the migration table: %v", err)
	}
	if err := sqlite.ApplyMigration(db, migrations[0]); err != nil {
		t.Fatalf("ApplyMigration(baseline) error = %v", err)
	}
	ctx := context.Background()
	if _, err := db.SQL().ExecContext(ctx,
		"INSERT INTO usage_events (request_id, timestamp, provider, model, status, duration_ms) VALUES (?, ?, ?, ?, ?, ?)",
		"before", 1000, "openai", "gpt-4o", 200, 5); err != nil {
		t.Fatalf("seed a request in the old schema: %v", err)
	}
	if err := sqlite.Migrate(db); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	var origin string
	if err := db.SQL().QueryRowContext(ctx,
		"SELECT origin FROM usage_events WHERE request_id = 'before'").Scan(&origin); err != nil {
		t.Fatalf("read the upgraded origin: %v", err)
	}
	if origin != inference.OriginExternal {
		t.Fatalf("origin = %q, want the stored row to read as external traffic", origin)
	}

	recorder := newRecorder(t, db)
	if _, err := recorder.AppendEvent(ctx, sqlite.UsageEvent{
		RequestID: "after", Timestamp: 2000, Provider: "openai", Model: "gpt-4o",
		Status: 200, Origin: inference.OriginInternal,
	}); err != nil {
		t.Fatalf("AppendEvent(after) error = %v", err)
	}
	page, err := sqlite.NewUsageQuery(db).Logs(ctx, activity.LogQuery{
		Filter: activity.UsageFilter{Origin: inference.OriginInternal},
	})
	if err != nil {
		t.Fatalf("Logs() error = %v", err)
	}
	if len(page.Rows) != 1 || page.Rows[0].RequestID != "after" || page.Rows[0].Origin != inference.OriginInternal {
		t.Fatalf("rows = %+v, want the one internal request", page.Rows)
	}
}
