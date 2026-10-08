package storage_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	"github.com/jonaskahn/relo/tests/testkit"
)

func TestTemplateSettingsCarryContextVariants(t *testing.T) {
	logger, _ := testkit.TestLogger(t)
	db, err := sqlite.OpenDB(filepath.Join(t.TempDir(), "state.sqlite"), logger)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.SQL().Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER NOT NULL PRIMARY KEY,
		applied_at INTEGER NOT NULL,
		checksum TEXT NOT NULL
	) STRICT`); err != nil {
		t.Fatalf("create schema_migrations: %v", err)
	}
	migrations, err := sqlite.LoadMigrations()
	if err != nil {
		t.Fatalf("LoadMigrations() error = %v", err)
	}
	for _, migration := range migrations {
		if migration.Version >= 18 {
			break
		}
		if err := sqlite.ApplyMigration(db, migration); err != nil {
			t.Fatalf("apply %s: %v", migration.Name, err)
		}
	}
	if _, err := db.SQL().Exec(`INSERT INTO context_variants (template_id, enabled, updated_at) VALUES ('claude', 0, 10)`); err != nil {
		t.Fatalf("seed context variant: %v", err)
	}
	for _, migration := range migrations {
		if migration.Version != 18 {
			continue
		}
		if err := sqlite.ApplyMigration(db, migration); err != nil {
			t.Fatalf("apply %s: %v", migration.Name, err)
		}
	}
	choice, found, err := sqlite.NewTemplateSettings(db).Get(context.Background(), "claude")
	if err != nil || !found {
		t.Fatalf("Get() = %+v, %v, %v, want the carried row", choice, found, err)
	}
	if choice.LongContext || !choice.AutoRefresh {
		t.Fatalf("choice = %+v, want long context off and refresh on", choice)
	}
}

func TestTemplateSettingsSave(t *testing.T) {
	db := testkit.OpenTestDB(t)
	store := sqlite.NewTemplateSettings(db)
	ctx := context.Background()
	off := false
	if err := store.Save(ctx, "claude", &off, nil, 20); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	choice, found, err := store.Get(ctx, "claude")
	if err != nil || !found || choice.LongContext || !choice.AutoRefresh {
		t.Fatalf("Get() = %+v, %v, %v, want long context off and refresh still on", choice, found, err)
	}
	if err := store.Save(ctx, "claude", nil, &off, 21); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if store.AutoRefresh(ctx, "claude") {
		t.Fatal("AutoRefresh() = true, want the stored off switch")
	}
	if err := store.Save(ctx, "openai-codex", nil, &off, 22); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if store.AutoRefresh(ctx, "openai-codex") {
		t.Fatal("AutoRefresh() = true, want the ChatGPT switch to be honored too")
	}
	on := true
	if err := store.Save(ctx, "claude", nil, &on, 23); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if !store.AutoRefresh(ctx, "claude") || store.AutoRefresh(ctx, "openai-codex") || store.AutoRefresh(ctx, "other") != true {
		t.Fatal("AutoRefresh() did not read the stored rows, or leaked to a provider without one")
	}
}
