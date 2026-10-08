package storage_test

import (
	"errors"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	"github.com/jonaskahn/relo/tests/testkit"
)

func TestLoadMigrationsFrom(t *testing.T) {
	t.Run("reads and sorts migrations", func(t *testing.T) {
		fsys := fstest.MapFS{
			"migrations/0002_second.sql": {Data: []byte("SELECT 2")},
			"migrations/0001_first.sql":  {Data: []byte("SELECT 1")},
		}
		migrations, err := sqlite.LoadMigrationsFrom(fsys)
		if err != nil {
			t.Fatalf("LoadMigrationsFrom() error = %v", err)
		}
		if len(migrations) != 2 {
			t.Fatalf("got %d migrations, want 2", len(migrations))
		}
		if migrations[0].Name != "0001_first.sql" || migrations[1].Name != "0002_second.sql" {
			t.Fatalf("order = %q, %q, want version order", migrations[0].Name, migrations[1].Name)
		}
		if migrations[1].Version != 2 || migrations[1].SQL != "SELECT 2" || len(migrations[1].Checksum) != 64 {
			t.Fatalf("second migration = %+v, want version 2 with a digest", migrations[1])
		}
	})

	t.Run("missing directory reports an error", func(t *testing.T) {
		if _, err := sqlite.LoadMigrationsFrom(fstest.MapFS{}); err == nil {
			t.Fatal("LoadMigrationsFrom() error = nil, want a read failure")
		}
	})

	t.Run("unnamed migration without a number is rejected", func(t *testing.T) {
		fsys := fstest.MapFS{"migrations/foundation.sql": {Data: []byte("SELECT 1")}}
		_, err := sqlite.LoadMigrationsFrom(fsys)
		if !errors.Is(err, sqlite.ErrMigrationName) {
			t.Fatalf("LoadMigrationsFrom() error = %v, want %v", err, sqlite.ErrMigrationName)
		}
	})

	t.Run("unnamed migration with a non-numeric prefix is rejected", func(t *testing.T) {
		fsys := fstest.MapFS{"migrations/latest_foundation.sql": {Data: []byte("SELECT 1")}}
		_, err := sqlite.LoadMigrationsFrom(fsys)
		if !errors.Is(err, sqlite.ErrMigrationName) {
			t.Fatalf("LoadMigrationsFrom() error = %v, want %v", err, sqlite.ErrMigrationName)
		}
	})

	t.Run("unreadable migration reports an error", func(t *testing.T) {
		fsys := unreadableFS{inner: fstest.MapFS{
			"migrations/0001_locked.sql": {Data: []byte("SELECT 1")},
		}}
		if _, err := sqlite.LoadMigrationsFrom(fsys); err == nil {
			t.Fatal("LoadMigrationsFrom() error = nil, want a permission failure")
		}
	})
}

func TestMigrateFailureModes(t *testing.T) {
	t.Run("Migrate refuses a closed database", func(t *testing.T) {
		db := testkit.OpenTestDB(t)
		if err := db.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
		if err := sqlite.Migrate(db); err == nil {
			t.Fatal("Migrate() on a closed database error = nil, want a failure")
		}
	})

	t.Run("a migration that breaks its own bookkeeping rolls back", func(t *testing.T) {
		db := testkit.OpenTestDB(t)
		sneaky := sqlite.Migration{
			Version:  5,
			Name:     "0005_sneaky.sql",
			SQL:      "DROP TABLE schema_migrations",
			Checksum: "sneaky",
		}
		if err := sqlite.ApplyMigration(db, sneaky); err == nil {
			t.Fatal("ApplyMigration() error = nil, want a bookkeeping failure")
		}
		if !tableExists(t, db, "schema_migrations") {
			t.Fatal("the rollback did not restore schema_migrations")
		}
	})

	t.Run("Close on a nil handle is safe", func(t *testing.T) {
		var db *sqlite.DB
		if err := db.Close(); err != nil {
			t.Fatalf("Close() on a nil database = %v, want nil", err)
		}
	})
}

// unreadableFS denies reading locked migrations, which is how a
// permission problem surfaces in production. It deliberately exposes no
// bulk read, so the per-file open path is exercised.
type unreadableFS struct {
	inner fstest.MapFS
}

func (f unreadableFS) ReadDir(name string) ([]fs.DirEntry, error) {
	return f.inner.ReadDir(name)
}

func (f unreadableFS) Open(name string) (fs.File, error) {
	if strings.Contains(name, "locked") {
		return nil, fs.ErrPermission
	}
	return f.inner.Open(name)
}
