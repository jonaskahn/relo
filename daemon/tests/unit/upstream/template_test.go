package dispatch_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jonaskahn/relo/internal/account"
	"github.com/jonaskahn/relo/internal/adapters/oauth"
	"github.com/jonaskahn/relo/internal/adapters/upstream"
	"github.com/jonaskahn/relo/internal/catalog"
)

// A connection Relo creates from a template keeps its own id, while the vendor
// Relo signs in to is the template. Everything a renewal needs - the flow, the
// authorizer, the refresh policy - is named after the template, and everything
// that records the outcome is named after the connection.
const connectionID = "claude-work"

// templateSource is a source over one expiring credential on a connection whose
// template is Claude's.
func templateSource(options upstream.Options) (*upstream.Source, *memoryFlows, *memoryPools) {
	entry := account.PoolEntry{ID: "cred-1", ProviderID: connectionID, SecretRef: "secret"}
	pools := &memoryPools{entries: map[string]account.PoolEntry{entry.ID: entry}}
	secrets := &memorySecrets{values: map[string]string{
		"secret": `{"access_token":"stale","refresh_token":"refresh"}`,
	}}
	flows := &memoryFlows{}
	options.Pools, options.Secrets, options.Flows = pools, secrets, flows
	options.Adapters = templateAdapters{}
	options.TemplateResolver = func(providerID string) string {
		if providerID == connectionID {
			return oauth.FlowClaude
		}
		return providerID
	}
	return upstream.New(options), flows, pools
}

func TestAuthorizeResolvesTheConnectionTemplate(t *testing.T) {
	t.Run("an expiring credential is renewed through the template's flow", func(t *testing.T) {
		source, flows, _ := templateSource(upstream.Options{})
		auth, err := source.AuthorizeCredential(context.Background(), catalog.AuthRequest{
			ProviderID: connectionID, Auth: catalog.AuthOAuth,
		}, "cred-1")
		if err != nil {
			t.Fatalf("AuthorizeCredential() error = %v", err)
		}
		if flows.calls != 1 {
			t.Fatalf("refresh calls = %d, want one refresh before the request", flows.calls)
		}
		if auth.Token != "fresh" {
			t.Fatalf("token = %q, want the renewed token", auth.Token)
		}
	})

	t.Run("a provider with no template refreshes under its own name", func(t *testing.T) {
		entry := account.PoolEntry{ID: "cred-1", ProviderID: "cursor", SecretRef: "secret"}
		pools := &memoryPools{entries: map[string]account.PoolEntry{entry.ID: entry}}
		flows := &memoryFlows{}
		source := upstream.New(upstream.Options{
			Pools: pools, Secrets: &memorySecrets{values: map[string]string{
				"secret": `{"access_token":"stale","refresh_token":"refresh"}`,
			}},
			Flows: flows, Adapters: templateAdapters{},
			TemplateResolver: func(providerID string) string { return "" },
		})
		if _, err := source.AuthorizeCredential(context.Background(), catalog.AuthRequest{
			ProviderID: "cursor", Auth: catalog.AuthOAuth,
		}, entry.ID); err != nil {
			t.Fatalf("AuthorizeCredential() error = %v", err)
		}
		if flows.calls != 1 {
			t.Fatalf("refresh calls = %d, want one refresh", flows.calls)
		}
	})

	t.Run("a credential that expires with refresh off asks for a new login", func(t *testing.T) {
		source, flows, pools := templateSource(upstream.Options{Policy: refreshOff{}})
		_, err := source.AuthorizeCredential(context.Background(), catalog.AuthRequest{
			ProviderID: connectionID, Auth: catalog.AuthOAuth,
		}, "cred-1")
		if !errors.Is(err, upstream.ErrReauthRequired) {
			t.Fatalf("error = %v, want a reauth", err)
		}
		if flows.calls != 0 {
			t.Fatalf("refresh calls = %d, want no refresh", flows.calls)
		}
		if len(pools.reauth) != 1 || pools.reauth[0] != "cred-1" {
			t.Fatalf("marked = %v, want the connection's credential", pools.reauth)
		}
	})

	t.Run("a source with no flow registry cannot renew anything", func(t *testing.T) {
		entry := account.PoolEntry{ID: "cred-1", ProviderID: connectionID, SecretRef: "secret"}
		pools := &memoryPools{entries: map[string]account.PoolEntry{entry.ID: entry}}
		secrets := &memorySecrets{values: map[string]string{
			"secret": `{"access_token":"stale","refresh_token":"refresh"}`,
		}}
		source := upstream.New(upstream.Options{
			Pools: pools, Secrets: secrets, Adapters: templateAdapters{},
			TemplateResolver: func(string) string { return oauth.FlowClaude },
		})

		_, refreshErr := source.AuthorizeCredential(context.Background(), catalog.AuthRequest{
			ProviderID: connectionID, Auth: catalog.AuthOAuth,
		}, entry.ID)
		if !errors.Is(refreshErr, oauth.ErrFlowNotFound) {
			t.Fatalf("AuthorizeCredential() error = %v, want ErrFlowNotFound", refreshErr)
		}

		_, rejectedErr := source.RefreshRejected(context.Background(), connectionID, entry.ID, "stale")
		if !errors.Is(rejectedErr, oauth.ErrFlowNotFound) {
			t.Fatalf("RefreshRejected() error = %v, want ErrFlowNotFound", rejectedErr)
		}
	})
}

// TestRefreshRejectedNamesTheConnectionTemplate covers the same split on the
// renewal a refused token triggers: the flow and the authorizer come from the
// template, the account and the credential from the connection.
func TestRefreshRejectedNamesTheConnectionTemplate(t *testing.T) {
	t.Run("the template's authorizer builds the authorization", func(t *testing.T) {
		source, flows, _ := templateSource(upstream.Options{})
		auth, err := source.RefreshRejected(context.Background(), connectionID, "cred-1", "stale")
		if err != nil {
			t.Fatalf("RefreshRejected() error = %v", err)
		}
		if flows.calls != 1 || auth.Token != "fresh" {
			t.Fatalf("calls = %d token = %q, want the renewed token", flows.calls, auth.Token)
		}
	})

	t.Run("a token another request already replaced is not renewed again", func(t *testing.T) {
		source, flows, _ := templateSource(upstream.Options{})
		auth, err := source.RefreshRejected(context.Background(), connectionID, "cred-1", "someone-elses")
		if err != nil {
			t.Fatalf("RefreshRejected() error = %v", err)
		}
		if flows.calls != 0 {
			t.Fatalf("refresh calls = %d, want the stored token to stand", flows.calls)
		}
		if auth.Token != "stale" {
			t.Fatalf("token = %q, want the stored token", auth.Token)
		}
	})

	t.Run("an unknown credential is reported against the connection", func(t *testing.T) {
		source, _, _ := templateSource(upstream.Options{})
		_, err := source.RefreshRejected(context.Background(), connectionID, "no-such-credential", "stale")
		if !errors.Is(err, account.ErrNotFound) {
			t.Fatalf("RefreshRejected() error = %v, want ErrNotFound", err)
		}
	})
}
