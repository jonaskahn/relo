package service_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"sync"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/cursor"
	"github.com/jonaskahn/relo/internal/adapters/discovery"
	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/tests/testkit"
)

// cursorListingCredentials authorizes each account with its own token, the
// way the daemon's credential source does for an OAuth connection.
type cursorListingCredentials struct {
	tokens map[string]string
}

func (c cursorListingCredentials) Authorize(_ context.Context, _ catalog.AuthRequest) (catalog.Authorization, error) {
	return catalog.Authorization{}, fmt.Errorf("no pool selection in this test")
}

func (c cursorListingCredentials) AuthorizeCredential(_ context.Context, _ catalog.AuthRequest, credentialID string) (catalog.Authorization, error) {
	token, found := c.tokens[credentialID]
	if !found {
		return catalog.Authorization{}, fmt.Errorf("unknown credential %s", credentialID)
	}
	return catalog.Authorization{CredentialID: credentialID, Auth: catalog.AuthOAuth, Token: token}, nil
}

// cursorListingServer serves the two Cursor model RPCs, answering the rich
// listing per account token so entitlements differ across the connection.
type cursorListingServer struct {
	t       *testing.T
	server  *httptest.Server
	mu      sync.Mutex
	broken  map[string]bool
	usable  map[string][]string
	variant map[string][]cursorFixtureVariant
}

type cursorFixtureVariant struct {
	params string
	slug   string
}

func newCursorListingServer(t *testing.T) *cursorListingServer {
	t.Helper()
	fixture := &cursorListingServer{
		t: t, broken: map[string]bool{},
		usable: map[string][]string{
			"tok-a": {"sonnet-low", "sonnet-high"},
			"tok-b": {"sonnet-medium"},
		},
		variant: map[string][]cursorFixtureVariant{
			"tok-a": {{params: "low", slug: "sonnet-low"}, {params: "high", slug: "sonnet-high"}},
			"tok-b": {{params: "medium", slug: "sonnet-medium"}},
		},
	}
	fixture.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := bearerToken(r.Header.Get("Authorization"))
		fixture.mu.Lock()
		broken := fixture.broken[token]
		fixture.mu.Unlock()
		if broken {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		switch r.URL.Path {
		case "/agent.v1.AgentService/GetUsableModels":
			writeCursorUsable(t, w, fixture.usableFor(token))
		case "/aiserver.v1.AiService/AvailableModels":
			writeCursorRich(t, w, fixture.variant[token])
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(fixture.server.Close)
	return fixture
}

func (f *cursorListingServer) usableFor(token string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.usable[token]
}

func (f *cursorListingServer) breakToken(token string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.broken[token] = true
}

func bearerToken(header string) string {
	const prefix = "Bearer "
	if len(header) <= len(prefix) {
		return ""
	}
	return header[len(prefix):]
}

func writeCursorUsable(t *testing.T, w http.ResponseWriter, ids []string) {
	t.Helper()
	var out cursor.Writer
	for _, id := range ids {
		var row cursor.Writer
		row.String(1, id)
		row.String(4, id)
		out.Message(1, row.BytesOut())
	}
	w.Header().Set("Content-Type", "application/proto")
	_, _ = w.Write(out.BytesOut())
}

func writeCursorRich(t *testing.T, w http.ResponseWriter, variants []cursorFixtureVariant) {
	t.Helper()
	var row cursor.Writer
	row.String(1, "sonnet")
	row.String(17, "Sonnet")
	row.Bool(5, true)
	row.Bool(9, true)
	row.Uint32(15, 200000)
	row.String(36, "sonnet-low")
	row.String(36, "sonnet-high")
	row.String(36, "sonnet-medium")
	for _, variant := range variants {
		var encoded cursor.Writer
		var pair cursor.Writer
		pair.String(1, "effort")
		pair.String(2, variant.params)
		encoded.Message(1, pair.BytesOut())
		encoded.String(11, variant.slug)
		row.Message(30, encoded.BytesOut())
	}
	var out cursor.Writer
	out.Message(2, row.BytesOut())
	w.Header().Set("Content-Type", "application/proto")
	_, _ = w.Write(out.BytesOut())
}

func cursorSelections(configs []catalog.ModelRequestConfig) []string {
	selections := make([]string, 0, len(configs))
	for _, config := range configs {
		selections = append(selections, config.ModelID+"/"+config.Selection)
	}
	sort.Strings(selections)
	return selections
}

func cursorRow(baseURL string) sqlite.ProviderRow {
	return sqlite.ProviderRow{
		ID: "cursor", TemplateID: "cursor", Origin: string(catalog.OriginSignIn),
		Label: "Cursor", Auth: string(catalog.AuthOAuth),
		APIFormat: string(catalog.FormatCursorAgent), KeyHeader: string(catalog.KeyHeaderBearer),
		BaseURL: baseURL, ModelsFormat: string(catalog.ModelsCursor),
		Enabled: true, Rank: 100,
	}
}

// TestCursorRefreshGroupsAccounts is the grouped roster rule: two accounts
// with different entitlements publish one lane with the union of efforts,
// while each account keeps its own exact request tuples for selection.
func TestCursorRefreshGroupsAccounts(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	listing := newCursorListingServer(t)
	seedRow(t, h, cursorRow(listing.server.URL))
	credA := h.addAccount("cursor", "a", `{"access_token":"tok-a","refresh_token":"r-a"}`)
	credB := h.addAccount("cursor", "b", `{"access_token":"tok-b","refresh_token":"r-b"}`)
	h.credentials = cursorListingCredentials{tokens: map[string]string{
		credA.ID: "tok-a", credB.ID: "tok-b",
	}}
	manager := h.buildServiceWith(h.catalog, h.pools, testkit.SecretStore(h.secrets), nil, discovery.New(nil))

	result, err := manager.RefreshProviderModels(ctx, "cursor")
	if err != nil {
		t.Fatalf("RefreshProviderModels() error = %v", err)
	}
	if result.Listed != 1 || result.Accounts != 2 {
		t.Fatalf("result = %+v, want one grouped lane across two accounts", result)
	}
	ids := modelIDsOf(t, manager, "cursor")
	found := false
	for _, id := range ids {
		if id == "sonnet" {
			found = true
		}
	}
	if !found {
		t.Fatalf("models = %v, want the grouped lane", ids)
	}
	model, err := manager.Model(ctx, "cursor", "sonnet")
	if err != nil {
		t.Fatalf("Model(sonnet) error = %v", err)
	}
	if model.Capabilities.Reasoning == nil || !*model.Capabilities.Reasoning {
		t.Fatalf("model = %+v, want reasoning stated", model)
	}
	efforts := providerEfforts(t, h, "cursor", "sonnet")
	if len(efforts) != 3 || efforts[0] != "high" || efforts[1] != "low" || efforts[2] != "medium" {
		t.Fatalf("efforts = %v, want the union across both accounts", efforts)
	}

	configs := sqlite.NewRequestConfigRepo(h.db)
	storedA, err := configs.Load(ctx, "cursor", credA.ID)
	if err != nil {
		t.Fatalf("Load(a) error = %v", err)
	}
	if got := cursorSelections(storedA); len(got) != 2 || got[0] != "sonnet/high" || got[1] != "sonnet/low" {
		t.Fatalf("account a = %v, want its own two tuples", got)
	}
	storedB, err := configs.Load(ctx, "cursor", credB.ID)
	if err != nil {
		t.Fatalf("Load(b) error = %v", err)
	}
	if got := cursorSelections(storedB); len(got) != 1 || got[0] != "sonnet/medium" {
		t.Fatalf("account b = %v, want its own tuple", got)
	}

	// A failed account listing keeps its stored tuples while the roster it
	// can no longer confirm leaves the union.
	listing.breakToken("tok-b")
	again, err := manager.RefreshProviderModels(ctx, "cursor")
	if err != nil {
		t.Fatalf("RefreshProviderModels() error = %v", err)
	}
	if len(again.AccountFailures) != 1 {
		t.Fatalf("result = %+v, want the failed account named", again)
	}
	kept, err := configs.Load(ctx, "cursor", credB.ID)
	if err != nil {
		t.Fatalf("Load(b) error = %v", err)
	}
	if got := cursorSelections(kept); len(got) != 1 || got[0] != "sonnet/medium" {
		t.Fatalf("account b = %v, want its tuples kept across the failure", got)
	}
	refreshed, err := manager.Model(ctx, "cursor", "sonnet")
	if err != nil {
		t.Fatalf("Model(sonnet) error = %v", err)
	}
	_ = refreshed
	if efforts := providerEfforts(t, h, "cursor", "sonnet"); len(efforts) != 2 {
		t.Fatalf("efforts = %v, want only what the answering account confirmed", efforts)
	}
}

func providerEfforts(t *testing.T, h *harness, providerID, modelID string) []string {
	t.Helper()
	layers, err := sqlite.NewCatalogRepo(h.db).ListModelFacts(context.Background(), providerID, modelID)
	if err != nil {
		t.Fatalf("ListModelFacts() error = %v", err)
	}
	for _, layer := range layers {
		if layer.Layer == "provider" {
			efforts := append([]string(nil), layer.ReasoningEfforts...)
			sort.Strings(efforts)
			return efforts
		}
	}
	t.Fatalf("no provider facts for %s", modelID)
	return nil
}
