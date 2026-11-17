package storage_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	appproxy "github.com/jonaskahn/relo/internal/proxy"
)

// TestProxyStoreReportsAClosedDatabase covers every query's failure path: a
// store over a closed database returns the error rather than an empty answer,
// so a broken pool reports a broken pool.
func TestProxyStoreReportsAClosedDatabase(t *testing.T) {
	ctx := context.Background()
	db := openProxyDB(t)
	store := sqlite.NewProxyStore(db)
	entry := appproxy.Entry{
		ID: "p1", Label: "one", Scheme: "http", Host: "127.0.0.1", Port: 7890, Enabled: true,
	}
	if _, err := store.Create(ctx, entry); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if _, err := store.List(ctx); err == nil {
		t.Fatal("List() on a closed database error = nil, want the failure")
	}
	if _, _, err := store.Get(ctx, "p1"); err == nil {
		t.Fatal("Get() on a closed database error = nil, want the failure")
	}
	if _, err := store.Create(ctx, entry); err == nil {
		t.Fatal("Create() on a closed database error = nil, want the failure")
	}
	if err := store.Update(ctx, entry); err == nil {
		t.Fatal("Update() on a closed database error = nil, want the failure")
	}
	if err := store.Delete(ctx, "p1"); err == nil {
		t.Fatal("Delete() on a closed database error = nil, want the failure")
	}
	if _, err := store.Count(ctx); err == nil {
		t.Fatal("Count() on a closed database error = nil, want the failure")
	}
}

// TestProxyStoreRefusesAnAddressItDoesNotHold covers the affected-row check:
// updating or deleting an id nothing stores is the pool's own not-found error,
// not a silent success.
func TestProxyStoreRefusesAnAddressItDoesNotHold(t *testing.T) {
	ctx := context.Background()
	store := sqlite.NewProxyStore(openProxyDB(t))
	missing := appproxy.Entry{ID: "gone", Label: "x", Scheme: "http", Host: "127.0.0.1", Port: 1}
	if err := store.Update(ctx, missing); !errors.Is(err, appproxy.ErrNotFound) {
		t.Fatalf("Update(missing) error = %v, want ErrNotFound", err)
	}
	if err := store.Delete(ctx, "gone"); !errors.Is(err, appproxy.ErrNotFound) {
		t.Fatalf("Delete(missing) error = %v, want ErrNotFound", err)
	}
	if _, found, err := store.Get(ctx, "gone"); err != nil || found {
		t.Fatalf("Get(missing) = found %v, %v", found, err)
	}
}

// TestProxyStoreRejectsADuplicateID covers the primary-key refusal, which keeps
// two rows from claiming the same secret reference.
func TestProxyStoreRejectsADuplicateID(t *testing.T) {
	ctx := context.Background()
	store := sqlite.NewProxyStore(openProxyDB(t))
	entry := appproxy.Entry{ID: "p1", Label: "one", Scheme: "http", Host: "127.0.0.1", Port: 7890}
	if _, err := store.Create(ctx, entry); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if _, err := store.Create(ctx, entry); err == nil || !strings.Contains(err.Error(), "create proxy") {
		t.Fatalf("Create(duplicate) error = %v, want the wrapped refusal", err)
	}
}
