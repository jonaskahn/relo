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
	"github.com/jonaskahn/relo/tests/testkit"
)

func TestDevinBrowserWait(t *testing.T) {
	t.Run("a browser login delivers the redirect itself", func(t *testing.T) {
		server, _ := newProvider(t)
		server.handle("/register", jsonHandler(`{"api_key":"devin-api-key"}`))
		browser := &fakeBrowser{open: callbackDeliverer(t)}
		flow := oauth.NewDevinFlow(
			oauth.WithCallbackPort(-1),
			oauth.WithBrowser(browser),
			oauth.WithCredentialsPath(t.TempDir()+"/absent.toml"),
			oauth.WithEndpoints(oauth.Endpoints{AuthURL: server.url("/login"), APIBaseURL: server.url("/register")}),
		)
		credential, err := flow.Login(context.Background(), oauth.LoginOpts{Timeout: 5 * time.Second})
		if err != nil {
			t.Fatalf("Login() error = %v", err)
		}
		if credential.AccessToken != "devin-api-key" {
			t.Fatalf("credential = %+v, want the registered key", credential)
		}
		if len(browser.opened) != 1 {
			t.Fatalf("opened = %v, want one browser call", browser.opened)
		}
	})

	t.Run("the cli file is read from the user config directory", func(t *testing.T) {
		t.Setenv("XDG_DATA_HOME", "")
		home := t.TempDir()
		t.Setenv("HOME", home)
		configDir, err := os.UserConfigDir()
		if err != nil {
			t.Fatalf("UserConfigDir() error = %v", err)
		}
		directory := filepath.Join(configDir, "devin")
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatalf("create %s: %v", directory, err)
		}
		credentials := tomlLine("windsurf_api_key", "devin-session-token$config")
		if err := os.WriteFile(filepath.Join(directory, "credentials.toml"), []byte(credentials), 0o600); err != nil {
			t.Fatalf("write credentials: %v", err)
		}
		_, credential := loginManual(t, oauth.NewDevinFlow(), "")
		if credential.AccessToken != "devin-session-token$config" {
			t.Fatalf("credential = %+v, want the imported token", credential)
		}
	})

	t.Run("a session token without claims still gets an identity", func(t *testing.T) {
		path := writeFile(t, "credentials.toml", tomlLine("windsurf_api_key", "devin-session-token$opaque"))
		_, credential := loginManual(t, oauth.NewDevinFlow(oauth.WithCredentialsPath(path)), "")
		if credential.AccountID != "" {
			t.Fatalf("credential = %+v, want no invented account id", credential)
		}
	})
}

func TestXAIDiscovery(t *testing.T) {
	t.Run("a device endpoint on an untrusted host is refused", func(t *testing.T) {
		server, _ := newProvider(t)
		server.handle("/discovery", jsonHandler(`{"device_authorization_endpoint":"https://evil.test/oauth2/device/code","token_endpoint":"https://auth.x.ai/oauth2/token"}`))
		flow := oauth.NewXAIDeviceFlow(oauth.WithEndpoints(oauth.Endpoints{DiscoveryURL: server.url("/discovery")}))
		_, err := flow.Login(context.Background(), oauth.LoginOpts{Timeout: time.Second})
		if !errors.Is(err, oauth.ErrUntrustedEndpoint) {
			t.Fatalf("Login() error = %v, want %v", err, oauth.ErrUntrustedEndpoint)
		}
	})

	t.Run("a discovery without a device endpoint is refused", func(t *testing.T) {
		server, _ := newProvider(t)
		server.handle("/discovery", jsonHandler(`{"issuer":"https://auth.x.ai"}`))
		flow := oauth.NewXAIDeviceFlow(oauth.WithEndpoints(oauth.Endpoints{DiscoveryURL: server.url("/discovery")}))
		_, err := flow.Login(context.Background(), oauth.LoginOpts{Timeout: time.Second})
		if !errors.Is(err, oauth.ErrUntrustedEndpoint) {
			t.Fatalf("Login() error = %v, want %v", err, oauth.ErrUntrustedEndpoint)
		}
	})

	t.Run("a discovery failure is reported", func(t *testing.T) {
		server, _ := newProvider(t)
		server.handle("/discovery", jsonHandler(`{"device_authorization_endpoint":"https://auth.x.ai/d","token_endpoint":"https://auth.x.ai/t"}`, http.StatusInternalServerError))
		flow := oauth.NewXAIDeviceFlow(oauth.WithEndpoints(oauth.Endpoints{DiscoveryURL: server.url("/discovery")}))
		if _, err := flow.Login(context.Background(), oauth.LoginOpts{Timeout: time.Second}); err == nil {
			t.Fatal("Login() error = nil, want the discovery failure")
		}
	})
}

func TestTokenClientEdges(t *testing.T) {
	t.Run("an http date Retry-After is honored", func(t *testing.T) {
		clock := testkit.NewFakeClock(time.Now())
		server, _ := newProvider(t)
		server.handle("/token", sequenceHandler(
			jsonHandlerWithHeaders(`{"error":"rate_limited"}`, []int{http.StatusTooManyRequests},
				map[string]string{"Retry-After": time.Now().Add(time.Minute).UTC().Format(http.TimeFormat)}),
			jsonHandler(`{"access_token":"fresh","expires_in":60}`),
		))
		flow := oauth.NewAnthropicFlow(oauth.WithClock(clock), oauth.WithEndpoints(oauth.Endpoints{
			AuthURL: server.url("/authorize"), TokenURL: server.url("/token"),
		}))
		var refreshed *oauth.OAuthCredential
		runWithClock(t, clock, 10*time.Second, func() error {
			credential, err := flow.Refresh(context.Background(), &oauth.OAuthCredential{RefreshToken: "r"})
			refreshed = credential
			return err
		})
		if refreshed == nil || refreshed.AccessToken != "fresh" {
			t.Fatalf("credential = %+v, want the retried response", refreshed)
		}
	})

	t.Run("an empty token body is refused", func(t *testing.T) {
		server, _ := newProvider(t)
		server.handle("/token", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		})
		flow := oauth.NewAnthropicFlow(oauth.WithEndpoints(oauth.Endpoints{
			AuthURL: server.url("/authorize"), TokenURL: server.url("/token"),
		}))
		if _, err := flow.Refresh(context.Background(), &oauth.OAuthCredential{RefreshToken: "r"}); !errors.Is(err, oauth.ErrTokenResponse) {
			t.Fatalf("Refresh() error = %v, want %v", err, oauth.ErrTokenResponse)
		}
	})

	t.Run("a cancelled context stops a retry", func(t *testing.T) {
		clock := testkit.NewFakeClock(time.Now())
		server, _ := newProvider(t)
		server.handle("/token", jsonHandler(`{"error":"rate_limited"}`, http.StatusTooManyRequests))
		flow := oauth.NewAnthropicFlow(oauth.WithClock(clock), oauth.WithEndpoints(oauth.Endpoints{
			AuthURL: server.url("/authorize"), TokenURL: server.url("/token"),
		}))
		ctx, cancel := context.WithCancel(context.Background())
		reached := make(chan struct{}, 4)
		go func() {
			for {
				select {
				case reached <- struct{}{}:
					cancel()
					return
				case <-time.After(time.Second):
					return
				}
			}
		}()
		<-reached
		if _, err := flow.Refresh(ctx, &oauth.OAuthCredential{RefreshToken: "r"}); err == nil {
			t.Fatal("Refresh() error = nil, want the cancelled context")
		}
	})

	t.Run("a transport failure is reported", func(t *testing.T) {
		flow := oauth.NewAnthropicFlow(
			oauth.WithRetries(1),
			oauth.WithHTTPClient(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return nil, errors.New("connection refused")
			})}),
			oauth.WithEndpoints(oauth.Endpoints{AuthURL: "https://auth.test", TokenURL: "https://auth.test"}),
		)
		if _, err := flow.Refresh(context.Background(), &oauth.OAuthCredential{RefreshToken: "r"}); err == nil {
			t.Fatal("Refresh() error = nil, want the transport failure")
		}
	})
}

func TestGuardianStoreFailures(t *testing.T) {
	t.Run("a save failure is reported but does not stop the sweep", func(t *testing.T) {
		now := time.Now()
		store := &fakeStore{
			credentials: []oauth.Credential{{ID: "due", ProviderID: "test", Value: oauth.OAuthCredential{RefreshToken: "r", ExpiresAt: now.Add(time.Minute)}}},
			saveErr:     errors.New("disk is full"),
		}
		guardian := oauth.NewGuardian(registryOf{flow: &fakeFlow{}}, store, oauth.GuardianOptions{Clock: testkit.NewFakeClock(now)})
		if _, err := guardian.RefreshDue(context.Background()); err != nil {
			t.Fatalf("RefreshDue() error = %v, want the sweep to continue", err)
		}
	})

	t.Run("a failed needs_reauth write is logged, not fatal", func(t *testing.T) {
		now := time.Now()
		store := &fakeStore{
			credentials: []oauth.Credential{{ID: "rejected", ProviderID: "test", NeedsReauth: false, Value: oauth.OAuthCredential{RefreshToken: "r", ExpiresAt: now.Add(time.Minute)}}},
			markErr:     errors.New("database is gone"),
		}
		flow := &rejectingFlow{}
		guardian := oauth.NewGuardian(registryOf{flow: flow}, store, oauth.GuardianOptions{Clock: testkit.NewFakeClock(now)})
		if _, err := guardian.RefreshDue(context.Background()); err != nil {
			t.Fatalf("RefreshDue() error = %v, want the sweep to continue", err)
		}
	})
}

type rejectingFlow struct{}

func (rejectingFlow) ProviderID() string { return "test" }

func (rejectingFlow) CallbackPort() int { return 0 }

func (rejectingFlow) Login(context.Context, oauth.LoginOpts) (*oauth.OAuthCredential, error) {
	return nil, oauth.ErrRefreshRejected
}

func (rejectingFlow) Refresh(context.Context, *oauth.OAuthCredential) (*oauth.OAuthCredential, error) {
	return nil, oauth.ErrRefreshRejected
}

func (rejectingFlow) Validate(context.Context, *oauth.OAuthCredential) error { return nil }

func TestKiroRefresh(t *testing.T) {
	session := oauth.KiroSession{
		AccessToken: "kiro-access", RefreshToken: "kiro-refresh", ExpiresAt: time.Now().Add(time.Hour),
		Region: "us-east-1", ProfileARN: "arn:aws:codewhisperer:us-east-1:123456789012:profile/ABCD",
	}

	t.Run("a successful refresh keeps the metadata", func(t *testing.T) {
		server, recorder := newProvider(t)
		server.handle("/refreshToken", jsonHandler(`{"accessToken":"kiro-fresh","refreshToken":"kiro-rotated","expiresIn":1800}`))
		flow := oauth.NewKiroFlow(
			oauth.WithKiroSessions(&fakeKiroSessions{sessions: []oauth.KiroSession{session}}),
			oauth.WithEndpoints(oauth.Endpoints{TokenURL: server.url("/refreshToken")}),
		)
		if _, err := flow.Login(context.Background(), oauth.LoginOpts{Timeout: time.Second}); err != nil {
			t.Fatalf("Login() error = %v", err)
		}
		refreshed, err := flow.Refresh(context.Background(), &oauth.OAuthCredential{
			RefreshToken: "kiro-refresh", Extra: map[string]string{"region": "us-east-1", "profileArn": session.ProfileARN},
		})
		if err != nil {
			t.Fatalf("Refresh() error = %v", err)
		}
		if refreshed.AccessToken != "kiro-fresh" || refreshed.RefreshToken != "kiro-rotated" {
			t.Fatalf("credential = %+v, want the rotated tokens", refreshed)
		}
		if refreshed.Extra["profileArn"] == "" {
			t.Fatalf("extra = %v, want the profile arn kept", refreshed.Extra)
		}
		if !strings.Contains(recorder.last("/refreshToken").body, "kiro-refresh") {
			t.Fatalf("body = %q, want the refresh token", recorder.last("/refreshToken").body)
		}
	})

	t.Run("a malformed refresh answer is refused", func(t *testing.T) {
		server, _ := newProvider(t)
		server.handle("/refreshToken", jsonHandler(`{"refreshToken":"only"}`))
		flow := oauth.NewKiroFlow(oauth.WithEndpoints(oauth.Endpoints{TokenURL: server.url("/refreshToken")}))
		if _, err := flow.Refresh(context.Background(), &oauth.OAuthCredential{RefreshToken: "r"}); !errors.Is(err, oauth.ErrTokenResponse) {
			t.Fatalf("Refresh() error = %v, want %v", err, oauth.ErrTokenResponse)
		}
	})

	t.Run("a snapshot without a token is not used for recovery", func(t *testing.T) {
		server, _ := newProvider(t)
		server.handle("/refreshToken", jsonHandler(`{"error":"invalid_grant"}`, http.StatusUnauthorized))
		flow := oauth.NewKiroFlow(
			oauth.WithKiroSessions(&fakeKiroSessions{sessions: []oauth.KiroSession{{AccountID: "kiro-user"}}}),
			oauth.WithEndpoints(oauth.Endpoints{TokenURL: server.url("/refreshToken")}),
		)
		if _, err := flow.Login(context.Background(), oauth.LoginOpts{Timeout: time.Second}); !errors.Is(err, oauth.ErrKiroSessionEmpty) {
			t.Fatalf("Login() error = %v, want %v", err, oauth.ErrKiroSessionEmpty)
		}
		if _, err := flow.Refresh(context.Background(), &oauth.OAuthCredential{RefreshToken: "gone", AccountID: "kiro-user"}); !errors.Is(err, oauth.ErrRefreshRejected) {
			t.Fatalf("Refresh() error = %v, want %v", err, oauth.ErrRefreshRejected)
		}
	})

	t.Run("the account id matches a snapshot session", func(t *testing.T) {
		server, _ := newProvider(t)
		server.handle("/refreshToken", jsonHandler(`{"error":"invalid_grant"}`, http.StatusUnauthorized))
		snapshot := session
		snapshot.RefreshToken = ""
		snapshot.AccountID = "kiro-user"
		flow := oauth.NewKiroFlow(
			oauth.WithKiroSessions(&fakeKiroSessions{sessions: []oauth.KiroSession{snapshot}}),
			oauth.WithEndpoints(oauth.Endpoints{TokenURL: server.url("/refreshToken")}),
		)
		if _, err := flow.Login(context.Background(), oauth.LoginOpts{Timeout: time.Second}); err != nil {
			t.Fatalf("Login() error = %v", err)
		}
		restored, err := flow.Refresh(context.Background(), &oauth.OAuthCredential{RefreshToken: "gone", AccountID: "kiro-user"})
		if err != nil {
			t.Fatalf("Refresh() error = %v, want the snapshot to restore the credential", err)
		}
		if restored == nil || restored.AccountID != "kiro-user" {
			t.Fatalf("credential = %+v, want the snapshot session", restored)
		}
	})
}

func TestMetaMuseDeviceEdges(t *testing.T) {
	t.Run("a minted response without a key is refused", func(t *testing.T) {
		server, _ := newProvider(t)
		server.handle("/muse-code/key", jsonHandler(`{"user_id":"user-1"}`))
		flow := oauth.NewMetaMuseFlow(oauth.WithEndpoints(oauth.Endpoints{APIBaseURL: server.url("/muse-code/key")}))
		_, err := flow.Refresh(context.Background(), &oauth.OAuthCredential{
			Extra: map[string]string{"museOAuthAccessToken": "account-token"},
		})
		if !errors.Is(err, oauth.ErrMuseKeyMissing) {
			t.Fatalf("Refresh() error = %v, want %v", err, oauth.ErrMuseKeyMissing)
		}
	})

	t.Run("a refused mint must log in again", func(t *testing.T) {
		server, _ := newProvider(t)
		server.handle("/muse-code/key", jsonHandler(`{"error":"unauthorized"}`, http.StatusUnauthorized))
		flow := oauth.NewMetaMuseFlow(oauth.WithEndpoints(oauth.Endpoints{APIBaseURL: server.url("/muse-code/key")}))
		_, err := flow.Refresh(context.Background(), &oauth.OAuthCredential{
			Extra: map[string]string{"museOAuthAccessToken": "account-token"},
		})
		if !errors.Is(err, oauth.ErrRefreshRejected) {
			t.Fatalf("Refresh() error = %v, want %v", err, oauth.ErrRefreshRejected)
		}
	})

	t.Run("a credential without an account token cannot be refreshed", func(t *testing.T) {
		flow := oauth.NewMetaMuseFlow()
		if _, err := flow.Refresh(context.Background(), nil); !errors.Is(err, oauth.ErrNoRefreshToken) {
			t.Fatalf("Refresh() error = %v, want %v", err, oauth.ErrNoRefreshToken)
		}
	})

	t.Run("a raw keychain value is used as the key", func(t *testing.T) {
		flow := oauth.NewMetaMuseFlow(
			oauth.WithCredentialsPath(writeFile(t, "muse.json", `{"providers":{"meta":{"storage":"keychain"}}}`)),
			oauth.WithKeychain(&fakeKeychain{value: "LLM|raw"}),
		)
		_, credential := loginManual(t, flow, "")
		if credential.AccessToken != "LLM|raw" {
			t.Fatalf("credential = %+v, want the raw keychain value", credential)
		}
	})
}

func TestDevicePollEdges(t *testing.T) {
	t.Run("a device answer without grants is refused", func(t *testing.T) {
		clock := testkit.NewFakeClock(time.Now())
		server, _ := newProvider(t)
		server.handle("/usercode", jsonHandler(`{"device_auth_id":"d","user_code":"u","interval":1}`))
		server.handle("/deviceauth/token", jsonHandler(`{"authorization_code":"only"}`))
		flow := oauth.NewChatGPTDeviceFlow(oauth.WithClock(clock), oauth.WithEndpoints(oauth.Endpoints{
			DeviceURL: server.url("/usercode"), DeviceTokenURL: server.url("/deviceauth/token"),
		}))
		var loginErr error
		runWithClock(t, clock, 2*time.Second, func() error {
			_, err := flow.Login(context.Background(), oauth.LoginOpts{Timeout: time.Minute})
			loginErr = err
			return nil
		})
		if !errors.Is(loginErr, oauth.ErrDeviceDenied) {
			t.Fatalf("Login() error = %v, want %v", loginErr, oauth.ErrDeviceDenied)
		}
	})

	t.Run("a device exchange failure is reported", func(t *testing.T) {
		clock := testkit.NewFakeClock(time.Now())
		server, _ := newProvider(t)
		server.handle("/usercode", jsonHandler(`{"device_auth_id":"d","user_code":"u","interval":1}`))
		server.handle("/deviceauth/token", jsonHandler(`{"authorization_code":"code","code_verifier":"verifier"}`))
		server.handle("/oauth/token", jsonHandler(`{"error":"invalid_grant"}`, http.StatusBadRequest))
		flow := oauth.NewChatGPTDeviceFlow(oauth.WithClock(clock), oauth.WithEndpoints(oauth.Endpoints{
			DeviceURL: server.url("/usercode"), DeviceTokenURL: server.url("/deviceauth/token"), TokenURL: server.url("/oauth/token"),
		}))
		var loginErr error
		runWithClock(t, clock, 2*time.Second, func() error {
			_, err := flow.Login(context.Background(), oauth.LoginOpts{Timeout: time.Minute})
			loginErr = err
			return nil
		})
		if loginErr == nil {
			t.Fatal("Login() error = nil, want the exchange failure")
		}
	})

	t.Run("a cancelled device poll stops", func(t *testing.T) {
		clock := testkit.NewFakeClock(time.Now())
		server, _ := newProvider(t)
		server.handle("/usercode", jsonHandler(`{"device_auth_id":"d","user_code":"u","interval":1}`))
		server.handle("/deviceauth/token", jsonHandler(`{}`, http.StatusForbidden))
		flow := oauth.NewChatGPTDeviceFlow(oauth.WithClock(clock), oauth.WithEndpoints(oauth.Endpoints{
			DeviceURL: server.url("/usercode"), DeviceTokenURL: server.url("/deviceauth/token"),
		}))
		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			time.Sleep(50 * time.Millisecond)
			cancel()
		}()
		if _, err := flow.Login(ctx, oauth.LoginOpts{Timeout: time.Minute}); err == nil {
			t.Fatal("Login() error = nil, want the cancelled context")
		}
	})

	t.Run("a cursor login answers with a terminal status", func(t *testing.T) {
		clock := testkit.NewFakeClock(time.Now())
		server, _ := newProvider(t)
		server.handle("/poll", jsonHandler(`{"error":"gone"}`, http.StatusGone))
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

	t.Run("a nous device grant reports a token endpoint failure", func(t *testing.T) {
		clock := testkit.NewFakeClock(time.Now())
		server, _ := newProvider(t)
		server.handle("/code", jsonHandler(`{"device_code":"d","user_code":"u","interval":1}`))
		server.handle("/token", jsonHandler(`{"error":"server_error"}`, http.StatusInternalServerError))
		flow := oauth.NewNousFlow(oauth.WithClock(clock), oauth.WithEndpoints(oauth.Endpoints{
			DeviceURL: server.url("/code"), TokenURL: server.url("/token"),
		}))
		var loginErr error
		runWithClock(t, clock, 2*time.Second, func() error {
			_, err := flow.Login(context.Background(), oauth.LoginOpts{Timeout: time.Minute})
			loginErr = err
			return nil
		})
		if loginErr == nil {
			t.Fatal("Login() error = nil, want the token endpoint failure")
		}
	})
}

func TestProviderRequests(t *testing.T) {
	t.Run("a provider answer that is not json is reported", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w, "this is not json")
		}))
		t.Cleanup(server.Close)
		flow := oauth.NewCommandCodeFlow(oauth.WithEndpoints(oauth.Endpoints{APIBaseURL: server.URL}))
		if _, err := flow.Refresh(context.Background(), &oauth.OAuthCredential{AccessToken: "k"}); err == nil {
			t.Fatal("Refresh() error = nil, want the decode failure")
		}
	})
}

func TestProviderEdgeAnswers(t *testing.T) {
	t.Run("a device start answer that is not json is refused", func(t *testing.T) {
		server, _ := newProvider(t)
		server.handle("/usercode", func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w, "not json")
		})
		flow := oauth.NewChatGPTDeviceFlow(oauth.WithEndpoints(oauth.Endpoints{DeviceURL: server.url("/usercode")}))
		if _, err := flow.Login(context.Background(), oauth.LoginOpts{Timeout: time.Second}); err == nil {
			t.Fatal("Login() error = nil, want the decode failure")
		}
	})

	t.Run("a cursor poll that is rejected ends the login", func(t *testing.T) {
		clock := testkit.NewFakeClock(time.Now())
		server, _ := newProvider(t)
		server.handle("/poll", jsonHandler(`{"error":"expired"}`, http.StatusBadRequest))
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

	t.Run("an onboarding that is refused stops polling", func(t *testing.T) {
		clock := testkit.NewFakeClock(time.Now())
		server, recorder := newProvider(t)
		server.handle("/token", jsonHandler(`{"access_token":"a","refresh_token":"r","expires_in":60}`))
		server.handle("/v1internal:loadCodeAssist", jsonHandler(`{}`))
		server.handle("/v1internal:onboardUser", jsonHandler(`{"error":"permission_denied"}`, http.StatusForbidden))
		flow := oauth.NewGoogleAntigravityFlow(oauth.WithClock(clock), oauth.WithCallbackPort(-1), oauth.WithEndpoints(oauth.Endpoints{
			AuthURL: server.url("/authorize"), TokenURL: server.url("/token"),
			APIBaseURL: server.url(""), DailyAPIBaseURL: server.url(""),
		}))
		var loginErr error
		runWithClock(t, clock, time.Second, func() error {
			_, err := flow.Login(context.Background(), oauth.LoginOpts{
				ManualCode: func(oauth.AuthPrompt) (string, error) { return "code", nil },
				Timeout:    time.Minute,
			})
			loginErr = err
			return nil
		})
		if !errors.Is(loginErr, oauth.ErrOnboardingFailed) {
			t.Fatalf("Login() error = %v, want %v", loginErr, oauth.ErrOnboardingFailed)
		}
		if recorder.count("/v1internal:onboardUser") != 1 {
			t.Fatalf("onboard attempts = %d, want the refusal to stop the poll", recorder.count("/v1internal:onboardUser"))
		}
	})

	t.Run("a claim-bearing session token keeps its account id", func(t *testing.T) {
		token := "devin-session-token$" + jwt(t, map[string]any{"chatgpt_account_id": "devin-account"})
		path := writeFile(t, "credentials.toml", tomlLine("windsurf_api_key", token))
		_, credential := loginManual(t, oauth.NewDevinFlow(oauth.WithCredentialsPath(path)), "")
		if credential.AccountID != "devin-account" {
			t.Fatalf("credential = %+v, want the claim account id", credential)
		}
	})
}

func TestExplicitCallbackPortIsHonored(t *testing.T) {
	flow := oauth.NewAnthropicFlow(oauth.WithCallbackPort(1234))
	if got := flow.CallbackPort(); got != 1234 {
		t.Fatalf("CallbackPort() = %d, want the explicit port", got)
	}
}

func TestCopilotRequestHeadersNameTheClient(t *testing.T) {
	headers := oauth.CopilotRequestHeaders()
	for _, name := range []string{"Editor-Version", "Editor-Plugin-Version", "Copilot-Integration-Id"} {
		if headers[name] == "" {
			t.Fatalf("headers = %v, want %s set", headers, name)
		}
	}
}

func TestCallbackPortForReadsTheRegistry(t *testing.T) {
	registry := oauth.DefaultRegistry()
	if got := registry.CallbackPortFor("no-such-provider"); got != 0 {
		t.Fatalf("CallbackPortFor(unknown) = %d, want zero", got)
	}
	if got, want := registry.CallbackPortFor(oauth.FlowClaude), oauth.NewAnthropicFlow().CallbackPort(); got != want {
		t.Fatalf("CallbackPortFor(claude) = %d, want the flow's own %d", got, want)
	}
	// A device flow binds no listener of its own, so it reports zero.
	if got := registry.CallbackPortFor(oauth.FlowQwen); got != 0 {
		t.Fatalf("CallbackPortFor(qwen) = %d, want zero", got)
	}
}
