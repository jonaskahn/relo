package storage_test

import (
	"context"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	"github.com/jonaskahn/relo/internal/config"
	"github.com/jonaskahn/relo/tests/testkit"
)

// TestCaptureUpgradeAddsTheRawLog covers a state database written before
// request capture existed: the upgrade adds the capture and archive tables,
// gives the attempt log the account column, and counts the attempts a request
// already made, so a rollup reads a history that predates the feature.
func TestCaptureUpgradeAddsTheRawLog(t *testing.T) {
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
	if len(migrations) < 12 {
		t.Fatalf("migrations = %d, want at least the twelve this build ships", len(migrations))
	}
	if _, err := db.SQL().Exec("CREATE TABLE schema_migrations (" +
		"version INTEGER NOT NULL PRIMARY KEY, applied_at INTEGER NOT NULL, checksum TEXT NOT NULL) STRICT"); err != nil {
		t.Fatalf("create the migration table: %v", err)
	}
	// Everything before the capture migration, which is where this one begins.
	for _, migration := range migrations[:11] {
		if err := sqlite.ApplyMigration(db, migration); err != nil {
			t.Fatalf("ApplyMigration(%d) error = %v", migration.Version, err)
		}
	}

	ctx := context.Background()
	at := time.Date(2026, 3, 4, 10, 0, 0, 0, time.UTC).UnixMilli()
	result, err := db.SQL().ExecContext(ctx,
		"INSERT INTO usage_events (request_id, timestamp, provider, model, status, duration_ms, input_tokens, output_tokens) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		"old-request", at, "openai", "gpt-4o", 200, 25, 9, 4)
	if err != nil {
		t.Fatalf("seed the old schema: %v", err)
	}
	eventID, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("read the seeded id: %v", err)
	}
	for ordinal := 0; ordinal < 2; ordinal++ {
		if _, err := db.SQL().ExecContext(ctx,
			"INSERT INTO usage_attempts (event_id, ordinal, provider, model, credential_label, status, duration_ms) VALUES (?, ?, ?, ?, ?, ?, ?)",
			eventID, ordinal, "openai", "gpt-4o", "default", 200, 12); err != nil {
			t.Fatalf("seed the old attempt: %v", err)
		}
	}

	if err := sqlite.Migrate(db); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	var attempts, retried int
	if err := db.SQL().QueryRowContext(ctx,
		"SELECT attempts, retried FROM usage_events WHERE id = ?", eventID).Scan(&attempts, &retried); err != nil {
		t.Fatalf("read the counted attempts: %v", err)
	}
	if attempts != 2 || retried != 1 {
		t.Fatalf("attempts = %d, retried = %d, want a retried request counted from its attempts", attempts, retried)
	}

	// The capture store the upgrade added writes what the log's raw view reads.
	store := sqlite.NewCaptureStore(db)
	ordinal := 1
	if err := store.Append(ctx, eventID, []sqlite.Capture{{
		Ordinal: &ordinal, Kind: sqlite.CaptureProviderRequest, Method: "POST",
		URL:     "https://api.example.test/v1/chat/completions",
		Headers: map[string][]string{"Authorization": {"Bearer sk-old"}},
		Body:    []byte(`{"model":"gpt-4o"}`),
	}}); err != nil {
		t.Fatalf("Append() error = %v", err)
	}
	manifest, err := store.Manifest(ctx, eventID)
	if err != nil {
		t.Fatalf("Manifest() error = %v", err)
	}
	if len(manifest) != 1 || manifest[0].Kind != sqlite.CaptureProviderRequest {
		t.Fatalf("manifest = %+v, want the one stored capture", manifest)
	}
	body, err := store.Body(ctx, eventID, manifest[0].ID)
	if err != nil {
		t.Fatalf("Body() error = %v", err)
	}
	if string(body.Body) != `{"model":"gpt-4o"}` {
		t.Fatalf("body = %q, want the stored request", body.Body)
	}

	// The archive the upgrade added closes the day of the old row, so its
	// totals outlive the row itself.
	archive := sqlite.NewArchive(db)
	archive.SetClock(func() time.Time { return time.UnixMilli(at).Add(24 * time.Hour) })
	if _, err := archive.Finalize(ctx); err != nil {
		t.Fatalf("Finalize() error = %v", err)
	}
	var requests, tokens int64
	if err := db.SQL().QueryRowContext(ctx,
		"SELECT COALESCE(SUM(requests), 0), COALESCE(SUM(input_tokens), 0) FROM usage_daily WHERE day = ?",
		"2026-03-04").Scan(&requests, &tokens); err != nil {
		t.Fatalf("read the archived day: %v", err)
	}
	if requests != 1 || tokens != 9 {
		t.Fatalf("archived day = %d requests, %d input tokens, want the old row counted once", requests, tokens)
	}
}
