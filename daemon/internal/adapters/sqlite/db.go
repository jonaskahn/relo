// Package sqlite is the storage adapter: it owns the database handle,
// migrations, and every query Relo runs.
package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	_ "modernc.org/sqlite"
)

// Database errors name the startup refusals an operator must fix: a remote
// home, a journal or foreign-key mode that is off, a missing row, and an
// integrity check that failed.
var (
	ErrNetworkFS            = errors.New("relo requires RELO_HOME on a local disk")
	ErrWALNotActive         = errors.New("WAL journal mode not active")
	ErrForeignKeysNotActive = errors.New("foreign key enforcement not active")
	ErrCredentialNotFound   = errors.New("credential not found")
	ErrIntegrity            = errors.New("check database integrity")
)

const (
	dsnParams    = "?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(ON)&_pragma=busy_timeout(5000)"
	readOnlyDSN  = "?mode=ro&_pragma=foreign_keys(ON)&_pragma=busy_timeout(5000)"
	databaseMode = 0o600
	sidecarWal   = "-wal"
	sidecarShm   = "-shm"
	sqliteDriver = "sqlite"
)

// DB wraps a sql.DB with the Relo-specific lifecycle.
type DB struct {
	sql    *sql.DB
	path   string
	logger *slog.Logger
	// maintenance serializes retention runs, so a save, a startup pass, and
	// the background worker never delete or compact at the same time.
	maintenance sync.Mutex
}

// OpenDB opens or creates the SQLite database at path, applies the pragmas
// Relo depends on, and refuses to run on a network filesystem.
func OpenDB(path string, logger *slog.Logger) (*DB, error) {
	remote, err := IsNetworkFS(filepath.Dir(path))
	if err != nil {
		return nil, fmt.Errorf("inspect filesystem for %s: %w", path, err)
	}
	if remote {
		return nil, fmt.Errorf("%s: %w", path, ErrNetworkFS)
	}
	handle, err := sql.Open(sqliteDriver, path+dsnParams)
	if err != nil {
		return nil, fmt.Errorf("open database %s: %w", path, err)
	}
	db := &DB{sql: handle, path: path, logger: logger}
	if err := db.verify(); err != nil {
		_ = handle.Close()
		return nil, err
	}
	return db, db.harden()
}

// OpenReadOnly opens an existing Relo database without creating or changing it.
func OpenReadOnly(path string, logger *slog.Logger) (*DB, error) {
	remote, err := IsNetworkFS(filepath.Dir(path))
	if err != nil {
		return nil, fmt.Errorf("inspect filesystem for %s: %w", path, err)
	}
	if remote {
		return nil, fmt.Errorf("%s: %w", path, ErrNetworkFS)
	}
	handle, err := sql.Open(sqliteDriver, "file:"+filepath.ToSlash(path)+readOnlyDSN)
	if err != nil {
		return nil, fmt.Errorf("open database %s read-only: %w", path, err)
	}
	db := &DB{sql: handle, path: path, logger: logger}
	if err := db.verify(); err != nil {
		_ = handle.Close()
		return nil, err
	}
	return db, nil
}

// Close releases the database handle. Closing a nil or closed handle is safe.
func (db *DB) Close() error {
	if db == nil || db.sql == nil {
		return nil
	}
	if err := db.sql.Close(); err != nil {
		return fmt.Errorf("close database %s: %w", db.path, err)
	}
	return nil
}

// SQL returns the underlying handle for queries issued by this package.
func (db *DB) SQL() *sql.DB {
	return db.sql
}

// Health runs the storage engine's own integrity check, so nothing outside
// this package has to know how the database is asked.
func (db *DB) Health(ctx context.Context) error {
	var result string
	if err := db.sql.QueryRowContext(ctx, "PRAGMA quick_check").Scan(&result); err != nil {
		return fmt.Errorf("%w: %w", ErrIntegrity, err)
	}
	if result != "ok" {
		return fmt.Errorf("%w: %s", ErrIntegrity, result)
	}
	return nil
}

// Path returns the file backing this database.
func (db *DB) Path() string {
	return db.path
}

// SchemaVersion returns the highest applied migration version, or zero
// when no migration has run yet.
func (db *DB) SchemaVersion() (int, error) {
	var version sql.NullInt64
	if err := db.sql.QueryRow("SELECT max(version) FROM schema_migrations").Scan(&version); err != nil {
		return 0, fmt.Errorf("read schema version of %s: %w", db.path, err)
	}
	return int(version.Int64), nil
}

func (db *DB) verify() error {
	if err := db.sql.Ping(); err != nil {
		return fmt.Errorf("connect to %s: %w", db.path, err)
	}
	var mode string
	if err := db.sql.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
		return fmt.Errorf("read journal_mode of %s: %w", db.path, err)
	}
	if !strings.EqualFold(mode, "wal") {
		return fmt.Errorf("%s: %w (got %s)", db.path, ErrWALNotActive, mode)
	}
	return db.verifyForeignKeys()
}

func (db *DB) verifyForeignKeys() error {
	var enabled int
	if err := db.sql.QueryRow("PRAGMA foreign_keys").Scan(&enabled); err != nil {
		return fmt.Errorf("read pragma foreign_keys: %w", err)
	}
	if enabled != 1 {
		return fmt.Errorf("%s: %w", db.path, ErrForeignKeysNotActive)
	}
	return nil
}

func (db *DB) harden() error {
	for _, path := range []string{db.path, db.path + sidecarWal, db.path + sidecarShm} {
		if err := hardenFile(path); err != nil {
			return err
		}
	}
	return nil
}

func hardenFile(path string) error {
	if err := os.Chmod(path, databaseMode); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("harden %s: %w", path, err)
	}
	return nil
}
