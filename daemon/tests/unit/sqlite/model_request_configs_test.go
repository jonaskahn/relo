package storage_test

import (
	"context"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/internal/config"
	"github.com/jonaskahn/relo/tests/testkit"
)

func requestVariant(group, selection, effort, details string) catalog.ModelRequestConfig {
	return catalog.EncodeCursorConfig(catalog.CursorVariant{
		GroupID: group, Selection: selection, Effort: effort,
		ModelID: "sonnet", DetailsID: details,
		Params:  []catalog.CursorParam{{ID: "effort", Value: effort}},
		Context: 200000, Reasoning: true,
	})
}

func TestRequestConfigsRoundTrip(t *testing.T) {
	ctx := context.Background()
	db := testkit.OpenTestDB(t)
	repo := sqlite.NewRequestConfigRepo(db)
	configs := []catalog.ModelRequestConfig{
		requestVariant("sonnet", "low", "low", "sonnet-low"),
		requestVariant("sonnet", "high", "high", "sonnet-high"),
	}
	configs[0].Default = true
	if err := repo.Save(ctx, "cursor", "cred-a", configs); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	stored, err := repo.Load(ctx, "cursor", "cred-a")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	bySelection := map[string]catalog.CursorVariant{}
	for _, config := range stored {
		variant, ok := catalog.DecodeCursorConfig(config)
		if !ok {
			t.Fatalf("stored = %+v, want cursor tuples back", stored)
		}
		bySelection[variant.Selection] = variant
	}
	low, high := bySelection["low"], bySelection["high"]
	if len(stored) != 2 || !low.Default || high.DetailsID != "sonnet-high" {
		t.Fatalf("stored = %+v", stored)
	}
	if len(low.Params) != 1 || low.Params[0].ID != "effort" || low.Context != 200000 || !low.Reasoning {
		t.Fatalf("low = %+v, want params, context, and reasoning kept", low)
	}
	if _, err := repo.Load(ctx, "cursor", "cred-b"); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
}

func TestRequestConfigsReplace(t *testing.T) {
	ctx := context.Background()
	db := testkit.OpenTestDB(t)
	repo := sqlite.NewRequestConfigRepo(db)
	if err := repo.Save(ctx, "cursor", "cred-a", []catalog.ModelRequestConfig{
		requestVariant("sonnet", "low", "low", "sonnet-low"),
		requestVariant("sonnet", "high", "high", "sonnet-high"),
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if err := repo.Save(ctx, "cursor", "cred-a", []catalog.ModelRequestConfig{
		requestVariant("sonnet", "low", "low", "sonnet-low"),
		requestVariant("sonnet", "low", "low", "sonnet-low"),
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	stored, err := repo.Load(ctx, "cursor", "cred-a")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(stored) != 1 {
		t.Fatalf("stored = %+v, want the duplicate dropped and the stale row replaced", stored)
	}
	if err := repo.Save(ctx, "cursor", "cred-a", nil); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	cleared, err := repo.Load(ctx, "cursor", "cred-a")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(cleared) != 0 {
		t.Fatalf("cleared = %+v, want an empty refresh to clear the account", cleared)
	}
}

func TestRequestConfigsForget(t *testing.T) {
	ctx := context.Background()
	db := testkit.OpenTestDB(t)
	repo := sqlite.NewRequestConfigRepo(db)
	if err := repo.Save(ctx, "cursor", "cred-a", []catalog.ModelRequestConfig{
		requestVariant("sonnet", "low", "low", "sonnet-low"),
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if err := repo.Forget(ctx, "cred-a"); err != nil {
		t.Fatalf("Forget() error = %v", err)
	}
	stored, err := repo.Load(ctx, "cursor", "cred-a")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(stored) != 0 {
		t.Fatalf("stored = %+v, want nothing after forget", stored)
	}
}

// TestRequestConfigsUpgrade covers a state database written before request
// configurations were shared: every Cursor tuple survives the move with its
// exact triple, and the old table is gone.
func TestRequestConfigsUpgrade(t *testing.T) {
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
	var cursor, shared int
	for index, migration := range migrations {
		switch migration.Version {
		case 27:
			cursor = index
		case 29:
			shared = index
		}
	}
	if shared == 0 {
		t.Fatalf("migrations = %+v, want the shared request configs migration", migrations)
	}
	if _, err := db.SQL().Exec("CREATE TABLE schema_migrations (" +
		"version INTEGER NOT NULL PRIMARY KEY, applied_at INTEGER NOT NULL, checksum TEXT NOT NULL) STRICT"); err != nil {
		t.Fatalf("create the migration table: %v", err)
	}
	for _, migration := range migrations[:cursor+1] {
		if err := sqlite.ApplyMigration(db, migration); err != nil {
			t.Fatalf("ApplyMigration(%d) error = %v", migration.Version, err)
		}
	}
	ctx := context.Background()
	if _, err := db.SQL().ExecContext(ctx,
		`INSERT INTO cursor_model_configs
			(provider_id, credential_id, group_id, selection, effort, is_default,
			 model_id, details_id, max_mode, params, context_window, images, reasoning)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"cursor", "cred-a", "sonnet", "low", "low", 1,
		"sonnet", "sonnet-low", 0, `[{"id":"effort","value":"low"}]`,
		200000, 0, 1); err != nil {
		t.Fatalf("seed the cursor table: %v", err)
	}
	if _, err := db.SQL().ExecContext(ctx,
		`INSERT INTO cursor_model_configs
			(provider_id, credential_id, group_id, selection, effort, is_default,
			 model_id, details_id, max_mode, params, context_window, images, reasoning)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"cursor", "cred-a", "sonnet", "high", "high", 0,
		"sonnet", "sonnet-high", 1, `[{"id":"effort","value":"high"}]`,
		200000, 0, 1); err != nil {
		t.Fatalf("seed the cursor table: %v", err)
	}

	for _, migration := range migrations[cursor+1 : shared+1] {
		if err := sqlite.ApplyMigration(db, migration); err != nil {
			t.Fatalf("ApplyMigration(%d) error = %v", migration.Version, err)
		}
	}

	stored, err := sqlite.NewRequestConfigRepo(db).Load(ctx, "cursor", "cred-a")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(stored) != 2 {
		t.Fatalf("stored = %+v, want both tuples moved", stored)
	}
	bySelection := map[string]catalog.CursorVariant{}
	for _, config := range stored {
		variant, ok := catalog.DecodeCursorConfig(config)
		if !ok {
			t.Fatalf("stored = %+v, want cursor tuples back", stored)
		}
		bySelection[variant.Selection] = variant
	}
	low, high := bySelection["low"], bySelection["high"]
	if !low.Default || low.DetailsID != "sonnet-low" || high.DetailsID != "sonnet-high" || !high.MaxMode {
		t.Fatalf("stored = %+v, want the exact triples kept", stored)
	}
	if len(low.Params) != 1 || low.Params[0].Value != "low" {
		t.Fatalf("low = %+v, want the params kept", low)
	}
	var tables int
	if err := db.SQL().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'cursor_model_configs'`).Scan(&tables); err != nil {
		t.Fatalf("count the old table: %v", err)
	}
	if tables != 0 {
		t.Fatal("cursor_model_configs still exists")
	}
}
