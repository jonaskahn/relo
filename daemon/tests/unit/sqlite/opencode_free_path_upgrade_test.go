package storage_test

import (
	"context"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	"github.com/jonaskahn/relo/internal/config"
	"github.com/jonaskahn/relo/tests/testkit"
)

// TestOpenCodeFreePathUpgradeRenamesOnlyTheShippedDefault covers the base URL
// the signed-out lane now answers on: the template's own /zen default moves to
// the current /inference path, while an operator's custom address and the Go
// lane keep theirs.
func TestOpenCodeFreePathUpgradeRenamesOnlyTheShippedDefault(t *testing.T) {
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
	// Everything before the path rename, which is where this migration begins.
	// Later migrations are unrelated and must not become the one under test.
	const pathRename = 34
	var rename *sqlite.Migration
	for i := range migrations {
		migration := &migrations[i]
		if migration.Version > pathRename {
			continue
		}
		if migration.Version == pathRename {
			rename = migration
			continue
		}
		if err := sqlite.ApplyMigration(db, *migration); err != nil {
			t.Fatalf("ApplyMigration(%d) error = %v", migration.Version, err)
		}
	}
	if rename == nil {
		t.Fatal("the OpenCode path migration is missing")
	}
	ctx := context.Background()
	rows := []struct{ id, templateID, baseURL string }{
		{"opencode-free", "opencode-free", "https://opencode.ai/zen/v1"},
		{"custom-free", "opencode-free", "https://example.test/v1"},
		{"opencode-go", "opencode-go", "https://opencode.ai/zen/go/v1"},
	}
	for _, row := range rows {
		if _, err := db.SQL().ExecContext(ctx,
			`INSERT INTO providers (id, template_id, origin, label, auth, api_format, key_header, base_url, models_format, enabled, rank, pool_strategy, created_at, updated_at)`+
				` VALUES (?, ?, 'template', ?, 'none', 'openai-chat', 'none', ?, 'openai', 1, 100, 'least-loaded', 1, 1)`,
			row.id, row.templateID, row.id, row.baseURL); err != nil {
			t.Fatalf("seed %s: %v", row.id, err)
		}
	}

	if err := sqlite.ApplyMigration(db, *rename); err != nil {
		t.Fatalf("ApplyMigration(%s) error = %v", rename.Name, err)
	}

	want := map[string]string{
		"opencode-free": "https://opencode.ai/inference/openai/v1",
		"custom-free":   "https://example.test/v1",
		"opencode-go":   "https://opencode.ai/zen/go/v1",
	}
	for id, expected := range want {
		var got string
		if err := db.SQL().QueryRowContext(ctx,
			"SELECT base_url FROM providers WHERE id = ?", id).Scan(&got); err != nil {
			t.Fatalf("read %s: %v", id, err)
		}
		if got != expected {
			t.Errorf("%s base url = %q, want %q", id, got, expected)
		}
	}
}
