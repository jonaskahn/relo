package quota_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/activity"
	"github.com/jonaskahn/relo/internal/adapters/quota"
	"github.com/jonaskahn/relo/internal/adapters/wire/anthropic"
)

func TestClaudeProberReadsTheSessionAndWeeklyWindows(t *testing.T) {
	const body = `{"five_hour":{"utilization":42.5,"resets_at":"2026-09-30T06:00:00Z"},
		"seven_day":{"utilization":18,"resets_at":"2026-10-06T00:00:00Z"},
		"seven_day_sonnet":{"utilization":7,"resets_at":"2026-10-06T00:00:00Z"},
		"limits":[{"kind":"weekly_scoped","percent":3,"resets_at":"2026-10-06T00:00:00Z",
			"scope":{"model":{"display_name":"Fable"}}}]}`
	server, requests := signInEndpoint(t, http.StatusOK, body)
	prober := quota.NewClaudeProber(quota.SignInProberOptions{Endpoint: server.URL})
	windows, err := prober.Probe(context.Background(), activity.Credential{ID: "credential-1", AccessToken: "token"})
	if err != nil {
		t.Fatalf("Probe() error = %v", err)
	}
	if len(windows) != 4 {
		t.Fatalf("windows = %+v, want session, weekly, sonnet, and fable", windows)
	}
	if windows[0].Window != activity.WindowFiveHours || windows[0].UsedPercent != 42.5 {
		t.Fatalf("session = %+v, want 42.5%% of 5h", windows[0])
	}
	if windows[1].Window != activity.WindowSevenDays || windows[2].Window != "Sonnet" || windows[3].Window != "Fable" {
		t.Fatalf("windows = %+v, want weekly, Sonnet, then Fable", windows)
	}
	reset := time.Unix(windows[0].ResetAt, 0).UTC()
	if reset.Format(time.RFC3339) != "2026-09-30T06:00:00Z" {
		t.Fatalf("session reset = %s", reset)
	}
	request := <-requests
	if request.Header.Get("Authorization") != "Bearer token" {
		t.Fatalf("authorization = %q, want the stored access token", request.Header.Get("Authorization"))
	}
	if request.Header.Get("anthropic-beta") != anthropic.ClaudeCodeBeta {
		t.Fatalf("beta = %q, want the Claude Code beta profile", request.Header.Get("anthropic-beta"))
	}
	if beta := request.Header.Get("anthropic-beta"); strings.Contains(beta, "context-1m-2025-08-07") || strings.Contains(beta, "afk-mode-2026-01-31") {
		t.Fatalf("beta = %q, want the current Claude Code profile", beta)
	}
	if request.Header.Get("User-Agent") != anthropic.CLIUserAgent {
		t.Fatalf("user agent = %q, want the Claude Code agent", request.Header.Get("User-Agent"))
	}
	if request.Header.Get("Accept") != "application/json, text/plain, */*" {
		t.Fatalf("accept = %q", request.Header.Get("Accept"))
	}
	if request.Header.Get("Accept-Encoding") != "gzip, compress, deflate, br" {
		t.Fatalf("accept-encoding = %q", request.Header.Get("Accept-Encoding"))
	}
	if request.Header.Get("Content-Type") != "application/json" || request.Header.Get("Connection") != "keep-alive" {
		t.Fatalf("content-type = %q, connection = %q", request.Header.Get("Content-Type"), request.Header.Get("Connection"))
	}
}

func TestClaudeProberRejectsAnEmptyReading(t *testing.T) {
	server, _ := signInEndpoint(t, http.StatusOK, `{}`)
	prober := quota.NewClaudeProber(quota.SignInProberOptions{Endpoint: server.URL})
	_, err := prober.Probe(context.Background(), activity.Credential{ID: "credential-1", AccessToken: "token"})
	if !errors.Is(err, activity.ErrProbeResponse) {
		t.Fatalf("Probe() error = %v, want %v", err, activity.ErrProbeResponse)
	}
}

func TestClaudeProberRefusalIsARejectedLogin(t *testing.T) {
	server, _ := signInEndpoint(t, http.StatusUnauthorized, `{}`)
	prober := quota.NewClaudeProber(quota.SignInProberOptions{Endpoint: server.URL})
	_, err := prober.Probe(context.Background(), activity.Credential{ID: "credential-1", AccessToken: "token"})
	if !errors.Is(err, activity.ErrProbeRejected) {
		t.Fatalf("Probe() error = %v, want %v", err, activity.ErrProbeRejected)
	}
}

func TestClaudeProberWithoutATokenIsUnavailable(t *testing.T) {
	_, err := quota.NewClaudeProber(quota.SignInProberOptions{}).Probe(
		context.Background(), activity.Credential{ID: "credential-1"})
	if !errors.Is(err, activity.ErrNoCredential) {
		t.Fatalf("Probe() error = %v, want %v", err, activity.ErrNoCredential)
	}
}
