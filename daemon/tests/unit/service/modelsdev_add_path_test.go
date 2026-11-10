package service_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/adapters/discovery"
	"github.com/jonaskahn/relo/internal/adapters/modelsdev"
	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/tests/testkit"
)

// countingCatalog serves a models.dev document and counts every download of
// it, so a test can prove a path never asks the network.
func countingCatalog(t *testing.T, hits *atomic.Int64) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"deepseek":{"id":"deepseek","name":"DeepSeek","models":{}}}`)
	}))
	t.Cleanup(server.Close)
	return server
}

// addPathService is a harness whose models.dev source counts downloads and
// whose provider listing is served from this process.
func addPathService(t *testing.T, hits *atomic.Int64) (*harness, *session, string) {
	t.Helper()
	h := newHarness(t)
	listing := listingServer(t, http.StatusOK, `{"data":[{"id":"deepseek-chat"}]}`)
	client := &http.Client{Timeout: 5 * time.Second}
	catalogURL := countingCatalog(t, hits)
	return h, h.buildServiceWith(h.catalog, h.pools, testkit.SecretStore(h.secrets),
		modelsdev.NewDirectory(filepath.Join(h.home, "cache"), catalogURL.URL, client),
		discovery.New(client)), listing.URL
}

// TestAddingAConnectionNeverDownloadsModelsDev pins the rule that a download
// belongs to the schedule and the update button only: the add picker, the
// probe, a sign-in's connection, and a roster refresh all read the copy Relo
// already holds. A download here is what surfaced a cancelled tab as
// "Get https://models.dev/api.json: context canceled" and refused the add.
func TestAddingAConnectionNeverDownloadsModelsDev(t *testing.T) {
	var hits atomic.Int64
	h, s, listing := addPathService(t, &hits)

	if _, state, err := s.Templates(context.Background()); err != nil {
		t.Fatalf("Templates() error = %v", err)
	} else if state.LastError != "" {
		t.Fatalf("Templates() modelsdev error = %q, want none", state.LastError)
	}
	if _, err := s.EnsureProvider(context.Background(), "deepseek", false); err != nil {
		t.Fatalf("EnsureProvider() error = %v", err)
	}
	seedRow(t, h, sqlite.ProviderRow{
		ID: "deepseek", TemplateID: "deepseek", Origin: string(catalog.OriginTemplate),
		Label: "DeepSeek", Auth: string(catalog.AuthAPIKey),
		APIFormat: string(catalog.FormatOpenAIChat), BaseURL: listing + "/v1",
		ModelsFormat: string(catalog.ModelsOpenAI), ModelsDevProviderID: "deepseek",
		Enabled: true, Rank: 100,
	})
	if _, err := s.ProbeProvider(context.Background(), probeJSON(t,
		`{"template_id":"deepseek","base_url":"`+listing+`/v1","credential":{"kind":"api_key","secret":"sk-test"}}`)); err != nil {
		t.Fatalf("ProbeProvider() error = %v", err)
	}
	if _, err := s.RefreshProviderModels(context.Background(), "deepseek"); err != nil {
		t.Fatalf("RefreshProviderModels() error = %v", err)
	}

	if got := hits.Load(); got != 0 {
		t.Fatalf("models.dev downloads = %d, want 0 for an add path", got)
	}
}

// TestScheduledDownloadIsTheOnlyFetch pins the other half: the scheduled entry
// point does ask the network, and the copy it saves is what later reads serve.
func TestScheduledDownloadIsTheOnlyFetch(t *testing.T) {
	var hits atomic.Int64
	_, s, _ := addPathService(t, &hits)

	if err := s.FetchModelsDev(context.Background()); err != nil {
		t.Fatalf("FetchModelsDev() error = %v", err)
	}
	if got := hits.Load(); got != 1 {
		t.Fatalf("models.dev downloads = %d, want 1", got)
	}

	// A read after the download serves the saved copy without another request.
	if _, _, err := s.Templates(context.Background()); err != nil {
		t.Fatalf("Templates() error = %v", err)
	}
	if got := hits.Load(); got != 1 {
		t.Fatalf("models.dev downloads = %d after a read, want 1", got)
	}
}
