// Pool directory: the account pools catalog reads resolve against.
package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jonaskahn/relo/internal/account"
	"github.com/jonaskahn/relo/internal/adapters/oauth"
	"github.com/jonaskahn/relo/internal/adapters/secrets"
	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	"github.com/jonaskahn/relo/internal/adapters/upstream"
	"github.com/jonaskahn/relo/internal/catalog"
)

// PoolDirectory adapts the credential pools to the two questions the catalog
// asks about a provider, so the catalog never holds a pool itself.
type PoolDirectory struct {
	pools *account.Manager
}

// NewPoolDirectory returns a directory over the given pools.
func NewPoolDirectory(pools *account.Manager) *PoolDirectory {
	return &PoolDirectory{pools: pools}
}

// Active reports how many credentials a provider can put in rotation.
func (d *PoolDirectory) Active(providerID string) int {
	if d.pools == nil {
		return 0
	}
	return d.pools.Active(providerID)
}

// Available reports whether a provider has a credential that can take a
// request right now.
func (d *PoolDirectory) Available(providerID string) bool {
	if d.pools == nil {
		return false
	}
	return d.pools.Available(providerID)
}

// ServesModel reports whether an account that could take a request right now
// is known to serve the model. A provider whose accounts publish no roster of
// their own serves everything it lists, which is what keeps one shared
// listing working unchanged.
func (d *PoolDirectory) ServesModel(providerID string, model catalog.Model) bool {
	if d.pools == nil {
		return false
	}
	return d.pools.Serves(providerID, modelIdentifiers(model))
}

// Coverage counts the accounts of one provider that could take a request now
// and how many of them can serve the model.
func (d *PoolDirectory) Coverage(providerID string, model catalog.Model) (int, int) {
	if d.pools == nil {
		return 0, 0
	}
	return d.pools.Coverage(providerID, modelIdentifiers(model))
}

// CooldownUntil is when every active account of a provider may take a
// request again.
func (d *PoolDirectory) CooldownUntil(providerID string) time.Time {
	if d.pools == nil {
		return time.Time{}
	}
	return d.pools.CooldownUntil(providerID)
}

// CredentialPools adapts the pools to what the credential source needs, so
// the source reads a credential without knowing the pool implementation.
type CredentialPools struct {
	pools *account.Manager
}

// NewCredentialPools returns an adapter over the given pools.
func NewCredentialPools(pools *account.Manager) *CredentialPools {
	return &CredentialPools{pools: pools}
}

// Select picks an account that can serve a request.
func (p *CredentialPools) Select(providerID string, routing account.Selection) (account.PoolEntry, error) {
	return p.pools.GetPool(providerID).Select(routing)
}

// Entry returns one named account of a provider, which is how a caller reads
// a single account's own model roster rather than the pool's own choice.
func (p *CredentialPools) Entry(providerID, credentialID string) (account.PoolEntry, bool) {
	return p.pools.GetPool(providerID).Entry(credentialID)
}

// ResolveSecret reads the secret one credential points at.
func (p *CredentialPools) ResolveSecret(entry account.PoolEntry) (string, error) {
	return p.pools.ResolveSecret(entry)
}

// RecordSuccess reports that an account answered a request.
func (p *CredentialPools) RecordSuccess(providerID, id string) {
	p.pools.GetPool(providerID).RecordSuccess(id)
}

// RecordFailure reports what an upstream refusal means for an account.
func (p *CredentialPools) RecordFailure(providerID, id string, status int, retryAfter time.Duration) {
	p.pools.GetPool(providerID).RecordFailure(id, status, retryAfter)
}

// MarkNeedsReauth takes an account out of rotation until it logs in again.
func (p *CredentialPools) MarkNeedsReauth(ctx context.Context, providerID, id string) error {
	return p.pools.MarkNeedsReauth(ctx, providerID, id)
}

// FlowRegistry adapts the OAuth registry to the credential source.
type FlowRegistry struct {
	flows *oauth.Registry
}

// NewFlowRegistry returns an adapter over the given flow registry.
func NewFlowRegistry(flows *oauth.Registry) *FlowRegistry {
	return &FlowRegistry{flows: flows}
}

// Flow returns the flow that refreshes one provider's credential.
func (f *FlowRegistry) Flow(providerID string) (oauth.OAuthFlow, error) {
	return f.flows.Flow(providerID)
}

// OAuthCredentials adapts the credential table to the refresh guardian, so a
// subscription token is renewed before any request waits for it.
type OAuthCredentials struct {
	repo    *sqlite.CredentialRepo
	secrets secrets.SecretStore
	pools   *account.Manager
}

// NewOAuthCredentials returns a guardian store over the given pieces.
func NewOAuthCredentials(db *sqlite.DB, secrets secrets.SecretStore, pools *account.Manager) *OAuthCredentials {
	return &OAuthCredentials{repo: sqlite.NewCredentialRepo(db), secrets: secrets, pools: pools}
}

// List returns every live OAuth credential with its stored tokens.
func (o *OAuthCredentials) List(ctx context.Context) ([]oauth.Credential, error) {
	rows, err := o.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	credentials := make([]oauth.Credential, 0, len(rows))
	for _, row := range rows {
		if row.Kind != "oauth" || row.SecretRef == "" || o.secrets == nil {
			continue
		}
		value, err := o.decoded(row.SecretRef)
		if err != nil {
			continue
		}
		credentials = append(credentials, oauth.Credential{
			ID: row.ID, ProviderID: row.ProviderID, Value: value,
			NeedsReauth: row.Status == account.StatusNeedsReauth,
		})
	}
	return credentials, nil
}

// Save stores a refreshed credential, keeping the account in rotation.
func (o *OAuthCredentials) Save(ctx context.Context, refreshed oauth.Credential) error {
	return o.write(ctx, refreshed, account.StatusActive)
}

// MarkNeedsReauth records an account whose refresh the provider refused.
func (o *OAuthCredentials) MarkNeedsReauth(ctx context.Context, id string) error {
	rows, err := o.repo.List(ctx)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if row.ID != id {
			continue
		}
		if o.pools != nil {
			return o.pools.MarkNeedsReauth(ctx, row.ProviderID, id)
		}
		return o.repo.SetStatus(ctx, id, account.StatusNeedsReauth)
	}
	return nil
}

func (o *OAuthCredentials) decoded(ref string) (oauth.OAuthCredential, error) {
	if o.secrets == nil {
		return oauth.OAuthCredential{}, upstream.ErrNotSignedIn
	}
	raw, err := o.secrets.Get(ref)
	if err != nil {
		return oauth.OAuthCredential{}, err
	}
	var value oauth.OAuthCredential
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return oauth.OAuthCredential{}, fmt.Errorf("%s: %w", ref, upstream.ErrNotSignedIn)
	}
	return value, nil
}

func (o *OAuthCredentials) write(ctx context.Context, refreshed oauth.Credential, status string) error {
	rows, err := o.repo.List(ctx)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if row.ID != refreshed.ID {
			continue
		}
		if err := o.store(row.SecretRef, refreshed.Value); err != nil {
			return err
		}
		if o.pools != nil {
			return o.pools.Resume(ctx, row.ProviderID, refreshed.ID)
		}
		return o.repo.SetStatus(ctx, refreshed.ID, status)
	}
	return nil
}

func (o *OAuthCredentials) store(ref string, value oauth.OAuthCredential) error {
	if o.secrets == nil || ref == "" {
		return nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode the refreshed credential: %w", err)
	}
	return o.secrets.Set(ref, string(encoded))
}

func modelIdentifiers(model catalog.Model) []string {
	identifiers := make([]string, 0, 3)
	for _, id := range []string{model.ID, model.UpstreamID, model.ClonedFrom} {
		if id != "" {
			identifiers = append(identifiers, id)
		}
	}
	return identifiers
}
