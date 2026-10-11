package dispatch_test

import (
	"context"
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
