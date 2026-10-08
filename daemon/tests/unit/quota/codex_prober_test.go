package quota_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/activity"
	"github.com/jonaskahn/relo/internal/adapters/quota"
)

// codexUsageBody is what the Codex usage endpoint answers: the two rate
// limit windows, each with a length and a reset expressed as a delay.
const codexUsageBody = `{"plan_type":"plus","rate_limit":{
	"limit_reached":false,
	"primary_window":{"used_percent":42,"limit_window_seconds":18000,"reset_after_seconds":3600},
	"secondary_window":{"used_percent":12.5,"limit_window_seconds":604800,"reset_after_seconds":86400}
}}`

func TestCodexProber(t *testing.T) {
	now := time.Unix(1_784_800_000, 0)

	t.Run("the two windows are rendered with their reset moments", func(t *testing.T) {
		server, requests := codexEndpoint(t, http.StatusOK, codexUsageBody)
		prober := quota.NewCodexProber(quota.ProberOptions{
			Endpoint: server.URL, Now: func() time.Time { return now },
		})
		windows, err := prober.Probe(context.Background(), activity.Credential{
			ID: "credential-1", AccessToken: "token", Extra: map[string]string{"account_id": "account-9"},
		})
		if err != nil {
			t.Fatalf("Probe() error = %v", err)
		}
		if len(windows) != 2 {
			t.Fatalf("windows = %+v, want both windows", windows)
		}
		if windows[0].Window != activity.WindowFiveHours || windows[0].UsedPercent != 42 {
			t.Fatalf("primary window = %+v, want the five-hour window at 42%%", windows[0])
		}
		if windows[0].ResetAt != now.Add(time.Hour).Unix() {
			t.Fatalf("reset = %d, want the delay turned into a moment", windows[0].ResetAt)
		}
		if windows[1].Window != activity.WindowSevenDays || windows[1].Seconds != 604800 {
			t.Fatalf("secondary window = %+v, want the seven-day window", windows[1])
		}
		request := <-requests
		if request.Header.Get("Authorization") != "Bearer token" {
			t.Fatalf("authorization = %q, want the stored access token", request.Header.Get("Authorization"))
		}
		if request.Header.Get("chatgpt-account-id") != "account-9" {
			t.Fatalf("account header = %q, want the account the credential names", request.Header.Get("chatgpt-account-id"))
		}
	})

	t.Run("a refused probe is the only one that asks for a new login", func(t *testing.T) {
		for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden} {
			server, _ := codexEndpoint(t, status, "{}")
			prober := quota.NewCodexProber(quota.ProberOptions{Endpoint: server.URL})
			_, err := prober.Probe(context.Background(), activity.Credential{ID: "credential-1", AccessToken: "token"})
			if !errors.Is(err, activity.ErrProbeRejected) {
				t.Fatalf("Probe(%d) error = %v, want %v", status, err, activity.ErrProbeRejected)
			}
		}
	})

	t.Run("a rate limit keeps the last good reading", func(t *testing.T) {
		server, _ := codexEndpoint(t, http.StatusTooManyRequests, "{}")
		prober := quota.NewCodexProber(quota.ProberOptions{Endpoint: server.URL})
		_, err := prober.Probe(context.Background(), activity.Credential{ID: "credential-1", AccessToken: "token"})
		if !errors.Is(err, activity.ErrProbeResponse) {
			t.Fatalf("Probe() error = %v, want %v", err, activity.ErrProbeResponse)
		}
		if errors.Is(err, activity.ErrProbeRejected) {
			t.Fatal("a rate limit was reported as a refused credential")
		}
	})

	t.Run("an unreadable response is reported rather than stored", func(t *testing.T) {
		for name, body := range map[string]string{
			"malformed json": "{not json",
			"empty windows":  `{"rate_limit":{}}`,
		} {
			server, _ := codexEndpoint(t, http.StatusOK, body)
			prober := quota.NewCodexProber(quota.ProberOptions{Endpoint: server.URL})
			if _, err := prober.Probe(context.Background(), activity.Credential{ID: "credential-1", AccessToken: "token"}); !errors.Is(err, activity.ErrProbeResponse) {
				t.Fatalf("Probe(%s) error = %v, want %v", name, err, activity.ErrProbeResponse)
			}
		}
	})

	t.Run("a credential without a token is refused before anything is sent", func(t *testing.T) {
		prober := quota.NewCodexProber(quota.ProberOptions{Endpoint: "http://127.0.0.1:1/usage"})
		if _, err := prober.Probe(context.Background(), activity.Credential{ID: "credential-1"}); !errors.Is(err, activity.ErrNoCredential) {
			t.Fatalf("Probe() error = %v, want %v", err, activity.ErrNoCredential)
		}
	})
}

// codexEndpoint answers every probe with one status and body, and hands the
// requests it received to the caller.
func codexEndpoint(t *testing.T, status int, body string) (*httptest.Server, chan *http.Request) {
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
