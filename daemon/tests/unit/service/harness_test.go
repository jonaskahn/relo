package service_test

import (
	"context"
	"net/http"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/account"
	"github.com/jonaskahn/relo/internal/adapters/discovery"
	"github.com/jonaskahn/relo/internal/adapters/modelsdev"
	"github.com/jonaskahn/relo/internal/adapters/secrets"
	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	"github.com/jonaskahn/relo/internal/adapters/upstream"
	"github.com/jonaskahn/relo/internal/adapters/wire/formats"
	appaccess "github.com/jonaskahn/relo/internal/application/access"
	appaccount "github.com/jonaskahn/relo/internal/application/account"
	appcatalog "github.com/jonaskahn/relo/internal/application/catalog"
	approuting "github.com/jonaskahn/relo/internal/application/routing"
	appsettings "github.com/jonaskahn/relo/internal/application/settings"
	appstatus "github.com/jonaskahn/relo/internal/application/status"
	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/internal/config"
	"github.com/jonaskahn/relo/internal/platform"
	"github.com/jonaskahn/relo/tests/testkit"
)

// harness wires the use cases the way the daemon does: a real database, a
// catalog built over it, and a pool whose secrets live in memory.
type harness struct {
	t         *testing.T
	home      string
	db        *sqlite.DB
	catalog   *catalog.Catalog
	pools     *account.Manager
	secrets   map[string]string
	clock     *testkit.FakeClock
	modelsDev *modelsdev.Directory
	service   *appcatalog.Service
	proxyURL  func() string
	accounts  *appaccount.Service
	routes    *approuting.Service
	keys      *appaccess.Keys
	status    *appstatus.Service
	settings  *appsettings.Service
	// refreshes counts the roster refreshes the accounts service starts, so
	// cleanup can wait for them before the temporary home is removed.
	refreshes *refreshTracker
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	built := &harness{
		t: t, home: testkit.TempHome(t), db: testkit.OpenTestDB(t),
		secrets: map[string]string{}, clock: testkit.NewFakeClock(time.Unix(1_700_000_000, 0)),
		refreshes: newRefreshTracker(),
	}
	built.catalog = built.newCatalog()
	store := testkit.SecretStore(built.secrets)
	built.pools = account.NewManager(sqlite.NewCredentialStore(built.db), store)
	if err := built.pools.LoadFromDB(context.Background()); err != nil {
		t.Fatalf("LoadFromDB() error = %v", err)
	}
	built.seedProvider("openai", "OpenAI", string(catalog.AuthAPIKey))
	built.seedProvider("claude", "Claude", string(catalog.AuthOAuth))
	built.buildService(built.catalog, built.pools, store)
	built.settings = appsettings.New(appsettings.Options{
		Retention:    sqlite.NewRetentionSettings(built.db, sqlite.RetentionOptions{Now: built.clock.Now}),
		QuotaDisplay: sqlite.NewQuotaDisplaySettings(built.db),
		ConfigPath:   config.ConfigPath(built.home),
		Clock:        built.clock,
	})
	return built
}

func (h *harness) newCatalog() *catalog.Catalog {
	return catalog.New(sqlite.NewCatalogReader(h.db), nil, formats.New())
}

// newCredentialCatalog returns the catalog the daemon builds: one that knows
// which providers hold a credential, which is what an update of every
// connection reads its targets from.
func (h *harness) newCredentialCatalog() *catalog.Catalog {
	return catalog.New(sqlite.NewCatalogReader(h.db), platform.NewPoolDirectory(h.pools), formats.New())
}

// seedProvider stores one provider an operator would have added by hand, so a
// test has something to configure.
func (h *harness) seedProvider(id, label, auth string) {
	h.t.Helper()
	if _, found := h.catalog.Snapshot(); found {
		if _, known := h.snapshotProvider(id); known {
			return
		}
	}
	repo := sqlite.NewCatalogRepo(h.db)
	host := sqlite.ProviderRow{
		ID: id, Origin: string(catalog.OriginCustom), Label: label, Auth: auth,
		APIFormat: string(catalog.FormatOpenAIChat), BaseURL: "https://" + id + "/v1",
		KeyHeader:    string(catalog.KeyHeaderBearer),
		ModelsFormat: string(catalog.ModelsNone), Headers: map[string]string{},
		Variables: map[string]string{}, Enabled: true, Rank: 100,
		PoolStrategy: sqlite.StrategyLeastLoaded,
	}
	if err := repo.SaveProvider(context.Background(), host); err != nil {
		h.t.Fatalf("save provider %s: %v", id, err)
	}
	model := sqlite.ModelRow{
		ProviderID: id, ModelID: "model-1", Source: "manual",
		Enabled: true,
	}
	if err := repo.SaveModel(context.Background(), model); err != nil {
		h.t.Fatalf("save model of %s: %v", id, err)
	}
	name := "Model 1"
	cat := string(catalog.CategoryChat)
	status := "active"
	facts := sqlite.ModelFactsRow{
		ProviderID: id, ModelID: "model-1", Layer: "override",
		Name: &name, Category: &cat, Status: &status,
	}
	if err := repo.SaveModelFacts(context.Background(), facts); err != nil {
		h.t.Fatalf("save model facts of %s: %v", id, err)
	}
	if err := h.catalog.Reload(context.Background()); err != nil {
		h.t.Fatalf("reload catalog: %v", err)
	}
}

// snapshotProvider reports whether the catalog already holds one provider.
func (h *harness) snapshotProvider(id string) (catalog.Provider, bool) {
	snapshot, found := h.catalog.Snapshot()
	if !found {
		return catalog.Provider{}, false
	}
	return snapshot.Provider(id)
}

// session is one wired catalog stack: the catalog use cases and the account
// lifecycle they coordinate with, which a test drives together.
type session struct {
	*appcatalog.Service
	accounts  *appaccount.Service
	routes    *approuting.Service
	modelsDev *modelsdev.Directory
	status    *appstatus.Service
}

// session reports the catalog stack the harness built last.
func (h *harness) session() *session {
	return &session{
		Service: h.service, accounts: h.accounts, routes: h.routes,
		modelsDev: h.modelsDev, status: h.status,
	}
}

// withoutCatalog rebuilds the service with no catalog attached, which is the
// state a management-only process can be in.
func (h *harness) withoutCatalog() *session {
	return h.buildService(nil, h.pools, testkit.SecretStore(h.secrets))
}

// withoutPool rebuilds the service with no pool attached, which is what a
// one-shot management command runs with.
func (h *harness) withoutPool() *session {
	return h.buildService(h.catalog, nil, testkit.SecretStore(h.secrets))
}

// withoutSecrets rebuilds the service with no secret store attached.
func (h *harness) withoutSecrets() *session {
	return h.buildService(h.catalog, h.pools, nil)
}

// buildService assembles one catalog session over the given pieces, the way
// the daemon does, and keeps the account lifecycle it coordinates with.
func (h *harness) buildService(cat *catalog.Catalog, pools *account.Manager, store secrets.SecretStore) *session {
	return h.buildServiceWith(cat, pools, store, nil, nil)
}

// buildServiceWith is buildService with the models.dev directory and the
// listing client a probe reads through. An absent directory points at a
// catalog server serving nothing, so a test never reaches models.dev.
func (h *harness) buildServiceWith(cat *catalog.Catalog, pools *account.Manager, store secrets.SecretStore, md *modelsdev.Directory, lister *discovery.Lister) *session {
	h.t.Helper()
	logger, _ := testkit.TestLogger(h.t)
	if md == nil {
		md = modelsdev.NewDirectory(filepath.Join(h.home, "cache"),
			catalogServer(h.t, map[string]map[string]any{}).URL,
			&http.Client{Timeout: 5 * time.Second})
	}
	h.modelsDev = md
	quiet := platform.NewQuietSecrets(store)
	h.keys = appaccess.NewKeys(sqlite.NewAccessKeyStore(h.db), h.clock)
	edges := &platform.AccountEdges{}
	// AddAccount starts its roster refresh on a goroutine of its own, so a test
	// that adds a credential and returns can still have that refresh running
	// against the temporary home. The harness tracks every one of them and
	// waits at cleanup, before the temporary directories are removed.
	h.t.Cleanup(h.refreshes.wait)
	h.accounts = appaccount.New(appaccount.Options{
		Entries: sqlite.NewCredentialStore(h.db), Secrets: quiet,
		Pools: pools, Catalog: cat, Facts: sqlite.NewAccountFactStore(h.db),
		Providers: edges, Discover: edges, Refresh: h.refreshes.track(edges.RefreshAfterCredential),
		Logger: logger,
	})
	h.service = appcatalog.New(appcatalog.Options{
		Store: sqlite.NewCatalogRepo(h.db), Catalog: cat, Pools: pools,
		Entries: sqlite.NewCredentialStore(h.db), Accounts: h.accounts, Secrets: quiet,
		Credentials: nil, Direct: upstream.Direct,
		Discover: lister, ModelsDev: md, Templates: platform.TemplateRegistry{},
		Logger: logger, ProxyURL: h.proxyURL,
	})
	edges.Catalog = h.service
	h.routes = approuting.New(approuting.Options{
		Snapshot: h.service, Routes: sqlite.NewCatalogRepo(h.db), Reload: h.service,
	})
	h.status = appstatus.New(appstatus.Options{
		DB: h.db,
		Retention: sqlite.NewRetention(h.db, sqlite.RetentionOptions{
			Now: h.clock.Now,
		}),
		Secrets: platform.NewSecretView(store), Entries: sqlite.NewCredentialStore(h.db),
		Quotas: platform.NewQuotaStore(h.db), Keys: h.keys,
		Accounts: h.accounts, Catalog: cat, Home: h.home,
	})
	return h.session()
}

// addAccount stores a credential the way the account commands do.
func (h *harness) addAccount(providerID, label, key string) appaccount.Account {
	h.t.Helper()
	account, err := h.accounts.AddAccount(context.Background(), appaccount.NewAccount{
		ProviderID: providerID, Label: label, SecretValue: key,
	})
	if err != nil {
		h.t.Fatalf("AddAccount(%s) error = %v", providerID, err)
	}
	return account
}

// refreshTracker counts the roster refreshes AddAccount starts so the harness
// can wait for them. Without it a test that adds a credential and returns
// leaves a goroutine writing into the temporary home while the testing package
// is already removing it.
type refreshTracker struct {
	mutex    sync.Mutex
	inFlight int
	quiet    time.Duration
	settled  time.Duration
}

// newRefreshTracker returns a tracker that waits for a refresh to have been
// running for a moment and then to stop, rather than for a count to reach
// zero: a refresh the accounts service has already started may not have run
// yet, so its count is not raised when the test finishes.
func newRefreshTracker() *refreshTracker {
	return &refreshTracker{quiet: 20 * time.Millisecond, settled: 2 * time.Second}
}

// track wraps the refresh the accounts service calls, counting it while it
// runs. It runs on the goroutine the accounts service started.
func (r *refreshTracker) track(refresh func(providerID string)) func(providerID string) {
	return func(providerID string) {
		r.mutex.Lock()
		r.inFlight++
		r.mutex.Unlock()
		defer func() {
			r.mutex.Lock()
			r.inFlight--
			r.mutex.Unlock()
		}()
		refresh(providerID)
	}
}

// wait blocks until no refresh has been running for a moment, or until a
// refresh has run for longer than the daemon's own refresh timeout, which is
// the longest a well-behaved refresh takes.
func (r *refreshTracker) wait() {
	deadline := time.Now().Add(r.settled)
	last := time.Now()
	for time.Now().Before(deadline) {
		r.mutex.Lock()
		running := r.inFlight
		r.mutex.Unlock()

		if running > 0 {
			last = time.Now()
		} else if time.Since(last) >= r.quiet {
			return
		}
		time.Sleep(time.Millisecond)
	}
}

// waitForRefresh waits until a provider's roster has been refreshed, so a test
// that asserts on the models an added credential produced does not race the
// background refresh AddAccount starts.
func (h *harness) waitForRefresh(providerID string) {
	h.t.Helper()
	repo := sqlite.NewCatalogRepo(h.db)
	deadline := time.Now().Add(10 * time.Second)
	for {
		row, err := repo.GetProvider(context.Background(), providerID)
		if err == nil && row.LastRefreshedAtMs != nil {
			// The refresh records its outcome before it reloads the catalog,
			// so reading the snapshot right here would race the reload.
			if err := h.catalog.Reload(context.Background()); err != nil {
				h.t.Fatalf("reload the catalog: %v", err)
			}
			return
		}
		if time.Now().After(deadline) {
			h.t.Fatalf("provider %s was never refreshed", providerID)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// storedSecret returns what the secret store holds under a reference.
func (h *harness) storedSecret(ref string) (string, bool) {
	value, found := h.secrets[ref]
	return value, found
}

// credentialRow reads one credential straight from the table, which is what
// the assertions on persistence need.
func (h *harness) credentialRow(id string) (sqlite.CredentialRow, bool) {
	h.t.Helper()
	rows, err := sqlite.NewCredentialRepo(h.db).List(context.Background())
	if err != nil {
		h.t.Fatalf("list credentials: %v", err)
	}
	for _, row := range rows {
		if row.ID == id {
			return row, true
		}
	}
	return sqlite.CredentialRow{}, false
}
