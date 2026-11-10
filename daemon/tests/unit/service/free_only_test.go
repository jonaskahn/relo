package service_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	appcatalog "github.com/jonaskahn/relo/internal/application/catalog"
	"github.com/jonaskahn/relo/internal/catalog"
)

// mutableListing serves a provider model list the test can change between a
// probe and a refresh.
type mutableListing struct {
	value atomic.Value
}

func (l *mutableListing) store(body string) {
	l.value.Store(body)
}

func (l *mutableListing) load() string {
	body, _ := l.value.Load().(string)
	return body
}

func (l *mutableListing) serve(t *testing.T) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(l.load()))
	}))
	t.Cleanup(server.Close)
	return server.URL
}

func listedIDs(snapshot *catalog.Snapshot) []string {
	var ids []string
	for _, model := range snapshot.Listed() {
		ids = append(ids, model.ProviderID+"/"+model.ID)
	}
	return ids
}

func containsID(ids []string, want string) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}

// routableSnapshot reloads the credential-aware catalog, the one that sees
// the pool the commit adopted its credential into, so the published model
// list reads the connection the way routing does. The harness catalog holds
// no live accounts and never marks a keyed connection configured.
func routableSnapshot(t *testing.T, h *harness) *catalog.Snapshot {
	t.Helper()
	credCatalog := h.newCredentialCatalog()
	if err := credCatalog.Reload(context.Background()); err != nil {
		t.Fatalf("Reload() error = %v", err)
	}
	snapshot, found := credCatalog.Snapshot()
	if !found {
		t.Fatal("Snapshot() found = false, want a loaded catalog")
	}
	return snapshot
}

func commitFreeOnlyProbe(t *testing.T, manager *session, baseURL string) string {
	t.Helper()
	result, err := manager.ProbeProvider(context.Background(), probeJSON(t,
		`{"template_id":"groq","base_url":"`+baseURL+`/v1","credential":{"kind":"api_key","secret":"sk-test"}}`))
	if err != nil {
		t.Fatalf("ProbeProvider() error = %v", err)
	}
	off := []string{"big-pickle"}
	host, err := manager.CommitProbe(context.Background(), result.ProbeID, appcatalog.CommitProbeRequest{
		ProviderID: "groq-free", Label: "Groq", DisabledModels: &off, FreeOnly: true,
	})
	if err != nil {
		t.Fatalf("CommitProbe() error = %v", err)
	}
	return host.ID
}

// TestCommitProbeStoresFreeOnly covers the durable flag behind Review's free
// mode: committing with paid models switched off stores the connection as
// free-only, and the published model list keeps the paid models out even when
// one is switched back on.
func TestCommitProbeStoresFreeOnly(t *testing.T) {
	h := newHarness(t)
	var listing mutableListing
	listing.store(`{"data":[{"id":"exo-free"},{"id":"big-pickle"}]}`)
	manager := probeService(t, h, catalogServer(t, map[string]map[string]any{"groq": {"id": "groq", "name": "Groq", "npm": "@ai-sdk/groq", "api": "https://api.groq.com/openai/v1", "models": map[string]any{}}}).URL)
	id := commitFreeOnlyProbe(t, manager, listing.serve(t))

	snapshot, found := h.catalog.Snapshot()
	if !found {
		t.Fatal("Snapshot() found = false, want a loaded catalog")
	}
	host, found := snapshot.Provider(id)
	if !found || !host.FreeOnly {
		t.Fatalf("stored connection = %+v, found %t, want free-only", host, found)
	}

	// The paid model is switched back on, the way an operator's toggle would,
	// and the published list still hides it: the flag decides, not the switch.
	if err := manager.ToggleModels(context.Background(), id, []string{"big-pickle"}, true); err != nil {
		t.Fatalf("ToggleModels() error = %v", err)
	}
	published := routableSnapshot(t, h)
	ids := listedIDs(published)
	if !containsID(ids, id+"/exo-free") {
		t.Fatalf("published models = %v, want the free model listed", ids)
	}
	if containsID(ids, id+"/big-pickle") {
		t.Fatalf("published models = %v, want the paid model hidden", ids)
	}
}

// TestRefreshDropsPaidModelsOnFreeOnlyConnection covers enforcement on every
// update: a refresh that lists a new paid model never stores it, and the paid
// rows already stored are marked unavailable so routing and the model list
// drop them.
func TestRefreshDropsPaidModelsOnFreeOnlyConnection(t *testing.T) {
	h := newHarness(t)
	var listing mutableListing
	listing.store(`{"data":[{"id":"exo-free"},{"id":"big-pickle"}]}`)
	manager := probeService(t, h, catalogServer(t, map[string]map[string]any{"groq": {"id": "groq", "name": "Groq", "npm": "@ai-sdk/groq", "api": "https://api.groq.com/openai/v1", "models": map[string]any{}}}).URL)
	id := commitFreeOnlyProbe(t, manager, listing.serve(t))

	listing.store(`{"data":[{"id":"exo-free"},{"id":"big-pickle"},{"id":"novel-paid"}]}`)
	refreshed, err := manager.RefreshProviderModels(context.Background(), id)
	if err != nil {
		t.Fatalf("RefreshProviderModels() error = %v", err)
	}
	if refreshed.Status != appcatalog.RefreshUpdated {
		t.Fatalf("refresh status = %q, want updated", refreshed.Status)
	}
	if refreshed.Added != 0 {
		t.Fatalf("added = %d, want the paid model left out of the roster", refreshed.Added)
	}

	store := routableSnapshot(t, h)
	if _, found := store.Model(id, "novel-paid"); found {
		t.Fatal("novel-paid was stored, want a free-only refresh to skip it")
	}
	paid, found := store.Model(id, "big-pickle")
	if !found || paid.Available == nil || *paid.Available {
		t.Fatalf("big-pickle = %+v, found %t, want it marked unavailable", paid, found)
	}
	ids := listedIDs(store)
	if !containsID(ids, id+"/exo-free") {
		t.Fatalf("published models = %v, want the free model listed", ids)
	}
	if containsID(ids, id+"/big-pickle") || containsID(ids, id+"/novel-paid") {
		t.Fatalf("published models = %v, want paid models hidden", ids)
	}

	// Re-applying the stored roster keeps the paid rows out as well.
	if _, err := manager.RefilterProviderModels(context.Background(), id); err != nil {
		t.Fatalf("RefilterProviderModels() error = %v", err)
	}
	filtered, found := h.catalog.Snapshot()
	if !found {
		t.Fatal("Snapshot() found = false, want a loaded catalog")
	}
	if model, found := filtered.Model(id, "big-pickle"); !found || model.Available == nil || *model.Available {
		t.Fatalf("big-pickle after refilter = %+v, found %t, want it unavailable", model, found)
	}
}
