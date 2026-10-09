package server_test

import (
	"context"
	"encoding/json"
	"github.com/jonaskahn/relo/internal/inference"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/BurntSushi/toml"

	"github.com/jonaskahn/relo/internal/access"
	"github.com/jonaskahn/relo/internal/account"
	"github.com/jonaskahn/relo/internal/adapters/codingclients"
	"github.com/jonaskahn/relo/internal/adapters/modelsdev"
	"github.com/jonaskahn/relo/internal/adapters/oauth"
	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	"github.com/jonaskahn/relo/internal/adapters/upstream"
	wireformats "github.com/jonaskahn/relo/internal/adapters/wire/formats"
	appaccess "github.com/jonaskahn/relo/internal/application/access"
	appaccount "github.com/jonaskahn/relo/internal/application/account"
	appactivity "github.com/jonaskahn/relo/internal/application/activity"
	appcatalog "github.com/jonaskahn/relo/internal/application/catalog"
	appintegration "github.com/jonaskahn/relo/internal/application/integration"
	approuting "github.com/jonaskahn/relo/internal/application/routing"
	appsettings "github.com/jonaskahn/relo/internal/application/settings"
	appstatus "github.com/jonaskahn/relo/internal/application/status"
	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/internal/config"
	"github.com/jonaskahn/relo/internal/platform"
	"github.com/jonaskahn/relo/internal/routing"
	"github.com/jonaskahn/relo/internal/server"
	"github.com/jonaskahn/relo/tests/testkit"
)

const (
	adminToken    = "admin-token-value"
	testVersion   = "test-version"
	credentialRef = "apikey/openai/one"
)

// dataPlaneToken is the secret of the client key the current harness issued.
// It is a variable rather than a constant because a secret exists only at
// the moment it is minted, and the tests are not parallel.
var dataPlaneToken string

// harness wires a server exactly the way the daemon does, with a mock
// upstream and an in-memory secret store standing in for the keychain.
type harness struct {
	t         *testing.T
	cfg       *config.Config
	db        *sqlite.DB
	logger    *slog.Logger
	logBuffer *testkit.SyncBuffer
	clock     *testkit.FakeClock
	server    *server.Server
	usage     *sqlite.UsageRecorder
	upstream  *upstreamServer
	service   *appcatalog.Service
	accounts  *appaccount.Service
	routes    *approuting.Service
	keys      *appaccess.Keys
	status    *appstatus.Service
	settings  *appsettings.Service
	// settingsPath is the startup file the settings service reads and writes,
	// so a test can store one and compare the file against the boot config.
	settingsPath string
	catalog      *catalog.Catalog
	pools        *account.Manager
	// providers are catalog rows a test wants in the snapshot beside the mock
	// upstream, such as a provider that signs in rather than taking a key.
	providers     []sqlite.ProviderRow
	providerSaved bool
	secrets       map[string]string
	events        *server.EventBus
	sessions      *server.Sessions
	login         *stubLogin
	clientPaths   codingclients.Paths
}

// stubLogin answers every login the management API starts with one stored
// credential, which is what the async flow tests need.
type stubLogin struct {
	mu      sync.Mutex
	started int
	failure error
	calls   []server.LoginRequest
}

func (s *stubLogin) Login(_ context.Context, request server.LoginRequest) (server.LoginResult, error) {
	s.mu.Lock()
	s.started++
	s.calls = append(s.calls, request)
	failure := s.failure
	s.mu.Unlock()
	if request.Prompt != nil {
		_ = request.Prompt(server.LoginPrompt{URL: "https://example.test/authorize"})
	}
	if failure != nil {
		return server.LoginResult{}, failure
	}
	return server.LoginResult{Label: "stub-login", Kind: "oauth", Secret: "{stub-credential}"}, nil
}

func (s *stubLogin) setFailure(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failure = err
}

func (s *stubLogin) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.started
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	logger, buffer := testkit.TestLogger(t)
	harness := &harness{
		t:         t,
		cfg:       defaultConfig(),
		db:        testkit.OpenTestDB(t),
		logger:    logger,
		logBuffer: buffer,
		clock:     testkit.NewFakeClock(time.Unix(1_700_000_000, 0)),
		upstream:  newUpstreamServer(t),
		secrets:   map[string]string{credentialRef: "sk-test"},
		login:     &stubLogin{},
	}
	harness.settingsPath = config.ConfigPath(t.TempDir())
	harness.settings = appsettings.New(appsettings.Options{
		Retention:    sqlite.NewRetentionSettings(harness.db, sqlite.RetentionOptions{Now: harness.clock.Now}),
		QuotaDisplay: sqlite.NewQuotaDisplaySettings(harness.db),
		ConfigPath:   harness.settingsPath,
		Clock:        harness.clock,
	})
	// The startup file matches the boot configuration, so a fresh harness
	// reports no restart pending: that is the state a running daemon is in.
	harness.storeBootConfig(t)
	harness.events = server.NewEventBus(server.EventBusOptions{Now: harness.clock.Now})
	harness.sessions = server.NewSessions(harness.clock)
	harness.server = harness.newServerWithPort(harness.cfg.Server.Port)
	dataPlaneToken = harness.issueKey("test-agent", appaccess.NewAccessKey{
		Name: "test-agent", Kind: appaccess.Agent, Client: access.CustomClient,
	})
	return harness
}

// issueKey creates one client key the data plane accepts and returns its
// secret, which is the only time it exists.
func (h *harness) issueKey(name string, request appaccess.NewAccessKey) string {
	h.t.Helper()
	issued, err := h.keys.CreateAccessKey(context.Background(), request)
	if err != nil {
		h.t.Fatalf("CreateAccessKey(%s) error = %v", name, err)
	}
	return issued.Token
}

// newServerWithPort builds a server against the harness database and the
// mock upstream, which is what both handler tests and Start tests use.
func (h *harness) newServerWithPort(port int) *server.Server {
	return h.newServerWithCredentials(port, defaultEntry())
}

// defaultEntry is the one account the harness stores: an active API key for
// the provider the mock upstream answers for.
func defaultEntry() account.PoolEntry {
	return account.PoolEntry{
		ID: "one", ProviderID: "openai", Kind: "api_key",
		Label: "default", SecretRef: credentialRef, Status: account.StatusActive,
	}
}

// newServerWithCredentials builds a server whose pool holds exactly the
// given credentials, which is how the handler tests exercise both the
// working and the empty-pool paths.
func (h *harness) newServerWithCredentials(port int, entries ...account.PoolEntry) *server.Server {
	h.t.Helper()
	cfg := *h.cfg
	cfg.Server.Port = port
	return h.newServerWithConfig(&cfg, entries...)
}

// newServerWithConfig builds a server from one configuration, which a test
// that starts a real listener uses to ask for ports this machine can bind.
func (h *harness) newServerWithConfig(cfg *config.Config, entries ...account.PoolEntry) *server.Server {
	return h.newServerWithOptions(cfg, entries, nil)
}

// newServerWithOptions builds a server from one configuration with the last
// word on its options, which a test that needs a collaborator the harness
// does not wire uses.
func (h *harness) newServerWithOptions(cfg *config.Config, entries []account.PoolEntry, apply func(*server.Options)) *server.Server {
	h.t.Helper()
	recorder, err := sqlite.NewUsageRecorder(h.db, h.logger)
	if err != nil {
		h.t.Fatalf("NewUsageRecorder() error = %v", err)
	}
	h.usage = recorder
	h.t.Cleanup(func() { _ = recorder.Close() })
	captures := sqlite.NewCaptureStore(h.db)
	// The catalog rows come first: a credential names the provider it belongs
	// to, so the provider has to exist before one is stored.
	h.saveMockProvider()
	pools := h.newPools(entries...)
	h.pools = pools
	h.catalog = h.newCatalog(pools)
	h.service = h.newService(h.catalog, pools)
	options := server.Options{
		Config:          cfg,
		SchemaVersion:   h.db,
		Logger:          h.logger,
		Clock:           h.clock,
		Version:         testVersion,
		AdminToken:      adminToken,
		Catalog:         h.catalog,
		Router:          routing.New(routing.Options{Catalog: h.catalog, Pools: pools, FailoverBackoff: account.DefaultFailoverBackoff()}),
		Relay:           platform.NewRelay(h.newCredentials(pools), upstream.NewExecutor(&http.Client{}, h.logger)),
		Templates:       platform.NewTemplateSource(),
		CaptureRedactor: platform.NewCaptureRedactor(),
		Pools:           pools,
		Secrets:         platform.SecretView{SecretStore: testkit.SecretStore(h.secrets)},
		Usage:           recorder,
		Captures:        captures,
		Activity: appactivity.New(appactivity.Options{
			Usage: sqlite.NewUsageQuery(h.db), Captures: captures,
		}),
		Accounts:   h.accounts,
		Status:     h.status,
		Routes:     h.routes,
		CatalogAPI: h.service,
		Formats:    wireformats.New(),
		Keys:       h.keys,
		Settings:   h.settings,
		// The shipped retry waits are seconds long; tests run the same path
		// with a millisecond so a retry costs nothing.
		RetryDelay: func(int) time.Duration { return time.Millisecond },
		Integrations: appintegration.NewService(appintegration.ServiceOptions{
			Paths:     platform.NewPathResolver(h.integrationPaths()),
			Agents:    platform.NewAgentRegistry(),
			Files:     platform.NewClientFiles(h.integrationPaths()),
			Processes: platform.NewAgentProcesses(),
			Wire:      platform.NewWireCodec(),
			Store:     platform.NewIntegrationStore(h.db), Catalog: h.catalog,
			Keys: h.keys, Now: h.clock.Now,
		}),
		Events:   h.events,
		Sessions: h.sessions,
		Login:    h.login,
	}
	if apply != nil {
		apply(&options)
	}
	// The harness wires the callback surface the way the daemon does: one
	// broker the page and the login share, the shared brand icon, and the
	// default port registry.
	if options.Callbacks == nil {
		options.Callbacks = platform.NewCallbackBridge(callbackBroker(cfg.Server.Port))
	}
	if options.CallbackPorts == nil {
		options.CallbackPorts = oauth.DefaultRegistry()
	}
	if options.CallbackIcon == "" {
		options.CallbackIcon = oauth.CallbackIcon
	}
	served := server.New(options)
	// The management guard is what the tests exercise, so the console is a
	// stub rather than a compiled console build.
	served.SetDashboard(stubConsole())
	return served
}

func (h *harness) integrationPaths() codingclients.Paths {
	h.t.Helper()
	if h.clientPaths.Home != "" {
		return h.clientPaths
	}
	home := h.t.TempDir()
	// Codex and Claude Code export their homes into this process. Leaving
	// those set writes the operator's install and restarts the live daemon.
	h.t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
	h.t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(home, ".claude"))
	paths := codingclients.Paths{
		Home: home, StateHome: h.t.TempDir(),
		DataPlane: func(protocol string) string {
			if protocol == inference.ProtocolOpenAI {
				return "http://127.0.0.1:10201"
			}
			return ""
		},
	}
	if got, want := paths.CodexConfig(), filepath.Join(home, ".codex", "config.toml"); got != want {
		h.t.Fatalf("CodexConfig() = %q, want %q", got, want)
	}
	if got, want := paths.ClaudeConfigDir(), filepath.Join(home, ".claude"); got != want {
		h.t.Fatalf("ClaudeConfigDir() = %q, want %q", got, want)
	}
	h.clientPaths = paths
	return paths
}

// newCredentials builds the source that turns a stored account into the
// authentication one attempt carries, which is what the relay resolves before
// it sends anything.
func (h *harness) newCredentials(pools *account.Manager) *upstream.Source {
	return upstream.New(upstream.Options{
		Pools: platform.NewCredentialPools(pools), Secrets: testkit.SecretStore(h.secrets),
		Flows: platform.NewFlowRegistry(oauth.DefaultRegistry()), Now: h.clock.Now,
	})
}

// saveMockProvider stores the one provider and model the mock upstream serves,
// which is what every server instance in this package routes to.
func (h *harness) saveMockProvider() {
	h.t.Helper()
	if h.providerSaved {
		return
	}
	h.providerSaved = true
	ctx := context.Background()
	repo := sqlite.NewCatalogRepo(h.db)
	if err := repo.SaveProvider(ctx, sqlite.ProviderRow{
		ID: "openai", Origin: string(catalog.OriginCustom), Label: "OpenAI",
		Auth: string(catalog.AuthAPIKey), APIFormat: string(catalog.FormatOpenAIChat),
		BaseURL: h.upstream.URL(), ModelsFormat: string(catalog.ModelsNone),
		Enabled: true, Rank: 100, PoolStrategy: "least-loaded",
	}); err != nil {
		h.t.Fatalf("save the mock provider: %v", err)
	}
	if err := repo.SaveModel(ctx, sqlite.ModelRow{
		ProviderID: "openai", ModelID: "gpt-4o", Source: "manual",
		APIFormat: string(catalog.FormatOpenAIChat),
		Enabled:   true,
	}); err != nil {
		h.t.Fatalf("save the mock model: %v", err)
	}
	name, category, status := "GPT-4o", string(catalog.CategoryChat), "active"
	if err := repo.SaveModelFacts(ctx, sqlite.ModelFactsRow{
		ProviderID: "openai", ModelID: "gpt-4o", Layer: "override",
		Name: &name, Category: &category, Status: &status,
	}); err != nil {
		h.t.Fatalf("save the mock model facts: %v", err)
	}
	for _, row := range h.providers {
		if err := repo.SaveProvider(ctx, row); err != nil {
			h.t.Fatalf("save provider %s: %v", row.ID, err)
		}
	}
}

// newCatalog builds the runtime catalog the server resolves against. The
// pools are what make the provider configured, so a catalog built here routes
// to it.
func (h *harness) newCatalog(pools *account.Manager) *catalog.Catalog {
	h.t.Helper()
	models := catalog.New(sqlite.NewCatalogReader(h.db), platform.NewPoolDirectory(pools), wireformats.New())
	if err := models.Reload(context.Background()); err != nil {
		h.t.Fatalf("reload catalog: %v", err)
	}
	return models
}

// stubConsole stands in for the compiled console, which the tests never
// build, and answers the way it does: pages read, writes do not.
func stubConsole() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "the console is read-only", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, "<!doctype html><title>Relo</title>")
	})
}

// newPools returns the pools a server instance serves credentials from,
// with the given entries stored in the database the way the daemon stores
// them.
func (h *harness) newPools(entries ...account.PoolEntry) *account.Manager {
	h.t.Helper()
	ctx := context.Background()
	repo := sqlite.NewCredentialStore(h.db)
	if err := h.syncCredentials(repo, entries); err != nil {
		h.t.Fatalf("sync credentials: %v", err)
	}
	pools := account.NewManager(repo, testkit.SecretStore(h.secrets))
	if err := pools.LoadFromDB(ctx); err != nil {
		h.t.Fatalf("LoadFromDB() error = %v", err)
	}
	return pools
}

// syncCredentials makes the credential table hold exactly the entries a
// server instance was built with, so a test can ask for an empty pool.
func (h *harness) syncCredentials(repo account.CredentialRepository, entries []account.PoolEntry) error {
	ctx := context.Background()
	if _, err := h.db.SQL().ExecContext(ctx, "DELETE FROM credentials"); err != nil {
		return err
	}
	for _, entry := range entries {
		if err := repo.Insert(ctx, entry); err != nil {
			return err
		}
	}
	return nil
}

// newService builds the application service the management API and the
// dashboard talk to, exactly the way the daemon does.
func (h *harness) newService(models *catalog.Catalog, pools *account.Manager) *appcatalog.Service {
	h.t.Helper()
	store := testkit.SecretStore(h.secrets)
	quiet := platform.NewQuietSecrets(store)
	edges := &platform.AccountEdges{}
	h.accounts = appaccount.New(appaccount.Options{
		Entries: sqlite.NewCredentialStore(h.db), Secrets: quiet, Pools: pools, Catalog: models,
		Facts: sqlite.NewAccountFactStore(h.db), Providers: edges, Discover: edges,
		Refresh: edges.RefreshAfterCredential, Logger: h.logger,
	})
	h.keys = appaccess.NewKeys(sqlite.NewAccessKeyStore(h.db), h.clock)
	h.service = appcatalog.New(appcatalog.Options{
		Store: sqlite.NewCatalogRepo(h.db), Catalog: models, Pools: pools,
		Entries: sqlite.NewCredentialStore(h.db), Accounts: h.accounts, Secrets: quiet,
		Direct:    upstream.Direct,
		ModelsDev: modelsdev.NewDirectory(filepath.Join(h.t.TempDir(), "cache"), "", nil),
		Templates: platform.TemplateRegistry{}, Logger: h.logger,
	})
	edges.Catalog = h.service
	h.routes = approuting.New(approuting.Options{
		Snapshot: h.service, Routes: sqlite.NewCatalogRepo(h.db), Reload: h.service,
	})
	h.status = appstatus.New(appstatus.Options{
		DB: h.db,
		Retention: sqlite.NewRetention(h.db, sqlite.RetentionOptions{
			Logger: h.logger, Now: h.clock.Now,
		}),
		Secrets: platform.NewSecretView(store), Entries: sqlite.NewCredentialStore(h.db),
		Quotas: platform.NewQuotaStore(h.db), Keys: h.keys,
		Accounts: h.accounts, Catalog: models,
	})
	return h.service
}

func (h *harness) dataPlane(method, path, token string, body io.Reader) *httptest.ResponseRecorder {
	h.t.Helper()
	return h.dataPlaneOn(inference.ProtocolOpenAI, method, path, token, body)
}

// dataPlaneOn performs one request against the listener of one protocol, and
// fails the test when this process serves no such port.
func (h *harness) dataPlaneOn(protocol, method, path, token string, body io.Reader) *httptest.ResponseRecorder {
	h.t.Helper()
	handler, found := h.server.DataPlaneHandler(protocol)
	if !found {
		h.t.Fatalf("the server serves no %s port", protocol)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request(method, path, token, body))
	return recorder
}

// serverWithoutRelay builds a server with none of the collaborators the data
// plane resolves a request through, which is what a build without them
// answers inference from.
func serverWithoutRelay(t *testing.T, h *harness) *server.Server {
	t.Helper()
	return bareServer(h)
}

// serverWithoutService builds a server with no management service, which is
// what a build without a store answers the management API from.
func serverWithoutService(h *harness) *server.Server {
	return bareServer(h)
}

// bareServer is a server built from the harness collaborators and nothing
// else: no database, no catalog, no service.
func bareServer(h *harness) *server.Server {
	served := server.New(server.Options{
		Config: h.cfg, Logger: h.logger, Clock: h.clock, Version: testVersion,
		AdminToken: adminToken, Sessions: h.sessions,
	})
	served.SetDashboard(stubConsole())
	return served
}

func (h *harness) management(method, path, token string, body io.Reader) *httptest.ResponseRecorder {
	h.t.Helper()
	return h.managementOn(h.server, method, path, token, body)
}

// managementOn performs one management request against a server the test
// built itself, which is how a build missing a collaborator is exercised.
func (h *harness) managementOn(served *server.Server, method, path, token string, body io.Reader) *httptest.ResponseRecorder {
	h.t.Helper()
	recorder := httptest.NewRecorder()
	served.Handler().ServeHTTP(recorder, request(method, path, token, body))
	return recorder
}

func (h *harness) usageRows() int {
	h.t.Helper()
	var count int
	if err := h.db.SQL().QueryRow("SELECT count(*) FROM usage_events").Scan(&count); err != nil {
		h.t.Fatalf("count usage events: %v", err)
	}
	return count
}

// defaultConfig is the configuration the handler tests build servers from. No
// data plane port is asked for, so a test that never starts a listener never
// takes one, and the handler tests reach a protocol's routes through
// DataPlaneHandler instead.
func defaultConfig() *config.Config {
	cfg := config.DefaultConfig()
	cfg.Server.DataPlane = config.DataPlaneConfig{}
	// The guard the handler tests exercise is the one an operator asks for by
	// turning the login on; a test of the open console turns it back off.
	cfg.Admin.Login = true
	return &cfg
}

// storeBootConfig writes the harness boot configuration to the startup file
// the settings service reads, so the baseline is a daemon whose file matches
// what it booted with.
func (h *harness) storeBootConfig(t *testing.T) {
	t.Helper()
	encoded, err := toml.Marshal(h.cfg)
	if err != nil {
		t.Fatalf("encode the harness config: %v", err)
	}
	if err := os.WriteFile(h.settingsPath, encoded, 0o600); err != nil {
		t.Fatalf("write the harness config: %v", err)
	}
}

// freeConfig asks for ports this machine can bind: an ephemeral one for the
// management listener and one per protocol, so a test that starts a server
// never collides with a daemon or with another test.
func freeConfig(t *testing.T) *config.Config {
	t.Helper()
	cfg := config.DefaultConfig()
	cfg.Server.Bind = "127.0.0.1"
	cfg.Server.Port = 0
	cfg.Server.DataPlane = config.DataPlaneConfig{OpenAI: freePort(t), Anthropic: freePort(t)}
	cfg.Admin.Login = true
	return &cfg
}

// freePort returns a port nothing is listening on right now.
func freePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("find a free port: %v", err)
	}
	defer func() { _ = listener.Close() }()
	return listener.Addr().(*net.TCPAddr).Port
}

func request(method, path, token string, body io.Reader) *http.Request {
	req := httptest.NewRequest(method, path, body)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return req
}

// upstreamServer is a mock provider endpoint whose response tests can
// change between calls.
type upstreamServer struct {
	server   *httptest.Server
	mu       sync.Mutex
	fixture  string
	sse      bool
	status   int
	delay    time.Duration
	headers  map[string]string
	requests []string
	// bodies keeps each request's payload, which is what proves what the
	// relay actually asked the upstream for.
	bodies        []string
	auths         []string
	waitForCancel chan struct{}
	cancelled     chan struct{}
}

func newUpstreamServer(t *testing.T) *upstreamServer {
	t.Helper()
	upstream := &upstreamServer{fixture: "openai/chat_streaming.txt", sse: true, status: http.StatusOK}
	upstream.server = httptest.NewServer(http.HandlerFunc(upstream.serve))
	t.Cleanup(upstream.server.Close)
	return upstream
}

func (u *upstreamServer) serve(w http.ResponseWriter, r *http.Request) {
	payload, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	u.mu.Lock()
	fixture, sse, status, delay := u.fixture, u.sse, u.status, u.delay
	headers := u.headers
	waitForCancel, cancelled := u.waitForCancel, u.cancelled
	u.requests = append(u.requests, r.URL.Path)
	u.bodies = append(u.bodies, string(payload))
	u.auths = append(u.auths, r.Header.Get("Authorization"))
	u.mu.Unlock()
	if waitForCancel != nil {
		close(waitForCancel)
		<-r.Context().Done()
		close(cancelled)
		return
	}
	body, err := os.ReadFile(testkit.FixturePath(fixture))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if delay > 0 {
		time.Sleep(delay)
	}
	contentType := "application/json"
	if sse {
		contentType = "text/event-stream"
	}
	w.Header().Set("Content-Type", contentType)
	for name, value := range headers {
		w.Header().Set(name, value)
	}
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

// lastBody returns the payload of the most recent request, which is what a
// test reads to see what the relay asked the upstream for.
func (u *upstreamServer) lastBody() string {
	u.mu.Lock()
	defer u.mu.Unlock()
	if len(u.bodies) == 0 {
		return ""
	}
	return u.bodies[len(u.bodies)-1]
}

func (u *upstreamServer) lastAuthorization() string {
	u.mu.Lock()
	defer u.mu.Unlock()
	if len(u.auths) == 0 {
		return ""
	}
	return u.auths[len(u.auths)-1]
}

func (u *upstreamServer) URL() string {
	return u.server.URL
}

func (u *upstreamServer) delayBy(delay time.Duration) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.delay = delay
}

// setHeaders makes every following answer carry the given headers, which is
// how a test stands in for a provider that publishes its quota beside a
// response.
func (u *upstreamServer) setHeaders(headers map[string]string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.headers = headers
}

func (u *upstreamServer) requestCount() int {
	u.mu.Lock()
	defer u.mu.Unlock()
	return len(u.requests)
}

// waitForAddress polls until the server records the address the test
// asked for, which happens once the listener is bound.
func waitForAddress(t *testing.T, address func() string) string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if value := address(); value != "" {
			return value
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("the server never bound its listener")
	return ""
}

// waitForDataPlaneAddress polls until the server records the address it bound
// for one protocol, which happens once every listener is up.
func waitForDataPlaneAddress(t *testing.T, built *server.Server, protocol string) string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if addr := built.DataPlaneAddrs()[protocol]; addr != "" {
			return addr
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("the server never bound the %s listener", protocol)
	return ""
}

func portOf(t *testing.T, rawURL string) int {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse %s: %v", rawURL, err)
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil {
		t.Fatalf("parse port of %s: %v", rawURL, err)
	}
	return port
}

// flushRecorder records the status the handler chose.
type flushRecorder struct {
	httptest.ResponseRecorder
	status int
}

func (f *flushRecorder) WriteHeader(status int) {
	f.status = status
	f.ResponseRecorder.WriteHeader(status)
}

func (f *flushRecorder) Flush() {
	f.ResponseRecorder.Flush()
}

// streamingBody is a chat completions request that asks for SSE.
func streamingBody() string {
	return `{"model":"gpt-4o","stream":true,"messages":[{"role":"user","content":"hi"}]}`
}

// managementJSON performs one management request with a JSON body, decodes
// the response into target, and returns the status.
func (h *harness) managementJSON(t *testing.T, method, path, payload string, target any) int {
	t.Helper()
	recorder := h.management(method, path, adminToken, strings.NewReader(payload))
	if target != nil && recorder.Body.Len() > 0 {
		if err := json.Unmarshal(recorder.Body.Bytes(), target); err != nil {
			t.Fatalf("decode the response of %s: %v", path, err)
		}
	}
	return recorder.Code
}
