package storage_test

import (
	"context"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	"github.com/jonaskahn/relo/tests/testkit"
)

// TestProviderFreeOnlyRoundTrip covers the free_only column the commit path
// writes: a free-only connection reads back as one, and clearing it reads
// back as a paid connection.
func TestProviderFreeOnlyRoundTrip(t *testing.T) {
	db := testkit.OpenTestDB(t)
	repo := sqlite.NewCatalogRepo(db)
	ctx := context.Background()

	row := sqlite.ProviderRow{
		ID: "openai", Origin: "custom", Label: "OpenAI", Auth: "api_key", FreeOnly: true,
	}
	if err := repo.SaveProvider(ctx, row); err != nil {
		t.Fatalf("SaveProvider() error = %v", err)
	}
	got, err := repo.GetProvider(ctx, "openai")
	if err != nil {
		t.Fatalf("GetProvider() error = %v", err)
	}
	if !got.FreeOnly {
		t.Fatal("FreeOnly = false, want the stored free-only flag")
	}
	listed, err := repo.ListProviders(ctx)
	if err != nil {
		t.Fatalf("ListProviders() error = %v", err)
	}
	found := false
	for _, entry := range listed {
		if entry.ID == "openai" {
			found = true
			if !entry.FreeOnly {
				t.Fatal("ListProviders() lost the free-only flag")
			}
		}
	}
	if !found {
		t.Fatal("ListProviders() is missing the stored connection")
	}

	row.FreeOnly = false
	if err := repo.SaveProvider(ctx, row); err != nil {
		t.Fatalf("SaveProvider() error = %v", err)
	}
	cleared, err := repo.GetProvider(ctx, "openai")
	if err != nil {
		t.Fatalf("GetProvider() error = %v", err)
	}
	if cleared.FreeOnly {
		t.Fatal("FreeOnly = true, want the cleared flag to read back")
	}
}

// TestProviderFreeOnlyDefaultsOff covers the migration default: a row written
// before the column existed reads as a paid connection.
func TestProviderFreeOnlyDefaultsOff(t *testing.T) {
	db := testkit.OpenTestDB(t)
	ctx := context.Background()
	if _, err := db.SQL().ExecContext(ctx,
		`INSERT INTO providers (id, origin, label, auth, created_at, updated_at)
		 VALUES ('legacy', 'custom', 'Legacy', 'api_key', 1, 1)`); err != nil {
		t.Fatalf("insert a legacy provider: %v", err)
	}
	got, err := sqlite.NewCatalogRepo(db).GetProvider(ctx, "legacy")
	if err != nil {
		t.Fatalf("GetProvider() error = %v", err)
	}
	if got.FreeOnly {
		t.Fatal("FreeOnly = true, want a legacy row to default to paid")
	}
}
