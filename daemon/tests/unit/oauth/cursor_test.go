package oauth_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/adapters/oauth"
	"github.com/jonaskahn/relo/tests/testkit"
)

func TestCursorFlow(t *testing.T) {
	t.Run("the login url carries only the challenge", func(t *testing.T) {
		clock := testkit.NewFakeClock(time.Now())
		access := jwt(t, map[string]any{"sub": "cursor-user", "email": "User@Example.test", "exp": float64(time.Now().Add(time.Hour).Unix())})
		server, recorder := newProvider(t)
		server.handle("/poll", sequenceHandler(
			jsonHandler(`{"status":"pending"}`, 404),
			jsonHandler(`{"accessToken":"`+access+`","refreshToken":"cursor-refresh"}`),
		))
		flow := oauth.NewCursorFlow(oauth.WithClock(clock), oauth.WithEndpoints(oauth.Endpoints{
			AuthURL: server.url("/loginDeepControl"), APIBaseURL: server.url("/poll"),
		}))
		var prompt oauth.AuthPrompt
		var credential *oauth.OAuthCredential
		runWithClock(t, clock, 2*time.Second, func() error {
			created, err := flow.Login(context.Background(), oauth.LoginOpts{
				Prompt:    func(shown oauth.AuthPrompt) { prompt = shown },
				NoBrowser: true,
				Timeout:   time.Minute,
			})
			credential = created
			return err
		})
		if credential.AccountID != "cursor-user" || credential.Email != "user@example.test" {
			t.Fatalf("credential = %+v, want the token identity", credential)
		}
		if !strings.Contains(prompt.URL, "mode=login") || !strings.Contains(prompt.URL, "redirectTarget=cli") {
			t.Fatalf("authorize url = %q, want the deep control parameters", prompt.URL)
		}
		if strings.Contains(prompt.URL, "verifier=") {
			t.Fatalf("authorize url = %q, must not carry the pkce verifier", prompt.URL)
		}
		if !credential.ExpiresAt.Before(time.Now().Add(time.Hour)) {
			t.Fatalf("expiry = %v, want the token exp less the refresh skew", credential.ExpiresAt)
		}
		if recorder.count("/poll") < 2 {
			t.Fatalf("polls = %d, want the pending answer polled again", recorder.count("/poll"))
		}
		if !strings.Contains(recorder.last("/poll").path, "/poll") {
			t.Fatalf("poll path = %q", recorder.last("/poll").path)
		}
	})

	t.Run("a terminal status fails the login", func(t *testing.T) {
		clock := testkit.NewFakeClock(time.Now())
		server, _ := newProvider(t)
		server.handle("/poll", jsonHandler(`{"error":"revoked"}`, 403))
		flow := oauth.NewCursorFlow(oauth.WithClock(clock), oauth.WithEndpoints(oauth.Endpoints{
			AuthURL: server.url("/login"), APIBaseURL: server.url("/poll"),
		}))
		var loginErr error
		runWithClock(t, clock, 2*time.Second, func() error {
			_, err := flow.Login(context.Background(), oauth.LoginOpts{NoBrowser: true, Timeout: time.Minute})
			loginErr = err
			return nil
		})
		if !errors.Is(loginErr, oauth.ErrCursorLoginRejected) {
			t.Fatalf("Login() error = %v, want %v", loginErr, oauth.ErrCursorLoginRejected)
		}
	})

	t.Run("an unanswered login runs out of attempts", func(t *testing.T) {
		clock := testkit.NewFakeClock(time.Now())
		server, _ := newProvider(t)
		server.handle("/poll", jsonHandler(`{"status":"pending"}`, 404))
		flow := oauth.NewCursorFlow(oauth.WithClock(clock), oauth.WithEndpoints(oauth.Endpoints{
			AuthURL: server.url("/login"), APIBaseURL: server.url("/poll"),
		}))
		var loginErr error
		runWithClock(t, clock, time.Minute, func() error {
			_, err := flow.Login(context.Background(), oauth.LoginOpts{NoBrowser: true, Timeout: time.Hour})
			loginErr = err
			return nil
		})
		if !errors.Is(loginErr, oauth.ErrCallbackTimeout) {
			t.Fatalf("Login() error = %v, want %v", loginErr, oauth.ErrCallbackTimeout)
		}
	})

	t.Run("a poll answer without tokens is refused", func(t *testing.T) {
		clock := testkit.NewFakeClock(time.Now())
		server, _ := newProvider(t)
		server.handle("/poll", jsonHandler(`{"accessToken":"only"}`))
		flow := oauth.NewCursorFlow(oauth.WithClock(clock), oauth.WithEndpoints(oauth.Endpoints{
			AuthURL: server.url("/login"), APIBaseURL: server.url("/poll"),
		}))
		var loginErr error
		runWithClock(t, clock, time.Second, func() error {
			_, err := flow.Login(context.Background(), oauth.LoginOpts{NoBrowser: true, Timeout: time.Minute})
			loginErr = err
			return nil
		})
		if !errors.Is(loginErr, oauth.ErrTokenResponse) {
			t.Fatalf("Login() error = %v, want %v", loginErr, oauth.ErrTokenResponse)
		}
	})

	t.Run("refresh exchanges the stored token", func(t *testing.T) {
		server, recorder := newProvider(t)
		server.handle("/exchange", jsonHandler(`{"accessToken":"cursor-fresh","refreshToken":"cursor-rotated"}`))
		flow := oauth.NewCursorFlow(oauth.WithEndpoints(oauth.Endpoints{
			AuthURL: server.url("/login"), APIBaseURL: server.url("/poll"), TokenURL: server.url("/exchange"),
		}))
		refreshed, err := flow.Refresh(context.Background(), &oauth.OAuthCredential{
			AccessToken: "old", RefreshToken: "cursor-refresh", AccountID: "cursor-user",
		})
		if err != nil {
			t.Fatalf("Refresh() error = %v", err)
		}
		if refreshed.AccessToken != "cursor-fresh" || refreshed.RefreshToken != "cursor-rotated" {
			t.Fatalf("credential = %+v, want the rotated tokens", refreshed)
		}
		if refreshed.AccountID != "cursor-user" {
			t.Fatalf("credential = %+v, want the identity kept", refreshed)
		}
		if recorder.last("/exchange").authorization != "Bearer cursor-refresh" {
			t.Fatalf("authorization = %q, want the refresh token", recorder.last("/exchange").authorization)
		}
		if _, err := flow.Refresh(context.Background(), &oauth.OAuthCredential{}); !errors.Is(err, oauth.ErrNoRefreshToken) {
			t.Fatalf("Refresh() error = %v, want %v", err, oauth.ErrNoRefreshToken)
		}
	})

	t.Run("refresh reports a refusal and a malformed answer", func(t *testing.T) {
		server, _ := newProvider(t)
		server.handle("/exchange", jsonHandler(`{"error":"invalid_grant"}`, 401))
		flow := oauth.NewCursorFlow(oauth.WithEndpoints(oauth.Endpoints{TokenURL: server.url("/exchange")}))
		if _, err := flow.Refresh(context.Background(), &oauth.OAuthCredential{RefreshToken: "revoked"}); err == nil {
			t.Fatal("Refresh() error = nil, want the refusal")
		}
		server.handle("/exchange", jsonHandler(`not json`))
		if _, err := flow.Refresh(context.Background(), &oauth.OAuthCredential{RefreshToken: "r"}); err == nil {
			t.Fatal("Refresh() error = nil, want the decode failure")
		}
	})

	t.Run("validate reports the credential state", func(t *testing.T) {
		flow := oauth.NewCursorFlow()
		if err := flow.Validate(context.Background(), &oauth.OAuthCredential{AccessToken: "a"}); err != nil {
			t.Fatalf("Validate() error = %v, want a token without an expiry to pass", err)
		}
		if err := flow.Validate(context.Background(), nil); !errors.Is(err, oauth.ErrTokenResponse) {
			t.Fatalf("Validate() error = %v, want %v", err, oauth.ErrTokenResponse)
		}
	})
}

func TestStateValidation(t *testing.T) {
	t.Run("a pasted redirect with another state is refused", func(t *testing.T) {
		server, _ := newProvider(t)
		server.handle("/token", jsonHandler(`{"access_token":"a","refresh_token":"r"}`))
		flow := oauth.NewChatGPTFlow(oauth.WithCallbackPort(-1), oauth.WithEndpoints(oauth.Endpoints{
			AuthURL: server.url("/authorize"), TokenURL: server.url("/token"),
		}))
		_, err := flow.Login(context.Background(), oauth.LoginOpts{
			ManualCode: func(oauth.AuthPrompt) (string, error) {
				return "http://localhost:1455/auth/callback?code=stolen&state=not-the-expected-state", nil
			},
			NoBrowser: true,
			Timeout:   time.Second,
		})
		if !errors.Is(err, oauth.ErrInvalidState) {
			t.Fatalf("Login() error = %v, want %v", err, oauth.ErrInvalidState)
		}
	})

	t.Run("a pasted answer without a code is refused", func(t *testing.T) {
		server, _ := newProvider(t)
		flow := oauth.NewChatGPTFlow(oauth.WithCallbackPort(-1), oauth.WithEndpoints(oauth.Endpoints{AuthURL: server.url("/authorize"), TokenURL: server.url("/token")}))
		_, err := flow.Login(context.Background(), oauth.LoginOpts{
			ManualCode: func(oauth.AuthPrompt) (string, error) { return "state=abc", nil },
			NoBrowser:  true,
			Timeout:    time.Second,
		})
		if !errors.Is(err, oauth.ErrMissingCode) {
			t.Fatalf("Login() error = %v, want %v", err, oauth.ErrMissingCode)
		}
	})
}

func TestGuardianBackgroundSweep(t *testing.T) {
	t.Run("run refreshes on every tick until cancelled", func(t *testing.T) {
		now := time.Now()
		clock := testkit.NewFakeClock(now)
		store := &fakeStore{credentials: []oauth.Credential{{
			ID: "due", ProviderID: "test",
			Value: oauth.OAuthCredential{RefreshToken: "r", ExpiresAt: now.Add(time.Minute)},
		}}}
		guardian := oauth.NewGuardian(registryOf{flow: &fakeFlow{}}, store, oauth.GuardianOptions{Clock: clock})
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() {
			guardian.Run(ctx, time.Minute)
			close(done)
		}()
		deadline := time.Now().Add(2 * time.Second)
		for store.savedCount() == 0 && time.Now().Before(deadline) {
			clock.Add(30 * time.Second)
			time.Sleep(10 * time.Millisecond)
		}
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("Run() did not stop when its context ended")
		}
		if store.savedCount() == 0 {
			t.Fatal("Run() did not refresh the due credential")
		}
	})
}

func TestFlowOptions(t *testing.T) {
	t.Run("every option reaches the flow", func(t *testing.T) {
		browser := &fakeBrowser{}
		clock := testkit.NewFakeClock(time.Now())
		client := &httpClientStub{}
		flow := oauth.NewAnthropicFlow(
			oauth.WithCallbackPort(-1),
			oauth.WithHTTPClient(client.client()),
			oauth.WithClock(clock),
			oauth.WithBrowser(browser),
			oauth.WithRetries(1),
			oauth.WithCallbackPort(-1),
			oauth.WithRedirectHost("localhost"),
			oauth.WithEndpoints(oauth.Endpoints{AuthURL: "https://auth.test/authorize", TokenURL: "https://auth.test/token"}),
			oauth.WithWarn(func(string) {}),
			oauth.WithLogger(nil),
			oauth.WithKeychain(&fakeKeychain{value: "k"}),
			oauth.WithKiroSessions(&fakeKiroSessions{}),
		)
		var loginErr error
		runWithClock(t, clock, time.Second, func() error {
			_, err := flow.Login(context.Background(), oauth.LoginOpts{
				ManualCode: func(oauth.AuthPrompt) (string, error) { return "code", nil },
				Timeout:    time.Second,
			})
			loginErr = err
			return nil
		})
		if loginErr == nil {
			t.Fatal("Login() error = nil, want the stub client to fail the exchange")
		}
		if client.calls == 0 {
			t.Fatal("the injected http client was not used")
		}
	})

	t.Run("a login without an explicit timeout uses the default", func(t *testing.T) {
		flow := oauth.NewAnthropicFlow(oauth.WithCallbackPort(-1), oauth.WithEndpoints(oauth.Endpoints{AuthURL: "https://auth.test", TokenURL: "https://auth.test"}))
		_, err := flow.Login(context.Background(), oauth.LoginOpts{
			ManualCode: func(oauth.AuthPrompt) (string, error) { return "", errors.New("stopped") },
		})
		if err == nil {
			t.Fatal("Login() error = nil, want the manual-code failure")
		}
	})

	t.Run("a login without a prompt never panics", func(t *testing.T) {
		flow := oauth.NewDevinFlow(oauth.WithCredentialsPath(t.TempDir() + "/absent.toml"))
		if err := flow.Validate(context.Background(), &oauth.OAuthCredential{AccessToken: "a"}); err != nil {
			t.Fatalf("Validate() error = %v", err)
		}
	})
}

// httpClientStub fails every call, so a test can prove an injected client
// is the one a flow uses without touching the network.
type httpClientStub struct {
	calls int
}

func (s *httpClientStub) client() *http.Client {
	return &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		s.calls++
		return nil, errors.New("the network is disabled in this test")
	})}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}
