package storage_test

import (
	"context"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	"github.com/jonaskahn/relo/internal/config"
	"github.com/jonaskahn/relo/tests/testkit"
)

// TestAntigravitySunsetTurnsOnlyThatConnectionOff covers the ban Google put on
// its own Cloud Code Assist service: an install that already carries the
// connection starts with it off, while the row, its accounts and its models
// stay so an operator can still switch it back on.
func TestAntigravitySunsetTurnsOnlyThatConnectionOff(t *testing.T) {
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
	if _, err := db.SQL().Exec("CREATE TABLE schema_migrations (" +
		"version INTEGER NOT NULL PRIMARY KEY, applied_at INTEGER NOT NULL, checksum TEXT NOT NULL) STRICT"); err != nil {
		t.Fatalf("create the migration table: %v", err)
	}
	// Everything before the sunset, which is where this migration begins.
	// Later migrations are unrelated and must not become the one under test.
	const sunset = 41
	var disable *sqlite.Migration
	for i := range migrations {
		migration := &migrations[i]
		if migration.Version > sunset {
			continue
		}
		if migration.Version == sunset {
			disable = migration
			continue
		}
		if err := sqlite.ApplyMigration(db, *migration); err != nil {
			t.Fatalf("ApplyMigration(%d) error = %v", migration.Version, err)
		}
	}
	if disable == nil {
		t.Fatal("the Antigravity sunset migration is missing")
	}
	ctx := context.Background()
	rows := []struct{ id, templateID string }{
		{"google-antigravity", "google-antigravity"},
		{"claude", "claude"},
	}
	for _, row := range rows {
		if _, err := db.SQL().ExecContext(ctx,
			`INSERT INTO providers (id, template_id, origin, label, auth, api_format, key_header, base_url, models_format, enabled, rank, pool_strategy, created_at, updated_at)`+
				` VALUES (?, ?, 'signin', ?, 'oauth', 'cloud-code-assist', 'bearer', 'https://example.test', 'antigravity', 1, 100, 'least-loaded', 1, 1)`,
			row.id, row.templateID, row.id); err != nil {
			t.Fatalf("seed %s: %v", row.id, err)
		}
		if _, err := db.SQL().ExecContext(ctx,
			`INSERT INTO credentials (id, provider_id, kind, label, status, priority, generation, created_at, updated_at)`+
				` VALUES (?, ?, 'oauth', 'kept', 'active', 0, 0, 1, 1)`,
			row.id+"-account", row.id); err != nil {
			t.Fatalf("seed the %s account: %v", row.id, err)
		}
		if _, err := db.SQL().ExecContext(ctx,
			`INSERT INTO models (provider_id, model_id, source, api_format, base_url, match, enabled, updated_at)`+
				` VALUES (?, 'kept-model', 'listing', 'cloud-code-assist', 'https://example.test', 'none', 1, 1)`,
			row.id); err != nil {
			t.Fatalf("seed a %s model: %v", row.id, err)
		}
	}

	if err := sqlite.ApplyMigration(db, *disable); err != nil {
		t.Fatalf("ApplyMigration(%s) error = %v", disable.Name, err)
	}

	for _, row := range rows {
		want := 0
		if row.templateID != "google-antigravity" {
			want = 1
		}
		var enabled int
		if err := db.SQL().QueryRowContext(ctx,
			"SELECT enabled FROM providers WHERE id = ?", row.id).Scan(&enabled); err != nil {
			t.Fatalf("read %s: %v", row.id, err)
		}
		if enabled != want {
			t.Errorf("%s enabled = %d, want %d", row.id, enabled, want)
		}
		var accounts int
		if err := db.SQL().QueryRowContext(ctx,
			"SELECT COUNT(*) FROM credentials WHERE provider_id = ? AND deleted_at IS NULL", row.id).Scan(&accounts); err != nil {
			t.Fatalf("count the %s accounts: %v", row.id, err)
		}
		if accounts != 1 {
			t.Errorf("%s kept %d accounts, want the stored one", row.id, accounts)
		}
		var models int
		if err := db.SQL().QueryRowContext(ctx,
			"SELECT COUNT(*) FROM models WHERE provider_id = ?", row.id).Scan(&models); err != nil {
			t.Fatalf("count the %s models: %v", row.id, err)
		}
		if models != 1 {
			t.Errorf("%s kept %d models, want the listed one", row.id, models)
		}
	}
}
