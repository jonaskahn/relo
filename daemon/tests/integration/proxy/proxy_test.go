package proxy_test

import (
	"bufio"
	"context"
	"github.com/jonaskahn/relo/internal/inference"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/access"
	"github.com/jonaskahn/relo/internal/account"
	"github.com/jonaskahn/relo/internal/adapters/codingclients"
	"github.com/jonaskahn/relo/internal/adapters/discovery"
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
	appstatus "github.com/jonaskahn/relo/internal/application/status"
	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/internal/clock"
	"github.com/jonaskahn/relo/internal/config"
	"github.com/jonaskahn/relo/internal/platform"
	"github.com/jonaskahn/relo/internal/routing"
	"github.com/jonaskahn/relo/internal/server"
	"github.com/jonaskahn/relo/tests/testkit"
	"path/filepath"
)

const (
	adminToken    = "admin-token-value"
	credentialRef = "apikey/openai/one"
)

// dataPlaneToken is the secret of the client key the current daemon issued.
// It is a variable because a secret exists only at the moment it is minted,
// and these tests are not parallel.
var dataPlaneToken string

// TestStreamingProxy checks that an authenticated Chat Completions stream
// travels through a real listener to a mock OpenAI upstream and lands one
// usage row in SQLite.
func TestStreamingProxy(t *testing.T) {
	origin := newUpstream(t, "openai/chat_streaming.txt", true)
	daemon := startDaemon(t, origin.URL())
	token := dataPlaneToken

	t.Run("a streaming request is relayed frame by frame", func(t *testing.T) {
		response := send(t, daemon.dataPlane, "/v1/chat/completions", token, streamingRequest())
		defer func() { _ = response.Body.Close() }()
		if response.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", response.StatusCode)
		}
		if got := response.Header.Get("Content-Type"); got != "text/event-stream" {
			t.Fatalf("Content-Type = %q, want text/event-stream", got)
		}
		body, err := io.ReadAll(response.Body)
		if err != nil {
			t.Fatalf("read stream: %v", err)
		}
		if !strings.Contains(string(body), "Hello") || !strings.Contains(string(body), "data: [DONE]") {
			t.Fatalf("body = %q, want the relayed chunks and the sentinel", body)
		}
		if origin.requestCount() != 1 {
			t.Fatalf("upstream requests = %d, want 1", origin.requestCount())
		}
		if got := origin.lastAuthorization(); got != "Bearer sk-test" {
			t.Fatalf("upstream Authorization = %q, want the stored credential", got)
		}
	})

	t.Run("the request is recorded once in SQLite", func(t *testing.T) {
		event := daemon.lastUsageEvent(t)
		if event.Provider != "openai" || event.Model != "gpt-4o" || event.Surface != "chat-completions" {
			t.Fatalf("event = %+v, want the request identity", event)
		}
		if event.Status != http.StatusOK || event.InputTokens != 9 || event.OutputTokens != 2 {
			t.Fatalf("event = %+v, want the upstream status and token counts", event)
		}
		if event.CredentialLabel != "default" {
			t.Fatalf("credential = %q, want the pool entry label", event.CredentialLabel)
		}
		attempts := daemon.attemptCount(t)
		if attempts != 1 {
			t.Fatalf("usage attempts = %d, want 1", attempts)
		}
	})

	t.Run("a rejected request is not relayed", func(t *testing.T) {
		response := send(t, daemon.dataPlane, "/v1/chat/completions", "wrong-token", streamingRequest())
		defer func() { _ = response.Body.Close() }()
		if response.StatusCode != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", response.StatusCode)
		}
		if origin.requestCount() != 1 {
			t.Fatalf("upstream requests = %d, want the unauthenticated request to stay local", origin.requestCount())
		}
	})

	t.Run("a non-streaming request is relayed as JSON", func(t *testing.T) {
		origin.respond("openai/chat_nonstreaming.json", false)
		response := send(t, daemon.dataPlane, "/v1/chat/completions", token, completeRequest())
		defer func() { _ = response.Body.Close() }()
		if response.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", response.StatusCode)
		}
		if got := response.Header.Get("Content-Type"); got != "application/json" {
			t.Fatalf("Content-Type = %q, want application/json", got)
		}
		body, err := io.ReadAll(response.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if !strings.Contains(string(body), "Hi there") {
			t.Fatalf("body = %q, want the completion", body)
		}
		if count := daemon.usageCount(t); count != 2 {
			t.Fatalf("usage rows = %d, want the second request recorded", count)
		}
	})

	t.Run("an upstream failure is relayed and recorded", func(t *testing.T) {
		origin.respond("openai/chat_error.json", false)
		origin.setStatus(http.StatusTooManyRequests)
		response := send(t, daemon.dataPlane, "/v1/chat/completions", token, completeRequest())
		defer func() { _ = response.Body.Close() }()
		if response.StatusCode != http.StatusTooManyRequests {
			t.Fatalf("status = %d, want the upstream status", response.StatusCode)
		}
		body, err := io.ReadAll(response.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if !strings.Contains(string(body), "Incorrect API key") {
			t.Fatalf("body = %q, want the upstream error", body)
		}
		if got := daemon.lastUsageEvent(t).Status; got != http.StatusTooManyRequests {
			t.Fatalf("recorded status = %d, want the upstream status", got)
		}
	})

	t.Run("the stream arrives before the upstream finishes", func(t *testing.T) {
		origin.respond("openai/chat_streaming.txt", true)
		origin.setDelay(300 * time.Millisecond)
		defer origin.setDelay(0)
		response := send(t, daemon.dataPlane, "/v1/chat/completions", token, streamingRequest())
		defer func() { _ = response.Body.Close() }()
		reader := bufio.NewReader(response.Body)
		started := time.Now()
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("read first frame: %v", err)
		}
		elapsed := time.Since(started)
		if !strings.HasPrefix(line, "data: ") {
			t.Fatalf("first line = %q, want an SSE frame", line)
		}
		if elapsed >= 300*time.Millisecond {
			t.Fatalf("the first frame took %v, want it to arrive while the upstream is still busy", elapsed)
		}
	})
}

// daemon is a running Relo instance bound to ephemeral ports: the management
// listener, and the data plane port a client of one protocol points at.
type daemon struct {
	addr      string
	dataPlane string
	db        *sqlite.DB
	// service is the catalog application layer the management API runs on,
	// and integrations is the coding-client setup layer, which is what a test
	// reaches for when it acts on the daemon rather than on a client.
	service      *appcatalog.Service
	integrations *appintegration.Service
}

// startDaemon runs a daemon whose one provider serves Chat Completions from
// upstreamURL.
func startDaemon(t *testing.T, upstreamURL string) daemon {
	t.Helper()
	return startDaemonWith(t, catalog.FormatOpenAIChat, upstreamURL, "gpt-4o")
}

// startDaemonWith runs a daemon whose only provider speaks one API format
// from baseURL and serves the given models, so a test can relay through any
// wire family. The catalog rows are what the data plane resolves a request
// through: the provider and its models are stored the way the daemon stores
// them, and the credential is the one account the pool holds.
func startDaemonWith(t *testing.T, format catalog.APIFormat, baseURL string, modelIDs ...string) daemon {
	t.Helper()
	return startDaemonSeeded(t, seed{
		Provider: sqlite.ProviderRow{
			ID: "openai", Origin: string(catalog.OriginCustom), Label: "OpenAI",
			Auth: string(catalog.AuthAPIKey), APIFormat: string(format), BaseURL: baseURL,
			ModelsFormat: string(catalog.ModelsNone),
			Headers:      map[string]string{}, Variables: map[string]string{},
			Enabled: true, Rank: 100, PoolStrategy: sqlite.StrategyLeastLoaded,
		},
		Format:  format,
		Models:  modelIDs,
		Account: "sk-test",
	})
}

// seed describes the one provider a test daemon serves, so a test can relay
// through a connection that keeps no account as well as one that does.
type seed struct {
	Provider sqlite.ProviderRow
	Format   catalog.APIFormat
	// ModelRows are the stored models, each with the wire it answers on. A
	// nil slice falls back to one model per id on Format.
	Models []string
	// ModelRows is the stored roster, which a test needs to state per model.
	ModelRows []seedModel
	// Account is the secret the provider's single credential holds. An empty
	// value means the connection keeps none, which is what a keyless
	// connection serves.
	Account string
}

// seedModel is one stored model and the upstream wire it answers on.
type seedModel struct {
	ID     string
	Format catalog.APIFormat
}

// storedModels is the roster this seed writes, so a test states per-model
// wires when it needs them and bare ids when it does not.
func (s seed) storedModels() []seedModel {
	if len(s.ModelRows) > 0 {
		return s.ModelRows
	}
	models := make([]seedModel, 0, len(s.Models))
	for _, id := range s.Models {
		models = append(models, seedModel{ID: id, Format: s.Format})
	}
	return models
}

func startDaemonSeeded(t *testing.T, s seed) daemon {
	t.Helper()
	logger, _ := testkit.TestLogger(t)
	cfg := config.DefaultConfig()
	cfg.Server.Bind = "127.0.0.1"
	cfg.Server.Port = 0
	// The data plane refuses a port of zero, so each protocol asks for one
	// this machine can bind rather than the default another test may hold.
	cfg.Server.DataPlane = config.DataPlaneConfig{OpenAI: freePort(t), Anthropic: freePort(t)}
	db := testkit.OpenTestDB(t)
	recorder, err := sqlite.NewUsageRecorder(db, logger)
	if err != nil {
		t.Fatalf("NewUsageRecorder() error = %v", err)
	}
	t.Cleanup(func() { _ = recorder.Close() })

	ctx := context.Background()
	repo := sqlite.NewCatalogRepo(db)
	if err := repo.SaveProvider(ctx, s.Provider); err != nil {
		t.Fatalf("save the provider: %v", err)
	}
	for _, model := range s.storedModels() {
		if err := repo.SaveModel(ctx, sqlite.ModelRow{
			ProviderID: s.Provider.ID, ModelID: model.ID, Source: "listing",
			APIFormat: string(model.Format), Enabled: true,
		}); err != nil {
			t.Fatalf("save model %s: %v", model.ID, err)
		}
		name, category, status := model.ID, string(catalog.CategoryChat), "active"
		if err := repo.SaveModelFacts(ctx, sqlite.ModelFactsRow{
			ProviderID: s.Provider.ID, ModelID: model.ID, Layer: "override",
			Name: &name, Category: &category, Status: &status,
		}); err != nil {
			t.Fatalf("save model facts %s: %v", model.ID, err)
		}
	}

	secrets := testkit.SecretStore(map[string]string{credentialRef: "sk-test"})
	store := sqlite.NewCredentialStore(db)
	if s.Account != "" {
		if err := store.Insert(ctx, account.PoolEntry{
			ID: "one", ProviderID: s.Provider.ID, Kind: "api_key",
			Label: "default", SecretRef: credentialRef, Status: account.StatusActive,
		}); err != nil {
			t.Fatalf("store the credential: %v", err)
		}
	}
	pools := account.NewManager(store, secrets)
	if err := pools.LoadFromDB(ctx); err != nil {
		t.Fatalf("LoadFromDB() error = %v", err)
	}
	clk := clock.New()
	models := catalog.New(sqlite.NewCatalogReader(db), platform.NewPoolDirectory(pools), wireformats.New())
	if err := models.Reload(ctx); err != nil {
		t.Fatalf("reload the catalog: %v", err)
	}
	credentials := upstream.New(upstream.Options{
		Pools: platform.NewCredentialPools(pools), Secrets: secrets,
		Flows: platform.NewFlowRegistry(oauth.DefaultRegistry()), Now: clk.Now,
	})
	home := t.TempDir()
	keys := appaccess.NewKeys(sqlite.NewAccessKeyStore(db), clk)
	quiet := platform.NewQuietSecrets(secrets)
	edges := &platform.AccountEdges{}
	accounts := appaccount.New(appaccount.Options{
		Entries: store, Secrets: quiet, Pools: pools, Catalog: models,
		Facts: sqlite.NewAccountFactStore(db), Providers: edges, Discover: edges,
		Refresh: edges.RefreshAfterCredential, Logger: logger,
	})
	catalogSvc := appcatalog.New(appcatalog.Options{
		Store: sqlite.NewCatalogRepo(db), Catalog: models, Pools: pools,
		Entries: store, Accounts: accounts, Secrets: quiet,
		Credentials: credentials, Direct: upstream.Direct,
		Discover:  discovery.New(&http.Client{Timeout: 5 * time.Second}),
		ModelsDev: modelsdev.NewDirectory(filepath.Join(home, "cache"), "", nil),
		Templates: platform.TemplateRegistry{}, Logger: logger,
	})
	edges.Catalog = catalogSvc
	routeSvc := approuting.New(approuting.Options{
		Snapshot: catalogSvc, Routes: sqlite.NewCatalogRepo(db), Reload: catalogSvc,
	})
	statuses := appstatus.New(appstatus.Options{
		DB: db,
		Retention: sqlite.NewRetention(db, sqlite.RetentionOptions{
			Logger: logger, Now: clk.Now,
		}),
		Secrets: platform.NewSecretView(secrets), Entries: store, Quotas: platform.NewQuotaStore(db),
		Keys: keys, Accounts: accounts, Catalog: models, Home: home,
	})
	userHome, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("resolve the operator home: %v", err)
	}
	relo, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve the running binary: %v", err)
	}
	clientPaths := codingclients.Paths{
		Home: userHome, StateHome: home, Relo: relo,
		DataPlane: func(protocol string) string {
			address := cfg.DataPlaneAddr(protocol)
			if address == "" {
				return ""
			}
			return "http://" + address
		},
	}
	integrations := appintegration.NewService(appintegration.ServiceOptions{
		Paths:     platform.NewPathResolver(clientPaths),
		Agents:    platform.NewAgentRegistry(),
		Files:     platform.NewClientFiles(clientPaths),
		Processes: platform.NewAgentProcesses(),
		Wire:      platform.NewWireCodec(),
		Store:     platform.NewIntegrationStore(db), Keys: keys, Catalog: models,
		Logger: logger, Now: clk.Now,
	})
	issued, err := keys.CreateAccessKey(ctx, appaccess.NewAccessKey{
		Name: "integration-agent", Kind: appaccess.Agent, Client: access.CustomClient,
	})
	if err != nil {
		t.Fatalf("CreateAccessKey() error = %v", err)
	}
	dataPlaneToken = issued.Token
	instance := server.New(server.Options{
		Config: &cfg, SchemaVersion: db, Logger: logger, Clock: clk, Version: "integration",
		AdminToken: adminToken,
		Catalog:    models, Router: routing.New(routing.Options{Catalog: models, Pools: pools, FailoverBackoff: account.DefaultFailoverBackoff()}),
		Relay:     platform.NewRelay(credentials, upstream.NewExecutor(&http.Client{}, logger)),
		Templates: platform.NewTemplateSource(), CaptureRedactor: platform.NewCaptureRedactor(),
		Pools: pools, Secrets: platform.SecretView{SecretStore: secrets}, Usage: recorder,
		Captures: sqlite.NewCaptureStore(db),
		Activity: appactivity.New(appactivity.Options{
			Usage: sqlite.NewUsageQuery(db), Captures: sqlite.NewCaptureStore(db),
		}),
		Accounts:   accounts,
		Status:     statuses,
		Routes:     routeSvc,
		CatalogAPI: catalogSvc, Keys: keys,
		Formats: wireformats.New(),
		Callbacks: platform.NewCallbackBridge(oauth.NewCallbackBroker(oauth.CallbackBrokerOptions{
			ManagementPort: cfg.Server.Port,
		})),
		CallbackIcon:  oauth.CallbackIcon,
		CallbackPorts: oauth.DefaultRegistry(),
	})
	ctx, cancel := context.WithCancel(context.Background())
	serving := make(chan error, 1)
	go func() { serving <- instance.Start(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-serving:
			if err != nil {
				t.Errorf("Start() error = %v", err)
			}
		case <-time.After(15 * time.Second):
			t.Error("the daemon did not stop")
		}
	})
	return daemon{
		addr:         waitForAddr(t, instance.Addr),
		dataPlane:    waitForDataPlane(t, instance, inference.ProtocolOpenAI),
		db:           db,
		service:      catalogSvc,
		integrations: integrations,
	}
}

// waitForDataPlane waits until the daemon records the address it bound for
// one protocol, which happens once every listener is up.
func waitForDataPlane(t *testing.T, instance *server.Server, protocol string) string {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if addr := instance.DataPlaneAddrs()[protocol]; addr != "" {
			return addr
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("the daemon never bound the %s listener", protocol)
	return ""
}

func (d daemon) usageCount(t *testing.T) int {
	t.Helper()
	var count int
	if err := d.db.SQL().QueryRow("SELECT count(*) FROM usage_events").Scan(&count); err != nil {
		t.Fatalf("count usage events: %v", err)
	}
	return count
}

func (d daemon) attemptCount(t *testing.T) int {
	t.Helper()
	var count int
	if err := d.db.SQL().QueryRow("SELECT count(*) FROM usage_attempts").Scan(&count); err != nil {
		t.Fatalf("count usage attempts: %v", err)
	}
	return count
}

func (d daemon) lastUsageEvent(t *testing.T) sqlite.UsageEvent {
	t.Helper()
	const query = "SELECT request_id, provider, model, credential_label, surface, status, input_tokens, output_tokens" +
		" FROM usage_events ORDER BY id DESC LIMIT 1"
	var event sqlite.UsageEvent
	row := d.db.SQL().QueryRow(query)
	if err := row.Scan(&event.RequestID, &event.Provider, &event.Model, &event.CredentialLabel,
		&event.Surface, &event.Status, &event.InputTokens, &event.OutputTokens); err != nil {
		t.Fatalf("read usage event: %v", err)
	}
	return event
}

// origin is a mock OpenAI endpoint whose behaviour tests can change.
type origin struct {
	server    *httptest.Server
	fixture   string
	sse       bool
	status    int
	delay     time.Duration
	requests  int
	lastAuth  string
	lastError error
}

func newUpstream(t *testing.T, fixture string, sse bool) *origin {
	t.Helper()
	mock := &origin{fixture: fixture, sse: sse, status: http.StatusOK}
	mock.server = httptest.NewServer(http.HandlerFunc(mock.serve))
	t.Cleanup(mock.server.Close)
	return mock
}

func (u *origin) serve(w http.ResponseWriter, r *http.Request) {
	u.requests++
	u.lastAuth = r.Header.Get("Authorization")
	body, err := os.ReadFile(testkit.FixturePath(u.fixture))
	if err != nil {
		u.lastError = err
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if u.delay > 0 {
		time.Sleep(u.delay)
	}
	contentType := "application/json"
	if u.sse {
		contentType = "text/event-stream"
	}
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(u.status)
	_, _ = w.Write(body)
}

func (u *origin) URL() string {
	return u.server.URL
}

func (u *origin) respond(fixture string, sse bool) {
	u.fixture, u.sse, u.status = fixture, sse, http.StatusOK
}

func (u *origin) setStatus(status int) {
	u.status = status
}

func (u *origin) setDelay(delay time.Duration) {
	u.delay = delay
}

func (u *origin) requestCount() int {
	return u.requests
}

func (u *origin) lastAuthorization() string {
	return u.lastAuth
}

func streamingRequest() string {
	return `{"model":"gpt-4o","stream":true,"messages":[{"role":"user","content":"hello"}]}`
}

func completeRequest() string {
	return `{"model":"gpt-4o","stream":false,"messages":[{"role":"user","content":"hello"}]}`
}

func zenCompleteRequest() string {
	return `{"model":"space-bunny-free","stream":false,"messages":[{"role":"user","content":"Say OK."}]}`
}

func zenStreamingRequest() string {
	return `{"model":"space-bunny-free","stream":true,"messages":[{"role":"user","content":"Say OK."}]}`
}

func send(t *testing.T, addr, path, token, body string) *http.Response {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, "http://"+addr+path, strings.NewReader(body))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	return response
}

func waitForAddr(t *testing.T, address func() string) string {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if value := address(); value != "" {
			return value
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("the daemon never bound its listener")
	return ""
}

// freePort returns a port nothing is listening on right now, so a test binds
// the one it asked for instead of the default another test may hold.
func freePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("find a free port: %v", err)
	}
	defer func() { _ = listener.Close() }()
	return listener.Addr().(*net.TCPAddr).Port
}
