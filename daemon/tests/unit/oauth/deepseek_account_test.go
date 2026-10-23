package oauth_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/oauth"
)

// writeHarnessStore writes a DeepSeek Harness credential store holding the
// records given, which is the shape the harness writes for a signed-in
// account.
func writeHarnessStore(t *testing.T, records string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "credentials.yaml")
	body := "records:\n" + records
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write the harness store: %v", err)
	}
	return path
}

// signedInGrant is the record a signed-in harness keeps for its account.
const signedInGrant = `  deepseek-account-platform/default:
    kind: grant
    payload:
      version: 1
      token: dsh-grant-token
      issuer: https://platform.deepseek.com
`

// profileServer answers the platform profile endpoint with body, recording the
// header the request carried.
func profileServer(t *testing.T, status int, body string, seen *http.Header) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if seen != nil {
			*seen = r.Header.Clone()
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(server.Close)
	return server
}

const profileOK = `{"code":0,"data":{"biz_code":0,"biz_data":` +
	`{"id":"acct-77","email":"Person@Example.test","id_profile":{"name":"Person"}}}}`

func TestDeepSeekAccountLogin(t *testing.T) {
	t.Run("imports the harness grant and verifies it", func(t *testing.T) {
		var seen http.Header
		server := profileServer(t, http.StatusOK, profileOK, &seen)
		flow := oauth.NewDeepSeekAccountFlow(
			oauth.WithCredentialsPath(writeHarnessStore(t, signedInGrant)),
			oauth.WithEndpoints(oauth.Endpoints{UserURL: server.URL}),
		)
		_, credential := loginManual(t, flow, "")
		if credential.AccessToken != "dsh-grant-token" {
			t.Fatalf("access token = %q, want the imported grant", credential.AccessToken)
		}
		if credential.AccountID != "acct-77" {
			t.Fatalf("account id = %q, want the platform account id", credential.AccountID)
		}
		if credential.Email != "person@example.test" {
			t.Fatalf("email = %q, want the lowercased platform address", credential.Email)
		}
		if got := seen.Get("x-dsh-auth-token"); got != "dsh-grant-token" {
			t.Fatalf("profile header = %q, want the grant in the harness header", got)
		}
	})

	t.Run("a store the harness never wrote is named", func(t *testing.T) {
		flow := oauth.NewDeepSeekAccountFlow(oauth.WithCredentialsPath(
			filepath.Join(t.TempDir(), "credentials.yaml")))
		_, err := flow.Login(context.Background(), oauth.LoginOpts{NoBrowser: true})
		if !errors.Is(err, oauth.ErrDeepSeekHarnessMissing) {
			t.Fatalf("Login() error = %v, want %v", err, oauth.ErrDeepSeekHarnessMissing)
		}
	})

	t.Run("a store without an account grant is named", func(t *testing.T) {
		store := writeHarnessStore(t, `  deepseek-account-platform/default:
    kind: other
    payload:
      token: not-a-grant
`)
		flow := oauth.NewDeepSeekAccountFlow(oauth.WithCredentialsPath(store))
		_, err := flow.Login(context.Background(), oauth.LoginOpts{NoBrowser: true})
		if !errors.Is(err, oauth.ErrDeepSeekGrantMissing) {
			t.Fatalf("Login() error = %v, want %v", err, oauth.ErrDeepSeekGrantMissing)
		}
	})

	t.Run("a refused grant is never stored", func(t *testing.T) {
		server := profileServer(t, http.StatusUnauthorized, `{}`, nil)
		flow := oauth.NewDeepSeekAccountFlow(
			oauth.WithCredentialsPath(writeHarnessStore(t, signedInGrant)),
			oauth.WithEndpoints(oauth.Endpoints{UserURL: server.URL}),
		)
		if _, err := flow.Login(context.Background(), oauth.LoginOpts{NoBrowser: true}); !errors.Is(err, oauth.ErrDeepSeekGrantRejected) {
			t.Fatalf("Login() error = %v, want %v", err, oauth.ErrDeepSeekGrantRejected)
		}
	})

	t.Run("a profile without an account id is refused", func(t *testing.T) {
		server := profileServer(t, http.StatusOK,
			`{"code":0,"data":{"biz_code":0,"biz_data":{"email":"p@example.test"}}}`, nil)
		flow := oauth.NewDeepSeekAccountFlow(
			oauth.WithCredentialsPath(writeHarnessStore(t, signedInGrant)),
			oauth.WithEndpoints(oauth.Endpoints{UserURL: server.URL}),
		)
		if _, err := flow.Login(context.Background(), oauth.LoginOpts{NoBrowser: true}); !errors.Is(err, oauth.ErrDeepSeekIdentity) {
			t.Fatalf("Login() error = %v, want %v", err, oauth.ErrDeepSeekIdentity)
		}
	})

	t.Run("the harness store is left untouched", func(t *testing.T) {
		store := writeHarnessStore(t, signedInGrant)
		before, err := os.ReadFile(store)
		if err != nil {
			t.Fatalf("read the store: %v", err)
		}
		server := profileServer(t, http.StatusOK, profileOK, nil)
		flow := oauth.NewDeepSeekAccountFlow(
			oauth.WithCredentialsPath(store),
			oauth.WithEndpoints(oauth.Endpoints{UserURL: server.URL}),
		)
		_, _ = loginManual(t, flow, "")
		after, err := os.ReadFile(store)
		if err != nil {
			t.Fatalf("read the store again: %v", err)
		}
		if string(before) != string(after) {
			t.Fatal("the import changed the harness credential store")
		}
	})
}

func TestDeepSeekAccountRefresh(t *testing.T) {
	t.Run("a live grant is kept", func(t *testing.T) {
		server := profileServer(t, http.StatusOK, profileOK, nil)
		flow := oauth.NewDeepSeekAccountFlow(oauth.WithEndpoints(oauth.Endpoints{UserURL: server.URL}))
		refreshed, err := flow.Refresh(context.Background(), &oauth.OAuthCredential{AccessToken: "dsh-grant-token"})
		if err != nil {
			t.Fatalf("Refresh() error = %v", err)
		}
		if refreshed.AccessToken != "dsh-grant-token" {
			t.Fatalf("token = %q, want the same grant", refreshed.AccessToken)
		}
	})

	// The grant has no rotation endpoint, so a refusal has to ask for a new
	// import rather than a silent retry of the same rejected token.
	t.Run("a refused grant asks for a new import", func(t *testing.T) {
		server := profileServer(t, http.StatusUnauthorized, `{}`, nil)
		flow := oauth.NewDeepSeekAccountFlow(oauth.WithEndpoints(oauth.Endpoints{UserURL: server.URL}))
		_, err := flow.Refresh(context.Background(), &oauth.OAuthCredential{AccessToken: "stale"})
		if !errors.Is(err, oauth.ErrDeepSeekGrantRejected) {
			t.Fatalf("Refresh() error = %v, want %v", err, oauth.ErrDeepSeekGrantRejected)
		}
	})

	// A refused grant is what the relay turns into needs_reauth, so it has to
	// be reported as a rejected refresh rather than a transport failure.
	t.Run("a refusal settles the credential", func(t *testing.T) {
		server := profileServer(t, http.StatusForbidden, `{}`, nil)
		flow := oauth.NewDeepSeekAccountFlow(oauth.WithEndpoints(oauth.Endpoints{UserURL: server.URL}))
		if _, err := flow.Refresh(context.Background(), &oauth.OAuthCredential{AccessToken: "stale"}); !errors.Is(err, oauth.ErrDeepSeekGrantRejected) {
			t.Fatalf("Refresh() error = %v, want the refusal", err)
		}
	})

	t.Run("no stored token is refused", func(t *testing.T) {
		flow := oauth.NewDeepSeekAccountFlow()
		if _, err := flow.Refresh(context.Background(), &oauth.OAuthCredential{}); !errors.Is(err, oauth.ErrNoRefreshToken) {
			t.Fatalf("Refresh() error = %v, want %v", err, oauth.ErrNoRefreshToken)
		}
	})
}

func TestDeepSeekAccountReadsTheHarnessHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("DSH_HOME", home)
	body := "records:\n" + signedInGrant
	if err := os.WriteFile(filepath.Join(home, "credentials.yaml"), []byte(body), 0o600); err != nil {
		t.Fatalf("write the harness store: %v", err)
	}
	server := profileServer(t, http.StatusOK, profileOK, nil)
	flow := oauth.NewDeepSeekAccountFlow(oauth.WithEndpoints(oauth.Endpoints{UserURL: server.URL}))
	_, credential := loginManual(t, flow, "")
	if credential.AccessToken != "dsh-grant-token" {
		t.Fatalf("access token = %q, want the grant from DSH_HOME", credential.AccessToken)
	}
}

// The grant travels in a vendor header, so the flow's provider replaces the
// API-key lane that would otherwise own the DeepSeek host.
func TestDeepSeekAccountFlowIsRegistered(t *testing.T) {
	flow, err := oauth.DefaultRegistry().Flow(oauth.FlowDeepSeekAccount)
	if err != nil {
		t.Fatalf("Flow() error = %v", err)
	}
	if flow.ProviderID() != oauth.FlowDeepSeekAccount {
		t.Fatalf("provider = %q, want %q", flow.ProviderID(), oauth.FlowDeepSeekAccount)
	}
	if port := flow.CallbackPort(); port != 0 {
		t.Fatalf("callback port = %d, want no listener for an import", port)
	}
}
