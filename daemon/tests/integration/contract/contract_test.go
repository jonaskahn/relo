package contract_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/BurntSushi/toml"

	"github.com/jonaskahn/relo/internal/access"
	"github.com/jonaskahn/relo/internal/account"
	"github.com/jonaskahn/relo/internal/activity"
	"github.com/jonaskahn/relo/internal/adapters/codingclients"
	"github.com/jonaskahn/relo/internal/adapters/modelsdev"
	"github.com/jonaskahn/relo/internal/adapters/oauth"
	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	"github.com/jonaskahn/relo/internal/adapters/upstream"
	wireformats "github.com/jonaskahn/relo/internal/adapters/wire/formats"
	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/internal/config"
	"github.com/jonaskahn/relo/internal/inference"
	"github.com/jonaskahn/relo/internal/platform"
	"github.com/jonaskahn/relo/internal/server"
	"github.com/jonaskahn/relo/tests/testkit"

	appaccess "github.com/jonaskahn/relo/internal/application/access"
	appaccount "github.com/jonaskahn/relo/internal/application/account"
	appactivity "github.com/jonaskahn/relo/internal/application/activity"
	appcatalog "github.com/jonaskahn/relo/internal/application/catalog"
	appintegration "github.com/jonaskahn/relo/internal/application/integration"
	approuting "github.com/jonaskahn/relo/internal/application/routing"
	appsettings "github.com/jonaskahn/relo/internal/application/settings"
	appstatus "github.com/jonaskahn/relo/internal/application/status"
	apptemplates "github.com/jonaskahn/relo/internal/application/templatesettings"
	"github.com/jonaskahn/relo/internal/routing"
	"github.com/jonaskahn/relo/internal/updates"
)

// fixedNow freezes every timestamp the contract fixtures record.
func fixedNow() time.Time {
	return time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
}

const (
	adminToken     = "contract-admin-token"
	testVersion    = "contract-version"
	openaiSecret   = "apikey/openai/contract"
	responsesRef   = "apikey/responses/contract"
	anthropicRef   = "apikey/anthropic/contract"
	responsesModel = "contract-resp-model"
	anthropicModel = "contract-claude-model"
)

// harness wires a server exactly the way the daemon does, with a frozen
// clock, a temporary database, and a mock upstream standing in for every
// provider, so every recorded response is reproducible.
type harness struct {
	t        *testing.T
	cfg      *config.Config
	db       *sqlite.DB
	logger   *slog.Logger
	clock    *testkit.FakeClock
	server   *server.Server
	upstream *mockUpstream
	service  *appcatalog.Service
	accounts *appaccount.Service
	routes   *approuting.Service
	keys     *appaccess.Keys
	settings *appsettings.Service
	catalog  *catalog.Catalog
	pools    *account.Manager
	events   *server.EventBus
	sessions *server.Sessions
	login    *stubLogin
	status   *appstatus.Service
	keyToken string
	// volatileDirs holds the exact temporary directories this run created,
	// so previews embedding them are pinned without patterns.
	volatileDirs []string
	settingsP    string
	saved        bool
	secrets      map[string]string
}

type stubLogin struct {
	mu      sync.Mutex
	started int
	calls   []server.LoginRequest
}

func (s *stubLogin) Login(_ context.Context, request server.LoginRequest) (server.LoginResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.started++
	s.calls = append(s.calls, request)
	if request.Prompt != nil {
		_ = request.Prompt(server.LoginPrompt{URL: "https://example.test/authorize"})
	}
	return server.LoginResult{Label: "stub-login", Kind: "oauth", Secret: "{stub-credential}"}, nil
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	logger, _ := testkit.TestLogger(t)
	h := &harness{
		t:       t,
		cfg:     defaultConfig(),
		db:      testkit.OpenTestDB(t),
		logger:  logger,
		clock:   testkit.NewFakeClock(fixedNow()),
		login:   &stubLogin{},
		secrets: map[string]string{openaiSecret: "sk-contract", responsesRef: "sk-contract", anthropicRef: "sk-contract"},
	}
	h.upstream = newMockUpstream(t)
	h.settingsP = config.ConfigPath(t.TempDir())
	h.settings = appsettings.New(appsettings.Options{
		Retention:    sqlite.NewRetentionSettings(h.db, sqlite.RetentionOptions{Now: h.clock.Now}),
		QuotaDisplay: sqlite.NewQuotaDisplaySettings(h.db),
		ConfigPath:   h.settingsP,
		Clock:        h.clock,
	})
	h.storeBootConfig(t)
	h.events = server.NewEventBus(server.EventBusOptions{Now: h.clock.Now})
	h.sessions = server.NewSessions(h.clock)
	h.server = h.newServer(h.cfg.Server.Port)
	issued, err := h.keys.CreateAccessKey(context.Background(), appaccess.NewAccessKey{
		Name: "contract-agent", Kind: appaccess.Agent, Client: access.CustomClient,
	})
	if err != nil {
		t.Fatalf("CreateAccessKey() error = %v", err)
	}
	h.keyToken = issued.Token
	return h
}

func defaultConfig() *config.Config {
	cfg := config.DefaultConfig()
	cfg.Server.DataPlane = config.DataPlaneConfig{}
	cfg.Admin.Login = true
	return &cfg
}

func (h *harness) storeBootConfig(t *testing.T) {
	t.Helper()
	encoded, err := toml.Marshal(h.cfg)
	if err != nil {
		t.Fatalf("encode the harness config: %v", err)
	}
	if err := os.WriteFile(h.settingsP, encoded, 0o600); err != nil {
		t.Fatalf("write the harness config: %v", err)
	}
}

func (h *harness) newServer(port int) *server.Server {
	h.t.Helper()
	cfg := *h.cfg
	cfg.Server.Port = port
	recorder, err := sqlite.NewUsageRecorder(h.db, h.logger)
	if err != nil {
		h.t.Fatalf("NewUsageRecorder() error = %v", err)
	}
	h.t.Cleanup(func() { _ = recorder.Close() })
	captures := sqlite.NewCaptureStore(h.db)
	h.saveProviders()
	pools := h.newPools(
		account.PoolEntry{ID: "one", ProviderID: "openai", Kind: "api_key", Label: "default", SecretRef: openaiSecret, Status: account.StatusActive},
		account.PoolEntry{ID: "two", ProviderID: "openai-resp", Kind: "api_key", Label: "default", SecretRef: responsesRef, Status: account.StatusActive},
		account.PoolEntry{ID: "three", ProviderID: "anthropic", Kind: "api_key", Label: "default", SecretRef: anthropicRef, Status: account.StatusActive},
	)
	h.pools = pools
	h.catalog = h.newCatalog(pools)
	h.buildServices(pools)
	clientPaths := h.integrationPaths()
	options := server.Options{
		Config:        &cfg,
		SchemaVersion: h.db,
		Logger:        h.logger,
		Clock:         h.clock,
		Version:       testVersion,
		AdminToken:    adminToken,
		Catalog:       h.catalog,
		Router:        routing.New(routing.Options{Catalog: h.catalog, Pools: pools, FailoverBackoff: account.DefaultFailoverBackoff()}),
		Relay: platform.NewRelay(upstream.New(upstream.Options{
			Pools: platform.NewCredentialPools(pools), Secrets: testkit.SecretStore(h.secrets),
			Flows: platform.NewFlowRegistry(oauth.DefaultRegistry()), Now: h.clock.Now,
		}), upstream.NewExecutor(&http.Client{}, h.logger)),
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
		RetryDelay: func(int) time.Duration { return time.Millisecond },
		Integrations: appintegration.NewService(appintegration.ServiceOptions{
			Paths:     platform.NewPathResolver(clientPaths),
			Agents:    platform.NewAgentRegistry(),
			Files:     platform.NewClientFiles(clientPaths),
			Processes: platform.NewAgentProcesses(),
			Wire:      platform.NewWireCodec(),
			Store:     platform.NewIntegrationStore(h.db), Catalog: h.catalog,
			Keys: h.keys, Now: h.clock.Now,
		}),
		Events:       h.events,
		Sessions:     h.sessions,
		Login:        h.login,
		QuotaRefresh: fixedQuotaRefresh{},
		// The update feed stays empty, so the check reports the same
		// no-update answer with and without a network.
		Updates: &updates.Checker{Current: testVersion},
		TemplateSettings: apptemplates.New(apptemplates.Options{
			Store:       templateStore{inner: sqlite.NewTemplateSettings(h.db)},
			Connections: templateConns{},
			Clock:       h.clock,
		}),
		Callbacks: platform.NewCallbackBridge(oauth.NewCallbackBroker(oauth.CallbackBrokerOptions{
			ManagementPort: cfg.Server.Port,
		})),
		CallbackIcon:  oauth.CallbackIcon,
		CallbackPorts: oauth.DefaultRegistry(),
	}
	served := server.New(options)
	served.SetDashboard(stubConsole())
	return served
}

func (h *harness) saveProviders() {
	h.t.Helper()
	if h.saved {
		return
	}
	h.saved = true
	ctx := context.Background()
	repo := sqlite.NewCatalogRepo(h.db)
	providers := []sqlite.ProviderRow{
		{ID: "openai", Origin: string(catalog.OriginCustom), Label: "OpenAI",
			Auth: string(catalog.AuthAPIKey), APIFormat: string(catalog.FormatOpenAIChat),
			BaseURL: h.upstream.URL(), ModelsFormat: string(catalog.ModelsNone),
			Enabled: true, Rank: 100, PoolStrategy: "least-loaded"},
		{ID: "openai-resp", Origin: string(catalog.OriginCustom), Label: "Responses",
			Auth: string(catalog.AuthAPIKey), APIFormat: string(catalog.FormatOpenAIResp),
			BaseURL: h.upstream.URL(), ModelsFormat: string(catalog.ModelsNone),
			Enabled: true, Rank: 90, PoolStrategy: "least-loaded"},
		{ID: "anthropic", Origin: string(catalog.OriginCustom), Label: "Anthropic",
			Auth: string(catalog.AuthAPIKey), APIFormat: string(catalog.FormatAnthropic),
			BaseURL: h.upstream.URL(), ModelsFormat: string(catalog.ModelsNone),
			Enabled: true, Rank: 80, PoolStrategy: "least-loaded"},
	}
	for _, row := range providers {
		if err := repo.SaveProvider(ctx, row); err != nil {
			h.t.Fatalf("save provider %s: %v", row.ID, err)
		}
	}
	models := []sqlite.ModelRow{
		{ProviderID: "openai", ModelID: "gpt-4o", Source: "manual", APIFormat: string(catalog.FormatOpenAIChat), Enabled: true},
		{ProviderID: "openai-resp", ModelID: responsesModel, Source: "manual", APIFormat: string(catalog.FormatOpenAIResp), Enabled: true},
		{ProviderID: "anthropic", ModelID: anthropicModel, Source: "manual", APIFormat: string(catalog.FormatAnthropic), Enabled: true},
	}
	for _, row := range models {
		if err := repo.SaveModel(ctx, row); err != nil {
			h.t.Fatalf("save model %s: %v", row.ModelID, err)
		}
	}
}

func (h *harness) newPools(entries ...account.PoolEntry) *account.Manager {
	h.t.Helper()
	ctx := context.Background()
	repo := sqlite.NewCredentialStore(h.db)
	if _, err := h.db.SQL().ExecContext(ctx, "DELETE FROM credentials"); err != nil {
		h.t.Fatalf("clear credentials: %v", err)
	}
	for _, entry := range entries {
		if err := repo.Insert(ctx, entry); err != nil {
			h.t.Fatalf("insert credential: %v", err)
		}
	}
	pools := account.NewManager(repo, testkit.SecretStore(h.secrets))
	if err := pools.LoadFromDB(ctx); err != nil {
		h.t.Fatalf("LoadFromDB() error = %v", err)
	}
	return pools
}

func (h *harness) newCatalog(pools *account.Manager) *catalog.Catalog {
	h.t.Helper()
	models := catalog.New(sqlite.NewCatalogReader(h.db), platform.NewPoolDirectory(pools), wireformats.New())
	if err := models.Reload(context.Background()); err != nil {
		h.t.Fatalf("reload catalog: %v", err)
	}
	return models
}

func (h *harness) buildServices(pools *account.Manager) {
	store := testkit.SecretStore(h.secrets)
	quiet := platform.NewQuietSecrets(store)
	edges := &platform.AccountEdges{}
	h.accounts = appaccount.New(appaccount.Options{
		Entries: sqlite.NewCredentialStore(h.db), Secrets: quiet, Pools: pools, Catalog: h.catalog,
		Facts: sqlite.NewAccountFactStore(h.db), Providers: edges, Discover: edges,
		Refresh: edges.RefreshAfterCredential, Logger: h.logger,
	})
	h.keys = appaccess.NewKeys(sqlite.NewAccessKeyStore(h.db), h.clock)
	h.service = appcatalog.New(appcatalog.Options{
		Store: sqlite.NewCatalogRepo(h.db), Catalog: h.catalog, Pools: pools,
		Entries: sqlite.NewCredentialStore(h.db), Accounts: h.accounts, Secrets: quiet,
		Direct:    upstream.Direct,
		ModelsDev: modelsdev.NewDirectory(filepath.Join(h.t.TempDir(), "cache"), h.upstream.URL(), nil),
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
		Accounts: h.accounts, Catalog: h.catalog,
	})
}

func (h *harness) integrationPaths() codingclients.Paths {
	h.t.Helper()
	home := h.t.TempDir()
	state := h.t.TempDir()
	h.volatileDirs = []string{home, state}
	h.t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
	h.t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(home, ".claude"))
	return codingclients.Paths{
		Home: home, StateHome: state,
		DataPlane: func(protocol string) string {
			if protocol == inference.ProtocolOpenAI {
				return "http://127.0.0.1:10201"
			}
			return ""
		},
	}
}

// templateStore adapts the SQLite choices table to the template-settings
// port, the same translation the platform wires for the daemon.
type templateStore struct {
	inner *sqlite.TemplateSettings
}

func (s templateStore) Get(ctx context.Context, templateID string) (bool, bool, bool, error) {
	choice, found, err := s.inner.Get(ctx, templateID)
	if err != nil || !found {
		return false, false, found, err
	}
	return choice.LongContext, choice.AutoRefresh, true, nil
}

func (s templateStore) Save(ctx context.Context, templateID string, longContext, autoRefresh *bool, nowMs int64) error {
	return s.inner.Save(ctx, templateID, longContext, autoRefresh, nowMs)
}

// templateConns reports no template behind a connection: the contract seed
// holds custom providers only, so template reads answer with defaults.
type templateConns struct{}

func (templateConns) TemplateID(string) (string, bool) { return "", false }

// fixedQuotaRefresh stands in for the quota worker: pane refreshes answer
// with the same receipt the daemon writes, without probing any vendor.
type fixedQuotaRefresh struct{}

func (fixedQuotaRefresh) ProbeCredential(context.Context, string) ([]activity.Snapshot, error) {
	return nil, nil
}

func (fixedQuotaRefresh) ProbeProvider(context.Context, string) ([]activity.Snapshot, error) {
	return nil, nil
}

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

func (h *harness) management(method, path, token string, body io.Reader) *httptest.ResponseRecorder {
	h.t.Helper()
	req := httptest.NewRequest(method, path, body)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	recorder := httptest.NewRecorder()
	h.server.Handler().ServeHTTP(recorder, req)
	return recorder
}

func (h *harness) dataPlaneOn(protocol, method, path, token string, body io.Reader) *httptest.ResponseRecorder {
	h.t.Helper()
	handler, found := h.server.DataPlaneHandler(protocol)
	if !found {
		h.t.Fatalf("the server serves no %s port", protocol)
	}
	req := httptest.NewRequest(method, path, body)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	return recorder
}

type mockUpstream struct {
	server  *httptest.Server
	mu      sync.Mutex
	fixture string
	sse     bool
	status  int
}

func newMockUpstream(t *testing.T) *mockUpstream {
	t.Helper()
	upstream := &mockUpstream{fixture: "openai/chat_streaming.txt", sse: true, status: http.StatusOK}
	upstream.server = httptest.NewServer(http.HandlerFunc(upstream.serve))
	t.Cleanup(upstream.server.Close)
	return upstream
}

func (u *mockUpstream) serve(w http.ResponseWriter, r *http.Request) {
	_, _ = io.ReadAll(io.LimitReader(r.Body, 1<<20))
	u.mu.Lock()
	fixture, sse, status := u.fixture, u.sse, u.status
	u.mu.Unlock()
	body, err := os.ReadFile(testkit.FixturePath(fixture))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	contentType := "application/json"
	if sse {
		contentType = "text/event-stream"
	}
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

func (u *mockUpstream) URL() string {
	return u.server.URL
}

func (u *mockUpstream) setFixture(fixture string, sse bool) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.fixture, u.sse = fixture, sse
}

// volatileKeys names every JSON field whose value varies between runs: the
// frozen clock still leaves identifiers, secrets, and listener addresses to
// the runtime. The list is explicit so a renamed field fails the comparison
// instead of being silently normalized.
// volatileKeys names every JSON field whose value varies between runs: the
// frozen clock stops the application timestamps, but identifiers, secrets,
// relay delivery stamps, wall-clock persistence timestamps, and listener
// addresses still come from the runtime. The list is explicit so a renamed
// field fails the comparison instead of being silently normalized.
var volatileKeys = map[string]bool{
	"id": true, "item_id": true, "probe_id": true, "request_id": true,
	"token": true, "secret": true, "token_hint": true,
	"access_token": true, "refresh_token": true,
	"uptime_seconds": true, "duration_ms": true, "started": true, "started_at": true,
	"created": true, "created_at": true, "updated_at": true,
	"created_at_ms": true, "updated_at_ms": true, "fetched_at_ms": true,
	"last_attempt_at_ms": true, "started_at_ms": true, "expires_at_ms": true,
	"port": true, "data_plane_port": true, "pid": true,
	"addr": true, "address": true, "base_url": true, "url": true, "source_url": true,
}

// normalize pins every volatile value to its placeholder, keeping field
// names and shapes exact. Non-JSON bodies pass through byte-identical.
func (h *harness) normalize(body []byte) []byte {
	var decoded any
	if err := json.Unmarshal(body, &decoded); err != nil {
		return h.scrubDirs(body)
	}
	var pinned strings.Builder
	encoder := json.NewEncoder(&pinned)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(h.pin(decoded)); err != nil {
		h.t.Fatalf("re-encode the normalized body: %v", err)
	}
	return h.scrubDirs([]byte(strings.TrimSuffix(pinned.String(), "\n")))
}

// scrubDirs substitutes the exact temporary directories this run created,
// which integration previews embed in file paths. The values are known, so
// no pattern ever touches the body.
func (h *harness) scrubDirs(body []byte) []byte {
	text := string(body)
	for _, dir := range h.volatileDirs {
		if dir != "" {
			text = strings.ReplaceAll(text, dir, "<tmpdir>")
		}
	}
	return []byte(text)
}

func (h *harness) pin(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		for key, item := range typed {
			if volatileKeys[key] {
				typed[key] = pinnedValue(item)
			} else {
				typed[key] = h.pin(item)
			}
		}
		return typed
	case []any:
		for i, item := range typed {
			typed[i] = h.pin(item)
		}
		return typed
	default:
		return value
	}
}

func pinnedValue(value any) any {
	switch value.(type) {
	case string:
		return "<volatile>"
	case float64:
		return 0
	case bool:
		return false
	default:
		return value
	}
}

// normalizeStream pins the volatile fields of every JSON payload in a
// server-sent-events body, keeping framing, order, and event names exact.
// The relay stamps each frame with the delivery id and instant, which no
// frozen clock reaches, so those two fields are genuinely volatile.
func (h *harness) normalizeStream(body []byte) []byte {
	lines := strings.Split(string(body), "\n")
	for i, line := range lines {
		payload, ok := strings.CutPrefix(line, "data: ")
		if !ok {
			continue
		}
		trimmed := strings.TrimSpace(payload)
		if !strings.HasPrefix(trimmed, "{") {
			continue
		}
		lines[i] = "data: " + string(h.normalize([]byte(trimmed)))
	}
	return []byte(strings.Join(lines, "\n"))
}
