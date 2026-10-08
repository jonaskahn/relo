package storage_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	"github.com/jonaskahn/relo/tests/testkit"
)

// seedProvider inserts a minimal provider row so credential FK constraints pass.
func seedProvider(t *testing.T, db *sqlite.DB, id string) {
	t.Helper()
	repo := sqlite.NewCatalogRepo(db)
	if err := repo.SaveProvider(context.Background(), sqlite.ProviderRow{
		ID: id, Origin: "custom", Label: id, Auth: "api_key",
		APIFormat: "openai-chat", KeyHeader: "bearer",
		ModelsFormat: "none", Headers: map[string]string{},
		Variables: map[string]string{},
		Enabled:   true, Rank: 100, PoolStrategy: sqlite.StrategyLeastLoaded,
	}); err != nil {
		t.Fatalf("seed provider %s: %v", id, err)
	}
}

func TestCredentialRepo(t *testing.T) {
	t.Run("list is empty on a fresh database", func(t *testing.T) {
		repo := sqlite.NewCredentialRepo(testkit.OpenTestDB(t))
		rows, err := repo.List(context.Background())
		if err != nil {
			t.Fatalf("List() error = %v", err)
		}
		if len(rows) != 0 {
			t.Fatalf("List() returned %d rows, want 0", len(rows))
		}
	})

	t.Run("insert then list round-trips", func(t *testing.T) {
		db := testkit.OpenTestDB(t)
		seedProvider(t, db, "openai")
		repo := sqlite.NewCredentialRepo(db)
		ctx := context.Background()
		row := sqlite.CredentialRow{
			ID: "cred-1", ProviderID: "openai", Kind: "api_key",
			Label: "default", SecretRef: "apikey/openai/cred-1", Status: "active", Priority: 3,
		}
		if err := repo.Insert(ctx, row); err != nil {
			t.Fatalf("Insert() error = %v", err)
		}
		rows, err := repo.List(ctx)
		if err != nil {
			t.Fatalf("List() error = %v", err)
		}
		if len(rows) != 1 || rows[0] != row {
			t.Fatalf("List() = %+v, want %+v", rows, row)
		}
	})

	t.Run("a row without a label takes the schema default", func(t *testing.T) {
		db := testkit.OpenTestDB(t)
		seedProvider(t, db, "openai")
		const insert = "INSERT INTO credentials (id, provider_id, kind, status, priority, generation, created_at, updated_at)" +
			" VALUES ('cred-null', 'openai', 'api_key', 'active', 0, 0, 1, 1)"
		if _, err := db.SQL().Exec(insert); err != nil {
			t.Fatalf("insert a credential with nulls: %v", err)
		}
		rows, err := sqlite.NewCredentialRepo(db).List(context.Background())
		if err != nil {
			t.Fatalf("List() error = %v", err)
		}
		if len(rows) != 1 || rows[0].Label != "default" || rows[0].SecretRef != "" {
			t.Fatalf("List() = %+v, want the default label and no secret ref", rows)
		}
	})

	t.Run("set status updates a live credential", func(t *testing.T) {
		db := testkit.OpenTestDB(t)
		seedProvider(t, db, "openai")
		repo := sqlite.NewCredentialRepo(db)
		ctx := context.Background()
		if err := repo.Insert(ctx, sqlite.CredentialRow{ID: "cred-1", ProviderID: "openai", Kind: "api_key", Status: "active"}); err != nil {
			t.Fatalf("Insert() error = %v", err)
		}
		if err := repo.SetStatus(ctx, "cred-1", "paused"); err != nil {
			t.Fatalf("SetStatus() error = %v", err)
		}
		rows, err := repo.List(ctx)
		if err != nil {
			t.Fatalf("List() error = %v", err)
		}
		if rows[0].Status != "paused" {
			t.Fatalf("status = %q, want paused", rows[0].Status)
		}
		if err := repo.SetStatus(ctx, "absent", "paused"); !errors.Is(err, sqlite.ErrCredentialNotFound) {
			t.Fatalf("SetStatus(absent) error = %v, want %v", err, sqlite.ErrCredentialNotFound)
		}
	})

	t.Run("set priority updates a live credential", func(t *testing.T) {
		db := testkit.OpenTestDB(t)
		seedProvider(t, db, "openai")
		repo := sqlite.NewCredentialRepo(db)
		ctx := context.Background()
		row := sqlite.CredentialRow{
			ID: "cred-1", ProviderID: "openai", Kind: "api_key",
			Label: "default", SecretRef: "apikey/openai/cred-1", Status: "active",
		}
		if err := repo.Insert(ctx, row); err != nil {
			t.Fatalf("Insert() error = %v", err)
		}
		if err := repo.SetPriority(ctx, "cred-1", 7); err != nil {
			t.Fatalf("SetPriority() error = %v", err)
		}
		rows, err := repo.List(ctx)
		if err != nil {
			t.Fatalf("List() error = %v", err)
		}
		if len(rows) != 1 || rows[0].Priority != 7 {
			t.Fatalf("List() = %+v, want the new priority", rows)
		}
		if err := repo.SetPriority(ctx, "absent", 1); !errors.Is(err, sqlite.ErrCredentialNotFound) {
			t.Fatalf("SetPriority(absent) error = %v, want %v", err, sqlite.ErrCredentialNotFound)
		}
	})

	t.Run("soft delete tombstones a credential", func(t *testing.T) {
		db := testkit.OpenTestDB(t)
		seedProvider(t, db, "openai")
		repo := sqlite.NewCredentialRepo(db)
		ctx := context.Background()
		if err := repo.Insert(ctx, sqlite.CredentialRow{ID: "cred-1", ProviderID: "openai", Kind: "api_key", Status: "active"}); err != nil {
			t.Fatalf("Insert() error = %v", err)
		}
		if err := repo.SoftDelete(ctx, "cred-1"); err != nil {
			t.Fatalf("SoftDelete() error = %v", err)
		}
		rows, err := repo.List(ctx)
		if err != nil {
			t.Fatalf("List() error = %v", err)
		}
		if len(rows) != 0 {
			t.Fatalf("List() returned %d rows after deletion, want 0", len(rows))
		}
		if got := countRows(t, db, "credentials"); got != 1 {
			t.Fatalf("credentials rows = %d, want the tombstone to stay", got)
		}
		if err := repo.SoftDelete(ctx, "cred-1"); !errors.Is(err, sqlite.ErrCredentialNotFound) {
			t.Fatalf("second SoftDelete() error = %v, want %v", err, sqlite.ErrCredentialNotFound)
		}
	})

	t.Run("list orders by provider, priority, and id", func(t *testing.T) {
		db := testkit.OpenTestDB(t)
		seedProvider(t, db, "openai")
		seedProvider(t, db, "anthropic")
		repo := sqlite.NewCredentialRepo(db)
		ctx := context.Background()
		rows := []sqlite.CredentialRow{
			{ID: "b", ProviderID: "openai", Kind: "api_key", Status: "active", Priority: 2},
			{ID: "a", ProviderID: "openai", Kind: "api_key", Status: "active", Priority: 1},
			{ID: "c", ProviderID: "anthropic", Kind: "api_key", Status: "active"},
		}
		for _, row := range rows {
			if err := repo.Insert(ctx, row); err != nil {
				t.Fatalf("Insert(%s) error = %v", row.ID, err)
			}
		}
		listed, err := repo.List(ctx)
		if err != nil {
			t.Fatalf("List() error = %v", err)
		}
		want := []string{"c", "a", "b"}
		for index, id := range want {
			if listed[index].ID != id {
				t.Fatalf("List()[%d] = %q, want %q (order %+v)", index, listed[index].ID, id, listed)
			}
		}
	})

	t.Run("duplicate insert is rejected", func(t *testing.T) {
		db := testkit.OpenTestDB(t)
		seedProvider(t, db, "openai")
		repo := sqlite.NewCredentialRepo(db)
		ctx := context.Background()
		row := sqlite.CredentialRow{ID: "cred-1", ProviderID: "openai", Kind: "api_key", Status: "active"}
		if err := repo.Insert(ctx, row); err != nil {
			t.Fatalf("Insert() error = %v", err)
		}
		if err := repo.Insert(ctx, row); err == nil {
			t.Fatal("second Insert() error = nil, want a constraint failure")
		}
	})

	t.Run("closed database reports errors", func(t *testing.T) {
		db := testkit.OpenTestDB(t)
		repo := sqlite.NewCredentialRepo(db)
		if err := db.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
		ctx := context.Background()
		if _, err := repo.List(ctx); err == nil {
			t.Fatal("List() on a closed database error = nil, want a failure")
		}
		if err := repo.Insert(ctx, sqlite.CredentialRow{ID: "cred-2"}); err == nil {
			t.Fatal("Insert() on a closed database error = nil, want a failure")
		}
		if err := repo.SetStatus(ctx, "cred-2", "paused"); err == nil {
			t.Fatal("SetStatus() on a closed database error = nil, want a failure")
		}
		if err := repo.SoftDelete(ctx, "cred-2"); err == nil {
			t.Fatal("SoftDelete() on a closed database error = nil, want a failure")
		}
	})

	t.Run("a drifted table reports a scan error", func(t *testing.T) {
		db := testkit.OpenTestDB(t)
		if _, err := db.SQL().Exec("DROP TABLE credentials"); err != nil {
			t.Fatalf("drop the credentials table: %v", err)
		}
		const drifted = "CREATE TABLE credentials (id TEXT, provider_id TEXT, kind TEXT, label TEXT," +
			" secret_ref TEXT, status TEXT, priority TEXT, deleted_at INTEGER)"
		if _, err := db.SQL().Exec(drifted); err != nil {
			t.Fatalf("create a drifted credentials table: %v", err)
		}
		if _, err := db.SQL().Exec("INSERT INTO credentials VALUES ('c1', 'openai', 'api_key', '', '', 'active', 'not-a-number', NULL)"); err != nil {
			t.Fatalf("insert a drifted row: %v", err)
		}
		if _, err := sqlite.NewCredentialRepo(db).List(context.Background()); err == nil {
			t.Fatal("List() error = nil, want a scan failure")
		}
	})
}
