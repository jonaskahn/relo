package storage_test

import (
	"context"
	"testing"

	"github.com/jonaskahn/relo/internal/activity"
	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	"github.com/jonaskahn/relo/internal/config"
	"github.com/jonaskahn/relo/tests/testkit"
)

// TestUsageClientAppUpgrade covers a state database written before a request
// stored the coding client it was made with: the baseline still opens, the
// stored row copies the client from the key it named, and a filter finds it
// after that key is gone.
func TestUsageClientAppUpgrade(t *testing.T) {
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
		"INSERT INTO access_keys (id, name, kind, client, token_digest, token_hint, generation, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
		"key-codex", "codex", "agent", "codex", "digest-key-codex", "abcd...yz", 1, 1_000, 1_000); err != nil {
		t.Fatalf("seed the key the old request named: %v", err)
	}
	if _, err := db.SQL().ExecContext(ctx,
		"INSERT INTO usage_events (request_id, timestamp, provider, model, status, duration_ms, client_key_id, client_key_name) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		"before", 1000, "openai", "gpt-4o", 200, 5, "key-codex", "codex"); err != nil {
		t.Fatalf("seed a request in the old schema: %v", err)
	}
	if err := sqlite.Migrate(db); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	var clientApp string
	if err := db.SQL().QueryRowContext(ctx,
		"SELECT client_app FROM usage_events WHERE request_id = 'before'").Scan(&clientApp); err != nil {
		t.Fatalf("read the upgraded client: %v", err)
	}
	if clientApp != "codex" {
		t.Fatalf("client_app = %q, want the client the key was issued for", clientApp)
	}

	if _, err := db.SQL().ExecContext(ctx, "DELETE FROM access_keys WHERE id = ?", "key-codex"); err != nil {
		t.Fatalf("delete the key the request named: %v", err)
	}
	page, err := sqlite.NewUsageQuery(db).Logs(ctx, activity.LogQuery{
		Filter: activity.UsageFilter{AccessClient: "codex"},
	})
	if err != nil {
		t.Fatalf("Logs() error = %v", err)
	}
	if len(page.Rows) != 1 || page.Rows[0].RequestID != "before" || page.Rows[0].ClientKeyName != "codex" {
		t.Fatalf("rows = %+v, want the request after its key is gone", page.Rows)
	}
}
