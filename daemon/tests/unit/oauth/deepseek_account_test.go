package oauth_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

// The store is another application's file: an unreadable one, a malformed one,
// and one that answers with a refusal all have to name why the import failed
// rather than surfacing a bare transport error.
func TestDeepSeekAccountFailures(t *testing.T) {
	t.Run("Validate accepts a live grant", func(t *testing.T) {
		flow := oauth.NewDeepSeekAccountFlow()
		if err := flow.Validate(context.Background(), &oauth.OAuthCredential{AccessToken: "a"}); err != nil {
			t.Fatalf("Validate() error = %v", err)
		}
	})

	t.Run("Validate names a missing and an expired grant", func(t *testing.T) {
		flow := oauth.NewDeepSeekAccountFlow()
		if err := flow.Validate(context.Background(), nil); !errors.Is(err, oauth.ErrTokenResponse) {
			t.Fatalf("Validate(nil) error = %v, want %v", err, oauth.ErrTokenResponse)
		}
		expired := &oauth.OAuthCredential{AccessToken: "a", ExpiresAt: time.Now().Add(-time.Minute)}
		if err := flow.Validate(context.Background(), expired); !errors.Is(err, oauth.ErrCredentialExpired) {
			t.Fatalf("Validate(expired) error = %v, want %v", err, oauth.ErrCredentialExpired)
		}
	})

	t.Run("a store that cannot be read is named", func(t *testing.T) {
		// A directory cannot be read as a file, which is the read failure that
		// is not the harness-never-wrote-it case.
		flow := oauth.NewDeepSeekAccountFlow(oauth.WithCredentialsPath(t.TempDir()))
		_, err := flow.Login(context.Background(), oauth.LoginOpts{NoBrowser: true})
		if err == nil || errors.Is(err, oauth.ErrDeepSeekHarnessMissing) {
			t.Fatalf("Login() error = %v, want the read failure", err)
		}
		if !strings.Contains(err.Error(), "read the DeepSeek Harness credential") {
			t.Fatalf("Login() error = %q, want the read failure named", err)
		}
	})

	t.Run("a malformed store is named", func(t *testing.T) {
		store := filepath.Join(t.TempDir(), "credentials.yaml")
		if err := os.WriteFile(store, []byte("records: [not: a: map"), 0o600); err != nil {
			t.Fatalf("write the store: %v", err)
		}
		flow := oauth.NewDeepSeekAccountFlow(oauth.WithCredentialsPath(store))
		_, err := flow.Login(context.Background(), oauth.LoginOpts{NoBrowser: true})
		if err == nil || !strings.Contains(err.Error(), "decode the DeepSeek Harness credential") {
			t.Fatalf("Login() error = %v, want the decode failure named", err)
		}
	})

	t.Run("an unreachable platform is a transport failure", func(t *testing.T) {
		flow := oauth.NewDeepSeekAccountFlow(
			oauth.WithCredentialsPath(writeHarnessStore(t, signedInGrant)),
			oauth.WithEndpoints(oauth.Endpoints{UserURL: "http://127.0.0.1:1/profile"}),
		)
		_, err := flow.Login(context.Background(), oauth.LoginOpts{NoBrowser: true})
		if err == nil || !strings.Contains(err.Error(), "validate the DeepSeek account grant") {
			t.Fatalf("Login() error = %v, want the transport failure named", err)
		}
	})

	t.Run("a non-success status carries the platform code", func(t *testing.T) {
		server := profileServer(t, http.StatusInternalServerError,
			`{"error":{"code":"internal"}}`, nil)
		flow := oauth.NewDeepSeekAccountFlow(
			oauth.WithCredentialsPath(writeHarnessStore(t, signedInGrant)),
			oauth.WithEndpoints(oauth.Endpoints{UserURL: server.URL}),
		)
		_, err := flow.Login(context.Background(), oauth.LoginOpts{NoBrowser: true})
		if err == nil || !strings.Contains(err.Error(), "validate the DeepSeek account grant") {
			t.Fatalf("Login() error = %v, want the status failure named", err)
		}
	})

	t.Run("a body that is not JSON is refused", func(t *testing.T) {
		server := profileServer(t, http.StatusOK, "not json", nil)
		flow := oauth.NewDeepSeekAccountFlow(
			oauth.WithCredentialsPath(writeHarnessStore(t, signedInGrant)),
			oauth.WithEndpoints(oauth.Endpoints{UserURL: server.URL}),
		)
		if _, err := flow.Login(context.Background(), oauth.LoginOpts{NoBrowser: true}); err == nil {
			t.Fatal("Login() error = nil, want the decode failure")
		}
	})

	// The envelope has two codes, and either one being non-zero is the
	// platform refusing the grant rather than answering with a profile.
	t.Run("a non-zero envelope code is a refusal", func(t *testing.T) {
		for _, body := range []string{
			`{"code":1,"data":{"biz_code":0,"biz_data":{"id":"acct-77"}}}`,
			`{"code":0,"data":{"biz_code":1,"biz_data":{"id":"acct-77"}}}`,
		} {
			server := profileServer(t, http.StatusOK, body, nil)
			flow := oauth.NewDeepSeekAccountFlow(
				oauth.WithCredentialsPath(writeHarnessStore(t, signedInGrant)),
				oauth.WithEndpoints(oauth.Endpoints{UserURL: server.URL}),
			)
			if _, err := flow.Login(context.Background(), oauth.LoginOpts{NoBrowser: true}); !errors.Is(err, oauth.ErrDeepSeekGrantRejected) {
				t.Fatalf("Login(%s) error = %v, want %v", body, err, oauth.ErrDeepSeekGrantRejected)
			}
		}
	})
}

// The store lives under the harness home, which falls back to the user's home
// directory when DSH_HOME names nothing. A host that cannot report one has no
// path to read, so the import names the missing store rather than guessing.
func TestDeepSeekAccountWithoutAHome(t *testing.T) {
	t.Setenv("DSH_HOME", "")
	t.Setenv("HOME", "")
	flow := oauth.NewDeepSeekAccountFlow()
	_, err := flow.Login(context.Background(), oauth.LoginOpts{NoBrowser: true})
	if !errors.Is(err, oauth.ErrDeepSeekHarnessMissing) {
		t.Fatalf("Login() error = %v, want %v", err, oauth.ErrDeepSeekHarnessMissing)
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
