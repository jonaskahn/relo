// Package account owns credential lifecycle: adding a key or a sign-in to a
// connection, pausing and ranking it, and removing it when the operator does.
package account

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	pool "github.com/jonaskahn/relo/internal/account"
	"github.com/jonaskahn/relo/internal/catalog"
)

const (
	credentialIDBytes = 16
	shortIDLength     = 8
	defaultLabel      = "default"
	// MinPriority is the lowest rank a credential can carry.
	MinPriority = -100
	// MaxPriority is the highest rank a credential can carry.
	MaxPriority       = 100
	maskPrefix        = 4
	maskSuffix        = 2
	maskPlaceholder   = "-"
	oauthSecretMarker = "{"
	kindOAuth         = "oauth"
	kindAWSKeys       = "aws_keys"
	kindGCP           = "gcp_service_account"
)

var (
	// ErrUnknownProvider reports a provider Relo has no row for.
	ErrUnknownProvider = errors.New("unknown provider")
	// ErrEmptySecret reports an API-key credential with no key.
	ErrEmptySecret = errors.New("the secret must not be empty")
	// ErrAccountNotFound reports a credential id that matches nothing.
	ErrAccountNotFound = errors.New("account not found")
	// ErrPriorityRange reports a rank outside the range the pool orders by.
	ErrPriorityRange = errors.New("priority must be between -100 and 100")
	// ErrNoSecretStore reports a write that needs somewhere to keep a secret.
	ErrNoSecretStore = errors.New("no secret store is configured")
	// ErrDuplicateAccount reports a credential the provider already stores.
	ErrDuplicateAccount = errors.New("this credential is already stored for the provider")
	// ErrProviderNotFound reports a connection id that matches nothing.
	ErrProviderNotFound = errors.New("provider not found")
	// ErrSharedRoster reports an account whose provider publishes one model list.
	ErrSharedRoster = errors.New("this connection publishes one model list for every account")
	// ErrInvalidCatalogRow reports a catalog write the rules refuse.
	ErrInvalidCatalogRow = errors.New("invalid catalog row")
)

// Account is one stored credential as every surface reports it.
type Account struct {
	ID         string
	ProviderID string
	Kind       string
	Label      string
	Status     string
	Priority   int
	SecretMask string
	// LimitState is the credential breaker's live state: ready, limited
	// while a backoff runs, or probing on the half-open trial.
	LimitState     string
	LimitedUntilMs int64
	ModelsKnown    bool
	ModelsCount    int
}

// NewAccount is one credential a caller wants stored.
type NewAccount struct {
	ProviderID  string
	Kind        string
	Label       string
	SecretValue string
	Priority    int
}

// SecretStore keeps credential secrets. Delete of a missing secret returns nil.
type SecretStore interface {
	Get(ref string) (string, error)
	Set(ref, value string) error
	Delete(ref string) error
}

// Providers answers how a connection authenticates and removes one when its
// last credential goes.
type Providers interface {
	Auth(ctx context.Context, id string) (catalog.Auth, error)
	Delete(ctx context.Context, id string) error
}

// FactStore persists per-account context-window overrides.
type FactStore interface {
	List(ctx context.Context, credentialID string) ([]pool.ContextFact, error)
	ListProvider(ctx context.Context, providerID string) ([]pool.ContextFact, error)
	Save(ctx context.Context, fact pool.ContextFact) error
	Delete(ctx context.Context, credentialID string) error
}

// Discoverer reads one account's model list and folds it into the connection.
type Discoverer interface {
	PerAccount(format catalog.ModelsFormat) bool
	List(ctx context.Context, host catalog.Provider, credentialID string) ([]string, error)
	Refresh(ctx context.Context, providerID string) error
}

// Options configure the account use cases.
type Options struct {
	Entries   pool.CredentialRepository
	Secrets   SecretStore
	Pools     *pool.Manager
	Catalog   *catalog.Catalog
	Facts     FactStore
	Providers Providers
	Discover  Discoverer
	Refresh   func(providerID string)
	Logger    *slog.Logger
}

// Service is the account use cases.
type Service struct {
	entries    pool.CredentialRepository
	secrets    SecretStore
	pools      *pool.Manager
	catalog    *catalog.Catalog
	facts      FactStore
	providers  Providers
	discover   Discoverer
	refresh    func(providerID string)
	logger     *slog.Logger
	mu         sync.Mutex
	refreshMu  sync.Mutex
	refreshing map[string]*refreshState
}

type refreshState struct {
	running bool
	again   bool
}

// New returns the account use cases.
func New(options Options) *Service {
	logger := options.Logger
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &Service{
		entries: options.Entries, secrets: options.Secrets, pools: options.Pools,
		catalog: options.Catalog, facts: options.Facts, providers: options.Providers,
		discover: options.Discover, refresh: options.Refresh, logger: logger,
		refreshing: map[string]*refreshState{},
	}
}

// WithLock runs fn while account additions and removals are serialized.
func (s *Service) WithLock(fn func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn()
}

// AddAccount stores a credential and puts it into rotation. A sign-in for an
// account that already needs one replaces that account's token and returns
// it to rotation, rather than storing a second copy.
func (s *Service) AddAccount(ctx context.Context, draft NewAccount) (Account, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, err := s.newEntry(draft)
	if err != nil {
		return Account{}, err
	}
	existing, err := s.duplicateOf(ctx, entry, draft.SecretValue)
	if err != nil {
		return Account{}, err
	}
	if existing != nil {
		if existing.Kind == kindOAuth && existing.Status == pool.StatusNeedsReauth {
			return s.replaceSignIn(ctx, *existing, draft.SecretValue)
		}
		return Account{}, fmt.Errorf("%s: %w (%s)", entry.ProviderID, ErrDuplicateAccount, duplicateDetail(entry.Kind, *existing))
	}
	if draft.SecretValue != "" {
		if err := s.writeSecret(entry.SecretRef, draft.SecretValue); err != nil {
			return Account{}, err
		}
	}
	if err := s.storeEntry(ctx, entry); err != nil {
		_ = s.forgetSecret(entry.SecretRef)
		return Account{}, err
	}
	s.refreshProvider(entry.ProviderID)
	return s.describe(entry), nil
}

func (s *Service) replaceSignIn(ctx context.Context, existing pool.PoolEntry, secret string) (Account, error) {
	if err := s.writeSecret(existing.SecretRef, secret); err != nil {
		return Account{}, err
	}
	if err := s.storeStatus(ctx, existing, pool.StatusActive); err != nil {
		return Account{}, err
	}
	existing.Status = pool.StatusActive
	s.refreshProvider(existing.ProviderID)
	return s.describe(existing), nil
}

func (s *Service) refreshProvider(providerID string) {
	if s.refresh == nil {
		return
	}
	s.refreshMu.Lock()
	state := s.refreshing[providerID]
	if state == nil {
		state = &refreshState{}
		s.refreshing[providerID] = state
	}
	if state.running {
		state.again = true
		s.refreshMu.Unlock()
		return
	}
	state.running = true
	s.refreshMu.Unlock()
	go s.refreshProviderNow(providerID, state)
}

func (s *Service) refreshProviderNow(providerID string, state *refreshState) {
	for {
		s.refresh(providerID)
		s.refreshMu.Lock()
		if !state.again {
			state.running = false
			delete(s.refreshing, providerID)
			s.refreshMu.Unlock()
			return
		}
		state.again = false
		s.refreshMu.Unlock()
	}
}

func (s *Service) duplicateOf(ctx context.Context, entry pool.PoolEntry, secret string) (*pool.PoolEntry, error) {
	rows, err := s.entries.List(ctx)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		if row.ProviderID != entry.ProviderID || row.Kind != entry.Kind {
			continue
		}
		if !s.sameCredential(row, entry.Kind, secret) {
			continue
		}
		found := row
		return &found, nil
	}
	return nil, nil
}

func duplicateDetail(kind string, row pool.PoolEntry) string {
	if kind == kindOAuth {
		return fmt.Sprintf("the same account is already signed in as %s", row.Label)
	}
	return fmt.Sprintf("the same key is already stored as %s", row.Label)
}

func (s *Service) sameCredential(row pool.PoolEntry, kind, secret string) bool {
	stored := s.storedSecret(row.SecretRef)
	if kind == kindOAuth {
		return sameOAuthAccount(stored, secret)
	}
	return sameSecret(kind, stored, secret)
}

func (s *Service) storedSecret(ref string) string {
	if ref == "" || s.secrets == nil {
		return ""
	}
	value, err := s.secrets.Get(ref)
	if err != nil {
		return ""
	}
	return value
}

func sameSecret(kind, stored, incoming string) bool {
	stored, incoming = strings.TrimSpace(stored), strings.TrimSpace(incoming)
	if stored == "" {
		return false
	}
	if stored == incoming {
		return true
	}
	if kind != kindAWSKeys && kind != kindGCP {
		return false
	}
	return sameJSON(stored, incoming)
}

func sameJSON(left, right string) bool {
	var leftValue, rightValue any
	if json.Unmarshal([]byte(left), &leftValue) != nil || json.Unmarshal([]byte(right), &rightValue) != nil {
		return false
	}
	leftEncoded, leftErr := json.Marshal(leftValue)
	rightEncoded, rightErr := json.Marshal(rightValue)
	return leftErr == nil && rightErr == nil && string(leftEncoded) == string(rightEncoded)
}

func sameOAuthAccount(stored, incoming string) bool {
	var left, right struct {
		AccountID string `json:"account_id"`
		Email     string `json:"email"`
	}
	if json.Unmarshal([]byte(stored), &left) != nil || json.Unmarshal([]byte(incoming), &right) != nil {
		return false
	}
	if left.AccountID != "" && left.AccountID == right.AccountID {
		return true
	}
	return left.Email != "" && strings.EqualFold(left.Email, right.Email)
}

// Accounts returns the stored credentials, optionally filtered by provider.
func (s *Service) Accounts(ctx context.Context, providerID string) ([]Account, error) {
	rows, err := s.entries.List(ctx)
	if err != nil {
		return nil, err
	}
	accounts := make([]Account, 0, len(rows))
	for _, row := range rows {
		if providerID != "" && row.ProviderID != providerID {
			continue
		}
		accounts = append(accounts, s.describe(row))
	}
	return accounts, nil
}

// Account returns one credential, named by its full id or a unique prefix.
func (s *Service) Account(ctx context.Context, id string) (Account, error) {
	entry, err := s.lookup(ctx, id)
	if err != nil {
		return Account{}, err
	}
	return s.describe(entry), nil
}

// RemoveAccount removes a credential. Removing the final credential of a
// credential-backed provider also removes that provider and its models.
func (s *Service) RemoveAccount(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, err := s.lookup(ctx, id)
	if err != nil {
		return err
	}
	auth, err := s.providers.Auth(ctx, entry.ProviderID)
	if err != nil {
		return err
	}
	if auth != catalog.AuthNone {
		return s.removeAuthedAccount(ctx, entry)
	}
	if err := s.removeEntry(ctx, entry); err != nil {
		return err
	}
	return s.forgetAccountContext(ctx, entry)
}

func (s *Service) removeAuthedAccount(ctx context.Context, entry pool.PoolEntry) error {
	rows, err := s.entries.List(ctx)
	if err != nil {
		return err
	}
	count := 0
	for _, row := range rows {
		if row.ProviderID == entry.ProviderID {
			count++
		}
	}
	if count == 1 {
		return s.providers.Delete(ctx, entry.ProviderID)
	}
	if err := s.removeEntry(ctx, entry); err != nil {
		return err
	}
	return s.forgetAccountContext(ctx, entry)
}

func (s *Service) forgetAccountContext(ctx context.Context, entry pool.PoolEntry) error {
	if s.facts != nil {
		if err := s.facts.Delete(ctx, entry.ID); err != nil {
			s.logger.Warn("forget the account context overrides", "account", entry.ID, "error", err)
		}
	}
	return s.forgetSecret(entry.SecretRef)
}

// PauseAccount takes a credential out of rotation.
func (s *Service) PauseAccount(ctx context.Context, id string) error {
	return s.setStatus(ctx, id, pool.StatusPaused)
}

// ResumeAccount returns a paused credential to rotation.
func (s *Service) ResumeAccount(ctx context.Context, id string) error {
	return s.setStatus(ctx, id, pool.StatusActive)
}

// RenameAccount changes the name a console shows for one credential. A blank
// name falls back to the same default a new account gets.
func (s *Service) RenameAccount(ctx context.Context, id, label string) error {
	entry, err := s.lookup(ctx, id)
	if err != nil {
		return err
	}
	return s.storeLabel(ctx, entry, labelFor(label))
}

// SetAccountPriority changes how the pool ranks a credential.
func (s *Service) SetAccountPriority(ctx context.Context, id string, priority int) error {
	if priority < MinPriority || priority > MaxPriority {
		return fmt.Errorf("%d: %w", priority, ErrPriorityRange)
	}
	entry, err := s.lookup(ctx, id)
	if err != nil {
		return err
	}
	return s.storePriority(ctx, entry, priority)
}

func (s *Service) setStatus(ctx context.Context, id, status string) error {
	entry, err := s.lookup(ctx, id)
	if err != nil {
		return err
	}
	return s.storeStatus(ctx, entry, status)
}

func (s *Service) newEntry(draft NewAccount) (pool.PoolEntry, error) {
	host, err := s.credentialHost(draft.ProviderID, draft.Kind)
	if err != nil {
		return pool.PoolEntry{}, err
	}
	if strings.TrimSpace(draft.SecretValue) == "" {
		return pool.PoolEntry{}, ErrEmptySecret
	}
	id, err := newCredentialID()
	if err != nil {
		return pool.PoolEntry{}, err
	}
	kind := credentialKind(host.Auth)
	return pool.PoolEntry{
		ID: id, ProviderID: host.ID, Kind: kind, Label: labelFor(draft.Label),
		SecretRef: secretRef(kind, host.ID, id), Status: pool.StatusActive,
		Priority: draft.Priority,
	}, nil
}

func (s *Service) credentialHost(providerID, kind string) (catalog.Provider, error) {
	snapshot, err := s.snapshot()
	if err != nil {
		return catalog.Provider{}, err
	}
	host, found := snapshot.Provider(strings.TrimSpace(providerID))
	if !found {
		return catalog.Provider{}, fmt.Errorf("%s: %w", providerID, ErrUnknownProvider)
	}
	if !host.Routable {
		return catalog.Provider{}, fmt.Errorf("%s: %w (%s)", providerID, ErrUnknownProvider, host.UnroutableReason)
	}
	if host.Auth == catalog.AuthNone {
		return catalog.Provider{}, fmt.Errorf("%s: %w (it needs no credential)", providerID, ErrUnknownProvider)
	}
	if !kindMatchesAuth(kind, host.Auth) {
		return catalog.Provider{}, fmt.Errorf("%s: %w", kind, ErrUnknownProvider)
	}
	return host, nil
}

func kindMatchesAuth(requested string, auth catalog.Auth) bool {
	if requested == "" {
		return true
	}
	return requested == credentialKind(auth) || requested == string(auth)
}

func (s *Service) storeEntry(ctx context.Context, entry pool.PoolEntry) error {
	if s.pools == nil {
		return s.entries.Insert(ctx, entry)
	}
	return s.pools.AddCredential(ctx, entry)
}

func (s *Service) storeStatus(ctx context.Context, entry pool.PoolEntry, status string) error {
	if s.pools == nil {
		return s.entries.SetStatus(ctx, entry.ID, status)
	}
	if status == pool.StatusActive {
		return s.pools.Resume(ctx, entry.ProviderID, entry.ID)
	}
	return s.pools.Pause(ctx, entry.ProviderID, entry.ID)
}

func (s *Service) storeLabel(ctx context.Context, entry pool.PoolEntry, label string) error {
	if s.pools == nil {
		return s.entries.SetLabel(ctx, entry.ID, label)
	}
	return s.pools.SetLabel(ctx, entry.ProviderID, entry.ID, label)
}

func (s *Service) storePriority(ctx context.Context, entry pool.PoolEntry, priority int) error {
	if s.pools == nil {
		return s.entries.SetPriority(ctx, entry.ID, priority)
	}
	return s.pools.SetPriority(ctx, entry.ProviderID, entry.ID, priority)
}

func (s *Service) removeEntry(ctx context.Context, entry pool.PoolEntry) error {
	if s.pools == nil {
		return s.entries.Remove(ctx, entry.ID)
	}
	return s.pools.Remove(ctx, entry.ProviderID, entry.ID)
}

func (s *Service) writeSecret(ref, value string) error {
	if s.secrets == nil {
		return ErrNoSecretStore
	}
	return s.secrets.Set(ref, value)
}

func (s *Service) forgetSecret(ref string) error {
	if ref == "" || s.secrets == nil {
		return nil
	}
	if err := s.secrets.Delete(ref); err != nil {
		return fmt.Errorf("delete the secret of %s: %w", ref, err)
	}
	return nil
}

func (s *Service) lookup(ctx context.Context, id string) (pool.PoolEntry, error) {
	rows, err := s.entries.List(ctx)
	if err != nil {
		return pool.PoolEntry{}, err
	}
	for _, row := range rows {
		if row.ID == id || (len(id) >= shortIDLength && strings.HasPrefix(row.ID, id)) {
			return row, nil
		}
	}
	return pool.PoolEntry{}, fmt.Errorf("%s: %w", id, ErrAccountNotFound)
}

func (s *Service) lookupExact(ctx context.Context, id string) (pool.PoolEntry, error) {
	rows, err := s.entries.List(ctx)
	if err != nil {
		return pool.PoolEntry{}, err
	}
	wanted := strings.TrimSpace(id)
	for _, row := range rows {
		if row.ID == wanted {
			return row, nil
		}
	}
	return pool.PoolEntry{}, fmt.Errorf("%s: %w", id, ErrAccountNotFound)
}

// CredentialSecret returns the raw secret of one credential.
func (s *Service) CredentialSecret(ctx context.Context, id string) (string, error) {
	entry, err := s.lookup(ctx, id)
	if err != nil {
		return "", err
	}
	if entry.SecretRef == "" || s.secrets == nil {
		return "", ErrNoSecretStore
	}
	value, err := s.secrets.Get(entry.SecretRef)
	if err != nil {
		return "", fmt.Errorf("read the secret of %s: %w", entry.ID, err)
	}
	return value, nil
}

func (s *Service) describe(entry pool.PoolEntry) Account {
	out := Account{
		ID: entry.ID, ProviderID: entry.ProviderID, Kind: entry.Kind,
		Label: entry.Label, Status: entry.Status, Priority: entry.Priority,
		LimitState: "ready",
	}
	out.SecretMask = s.mask(entry.SecretRef)
	if s.pools != nil {
		if observation, models := s.pools.Roster(entry.ID); observation.CredentialID != "" {
			out.ModelsKnown = true
			out.ModelsCount = len(models)
		}
		health := s.pools.Health(entry.ProviderID, entry.ID)
		switch health.State {
		case pool.BreakerOpen:
			out.LimitState = "limited"
			if !health.Until.IsZero() {
				out.LimitedUntilMs = health.Until.UnixMilli()
			}
		case pool.BreakerHalfOpen:
			out.LimitState = "probing"
		}
	}
	return out
}

func (s *Service) mask(secretRef string) string {
	if secretRef == "" || s.secrets == nil {
		return maskPlaceholder
	}
	value, err := s.secrets.Get(secretRef)
	if err != nil {
		return maskPlaceholder
	}
	return MaskSecret(value)
}

// MaskSecret renders the readable ends of a secret, and never the whole of one.
func MaskSecret(value string) string {
	trimmed := strings.TrimSpace(value)
	switch {
	case trimmed == "":
		return maskPlaceholder
	case strings.HasPrefix(trimmed, oauthSecretMarker):
		return "oauth"
	case len(trimmed) <= maskPrefix+maskSuffix:
		return strings.Repeat("*", len(trimmed))
	default:
		return trimmed[:maskPrefix] + "..." + trimmed[len(trimmed)-maskSuffix:]
	}
}

func (s *Service) snapshot() (*catalog.Snapshot, error) {
	if s.catalog == nil {
		return nil, catalog.ErrNoCatalog
	}
	snap, found := s.catalog.Snapshot()
	if !found {
		return nil, catalog.ErrNoCatalog
	}
	return snap, nil
}

func labelFor(label string) string {
	if trimmed := strings.TrimSpace(label); trimmed != "" {
		return trimmed
	}
	return defaultLabel
}

func secretRef(kind, providerID, id string) string {
	prefix := "apikey"
	if strings.HasPrefix(kind, string(catalog.AuthOAuth)) {
		prefix = "oauth"
	}
	return fmt.Sprintf("%s/%s/%s", prefix, providerID, id)
}

func credentialKind(auth catalog.Auth) string {
	switch auth {
	case catalog.AuthOAuth:
		return "oauth"
	case catalog.AuthAWS:
		return "aws_keys"
	case catalog.AuthGCP:
		return "gcp_service_account"
	case catalog.AuthNone:
		return "none"
	default:
		return "api_key"
	}
}

func newCredentialID() (string, error) {
	raw := make([]byte, credentialIDBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate credential id: %w", err)
	}
	return hex.EncodeToString(raw), nil
}
