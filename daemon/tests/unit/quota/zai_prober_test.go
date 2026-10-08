package quota_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/jonaskahn/relo/internal/activity"
	"github.com/jonaskahn/relo/internal/adapters/quota"
)

func TestZAIProberReadsSessionAndWeeklyWindows(t *testing.T) {
	const body = `{"data":{"limits":[
		{"type":"CREDIT_LIMIT","unit":3,"number":5,"percentage":12.5,"nextResetTime":1790712000000},
		{"type":"TOKENS_LIMIT","unit":6,"number":1,"percentage":40,"nextResetTime":1791244800000}
	]}}`
	server, requests := signInEndpoint(t, http.StatusOK, body)
	prober := quota.NewZAIProber(quota.SignInProberOptions{Endpoint: server.URL})
	windows, err := prober.Probe(context.Background(), activity.Credential{ID: "credential-1", AccessToken: "zai-key"})
	if err != nil {
		t.Fatalf("Probe() error = %v", err)
	}
	if len(windows) != 2 {
		t.Fatalf("windows = %+v, want session and weekly", windows)
	}
	if windows[0].Window != activity.WindowFiveHours || windows[0].UsedPercent != 12.5 || windows[0].Seconds != 5*60*60 {
		t.Fatalf("session = %+v, want 12.5%% of 5h", windows[0])
	}
	if windows[1].Window != activity.WindowSevenDays || windows[1].UsedPercent != 40 {
		t.Fatalf("weekly = %+v, want 40%% of 7d", windows[1])
	}
	if windows[0].ResetAt != 1790712000 {
		t.Fatalf("session reset = %d, want milliseconds converted to seconds", windows[0].ResetAt)
	}
	request := <-requests
	if request.Header.Get("Authorization") != "Bearer zai-key" {
		t.Fatalf("authorization = %q", request.Header.Get("Authorization"))
	}
}

func TestZAIProberTreatsAMissingCodingPlanAsUnreadable(t *testing.T) {
	server, _ := signInEndpoint(t, http.StatusOK, `{"success":false,"code":500,"msg":"no active coding plan"}`)
	prober := quota.NewZAIProber(quota.SignInProberOptions{Endpoint: server.URL})
	_, err := prober.Probe(context.Background(), activity.Credential{ID: "credential-1", AccessToken: "zai-key"})
	if !errors.Is(err, activity.ErrProbeResponse) {
		t.Fatalf("Probe() error = %v, want %v", err, activity.ErrProbeResponse)
	}
}

func TestZAIProberRefusalIsARejectedLogin(t *testing.T) {
	server, _ := signInEndpoint(t, http.StatusUnauthorized, `{}`)
	prober := quota.NewZAIProber(quota.SignInProberOptions{Endpoint: server.URL})
	_, err := prober.Probe(context.Background(), activity.Credential{ID: "credential-1", AccessToken: "zai-key"})
	if !errors.Is(err, activity.ErrProbeRejected) {
		t.Fatalf("Probe() error = %v, want %v", err, activity.ErrProbeRejected)
	}
}

func TestZAIHost(t *testing.T) {
	if !quota.ZAIHost("https://api.z.ai/api/paas/v4") || !quota.ZAIHost("https://open.bigmodel.cn/api/paas/v4") {
		t.Fatal("a Z.ai host was not recognised")
	}
	if quota.ZAIHost("https://openrouter.ai/api/v1") {
		t.Fatal("OpenRouter was recognised as Z.ai")
	}
}
