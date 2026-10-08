// Quota wiring: strategy selection and snapshot storage.
package platform

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jonaskahn/relo/internal/account"
	"github.com/jonaskahn/relo/internal/activity"
	"github.com/jonaskahn/relo/internal/adapters/oauth"
	"github.com/jonaskahn/relo/internal/adapters/quota"
	"github.com/jonaskahn/relo/internal/adapters/secrets"
	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	"github.com/jonaskahn/relo/internal/adapters/upstream"
	"github.com/jonaskahn/relo/internal/adapters/wire"
	"github.com/jonaskahn/relo/internal/clock"
	"github.com/jonaskahn/relo/internal/config"
)

// QuotaStrategy ranks credentials by the quota headroom the cache holds, so a
// pool picks the account with the most room left.
func QuotaStrategy(db *sqlite.DB, clk clock.Clock) account.PoolStrategy {
	cache := activity.NewCache(NewQuotaStore(db), activity.CacheOptions{Clock: clk, Source: "cache"})
	return &account.LeastLoadedStrategy{Headroom: func(credentialID string) (float64, bool) {
		return cache.MostConstrained(context.Background(), credentialID)
	}}
}

func failoverBackoffOf(cfg *config.Config) []time.Duration {
	seconds := config.DefaultFailoverCooldownSeconds
	if cfg != nil {
		seconds = cfg.Upstream.FailoverCooldownSeconds
	}
	if err := config.ValidateUpstreamFailoverCooldown(seconds); err != nil {
		seconds = config.DefaultFailoverCooldownSeconds
	}
	return config.FailoverBackoff(seconds)
}

// QuotaStore adapts the quota tables to the quota cache's repository.
type QuotaStore struct {
	repo *sqlite.QuotaRepo
}

// NewQuotaStore returns a quota store over the given database.
func NewQuotaStore(db *sqlite.DB) *QuotaStore {
	return &QuotaStore{repo: sqlite.NewQuotaRepo(db)}
}

// UpsertSnapshots stores the current windows of every credential.
func (s *QuotaStore) UpsertSnapshots(ctx context.Context, snapshots []activity.Snapshot) error {
	return s.repo.UpsertSnapshots(ctx, toQuotaRows(snapshots))
}

// ListSnapshots returns the current windows of one credential.
func (s *QuotaStore) ListSnapshots(ctx context.Context, credentialID string) ([]activity.Snapshot, error) {
	rows, err := s.repo.ListSnapshots(ctx, credentialID)
	if err != nil {
		return nil, err
	}
	snapshots := make([]activity.Snapshot, 0, len(rows))
	for _, row := range rows {
		snapshots = append(snapshots, activity.Snapshot{
			CredentialID: row.CredentialID, Window: row.Window, UsedPercent: row.UsedPercent,
			Amount: row.Amount, Currency: row.Currency,
			ResetAt: row.ResetAt, Seconds: row.Seconds, Source: row.Source, UpdatedAt: row.ObservedAt,
		})
	}
	return snapshots, nil
}

// AppendHistory records the observations the history ring keeps.
func (s *QuotaStore) AppendHistory(ctx context.Context, snapshots []activity.Snapshot) error {
	return s.repo.AppendHistory(ctx, toQuotaRows(snapshots))
}

// TrimHistory brings the history ring back inside its bounds.
func (s *QuotaStore) TrimHistory(ctx context.Context, bounds activity.HistoryBounds) error {
	return s.repo.TrimHistory(ctx, sqlite.QuotaBounds{
		PerCredential: bounds.PerCredential, Credentials: bounds.Credentials,
		Total: bounds.Total, MaxAge: bounds.MaxAge,
	})
}

func toQuotaRows(snapshots []activity.Snapshot) []sqlite.QuotaRow {
	rows := make([]sqlite.QuotaRow, 0, len(snapshots))
	for _, snapshot := range snapshots {
		rows = append(rows, sqlite.QuotaRow{
			CredentialID: snapshot.CredentialID, Window: snapshot.Window,
			UsedPercent: snapshot.UsedPercent, ResetAt: snapshot.ResetAt,
			Amount: snapshot.Amount, Currency: snapshot.Currency,
			Seconds: snapshot.Seconds, Source: snapshot.Source, ObservedAt: snapshot.UpdatedAt,
		})
	}
	return rows
}

// QuotaCredentials adapts the stored sign-in accounts to what the quota
// worker probes, so a window belongs to the account that reported it.
type QuotaCredentials struct {
	store   *OAuthCredentials
	repo    *sqlite.CredentialRepo
	secrets secrets.SecretStore
	baseURL func(providerID string) string
}

// NewQuotaCredentials returns a quota credential source over the given
// database, secrets, and pools. baseURL names the endpoint a connection
// answers on, which a probe sends to when the account is not the vendor's
// default host.
func NewQuotaCredentials(db *sqlite.DB, secrets secrets.SecretStore, pools *account.Manager, baseURL func(providerID string) string) *QuotaCredentials {
	if baseURL == nil {
		baseURL = func(string) string { return "" }
	}
	return &QuotaCredentials{
		store: NewOAuthCredentials(db, secrets, pools), repo: sqlite.NewCredentialRepo(db),
		secrets: secrets, baseURL: baseURL,
	}
}

// List returns every sign-in account a probe may ask about its quota.
func (q *QuotaCredentials) List(ctx context.Context) ([]activity.Credential, error) {
	credentials, err := q.store.List(ctx)
	if err != nil {
		return nil, err
	}
	probed := make([]activity.Credential, 0, len(credentials))
	for _, credential := range credentials {
		probed = append(probed, activity.Credential{
			ID: credential.ID, ProviderID: credential.ProviderID,
			AccessToken:  credential.Value.AccessToken,
			RefreshToken: credential.Value.RefreshToken,
			BaseURL:      q.baseURL(credential.ProviderID),
			Extra:        credentialExtras(credential),
		})
	}
	keys, err := q.meteredKeys(ctx)
	if err != nil {
		return nil, err
	}
	return append(probed, keys...), nil
}

func (q *QuotaCredentials) meteredKeys(ctx context.Context) ([]activity.Credential, error) {
	if q.repo == nil || q.secrets == nil {
		return nil, nil
	}
	rows, err := q.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	keys := make([]activity.Credential, 0)
	for _, row := range rows {
		base := q.baseURL(row.ProviderID)
		if row.Kind != "api_key" || row.SecretRef == "" || !meteredKeyHost(base) {
			continue
		}
		secret, err := q.secrets.Get(row.SecretRef)
		if err != nil || secret == "" {
			continue
		}
		keys = append(keys, activity.Credential{
			ID: row.ID, ProviderID: row.ProviderID, AccessToken: secret, BaseURL: base,
		})
	}
	return keys, nil
}

func meteredKeyHost(baseURL string) bool {
	return wire.OpenCodeGo(baseURL) || quota.MeteredAPIKey(baseURL)
}

func credentialExtras(credential oauth.Credential) map[string]string {
	extras := map[string]string{}
	if credential.Value.AccountID != "" {
		extras["account_id"] = credential.Value.AccountID
	}
	if project := credential.Value.Extra[oauth.ExtraProjectID]; project != "" {
		extras["projectId"] = project
	}
	if base := credential.Value.Extra[oauth.ExtraAPIBaseURL]; base != "" {
		extras["api_server_url"] = base
	}
	return extras
}

// MarkNeedsReauth records an account a probe found unusable until it logs in
// again.
func (q *QuotaCredentials) MarkNeedsReauth(ctx context.Context, id string) error {
	return q.store.MarkNeedsReauth(ctx, id)
}

type oauthProbeRefresh struct {
	source *upstream.Source
}

// RefreshRejected refreshes a rejected OAuth credential during a quota
// probe, marking reauth where the vendor demands a fresh login.
func (c oauthProbeRefresh) RefreshRejected(ctx context.Context, providerID, credentialID, rejectedToken string) error {
	if c.source == nil {
		return activity.ErrNoCredential
	}
	_, err := c.source.RefreshRejected(ctx, providerID, credentialID, rejectedToken)
	if errors.Is(err, upstream.ErrReauthRequired) {
		return fmt.Errorf("%v: %w", err, activity.ErrReauthRequired)
	}
	if errors.Is(err, oauth.ErrFlowNotFound) {
		return fmt.Errorf("%v: %w", err, activity.ErrReauthRequired)
	}
	return err
}
