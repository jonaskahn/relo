package oauth_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/adapters/oauth"
	"github.com/jonaskahn/relo/tests/testkit"
)

func TestPKCE(t *testing.T) {
	t.Run("rfc 7636 appendix b vector", func(t *testing.T) {
		verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
		want := "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
		if got := oauth.ChallengeForVerifier(verifier); got != want {
			t.Fatalf("ChallengeForVerifier() = %q, want %q", got, want)
		}
	})

	t.Run("generated verifier is inside the allowed length", func(t *testing.T) {
		verifier, challenge := oauth.GeneratePKCE()
		if len(verifier) < 43 || len(verifier) > 128 {
			t.Fatalf("verifier length = %d, want 43..128", len(verifier))
		}
		if challenge != oauth.ChallengeForVerifier(verifier) {
			t.Fatalf("challenge %q does not match the verifier", challenge)
		}
	})

	t.Run("every call returns a fresh verifier", func(t *testing.T) {
		first, _ := oauth.GeneratePKCE()
		second, _ := oauth.GeneratePKCE()
		if first == second {
			t.Fatal("GeneratePKCE() repeated a verifier")
		}
	})

	t.Run("state is unguessable and unique", func(t *testing.T) {
		first, second := oauth.NewState(), oauth.NewState()
		if first == second || len(first) < 20 {
			t.Fatalf("states = %q and %q, want two distinct long values", first, second)
		}
	})
}

func TestCallbackServer(t *testing.T) {
	t.Run("captures the code for a valid state", func(t *testing.T) {
		server, err := oauth.NewCallbackServer(0, "/auth/callback", "expected-state")
		if err != nil {
			t.Fatalf("NewCallbackServer() error = %v", err)
		}
		if !strings.Contains(server.RedirectURI("localhost"), "/auth/callback") {
			t.Fatalf("RedirectURI() = %q, want the callback path", server.RedirectURI("localhost"))
		}
		deliver(t, server.RedirectURI("127.0.0.1")+"?code=abc123&state=expected-state")
		result, err := server.Wait(context.Background())
		if err != nil {
			t.Fatalf("Wait() error = %v", err)
		}
		if result.Code != "abc123" || result.State != "expected-state" {
			t.Fatalf("result = %+v, want the delivered code and state", result)
		}
	})

	t.Run("rejects a wrong state", func(t *testing.T) {
		server, err := oauth.NewCallbackServer(0, "/callback", "expected-state")
		if err != nil {
			t.Fatalf("NewCallbackServer() error = %v", err)
		}
		deliver(t, server.RedirectURI("")+"?code=abc123&state=other-state")
		if _, err := server.Wait(context.Background()); !errors.Is(err, oauth.ErrInvalidState) {
			t.Fatalf("Wait() error = %v, want %v", err, oauth.ErrInvalidState)
		}
	})

	t.Run("reports a missing code and a refused login", func(t *testing.T) {
		server, err := oauth.NewCallbackServer(0, "/callback", "")
		if err != nil {
			t.Fatalf("NewCallbackServer() error = %v", err)
		}
		deliver(t, server.RedirectURI("")+"?state=abc")
		if _, err := server.Wait(context.Background()); !errors.Is(err, oauth.ErrMissingCode) {
			t.Fatalf("Wait() error = %v, want %v", err, oauth.ErrMissingCode)
		}
		refused, err := oauth.NewCallbackServer(0, "/callback", "")
		if err != nil {
			t.Fatalf("NewCallbackServer() error = %v", err)
		}
		deliver(t, refused.RedirectURI("")+"?error=access_denied")
		if _, err := refused.Wait(context.Background()); !errors.Is(err, oauth.ErrLoginCancelled) {
			t.Fatalf("Wait() error = %v, want %v", err, oauth.ErrLoginCancelled)
		}
	})

	t.Run("timeout closes the listener", func(t *testing.T) {
		port := freePort(t)
		if _, err := oauth.ListenForCallback(port, "/callback", 50*time.Millisecond); !errors.Is(err, oauth.ErrCallbackTimeout) {
			t.Fatalf("ListenForCallback() error = %v, want %v", err, oauth.ErrCallbackTimeout)
		}
		rebind(t, port)
	})

	t.Run("port busy reports the port number", func(t *testing.T) {
		occupied := occupiedPort(t)
		_, err := oauth.ListenForCallback(occupied, "/callback", time.Second)
		if !errors.Is(err, oauth.ErrPortBusy) {
			t.Fatalf("ListenForCallback() error = %v, want %v", err, oauth.ErrPortBusy)
		}
		if !strings.Contains(err.Error(), fmt.Sprint(occupied)) {
			t.Fatalf("error = %q, want the port number", err)
		}
	})

	t.Run("cancellation leaves no listener behind", func(t *testing.T) {
		port := freePort(t)
		server, err := oauth.NewCallbackServer(port, "/callback", "")
		if err != nil {
			t.Fatalf("NewCallbackServer() error = %v", err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := server.Wait(ctx); !errors.Is(err, oauth.ErrLoginCancelled) {
			t.Fatalf("Wait() error = %v, want %v", err, oauth.ErrLoginCancelled)
		}
		if err := server.Close(); err != nil {
			t.Fatalf("Close() error = %v, want the repeat close to be a no-op", err)
		}
		rebind(t, port)
	})

	t.Run("serves a close page and refuses other paths", func(t *testing.T) {
		server, err := oauth.NewCallbackServer(0, "/callback", "")
		if err != nil {
			t.Fatalf("NewCallbackServer() error = %v", err)
		}
		defer func() { _ = server.Close() }()
		base := server.RedirectURI("")
		if body := fetch(t, base+"?code=abc"); !strings.Contains(body, "close this tab") {
			t.Fatalf("page = %q, want the close instruction", body)
		}
		if status := fetchStatus(t, base+"/other"); status != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", status)
		}
	})
}

func TestBrowser(t *testing.T) {
	t.Run("headless hosts report no browser", func(t *testing.T) {
		if !oauth.Headless(true) {
			t.Fatal("Headless(true) = false, want true")
		}
		t.Setenv("CI", "1")
		if err := oauth.OpenBrowser("https://example.test"); !errors.Is(err, oauth.ErrBrowserUnavailable) {
			t.Fatalf("OpenBrowser() error = %v, want %v", err, oauth.ErrBrowserUnavailable)
		}
	})

	t.Run("a pasted answer needs no browser", func(t *testing.T) {
		browser := &fakeBrowser{}
		server := tokenServer(t)
		flow := oauth.NewAnthropicFlow(
			oauth.WithCallbackPort(-1),
			oauth.WithBrowser(browser),
			oauth.WithEndpoints(oauth.Endpoints{AuthURL: server.URL + "/authorize", TokenURL: server.URL + "/token"}),
		)
		if _, err := flow.Login(context.Background(), oauth.LoginOpts{
			ManualCode: func(oauth.AuthPrompt) (string, error) { return "code-1", nil },
			Timeout:    time.Second,
		}); err != nil {
			t.Fatalf("Login() error = %v", err)
		}
		if len(browser.opened) != 0 {
			t.Fatalf("opened = %v, want no browser call for a pasted answer", browser.opened)
		}
	})

	t.Run("a browser login delivers the redirect", func(t *testing.T) {
		server := tokenServer(t)
		browser := &fakeBrowser{open: callbackDeliverer(t)}
		flow := oauth.NewChatGPTFlow(
			oauth.WithBrowser(browser),
			oauth.WithCallbackPort(-1),
			oauth.WithEndpoints(oauth.Endpoints{AuthURL: server.URL + "/authorize", TokenURL: server.URL + "/token"}),
		)
		credential, err := flow.Login(context.Background(), oauth.LoginOpts{Timeout: 5 * time.Second})
		if err != nil {
			t.Fatalf("Login() error = %v", err)
		}
		if credential.AccountID != "browser-account" {
			t.Fatalf("account id = %q, want the token workspace", credential.AccountID)
		}
		if len(browser.opened) != 1 {
			t.Fatalf("opened = %v, want one browser call", browser.opened)
		}
	})

	t.Run("headless keeps an explicit browser", func(t *testing.T) {
		server := tokenServer(t)
		browser := &fakeBrowser{open: callbackDeliverer(t)}
		flow := oauth.NewChatGPTFlow(
			oauth.WithBrowser(browser),
			oauth.WithHeadless(true),
			oauth.WithCallbackPort(-1),
			oauth.WithEndpoints(oauth.Endpoints{AuthURL: server.URL + "/authorize", TokenURL: server.URL + "/token"}),
		)
		credential, err := flow.Login(context.Background(), oauth.LoginOpts{Timeout: 5 * time.Second})
		if err != nil {
			t.Fatalf("Login() error = %v", err)
		}
		if credential.AccountID != "browser-account" {
			t.Fatalf("account id = %q, want the token workspace", credential.AccountID)
		}
		if len(browser.opened) != 1 {
			t.Fatalf("opened = %v, want one browser call", browser.opened)
		}
	})
}

func TestClaims(t *testing.T) {
	t.Run("account id from every claim path", func(t *testing.T) {
		top := jwt(t, map[string]any{"chatgpt_account_id": "acc-top"})
		namespaced := jwt(t, map[string]any{"https://api.openai.com/auth": map[string]any{"chatgpt_account_id": "acc-ns"}})
		organization := jwt(t, map[string]any{"organizations": []any{map[string]any{"id": "acc-org"}}})
		tests := []struct {
			name  string
			token string
			want  string
		}{
			{"top level claim", top, "acc-top"},
			{"auth namespace claim", namespaced, "acc-ns"},
			{"organization fallback", organization, "acc-org"},
			{"malformed token", "not-a-jwt", ""},
			{"blank claim", jwt(t, map[string]any{"chatgpt_account_id": "  "}), ""},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				if got := oauth.AccountIDFromTokens(tt.token, ""); got != tt.want {
					t.Fatalf("AccountIDFromTokens() = %q, want %q", got, tt.want)
				}
			})
		}
	})

	t.Run("identity falls back to the subject claim", func(t *testing.T) {
		token := jwt(t, map[string]any{"sub": "user-7", "email": "Person@Example.test"})
		accountID, email := oauth.IdentityFromTokens("", token)
		if accountID != "user-7" || email != "person@example.test" {
			t.Fatalf("IdentityFromTokens() = %q, %q, want the subject and lowercased email", accountID, email)
		}
		if got := oauth.SubjectFromToken(token); got != "user-7" {
			t.Fatalf("SubjectFromToken() = %q, want user-7", got)
		}
		if got := oauth.StringClaim(token, "email"); got != "Person@Example.test" {
			t.Fatalf("StringClaim() = %q, want the raw claim", got)
		}
		if got := oauth.SubjectFromToken("broken"); got != "" {
			t.Fatalf("SubjectFromToken() = %q, want an empty string", got)
		}
	})
}

func TestTokenRetry(t *testing.T) {
	t.Run("honors Retry-After on 429", func(t *testing.T) {
		clock := testkit.NewFakeClock(time.Now())
		attempts := 0
		reached := make(chan struct{}, 1)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			attempts++
			if attempts == 1 {
				w.Header().Set("Retry-After", "2")
				w.WriteHeader(http.StatusTooManyRequests)
				reached <- struct{}{}
				return
			}
			_, _ = io.WriteString(w, `{"access_token":"fresh","refresh_token":"r","expires_in":60}`)
		}))
		t.Cleanup(server.Close)
		flow := oauth.NewAnthropicFlow(
			oauth.WithClock(clock),
			oauth.WithEndpoints(oauth.Endpoints{AuthURL: server.URL, TokenURL: server.URL}),
		)
		results := make(chan tokenOutcome, 1)
		go func() {
			credential, err := flow.Refresh(context.Background(), &oauth.OAuthCredential{RefreshToken: "r"})
			results <- tokenOutcome{credential: credential, err: err}
		}()
		<-reached
		result := awaitResult(t, clock, results)
		if result.credential.AccessToken != "fresh" {
			t.Fatalf("access token = %q, want the retried response", result.credential.AccessToken)
		}
		if attempts != 2 {
			t.Fatalf("attempts = %d, want one retry", attempts)
		}
	})

	t.Run("invalid_grant is a terminal refresh failure", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"error":"invalid_grant","error_description":"expired"}`)
		}))
		t.Cleanup(server.Close)
		flow := oauth.NewAnthropicFlow(oauth.WithEndpoints(oauth.Endpoints{AuthURL: server.URL, TokenURL: server.URL}))
		_, err := flow.Refresh(context.Background(), &oauth.OAuthCredential{RefreshToken: "stale"})
		if !errors.Is(err, oauth.ErrRefreshRejected) {
			t.Fatalf("Refresh() error = %v, want %v", err, oauth.ErrRefreshRejected)
		}
	})

	t.Run("a credential without a refresh token is refused", func(t *testing.T) {
		flow := oauth.NewAnthropicFlow()
		if _, err := flow.Refresh(context.Background(), &oauth.OAuthCredential{}); !errors.Is(err, oauth.ErrNoRefreshToken) {
			t.Fatalf("Refresh() error = %v, want %v", err, oauth.ErrNoRefreshToken)
		}
	})
}

func TestGuardian(t *testing.T) {
	t.Run("refreshes only what expires inside the margin", func(t *testing.T) {
		now := time.Now()
		clock := testkit.NewFakeClock(now)
		store := &fakeStore{credentials: []oauth.Credential{
			{ID: "due", ProviderID: "test", Value: oauth.OAuthCredential{RefreshToken: "r", ExpiresAt: now.Add(3 * time.Minute)}},
			{ID: "later", ProviderID: "test", Value: oauth.OAuthCredential{RefreshToken: "r", ExpiresAt: now.Add(30 * time.Minute)}},
			{ID: "rejected", ProviderID: "test", Value: oauth.OAuthCredential{RefreshToken: "r", ExpiresAt: now.Add(time.Minute)}},
			{ID: "flagged", ProviderID: "test", NeedsReauth: true, Value: oauth.OAuthCredential{RefreshToken: "r", ExpiresAt: now.Add(time.Minute)}},
		}}
		flow := &fakeFlow{refreshErr: map[string]error{"rejected": oauth.ErrRefreshRejected}}
		guardian := oauth.NewGuardian(registryOf{flow: flow}, store, oauth.GuardianOptions{Clock: clock, Margin: 5 * time.Minute})
		refreshed, err := guardian.RefreshDue(context.Background())
		if err != nil {
			t.Fatalf("RefreshDue() error = %v", err)
		}
		if len(refreshed) != 1 || refreshed[0] != "due" {
			t.Fatalf("refreshed = %v, want only the due credential", refreshed)
		}
		if !store.marked("rejected") {
			t.Fatal("a rejected refresh must mark the credential for a new login")
		}
		if store.savedCount() != 1 {
			t.Fatalf("saved = %d, want one persisted refresh", store.savedCount())
		}
	})

	t.Run("reports a store that cannot list", func(t *testing.T) {
		store := &fakeStore{listErr: errors.New("database is gone")}
		guardian := oauth.NewGuardian(registryOf{flow: &fakeFlow{}}, store, oauth.GuardianOptions{})
		if _, err := guardian.RefreshDue(context.Background()); err == nil {
			t.Fatal("RefreshDue() error = nil, want the store failure")
		}
	})

	t.Run("a flow that refuses never stops the sweep", func(t *testing.T) {
		store := &fakeStore{credentials: []oauth.Credential{
			{ID: "broken", ProviderID: "absent", Value: oauth.OAuthCredential{ExpiresAt: time.Now()}},
		}}
		guardian := oauth.NewGuardian(oauth.NewRegistry(), store, oauth.GuardianOptions{})
		refreshed, err := guardian.RefreshDue(context.Background())
		if err != nil {
			t.Fatalf("RefreshDue() error = %v", err)
		}
		if len(refreshed) != 0 {
			t.Fatalf("refreshed = %v, want no credential refreshed", refreshed)
		}
	})
}

func TestRegistry(t *testing.T) {
	t.Run("registers and resolves flows", func(t *testing.T) {
		registry := oauth.NewRegistry()
		registry.RegisterFlow("test", func() oauth.OAuthFlow { return &fakeFlow{} })
		flow, err := registry.Flow("test")
		if err != nil {
			t.Fatalf("Flow() error = %v", err)
		}
		if flow.ProviderID() != "test" {
			t.Fatalf("ProviderID() = %q, want test", flow.ProviderID())
		}
		if got := registry.ProviderIDs(); len(got) != 1 || got[0] != "test" {
			t.Fatalf("ProviderIDs() = %v, want [test]", got)
		}
	})

	t.Run("unknown providers report a typed error", func(t *testing.T) {
		if _, err := oauth.NewRegistry().Flow("absent"); !errors.Is(err, oauth.ErrFlowNotFound) {
			t.Fatalf("Flow() error = %v, want %v", err, oauth.ErrFlowNotFound)
		}
	})

	t.Run("the default registry carries every shipped flow", func(t *testing.T) {
		registry := oauth.DefaultRegistry()
		want := []string{"claude", "command-code", "copilot", "cursor", "devin", "google-antigravity", "grok", "iflow", "kimi", "kiro", "meta-muse", "nous", "openai-codex", "openai-codex-device", "orcarouter-oauth", "qwen"}
		got := registry.ProviderIDs()
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("ProviderIDs() = %v, want %v", got, want)
		}
		for _, id := range want {
			flow, err := registry.Flow(id)
			if err != nil {
				t.Fatalf("Flow(%q) error = %v", id, err)
			}
			if flow.ProviderID() != id {
				t.Fatalf("Flow(%q).ProviderID() = %q", id, flow.ProviderID())
			}
		}
	})
}

func deliver(t *testing.T, url string) {
	t.Helper()
	response, err := http.Get(url)
	if err != nil {
		t.Fatalf("deliver %s: %v", url, err)
	}
	defer func() { _ = response.Body.Close() }()
	_, _ = io.Copy(io.Discard, response.Body)
}

func fetch(t *testing.T, url string) string {
	t.Helper()
	response, err := http.Get(url)
	if err != nil {
		t.Fatalf("get %s: %v", url, err)
	}
	defer func() { _ = response.Body.Close() }()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read %s: %v", url, err)
	}
	return string(body)
}

func fetchStatus(t *testing.T, url string) int {
	t.Helper()
	response, err := http.Get(url)
	if err != nil {
		t.Fatalf("get %s: %v", url, err)
	}
	defer func() { _ = response.Body.Close() }()
	_, _ = io.Copy(io.Discard, response.Body)
	return response.StatusCode
}

func freePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("find a free port: %v", err)
	}
	defer func() { _ = listener.Close() }()
	return listener.Addr().(*net.TCPAddr).Port
}

func occupiedPort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("occupy a port: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	return listener.Addr().(*net.TCPAddr).Port
}

func rebind(t *testing.T, port int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err == nil {
			_ = listener.Close()
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("rebind port %d: %v, want the listener to be closed", port, err)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func awaitResult(t *testing.T, clock *testkit.FakeClock, results <-chan tokenOutcome) tokenOutcome {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		select {
		case result := <-results:
			if result.err != nil {
				t.Fatalf("Refresh() error = %v", result.err)
			}
			return result
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("Refresh() did not finish after the retry delay elapsed")
		}
		clock.Add(time.Second)
		time.Sleep(20 * time.Millisecond)
	}
}

func tokenServer(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		access := jwt(t, map[string]any{
			"https://api.openai.com/auth":    map[string]any{"chatgpt_account_id": "browser-account"},
			"https://api.openai.com/profile": map[string]any{"email": "browser@example.test"},
		})
		_, _ = io.WriteString(w, `{"access_token":"`+access+`","refresh_token":"refresh-token","expires_in":3600}`)
	}))
	t.Cleanup(server.Close)
	return server
}

// callbackDeliverer answers an authorization URL by visiting the
// redirect_uri the flow registered, the way a browser would.
func callbackDeliverer(t *testing.T) func(url string) {
	t.Helper()
	return func(rawURL string) {
		parsed, err := url.Parse(rawURL)
		if err != nil {
			t.Errorf("parse authorization url: %v", err)
			return
		}
		query := parsed.Query()
		redirect := query.Get("redirect_uri")
		if redirect == "" {
			t.Error("authorization url carries no redirect_uri")
			return
		}
		deliver(t, redirect+"?code=code-1&state="+query.Get("state"))
	}
}

const headerClaims = "{\"alg\":\"none\"}"

func jwt(t *testing.T, claims map[string]any) string {
	t.Helper()
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("encode claims: %v", err)
	}
	header := base64.RawURLEncoding.EncodeToString([]byte(headerClaims))
	return header + "." + base64.RawURLEncoding.EncodeToString(payload) + ".signature"
}

type tokenOutcome struct {
	credential *oauth.OAuthCredential
	err        error
}

type fakeBrowser struct {
	opened []string
	open   func(url string)
}

func (b *fakeBrowser) Open(url string) error {
	b.opened = append(b.opened, url)
	if b.open != nil {
		b.open(url)
	}
	return nil
}

type fakeFlow struct {
	mu         sync.Mutex
	refreshes  int
	refreshErr map[string]error
}

func (f *fakeFlow) ProviderID() string { return "test" }

func (f *fakeFlow) CallbackPort() int { return 0 }

func (f *fakeFlow) Login(context.Context, oauth.LoginOpts) (*oauth.OAuthCredential, error) {
	return &oauth.OAuthCredential{AccessToken: "token"}, nil
}

func (f *fakeFlow) Refresh(_ context.Context, cred *oauth.OAuthCredential) (*oauth.OAuthCredential, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.refreshes++
	if f.refreshErr != nil {
		if err, found := f.refreshErr["rejected"]; found && cred.RefreshToken == "r" && cred.ExpiresAt.Before(time.Now().Add(2*time.Minute)) {
			return nil, err
		}
	}
	refreshed := cred.Clone()
	refreshed.AccessToken = "refreshed"
	refreshed.ExpiresAt = time.Now().Add(time.Hour)
	return &refreshed, nil
}

func (f *fakeFlow) Validate(context.Context, *oauth.OAuthCredential) error { return nil }

type registryOf struct {
	flow oauth.OAuthFlow
}

func (r registryOf) Flow(string) (oauth.OAuthFlow, error) { return r.flow, nil }

type fakeStore struct {
	mu          sync.Mutex
	credentials []oauth.Credential
	listErr     error
	saveErr     error
	markErr     error
	saved       int
	needsReauth map[string]bool
}

func (s *fakeStore) List(context.Context) ([]oauth.Credential, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	return s.credentials, nil
}

func (s *fakeStore) Save(context.Context, oauth.Credential) error {
	if s.saveErr != nil {
		return s.saveErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.saved++
	return nil
}

func (s *fakeStore) savedCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saved
}

func (s *fakeStore) marked(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.needsReauth[id]
}

func TestGuardianSkipsProvidersWhenRefreshIsOff(t *testing.T) {
	now := time.Now()
	store := &fakeStore{credentials: []oauth.Credential{
		{ID: "claude-1", ProviderID: oauth.FlowClaude, Value: oauth.OAuthCredential{RefreshToken: "r", ExpiresAt: now}},
		{ID: "other", ProviderID: "test", Value: oauth.OAuthCredential{RefreshToken: "r", ExpiresAt: now}},
	}}
	flow := &fakeFlow{}
	guardian := oauth.NewGuardian(registryOf{flow: flow}, store, oauth.GuardianOptions{
		Clock: testkit.NewFakeClock(now), Margin: 5 * time.Minute, Policy: refreshOff{},
	})
	refreshed, err := guardian.RefreshDue(context.Background())
	if err != nil {
		t.Fatalf("RefreshDue() error = %v", err)
	}
	if len(refreshed) != 0 {
		t.Fatalf("refreshed = %v, want no credential refreshed", refreshed)
	}
}

func TestGuardianAdoptsAPeerRotation(t *testing.T) {
	now := time.Now()
	store := &fakeStore{credentials: []oauth.Credential{
		{ID: "due", ProviderID: "test", Value: oauth.OAuthCredential{RefreshToken: "r", ExpiresAt: now.Add(time.Minute)}},
	}}
	guardian := oauth.NewGuardian(registryOf{flow: &rotatingPeerFlow{store: store}}, store, oauth.GuardianOptions{
		Clock: testkit.NewFakeClock(now), Margin: 5 * time.Minute,
	})
	if _, err := guardian.RefreshDue(context.Background()); err != nil {
		t.Fatalf("RefreshDue() error = %v", err)
	}
	if store.marked("due") {
		t.Fatal("a credential a peer already rotated must not be marked for a new login")
	}
}

type rotatingPeerFlow struct {
	store *fakeStore
}

func (f *rotatingPeerFlow) ProviderID() string { return "test" }

func (f *rotatingPeerFlow) CallbackPort() int { return 0 }

func (f *rotatingPeerFlow) Login(context.Context, oauth.LoginOpts) (*oauth.OAuthCredential, error) {
	return nil, errors.New("unused")
}

func (f *rotatingPeerFlow) Validate(context.Context, *oauth.OAuthCredential) error { return nil }

func (f *rotatingPeerFlow) Refresh(_ context.Context, cred *oauth.OAuthCredential) (*oauth.OAuthCredential, error) {
	f.store.credentials[0].Value.RefreshToken = "peer-rotated"
	return nil, oauth.ErrRefreshRejected
}

type refreshOff struct{}

func (refreshOff) AutoRefresh(context.Context, string) bool { return false }

func (s *fakeStore) MarkNeedsReauth(_ context.Context, id string) error {
	if s.markErr != nil {
		return s.markErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.needsReauth == nil {
		s.needsReauth = map[string]bool{}
	}
	s.needsReauth[id] = true
	return nil
}
