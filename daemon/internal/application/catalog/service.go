// Package catalog owns the connection and model lifecycle an operator
// manages: connections and their accounts' model lists, hand-added and
// cloned models, the models.dev pricing copy, and probes. It reads every
// dependency through the ports below, so it never reaches for a database,
// an HTTP client, or a file.
package catalog

import (
	"context"
	"log/slog"
	"sync"

	"github.com/jonaskahn/relo/internal/account"
	appaccount "github.com/jonaskahn/relo/internal/application/account"
	"github.com/jonaskahn/relo/internal/catalog"
)

// Store persists connections, models, and their detail layers.
type Store interface {
	GetProvider(ctx context.Context, id string) (catalog.ConnectionRecord, error)
	SaveProvider(ctx context.Context, conn catalog.ConnectionRecord) error
	DeleteProvider(ctx context.Context, id string) error
	GetModel(ctx context.Context, providerID, modelID string) (catalog.ModelRecord, error)
	SaveModel(ctx context.Context, model catalog.ModelRecord) error
	DeleteModelRow(ctx context.Context, providerID, modelID string) error
	ListModelFacts(ctx context.Context, providerID, modelID string) ([]catalog.FactsRecord, error)
	SaveModelFacts(ctx context.Context, facts catalog.FactsRecord) error
	DeleteModelFactsLayer(ctx context.Context, providerID, modelID, layer string) error
	CommitProbe(ctx context.Context, conn catalog.ConnectionRecord, cred account.PoolEntry, models []catalog.ModelRecord, facts []catalog.FactsRecord) error
	RecordRefresh(ctx context.Context, providerID string, atMs int64, refreshErr string) error
	RefreshProviderModels(ctx context.Context, providerID string, models []catalog.ModelRecord, facts []catalog.FactsRecord, unavailableIDs []string, atMs int64, refreshErr string) error
	SaveModelsDevFacts(ctx context.Context, facts []catalog.FactsRecord, state catalog.ModelsDevStateRecord) error
	GetModelsDevState(ctx context.Context) (catalog.ModelsDevStateRecord, bool, error)
}

// SecretStore keeps credential secrets. Deleting a secret that is already
// gone is a no-op.
type SecretStore interface {
	Set(ref, value string) error
	Delete(ref string) error
}

// Credentials resolves the credential a listing of one connection runs with.
type Credentials interface {
	Authorize(ctx context.Context, request catalog.AuthRequest) (catalog.Authorization, error)
	AuthorizeCredential(ctx context.Context, request catalog.AuthRequest, credentialID string) (catalog.Authorization, error)
}

// DirectAuthorizer resolves a credential Relo has not stored yet, which is
// what a connection probe tests with.
type DirectAuthorizer func(ctx context.Context, request catalog.AuthRequest, secret string) (catalog.Authorization, error)

// ModelLister reads the model list one connection publishes.
type ModelLister interface {
	List(ctx context.Context, target catalog.ListTarget) ([]catalog.Listed, error)
}

// ModelsDev is the models.dev copy a surface prices and searches through.
type ModelsDev interface {
	Get(ctx context.Context, mode catalog.ModelsDevFetchMode) (*catalog.ModelsDevIndex, error)
	Cached() (*catalog.ModelsDevIndex, error)
	CurrentState() catalog.ModelsDevState
}

// Templates is the registry of provider templates a connection is added
// from.
type Templates interface {
	Get(id string, idx *catalog.ModelsDevIndex) (catalog.Template, bool)
	All(idx *catalog.ModelsDevIndex) []catalog.Template
	Curated(id string) (catalog.Template, bool)
	RewrittenLabel(templateID, stored string) (string, bool)
}

// Accounts is the credential lifecycle the catalog coordinates with: a
// verified probe commits through it, and its lock serializes credential
// writes with connection removal.
type Accounts interface {
	AddAccount(ctx context.Context, draft appaccount.NewAccount) (appaccount.Account, error)
	WithLock(fn func())
}

// Options configure the catalog use cases.
type Options struct {
	Store       Store
	Catalog     *catalog.Catalog
	Pools       *account.Manager
	Entries     account.CredentialRepository
	Accounts    Accounts
	Secrets     SecretStore
	Credentials Credentials
	Direct      DirectAuthorizer
	Discover    ModelLister
	ModelsDev   ModelsDev
	Templates   Templates
	Logger      *slog.Logger
	// ProxyURL is the outbound proxy from Settings. Empty means no proxy.
	ProxyURL func() string
	// AfterReload runs after a successful rebuild. The composition root uses
	// it to refresh files that mirror the listing. It must not fail the reload.
	AfterReload func(ctx context.Context)
}

// Service runs the connection and model lifecycle.
type Service struct {
	store       Store
	catalog     *catalog.Catalog
	pools       *account.Manager
	entries     account.CredentialRepository
	accounts    Accounts
	secrets     SecretStore
	credentials Credentials
	direct      DirectAuthorizer
	discover    ModelLister
	modelsDev   ModelsDev
	templates   Templates
	logger      *slog.Logger
	proxyURL    func() string
	afterReload func(ctx context.Context)
	probes      sync.Map // probeID -> *probeSession
}

// New returns the catalog use cases over the given ports.
func New(options Options) *Service {
	logger := options.Logger
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &Service{
		store: options.Store, catalog: options.Catalog, pools: options.Pools,
		entries:  options.Entries,
		accounts: options.Accounts,
		secrets:  options.Secrets, credentials: options.Credentials, direct: options.Direct,
		discover: options.Discover, modelsDev: options.ModelsDev, templates: options.Templates,
		logger: logger, proxyURL: options.ProxyURL, afterReload: options.AfterReload,
	}
}

// Snapshot returns the live catalog snapshot, which is what route validation
// reads.
func (s *Service) Snapshot() (*catalog.Snapshot, error) {
	return s.snapshot()
}

// Reload rebuilds the runtime catalog after a write.
func (s *Service) Reload(ctx context.Context) error {
	return s.reload(ctx)
}

func (s *Service) forgetSecret(ref string) error {
	if ref == "" || s.secrets == nil {
		return nil
	}
	return s.secrets.Delete(ref)
}

// Gate serves the account use cases only the catalog can answer: how a
// connection authenticates, and removing it when its last credential goes.
type Gate struct{ svc *Service }

// Gate returns the account-facing edges of this service.
func (s *Service) Gate() *Gate {
	return &Gate{svc: s}
}

// Auth reports how a connection authenticates.
func (g *Gate) Auth(ctx context.Context, id string) (catalog.Auth, error) {
	row, err := g.svc.providerRow(ctx, id)
	if err != nil {
		return "", err
	}
	return catalog.Auth(row.Auth), nil
}

// Delete removes a connection whose last credential is being removed. The
// caller already holds the account lock, so this does not take it again.
func (g *Gate) Delete(ctx context.Context, id string) error {
	return g.svc.deleteProvider(ctx, id)
}
