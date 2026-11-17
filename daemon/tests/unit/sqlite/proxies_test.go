package storage_test

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	appproxy "github.com/jonaskahn/relo/internal/proxy"
)

func openProxyDB(t *testing.T) *sqlite.DB {
	t.Helper()
	db, err := sqlite.OpenDB(filepath.Join(t.TempDir(), "relo.db"), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlite.Migrate(db); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestProxyStoreRoundTripKeepsSecretsOutOfSQLite(t *testing.T) {
	ctx := context.Background()
	store := sqlite.NewProxyStore(openProxyDB(t))
	created, err := store.Create(ctx, appproxy.Entry{
		ID: "p1", Label: "one", Scheme: "http", Host: "127.0.0.1", Port: 7890,
		Username: "alice", SecretRef: "proxy/p1", Enabled: true, Rank: 2,
		CreatedAt: 1, UpdatedAt: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.SecretRef != "proxy/p1" {
		t.Fatalf("SecretRef = %q", created.SecretRef)
	}
	rows, err := store.List(ctx)
	if err != nil || len(rows) != 1 {
		t.Fatalf("List() = %+v, %v", rows, err)
	}
	if rows[0].Username != "alice" || rows[0].Port != 7890 || !rows[0].Enabled {
		t.Fatalf("row = %+v", rows[0])
	}
	if err := store.Update(ctx, appproxy.Entry{
		ID: "p1", Label: "renamed", Scheme: "socks5", Host: "127.0.0.1", Port: 1080,
		Enabled: false, Rank: 1, CreatedAt: 1, UpdatedAt: 2,
	}); err != nil {
		t.Fatal(err)
	}
	row, found, err := store.Get(ctx, "p1")
	if err != nil || !found || row.Label != "renamed" || row.Scheme != "socks5" || row.Enabled {
		t.Fatalf("Get() = %+v, %v, %v", row, found, err)
	}
	if err := store.Delete(ctx, "p1"); err != nil {
		t.Fatal(err)
	}
	if count, err := store.Count(ctx); err != nil || count != 0 {
		t.Fatalf("Count() = %d, %v", count, err)
	}
}
