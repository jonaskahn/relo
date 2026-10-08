package quota_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/jonaskahn/relo/internal/activity"
	"github.com/jonaskahn/relo/internal/adapters/quota"
)

// signInEndpoint answers one sign-in quota probe and reports what it received.
func signInEndpoint(t *testing.T, status int, body string) (*httptest.Server, chan *http.Request) {
	t.Helper()
	requests := make(chan *http.Request, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case requests <- r:
		default:
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return server, requests
}

// TestGrokProber covers the weekly shared pool a Grok subscription reports:
// the percentage, the period it applies to, and the CLI headers the proxy
// expects beside the token.
func TestGrokProber(t *testing.T) {
	const body = `{"config":{"creditUsagePercent":63.5,
		"currentPeriod":{"type":"USAGE_PERIOD_TYPE_WEEKLY","start":"2026-07-03T04:01:09Z","end":"2026-07-10T04:01:09Z"}}}`
	server, requests := signInEndpoint(t, http.StatusOK, body)
	prober := quota.NewGrokProber(quota.SignInProberOptions{Endpoint: server.URL})
	windows, err := prober.Probe(context.Background(), activity.Credential{
		ID: "credential-1", AccessToken: "token", Extra: map[string]string{"account_id": "user-7"},
	})
	if err != nil {
		t.Fatalf("Probe() error = %v", err)
	}
	if len(windows) != 1 || windows[0].UsedPercent != 63.5 {
		t.Fatalf("windows = %+v, want the weekly pool at 63.5%%", windows)
	}
	if windows[0].Window != activity.WindowSevenDays {
		t.Fatalf("window = %q, want the weekly name", windows[0].Window)
	}
	if windows[0].ResetAt == 0 {
		t.Fatal("reset = 0, want the period end")
	}
	request := <-requests
	if request.Header.Get("Authorization") != "Bearer token" {
		t.Fatalf("authorization = %q, want the stored access token", request.Header.Get("Authorization"))
	}
	if request.Header.Get("x-xai-token-auth") != "xai-grok-cli" {
		t.Fatalf("token auth header = %q, want the CLI scheme", request.Header.Get("x-xai-token-auth"))
	}
	if request.Header.Get("x-userid") != "user-7" {
		t.Fatalf("user header = %q, want the account the credential names", request.Header.Get("x-userid"))
	}
}

// TestGrokProberRejectsAnUnreadablePeriod keeps a schema change from being
// read as a quota of zero.
func TestGrokProberRejectsAnUnreadablePeriod(t *testing.T) {
	server, _ := signInEndpoint(t, http.StatusOK, `{"config":{"creditUsagePercent":10}}`)
	prober := quota.NewGrokProber(quota.SignInProberOptions{Endpoint: server.URL})
	_, err := prober.Probe(context.Background(), activity.Credential{ID: "credential-1", AccessToken: "token"})
	if !errors.Is(err, activity.ErrProbeResponse) {
		t.Fatalf("Probe() error = %v, want %v", err, activity.ErrProbeResponse)
	}
}

// TestCursorProber covers the monthly allowance, the two pools beside it, and
// the Connect call the dashboard answers.
func TestCursorProber(t *testing.T) {
	const body = `{"planUsage":{"limit":2000,"remaining":500,"totalPercentUsed":75,"autoPercentUsed":40,"apiPercentUsed":90},"billingCycleEnd":"2026-10-01T00:00:00Z"}`
	server, requests := signInEndpoint(t, http.StatusOK, body)
	prober := quota.NewCursorProber(quota.SignInProberOptions{Endpoint: server.URL})
	windows, err := prober.Probe(context.Background(), activity.Credential{ID: "credential-1", AccessToken: "token"})
	if err != nil {
		t.Fatalf("Probe() error = %v", err)
	}
	if len(windows) != 3 {
		t.Fatalf("windows = %+v, want the total and the two pools", windows)
	}
	if windows[0].UsedPercent != 75 || windows[0].Window != "30d" {
		t.Fatalf("total window = %+v, want the monthly total", windows[0])
	}
	if windows[1].UsedPercent != 40 || windows[2].UsedPercent != 90 {
		t.Fatalf("pool windows = %+v, want the auto and api pools", windows[1:])
	}
	request := <-requests
	if request.Method != http.MethodPost {
		t.Fatalf("method = %q, want the Connect call's POST", request.Method)
	}
	if request.Header.Get("Connect-Protocol-Version") != "1" {
		t.Fatalf("connect header = %q, want the Connect version", request.Header.Get("Connect-Protocol-Version"))
	}
}

// TestCursorProberDerivesPercentFromTheAmounts covers a response that names no
// percentage but states the limit and what is left of it.
func TestCursorProberDerivesPercentFromTheAmounts(t *testing.T) {
	server, _ := signInEndpoint(t, http.StatusOK, `{"planUsage":{"limit":100,"remaining":25}}`)
	prober := quota.NewCursorProber(quota.SignInProberOptions{Endpoint: server.URL})
	windows, err := prober.Probe(context.Background(), activity.Credential{ID: "credential-1", AccessToken: "token"})
	if err != nil {
		t.Fatalf("Probe() error = %v", err)
	}
	if len(windows) != 1 || windows[0].UsedPercent != 75 {
		t.Fatalf("windows = %+v, want the derived 75%%", windows)
	}
}

// TestCopilotProber covers the credit meter of a Copilot seat, which is
// authenticated by the GitHub token behind the Copilot token.
func TestCopilotProber(t *testing.T) {
	const body = `{"copilot_plan":"individual","quota_reset_date":"2026-10-01","quota_snapshots":{"premium_interactions":{"entitlement":1500,"remaining":300,"percent_remaining":80}}}`
	server, requests := signInEndpoint(t, http.StatusOK, body)
	prober := quota.NewCopilotProber(quota.SignInProberOptions{Endpoint: server.URL})
	windows, err := prober.Probe(context.Background(), activity.Credential{
		ID: "credential-1", AccessToken: "copilot-token", RefreshToken: "github-token",
	})
	if err != nil {
		t.Fatalf("Probe() error = %v", err)
	}
	if len(windows) != 1 || windows[0].UsedPercent != 20 {
		t.Fatalf("windows = %+v, want the credits at 20%% used", windows)
	}
	request := <-requests
	if request.Header.Get("Authorization") != "token github-token" {
		t.Fatalf("authorization = %q, want the GitHub token scheme", request.Header.Get("Authorization"))
	}
}

// TestCopilotProberWithoutAGitHubTokenIsUnavailable keeps a seat Relo cannot
// ask from inventing a meter.
func TestCopilotProberWithoutAGitHubTokenIsUnavailable(t *testing.T) {
	server, _ := signInEndpoint(t, http.StatusOK, `{}`)
	prober := quota.NewCopilotProber(quota.SignInProberOptions{Endpoint: server.URL})
	_, err := prober.Probe(context.Background(), activity.Credential{ID: "credential-1", AccessToken: "copilot-token"})
	if !errors.Is(err, activity.ErrNoCredential) {
		t.Fatalf("Probe() error = %v, want %v", err, activity.ErrNoCredential)
	}
}

// TestKimiProberReadsTheConnectionsOwnUsagePath covers a provider whose quota
// lives on the connection's own host rather than a fixed vendor endpoint.
func TestKimiProberReadsTheConnectionsOwnUsagePath(t *testing.T) {
	server, requests := signInEndpoint(t, http.StatusOK, `{"usage":{"limit":100,"used":30}}`)
	prober := quota.NewKimiProber(quota.SignInProberOptions{})
	windows, err := prober.Probe(context.Background(), activity.Credential{
		ID: "credential-1", AccessToken: "token", BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("Probe() error = %v", err)
	}
	if len(windows) != 1 || windows[0].UsedPercent != 30 {
		t.Fatalf("windows = %+v, want the weekly window at 30%%", windows)
	}
	request := <-requests
	if parsed, _ := url.Parse(request.URL.String()); parsed.Path != "/usages" {
		t.Fatalf("path = %q, want the connection's usage path", request.URL.Path)
	}
}

// TestSignInProberRefusalKeepsTheLastReading covers the classification the
// worker depends on: a refused credential is worth a fresh login, anything
// else is a reading the worker leaves alone.
func TestSignInProberRefusalKeepsTheLastReading(t *testing.T) {
	server, _ := signInEndpoint(t, http.StatusUnauthorized, `{}`)
	prober := quota.NewGrokProber(quota.SignInProberOptions{Endpoint: server.URL})
	_, err := prober.Probe(context.Background(), activity.Credential{ID: "credential-1", AccessToken: "token"})
	if !errors.Is(err, activity.ErrProbeRejected) {
		t.Fatalf("Probe() error = %v, want %v", err, activity.ErrProbeRejected)
	}
}
