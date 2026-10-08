package quota_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jonaskahn/relo/internal/activity"
	"github.com/jonaskahn/relo/internal/adapters/quota"
)

func TestDevinProberFlipsRemainingToUsed(t *testing.T) {
	const body = `{"userStatus":{"planStatus":{
		"planInfo":{"planName":"Pro","hideDailyQuota":false},
		"dailyQuotaRemainingPercent":25,
		"weeklyQuotaRemainingPercent":40,
		"dailyQuotaResetAtUnix":1790800000,
		"weeklyQuotaResetAtUnix":1791200000
	}}}`
	var captured struct {
		method  string
		connect string
		body    []byte
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured.method = r.Method
		captured.connect = r.Header.Get("Connect-Protocol-Version")
		captured.body, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	prober := quota.NewDevinProber(quota.SignInProberOptions{Endpoint: server.URL})
	windows, err := prober.Probe(context.Background(), activity.Credential{ID: "credential-1", AccessToken: "devin-key"})
	if err != nil {
		t.Fatalf("Probe() error = %v", err)
	}
	if len(windows) != 2 {
		t.Fatalf("windows = %+v, want daily and weekly", windows)
	}
	if windows[0].Window != "daily" || windows[0].UsedPercent != 75 || windows[0].ResetAt != 1790800000 {
		t.Fatalf("daily = %+v, want 75%% used", windows[0])
	}
	if windows[1].Window != activity.WindowSevenDays || windows[1].UsedPercent != 60 {
		t.Fatalf("weekly = %+v, want 60%% used", windows[1])
	}
	if captured.method != http.MethodPost || captured.connect != "1" {
		t.Fatalf("request = %s connect=%q, want POST with Connect 1", captured.method, captured.connect)
	}
	var payload map[string]any
	if err := json.Unmarshal(captured.body, &payload); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	metadata := payload["metadata"].(map[string]any)
	if metadata["apiKey"] != "devin-key" {
		t.Fatalf("metadata = %v, want the stored key", metadata)
	}
}

func TestDevinProberTreatsAMissingWeeklyPercentAsExhausted(t *testing.T) {
	const body = `{"userStatus":{"planStatus":{
		"planInfo":{"hideDailyQuota":true},
		"weeklyQuotaResetAtUnix":1791200000
	}}}`
	server, _ := signInEndpoint(t, http.StatusOK, body)
	prober := quota.NewDevinProber(quota.SignInProberOptions{Endpoint: server.URL})
	windows, err := prober.Probe(context.Background(), activity.Credential{ID: "credential-1", AccessToken: "key"})
	if err != nil {
		t.Fatalf("Probe() error = %v", err)
	}
	if len(windows) != 1 || windows[0].UsedPercent != 100 {
		t.Fatalf("windows = %+v, want a weekly window at 100%%", windows)
	}
}

func TestDevinProberRefusalIsARejectedLogin(t *testing.T) {
	server, _ := signInEndpoint(t, http.StatusForbidden, `{}`)
	prober := quota.NewDevinProber(quota.SignInProberOptions{Endpoint: server.URL})
	_, err := prober.Probe(context.Background(), activity.Credential{ID: "credential-1", AccessToken: "key"})
	if !errors.Is(err, activity.ErrProbeRejected) {
		t.Fatalf("Probe() error = %v, want %v", err, activity.ErrProbeRejected)
	}
}
