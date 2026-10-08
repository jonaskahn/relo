package testkit

import (
	"path/filepath"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/sqlite"
)

// OpenTestDB opens a temporary database with every migration applied. It
// accepts a benchmark as well as a test, so scale work can reuse it.
func OpenTestDB(t testing.TB) *sqlite.DB {
	t.Helper()
	logger, _ := TestLogger(t)
	db, err := sqlite.OpenDB(filepath.Join(t.TempDir(), "state.sqlite"), logger)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	if err := sqlite.Migrate(db); err != nil {
		t.Fatalf("migrate test database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}
