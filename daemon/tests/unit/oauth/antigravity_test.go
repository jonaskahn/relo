package oauth_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/adapters/antigravity"
	"github.com/jonaskahn/relo/internal/adapters/oauth"
	"github.com/jonaskahn/relo/tests/testkit"
)

// antigravityFingerprint is the header Cloud Code Assist expects from the
// Antigravity IDE. It is pinned here as well as in the package, so a client
// bump that drifts away from the IDE family fails the build.
const antigravityFingerprint = "antigravity/ide/2.5.5 (os_type=windows; arch=amd64; aidev_client; auth_method=oauth)"

func TestAntigravityFingerprint(t *testing.T) {
	t.Run("the pinned user agent names the IDE client family", func(t *testing.T) {
		if got := antigravity.UserAgent(); got != antigravityFingerprint {
			t.Fatalf("UserAgent() = %q, want %q", got, antigravityFingerprint)
		}
	})

	// The redirect has to be the one the client is registered for, or Google
	// refuses the sign-in before the account is ever consulted.
	t.Run("the sign-in returns to the registered callback", func(t *testing.T) {
		server, _ := newProvider(t)
		server.handle("/token", jsonHandler(`{"access_token":"a","refresh_token":"r","expires_in":3600}`))
		server.handle("/v1internal:loadCodeAssist", jsonHandler(`{"cloudaicompanionProject":"project-1"}`))
		prompt, _ := loginManual(t, antigravityFlow(server), "code-redirect")
		parsed, err := url.Parse(prompt.URL)
		if err != nil {
			t.Fatalf("parse authorize url: %v", err)
		}
		redirect := parsed.Query().Get("redirect_uri")
		if !strings.HasPrefix(redirect, "http://127.0.0.1:") || !strings.HasSuffix(redirect, "/oauth-callback") {
			t.Fatalf("redirect_uri = %q, want the registered loopback callback", redirect)
		}
	})
}

func TestGoogleAntigravitySignIn(t *testing.T) {
	t.Run("the project lookup carries the IDE fingerprint", func(t *testing.T) {
		server, recorder := newProvider(t)
		server.handle("/token", jsonHandler(`{"access_token":"antigravity-access","refresh_token":"antigravity-refresh","expires_in":3600}`))
		server.handle("/v1internal:loadCodeAssist", jsonHandler(`{"cloudaicompanionProject":"project-1"}`))
		flow := antigravityFlow(server)
		_, credential := loginManual(t, flow, "code-1")
		if credential.Extra[oauth.ExtraProjectID] != "project-1" {
			t.Fatalf("credential = %+v, want the discovered project", credential)
		}
		lookup := recorder.last("/v1internal:loadCodeAssist")
		if lookup.userAgent != antigravityFingerprint {
			t.Fatalf("user agent = %q, want the Antigravity IDE fingerprint %q", lookup.userAgent, antigravityFingerprint)
		}
		if lookup.accept != "*/*" {
			t.Fatalf("accept = %q, want the accept the IDE sends", lookup.accept)
		}
		if lookup.authorization != "Bearer antigravity-access" {
			t.Fatalf("authorization = %q, want the access token", lookup.authorization)
		}
		if got := nestedField(t, lookup.body, "metadata", "ideType"); got != "ANTIGRAVITY" {
			t.Fatalf("metadata.ideType = %q, want ANTIGRAVITY", got)
		}
		if recorder.count("/v1internal:onboardUser") != 0 {
			t.Fatal("onboarding ran even though the account already had a project")
		}
	})

	t.Run("a refused lookup onboards with the pinned metadata", func(t *testing.T) {
		clock := testkit.NewFakeClock(time.Now())
		server, recorder := newProvider(t)
		server.handle("/token", jsonHandler(`{"access_token":"antigravity-access","refresh_token":"antigravity-refresh","expires_in":3600}`))
		server.handle("/v1internal:loadCodeAssist", jsonHandler(
			`{"error":{"code":403,"status":"PERMISSION_DENIED","message":"Caller does not have permission"}}`, http.StatusForbidden))
		server.handle("/v1internal:onboardUser", jsonHandler(`{"name":"operations/onboard-1","done":false}`))
		server.handle("/v1internal/operations/onboard-1", sequenceHandler(
			jsonHandler(`{"done":false}`),
			jsonHandler(`{"done":true,"response":{"project":{"id":"project-2"}}}`),
		))
		flow := antigravityFlow(server, oauth.WithClock(clock))
		var credential *oauth.OAuthCredential
		runWithClock(t, clock, time.Second, func() error {
			result, err := flow.Login(context.Background(), oauth.LoginOpts{
				ManualCode: func(oauth.AuthPrompt) (string, error) { return "code-2", nil },
				NoBrowser:  true, Timeout: time.Minute,
			})
			credential = result
			return err
		})
		if credential.Extra[oauth.ExtraProjectID] != "project-2" {
			t.Fatalf("credential = %+v, want the onboarded project", credential)
		}
		onboard := recorder.last("/v1internal:onboardUser")
		if onboard.userAgent != antigravityFingerprint {
			t.Fatalf("user agent = %q, want the fingerprint on onboarding too", onboard.userAgent)
		}
		if onboard.accept != "*/*" {
			t.Fatalf("accept = %q, want the accept the IDE sends", onboard.accept)
		}
		if got := nestedField(t, onboard.body, "tierId"); got != "free-tier" {
			t.Fatalf("tierId = %q, want free-tier", got)
		}
		if got := nestedField(t, onboard.body, "metadata", "ideType"); got != "ANTIGRAVITY" {
			t.Fatalf("metadata.ideType = %q, want ANTIGRAVITY", got)
		}
		if recorder.count("/v1internal:onboardUser") != 1 {
			t.Fatalf("onboard calls = %d, want one start and then polling the operation", recorder.count("/v1internal:onboardUser"))
		}
		if recorder.count("/v1internal/operations/onboard-1") == 0 {
			t.Fatal("want the named operation polled until it finished")
		}
	})

	t.Run("a refused lookup and a refused onboarding name both stages", func(t *testing.T) {
		server, _ := newProvider(t)
		server.handle("/token", jsonHandler(`{"access_token":"antigravity-access","refresh_token":"antigravity-refresh","expires_in":3600}`))
		server.handle("/v1internal:loadCodeAssist", jsonHandler(
			`{"error":{"status":"PERMISSION_DENIED","message":"Caller does not have permission"}}`, http.StatusForbidden))
		server.handle("/v1internal:onboardUser", jsonHandler(
			`{"error":{"status":"FAILED_PRECONDITION","message":"Antigravity is not available for this account"}}`, http.StatusForbidden))
		_, err := antigravityFlow(server).Login(context.Background(), oauth.LoginOpts{
			ManualCode: func(oauth.AuthPrompt) (string, error) { return "code-3", nil },
			NoBrowser:  true, Timeout: time.Minute,
		})
		if !errors.Is(err, oauth.ErrOnboardingFailed) {
			t.Fatalf("Login() error = %v, want %v", err, oauth.ErrOnboardingFailed)
		}
		for _, want := range []string{
			"onboardUser answered 403 (FAILED_PRECONDITION)",
			"loadCodeAssist answered 403 (PERMISSION_DENIED)",
			"not available for this account",
		} {
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("error = %q, want it to name %q", err, want)
			}
		}
	})

	t.Run("a token response without a refresh token fails the login", func(t *testing.T) {
		server, _ := newProvider(t)
		server.handle("/token", jsonHandler(`{"access_token":"antigravity-access","expires_in":3600}`))
		_, err := antigravityFlow(server).Login(context.Background(), oauth.LoginOpts{
			ManualCode: func(oauth.AuthPrompt) (string, error) { return "code-4", nil },
			NoBrowser:  true, Timeout: time.Minute,
		})
		if !errors.Is(err, oauth.ErrNoRefreshToken) {
			t.Fatalf("Login() error = %v, want %v", err, oauth.ErrNoRefreshToken)
		}
		if !strings.Contains(err.Error(), "exchange the authorization code") {
			t.Fatalf("error = %q, want it to name the exchange", err)
		}
	})

	t.Run("refresh sends the client secret and fills in a missing project", func(t *testing.T) {
		server, recorder := newProvider(t)
		server.handle("/token", jsonHandler(`{"access_token":"fresh-access","expires_in":3600}`))
		server.handle("/v1internal:loadCodeAssist", jsonHandler(`{"cloudaicompanionProject":"project-3"}`))
		refreshed, err := antigravityFlow(server).Refresh(context.Background(), &oauth.OAuthCredential{
			AccessToken: "stale-access", RefreshToken: "antigravity-refresh",
		})
		if err != nil {
			t.Fatalf("Refresh() error = %v", err)
		}
		if refreshed.AccessToken != "fresh-access" || refreshed.RefreshToken != "antigravity-refresh" {
			t.Fatalf("credential = %+v, want the fresh access and the kept refresh token", refreshed)
		}
		if refreshed.Extra[oauth.ExtraProjectID] != "project-3" {
			t.Fatalf("credential = %+v, want the missing project filled in", refreshed)
		}
		refresh := recorder.last("/token")
		if got := formField(t, refresh.body, "grant_type"); got != "refresh_token" {
			t.Fatalf("grant_type = %q, want refresh_token", got)
		}
		if got := formField(t, refresh.body, "refresh_token"); got != "antigravity-refresh" {
			t.Fatalf("refresh_token = %q, want the stored token", got)
		}
		if formField(t, refresh.body, "client_secret") == "" {
			t.Fatal("refresh carried no client secret, which Google's installed-app clients expect")
		}
		if recorder.count("/v1internal:loadCodeAssist") != 1 {
			t.Fatalf("lookups = %d, want one discovery for the missing project", recorder.count("/v1internal:loadCodeAssist"))
		}
	})

	t.Run("refresh keeps a project it already has", func(t *testing.T) {
		server, recorder := newProvider(t)
		server.handle("/token", jsonHandler(`{"access_token":"fresh-access","expires_in":3600}`))
		refreshed, err := antigravityFlow(server).Refresh(context.Background(), &oauth.OAuthCredential{
			RefreshToken: "antigravity-refresh", Extra: map[string]string{oauth.ExtraProjectID: "project-4"},
		})
		if err != nil {
			t.Fatalf("Refresh() error = %v", err)
		}
		if refreshed.Extra[oauth.ExtraProjectID] != "project-4" {
			t.Fatalf("credential = %+v, want the project kept", refreshed)
		}
		if recorder.count("/v1internal:loadCodeAssist") != 0 {
			t.Fatal("refresh re-discovered a project the credential already had")
		}
	})
}

// antigravityFlow points the Antigravity login at one stub provider.
func antigravityFlow(server *provider, options ...oauth.Option) oauth.OAuthFlow {
	settings := []oauth.Option{
		oauth.WithCallbackPort(-1),
		oauth.WithEndpoints(oauth.Endpoints{
			AuthURL: server.url("/authorize"), TokenURL: server.url("/token"),
			APIBaseURL: server.url(""), DailyAPIBaseURL: server.url(""),
		}),
	}
	return oauth.NewGoogleAntigravityFlow(append(settings, options...)...)
}

// nestedField reads one string out of a nested JSON request body.
func nestedField(t *testing.T, body string, path ...string) string {
	t.Helper()
	var current any
	if err := json.Unmarshal([]byte(body), &current); err != nil {
		t.Fatalf("decode request body %q: %v", body, err)
	}
	for _, key := range path {
		fields, ok := current.(map[string]any)
		if !ok {
			t.Fatalf("body %q holds no object at %q", body, key)
		}
		current = fields[key]
	}
	value, _ := current.(string)
	return value
}

// formField reads one field out of an urlencoded request body.
func formField(t *testing.T, body, key string) string {
	t.Helper()
	values, err := url.ParseQuery(body)
	if err != nil {
		t.Fatalf("decode form body %q: %v", body, err)
	}
	return values.Get(key)
}
