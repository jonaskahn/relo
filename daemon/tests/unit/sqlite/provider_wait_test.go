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
	if stored.TimeoutSeconds != nil || stored.RetryBackoff != nil {
		t.Fatalf("stored = %+v, want unset overrides to read NULL", stored)
	}

	seconds := 30
	row.TimeoutSeconds = &seconds
	row.RetryBackoff = [][2]int{{2, 4}, {4, 6}, {6, 8}}
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
	if len(stored.RetryBackoff) != 3 || stored.RetryBackoff[0] != [2]int{2, 4} || stored.RetryBackoff[2] != [2]int{6, 8} {
		t.Fatalf("RetryBackoff = %v, want the stored windows", stored.RetryBackoff)
	}

	row.TimeoutSeconds, row.RetryBackoff = nil, nil
	if err := repo.SaveProvider(ctx, row); err != nil {
		t.Fatalf("SaveProvider(clear) error = %v", err)
	}
	stored, err = repo.GetProvider(ctx, "openai")
	if err != nil {
		t.Fatalf("GetProvider(clear) error = %v", err)
	}
	if stored.TimeoutSeconds != nil || stored.RetryBackoff != nil {
		t.Fatalf("stored = %+v, want the cleared overrides to read NULL again", stored)
	}
}
