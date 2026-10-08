// Dependency wiring: building every service the daemon serves.
package platform

import (
	"context"
	"encoding/json"
	"log/slog"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/jonaskahn/relo/internal/account"
	"github.com/jonaskahn/relo/internal/activity"
	"github.com/jonaskahn/relo/internal/adapters/codingclients"
	"github.com/jonaskahn/relo/internal/adapters/discovery"
	"github.com/jonaskahn/relo/internal/adapters/modelsdev"
	"github.com/jonaskahn/relo/internal/adapters/oauth"
	"github.com/jonaskahn/relo/internal/adapters/quota"
	"github.com/jonaskahn/relo/internal/adapters/secrets"
	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	"github.com/jonaskahn/relo/internal/adapters/templates"
	"github.com/jonaskahn/relo/internal/adapters/upstream"
	"github.com/jonaskahn/relo/internal/adapters/wire"
	wireformats "github.com/jonaskahn/relo/internal/adapters/wire/formats"
	appaccess "github.com/jonaskahn/relo/internal/application/access"
	appaccount "github.com/jonaskahn/relo/internal/application/account"
	appactivity "github.com/jonaskahn/relo/internal/application/activity"
	appcatalog "github.com/jonaskahn/relo/internal/application/catalog"
	appintegration "github.com/jonaskahn/relo/internal/application/integration"
	approuting "github.com/jonaskahn/relo/internal/application/routing"
	appsettings "github.com/jonaskahn/relo/internal/application/settings"
	appstatus "github.com/jonaskahn/relo/internal/application/status"
	apptemplates "github.com/jonaskahn/relo/internal/application/templatesettings"
	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/internal/clock"
	"github.com/jonaskahn/relo/internal/config"
	"github.com/jonaskahn/relo/internal/dashboard"
	// Login items are stored next to the tray code; the daemon uses them
	// without importing the desktop surface.
	"github.com/jonaskahn/relo/internal/i18n"
	"github.com/jonaskahn/relo/internal/platform/autostart"
	"github.com/jonaskahn/relo/internal/routing"
	"github.com/jonaskahn/relo/internal/server"
)

type wired struct {
	secrets secrets.SecretStore
	server  *server.Server
	deps    deps
}

type deps struct {
	config           *config.Config
	db               *sqlite.DB
	logger           *slog.Logger
	home             string
	language         string
	version          string
	catalogs         *i18n.Catalogs
	secrets          secrets.SecretStore
	usage            *sqlite.UsageRecorder
	headerQuota      *activity.Cache
	quotaWorker      *activity.Worker
	clock            clock.Clock
	pools            *account.Manager
	catalog          *catalog.Catalog
	router           *routing.Router
	credentials      *upstream.Source
	catalogSvc       *appcatalog.Service
	integrations     *appintegration.Service
	accounts         *appaccount.Service
	status           *appstatus.Service
	keys             *appaccess.Keys
	routes           *approuting.Service
	settings         *appsettings.Service
	templateSettings *apptemplates.Service
	events           *server.EventBus
	login            server.LoginRunner
	callbacks        *oauth.CallbackBroker
	adminToken       string
	onReady          func(addrs server.Addrs)
	// instanceID and shutdown are what make a headless run one the lifecycle
	// commands own, and are empty for a run that owns the proxy itself.
	instanceID string
	shutdown   func()
	// drain reports when the data plane is idle, which a compaction waits
	// for. It is bound to the server once there is one.
	drain *lateDrain
}

type wiring struct {
	ctx     context.Context
	cfg     *config.Config
	options Options
	db      *sqlite.DB
	logger  *slog.Logger
	clk     clock.Clock
	built   deps

	credRepo         *sqlite.CredentialStore
	templateSettings *sqlite.TemplateSettings
	models           *catalog.Catalog
	edges            *AccountEdges
	refreshClaude    func(context.Context)
}

func buildDeps(ctx context.Context, cfg *config.Config, options Options, db *sqlite.DB, logger *slog.Logger) (wired, error) {
	w := &wiring{ctx: ctx, cfg: cfg, options: options, db: db, logger: logger}
	w.built.config, w.built.db, w.built.logger = cfg, db, logger
	w.built.home, w.built.language, w.built.version = options.Home, options.Language, options.Version
	w.built.onReady, w.built.instanceID, w.built.shutdown = options.OnReady, options.instanceID, options.shutdown
	if err := w.foundation(); err != nil {
		return wired{}, err
	}
	if err := w.catalogLayer(); err != nil {
		return wired{}, err
	}
	w.services()
	w.integrations()
	if err := w.statusLayer(); err != nil {
		return wired{}, err
	}
	return wired{secrets: w.built.secrets, server: newServer(w.built), deps: w.built}, nil
}

func (w *wiring) foundation() error {
	secrets, err := secrets.Detect(w.options.Home, secrets.Options{
		Keychain: w.cfg.Secrets.Keychain, KeyFile: w.cfg.Secrets.KeyFile,
	}, w.logger)
	if err != nil {
		return err
	}
	usage, err := sqlite.NewUsageRecorder(w.db, w.logger)
	if err != nil {
		return err
	}
	clk := clock.New()
	pools := newPoolManager(w.db, secrets, clk, w.cfg)
	// Startup reads run to completion even when the serve context is
	// already cancelled, so a fast shutdown still leaves a coherent state.
	if err := pools.LoadFromDB(context.WithoutCancel(w.ctx)); err != nil {
		return err
	}
	adminToken, err := EnsureAdminToken(w.options.Home)
	if err != nil {
		return err
	}
	w.built.secrets, w.built.usage, w.built.clock = secrets, usage, clk
	w.built.pools, w.built.adminToken = pools, adminToken
	w.built.callbacks = oauth.NewCallbackBroker(oauth.CallbackBrokerOptions{
		ManagementPort: w.cfg.Server.Port,
	})
	w.clk = clk
	return nil
}

func newPoolManager(db *sqlite.DB, secrets secrets.SecretStore, clk clock.Clock, cfg *config.Config) *account.Manager {
	return account.NewManagerWithOptions(sqlite.NewCredentialStore(db), secrets,
		account.ManagerOptions{
			Strategy: QuotaStrategy(db, clk), Clock: clk,
			Models: account.NewModelIndex(clk), ModelRepo: sqlite.NewCredentialModelStore(db),
			FailoverBackoff: failoverBackoffOf(cfg),
		})
}

func (w *wiring) catalogLayer() error {
	templateSettings := sqlite.NewTemplateSettings(w.db)
	models := catalog.New(sqlite.NewCatalogReader(w.db), NewPoolDirectory(w.built.pools), wireformats.New())
	if err := models.Reload(context.WithoutCancel(w.ctx)); err != nil {
		return err
	}
	flows := oauth.DefaultRegistry(oauth.WithHeadless(w.cfg.System.Headless))
	credentials := upstream.New(upstream.Options{
		Pools: NewCredentialPools(w.built.pools), Secrets: w.built.secrets,
		Flows: NewFlowRegistry(flows), Adapters: seedAdapter{}, Policy: templateSettings, Now: w.clk.Now,
	})
	resolver := routing.New(routing.Options{Catalog: models, Pools: w.built.pools, Clock: w.clk, FailoverBackoff: failoverBackoffOf(w.cfg)})
	credRepo := sqlite.NewCredentialStore(w.db)
	keys := appaccess.NewKeys(sqlite.NewAccessKeyStore(w.db), w.clk)
	w.built.catalog, w.built.router, w.built.credentials = models, resolver, credentials
	w.built.keys = keys
	w.credRepo, w.templateSettings, w.models = credRepo, templateSettings, models
	return nil
}

func (w *wiring) services() {
	edges := &AccountEdges{}
	accounts := appaccount.New(appaccount.Options{
		Entries: w.credRepo, Secrets: NewQuietSecrets(w.built.secrets), Pools: w.built.pools, Catalog: w.models,
		Facts:     sqlite.NewAccountFactStore(w.db),
		Providers: edges, Discover: edges, Refresh: edges.RefreshAfterCredential,
		Logger: w.logger,
	})
	catalogSvc := w.newCatalogService(edges, accounts)
	edges.Catalog = catalogSvc
	w.built.routes = approuting.New(approuting.Options{
		Snapshot: catalogSvc, Routes: sqlite.NewCatalogRepo(w.db), Reload: catalogSvc,
	})
	w.built.accounts, w.built.catalogSvc, w.edges = accounts, catalogSvc, edges
}

func (w *wiring) newCatalogService(edges *AccountEdges, accounts *appaccount.Service) *appcatalog.Service {
	return appcatalog.New(appcatalog.Options{
		Store: sqlite.NewCatalogRepo(w.db), Catalog: w.models, Pools: w.built.pools,
		Entries: w.credRepo, Accounts: accounts, Secrets: NewQuietSecrets(w.built.secrets),
		Credentials: w.built.credentials, Direct: upstream.Direct,
		Discover:  discovery.New(NewHTTPClient()),
		ModelsDev: modelsdev.NewDirectory(filepath.Join(w.options.Home, "cache"), w.cfg.Catalog.ModelsDevURL, nil),
		Templates: TemplateRegistry{}, Logger: w.logger,
		ProxyURL: func() string {
			raw, err := config.ReadProxyURL(config.ConfigPath(w.options.Home))
			if err != nil {
				return ""
			}
			return raw
		},
		AfterReload: func(ctx context.Context) {
			if w.refreshClaude != nil {
				w.refreshClaude(ctx)
			}
		},
	})
}

func (w *wiring) integrations() {
	clientPaths := codingclients.Paths{
		Home:      homeDirectory(""),
		StateHome: w.options.Home,
		Relo:      reloPath(""),
		DataPlane: func(protocol string) string { return dataPlaneBaseURL(w.cfg, protocol) },
	}
	integrationSvc := appintegration.NewService(appintegration.ServiceOptions{
		Paths:     NewPathResolver(clientPaths),
		Agents:    NewAgentRegistry(),
		Files:     NewClientFiles(clientPaths),
		Processes: NewAgentProcesses(),
		Wire:      NewWireCodec(),
		Store:     NewIntegrationStore(w.db), Keys: w.built.keys, Catalog: w.models,
		Logger: w.logger, Now: w.clk.Now,
	})
	w.refreshClaude = func(ctx context.Context) {
		integrationSvc.RefreshClaudeGateway(ctx)
		integrationSvc.RefreshCodexCatalog(ctx)
		integrationSvc.RefreshClaudeDesktop(ctx)
	}
	w.built.integrations = integrationSvc
}

func (w *wiring) statusLayer() error {
	drain := &lateDrain{}
	statuses := appstatus.New(appstatus.Options{
		DB: w.db, Retention: sqlite.NewRetention(w.db, sqlite.RetentionOptions{
			Logger: w.logger, Now: w.clk.Now,
		}),
		Secrets: NewSecretView(w.built.secrets), Entries: w.credRepo, Quotas: NewQuotaStore(w.db),
		Keys: w.built.keys, Accounts: w.built.accounts, Catalog: w.models, Home: w.options.Home,
		StartupLog: w.options.StartupLog,
	})
	settings := newSettings(w.db, w.options.Home, w.clk, w.logger, drain)
	templateAPI := newTemplateSettings(w.templateSettings, w.models, w.clk)
	catalogs, err := i18n.Load()
	if err != nil {
		return err
	}
	w.built.drain, w.built.status, w.built.settings = drain, statuses, settings
	w.built.templateSettings, w.built.catalogs = templateAPI, catalogs
	w.built.headerQuota = activity.NewCache(NewQuotaStore(w.db),
		activity.CacheOptions{Clock: w.clk, Source: "header"})
	w.built.quotaWorker = newQuotaWorker(w.db, w.models, w.built.pools, w.built.secrets, w.built.credentials, w.clk, w.logger)
	w.built.events = server.NewEventBus(server.EventBusOptions{})
	w.built.login = loginRunner{logger: w.logger, broker: w.built.callbacks}
	return nil
}

type seedAdapter struct{}

func newSettings(db *sqlite.DB, home string, clk clock.Clock, logger *slog.Logger, drain sqlite.DrainProbe) *appsettings.Service {
	return appsettings.New(appsettings.Options{
		Retention: sqlite.NewRetentionSettings(db, sqlite.RetentionOptions{
			Logger: logger, Now: clk.Now, Drain: drain,
		}),
		QuotaDisplay: sqlite.NewQuotaDisplaySettings(db),
		ConfigPath:   config.ConfigPath(home),
		Clock:        clk,
		ApplyAutostart: func(enabled bool) {
			applyAutostart(home, logger, enabled)
		},
	})
}

func applyAutostart(home string, logger *slog.Logger, enabled bool) {
	exe, err := autostart.Executable()
	if err != nil {
		logger.Warn("could not resolve the executable for start at login", "error", err)
		return
	}
	manager := autostart.New()
	if !enabled {
		if err := manager.Disable(); err != nil {
			logger.Warn("could not change the login item", "error", err)
		}
		return
	}
	if err := manager.Enable(exe); err != nil {
		logger.Warn("could not change the login item", "error", err)
	}
}

func newTemplateSettings(store *sqlite.TemplateSettings, models *catalog.Catalog, clk clock.Clock) *apptemplates.Service {
	return apptemplates.New(apptemplates.Options{
		Store: templateChoiceStore{settings: store}, Connections: templateConnections{catalog: models},
		Clock: clk,
	})
}

type templateChoiceStore struct {
	settings *sqlite.TemplateSettings
}

// Get reads one template's stored context and refresh choices.
func (s templateChoiceStore) Get(ctx context.Context, templateID string) (bool, bool, bool, error) {
	choice, found, err := s.settings.Get(ctx, templateID)
	if err != nil || !found {
		return false, false, found, err
	}
	return choice.LongContext, choice.AutoRefresh, true, nil
}

// Save stores one template's context and refresh choices.
func (s templateChoiceStore) Save(ctx context.Context, templateID string, longContext, autoRefresh *bool, nowMs int64) error {
	return s.settings.Save(ctx, templateID, longContext, autoRefresh, nowMs)
}

type templateConnections struct {
	catalog *catalog.Catalog
}

// TemplateID names the template one connection was created from.
func (c templateConnections) TemplateID(providerID string) (string, bool) {
	snapshot, found := c.catalog.Snapshot()
	if !found {
		return "", false
	}
	host, ok := snapshot.Provider(providerID)
	if !ok {
		return "", false
	}
	return host.TemplateID, true
}

// Authorizer returns the adapter for one sign-in provider.
func (seedAdapter) Authorizer(providerID string) (upstream.Authorizer, bool) {
	adapter, found := templates.Authorizer(providerID)
	if !found {
		return nil, false
	}
	return upstream.Authorizer(adapter), true
}

type loginRunner struct {
	logger *slog.Logger
	broker *oauth.CallbackBroker
}

// Login runs one authorization flow and hands back the credential it
// produced, which the caller stores. A browser login registers through the
// broker the request carries, which is the one the daemon serves the
// callback page from; a login with no broker renders its own page on the
// loopback listener the provider calls back on.
func (r loginRunner) Login(ctx context.Context, request server.LoginRequest) (server.LoginResult, error) {
	registry := oauth.DefaultRegistry(oauth.WithCallbacks(r.broker))
	// The request names the login to run, which is the provider's own flow
	// unless the operator picked another way in.
	flowID := request.Flow
	if flowID == "" {
		flowID = request.ProviderID
	}
	flow, err := registry.Flow(flowID)
	if err != nil {
		return server.LoginResult{}, err
	}
	credential, err := runLoginFlow(ctx, flow, request)
	if err != nil {
		return server.LoginResult{}, err
	}
	encoded, err := json.Marshal(credential)
	if err != nil {
		return server.LoginResult{}, err
	}
	return server.LoginResult{
		Label: firstNonEmptyLabel(request.Label, credential.Email, credential.AccountID),
		Kind:  string(catalog.AuthOAuth), Secret: string(encoded),
	}, nil
}

func runLoginFlow(ctx context.Context, flow oauth.OAuthFlow, request server.LoginRequest) (*oauth.OAuthCredential, error) {
	return flow.Login(ctx, oauth.LoginOpts{
		NoBrowser: true,
		Prompt: func(prompt oauth.AuthPrompt) {
			if request.Prompt != nil {
				_ = request.Prompt(server.LoginPrompt{
					URL: prompt.URL, DeviceCode: prompt.DeviceCode, Instructions: prompt.Instructions,
					Ticket: prompt.Ticket,
				})
			}
		},
	})
}

const maintenanceInterval = 15 * time.Minute

const refreshInterval = time.Minute

const discoveryInterval = 24 * time.Hour

const discoveryDelay = 60 * time.Second

func startRefreshing(ctx context.Context, built deps) {
	guardian := oauth.NewGuardian(
		oauth.DefaultRegistry(),
		NewOAuthCredentials(built.db, built.secrets, built.pools),
		oauth.GuardianOptions{Clock: built.clock, Logger: built.logger, Policy: sqlite.NewTemplateSettings(built.db)},
	)
	go guardian.Run(ctx, refreshInterval)
}

func startDiscovery(ctx context.Context, built deps) {
	go func() {
		if !sleep(ctx, discoveryDelay) {
			return
		}
		discoverEveryConfigured(ctx, built)
		ticker := built.clock.NewTicker(discoveryInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C():
				discoverEveryConfigured(ctx, built)
			}
		}
	}()
}

func discoverEveryConfigured(ctx context.Context, built deps) {
	snapshot, found := built.catalog.Snapshot()
	if !found {
		return
	}
	for _, host := range snapshot.Providers {
		if !host.Configured {
			continue
		}
		if _, err := built.catalogSvc.RefreshProviderModels(ctx, host.ID); err != nil {
			built.logger.Debug("skip model discovery", "provider", host.ID, "error", err)
		}
	}
}

func sleep(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func startMaintenance(ctx context.Context, daemon *server.Server, built deps) {
	retention := sqlite.NewRetention(built.db, sqlite.RetentionOptions{
		Logger: built.logger, Drain: daemon,
	})
	go func() {
		retention.MaintainOnce(ctx)
		if err := retention.Run(ctx, maintenanceInterval); err != nil {
			built.logger.Warn("retention stopped", "error", err)
		}
	}()
}

type lateDrain struct {
	server atomic.Pointer[server.Server]
}

func (d *lateDrain) bind(bound *server.Server) {
	d.server.Store(bound)
}

// Drained reports whether the bound server finished its in-flight requests,
// true while no server is bound yet.
func (d *lateDrain) Drained() bool {
	if d == nil {
		return true
	}
	bound := d.server.Load()
	return bound == nil || bound.Drained()
}

func newServer(built deps) *server.Server {
	options := newServerOptions(built)
	served := server.New(options)
	built.drain.bind(served)
	// The console is the build this binary embedded; a binary compiled
	// without one serves a stub that reports it.
	served.SetDashboard(dashboard.Handler(dashboard.Options{
		Language: func() string { return consoleLanguage(built) },
		Appearance: func() (string, string) {
			ui, err := config.ReadAppearance(config.ConfigPath(built.home))
			if err != nil {
				return config.DefaultUITheme, config.DefaultUIAccent
			}
			return ui.Theme, ui.Accent
		},
	}))
	if built.onReady != nil {
		ready := built.onReady
		served.OnReady(func(addrs server.Addrs) { ready(addrs) })
	}
	return served
}

func newServerOptions(built deps) server.Options {
	captures := sqlite.NewCaptureStore(built.db)
	options := coreServerOptions(built, captures)
	attachServerAPIs(&options, built)
	attachServerRestart(&options, built)
	attachServerQuota(&options, built)
	return options
}

func coreServerOptions(built deps, captures *sqlite.CaptureStore) server.Options {
	return server.Options{
		Config:          built.config,
		SchemaVersion:   built.db,
		Logger:          built.logger,
		Version:         built.version,
		AdminToken:      built.adminToken,
		Catalog:         built.catalog,
		Router:          built.router,
		Relay:           NewRelay(built.credentials, upstream.NewExecutor(NewInferenceClient(), built.logger)),
		Templates:       NewTemplateSource(),
		CaptureRedactor: NewCaptureRedactor(),
		Pools:           built.pools,
		Secrets:         secretModeSource(built.secrets),
		Usage:           built.usage,
		Captures:        captures,
		Activity: appactivity.New(appactivity.Options{
			Usage: sqlite.NewUsageQuery(built.db), Captures: captures,
		}),
		Formats:       wireformats.New(),
		Ports:         NewPortInspector(),
		Callbacks:     NewCallbackBridge(built.callbacks),
		CallbackIcon:  oauth.CallbackIcon,
		CallbackPorts: oauth.DefaultRegistry(),
	}
}

func attachServerAPIs(options *server.Options, built deps) {
	options.Accounts = built.accounts
	options.Status = built.status
	options.Routes = built.routes
	options.CatalogAPI = built.catalogSvc
	options.Integrations = built.integrations
	options.Quota = built.headerQuota
	options.Keys = built.keys
	options.Settings = built.settings
	options.TemplateSettings = built.templateSettings
	options.Events = built.events
	options.Login = built.login
	options.Catalogs = built.catalogs
	options.Language = built.language
	options.InstanceID = built.instanceID
	options.Shutdown = built.shutdown
}

func attachServerRestart(options *server.Options, built deps) {
	if built.shutdown == nil {
		return
	}
	home := built.home
	logger := built.logger
	options.Restart = func(force bool) {
		time.Sleep(150 * time.Millisecond)
		if err := SpawnRestart(home, force); err != nil {
			logger.Error("could not start the restart helper", "error", err)
		}
	}
}

func attachServerQuota(options *server.Options, built deps) {
	if built.quotaWorker == nil {
		return
	}
	options.QuotaFreshness = built.quotaWorker
	options.QuotaRefresh = built.quotaWorker
}

func consoleLanguage(built deps) string {
	if tag, err := built.settings.Language(); err == nil && tag != i18n.Auto {
		return tag
	}
	return built.language
}

const defaultLabel = "default"

func firstNonEmptyLabel(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return defaultLabel
}

// The sign-in connections that publish a quota endpoint, so they are the
// ones the background prober asks.
const (
	antigravityTemplate = "google-antigravity"
	codexTemplate       = "openai-codex"
	grokTemplate        = "grok"
	cursorTemplate      = "cursor"
	copilotTemplate     = "copilot"
	kimiTemplate        = "kimi"
	claudeTemplate      = "claude"
	devinTemplate       = "devin"
)

func newQuotaWorker(db *sqlite.DB, models *catalog.Catalog, pools *account.Manager,
	secrets secrets.SecretStore, source *upstream.Source, clk clock.Clock, logger *slog.Logger) *activity.Worker {
	probers := quotaProbers(models, clk)
	resolver := quotaProberResolver(models, clk)
	cache := activity.NewCache(NewQuotaStore(db),
		activity.CacheOptions{Clock: clk, Source: "probe"})
	credentials := NewQuotaCredentials(db, secrets, pools,
		func(providerID string) string { return connectionBaseURL(models, providerID) })
	return activity.NewWorker(cache, activity.WorkerOptions{
		Probers: probers, Resolver: resolver, Source: credentials, Marker: credentials,
		Refresher: oauthProbeRefresh{source: source},
		Clock:     clk, Logger: logger,
	})
}

func startQuota(ctx context.Context, built deps) {
	if built.quotaWorker == nil {
		return
	}
	go built.quotaWorker.Run(ctx)
}

func quotaProbers(models *catalog.Catalog, clk clock.Clock) map[string]activity.QuotaProber {
	probers := map[string]activity.QuotaProber{}
	snapshot, found := models.Snapshot()
	if !found {
		return probers
	}
	for _, host := range snapshot.Providers {
		if wire.OpenCodeGo(host.BaseURL) {
			probers[host.ID] = quota.NewOpenCodeGoProber(quota.ProberOptions{BaseURL: quotaProbeBase(host.BaseURL)})
			continue
		}
		if prober, found := apiKeyBalanceProber(host.BaseURL); found {
			probers[host.ID] = prober
			continue
		}
		if quota.OpenRouterHost(host.BaseURL) {
			probers[host.ID] = quota.NewOpenRouterProber(quota.SignInProberOptions{})
			continue
		}
		if quota.ZAIHost(host.BaseURL) {
			probers[host.ID] = quota.NewZAIProber(quota.SignInProberOptions{})
			continue
		}
		if prober, found := templateQuotaProber(host.TemplateID, host.BaseURL, clk); found {
			probers[host.ID] = prober
		}
	}
	return probers
}

func templateQuotaProber(templateID, baseURL string, clk clock.Clock) (activity.QuotaProber, bool) {
	switch templateID {
	case antigravityTemplate:
		return quota.NewAntigravityProber(quota.ProberOptions{
			BaseURL: quotaProbeBase(baseURL),
		}), true
	case codexTemplate:
		return quota.NewCodexProber(quota.ProberOptions{Now: clk.Now}), true
	case claudeTemplate:
		return quota.NewClaudeProber(quota.SignInProberOptions{}), true
	case devinTemplate:
		return quota.NewDevinProber(quota.SignInProberOptions{}), true
	default:
		return nil, false
	}
}

func quotaProbeProber(providerID, baseURL string, clk clock.Clock) (activity.QuotaProber, bool) {
	if wire.OpenCodeGo(baseURL) {
		return quota.NewOpenCodeGoProber(quota.ProberOptions{BaseURL: quotaProbeBase(baseURL)}), true
	}
	if prober, found := apiKeyBalanceProber(baseURL); found {
		return prober, true
	}
	if quota.OpenRouterHost(baseURL) {
		return quota.NewOpenRouterProber(quota.SignInProberOptions{}), true
	}
	if quota.ZAIHost(baseURL) {
		return quota.NewZAIProber(quota.SignInProberOptions{}), true
	}
	return namedQuotaProber(providerID, baseURL, clk)
}

func namedQuotaProber(providerID, baseURL string, clk clock.Clock) (activity.QuotaProber, bool) {
	switch providerID {
	case grokTemplate:
		return quota.NewGrokProber(quota.SignInProberOptions{}), true
	case cursorTemplate:
		return quota.NewCursorProber(quota.SignInProberOptions{}), true
	case copilotTemplate:
		return quota.NewCopilotProber(quota.SignInProberOptions{}), true
	case kimiTemplate:
		return quota.NewKimiProber(quota.SignInProberOptions{}), true
	case antigravityTemplate:
		return quota.NewAntigravityProber(quota.ProberOptions{BaseURL: quotaProbeBase(baseURL)}), true
	case codexTemplate:
		return quota.NewCodexProber(quota.ProberOptions{Now: clk.Now}), true
	case claudeTemplate:
		return quota.NewClaudeProber(quota.SignInProberOptions{}), true
	case devinTemplate:
		return quota.NewDevinProber(quota.SignInProberOptions{}), true
	default:
		return nil, false
	}
}

func apiKeyBalanceProber(baseURL string) (activity.QuotaProber, bool) {
	switch {
	case quota.DeepSeekHost(baseURL):
		return quota.NewDeepSeekProber(quota.ProberOptions{}), true
	case quota.TeamoRouterHost(baseURL):
		return quota.NewTeamoRouterProber(quota.ProberOptions{}), true
	case quota.OrcaRouterHost(baseURL):
		return quota.NewOrcaRouterProber(quota.ProberOptions{}), true
	case quota.VercelGatewayHost(baseURL):
		return quota.NewVercelGatewayProber(quota.ProberOptions{}), true
	case quota.MoonshotHost(baseURL):
		return quota.NewMoonshotProber(quota.ProberOptions{}), true
	case quota.SiliconFlowHost(baseURL):
		return quota.NewSiliconFlowProber(quota.ProberOptions{}), true
	case quota.NovitaHost(baseURL):
		return quota.NewNovitaProber(quota.ProberOptions{}), true
	default:
		return nil, false
	}
}

func quotaProberResolver(models *catalog.Catalog, clk clock.Clock) func(activity.Credential) (activity.QuotaProber, bool) {
	return func(credential activity.Credential) (activity.QuotaProber, bool) {
		baseURL := credential.BaseURL
		if baseURL == "" {
			baseURL = connectionBaseURL(models, credential.ProviderID)
		}
		return quotaProbeProber(credential.ProviderID, baseURL, clk)
	}
}

func quotaProbeBase(baseURL string) string {
	return strings.TrimSuffix(strings.TrimSpace(baseURL), "/")
}

func connectionBaseURL(models *catalog.Catalog, providerID string) string {
	snapshot, found := models.Snapshot()
	if !found {
		return ""
	}
	host, found := snapshot.Provider(providerID)
	if !found {
		return ""
	}
	return host.BaseURL
}
