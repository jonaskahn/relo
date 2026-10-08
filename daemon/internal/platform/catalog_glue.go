// Catalog glue: quiet secret and view adapters for services.
package platform

import (
	"context"
	"errors"
	"net/url"

	"github.com/jonaskahn/relo/internal/adapters/oauth"
	"github.com/jonaskahn/relo/internal/adapters/secrets"
	"github.com/jonaskahn/relo/internal/adapters/templates"
	appcatalog "github.com/jonaskahn/relo/internal/application/catalog"
	appstatus "github.com/jonaskahn/relo/internal/application/status"
	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/internal/server"
)

// QuietSecrets hides a missing secret from a delete, so a credential whose
// secret was already cleaned up can still be removed.
type QuietSecrets struct {
	secrets.SecretStore
}

// Delete removes one secret, treating one that is already gone as removed.
func (q QuietSecrets) Delete(ref string) error {
	err := q.SecretStore.Delete(ref)
	if errors.Is(err, secrets.ErrNotFound) {
		return nil
	}
	return err
}

// NewQuietSecrets wraps a secret store so deleting a missing secret is a
// no-op, which is what every credential cleanup expects, and reports none
// when the caller has no store.
func NewQuietSecrets(store secrets.SecretStore) secrets.SecretStore {
	if store == nil {
		return nil
	}
	return QuietSecrets{store}
}

// SecretView reports the secret store's mode as a plain string, which is
// what the status views show.
type SecretView struct {
	secrets.SecretStore
}

// Mode names the backend the secrets live in.
func (v SecretView) Mode() string {
	return string(v.SecretStore.Mode())
}

// NewSecretView wraps a secret store for the status use cases, and reports
// none when the caller has no store.
func NewSecretView(store secrets.SecretStore) appstatus.Secrets {
	if store == nil {
		return nil
	}
	return SecretView{store}
}

// secretModeSource wraps a secret store for the server status page, and
// reports no source when the daemon runs without one.
func secretModeSource(store secrets.SecretStore) server.SecretModeSource {
	if store == nil {
		return nil
	}
	return SecretView{store}
}

// CallbackBridge hands the server's callback surface to the one broker the
// daemon serves its callback page from, translating the adapter status into
// the neutral shape the page polls.
type CallbackBridge struct {
	broker *oauth.CallbackBroker
}

// NewCallbackBridge returns the server port over one broker, and no port
// when the daemon runs without one.
func NewCallbackBridge(broker *oauth.CallbackBroker) server.CallbackBroker {
	if broker == nil {
		return nil
	}
	return CallbackBridge{broker: broker}
}

// Deliver resolves one provider redirect against the login waiting for it.
func (b CallbackBridge) Deliver(provider string, query url.Values) (string, error) {
	return b.broker.Deliver(provider, query)
}

// Status reports how far one login got, and whether the process still tracks
// it.
func (b CallbackBridge) Status(ticket string) (server.CallbackStatus, bool) {
	status, found := b.broker.Status(ticket)
	if !found {
		return server.CallbackStatus{}, false
	}
	return server.CallbackStatus{
		Provider: status.Provider, Phase: server.CallbackOutcome(status.Phase),
		Account: status.Account, Error: status.Error,
	}, true
}

// Complete records the outcome of one login for the page watching it.
func (b CallbackBridge) Complete(ticket, account string, err error) {
	b.broker.Complete(ticket, account, err)
}

// AccountEdges couples the account lifecycle to the catalog: the account use
// cases learn how a connection authenticates, read one account's model list,
// and ask the catalog to remove a connection when its last credential goes.
// Catalog answers those questions and is bound after the account use cases
// are built, because each side needs the other.
type AccountEdges struct {
	Catalog *appcatalog.Service
}

// Auth reports how a connection authenticates.
func (e *AccountEdges) Auth(ctx context.Context, id string) (catalog.Auth, error) {
	return e.Catalog.Gate().Auth(ctx, id)
}

// Delete removes a connection whose last credential is being removed. The
// account use cases hold the account lock while they call this.
func (e *AccountEdges) Delete(ctx context.Context, id string) error {
	return e.Catalog.Gate().Delete(ctx, id)
}

// PerAccount reports whether one connection's model list is read once per
// account.
func (e *AccountEdges) PerAccount(format catalog.ModelsFormat) bool {
	return catalog.PerAccountRoster(format)
}

// List reads one account's own model list.
func (e *AccountEdges) List(ctx context.Context, host catalog.Provider, credentialID string) ([]string, error) {
	listed, err := e.Catalog.DiscoverAccountModels(ctx, host, credentialID)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(listed))
	for _, item := range listed {
		ids = append(ids, item.ID)
	}
	return ids, nil
}

// Refresh reads a connection's model list again, which is what a later
// account addition needs.
func (e *AccountEdges) Refresh(ctx context.Context, providerID string) error {
	_, err := e.Catalog.RefreshProviderModels(ctx, providerID)
	return err
}

// RefreshAfterCredential refreshes a connection's roster once a new
// credential is in rotation.
func (e *AccountEdges) RefreshAfterCredential(providerID string) {
	e.Catalog.RefreshAfterCredential(providerID)
}

// TemplateRegistry serves the provider template registry as a port, so the
// catalog use cases read curated and models.dev-derived templates without
// importing the registry's package.
type TemplateRegistry struct{}

// Get finds a template by id, curated first and models.dev-derived second.
func (TemplateRegistry) Get(id string, idx *catalog.ModelsDevIndex) (catalog.Template, bool) {
	return templates.Get(id, idx)
}

// All returns every template a build offers.
func (TemplateRegistry) All(idx *catalog.ModelsDevIndex) []catalog.Template {
	return templates.All(idx)
}

// Curated finds a hand-written template by id.
func (TemplateRegistry) Curated(id string) (catalog.Template, bool) {
	return templates.Curated(id)
}

// RewrittenLabel reports the current default for a stored label that still
// matches a retired template name.
func (TemplateRegistry) RewrittenLabel(templateID, stored string) (string, bool) {
	return templates.RewrittenLabel(templateID, stored)
}
