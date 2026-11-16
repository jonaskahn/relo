package storage_test

import (
	"context"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	"github.com/jonaskahn/relo/tests/testkit"
)

func TestDocumentsRoundTripWithinANamespace(t *testing.T) {
	ctx := context.Background()
	store := sqlite.NewDocuments(testkit.OpenTestDB(t))
	if err := store.Put(ctx, "test.notes", sqlite.DocumentRow{ID: "a", Version: 2, Body: []byte(`{"x":1}`)}, 10); err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	if err := store.Put(ctx, "test.notes", sqlite.DocumentRow{ID: "a", Version: 3, Body: []byte(`{"x":2}`)}, 20); err != nil {
		t.Fatalf("Put() replace error = %v", err)
	}
	if err := store.Put(ctx, "test.other", sqlite.DocumentRow{ID: "a", Version: 1, Body: []byte(`{}`)}, 30); err != nil {
		t.Fatalf("Put() other namespace error = %v", err)
	}
	rows, err := store.List(ctx, "test.notes")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(rows) != 1 || rows[0].Version != 3 || string(rows[0].Body) != `{"x":2}` {
		t.Fatalf("List() = %+v, want the replaced document of this namespace only", rows)
	}
	if err := store.Delete(ctx, "test.notes", "a"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if err := store.Delete(ctx, "test.notes", "missing"); err != nil {
		t.Fatalf("Delete() of a missing document error = %v", err)
	}
	if rows, _ := store.List(ctx, "test.notes"); len(rows) != 0 {
		t.Fatalf("List() after Delete = %+v", rows)
	}
	if rows, _ := store.List(ctx, "test.other"); len(rows) != 1 {
		t.Fatalf("Delete() reached another namespace: %+v", rows)
	}
}

func TestDocumentsRefuseABodyThatIsNotJSON(t *testing.T) {
	store := sqlite.NewDocuments(testkit.OpenTestDB(t))
	err := store.Put(context.Background(), "test.notes", sqlite.DocumentRow{ID: "a", Version: 1, Body: []byte(`{not json`)}, 1)
	if err == nil {
		t.Fatal("Put() error = nil, want json_valid to refuse the body")
	}
}
