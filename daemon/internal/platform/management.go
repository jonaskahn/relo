// Management surface: opening the operator-facing services.
package platform

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"

	"github.com/jonaskahn/relo/internal/account"
	"github.com/jonaskahn/relo/internal/adapters/codingclients"
	"github.com/jonaskahn/relo/internal/adapters/discovery"
	"github.com/jonaskahn/relo/internal/adapters/modelsdev"
	"github.com/jonaskahn/relo/internal/adapters/oauth"
	"github.com/jonaskahn/relo/internal/adapters/secrets"
	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	"github.com/jonaskahn/relo/internal/adapters/upstream"
	wireformats "github.com/jonaskahn/relo/internal/adapters/wire/formats"
	appaccess "github.com/jonaskahn/relo/internal/application/access"
	appaccount "github.com/jonaskahn/relo/internal/application/account"
	appcatalog "github.com/jonaskahn/relo/internal/application/catalog"
	appintegration "github.com/jonaskahn/relo/internal/application/integration"
	appsettings "github.com/jonaskahn/relo/internal/application/settings"
	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/internal/clock"
	"github.com/jonaskahn/relo/internal/config"
)

// Management is one offline management session: the use cases every command
// talks to and the database handle behind them. A daemon may be running at
// the same time, so the session only opens what SQLite lets several
// processes share.
type Management struct {
	catalog  *appcatalog.Service
	accounts *appaccount.Service
	keys     *appaccess.Keys
	settings *appsettings.Service
	db       *sqlite.DB
}

// OpenManagement opens the state database and builds the use cases a
// management command runs against, syncing the catalog first so a command
// sees the connections this binary ships.
func OpenManagement(ctx context.Context, home string, logger *slog.Logger) (*Management, error) {
	if err := config.EnsureReloHome(home); err != nil {
		return nil, err
	}
	db, err := sqlite.OpenDB(config.DatabasePath(home), defaultLogger(logger))
	if err != nil {
		return nil, err
	}
	if err := sqlite.Migrate(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	built, err := newManagement(ctx, home, db, logger)
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	return built, nil
}

type managementWiring struct {
	ctx    context.Context
	home   string
	db     *sqlite.DB
	logger *slog.Logger
	cfg    config.Config
	clk    clock.Clock

	secrets          secrets.SecretStore
	pools            *account.Manager
	models           *catalog.Catalog
	templateSettings *sqlite.TemplateSettings
	credRepo         *sqlite.CredentialStore
	credentials      *upstream.Source
	accounts         *appaccount.Service
	catalogSvc       *appcatalog.Service
}

func newManagement(ctx context.Context, home string, db *sqlite.DB, logger *slog.Logger) (*Management, error) {
	w := &managementWiring{ctx: ctx, home: home, db: db, logger: logger}
	if err := w.foundation(); err != nil {
		return nil, err
	}
	if err := w.catalog(); err != nil {
		return nil, err
	}
	w.services()
	return &Management{
		catalog: w.catalogSvc, accounts: w.accounts,
		keys:     appaccess.NewKeys(sqlite.NewAccessKeyStore(db), w.clk),
		settings: newSettings(db, home, w.clk, logger, nil), db: db,
	}, nil
}

func (w *managementWiring) foundation() error {
	cfg, err := config.LoadOffline(w.home)
	if err != nil {
		return err
	}
	secrets, err := secrets.Detect(w.home, secrets.Options{
		Keychain: cfg.Secrets.Keychain, KeyFile: cfg.Secrets.KeyFile,
	}, defaultLogger(w.logger))
	if err != nil {
		return err
	}
	clk := clock.New()
	pools := newPoolManager(w.db, secrets, clk, &cfg)
	if err := pools.LoadFromDB(w.ctx); err != nil {
		return err
	}
	w.cfg, w.secrets, w.clk, w.pools = cfg, secrets, clk, pools
	w.credRepo = sqlite.NewCredentialStore(w.db)
	return nil
}

func (w *managementWiring) catalog() error {
	w.templateSettings = sqlite.NewTemplateSettings(w.db)
	w.models = catalog.New(sqlite.NewCatalogReader(w.db), NewPoolDirectory(w.pools), wireformats.New())
	return w.models.Reload(w.ctx)
}

func (w *managementWiring) services() {
	w.credentials = upstream.New(upstream.Options{
		Pools: NewCredentialPools(w.pools), Secrets: w.secrets,
		Flows: NewFlowRegistry(oauth.DefaultRegistry(oauth.WithHeadless(w.cfg.System.Headless))), Adapters: seedAdapter{},
		Policy: w.templateSettings, Now: w.clk.Now,
	})
	edges := &AccountEdges{}
	w.accounts = appaccount.New(appaccount.Options{
		Entries: w.credRepo, Secrets: NewQuietSecrets(w.secrets), Pools: w.pools, Catalog: w.models,
		Facts:     sqlite.NewAccountFactStore(w.db),
		Providers: edges, Discover: edges, Refresh: edges.RefreshAfterCredential,
		Logger: w.logger,
	})
	w.catalogSvc = w.newManagementCatalog(edges)
	edges.Catalog = w.catalogSvc
}

func (w *managementWiring) newManagementCatalog(edges *AccountEdges) *appcatalog.Service {
	refreshClaude := w.managementRefresher()
	catalogSvc := appcatalog.New(appcatalog.Options{
		Store: sqlite.NewCatalogRepo(w.db), Catalog: w.models, Pools: w.pools,
		Entries: w.credRepo, Accounts: w.accounts, Secrets: NewQuietSecrets(w.secrets),
		Credentials: w.credentials, Direct: upstream.Direct,
		Discover:  discovery.New(NewHTTPClient()),
		ModelsDev: modelsdev.NewDirectory(filepath.Join(w.home, "cache"), w.cfg.Catalog.ModelsDevURL, nil),
		Templates: TemplateRegistry{}, Logger: w.logger, Lifetime: w.ctx,
		ProxyURL: func() string {
			raw, err := config.ReadProxyURL(config.ConfigPath(w.home))
			if err != nil {
				return ""
			}
			return raw
		},
		AfterReload: func(ctx context.Context) {
			if refreshClaude != nil {
				refreshClaude(ctx)
			}
		},
	})
	return catalogSvc
}

func (w *managementWiring) managementRefresher() func(context.Context) {
	clientPaths := codingclients.Paths{
		Home: homeDirectory(""), StateHome: w.home, Relo: reloPath(""),
		DataPlane: func(protocol string) string { return dataPlaneBaseURL(&w.cfg, protocol) },
	}
	integrations := appintegration.NewService(appintegration.ServiceOptions{
		Paths:     NewPathResolver(clientPaths),
		Agents:    NewAgentRegistry(),
		Files:     NewClientFiles(clientPaths),
		Processes: NewAgentProcesses(),
		Wire:      NewWireCodec(),
		Store:     NewIntegrationStore(w.db), Catalog: w.models, Logger: defaultLogger(w.logger),
	})
	return func(ctx context.Context) {
		integrations.RefreshClaudeGateway(ctx)
		integrations.RefreshCodexCatalog(ctx)
		integrations.RefreshClaudeDesktop(ctx)
	}
}

// Catalog returns the connection and model use cases this session runs
// commands through.
func (m *Management) Catalog() *appcatalog.Service {
	return m.catalog
}

// Accounts returns the credential lifecycle this session runs commands
// through.
func (m *Management) Accounts() *appaccount.Service {
	return m.accounts
}

// Keys returns the client-key lifecycle this session runs commands through.
func (m *Management) Keys() *appaccess.Keys {
	return m.keys
}

// Settings returns the settings use cases this session runs commands through.
func (m *Management) Settings() *appsettings.Service {
	return m.settings
}

// DB returns the database handle of this session.
func (m *Management) DB() *sqlite.DB {
	return m.db
}

// Close releases the database handle.
func (m *Management) Close() error {
	return m.db.Close()
}

func defaultLogger(logger *slog.Logger) *slog.Logger {
	if logger != nil {
		return logger
	}
	return discardLogger()
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
