package dispatch_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/account"
	"github.com/jonaskahn/relo/internal/adapters/oauth"
	"github.com/jonaskahn/relo/internal/adapters/upstream"
	"github.com/jonaskahn/relo/internal/catalog"
)

func TestAuthorizeCarriesTheSelectedAccount(t *testing.T) {
	entry := account.PoolEntry{ID: "cred-9", ProviderID: oauth.FlowClaude, Label: "Claude", SecretRef: "secret"}
	pools := &memoryPools{entries: map[string]account.PoolEntry{entry.ID: entry}}
	secrets := &memorySecrets{values: map[string]string{
		"secret": `{"access_token":"live","refresh_token":"r","account_id":"acct-9","expires_at":"2999-01-01T00:00:00Z"}`,
	}}
	source := upstream.New(upstream.Options{
		Pools: pools, Secrets: secrets, Flows: &memoryFlows{}, Adapters: templateAdapters{},
	})
	auth, err := source.AuthorizeCredential(context.Background(), catalog.AuthRequest{
		ProviderID: oauth.FlowClaude, Auth: catalog.AuthOAuth,
	}, entry.ID)
	if err != nil {
		t.Fatalf("AuthorizeCredential() error = %v", err)
	}
	if auth.Token != "live" {
		t.Fatalf("token = %q, want the stored token", auth.Token)
	}
	if auth.AccountID != "acct-9" {
		t.Fatalf("account id = %q, want the selected credential's account", auth.AccountID)
	}
}

func TestAuthorizeBackfillsMissingClaudeIdentity(t *testing.T) {
	identity := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"oauth_account":{"account_uuid":"acct-9",`+
			`"organization_uuid":"org-3","organization_name":"Acme"}}`)
	}))
	t.Cleanup(identity.Close)

	entry := account.PoolEntry{ID: "cred-9", ProviderID: oauth.FlowClaude, Label: "Claude", SecretRef: "secret"}
	newSource := func(values map[string]string) (*upstream.Source, *memorySecrets) {
		secrets := &memorySecrets{values: values}
		return upstream.New(upstream.Options{
			Pools: &memoryPools{entries: map[string]account.PoolEntry{entry.ID: entry}},
			Secrets: secrets, Flows: &memoryFlows{}, Adapters: templateAdapters{},
			IdentityEndpoint: identity.URL,
		}), secrets
	}
	authorize := func(t *testing.T, source *upstream.Source) catalog.Authorization {
		t.Helper()
		auth, err := source.AuthorizeCredential(context.Background(), catalog.AuthRequest{
			ProviderID: oauth.FlowClaude, Auth: catalog.AuthOAuth,
		}, entry.ID)
		if err != nil {
			t.Fatalf("AuthorizeCredential() error = %v", err)
		}
		return auth
	}

	t.Run("a stored credential without identity gains it", func(t *testing.T) {
		source, secrets := newSource(map[string]string{
			"secret": `{"access_token":"live","refresh_token":"r","expires_at":"2999-01-01T00:00:00Z"}`,
		})
		if auth := authorize(t, source); auth.AccountID != "acct-9" {
			t.Fatalf("account id = %q, want the backfilled account", auth.AccountID)
		}
		if got := secrets.values["secret"]; !strings.Contains(got, "acct-9") {
			t.Fatalf("stored = %s, want the backfilled account", got)
		}
	})

	t.Run("a failed lookup still authorizes without attribution", func(t *testing.T) {
		down := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		}))
		t.Cleanup(down.Close)
		secrets := &memorySecrets{values: map[string]string{
			"secret": `{"access_token":"live","refresh_token":"r","expires_at":"2999-01-01T00:00:00Z"}`,
		}}
		source := upstream.New(upstream.Options{
			Pools: &memoryPools{entries: map[string]account.PoolEntry{entry.ID: entry}},
			Secrets: secrets, Flows: &memoryFlows{}, Adapters: templateAdapters{},
			IdentityEndpoint: down.URL,
		})
		auth, err := source.AuthorizeCredential(context.Background(), catalog.AuthRequest{
			ProviderID: oauth.FlowClaude, Auth: catalog.AuthOAuth,
		}, entry.ID)
		if err != nil {
			t.Fatalf("AuthorizeCredential() error = %v", err)
		}
		if auth.Token != "live" || auth.AccountID != "" {
			t.Fatalf("token = %q account = %q, want the stored token without attribution", auth.Token, auth.AccountID)
		}
		if got := secrets.values["secret"]; !strings.Contains(got, "identityRetryAt") {
			t.Fatalf("stored = %s, want the retry marker", got)
		}
	})
}
