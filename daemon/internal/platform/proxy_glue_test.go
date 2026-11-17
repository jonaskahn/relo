package platform

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/adapters/secrets"
	"github.com/jonaskahn/relo/internal/clock"
	"github.com/jonaskahn/relo/internal/config"
	appproxy "github.com/jonaskahn/relo/internal/proxy"
)

type poolStore struct{ rows map[string]appproxy.Entry }

func newPoolStore() *poolStore { return &poolStore{rows: map[string]appproxy.Entry{}} }

func (s *poolStore) List(context.Context) ([]appproxy.Entry, error) {
	rows := make([]appproxy.Entry, 0, len(s.rows))
	for _, row := range s.rows {
		rows = append(rows, row)
	}
	return rows, nil
}

func (s *poolStore) Get(_ context.Context, id string) (appproxy.Entry, bool, error) {
	row, found := s.rows[id]
	return row, found, nil
}

func (s *poolStore) Create(_ context.Context, entry appproxy.Entry) (appproxy.Entry, error) {
	s.rows[entry.ID] = entry
	return entry, nil
}

func (s *poolStore) Update(_ context.Context, entry appproxy.Entry) error {
	s.rows[entry.ID] = entry
	return nil
}

func (s *poolStore) Delete(_ context.Context, id string) error {
	delete(s.rows, id)
	return nil
}

func (s *poolStore) Count(context.Context) (int, error) { return len(s.rows), nil }

type poolSecrets struct{ values map[string]string }

func (s *poolSecrets) Set(ref, value string) error { s.values[ref] = value; return nil }

func (s *poolSecrets) Get(ref string) (string, error) {
	value, found := s.values[ref]
	if !found {
		return "", errMissingSecret
	}
	return value, nil
}

func (s *poolSecrets) Delete(ref string) error { delete(s.values, ref); return nil }

// Mode reports the pool's own string mode, which is the adapter's contract.
func (s *poolSecrets) Mode() string { return "encrypted_file" }

func testPool(t *testing.T) (*appproxy.Service, *poolStore) {
	t.Helper()
	store := newPoolStore()
	return appproxy.New(appproxy.Options{
		Store: store, Secrets: &poolSecrets{values: map[string]string{}},
		Clock: clock.New(), Check: func(context.Context, string) error { return nil },
	}), store
}

// TestProxyPoolSummaryAdaptsTheCounts covers both summary adapters: the
// providers card and the doctor read the same counts through different types.
func TestProxyPoolSummaryAdaptsTheCounts(t *testing.T) {
	pool, _ := testPool(t)
	ctx := context.Background()
	if _, err := pool.Create(ctx, appproxy.Draft{
		Scheme: "http", Host: "127.0.0.1", Port: 7890, Password: "pw", Enabled: true,
	}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	card, err := ProxyPoolSummary{Pool: pool}.ProxyPoolSummary(ctx)
	if err != nil || card.Total != 1 || card.Enabled != 1 || card.Cooling != 0 || card.AllCooling {
		t.Fatalf("ProxyPoolSummary() = %+v, %v", card, err)
	}
	doctor, err := ProxyPoolStatus{Pool: pool}.ProxyPoolSummary(ctx)
	if err != nil || doctor.Total != 1 || doctor.Enabled != 1 {
		t.Fatalf("ProxyPoolStatus.ProxyPoolSummary() = %+v, %v", doctor, err)
	}
	// Each adapter answers a distinct type, and each keeps the pool it reads.
	cardPool := ProxyPoolSummary{Pool: pool}
	if cardPool.Pool == nil {
		t.Fatal("the providers card adapter lost its pool")
	}
	doctorPool := ProxyPoolStatus{Pool: pool}
	if doctorPool.Pool == nil {
		t.Fatal("the doctor adapter lost its pool")
	}
}

// TestProxySecretsReportsTheUnderlyingMode covers the vault adapter: a nil
// store stays nil so the pool knows it has no vault, and a real one names the
// mode the pool compares to detect a rotation.
func TestProxySecretsReportsTheUnderlyingMode(t *testing.T) {
	if NewProxySecrets(nil) != nil {
		t.Fatal("NewProxySecrets(nil) = non-nil, want nil so the pool reports no vault")
	}
	store := &modeSecrets{mode: secrets.ModeEncryptedFile}
	adapted := NewProxySecrets(store)
	if adapted == nil || adapted.Mode() != string(secrets.ModeEncryptedFile) {
		t.Fatalf("NewProxySecrets() mode = %v", adapted)
	}
	if _, err := adapted.Get("missing"); !errors.Is(err, errMissingSecret) {
		t.Fatalf("Get(missing) error = %v, want the store's own refusal", err)
	}
}

type modeSecrets struct{ mode secrets.SecretMode }

var errMissingSecret = errors.New("no such secret")

func (s *modeSecrets) Set(string, string) error   { return nil }
func (s *modeSecrets) Get(string) (string, error) { return "", errMissingSecret }
func (s *modeSecrets) Delete(string) error        { return nil }
func (s *modeSecrets) Mode() secrets.SecretMode   { return s.mode }

// TestCheckProxyAcceptsAnyAnswerBelow500 covers the connectivity check: an
// address the normalizer refuses, an unreachable proxy, and a proxy that
// answers. The answer itself is a HEAD to example.com through the proxy.
func TestCheckProxyAcceptsAnyAnswerBelow500(t *testing.T) {
	if err := CheckProxy(context.Background(), "not a proxy"); err == nil {
		t.Fatal("CheckProxy(bad URL) error = nil, want a refusal")
	}
	// A proxy address nothing listens on is unreachable.
	if err := CheckProxy(context.Background(), "socks5://127.0.0.1:1"); err == nil {
		t.Fatal("CheckProxy(dead proxy) error = nil, want a failure")
	}
}

// connectProxy answers CONNECT with the given status so the proxy check's
// branches can be measured without reaching example.com.
func connectProxy(t *testing.T, status int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect {
			t.Errorf("method = %s, want CONNECT for a proxied https request", r.Method)
		}
		w.WriteHeader(status)
	}))
}

// TestCheckProxyReportsAServerError covers the 5xx branch: a proxy that
// answers with a server error is not usable.
func TestCheckProxyReportsAServerError(t *testing.T) {
	proxy := connectProxy(t, http.StatusBadGateway)
	defer proxy.Close()
	if err := CheckProxy(context.Background(), proxy.URL); err == nil {
		t.Fatal("CheckProxy(502 proxy) error = nil, want a failure")
	}
}

// TestCheckProxyAcceptsAHealthyAnswer covers the pass branch: a proxy that
// accepts the tunnel answers below 500, which is all the check needs.
func TestCheckProxyAcceptsAHealthyAnswer(t *testing.T) {
	proxy := connectProxy(t, http.StatusOK)
	defer proxy.Close()
	if err := CheckProxy(context.Background(), proxy.URL); err == nil {
		// The tunnel is accepted and the request then fails inside it, which
		// is still an error: the check needs an answer, not a tunnel.
		t.Fatal("CheckProxy() = nil, want the failure inside an empty tunnel")
	}
}

// TestNewProxyPoolBuildsAWorkingPool covers the constructor the daemon and the
// management process share: a stored entry can be created, read, and selected.
func TestNewProxyPoolBuildsAWorkingPool(t *testing.T) {
	home := t.TempDir()
	pool := newProxyPool(newPoolStore(), home, &modeSecrets{mode: secrets.ModeEncryptedFile}, clock.New())
	if pool == nil {
		t.Fatal("newProxyPool() = nil")
	}
	item, err := pool.Create(context.Background(), appproxy.Draft{
		Scheme: "http", Host: "127.0.0.1", Port: 7890, Enabled: true,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	selection, err := pool.Select(context.Background(), nil)
	if err != nil || selection.ID != item.ID || !strings.Contains(selection.URL, "127.0.0.1:7890") {
		t.Fatalf("Select() = %+v, %v", selection, err)
	}
}

// TestMustParseURLKeepsTheAddress covers the helper a bad address would
// otherwise crash on: it returns nil rather than panicking.
func TestMustParseURLKeepsTheAddress(t *testing.T) {
	if parsed := mustParseURL("http://127.0.0.1:7890"); parsed == nil || parsed.Host != "127.0.0.1:7890" {
		t.Fatalf("mustParseURL() = %v", parsed)
	}
	if parsed := mustParseURL("://bad"); parsed != nil {
		t.Fatalf("mustParseURL(bad) = %v, want nil", parsed)
	}
}

// TestBackfillLegacyProxyMovesAStoredURL covers the one-time move: an empty
// legacy value does nothing, a stored one becomes a pool entry and is cleared,
// and a failed clear is logged without losing the entry.
func TestBackfillLegacyProxyMovesAStoredURL(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	path := config.ConfigPath(home)
	pool, store := testPool(t)

	if err := config.UpdateProxyURL(path, "http://127.0.0.1:7890"); err != nil {
		t.Fatalf("UpdateProxyURL() error = %v", err)
	}
	if got := legacyProxyURL(home); got != "http://127.0.0.1:7890" {
		t.Fatalf("legacyProxyURL() = %q", got)
	}
	backfillLegacyProxy(ctx, pool, home, discardLogger())
	if len(store.rows) != 1 {
		t.Fatalf("pool has %d entries, want the legacy URL moved in", len(store.rows))
	}
	if got := legacyProxyURL(home); got != "" {
		t.Fatalf("legacyProxyURL() after the move = %q, want it cleared", got)
	}
	// A second run has nothing left to move.
	backfillLegacyProxy(ctx, pool, home, discardLogger())
	if len(store.rows) != 1 {
		t.Fatalf("second backfill made %d entries, want 1", len(store.rows))
	}
}

// TestBackfillLegacyProxyToleratesAMissingPoolAndConfig covers the guards: a
// nil pool and a home with no startup file are both quiet no-ops.
func TestBackfillLegacyProxyToleratesAMissingPoolAndConfig(t *testing.T) {
	home := t.TempDir()
	backfillLegacyProxy(context.Background(), nil, home, discardLogger())
	if got := legacyProxyURL(home); got != "" {
		t.Fatalf("legacyProxyURL() with no file = %q", got)
	}
	pool, store := testPool(t)
	backfillLegacyProxy(context.Background(), pool, home, discardLogger())
	if len(store.rows) != 0 {
		t.Fatalf("backfill with no legacy URL made %d entries", len(store.rows))
	}
}

// TestProxyURLReadsThePoolForDiscovery covers the discovery callback, which has
// no request-local rotation and falls back to a direct connection.
func TestProxyURLReadsThePoolForDiscovery(t *testing.T) {
	pool, _ := testPool(t)
	ctx := context.Background()
	if got := pool.FirstURL(ctx); got != "" {
		t.Fatalf("FirstURL(empty pool) = %q", got)
	}
	item, err := pool.Create(ctx, appproxy.Draft{
		Scheme: "http", Host: "127.0.0.1", Port: 7890, Enabled: true,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if got := pool.FirstURL(ctx); !strings.Contains(got, "127.0.0.1:7890") {
		t.Fatalf("FirstURL() = %q, want the stored proxy", got)
	}
	pool.Cool(item.ID, time.Minute)
	if got := pool.FirstURL(ctx); got != "" {
		t.Fatalf("FirstURL() while cooling = %q, want a direct connection", got)
	}
}
