package storage_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	"github.com/jonaskahn/relo/tests/testkit"
)

var wantTables = []string{
	"schema_migrations", "providers", "models", "model_facts", "modelsdev_state",
	"credentials", "usage_events", "usage_attempts", "quota_snapshots", "quota_history",
}

func TestOpenDBFreshDatabase(t *testing.T) {
	t.Run("fresh DB creates tables and confirms WAL", func(t *testing.T) {
		logger, _ := testkit.TestLogger(t)
		path := filepath.Join(t.TempDir(), "state.sqlite")
		db, err := sqlite.OpenDB(path, logger)
		if err != nil {
			t.Fatalf("OpenDB() error = %v", err)
		}
		defer func() { _ = db.Close() }()
		if err := sqlite.Migrate(db); err != nil {
			t.Fatalf("Migrate() error = %v", err)
		}
		for _, table := range wantTables {
			if !tableExists(t, db, table) {
				t.Fatalf("table %s was not created", table)
			}
		}
		if mode := pragmaText(t, db, "journal_mode"); mode != "wal" {
			t.Fatalf("journal_mode = %q, want wal", mode)
		}
		if got := pragmaInt(t, db, "foreign_keys"); got != 1 {
			t.Fatalf("foreign_keys = %d, want 1", got)
		}
		version, err := db.SchemaVersion()
		if err != nil {
			t.Fatalf("SchemaVersion() error = %v", err)
		}
		if want := sqlite.LatestSchemaVersion(); version != want {
			t.Fatalf("SchemaVersion() = %d, want %d", version, want)
		}
		if db.Path() != path {
			t.Fatalf("Path() = %q, want %q", db.Path(), path)
		}
	})

	t.Run("OpenDB sets file permissions 0600", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("windows does not enforce unix permissions")
		}
		logger, _ := testkit.TestLogger(t)
		path := filepath.Join(testkit.TempHome(t), "state.sqlite")
		db, err := sqlite.OpenDB(path, logger)
		if err != nil {
			t.Fatalf("OpenDB() error = %v", err)
		}
		defer func() { _ = db.Close() }()
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat database: %v", err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("database mode = %v, want 0600", info.Mode().Perm())
		}
	})

	t.Run("schema version without migrations reports an error", func(t *testing.T) {
		logger, _ := testkit.TestLogger(t)
		db, err := sqlite.OpenDB(filepath.Join(t.TempDir(), "state.sqlite"), logger)
		if err != nil {
			t.Fatalf("OpenDB() error = %v", err)
		}
		defer func() { _ = db.Close() }()
		if _, err := db.SchemaVersion(); err == nil {
			t.Fatal("SchemaVersion() error = nil, want an error before migrating")
		}
	})

	t.Run("Close releases DB handle", func(t *testing.T) {
		db := testkit.OpenTestDB(t)
		if err := db.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
		if err := db.SQL().Ping(); err == nil {
			t.Fatal("Ping() after Close() = nil, want an error")
		}
		if err := db.Close(); err != nil {
			t.Fatalf("second Close() error = %v", err)
		}
	})

	t.Run("OpenDB rejects a missing directory", func(t *testing.T) {
		logger, _ := testkit.TestLogger(t)
		missing := filepath.Join(t.TempDir(), "gone", "state.sqlite")
		if _, err := sqlite.OpenDB(missing, logger); err == nil {
			t.Fatal("OpenDB() error = nil, want a failure for a missing directory")
		}
	})

	t.Run("OpenDB rejects a path that is a directory", func(t *testing.T) {
		logger, _ := testkit.TestLogger(t)
		path := filepath.Join(t.TempDir(), "state.sqlite")
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatalf("create a directory in place of the database: %v", err)
		}
		if _, err := sqlite.OpenDB(path, logger); err == nil {
			t.Fatal("OpenDB() error = nil, want a connection failure")
		}
	})

	t.Run("Migrate rejects an unexpected migration table", func(t *testing.T) {
		logger, _ := testkit.TestLogger(t)
		db, err := sqlite.OpenDB(filepath.Join(t.TempDir(), "state.sqlite"), logger)
		if err != nil {
			t.Fatalf("OpenDB() error = %v", err)
		}
		defer func() { _ = db.Close() }()
		if _, err := db.SQL().Exec("CREATE TABLE schema_migrations (version TEXT)"); err != nil {
			t.Fatalf("create a foreign migration table: %v", err)
		}
		if err := sqlite.Migrate(db); err == nil {
			t.Fatal("Migrate() error = nil, want a failure for an incompatible table")
		}
	})

	t.Run("OpenDB reports an unreadable database file", func(t *testing.T) {
		logger, _ := testkit.TestLogger(t)
		path := filepath.Join(t.TempDir(), "state.sqlite")
		if err := os.WriteFile(path, []byte("this is not a sqlite database"), 0o600); err != nil {
			t.Fatalf("write a corrupt database: %v", err)
		}
		if _, err := sqlite.OpenDB(path, logger); err == nil {
			t.Fatal("OpenDB() error = nil, want a corrupt-file failure")
		}
	})

	t.Run("OpenDB refuses a database without write-ahead logging", func(t *testing.T) {
		logger, _ := testkit.TestLogger(t)
		if _, err := sqlite.OpenDB(":memory:", logger); !errors.Is(err, sqlite.ErrWALNotActive) {
			t.Fatalf("OpenDB(:memory:) error = %v, want %v", err, sqlite.ErrWALNotActive)
		}
	})
}

func TestOpenReadOnly(t *testing.T) {
	logger, _ := testkit.TestLogger(t)
	path := filepath.Join(t.TempDir(), "state.sqlite")
	writer, err := sqlite.OpenDB(path, logger)
	if err != nil {
		t.Fatalf("OpenDB() error = %v", err)
	}
	if err := sqlite.Migrate(writer); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	if _, err := writer.SQL().Exec("INSERT INTO ui_settings (id, quota_display, updated_at) VALUES (1, 'used', 1)"); err != nil {
		t.Fatalf("seed database: %v", err)
	}

	reader, err := sqlite.OpenReadOnly(path, logger)
	if err != nil {
		t.Fatalf("OpenReadOnly() with writer open error = %v", err)
	}
	var value string
	if err := reader.SQL().QueryRow("SELECT quota_display FROM ui_settings WHERE id = 1").Scan(&value); err != nil {
		t.Fatalf("read live WAL value: %v", err)
	}
	if value != "used" {
		t.Fatalf("value = %q, want used", value)
	}
	if _, err := reader.SQL().Exec("DELETE FROM ui_settings"); err == nil {
		t.Fatal("write through read-only handle succeeded")
	}
	if err := reader.Close(); err != nil {
		t.Fatalf("close live reader: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	reader, err = sqlite.OpenReadOnly(path, logger)
	if err != nil {
		t.Fatalf("OpenReadOnly() after writer close error = %v", err)
	}
	defer func() { _ = reader.Close() }()
	if err := reader.SQL().QueryRow("SELECT quota_display FROM ui_settings WHERE id = 1").Scan(&value); err != nil {
		t.Fatalf("read closed database value: %v", err)
	}
}

func TestLoadMigrations(t *testing.T) {
	migrations, err := sqlite.LoadMigrations()
	if err != nil {
		t.Fatalf("LoadMigrations() error = %v", err)
	}
	if len(migrations) < 1 {
		t.Fatalf("LoadMigrations() returned %d migrations, want at least the baseline migration", len(migrations))
	}
	first := migrations[0]
	if first.Version != 1 || first.Name != "0001_baseline.sql" {
		t.Fatalf("migration = %+v, want version 1 named 0001_baseline.sql", first)
	}
	if len(first.Checksum) != 64 || first.SQL == "" {
		t.Fatalf("checksum = %q, want a sha256 hex digest over a non-empty migration", first.Checksum)
	}
	last := migrations[len(migrations)-1]
	if last.Version != sqlite.LatestSchemaVersion() {
		t.Fatalf("last migration = %+v, want the newest embedded migration at version %d", last, sqlite.LatestSchemaVersion())
	}
	for index, migration := range migrations {
		if index > 0 && migrations[index-1].Version >= migration.Version {
			t.Fatalf("migrations are out of order at %s", migration.Name)
		}
	}
}

func TestMigrate(t *testing.T) {
	t.Run("already migrated is no-op", func(t *testing.T) {
		db := testkit.OpenTestDB(t)
		if err := sqlite.Migrate(db); err != nil {
			t.Fatalf("second Migrate() error = %v", err)
		}
		version, err := db.SchemaVersion()
		if err != nil {
			t.Fatalf("SchemaVersion() error = %v", err)
		}
		if want := sqlite.LatestSchemaVersion(); version != want {
			t.Fatalf("SchemaVersion() = %d, want %d", version, want)
		}
	})

	t.Run("tampered migration checksum error", func(t *testing.T) {
		db := testkit.OpenTestDB(t)
		if _, err := db.SQL().Exec("UPDATE schema_migrations SET checksum = 'deadbeef' WHERE version = 1"); err != nil {
			t.Fatalf("tamper with the migration record: %v", err)
		}
		err := sqlite.Migrate(db)
		if !errors.Is(err, sqlite.ErrChecksumMismatch) {
			t.Fatalf("Migrate() error = %v, want %v", err, sqlite.ErrChecksumMismatch)
		}
	})

	t.Run("bad SQL rolls back and DB untouched", func(t *testing.T) {
		db := testkit.OpenTestDB(t)
		broken := sqlite.Migration{
			Version:  99,
			Name:     "0099_broken.sql",
			SQL:      "CREATE TABLE broken (id INTEGER); INSERT INTO no_such_table VALUES (1)",
			Checksum: "broken",
		}
		err := sqlite.ApplyMigration(db, broken)
		if !errors.Is(err, sqlite.ErrMigrationFailed) {
			t.Fatalf("ApplyMigration() error = %v, want %v", err, sqlite.ErrMigrationFailed)
		}
		if tableExists(t, db, "broken") {
			t.Fatal("a failed migration left part of its schema behind")
		}
	})

	t.Run("concurrent reads while write in flight", func(t *testing.T) {
		db := testkit.OpenTestDB(t)
		// Seed a provider so credential FK constraints pass.
		if err := sqlite.NewCatalogRepo(db).SaveProvider(context.Background(), sqlite.ProviderRow{
			ID: "openai", Origin: "custom", Label: "OpenAI", Auth: "api_key",
			KeyHeader: "bearer", ModelsFormat: "none",
			Headers: map[string]string{}, Variables: map[string]string{},
			Enabled: true, Rank: 100, PoolStrategy: sqlite.StrategyLeastLoaded,
		}); err != nil {
			t.Fatalf("seed provider: %v", err)
		}
		repo := sqlite.NewCredentialRepo(db)
		ctx := context.Background()
		errs := make(chan error, 20)
		var wg sync.WaitGroup
		for index := 0; index < 10; index++ {
			wg.Add(2)
			go func(index int) {
				defer wg.Done()
				errs <- repo.Insert(ctx, sqlite.CredentialRow{
					ID: fmt.Sprintf("cred-%d", index), ProviderID: "openai",
					Kind: "api_key", Status: "active",
				})
			}(index)
			go func() {
				defer wg.Done()
				_, err := repo.List(ctx)
				errs <- err
			}()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatalf("concurrent access error = %v", err)
			}
		}
		rows, err := repo.List(ctx)
		if err != nil {
			t.Fatalf("List() error = %v", err)
		}
		if len(rows) != 10 {
			t.Fatalf("List() returned %d credentials, want 10", len(rows))
		}
	})
}

func TestIsNetworkFS(t *testing.T) {
	remote, err := sqlite.IsNetworkFS(t.TempDir())
	if err != nil {
		t.Fatalf("IsNetworkFS() error = %v", err)
	}
	if remote {
		t.Skip("the temporary directory is on a network filesystem")
	}
	if _, err := sqlite.IsNetworkFS(filepath.Join(t.TempDir(), "absent")); err == nil {
		t.Fatal("IsNetworkFS() error = nil for a missing path, want an error")
	}
}

func tableExists(t *testing.T, db *sqlite.DB, table string) bool {
	t.Helper()
	var name string
	err := db.SQL().QueryRow("SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?", table).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		return false
	}
	if err != nil {
		t.Fatalf("query for table %s: %v", table, err)
	}
	return true
}

func pragmaText(t *testing.T, db *sqlite.DB, name string) string {
	t.Helper()
	var value string
	if err := db.SQL().QueryRow("PRAGMA " + name).Scan(&value); err != nil {
		t.Fatalf("read pragma %s: %v", name, err)
	}
	return value
}

func pragmaInt(t *testing.T, db *sqlite.DB, name string) int {
	t.Helper()
	var value int
	if err := db.SQL().QueryRow("PRAGMA " + name).Scan(&value); err != nil {
		t.Fatalf("read pragma %s: %v", name, err)
	}
	return value
}

// TestHealthFailureIsInspectable covers a failed integrity check with
// errors.Is, so a caller can tell a broken database from a closed one.
func TestHealthFailureIsInspectable(t *testing.T) {
	db := testkit.OpenTestDB(t)
	if err := db.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := db.Health(t.Context()); !errors.Is(err, sqlite.ErrIntegrity) {
		t.Fatalf("Health(closed) = %v, want ErrIntegrity", err)
	}
}
