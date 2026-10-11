package oauth_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/adapters/oauth"
)

func TestAnthropicLoginKeepsOrganization(t *testing.T) {
	t.Run("token account and organization reach the credential", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"access_token":"fresh","refresh_token":"r","expires_in":3600,`+
				`"account":{"uuid":"acct-9","email_address":"Person@Example.test"},`+
				`"organization":{"uuid":"org-3","name":"Acme"}}`)
		}))
		t.Cleanup(server.Close)
		flow := oauth.NewAnthropicFlow(
			oauth.WithCallbackPort(-1),
			oauth.WithBrowser(&fakeBrowser{open: callbackDeliverer(t)}),
			oauth.WithEndpoints(oauth.Endpoints{AuthURL: server.URL + "/authorize", TokenURL: server.URL + "/token"}),
		)
		credential, err := flow.Login(context.Background(), oauth.LoginOpts{Timeout: 5 * time.Second})
		if err != nil {
			t.Fatalf("Login() error = %v", err)
		}
		if credential.AccountID != "acct-9" {
			t.Fatalf("account id = %q, want the token account", credential.AccountID)
		}
		if credential.Email != "person@example.test" {
			t.Fatalf("email = %q, want the lowercased token email", credential.Email)
		}
		if credential.Extra[oauth.ExtraOrgID] != "org-3" {
			t.Fatalf("org id = %q, want the token organization", credential.Extra[oauth.ExtraOrgID])
		}
		if credential.Extra[oauth.ExtraOrgName] != "Acme" {
			t.Fatalf("org name = %q, want the token organization name", credential.Extra[oauth.ExtraOrgName])
		}
	})

	t.Run("a response without identity leaves earlier values alone", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"access_token":"fresh","refresh_token":"r","expires_in":3600}`)
		}))
		t.Cleanup(server.Close)
		flow := oauth.NewAnthropicFlow(
			oauth.WithEndpoints(oauth.Endpoints{AuthURL: server.URL, TokenURL: server.URL}),
		)
		credential, err := flow.Refresh(context.Background(), &oauth.OAuthCredential{
			RefreshToken: "r", AccountID: "acct-1",
			Extra: map[string]string{oauth.ExtraOrgID: "org-1"},
		})
		if err != nil {
			t.Fatalf("Refresh() error = %v", err)
		}
		if credential.AccountID != "acct-1" {
			t.Fatalf("account id = %q, want the stored account", credential.AccountID)
		}
		if credential.Extra[oauth.ExtraOrgID] != "org-1" {
			t.Fatalf("org id = %q, want the stored organization", credential.Extra[oauth.ExtraOrgID])
		}
	})
}

func TestClaudeIdentityFetch(t *testing.T) {
	t.Run("bootstrap identity fills account and workspace", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer token-1" {
				t.Errorf("authorization = %q, want the token", r.Header.Get("Authorization"))
			}
			if r.Header.Get("anthropic-beta") != "oauth-2025-04-20" {
				t.Errorf("beta = %q, want the oauth beta", r.Header.Get("anthropic-beta"))
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"oauth_account":{"account_uuid":"acct-9",`+
				`"account_email":"a@example.test","organization_uuid":"org-3","organization_name":"Acme"}}`)
		}))
		t.Cleanup(server.Close)
		identity, err := oauth.ClaudeIdentitySource{Endpoint: server.URL}.Fetch(context.Background(), "token-1")
		if err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}
		if identity.AccountID != "acct-9" || identity.OrgID != "org-3" || identity.OrgName != "Acme" {
			t.Fatalf("identity = %+v, want the bootstrap account", identity)
		}
	})

	t.Run("a refusal stays an error the caller degrades from", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		}))
		t.Cleanup(server.Close)
		source := oauth.ClaudeIdentitySource{Endpoint: server.URL}
		if _, err := source.Fetch(context.Background(), "stale"); err == nil {
			t.Fatal("Fetch() error = nil, want the refusal")
		}
	})
}
