package oauth_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/adapters/oauth"
	"github.com/jonaskahn/relo/tests/testkit"
)

func TestAnthropicFlow(t *testing.T) {
	t.Run("authorize url, exchange, and identity", func(t *testing.T) {
		server, recorder := newProvider(t)
		server.handle("/token", jsonHandler(`{"access_token":"anthropic-access","refresh_token":"anthropic-refresh","expires_in":3600,"account":{"uuid":"uuid-1","email_address":"User@Example.test"}}`))
		flow := oauth.NewAnthropicFlow(oauth.WithCallbackPort(-1), oauth.WithEndpoints(oauth.Endpoints{
			AuthURL: server.url("/authorize"), TokenURL: server.url("/token"),
		}))
		prompt, credential := loginManual(t, flow, "code-1")
		want := map[string]string{
			"client_id":             "9d1c250a-e61b-44d9-88ed-5944d1962f5e",
			"response_type":         "code",
			"code":                  "true",
			"code_challenge_method": "S256",
			"scope":                 "org:create_api_key user:profile user:inference",
		}
		assertQuery(t, prompt.URL, want, []string{"code_challenge", "state", "redirect_uri"})
		parsed, err := url.Parse(prompt.URL)
		if err != nil {
			t.Fatalf("parse authorize url: %v", err)
		}
		redirect := parsed.Query().Get("redirect_uri")
		if !strings.HasPrefix(redirect, "http://localhost:") || !strings.HasSuffix(redirect, "/callback") {
			t.Fatalf("redirect_uri = %q, want the registered localhost callback", redirect)
		}
		if !strings.Contains(prompt.Instructions, "browser") {
			t.Fatalf("instructions = %q, want the browser hint", prompt.Instructions)
		}
		exchange := recorder.last("/token")
		if !strings.HasPrefix(exchange.contentType, "application/json") {
			t.Fatalf("content type = %q, want json", exchange.contentType)
		}
		assertJSONFields(t, exchange.body, map[string]string{
			"grant_type": "authorization_code", "client_id": "9d1c250a-e61b-44d9-88ed-5944d1962f5e", "code": "code-1", "redirect_uri": redirect,
		})
		if credential.AccountID != "uuid-1" || credential.Email != "user@example.test" {
			t.Fatalf("credential = %+v, want the account block identity", credential)
		}
		if credential.Scope != "org:create_api_key user:profile user:inference" {
			t.Fatalf("scope = %q, want the requested scopes", credential.Scope)
		}
		assertNoLeak(t, prompt.URL, "anthropic-access", "anthropic-refresh", "code-1")
	})

	t.Run("the code fragment carries the state", func(t *testing.T) {
		server, recorder := newProvider(t)
		server.handle("/token", jsonHandler(`{"access_token":"a","refresh_token":"r"}`))
		flow := oauth.NewAnthropicFlow(oauth.WithCallbackPort(-1), oauth.WithEndpoints(oauth.Endpoints{
			AuthURL: server.url("/authorize"), TokenURL: server.url("/token"),
		}))
		_, err := flow.Login(context.Background(), oauth.LoginOpts{
			ManualCode: func(oauth.AuthPrompt) (string, error) { return "code-2#state-2", nil },
			Timeout:    time.Second,
		})
		if err != nil {
			t.Fatalf("Login() error = %v", err)
		}
		assertJSONFields(t, recorder.last("/token").body, map[string]string{"code": "code-2", "state": "state-2"})
	})

	t.Run("refresh keeps the identity and rejects invalid_grant", func(t *testing.T) {
		server, recorder := newProvider(t)
		server.handle("/token", jsonHandler(`{"access_token":"fresh","refresh_token":"rotated","expires_in":60}`))
		flow := oauth.NewAnthropicFlow(oauth.WithEndpoints(oauth.Endpoints{AuthURL: server.url("/authorize"), TokenURL: server.url("/token")}))
		refreshed, err := flow.Refresh(context.Background(), &oauth.OAuthCredential{
			AccessToken: "old", RefreshToken: "refresh-1", Email: "user@example.test", Extra: map[string]string{"plan": "max"},
		})
		if err != nil {
			t.Fatalf("Refresh() error = %v", err)
		}
		if refreshed.AccessToken != "fresh" || refreshed.RefreshToken != "rotated" {
			t.Fatalf("credential = %+v, want the rotated tokens", refreshed)
		}
		if refreshed.Email != "user@example.test" || refreshed.Extra["plan"] != "max" {
			t.Fatalf("credential = %+v, want the previous identity kept", refreshed)
		}
		assertJSONFields(t, recorder.last("/token").body, map[string]string{"grant_type": "refresh_token", "refresh_token": "refresh-1"})

		server.handle("/token", jsonHandler(`{"error":"invalid_grant"}`, http.StatusBadRequest))
		if _, err := flow.Refresh(context.Background(), &oauth.OAuthCredential{RefreshToken: "revoked"}); !errors.Is(err, oauth.ErrRefreshRejected) {
			t.Fatalf("Refresh() error = %v, want %v", err, oauth.ErrRefreshRejected)
		}
	})

	t.Run("validate reports the credential state", func(t *testing.T) {
		flow := oauth.NewAnthropicFlow()
		if err := flow.Validate(context.Background(), nil); !errors.Is(err, oauth.ErrTokenResponse) {
			t.Fatalf("Validate() error = %v, want %v", err, oauth.ErrTokenResponse)
		}
		if err := flow.Validate(context.Background(), &oauth.OAuthCredential{AccessToken: "a", ExpiresAt: time.Now().Add(-time.Minute)}); !errors.Is(err, oauth.ErrCredentialExpired) {
			t.Fatalf("Validate() error = %v, want %v", err, oauth.ErrCredentialExpired)
		}
		if err := flow.Validate(context.Background(), &oauth.OAuthCredential{AccessToken: "a"}); err != nil {
			t.Fatalf("Validate() error = %v, want a credential without an expiry to pass", err)
		}
		live := &oauth.OAuthCredential{AccessToken: "a", ExpiresAt: time.Now().Add(time.Hour)}
		if err := flow.Validate(context.Background(), live); err != nil {
			t.Fatalf("Validate() error = %v, want a live credential to pass", err)
		}
	})
}

func TestChatGPTFlow(t *testing.T) {
	t.Run("authorize url asks for the codex scopes", func(t *testing.T) {
		server, _ := newProvider(t)
		access := jwt(t, map[string]any{
			"https://api.openai.com/auth":    map[string]any{"chatgpt_account_id": "acc-1"},
			"https://api.openai.com/profile": map[string]any{"email": "user@example.test"},
		})
		server.handle("/token", jsonHandler(`{"access_token":"`+access+`","refresh_token":"codex-refresh","expires_in":3600}`))
		flow := oauth.NewChatGPTFlow(oauth.WithCallbackPort(-1), oauth.WithEndpoints(oauth.Endpoints{
			AuthURL: server.url("/authorize"), TokenURL: server.url("/token"),
		}))
		prompt, credential := loginManual(t, flow, "code-3")
		assertQuery(t, prompt.URL, map[string]string{
			"client_id":                  "app_EMoamEEZ73f0CkXaXp7hrann",
			"scope":                      "openid profile email offline_access api.connectors.read api.connectors.invoke",
			"codex_cli_simplified_flow":  "true",
			"id_token_add_organizations": "true",
			"originator":                 "omp",
		}, nil)
		if !strings.Contains(prompt.URL, "%2Fauth%2Fcallback") {
			t.Fatalf("authorize url = %q, want the ChatGPT callback path", prompt.URL)
		}
		if credential.RefreshToken != "codex-refresh" {
			t.Fatalf("credential = %+v, want the refresh token", credential)
		}
	})

	t.Run("account id is read from the access token claims", func(t *testing.T) {
		access := jwt(t, map[string]any{
			"https://api.openai.com/auth": map[string]any{
				"chatgpt_account_id": "acc-1",
				"chatgpt_plan_type":  "pro",
			},
			"https://api.openai.com/profile": map[string]any{"email": "User@Example.test"},
		})
		server, _ := newProvider(t)
		server.handle("/token", jsonHandler(`{"access_token":"`+access+`","refresh_token":"r","expires_in":60}`))
		flow := oauth.NewChatGPTFlow(oauth.WithCallbackPort(-1), oauth.WithEndpoints(oauth.Endpoints{
			AuthURL: server.url("/authorize"), TokenURL: server.url("/token"),
		}))
		_, credential := loginManual(t, flow, "code-4")
		if credential.AccountID != "acc-1" || credential.Email != "user@example.test" {
			t.Fatalf("credential = %+v, want the claims identity", credential)
		}
		if credential.Extra[oauth.ExtraChatGPTPlanType] != "pro" {
			t.Fatalf("plan = %q, want pro", credential.Extra[oauth.ExtraChatGPTPlanType])
		}
	})

	t.Run("an access token without an email still logs in", func(t *testing.T) {
		access := jwt(t, map[string]any{
			"https://api.openai.com/auth": map[string]any{"chatgpt_account_id": "acc-9"},
		})
		server, _ := newProvider(t)
		server.handle("/token", jsonHandler(`{"access_token":"`+access+`","refresh_token":"r","expires_in":60}`))
		flow := oauth.NewChatGPTFlow(oauth.WithCallbackPort(-1), oauth.WithEndpoints(oauth.Endpoints{
			AuthURL: server.url("/authorize"), TokenURL: server.url("/token"),
		}))
		_, credential := loginManual(t, flow, "code-6")
		if credential.AccountID != "acc-9" {
			t.Fatalf("credential = %+v, want the workspace identity", credential)
		}
	})

	t.Run("missing workspace identity rejects the login", func(t *testing.T) {
		server, _ := newProvider(t)
		server.handle("/token", jsonHandler(`{"access_token":"opaque","refresh_token":"r","expires_in":60}`))
		flow := oauth.NewChatGPTFlow(oauth.WithCallbackPort(-1), oauth.WithEndpoints(oauth.Endpoints{
			AuthURL: server.url("/authorize"), TokenURL: server.url("/token"),
		}))
		_, err := flow.Login(context.Background(), oauth.LoginOpts{
			NoBrowser: true,
			ManualCode: func(oauth.AuthPrompt) (string, error) {
				return "code-5", nil
			},
		})
		if !errors.Is(err, oauth.ErrTokenResponse) {
			t.Fatalf("Login() error = %v, want %v", err, oauth.ErrTokenResponse)
		}
	})

	t.Run("refresh keeps the stored workspace identity", func(t *testing.T) {
		refreshed := jwt(t, map[string]any{
			"https://api.openai.com/auth": map[string]any{
				"chatgpt_account_id": "new-account",
				"chatgpt_plan_type":  "free",
			},
			"https://api.openai.com/profile": map[string]any{"email": "new@example.test"},
		})
		server, _ := newProvider(t)
		server.handle("/token", jsonHandler(`{"access_token":"`+refreshed+`","refresh_token":"new-r","expires_in":60}`))
		flow := oauth.NewChatGPTFlow(oauth.WithEndpoints(oauth.Endpoints{TokenURL: server.url("/token")}))
		credential, err := flow.Refresh(context.Background(), &oauth.OAuthCredential{
			RefreshToken: "old-r",
			AccountID:    "stored-account",
			Email:        "stored@example.test",
			Extra:        map[string]string{oauth.ExtraChatGPTPlanType: "pro"},
		})
		if err != nil {
			t.Fatalf("Refresh() error = %v", err)
		}
		if credential.AccountID != "stored-account" || credential.Email != "stored@example.test" ||
			credential.Extra[oauth.ExtraChatGPTPlanType] != "pro" {
			t.Fatalf("credential = %+v, want the stored workspace identity", credential)
		}
	})

	t.Run("refresh classifies grant death from transport failures", func(t *testing.T) {
		server, _ := newProvider(t)
		flow := oauth.NewChatGPTFlow(oauth.WithEndpoints(oauth.Endpoints{TokenURL: server.url("/token")}))
		cases := []struct {
			name     string
			body     string
			rejected bool
		}{
			{"invalid_grant", `{"error":"invalid_grant"}`, true},
			{"invalid_token", `{"error":"invalid_token"}`, true},
			{"unauthorized_client", `{"error":"unauthorized_client"}`, true},
			{"revoked object", `{"error":{"code":"revoked","message":"the grant is gone"}}`, true},
			{"expired refresh token", `{"error":"invalid_grant","error_description":"refresh token expired"}`, true},
			{"invalid request stays transient", `{"error":"invalid_request"}`, false},
			{"expired access token stays transient", `{"error":{"message":"Could not validate your token.","type":"invalid_request_error","code":"token_expired"}}`, false},
		}
		for _, tt := range cases {
			t.Run(tt.name, func(t *testing.T) {
				server.handle("/token", jsonHandler(tt.body, http.StatusBadRequest))
				_, err := flow.Refresh(context.Background(), &oauth.OAuthCredential{RefreshToken: "r"})
				if err == nil {
					t.Fatal("Refresh() error = nil, want the refusal")
				}
				if got := errors.Is(err, oauth.ErrRefreshRejected); got != tt.rejected {
					t.Fatalf("Refresh() rejected = %v, want %v (%v)", got, tt.rejected, err)
				}
			})
		}
	})
}

// TestTokenRefusalReportsTheVendorReason covers the shape OpenAI refuses a
// token request with: the error member is an object carrying the machine
// readable code next to a message, and the login reports both instead of a
// bare status an operator cannot act on.
func TestTokenRefusalReportsTheVendorReason(t *testing.T) {
	server, _ := newProvider(t)
	server.handle("/token", jsonHandler(
		`{"error":{"message":"Could not validate your token. Please try signing in again.",`+
			`"type":"invalid_request_error","param":null,"code":"token_expired"}}`,
		http.StatusUnauthorized))
	flow := oauth.NewChatGPTFlow(oauth.WithCallbackPort(-1), oauth.WithEndpoints(oauth.Endpoints{
		AuthURL: server.url("/authorize"), TokenURL: server.url("/token"),
	}))

	_, err := flow.Login(context.Background(), oauth.LoginOpts{
		NoBrowser: true,
		Prompt:    func(oauth.AuthPrompt) {},
		ManualCode: func(oauth.AuthPrompt) (string, error) {
			return "code-1", nil
		},
		Timeout: 5 * time.Second,
	})
	if err == nil {
		t.Fatal("Login() error = nil, want the refusal")
	}
	for _, want := range []string{"401", "token_expired", "Could not validate your token"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %q, want it to name %q", err, want)
		}
	}
}

func TestGoogleAntigravityFlow(t *testing.T) {
	t.Run("login discovers an existing project", func(t *testing.T) {
		server, recorder := newProvider(t)
		server.handle("/token", jsonHandler(`{"access_token":"google-access","refresh_token":"google-refresh","expires_in":3600,"id_token":"`+jwt(t, map[string]any{"email": "User@Example.test"})+`"}`))
		server.handle("/v1internal:loadCodeAssist", jsonHandler(`{"cloudaicompanionProject":"project-1"}`))
		server.handle("/v1internal:onboardUser", jsonHandler(`{"done":true,"response":{"cloudaicompanionProject":"project-2"}}`))
		flow := oauth.NewGoogleAntigravityFlow(oauth.WithCallbackPort(-1), oauth.WithEndpoints(oauth.Endpoints{
			AuthURL: server.url("/authorize"), TokenURL: server.url("/token"), APIBaseURL: server.url(""),
		}))
		prompt, credential := loginManual(t, flow, "code-5")
		assertQuery(t, prompt.URL, map[string]string{
			"access_type": "offline", "prompt": "consent",
			"scope": "https://www.googleapis.com/auth/cloud-platform https://www.googleapis.com/auth/userinfo.email " +
				"https://www.googleapis.com/auth/userinfo.profile https://www.googleapis.com/auth/cclog " +
				"https://www.googleapis.com/auth/experimentsandconfigs",
		}, nil)
		if credential.Extra["projectId"] != "project-1" {
			t.Fatalf("credential = %+v, want the discovered project", credential)
		}
		if credential.Email != "user@example.test" {
			t.Fatalf("email = %q, want the id_token claim", credential.Email)
		}
		if recorder.count("/v1internal:onboardUser") != 0 {
			t.Fatal("onboarding ran even though the account already had a project")
		}
		tokenRequest := recorder.last("/v1internal:loadCodeAssist")
		if tokenRequest.authorization != "Bearer google-access" {
			t.Fatalf("authorization = %q, want the access token", tokenRequest.authorization)
		}
	})

	t.Run("onboarding polls until the project appears", func(t *testing.T) {
		clock := testkit.NewFakeClock(time.Now())
		server, recorder := newProvider(t)
		server.handle("/token", jsonHandler(`{"access_token":"google-access","refresh_token":"google-refresh","expires_in":3600}`))
		server.handle("/v1internal:loadCodeAssist", jsonHandler(`{"error":{"code":404}}`, http.StatusNotFound))
		server.handle("/v1internal:onboardUser", jsonHandler(`{"name":"operations/onboard-9","done":false}`))
		server.handle("/v1internal/operations/onboard-9", sequenceHandler(
			jsonHandler(`{"done":false}`),
			jsonHandler(`{"done":true,"response":{"project":{"id":"project-3"}}}`),
		))
		flow := oauth.NewGoogleAntigravityFlow(
			oauth.WithClock(clock),
			oauth.WithCallbackPort(-1),
			oauth.WithEndpoints(oauth.Endpoints{
				AuthURL: server.url("/authorize"), TokenURL: server.url("/token"),
				APIBaseURL: server.url(""), DailyAPIBaseURL: server.url(""),
			}),
		)
		var credential *oauth.OAuthCredential
		runWithClock(t, clock, time.Second, func() error {
			_, refreshed := loginManual(t, flow, "code-6")
			credential = refreshed
			return nil
		})
		if credential.Extra["projectId"] != "project-3" {
			t.Fatalf("credential = %+v, want the onboarded project", credential)
		}
		if recorder.count("/v1internal:onboardUser") != 1 {
			t.Fatalf("onboard starts = %d, want one start and then polling the operation", recorder.count("/v1internal:onboardUser"))
		}
		if recorder.count("/v1internal/operations/onboard-9") < 2 {
			t.Fatalf("operation polls = %d, want the poll to continue", recorder.count("/v1internal/operations/onboard-9"))
		}
	})

	t.Run("onboarding that never finishes is a typed error", func(t *testing.T) {
		clock := testkit.NewFakeClock(time.Now())
		server, _ := newProvider(t)
		server.handle("/token", jsonHandler(`{"access_token":"a","refresh_token":"r","expires_in":60}`))
		server.handle("/v1internal:loadCodeAssist", jsonHandler(`{}`))
		server.handle("/v1internal:onboardUser", jsonHandler(`{"name":"operations/stuck","done":false}`))
		server.handle("/v1internal/operations/stuck", jsonHandler(`{"done":false}`))
		flow := oauth.NewGoogleAntigravityFlow(
			oauth.WithClock(clock),
			oauth.WithCallbackPort(-1),
			oauth.WithEndpoints(oauth.Endpoints{
				AuthURL: server.url("/authorize"), TokenURL: server.url("/token"),
				APIBaseURL: server.url(""), DailyAPIBaseURL: server.url(""),
			}),
		)
		var loginErr error
		runWithClock(t, clock, time.Second, func() error {
			_, err := flow.Login(context.Background(), oauth.LoginOpts{
				ManualCode: func(oauth.AuthPrompt) (string, error) { return "code-7", nil },
				Timeout:    time.Minute,
			})
			loginErr = err
			return nil
		})
		if !errors.Is(loginErr, oauth.ErrOnboardingFailed) {
			t.Fatalf("Login() error = %v, want %v", loginErr, oauth.ErrOnboardingFailed)
		}
	})

	t.Run("refresh preserves the project id", func(t *testing.T) {
		server, _ := newProvider(t)
		server.handle("/token", jsonHandler(`{"access_token":"fresh","expires_in":60}`))
		flow := oauth.NewGoogleAntigravityFlow(oauth.WithEndpoints(oauth.Endpoints{AuthURL: server.url("/authorize"), TokenURL: server.url("/token")}))
		refreshed, err := flow.Refresh(context.Background(), &oauth.OAuthCredential{
			RefreshToken: "r", Extra: map[string]string{"projectId": "project-1"},
		})
		if err != nil {
			t.Fatalf("Refresh() error = %v", err)
		}
		if refreshed.Extra["projectId"] != "project-1" || refreshed.RefreshToken != "r" {
			t.Fatalf("credential = %+v, want the project and refresh token kept", refreshed)
		}
	})
}

func TestOrcaRouterFlow(t *testing.T) {
	t.Run("exchange turns the code into an api key", func(t *testing.T) {
		server, recorder := newProvider(t)
		server.handle("/keys", jsonHandler(`{"key":"orca-key","user_id":"user-1","scope":"api"}`))
		flow := oauth.NewOrcaRouterFlow(oauth.WithCallbackPort(-1), oauth.WithEndpoints(oauth.Endpoints{
			AuthURL: server.url("/authorize"), TokenURL: server.url("/keys"),
		}))
		prompt, credential := loginManual(t, flow, "code-9")
		assertQuery(t, prompt.URL, map[string]string{"scope": "api", "app_name": "Relo"}, nil)
		assertJSONFields(t, recorder.last("/keys").body, map[string]string{"code": "code-9", "code_challenge_method": "S256"})
		if credential.AccessToken != "orca-key" {
			t.Fatalf("credential = %+v, want the issued key", credential)
		}
		assertNoLeak(t, prompt.URL, "orca-key")
	})

	t.Run("an exchange that grants another scope is refused", func(t *testing.T) {
		server, _ := newProvider(t)
		server.handle("/keys", jsonHandler(`{"key":"orca-key","scope":"read"}`))
		flow := oauth.NewOrcaRouterFlow(oauth.WithCallbackPort(-1), oauth.WithEndpoints(oauth.Endpoints{
			AuthURL: server.url("/authorize"), TokenURL: server.url("/keys"),
		}))
		_, err := flow.Login(context.Background(), oauth.LoginOpts{
			ManualCode: func(oauth.AuthPrompt) (string, error) { return "code-10", nil },
			Timeout:    time.Second,
		})
		if !errors.Is(err, oauth.ErrOrcaRouterScope) {
			t.Fatalf("Login() error = %v, want %v", err, oauth.ErrOrcaRouterScope)
		}
	})

	t.Run("rotation keeps the quota history identity", func(t *testing.T) {
		flow := oauth.NewOrcaRouterFlow()
		refreshed, err := flow.Refresh(context.Background(), &oauth.OAuthCredential{
			AccessToken: "orca-key", RefreshToken: "orca-key", Extra: map[string]string{"quotaHistoryId": "history-1"},
		})
		if err != nil {
			t.Fatalf("Refresh() error = %v", err)
		}
		if refreshed.Extra["quotaHistoryId"] != "history-1" || refreshed.AccessToken != "orca-key" {
			t.Fatalf("credential = %+v, want the history identity to survive", refreshed)
		}
	})

	t.Run("a key exchange without a key is refused", func(t *testing.T) {
		server, _ := newProvider(t)
		server.handle("/keys", jsonHandler(`{"user_id":"user-1","scope":"api"}`))
		flow := oauth.NewOrcaRouterFlow(oauth.WithCallbackPort(-1), oauth.WithEndpoints(oauth.Endpoints{
			AuthURL: server.url("/authorize"), TokenURL: server.url("/keys"),
		}))
		_, err := flow.Login(context.Background(), oauth.LoginOpts{
			ManualCode: func(oauth.AuthPrompt) (string, error) { return "code-11", nil },
			Timeout:    time.Second,
		})
		if !errors.Is(err, oauth.ErrTokenResponse) {
			t.Fatalf("Login() error = %v, want %v", err, oauth.ErrTokenResponse)
		}
	})
}

func TestCommandCodeFlow(t *testing.T) {
	t.Run("imports the installed CLI key", func(t *testing.T) {
		server, _ := newProvider(t)
		server.handle("/alpha/whoami", jsonHandler(`{"userId":"user-9","userName":"operator","keyName":"laptop"}`))
		path := writeFile(t, "commandcode.json", `{"apiKey":"cc-key","userId":"user-9"}`)
		flow := oauth.NewCommandCodeFlow(
			oauth.WithCallbackPort(-1),
			oauth.WithCredentialsPath(path),
			oauth.WithEndpoints(oauth.Endpoints{APIBaseURL: server.url("")}),
		)
		_, credential := loginManual(t, flow, "")
		if credential.AccessToken != "cc-key" || credential.AccountID != "user-9" {
			t.Fatalf("credential = %+v, want the imported key", credential)
		}
	})

	t.Run("a rejected imported key is not adopted", func(t *testing.T) {
		server, _ := newProvider(t)
		server.handle("/alpha/whoami", jsonHandler(`{"error":"unauthorized"}`, http.StatusUnauthorized))
		path := writeFile(t, "commandcode.json", `{"apiKey":"cc-key"}`)
		flow := oauth.NewCommandCodeFlow(
			oauth.WithCallbackPort(-1),
			oauth.WithCredentialsPath(path),
			oauth.WithEndpoints(oauth.Endpoints{APIBaseURL: server.url("")}),
		)
		_, err := flow.Login(context.Background(), oauth.LoginOpts{
			ManualCode: func(oauth.AuthPrompt) (string, error) { return "c", nil },
			NoBrowser:  true,
			Timeout:    time.Second,
		})
		if !errors.Is(err, oauth.ErrCommandCodeKeyRejected) {
			t.Fatalf("Login() error = %v, want %v", err, oauth.ErrCommandCodeKeyRejected)
		}
	})

	t.Run("browser login hands the key back as the code", func(t *testing.T) {
		server, _ := newProvider(t)
		server.handle("/alpha/whoami", jsonHandler(`{"userId":"user-9","userName":"operator"}`))
		flow := oauth.NewCommandCodeFlow(
			oauth.WithCallbackPort(-1),
			oauth.WithCredentialsPath(t.TempDir()+"/absent.json"),
			oauth.WithEndpoints(oauth.Endpoints{
				AuthURL: server.url("/cli"), APIBaseURL: server.url(""),
			}),
		)
		prompt, credential := loginManual(t, flow, "cc-browser-key")
		if !strings.Contains(prompt.URL, "/cli") {
			t.Fatalf("authorize url = %q, want the studio page", prompt.URL)
		}
		if credential.AccessToken != "cc-browser-key" || credential.AccountID != "user-9" {
			t.Fatalf("credential = %+v, want the browser key", credential)
		}
	})

	t.Run("missing CLI authentication falls through to the browser", func(t *testing.T) {
		flow := oauth.NewCommandCodeFlow(oauth.WithCallbackPort(-1), oauth.WithCredentialsPath(t.TempDir()+"/absent.json"))
		_, err := flow.Login(context.Background(), oauth.LoginOpts{
			ManualCode: func(oauth.AuthPrompt) (string, error) { return "", errors.New("operator stopped") },
			Timeout:    time.Second,
		})
		if err == nil || errors.Is(err, oauth.ErrCommandCodeAuthMissing) {
			t.Fatalf("Login() error = %v, want the browser attempt to fail with the operator error", err)
		}
	})

	t.Run("refresh re-validates the key", func(t *testing.T) {
		server, _ := newProvider(t)
		server.handle("/alpha/whoami", jsonHandler(`{"userId":"user-9"}`))
		flow := oauth.NewCommandCodeFlow(oauth.WithEndpoints(oauth.Endpoints{APIBaseURL: server.url("")}))
		refreshed, err := flow.Refresh(context.Background(), &oauth.OAuthCredential{AccessToken: "cc-key"})
		if err != nil {
			t.Fatalf("Refresh() error = %v", err)
		}
		if refreshed.AccessToken != "cc-key" {
			t.Fatalf("credential = %+v, want the same key", refreshed)
		}
		if _, err := flow.Refresh(context.Background(), &oauth.OAuthCredential{}); !errors.Is(err, oauth.ErrNoRefreshToken) {
			t.Fatalf("Refresh() error = %v, want %v", err, oauth.ErrNoRefreshToken)
		}
	})

	t.Run("validate reports the credential state", func(t *testing.T) {
		flow := oauth.NewCommandCodeFlow()
		if err := flow.Validate(context.Background(), &oauth.OAuthCredential{AccessToken: "k"}); err != nil {
			t.Fatalf("Validate() error = %v, want a key without an expiry to pass", err)
		}
	})
}

type provider struct {
	server   *httptest.Server
	recorder *recorder
}

func newProvider(t *testing.T) (*provider, *recorder) {
	t.Helper()
	recorded := &recorder{}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		recorded.add(recordedRequest{
			path:          r.URL.Path,
			method:        r.Method,
			contentType:   r.Header.Get("Content-Type"),
			authorization: r.Header.Get("Authorization"),
			userAgent:     r.Header.Get("User-Agent"),
			accept:        r.Header.Get("Accept"),
			body:          string(body),
		})
		handler, found := recorded.route(r.URL.Path)
		if !found {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		handler(w, r)
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	instance := &provider{server: server, recorder: recorded}
	return instance, recorded
}

func (p *provider) handle(path string, handler http.HandlerFunc) {
	p.recorder.set(path, handler)
}

func (p *provider) url(path string) string {
	return p.server.URL + path
}

type recordedRequest struct {
	path          string
	method        string
	contentType   string
	authorization string
	userAgent     string
	accept        string
	body          string
}

type recorder struct {
	mu       sync.Mutex
	requests []recordedRequest
	handlers map[string]http.HandlerFunc
	counts   map[string]int
}

func (r *recorder) add(request recordedRequest) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.requests = append(r.requests, request)
}

func (r *recorder) set(path string, handler http.HandlerFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.handlers == nil {
		r.handlers = map[string]http.HandlerFunc{}
	}
	r.handlers[path] = handler
}

func (r *recorder) route(path string) (http.HandlerFunc, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.counts == nil {
		r.counts = map[string]int{}
	}
	r.counts[path]++
	handler, found := r.handlers[path]
	return handler, found
}

func (r *recorder) count(path string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.counts[path]
}

func (r *recorder) last(path string) recordedRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	for index := len(r.requests) - 1; index >= 0; index-- {
		if r.requests[index].path == path {
			return r.requests[index]
		}
	}
	return recordedRequest{}
}

func jsonHandler(body string, status ...int) http.HandlerFunc {
	return jsonHandlerWithHeaders(body, status, nil)
}

func jsonHandlerWithHeaders(body string, status []int, headers map[string]string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		for name, value := range headers {
			w.Header().Set(name, value)
		}
		w.Header().Set("Content-Type", "application/json")
		if len(status) > 0 {
			w.WriteHeader(status[0])
		}
		_, _ = io.WriteString(w, body)
	}
}

func sequenceHandler(handlers ...http.HandlerFunc) http.HandlerFunc {
	var mu sync.Mutex
	index := 0
	return func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		current := index
		if index < len(handlers)-1 {
			index++
		}
		mu.Unlock()
		handlers[current](w, r)
	}
}

func loginManual(t *testing.T, flow oauth.OAuthFlow, answer string) (oauth.AuthPrompt, *oauth.OAuthCredential) {
	t.Helper()
	var prompt oauth.AuthPrompt
	credential, err := flow.Login(context.Background(), oauth.LoginOpts{
		Prompt:     func(shown oauth.AuthPrompt) { prompt = shown },
		ManualCode: func(oauth.AuthPrompt) (string, error) { return answer, nil },
		NoBrowser:  true,
		Timeout:    5 * time.Second,
	})
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if credential == nil {
		t.Fatal("Login() returned no credential")
	}
	return prompt, credential
}

func assertQuery(t *testing.T, rawURL string, want map[string]string, present []string) {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse %q: %v", rawURL, err)
	}
	query := parsed.Query()
	for key, value := range want {
		if got := query.Get(key); got != value {
			t.Fatalf("query %s = %q, want %q", key, got, value)
		}
	}
	for _, key := range present {
		if query.Get(key) == "" {
			t.Fatalf("query %s is missing from %q", key, rawURL)
		}
	}
}

func assertJSONFields(t *testing.T, body string, want map[string]string) {
	t.Helper()
	fields := map[string]any{}
	if err := json.Unmarshal([]byte(body), &fields); err != nil {
		t.Fatalf("decode request body %q: %v", body, err)
	}
	for key, value := range want {
		if got, _ := fields[key].(string); got != value {
			t.Fatalf("body field %s = %v, want %q", key, fields[key], value)
		}
	}
}

func assertNoLeak(t *testing.T, value string, secrets ...string) {
	t.Helper()
	for _, secret := range secrets {
		if strings.Contains(value, secret) {
			t.Fatalf("%q leaks %q", value, secret)
		}
	}
}

func writeFile(t *testing.T, name, content string) string {
	t.Helper()
	path := t.TempDir() + "/" + name
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

func runWithClock(t *testing.T, clock *testkit.FakeClock, step time.Duration, action func() error) {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- action() }()
	deadline := time.Now().Add(10 * time.Second)
	for {
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("action failed: %v", err)
			}
			return
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("action did not finish while the clock advanced")
		}
		clock.Add(step)
		time.Sleep(2 * time.Millisecond)
	}
}
