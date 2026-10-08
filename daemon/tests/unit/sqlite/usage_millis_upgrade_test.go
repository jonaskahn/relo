package storage_test

import (
	"context"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	"github.com/jonaskahn/relo/internal/config"
	"github.com/jonaskahn/relo/tests/testkit"
)

// TestUsageTimestampUpgradeToMillis covers a state database written by a build
// that stored usage_events.timestamp in Unix seconds while every reader treated
// the column as milliseconds. The upgrade converts the stored rows so a date
// filter matches them, and adds the account column a fresh row can carry.
func TestUsageTimestampUpgradeToMillis(t *testing.T) {
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
	if len(migrations) < 10 {
		t.Fatalf("migrations = %d, want the ten this build ships", len(migrations))
	}
	if _, err := db.SQL().Exec("CREATE TABLE schema_migrations (" +
		"version INTEGER NOT NULL PRIMARY KEY, applied_at INTEGER NOT NULL, checksum TEXT NOT NULL) STRICT"); err != nil {
		t.Fatalf("create the migration table: %v", err)
	}
	// Everything up to the origin column, which is where this migration
	// begins.
	for _, migration := range migrations[:9] {
		if err := sqlite.ApplyMigration(db, migration); err != nil {
			t.Fatalf("ApplyMigration(%d) error = %v", migration.Version, err)
		}
	}
	ctx := context.Background()
	// One row in the old second-scale unit, and one already in milliseconds,
	// so the conversion is proven to skip a row it must not touch.
	seconds := int64(1_757_000_000)
	millis := int64(1_757_000_000_000)
	for _, stamp := range []int64{seconds, millis} {
		if _, err := db.SQL().ExecContext(ctx,
			"INSERT INTO usage_events (timestamp, provider, model, status, duration_ms, input_tokens, output_tokens) VALUES (?, ?, ?, ?, ?, ?, ?)",
			stamp, "openai", "gpt-4o", 200, 10, 5, 7); err != nil {
			t.Fatalf("seed the old schema: %v", err)
		}
	}

	if err := sqlite.Migrate(db); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	rows, err := db.SQL().QueryContext(ctx, "SELECT timestamp, credential_id FROM usage_events ORDER BY id")
	if err != nil {
		t.Fatalf("read the migrated rows: %v", err)
	}
	defer func() { _ = rows.Close() }()
	var stamps []int64
	for rows.Next() {
		var stamp int64
		var credential *string
		if err := rows.Scan(&stamp, &credential); err != nil {
			t.Fatalf("scan the migrated row: %v", err)
		}
		stamps = append(stamps, stamp)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate the migrated rows: %v", err)
	}
	if len(stamps) != 2 {
		t.Fatalf("rows = %d, want the two seeded", len(stamps))
	}
	if stamps[0] != millis {
		t.Fatalf("converted timestamp = %d, want %d", stamps[0], millis)
	}
	if stamps[1] != millis {
		t.Fatalf("millisecond timestamp = %d, want it left at %d", stamps[1], millis)
	}
}
