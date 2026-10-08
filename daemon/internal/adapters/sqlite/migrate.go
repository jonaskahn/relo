// Schema migrations: applying them in order, once.
package sqlite

import (
	"crypto/sha256"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Migration errors name why the schema cannot open: a changed file, a
// failed step, a bad name, or a database from a newer build.
var (
	ErrChecksumMismatch = errors.New("migration checksum mismatch")
	ErrMigrationFailed  = errors.New("migration execution failed")
	ErrMigrationName    = errors.New("migration file name must start with a four-digit version")
	// ErrLegacySchema reports a state database whose migrations this build
	// does not carry. Relo has no upgrade path across that boundary, so the
	// operator learns to start from an empty state directory instead of
	// reading a half-migrated catalog.
	ErrLegacySchema = errors.New("state database schema is not supported")
)

const (
	migrationsDir      = "migrations"
	migrationExt       = ".sql"
	migrationNameParts = 2
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

// Migration is one numbered, checksummed schema migration.
type Migration struct {
	Version  int
	Name     string
	SQL      string
	Checksum string
}

// Migrate applies every pending migration in version order, each inside
// its own transaction. Reapplying an applied migration only verifies that
// its checksum is unchanged.
func Migrate(db *DB) error {
	if err := createMigrationTable(db); err != nil {
		return err
	}
	applied, err := appliedChecksums(db)
	if err != nil {
		return err
	}
	pending, err := LoadMigrations()
	if err != nil {
		return err
	}
	if err := checkLegacySchema(db, applied, pending); err != nil {
		return err
	}
	for _, migration := range pending {
		if err := applyOne(db, migration, applied[migration.Version]); err != nil {
			return err
		}
	}
	return db.harden()
}

func checkLegacySchema(db *DB, applied map[int]string, pending []Migration) error {
	known := make(map[int]bool, len(pending))
	for _, migration := range pending {
		known[migration.Version] = true
	}
	for version := range applied {
		if known[version] {
			continue
		}
		return fmt.Errorf("%s: %w (applied migration %d is not in this build; delete %s and its -wal and -shm files, then start Relo again)",
			db.path, ErrLegacySchema, version, db.path)
	}
	return nil
}

// ApplyMigration runs one migration inside its own transaction. A failed
// migration rolls back entirely, leaving the schema untouched.
func ApplyMigration(db *DB, migration Migration) error {
	return applyMigration(db, migration)
}

// LoadMigrations returns the migrations compiled into Relo, sorted by version.
func LoadMigrations() ([]Migration, error) {
	return LoadMigrationsFrom(migrationFiles)
}

// LatestSchemaVersion returns the version of the newest embedded
// migration, which is the schema a migrated database reports.
func LatestSchemaVersion() int {
	migrations, err := LoadMigrations()
	if err != nil || len(migrations) == 0 {
		return 0
	}
	return migrations[len(migrations)-1].Version
}

// LoadMigrationsFrom reads migrations from any file system laid out like
// the internal migrations directory, sorted by version.
func LoadMigrationsFrom(fsys fs.FS) ([]Migration, error) {
	entries, err := fs.ReadDir(fsys, migrationsDir)
	if err != nil {
		return nil, fmt.Errorf("read migrations: %w", err)
	}
	migrations := make([]Migration, 0, len(entries))
	for _, entry := range entries {
		migration, err := loadMigration(fsys, entry.Name())
		if err != nil {
			return nil, err
		}
		migrations = append(migrations, migration)
	}
	sort.Slice(migrations, func(i, j int) bool { return migrations[i].Version < migrations[j].Version })
	return migrations, nil
}

func loadMigration(fsys fs.FS, name string) (Migration, error) {
	version, err := parseVersion(name)
	if err != nil {
		return Migration{}, err
	}
	body, err := fs.ReadFile(fsys, migrationsDir+"/"+name)
	if err != nil {
		return Migration{}, fmt.Errorf("read migration %s: %w", name, err)
	}
	sum := sha256.Sum256(body)
	return Migration{
		Version:  version,
		Name:     name,
		SQL:      string(body),
		Checksum: fmt.Sprintf("%x", sum),
	}, nil
}

func parseVersion(name string) (int, error) {
	base := strings.TrimSuffix(name, migrationExt)
	prefix, _, found := strings.Cut(base, "_")
	if !found {
		return 0, fmt.Errorf("%s: %w", name, ErrMigrationName)
	}
	version, err := strconv.Atoi(prefix)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", name, ErrMigrationName)
	}
	return version, nil
}

func createMigrationTable(db *DB) error {
	const statement = `CREATE TABLE IF NOT EXISTS schema_migrations (
		version    INTEGER NOT NULL PRIMARY KEY,
		applied_at INTEGER NOT NULL,
		checksum   TEXT    NOT NULL
	) STRICT`
	if _, err := db.sql.Exec(statement); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}
	return nil
}

func appliedChecksums(db *DB) (map[int]string, error) {
	rows, err := db.sql.Query("SELECT version, checksum FROM schema_migrations")
	if err != nil {
		return nil, fmt.Errorf("read applied migrations: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanChecksums(rows)
}

func scanChecksums(rows *sql.Rows) (map[int]string, error) {
	applied := map[int]string{}
	for rows.Next() {
		var version int
		var checksum string
		if err := rows.Scan(&version, &checksum); err != nil {
			return nil, fmt.Errorf("scan applied migration: %w", err)
		}
		applied[version] = checksum
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate applied migrations: %w", err)
	}
	return applied, nil
}

func applyOne(db *DB, migration Migration, recorded string) error {
	switch {
	case recorded == "":
		return applyMigration(db, migration)
	case recorded != migration.Checksum:
		return fmt.Errorf("migration %s: %w (recorded %s, embedded %s; this state database was made by an older build; remove %s and its -wal and -shm files, then start Relo again)",
			migration.Name, ErrChecksumMismatch, recorded, migration.Checksum, db.path)
	default:
		return nil
	}
}

func applyMigration(db *DB, migration Migration) error {
	tx, err := db.sql.Begin()
	if err != nil {
		return fmt.Errorf("begin migration %s: %w", migration.Name, err)
	}
	if err := runMigration(tx, migration); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func runMigration(tx *sql.Tx, migration Migration) error {
	if _, err := tx.Exec(migration.SQL); err != nil {
		return fmt.Errorf("migration %s: %w: %v", migration.Name, ErrMigrationFailed, err)
	}
	const record = "INSERT INTO schema_migrations (version, applied_at, checksum) VALUES (?, ?, ?)"
	if _, err := tx.Exec(record, migration.Version, time.Now().Unix(), migration.Checksum); err != nil {
		return fmt.Errorf("record migration %s: %w", migration.Name, err)
	}
	return nil
}
