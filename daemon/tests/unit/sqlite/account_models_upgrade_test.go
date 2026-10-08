package storage_test

import (
	"context"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	"github.com/jonaskahn/relo/internal/config"
	"github.com/jonaskahn/relo/tests/testkit"
)

// TestAccountModelsAndIntegrationUpgrade covers a state database written
// before an account could hold a model roster, before a route member could
// name a bare model, and before a client key could belong to an integration:
// every stored row survives, and the three new shapes are usable afterwards.
func TestAccountModelsAndIntegrationUpgrade(t *testing.T) {
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
	if len(migrations) < 8 {
		t.Fatalf("migrations = %d, want the eight this build ships", len(migrations))
	}
	if _, err := db.SQL().Exec("CREATE TABLE schema_migrations (" +
		"version INTEGER NOT NULL PRIMARY KEY, applied_at INTEGER NOT NULL, checksum TEXT NOT NULL) STRICT"); err != nil {
		t.Fatalf("create the migration table: %v", err)
	}
	// The database an operator already has: everything up to the Codex
	// dialect, which is where this build's new migrations begin.
	for _, migration := range migrations[:5] {
		if err := sqlite.ApplyMigration(db, migration); err != nil {
			t.Fatalf("ApplyMigration(%d) error = %v", migration.Version, err)
		}
	}
	ctx := context.Background()
	seed := []struct {
		statement string
		args      []any
	}{
		{"INSERT INTO providers (id, origin, label, auth, api_format, key_header, base_url, models_source, models_format, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
			[]any{"openai", "signin", "ChatGPT", "oauth", "openai-responses", "bearer", "https://example.test", "listing", "codex", 1, 1}},
		{"INSERT INTO models (provider_id, model_id, source, api_format, enabled, updated_at) VALUES (?, ?, ?, ?, ?, ?)",
			[]any{"openai", "gpt-5.5", "listing", "openai-responses", 1, 1}},
		{"INSERT INTO credentials (id, provider_id, kind, label, status, priority, generation, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
			[]any{"cred-1", "openai", "oauth", "work", "active", 0, 0, 1, 1}},
		{"INSERT INTO model_groups (id, label, strategy, enabled, listed, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
			[]any{"fast", "Fast", "priority", 1, 1, 1, 1}},
		{"INSERT INTO model_group_members (group_id, position, provider_id, model_id, weight, enabled) VALUES (?, ?, ?, ?, ?, ?)",
			[]any{"fast", 0, "openai", "gpt-5.5", 1, 1}},
		{"INSERT INTO access_keys (id, name, kind, client, token_digest, token_hint, generation, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
			[]any{"key-1", "operator-key", "agent", "codex", "digest", "abcd...ef", 1, 1, 1}},
	}
	for _, row := range seed {
		if _, err := db.SQL().ExecContext(ctx, row.statement, row.args...); err != nil {
			t.Fatalf("seed the old schema: %v", err)
		}
	}

	if err := sqlite.Migrate(db); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	// Everything the operator already had is still there.
	if _, err := sqlite.NewCatalogRepo(db).GetProvider(ctx, "openai"); err != nil {
		t.Fatalf("the provider did not survive: %v", err)
	}
	models, err := sqlite.NewCatalogRepo(db).ListModels(ctx)
	if err != nil || len(models) != 1 || models[0].ModelID != "gpt-5.5" {
		t.Fatalf("models = %+v error = %v, want the stored model", models, err)
	}
	group, err := sqlite.NewCatalogRepo(db).GetGroup(ctx, "fast")
	if err != nil {
		t.Fatalf("the route did not survive: %v", err)
	}
	if len(group.Members) != 1 || group.Members[0].Kind != "model" {
		t.Fatalf("members = %+v, want the stored member read as one connection's model", group.Members)
	}
	credentials, err := sqlite.NewCredentialRepo(db).List(ctx)
	if err != nil || len(credentials) != 1 {
		t.Fatalf("credentials = %+v error = %v, want the stored account", credentials, err)
	}
	keys, err := sqlite.NewAccessKeyRepo(db).List(ctx)
	if err != nil || len(keys) != 1 || keys[0].Owner != "" {
		t.Fatalf("keys = %+v error = %v, want the operator's own key left unowned", keys, err)
	}

	// The new shapes work on that same database.
	repo := sqlite.NewCredentialModelRepo(db)
	if err := repo.SetRoster(ctx, "cred-1", "listing", []string{"gpt-5.5"}, 2); err != nil {
		t.Fatalf("SetRoster() error = %v", err)
	}
	stored, rosters, err := repo.List(ctx)
	if err != nil || len(stored) != 1 || len(rosters) != 1 {
		t.Fatalf("stored = %+v rosters = %+v error = %v, want one of each", stored, rosters, err)
	}
	if err := sqlite.NewCatalogRepo(db).SaveGroup(ctx, sqlite.GroupRow{
		ID: "fast", Label: "Fast", Strategy: "priority", Enabled: true, Listed: true,
		Members: []sqlite.GroupMemberRow{
			{Position: 0, ModelID: "gpt-5.5", Kind: "auto", Weight: 1, Enabled: true},
		},
	}); err != nil {
		t.Fatalf("SaveGroup() with a bare member error = %v", err)
	}
	group, err = sqlite.NewCatalogRepo(db).GetGroup(ctx, "fast")
	if err != nil {
		t.Fatalf("GetGroup() error = %v", err)
	}
	if len(group.Members) != 1 || group.Members[0].Kind != "auto" || group.Members[0].ProviderID != "" {
		t.Fatalf("members = %+v, want the bare member stored without a connection", group.Members)
	}
	if err := sqlite.NewIntegrationRepo(db).Save(ctx, sqlite.IntegrationRow{
		ID: "codex", Enabled: true, KeyID: "key-1", State: "on", UpdatedAtMs: 3,
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	record, found, err := sqlite.NewIntegrationRepo(db).Get(ctx, "codex")
	if err != nil || !found || !record.Enabled {
		t.Fatalf("integration = %+v found = %v error = %v, want the stored integration", record, found, err)
	}
}
