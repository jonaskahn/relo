package storage_test

import (
	"context"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	"github.com/jonaskahn/relo/tests/testkit"
)

// TestProviderUpstreamWaitRoundTrip covers the two nullable override columns:
// an unset connection reads NULL, a stored value comes back, and saving nil
// again clears it rather than writing a zero.
func TestProviderUpstreamWaitRoundTrip(t *testing.T) {
	ctx := context.Background()
	repo := sqlite.NewCatalogRepo(testkit.OpenTestDB(t))
	row := sqlite.ProviderRow{
		ID: "openai", Origin: "custom", Label: "OpenAI", Auth: "api_key",
	}
	if err := repo.SaveProvider(ctx, row); err != nil {
		t.Fatalf("SaveProvider() error = %v", err)
	}
	stored, err := repo.GetProvider(ctx, "openai")
	if err != nil {
		t.Fatalf("GetProvider() error = %v", err)
	}
	if stored.TimeoutSeconds != nil || stored.RetryAttempts != nil {
		t.Fatalf("stored = %+v, want unset overrides to read NULL", stored)
	}

	seconds := 30
	row.TimeoutSeconds = &seconds
	attempts := 5
	row.RetryAttempts = &attempts
	if err := repo.SaveProvider(ctx, row); err != nil {
		t.Fatalf("SaveProvider(override) error = %v", err)
	}
	stored, err = repo.GetProvider(ctx, "openai")
	if err != nil {
		t.Fatalf("GetProvider(override) error = %v", err)
	}
	if stored.TimeoutSeconds == nil || *stored.TimeoutSeconds != 30 {
		t.Fatalf("TimeoutSeconds = %v, want 30", stored.TimeoutSeconds)
	}
	if stored.RetryAttempts == nil || *stored.RetryAttempts != 5 {
		t.Fatalf("RetryAttempts = %v, want 5", stored.RetryAttempts)
	}

	row.TimeoutSeconds, row.RetryAttempts = nil, nil
	if err := repo.SaveProvider(ctx, row); err != nil {
		t.Fatalf("SaveProvider(clear) error = %v", err)
	}
	stored, err = repo.GetProvider(ctx, "openai")
	if err != nil {
		t.Fatalf("GetProvider(clear) error = %v", err)
	}
	if stored.TimeoutSeconds != nil || stored.RetryAttempts != nil {
		t.Fatalf("stored = %+v, want the cleared overrides to read NULL again", stored)
	}
}

func TestProviderAllowOverridesRoundTrip(t *testing.T) {
	ctx := context.Background()
	repo := sqlite.NewCatalogRepo(testkit.OpenTestDB(t))
	row := sqlite.ProviderRow{ID: "openai", Origin: "custom", Label: "OpenAI", Auth: "api_key"}
	if err := repo.SaveProvider(ctx, row); err != nil {
		t.Fatalf("SaveProvider() error = %v", err)
	}
	stored, err := repo.GetProvider(ctx, "openai")
	if err != nil {
		t.Fatalf("GetProvider() error = %v", err)
	}
	if stored.AllowOverrides {
		t.Fatal("a default provider allowed overrides")
	}
	row.AllowOverrides = true
	if err := repo.SaveProvider(ctx, row); err != nil {
		t.Fatalf("SaveProvider(override) error = %v", err)
	}
	stored, err = repo.GetProvider(ctx, "openai")
	if err != nil {
		t.Fatalf("GetProvider() error = %v", err)
	}
	if !stored.AllowOverrides {
		t.Fatal("AllowOverrides was not stored")
	}
	connections, err := repo.ListConnections(ctx)
	if err != nil {
		t.Fatalf("ListConnections() error = %v", err)
	}
	if len(connections) != 1 || !connections[0].AllowOverrides {
		t.Fatalf("ListConnections() = %+v, want the stored override", connections)
	}
}
