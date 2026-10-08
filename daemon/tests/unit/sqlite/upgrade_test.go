package storage_test

import (
	"context"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	"github.com/jonaskahn/relo/internal/config"
	"github.com/jonaskahn/relo/tests/testkit"
)

// TestQuotaUpgrade covers a state database written before a quota window
// carried its length: the baseline schema still opens, and the new migration
// adds the column without touching the windows already stored.
func TestQuotaUpgrade(t *testing.T) {
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
	if len(migrations) < 2 {
		t.Fatalf("migrations = %d, want the baseline and the quota upgrade", len(migrations))
	}
	// Apply the baseline alone, which is what a database written by an
	// earlier build looks like.
	if _, err := db.SQL().Exec(`CREATE TABLE schema_migrations (
		version    INTEGER NOT NULL PRIMARY KEY,
		applied_at INTEGER NOT NULL,
		checksum   TEXT    NOT NULL
	) STRICT`); err != nil {
		t.Fatalf("create the migration table: %v", err)
	}
	if err := sqlite.ApplyMigration(db, migrations[0]); err != nil {
		t.Fatalf("ApplyMigration(baseline) error = %v", err)
	}
	ctx := context.Background()
	if _, err := db.SQL().ExecContext(ctx,
		"INSERT INTO quota_snapshots (credential_id, window, used_percent, reset_at, source, updated_at) VALUES (?, ?, ?, ?, ?, ?)",
		"credential-1", "Gem", 30.0, 100, "probe", 1); err != nil {
		t.Fatalf("seed a window in the old schema: %v", err)
	}
	if err := sqlite.Migrate(db); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	rows, err := sqlite.NewQuotaRepo(db).ListSnapshots(ctx, "credential-1")
	if err != nil {
		t.Fatalf("ListSnapshots() error = %v", err)
	}
	if len(rows) != 1 || rows[0].Window != "Gem" || rows[0].UsedPercent != 30 || rows[0].Seconds != 0 {
		t.Fatalf("rows = %+v, want the stored window with no length", rows)
	}
	if err := sqlite.NewQuotaRepo(db).UpsertSnapshots(ctx, []sqlite.QuotaRow{{
		CredentialID: "credential-1", Window: "5h", UsedPercent: 42,
		Seconds: 18000, Source: "header", ObservedAt: rows[0].ObservedAt,
	}}); err != nil {
		t.Fatalf("UpsertSnapshots() after the upgrade error = %v", err)
	}
	after, err := sqlite.NewQuotaRepo(db).ListSnapshots(ctx, "credential-1")
	if err != nil {
		t.Fatalf("ListSnapshots() error = %v", err)
	}
	if len(after) != 2 {
		t.Fatalf("rows = %+v, want the old window beside the new one", after)
	}
}

// TestProviderModelsFormatUpgrade covers a state database written before the
// Codex sign-in connection could be stored: the check that refused it is
// widened, and every model, fact, credential, and route member of an existing
// connection survives the table rebuild.
func TestProviderModelsFormatUpgrade(t *testing.T) {
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
	if len(migrations) < 5 {
		t.Fatalf("migrations = %d, want the baseline and the codex upgrade", len(migrations))
	}
	if _, err := db.SQL().Exec(`CREATE TABLE schema_migrations (
		version    INTEGER NOT NULL PRIMARY KEY,
		applied_at INTEGER NOT NULL,
		checksum   TEXT    NOT NULL
	) STRICT`); err != nil {
		t.Fatalf("create the migration table: %v", err)
	}
	if err := sqlite.ApplyMigration(db, migrations[0]); err != nil {
		t.Fatalf("ApplyMigration(baseline) error = %v", err)
	}
	ctx := context.Background()
	seed := []string{
		"INSERT INTO providers (id, template_id, origin, label, auth, api_format, models_format, created_at, updated_at)" +
			" VALUES ('claude','claude','signin','Claude','oauth','anthropic','anthropic',1,1)",
		"INSERT INTO models (provider_id, model_id, api_format, updated_at) VALUES ('claude','opus','anthropic',1)",
		"INSERT INTO model_facts (provider_id, model_id, layer, name) VALUES ('claude','opus','provider','Opus')",
		"INSERT INTO credentials (id, provider_id, kind, label, created_at, updated_at) VALUES ('c1','claude','oauth','default',1,1)",
		"INSERT INTO model_groups (id, label, strategy, created_at, updated_at) VALUES ('fast','Fast','priority',1,1)",
		"INSERT INTO model_group_members (group_id, position, provider_id, model_id) VALUES ('fast',0,'claude','opus')",
	}
	for _, statement := range seed {
		if _, err := db.SQL().ExecContext(ctx, statement); err != nil {
			t.Fatalf("seed %q: %v", statement, err)
		}
	}
	if err := sqlite.Migrate(db); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	counts := map[string]int{}
	for _, table := range []string{"providers", "models", "model_facts", "credentials", "model_group_members"} {
		var count int
		if err := db.SQL().QueryRowContext(ctx, "SELECT count(*) FROM "+table).Scan(&count); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		counts[table] = count
	}
	for _, table := range []string{"providers", "models", "model_facts", "credentials", "model_group_members"} {
		if counts[table] != 1 {
			t.Fatalf("%s holds %d rows after the upgrade, want the seeded one", table, counts[table])
		}
	}
	// The row the console could not write before is storable now, and the
	// vocabulary is still closed to everything else.
	repo := sqlite.NewCatalogRepo(db)
	if err := repo.SaveProvider(ctx, sqlite.ProviderRow{
		ID: "openai-codex", TemplateID: "openai-codex", Origin: "signin", Label: "ChatGPT (Codex sign-in)",
		Auth: "oauth", APIFormat: "openai-responses", ModelsFormat: "codex",
		Enabled: true, Rank: 100, PoolStrategy: "least-loaded", CreatedAtMs: 1, UpdatedAtMs: 1,
	}); err != nil {
		t.Fatalf("SaveProvider(codex) error = %v, want the widened check to accept it", err)
	}
	if err := repo.SaveProvider(ctx, sqlite.ProviderRow{
		ID: "bogus", Origin: "custom", Label: "Bogus", Auth: "api_key", ModelsFormat: "telepathy",
	}); err == nil {
		t.Fatal("SaveProvider(bogus) error = nil, want the check to stay closed")
	}
}
