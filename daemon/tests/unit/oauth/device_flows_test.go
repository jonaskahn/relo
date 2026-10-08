package oauth_test

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/adapters/oauth"
	"github.com/jonaskahn/relo/tests/testkit"
)

func TestChatGPTDeviceFlow(t *testing.T) {
	t.Run("pending then approved", func(t *testing.T) {
		clock := testkit.NewFakeClock(time.Now())
		server, recorder := newProvider(t)
		server.handle("/usercode", jsonHandler(`{"device_auth_id":"device-1","user_code":"ABCD-1234","interval":5}`))
		server.handle("/deviceauth/token", sequenceHandler(
			jsonHandler(`{}`, http.StatusForbidden),
			jsonHandler(`{"authorization_code":"code-1","code_verifier":"verifier-1"}`),
		))
		access := jwt(t, map[string]any{
			"https://api.openai.com/auth":    map[string]any{"chatgpt_account_id": "acc-device"},
			"https://api.openai.com/profile": map[string]any{"email": "device@example.test"},
		})
		server.handle("/oauth/token", jsonHandler(`{"access_token":"`+access+`","refresh_token":"device-refresh","expires_in":3600}`))
		flow := oauth.NewChatGPTDeviceFlow(oauth.WithClock(clock), oauth.WithEndpoints(oauth.Endpoints{
			DeviceURL: server.url("/usercode"), DeviceTokenURL: server.url("/deviceauth/token"),
			VerificationURL: server.url("/device"), TokenURL: server.url("/oauth/token"),
		}))
		var prompt oauth.AuthPrompt
		var credential *oauth.OAuthCredential
		runWithClock(t, clock, 6*time.Second, func() error {
			created, err := flow.Login(context.Background(), oauth.LoginOpts{
				Prompt:  func(shown oauth.AuthPrompt) { prompt = shown },
				Timeout: time.Minute,
			})
			credential = created
			return err
		})
		if credential.AccountID != "acc-device" {
			t.Fatalf("credential = %+v, want the exchanged account", credential)
		}
		if prompt.DeviceCode != "ABCD-1234" || !strings.Contains(prompt.Instructions, "ABCD-1234") {
			t.Fatalf("prompt = %+v, want the user code", prompt)
		}
		if recorder.count("/deviceauth/token") < 2 {
			t.Fatalf("polls = %d, want the pending answer to be polled again", recorder.count("/deviceauth/token"))
		}
	})

	t.Run("an expired grant is a typed error", func(t *testing.T) {
		clock := testkit.NewFakeClock(time.Now())
		server, _ := newProvider(t)
		server.handle("/usercode", jsonHandler(`{"device_auth_id":"device-1","usercode":"ABCD","interval":5}`))
		server.handle("/deviceauth/token", jsonHandler(`{}`, http.StatusForbidden))
		flow := oauth.NewChatGPTDeviceFlow(oauth.WithClock(clock), oauth.WithEndpoints(oauth.Endpoints{
			DeviceURL: server.url("/usercode"), DeviceTokenURL: server.url("/deviceauth/token"),
		}))
		var loginErr error
		runWithClock(t, clock, time.Minute, func() error {
			_, err := flow.Login(context.Background(), oauth.LoginOpts{Timeout: time.Hour})
			loginErr = err
			return nil
		})
		if !errors.Is(loginErr, oauth.ErrDeviceExpired) {
			t.Fatalf("Login() error = %v, want %v", loginErr, oauth.ErrDeviceExpired)
		}
	})

	t.Run("start failures and malformed answers are reported", func(t *testing.T) {
		server, _ := newProvider(t)
		server.handle("/usercode", jsonHandler(`{"error":"invalid_client"}`, http.StatusBadRequest))
		flow := oauth.NewChatGPTDeviceFlow(oauth.WithEndpoints(oauth.Endpoints{DeviceURL: server.url("/usercode")}))
		if _, err := flow.Login(context.Background(), oauth.LoginOpts{Timeout: time.Second}); err == nil {
			t.Fatal("Login() error = nil, want the start failure")
		}
		server.handle("/usercode", jsonHandler(`{"interval":5}`))
		if _, err := flow.Login(context.Background(), oauth.LoginOpts{Timeout: time.Second}); !errors.Is(err, oauth.ErrDeviceDenied) {
			t.Fatalf("Login() error = %v, want %v", err, oauth.ErrDeviceDenied)
		}
	})

	t.Run("refresh and validate", func(t *testing.T) {
		server, _ := newProvider(t)
		server.handle("/oauth/token", jsonHandler(`{"access_token":"refreshed","refresh_token":"rotated","expires_in":60}`))
		flow := oauth.NewChatGPTDeviceFlow(oauth.WithEndpoints(oauth.Endpoints{TokenURL: server.url("/oauth/token")}))
		refreshed, err := flow.Refresh(context.Background(), &oauth.OAuthCredential{RefreshToken: "r"})
		if err != nil {
			t.Fatalf("Refresh() error = %v", err)
		}
		if refreshed.AccessToken != "refreshed" {
			t.Fatalf("credential = %+v, want the refreshed token", refreshed)
		}
		if err := flow.Validate(context.Background(), refreshed); err != nil {
			t.Fatalf("Validate() error = %v", err)
		}
		if err := flow.Validate(context.Background(), nil); !errors.Is(err, oauth.ErrTokenResponse) {
			t.Fatalf("Validate() error = %v, want %v", err, oauth.ErrTokenResponse)
		}
	})
}

func TestKimiFlow(t *testing.T) {
	t.Run("device grant succeeds after a slow_down", func(t *testing.T) {
		clock := testkit.NewFakeClock(time.Now())
		server, _ := newProvider(t)
		server.handle("/device_authorization", jsonHandler(`{"device_code":"device-1","user_code":"KIMI-1","verification_uri":"https://auth.kimi.test/device","interval":1}`))
		server.handle("/token", sequenceHandler(
			jsonHandler(`{"error":"authorization_pending"}`, http.StatusBadRequest),
			jsonHandler(`{"error":"slow_down"}`, http.StatusBadRequest),
			jsonHandler(`{"access_token":"kimi-access","refresh_token":"kimi-refresh","expires_in":3600}`),
		))
		flow := oauth.NewKimiFlow(oauth.WithClock(clock), oauth.WithEndpoints(oauth.Endpoints{
			DeviceURL: server.url("/device_authorization"), TokenURL: server.url("/token"),
		}))
		var prompt oauth.AuthPrompt
		var credential *oauth.OAuthCredential
		runWithClock(t, clock, 3*time.Second, func() error {
			created, err := flow.Login(context.Background(), oauth.LoginOpts{
				Prompt:  func(shown oauth.AuthPrompt) { prompt = shown },
				Timeout: time.Minute,
			})
			credential = created
			return err
		})
		if credential.AccessToken != "kimi-access" {
			t.Fatalf("credential = %+v, want the device token", credential)
		}
		if prompt.DeviceCode != "KIMI-1" || prompt.URL == "" {
			t.Fatalf("prompt = %+v, want the user code and page", prompt)
		}
	})

	t.Run("a denied grant stops the poll", func(t *testing.T) {
		clock := testkit.NewFakeClock(time.Now())
		server, _ := newProvider(t)
		server.handle("/device_authorization", jsonHandler(`{"device_code":"d","user_code":"u","interval":1}`))
		server.handle("/token", jsonHandler(`{"error":"access_denied"}`, http.StatusBadRequest))
		flow := oauth.NewKimiFlow(oauth.WithClock(clock), oauth.WithEndpoints(oauth.Endpoints{
			DeviceURL: server.url("/device_authorization"), TokenURL: server.url("/token"),
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

	t.Run("a start answer without codes is refused", func(t *testing.T) {
		server, _ := newProvider(t)
		server.handle("/device_authorization", jsonHandler(`{"error":"invalid_client"}`, http.StatusBadRequest))
		flow := oauth.NewKimiFlow(oauth.WithEndpoints(oauth.Endpoints{DeviceURL: server.url("/device_authorization")}))
		if _, err := flow.Login(context.Background(), oauth.LoginOpts{Timeout: time.Second}); err == nil {
			t.Fatal("Login() error = nil, want the start failure")
		}
	})

	t.Run("refresh and validate", func(t *testing.T) {
		server, _ := newProvider(t)
		server.handle("/token", jsonHandler(`{"access_token":"kimi-fresh","expires_in":60}`))
		flow := oauth.NewKimiFlow(oauth.WithEndpoints(oauth.Endpoints{TokenURL: server.url("/token")}))
		refreshed, err := flow.Refresh(context.Background(), &oauth.OAuthCredential{RefreshToken: "r"})
		if err != nil {
			t.Fatalf("Refresh() error = %v", err)
		}
		if refreshed.AccessToken != "kimi-fresh" || refreshed.RefreshToken != "r" {
			t.Fatalf("credential = %+v, want the refreshed token and the kept refresh token", refreshed)
		}
		if err := flow.Validate(context.Background(), refreshed); err != nil {
			t.Fatalf("Validate() error = %v", err)
		}
	})
}

func TestNousFlow(t *testing.T) {
	t.Run("requires the inference scope", func(t *testing.T) {
		clock := testkit.NewFakeClock(time.Now())
		server, _ := newProvider(t)
		server.handle("/api/oauth/device/code", jsonHandler(`{"device_code":"d","user_code":"u","verification_uri":"https://portal.test/device","interval":1}`))
		server.handle("/api/oauth/token", jsonHandler(`{"access_token":"`+jwt(t, map[string]any{"sub": "user-1"})+`","refresh_token":"r","expires_in":60}`))
		flow := oauth.NewNousFlow(oauth.WithClock(clock), oauth.WithEndpoints(oauth.Endpoints{
			DeviceURL: server.url("/api/oauth/device/code"), TokenURL: server.url("/api/oauth/token"),
		}))
		var loginErr error
		runWithClock(t, clock, 2*time.Second, func() error {
			_, err := flow.Login(context.Background(), oauth.LoginOpts{Timeout: time.Minute})
			loginErr = err
			return nil
		})
		if !errors.Is(loginErr, oauth.ErrMissingScope) {
			t.Fatalf("Login() error = %v, want %v", loginErr, oauth.ErrMissingScope)
		}
	})

	t.Run("accepts a token that grants the scope", func(t *testing.T) {
		clock := testkit.NewFakeClock(time.Now())
		server, _ := newProvider(t)
		server.handle("/api/oauth/device/code", jsonHandler(`{"device_code":"d","user_code":"u","verification_url":"https://portal.test/device","interval":1}`))
		access := jwt(t, map[string]any{"sub": "user-1", "scope": "openid inference:invoke"})
		server.handle("/api/oauth/token", jsonHandler(`{"access_token":"`+access+`","refresh_token":"r","expires_in":60}`))
		flow := oauth.NewNousFlow(oauth.WithClock(clock), oauth.WithEndpoints(oauth.Endpoints{
			DeviceURL: server.url("/api/oauth/device/code"), TokenURL: server.url("/api/oauth/token"),
		}))
		var credential *oauth.OAuthCredential
		runWithClock(t, clock, 2*time.Second, func() error {
			created, err := flow.Login(context.Background(), oauth.LoginOpts{Timeout: time.Minute})
			credential = created
			return err
		})
		if credential.AccountID != "user-1" {
			t.Fatalf("credential = %+v, want the subject identity", credential)
		}
	})
}

func TestGitHubCopilotFlow(t *testing.T) {
	t.Run("device grant mints a copilot token", func(t *testing.T) {
		clock := testkit.NewFakeClock(time.Now())
		server, recorder := newProvider(t)
		server.handle("/login/device/code", jsonHandler(`{"device_code":"device-1","user_code":"GH-1","verification_uri":"https://github.com/login/device","interval":1}`))
		server.handle("/login/oauth/access_token", jsonHandler(`{"access_token":"gho_github","token_type":"bearer"}`))
		server.handle("/copilot_internal/v2/token", jsonHandler(`{"token":"copilot-token","expires_at":`+futureUnix()+`,"endpoints":{"api":"https://api.githubcopilot.com"}}`))
		server.handle("/user", jsonHandler(`{"login":"octocat","email":"Octo@Example.test"}`))
		flow := oauth.NewGitHubCopilotFlow(oauth.WithClock(clock), oauth.WithEndpoints(oauth.Endpoints{
			DeviceURL: server.url("/login/device/code"), TokenURL: server.url("/login/oauth/access_token"),
			VerificationURL: server.url("/device"),
			MintURL:         server.url("/copilot_internal/v2/token"), UserURL: server.url("/user"),
		}))
		var credential *oauth.OAuthCredential
		runWithClock(t, clock, 2*time.Second, func() error {
			created, err := flow.Login(context.Background(), oauth.LoginOpts{Timeout: time.Minute})
			credential = created
			return err
		})
		if credential.AccessToken != "copilot-token" {
			t.Fatalf("credential = %+v, want the minted copilot token", credential)
		}
		if credential.RefreshToken != "gho_github" {
			t.Fatalf("credential = %+v, want the GitHub token kept as the grant", credential)
		}
		if credential.Extra["apiBaseUrl"] != "https://api.githubcopilot.com" {
			t.Fatalf("extra = %v, want the api base", credential.Extra)
		}
		if credential.AccountID != "octocat" || credential.Email != "octo@example.test" {
			t.Fatalf("credential = %+v, want the GitHub identity", credential)
		}
		if recorder.last("/copilot_internal/v2/token").authorization != "token gho_github" {
			t.Fatalf("authorization = %q, want the GitHub token", recorder.last("/copilot_internal/v2/token").authorization)
		}
		if !strings.Contains(recorder.last("/login/device/code").body, "read%3Auser") {
			t.Fatalf("start body = %q, want the read:user scope", recorder.last("/login/device/code").body)
		}
	})

	t.Run("refresh re-mints the copilot token", func(t *testing.T) {
		server, _ := newProvider(t)
		server.handle("/copilot_internal/v2/token", jsonHandler(`{"token":"copilot-refreshed","expires_at":`+futureUnix()+`}`))
		server.handle("/user", jsonHandler(`{"login":"octocat"}`))
		flow := oauth.NewGitHubCopilotFlow(oauth.WithEndpoints(oauth.Endpoints{
			APIBaseURL: server.url(""), MintURL: server.url("/copilot_internal/v2/token"), UserURL: server.url("/user"),
		}))
		refreshed, err := flow.Refresh(context.Background(), &oauth.OAuthCredential{RefreshToken: "gho_github"})
		if err != nil {
			t.Fatalf("Refresh() error = %v", err)
		}
		if refreshed.AccessToken != "copilot-refreshed" {
			t.Fatalf("credential = %+v, want the re-minted token", refreshed)
		}
		if _, err := flow.Refresh(context.Background(), &oauth.OAuthCredential{}); !errors.Is(err, oauth.ErrNoRefreshToken) {
			t.Fatalf("Refresh() error = %v, want %v", err, oauth.ErrNoRefreshToken)
		}
	})

	t.Run("a refused github grant must log in again", func(t *testing.T) {
		server, _ := newProvider(t)
		server.handle("/copilot_internal/v2/token", jsonHandler(`{"message":"Bad credentials"}`, http.StatusUnauthorized))
		flow := oauth.NewGitHubCopilotFlow(oauth.WithEndpoints(oauth.Endpoints{
			APIBaseURL: server.url(""), MintURL: server.url("/copilot_internal/v2/token"),
		}))
		_, err := flow.Refresh(context.Background(), &oauth.OAuthCredential{RefreshToken: "gho_revoked"})
		if !errors.Is(err, oauth.ErrRefreshRejected) {
			t.Fatalf("Refresh() error = %v, want %v", err, oauth.ErrRefreshRejected)
		}
	})

	// GitHub answers its device poll with a 200 and an error member, so the
	// pending answer has to be read as a refusal rather than as a token.
	t.Run("a pending answer on a 200 keeps the poll waiting", func(t *testing.T) {
		clock := testkit.NewFakeClock(time.Now())
		server, recorder := newProvider(t)
		server.handle("/login/device/code", jsonHandler(`{"device_code":"device-1","user_code":"GH-1","verification_uri":"https://github.com/login/device","interval":1}`))
		server.handle("/login/oauth/access_token", sequenceHandler(
			jsonHandler(`{"error":"authorization_pending","error_description":"The authorization request is still pending."}`),
			jsonHandler(`{"access_token":"gho_github","token_type":"bearer"}`),
		))
		server.handle("/copilot_internal/v2/token", jsonHandler(`{"token":"copilot-token","expires_at":`+futureUnix()+`}`))
		server.handle("/user", jsonHandler(`{"login":"octocat"}`))
		flow := oauth.NewGitHubCopilotFlow(oauth.WithClock(clock), oauth.WithEndpoints(oauth.Endpoints{
			DeviceURL: server.url("/login/device/code"), TokenURL: server.url("/login/oauth/access_token"),
			MintURL: server.url("/copilot_internal/v2/token"), UserURL: server.url("/user"),
		}))
		var credential *oauth.OAuthCredential
		runWithClock(t, clock, 2*time.Second, func() error {
			created, err := flow.Login(context.Background(), oauth.LoginOpts{Timeout: time.Minute})
			credential = created
			return err
		})
		if credential == nil || credential.AccessToken != "copilot-token" {
			t.Fatalf("credential = %+v, want the token the poll minted", credential)
		}
		if recorder.count("/login/oauth/access_token") != 2 {
			t.Fatalf("polls = %d, want one pending answer and one token", recorder.count("/login/oauth/access_token"))
		}
	})

	t.Run("a refusal on a 200 fails the login", func(t *testing.T) {
		server, _ := newProvider(t)
		server.handle("/login/device/code", jsonHandler(`{"device_code":"device-1","user_code":"GH-1","verification_uri":"https://github.com/login/device"}`))
		server.handle("/login/oauth/access_token", jsonHandler(`{"error":"access_denied","error_description":"The user denied the request."}`))
		flow := oauth.NewGitHubCopilotFlow(oauth.WithEndpoints(oauth.Endpoints{
			DeviceURL: server.url("/login/device/code"), TokenURL: server.url("/login/oauth/access_token"),
		}))
		_, err := flow.Login(context.Background(), oauth.LoginOpts{Timeout: time.Second})
		if !errors.Is(err, oauth.ErrDeviceDenied) {
			t.Fatalf("Login() error = %v, want %v", err, oauth.ErrDeviceDenied)
		}
	})
}

func TestMetaMuseFlow(t *testing.T) {
	pointer := `{"providers":{"meta":{"storage":"keychain"}}}`

	t.Run("warns before reading the keychain", func(t *testing.T) {
		var warnings []string
		keychain := &fakeKeychain{value: `{"api_key":"LLM|muse-key"}`}
		path := writeFile(t, "muse.json", pointer)
		flow := oauth.NewMetaMuseFlow(
			oauth.WithCredentialsPath(path),
			oauth.WithKeychain(keychain),
			oauth.WithWarn(func(message string) { warnings = append(warnings, message) }),
		)
		_, credential := loginManual(t, flow, "")
		if len(warnings) != 1 || !strings.Contains(warnings[0], "terms of service") {
			t.Fatalf("warnings = %v, want the terms warning first", warnings)
		}
		if keychain.service != "ai.meta.dev.credentials" || keychain.account != "meta" {
			t.Fatalf("keychain lookup = %s/%s, want the Meta service", keychain.service, keychain.account)
		}
		if credential.AccessToken != "LLM|muse-key" {
			t.Fatalf("credential = %+v, want the stored key", credential)
		}
	})

	t.Run("a locked keychain falls back to the device login", func(t *testing.T) {
		clock := testkit.NewFakeClock(time.Now())
		server, _ := newProvider(t)
		server.handle("/device/authorization", jsonHandler(`{"device_code":"d","user_code":"MUSE-1","verification_uri":"https://auth.meta.test/device","interval":1}`))
		server.handle("/device/token", jsonHandler(`{"access_token":"meta-account-token","expires_in":3600}`))
		server.handle("/muse-code/key", jsonHandler(`{"key":"LLM|minted","user_id":"user-1","tier_name":"pro"}`))
		flow := oauth.NewMetaMuseFlow(
			oauth.WithClock(clock),
			oauth.WithCredentialsPath(writeFile(t, "muse.json", pointer)),
			oauth.WithEndpoints(oauth.Endpoints{
				DeviceURL: server.url("/device/authorization"), DeviceTokenURL: server.url("/device/token"),
				APIBaseURL: server.url("/muse-code/key"),
			}),
		)
		var credential *oauth.OAuthCredential
		runWithClock(t, clock, 2*time.Second, func() error {
			created, err := flow.Login(context.Background(), oauth.LoginOpts{Timeout: time.Minute})
			credential = created
			return err
		})
		if credential.AccessToken != "LLM|minted" || credential.RefreshToken != "meta-account-token" {
			t.Fatalf("credential = %+v, want the minted model key", credential)
		}
		if credential.Extra["museTierName"] != "pro" {
			t.Fatalf("extra = %v, want the tier", credential.Extra)
		}
	})

	t.Run("refresh re-mints from the device token", func(t *testing.T) {
		server, _ := newProvider(t)
		server.handle("/muse-code/key", jsonHandler(`{"key":"LLM|minted"}`))
		flow := oauth.NewMetaMuseFlow(oauth.WithEndpoints(oauth.Endpoints{APIBaseURL: server.url("/muse-code/key")}))
		credential := &oauth.OAuthCredential{
			AccessToken: "LLM|old", RefreshToken: "account-token",
			Extra: map[string]string{"museOAuthAccessToken": "account-token"},
		}
		refreshed, err := flow.Refresh(context.Background(), credential)
		if err != nil {
			t.Fatalf("Refresh() error = %v", err)
		}
		if refreshed.AccessToken != "LLM|minted" {
			t.Fatalf("credential = %+v, want the re-minted key", refreshed)
		}
	})

	t.Run("refresh re-reads an imported pointer", func(t *testing.T) {
		path := writeFile(t, "muse.json", pointer)
		flow := oauth.NewMetaMuseFlow(
			oauth.WithCredentialsPath(path),
			oauth.WithKeychain(&fakeKeychain{value: "LLM|from-keychain"}),
		)
		refreshed, err := flow.Refresh(context.Background(), &oauth.OAuthCredential{AccessToken: "LLM|old", RefreshToken: "LLM|old"})
		if err != nil {
			t.Fatalf("Refresh() error = %v", err)
		}
		if refreshed.AccessToken != "LLM|from-keychain" {
			t.Fatalf("credential = %+v, want the keychain key", refreshed)
		}
	})

	t.Run("a missing pointer reports the CLI login", func(t *testing.T) {
		server, _ := newProvider(t)
		server.handle("/device/authorization", jsonHandler(`{"error":"invalid_client"}`, http.StatusBadRequest))
		flow := oauth.NewMetaMuseFlow(
			oauth.WithCredentialsPath(t.TempDir()+"/absent.json"),
			oauth.WithKeychain(&fakeKeychain{value: "LLM|key"}),
			oauth.WithEndpoints(oauth.Endpoints{DeviceURL: server.url("/device/authorization")}),
		)
		_, err := flow.Login(context.Background(), oauth.LoginOpts{
			ManualCode: func(oauth.AuthPrompt) (string, error) { return "", errors.New("operator stopped") },
			Timeout:    time.Second,
		})
		if err == nil {
			t.Fatal("Login() error = nil, want the device attempt to fail")
		}
		if errors.Is(err, oauth.ErrMusePointerMissing) {
			t.Fatalf("Login() error = %v, want the device fallback", err)
		}
	})

	t.Run("a keychain failure is reported", func(t *testing.T) {
		server, _ := newProvider(t)
		server.handle("/device/authorization", jsonHandler(`{"error":"invalid_client"}`, http.StatusBadRequest))
		flow := oauth.NewMetaMuseFlow(
			oauth.WithCredentialsPath(writeFile(t, "muse.json", pointer)),
			oauth.WithKeychain(&fakeKeychain{err: errors.New("keychain is locked")}),
			oauth.WithEndpoints(oauth.Endpoints{DeviceURL: server.url("/device/authorization")}),
		)
		_, err := flow.Login(context.Background(), oauth.LoginOpts{
			ManualCode: func(oauth.AuthPrompt) (string, error) { return "", errors.New("operator stopped") },
			Timeout:    time.Second,
		})
		if err == nil || !strings.Contains(err.Error(), "keychain is locked") {
			t.Fatalf("Login() error = %v, want the keychain failure", err)
		}
	})

	t.Run("a pointer without a key is refused", func(t *testing.T) {
		server, _ := newProvider(t)
		server.handle("/device/authorization", jsonHandler(`{"error":"invalid_client"}`, http.StatusBadRequest))
		flow := oauth.NewMetaMuseFlow(
			oauth.WithCredentialsPath(writeFile(t, "muse.json", pointer)),
			oauth.WithKeychain(&fakeKeychain{value: "{ }"}),
			oauth.WithEndpoints(oauth.Endpoints{DeviceURL: server.url("/device/authorization")}),
		)
		_, err := flow.Login(context.Background(), oauth.LoginOpts{
			ManualCode: func(oauth.AuthPrompt) (string, error) { return "", errors.New("operator stopped") },
			Timeout:    time.Second,
		})
		if err == nil {
			t.Fatal("Login() error = nil, want the empty-key failure")
		}
	})

	t.Run("validate reports the credential state", func(t *testing.T) {
		flow := oauth.NewMetaMuseFlow()
		if err := flow.Validate(context.Background(), &oauth.OAuthCredential{AccessToken: "LLM|key"}); err != nil {
			t.Fatalf("Validate() error = %v", err)
		}
	})
}

func TestKiroFlow(t *testing.T) {
	session := oauth.KiroSession{
		AccessToken:  "kiro-access",
		RefreshToken: "kiro-refresh",
		ExpiresAt:    time.Now().Add(time.Hour),
		ProfileARN:   "arn:aws:codewhisperer:eu-west-1:123456789012:profile/ABCD",
		ClientID:     "client-1",
		ClientSecret: "secret-1",
		AccountID:    "kiro-user",
	}

	t.Run("imports the installed session", func(t *testing.T) {
		flow := oauth.NewKiroFlow(oauth.WithKiroSessions(&fakeKiroSessions{sessions: []oauth.KiroSession{session}}))
		_, credential := loginManual(t, flow, "")
		if credential.AccessToken != "kiro-access" || credential.RefreshToken != "kiro-refresh" {
			t.Fatalf("credential = %+v, want the imported session", credential)
		}
		if credential.Extra["region"] != "eu-west-1" {
			t.Fatalf("extra = %v, want the region inferred from the profile ARN", credential.Extra)
		}
		if credential.Extra["profileArn"] != session.ProfileARN || credential.Extra["clientId"] != "client-1" {
			t.Fatalf("extra = %v, want the account-scoped metadata", credential.Extra)
		}
	})

	t.Run("the newest session wins", func(t *testing.T) {
		stale := session
		stale.AccessToken = "stale"
		stale.ExpiresAt = time.Now().Add(-time.Hour)
		flow := oauth.NewKiroFlow(oauth.WithKiroSessions(&fakeKiroSessions{sessions: []oauth.KiroSession{stale, session}}))
		_, credential := loginManual(t, flow, "")
		if credential.AccessToken != "kiro-access" {
			t.Fatalf("credential = %+v, want the freshest session", credential)
		}
	})

	t.Run("a missing CLI reports the install hint", func(t *testing.T) {
		flow := oauth.NewKiroFlow()
		_, err := flow.Login(context.Background(), oauth.LoginOpts{Timeout: time.Second})
		if !errors.Is(err, oauth.ErrKiroCLIMissing) {
			t.Fatalf("Login() error = %v, want %v", err, oauth.ErrKiroCLIMissing)
		}
		if !strings.Contains(err.Error(), "kiro auth login") {
			t.Fatalf("error = %q, want the actionable hint", err)
		}
	})

	t.Run("an empty session store is reported", func(t *testing.T) {
		flow := oauth.NewKiroFlow(oauth.WithKiroSessions(&fakeKiroSessions{}))
		if _, err := flow.Login(context.Background(), oauth.LoginOpts{Timeout: time.Second}); !errors.Is(err, oauth.ErrKiroSessionEmpty) {
			t.Fatalf("Login() error = %v, want %v", err, oauth.ErrKiroSessionEmpty)
		}
	})

	t.Run("a reader failure is reported", func(t *testing.T) {
		flow := oauth.NewKiroFlow(oauth.WithKiroSessions(&fakeKiroSessions{err: errors.New("sqlite is locked")}))
		if _, err := flow.Login(context.Background(), oauth.LoginOpts{Timeout: time.Second}); err == nil {
			t.Fatal("Login() error = nil, want the reader failure")
		}
	})

	t.Run("refresh reports a provider failure", func(t *testing.T) {
		server, _ := newProvider(t)
		server.handle("/refreshToken", jsonHandler(`{"error":"server_error"}`, http.StatusInternalServerError))
		flow := oauth.NewKiroFlow(oauth.WithEndpoints(oauth.Endpoints{TokenURL: server.url("/refreshToken")}))
		if _, err := flow.Refresh(context.Background(), &oauth.OAuthCredential{AccessToken: "a"}); !errors.Is(err, oauth.ErrNoRefreshToken) {
			t.Fatalf("Refresh() error = %v, want %v", err, oauth.ErrNoRefreshToken)
		}
		_, err := flow.Refresh(context.Background(), &oauth.OAuthCredential{RefreshToken: "kiro-refresh"})
		if err == nil || errors.Is(err, oauth.ErrRefreshRejected) {
			t.Fatalf("Refresh() error = %v, want the transient provider failure", err)
		}
	})

	t.Run("a rejected grant restores the import snapshot", func(t *testing.T) {
		server, _ := newProvider(t)
		server.handle("/refreshToken", jsonHandler(`{"error":"invalid_grant"}`, http.StatusUnauthorized))
		flow := oauth.NewKiroFlow(
			oauth.WithKiroSessions(&fakeKiroSessions{sessions: []oauth.KiroSession{session}}),
			oauth.WithEndpoints(oauth.Endpoints{TokenURL: server.url("/refreshToken")}),
		)
		if _, err := flow.Login(context.Background(), oauth.LoginOpts{Timeout: time.Second}); err != nil {
			t.Fatalf("Login() error = %v", err)
		}
		restored, err := flow.Refresh(context.Background(), &oauth.OAuthCredential{
			AccessToken: "kiro-access", RefreshToken: "kiro-refresh", Extra: map[string]string{"region": "eu-west-1"},
		})
		if err != nil {
			t.Fatalf("Refresh() error = %v, want the snapshot to restore the credential", err)
		}
		if restored == nil || restored.AccessToken != "kiro-access" || restored.Extra["region"] != "eu-west-1" {
			t.Fatalf("credential = %+v, want the imported session", restored)
		}
	})

	t.Run("a rejected grant without a snapshot is reported", func(t *testing.T) {
		server, _ := newProvider(t)
		server.handle("/refreshToken", jsonHandler(`{"error":"invalid_grant"}`, http.StatusUnauthorized))
		flow := oauth.NewKiroFlow(oauth.WithEndpoints(oauth.Endpoints{TokenURL: server.url("/refreshToken")}))
		if _, err := flow.Refresh(context.Background(), &oauth.OAuthCredential{RefreshToken: "gone"}); !errors.Is(err, oauth.ErrRefreshRejected) {
			t.Fatalf("Refresh() error = %v, want %v", err, oauth.ErrRefreshRejected)
		}
	})

	t.Run("region inference reads the profile arn", func(t *testing.T) {
		tests := []struct {
			arn  string
			want string
		}{
			{"arn:aws:codewhisperer:us-east-1:123456789012:profile/ABCD", "us-east-1"},
			{"arn:aws:codewhisperer", ""},
			{"", ""},
		}
		for _, tt := range tests {
			if got := oauth.RegionFromProfileARN(tt.arn); got != tt.want {
				t.Fatalf("RegionFromProfileARN(%q) = %q, want %q", tt.arn, got, tt.want)
			}
		}
	})

	t.Run("validate reports the credential state", func(t *testing.T) {
		flow := oauth.NewKiroFlow()
		if err := flow.Validate(context.Background(), &oauth.OAuthCredential{AccessToken: "a"}); err != nil {
			t.Fatalf("Validate() error = %v", err)
		}
	})
}

func TestDevinFlow(t *testing.T) {
	t.Run("imports the CLI credentials", func(t *testing.T) {
		path := writeFile(t, "credentials.toml", tomlLine("windsurf_api_key", "devin-session-token$"+jwt(t, map[string]any{"sub": "devin-user"}))+tomlLine("email", "User@Example.test"))
		flow := oauth.NewDevinFlow(oauth.WithCredentialsPath(path))
		_, credential := loginManual(t, flow, "")
		if credential.AccessToken == "" || credential.AccountID != "devin-user" {
			t.Fatalf("credential = %+v, want the imported session token", credential)
		}
		if credential.Email != "user@example.test" {
			t.Fatalf("email = %q, want the imported address", credential.Email)
		}
	})

	t.Run("a credential without a token is refused", func(t *testing.T) {
		path := writeFile(t, "credentials.toml", tomlLine("email", "user@example.test"))
		flow := oauth.NewDevinFlow(oauth.WithCredentialsPath(path))
		if _, err := flow.Login(context.Background(), oauth.LoginOpts{Timeout: time.Second}); !errors.Is(err, oauth.ErrDevinTokenShape) {
			t.Fatalf("Login() error = %v, want %v", err, oauth.ErrDevinTokenShape)
		}
	})

	t.Run("a missing credential falls back to the browser", func(t *testing.T) {
		server, recorder := newProvider(t)
		server.handle("/register", jsonHandler(`{"api_key":"devin-api-key"}`))
		flow := oauth.NewDevinFlow(
			oauth.WithCallbackPort(-1),
			oauth.WithCredentialsPath(t.TempDir()+"/absent.toml"),
			oauth.WithEndpoints(oauth.Endpoints{AuthURL: server.url("/login"), APIBaseURL: server.url("/register")}),
		)
		prompt, credential := loginManual(t, flow, "sign-in-token")
		if !strings.Contains(prompt.URL, "/login") {
			t.Fatalf("authorize url = %q, want the vendor login page", prompt.URL)
		}
		if credential.AccessToken != "devin-api-key" {
			t.Fatalf("credential = %+v, want the registered API key", credential)
		}
		if recorder.count("/register") != 1 {
			t.Fatalf("register calls = %d, want one registration", recorder.count("/register"))
		}
	})

	t.Run("registration failures are reported", func(t *testing.T) {
		server, _ := newProvider(t)
		server.handle("/register", jsonHandler(`{"error":"unauthorized"}`, http.StatusUnauthorized))
		flow := oauth.NewDevinFlow(
			oauth.WithCallbackPort(-1),
			oauth.WithCredentialsPath(t.TempDir()+"/absent.toml"),
			oauth.WithEndpoints(oauth.Endpoints{AuthURL: server.url("/login"), APIBaseURL: server.url("/register")}),
		)
		if _, err := flow.Login(context.Background(), oauth.LoginOpts{
			ManualCode: func(oauth.AuthPrompt) (string, error) { return "token", nil },
			Timeout:    time.Second,
		}); err == nil {
			t.Fatal("Login() error = nil, want the registration failure")
		}
	})

	t.Run("a registration without a key is refused", func(t *testing.T) {
		server, _ := newProvider(t)
		server.handle("/register", jsonHandler(`{"user_id":"user-1"}`))
		flow := oauth.NewDevinFlow(
			oauth.WithCallbackPort(-1),
			oauth.WithCredentialsPath(t.TempDir()+"/absent.toml"),
			oauth.WithEndpoints(oauth.Endpoints{AuthURL: server.url("/login"), APIBaseURL: server.url("/register")}),
		)
		_, err := flow.Login(context.Background(), oauth.LoginOpts{
			ManualCode: func(oauth.AuthPrompt) (string, error) { return "token", nil },
			Timeout:    time.Second,
		})
		if !errors.Is(err, oauth.ErrDevinNoAPIKey) {
			t.Fatalf("Login() error = %v, want %v", err, oauth.ErrDevinNoAPIKey)
		}
	})

	t.Run("refresh re-imports the CLI credential", func(t *testing.T) {
		path := writeFile(t, "credentials.toml", tomlLine("windsurf_api_key", "devin-session-token$new"))
		flow := oauth.NewDevinFlow(oauth.WithCredentialsPath(path))
		refreshed, err := flow.Refresh(context.Background(), &oauth.OAuthCredential{AccessToken: "devin-session-token$old", Email: "user@example.test"})
		if err != nil {
			t.Fatalf("Refresh() error = %v", err)
		}
		if refreshed.AccessToken != "devin-session-token$new" {
			t.Fatalf("credential = %+v, want the re-imported token", refreshed)
		}
		if _, err := flow.Refresh(context.Background(), nil); !errors.Is(err, oauth.ErrNoRefreshToken) {
			t.Fatalf("Refresh() error = %v, want %v", err, oauth.ErrNoRefreshToken)
		}
	})
}

func TestBrowserCommand(t *testing.T) {
	directory := t.TempDir()
	t.Run("opens the platform handler with the url", func(t *testing.T) {
		marker := filepath.Join(directory, "opened.txt")
		script := "#!/bin/sh" + newline + "echo $@ > " + marker + newline
		for _, name := range []string{"open", "xdg-open"} {
			if err := os.WriteFile(filepath.Join(directory, name), []byte(script), 0o700); err != nil {
				t.Fatalf("write %s stub: %v", name, err)
			}
		}
		t.Setenv("PATH", directory+":"+os.Getenv("PATH"))
		t.Setenv("CI", "")
		t.Setenv("DISPLAY", ":0")
		if err := oauth.OpenBrowser("https://example.test/authorize"); err != nil {
			t.Fatalf("OpenBrowser() error = %v", err)
		}
		deadline := time.Now().Add(time.Second)
		for {
			content, err := os.ReadFile(marker)
			if err == nil && strings.Contains(string(content), "example.test") {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("marker = %q, want the opened url", string(content))
			}
			time.Sleep(10 * time.Millisecond)
		}
	})
}

func futureUnix() string {
	return stringOf(time.Now().Add(time.Hour).Unix())
}

func stringOf(value int64) string {
	return strconv.FormatInt(value, 10)
}

type fakeKeychain struct {
	service string
	account string
	value   string
	err     error
}

func (k *fakeKeychain) Get(service, account string) (string, error) {
	k.service = service
	k.account = account
	if k.err != nil {
		return "", k.err
	}
	return k.value, nil
}

type fakeKiroSessions struct {
	sessions []oauth.KiroSession
	err      error
}

func (f *fakeKiroSessions) Sessions(context.Context) ([]oauth.KiroSession, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.sessions, nil
}

const newline = string(rune(10))

func tomlLine(key, value string) string {
	return key + " = '" + value + "'" + newline
}

func TestXAIDeviceFlow(t *testing.T) {
	t.Run("the device grant signs a grok account in", func(t *testing.T) {
		clock := testkit.NewFakeClock(time.Now())
		server, recorder := newProvider(t)
		server.handle("/device/code", jsonHandler(`{"device_code":"device-1","user_code":"T288-8G8P","verification_uri":"https://accounts.x.ai/oauth2/device","verification_uri_complete":"https://accounts.x.ai/oauth2/device?user_code=T288-8G8P","expires_in":1800,"interval":5}`))
		server.handle("/token", sequenceHandler(
			jsonHandler(`{"error":"authorization_pending","error_description":"User has not yet authorized"}`),
			jsonHandler(`{"access_token":"xai-access","refresh_token":"xai-refresh","expires_in":21600,"id_token":"`+
				jwt(t, map[string]any{"sub": "user-1", "email": "grok@example.test"})+`"}`),
		))
		flow := oauth.NewXAIDeviceFlow(oauth.WithClock(clock), oauth.WithEndpoints(oauth.Endpoints{
			DeviceURL: server.url("/device/code"), TokenURL: server.url("/token"),
		}))
		var prompt oauth.AuthPrompt
		var credential *oauth.OAuthCredential
		runWithClock(t, clock, 10*time.Second, func() error {
			created, err := flow.Login(context.Background(), oauth.LoginOpts{
				Timeout: time.Minute,
				Prompt:  func(value oauth.AuthPrompt) { prompt = value },
			})
			credential = created
			return err
		})
		if credential == nil || credential.AccessToken != "xai-access" || credential.RefreshToken != "xai-refresh" {
			t.Fatalf("credential = %+v, want the exchanged tokens", credential)
		}
		if credential.AccountID != "user-1" || credential.Email != "grok@example.test" {
			t.Fatalf("credential = %+v, want the identity the sign-in token names", credential)
		}
		// The complete address carries the code, so it is what the console opens.
		if prompt.URL != "https://accounts.x.ai/oauth2/device?user_code=T288-8G8P" || prompt.DeviceCode != "T288-8G8P" {
			t.Fatalf("prompt = %+v, want the complete address and the code", prompt)
		}
		if !strings.Contains(recorder.last("/device/code").body, "grok-cli%3Aaccess") {
			t.Fatalf("start body = %q, want the grok-cli scope", recorder.last("/device/code").body)
		}
		if !strings.Contains(recorder.last("/token").body, "grant-type%3Adevice_code") {
			t.Fatalf("poll body = %q, want the device grant type", recorder.last("/token").body)
		}
		if recorder.count("/token") != 2 {
			t.Fatalf("polls = %d, want one pending answer and one token", recorder.count("/token"))
		}
	})

	t.Run("configured endpoints skip discovery", func(t *testing.T) {
		server, recorder := newProvider(t)
		server.handle("/discovery", jsonHandler(`{"device_authorization_endpoint":"https://auth.x.ai/d","token_endpoint":"https://auth.x.ai/t"}`))
		flow := oauth.NewXAIDeviceFlow(oauth.WithEndpoints(oauth.Endpoints{
			DiscoveryURL: server.url("/discovery"),
			DeviceURL:    server.url("/absent"),
			TokenURL:     server.url("/absent"),
		}))
		_, err := flow.Login(context.Background(), oauth.LoginOpts{Timeout: time.Second})
		if err == nil {
			t.Fatal("Login() error = nil, want the start call to fail against the configured address")
		}
		if recorder.count("/discovery") != 0 {
			t.Fatalf("discovery calls = %d, want none when both endpoints are configured", recorder.count("/discovery"))
		}
	})

	t.Run("refresh retries a 429 with Retry-After", func(t *testing.T) {
		clock := testkit.NewFakeClock(time.Now())
		server, recorder := newProvider(t)
		server.handle("/token", sequenceHandler(
			jsonHandlerWithHeaders(`{"error":"rate_limited"}`, []int{http.StatusTooManyRequests}, map[string]string{"Retry-After": "30"}),
			jsonHandler(`{"access_token":"xai-fresh","expires_in":60}`),
		))
		flow := oauth.NewXAIDeviceFlow(
			oauth.WithClock(clock),
			oauth.WithEndpoints(oauth.Endpoints{DeviceURL: server.url("/device/code"), TokenURL: server.url("/token")}),
		)
		var refreshed *oauth.OAuthCredential
		runWithClock(t, clock, 10*time.Second, func() error {
			credential, err := flow.Refresh(context.Background(), &oauth.OAuthCredential{RefreshToken: "r"})
			refreshed = credential
			return err
		})
		if refreshed == nil || refreshed.AccessToken != "xai-fresh" {
			t.Fatalf("credential = %+v, want the retried refresh", refreshed)
		}
		if recorder.count("/token") != 2 {
			t.Fatalf("attempts = %d, want one retry", recorder.count("/token"))
		}
	})
}
