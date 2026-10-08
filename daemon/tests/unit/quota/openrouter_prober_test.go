package quota_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jonaskahn/relo/internal/activity"
	"github.com/jonaskahn/relo/internal/adapters/quota"
)

func TestOpenRouterProberReadsCreditsAndTheKeyCap(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/credits":
			_, _ = w.Write([]byte(`{"data":{"total_credits":100,"total_usage":25}}`))
		case "/key":
			_, _ = w.Write([]byte(`{"data":{"limit":50,"limit_remaining":10}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	prober := quota.NewOpenRouterProber(quota.SignInProberOptions{Endpoint: server.URL})
	windows, err := prober.Probe(context.Background(), activity.Credential{ID: "credential-1", AccessToken: "sk-or"})
	if err != nil {
		t.Fatalf("Probe() error = %v", err)
	}
	if len(windows) != 2 {
		t.Fatalf("windows = %+v, want credits and the key cap", windows)
	}
	if windows[0].Window != "credits" || windows[0].UsedPercent != 25 {
		t.Fatalf("credits = %+v, want 25%%", windows[0])
	}
	if windows[1].Window != "key" || windows[1].UsedPercent != 80 {
		t.Fatalf("key = %+v, want 80%%", windows[1])
	}
}

func TestOpenRouterProberKeepsCreditsWhenTheKeyCallFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/key" {
			http.Error(w, "no", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"total_credits":40,"total_usage":10}}`))
	}))
	t.Cleanup(server.Close)
	prober := quota.NewOpenRouterProber(quota.SignInProberOptions{Endpoint: server.URL})
	windows, err := prober.Probe(context.Background(), activity.Credential{ID: "credential-1", AccessToken: "sk-or"})
	if err != nil {
		t.Fatalf("Probe() error = %v", err)
	}
	if len(windows) != 1 || windows[0].UsedPercent != 25 {
		t.Fatalf("windows = %+v, want credits alone", windows)
	}
}

func TestOpenRouterProberRefusalIsARejectedLogin(t *testing.T) {
	server, _ := signInEndpoint(t, http.StatusUnauthorized, `{}`)
	prober := quota.NewOpenRouterProber(quota.SignInProberOptions{Endpoint: server.URL})
	_, err := prober.Probe(context.Background(), activity.Credential{ID: "credential-1", AccessToken: "sk-or"})
	if !errors.Is(err, activity.ErrProbeRejected) {
		t.Fatalf("Probe() error = %v, want %v", err, activity.ErrProbeRejected)
	}
}

func TestOpenRouterHost(t *testing.T) {
	if !quota.OpenRouterHost("https://openrouter.ai/api/v1") {
		t.Fatal("openrouter.ai was not recognised")
	}
	if quota.OpenRouterHost("https://api.anthropic.com/v1") {
		t.Fatal("anthropic was recognised as OpenRouter")
	}
}
