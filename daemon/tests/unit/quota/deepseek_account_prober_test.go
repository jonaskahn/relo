package quota_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jonaskahn/relo/internal/activity"
	"github.com/jonaskahn/relo/internal/adapters/quota"
)

// The platform reports wallet amounts, not rate-limit windows, so a live
// probe confirms the grant and reports no window at all. A synthetic zero
// percent would rank an empty wallet as healthy.
func TestDeepSeekAccountProber(t *testing.T) {
	t.Run("a live grant reports no window", func(t *testing.T) {
		var seen http.Header
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			seen = r.Header.Clone()
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"code":0,"data":{"biz_code":0,"biz_data":`+
				`{"user_summary":{"normal_wallets":[{"currency":"CNY","balance":"0.00"}]}}}}`)
		}))
		t.Cleanup(server.Close)
		prober := quota.NewDeepSeekAccountProber(quota.ProberOptions{Endpoint: server.URL})
		samples, err := prober.Probe(context.Background(),
			activity.Credential{ID: "c1", AccessToken: "grant"})
		if err != nil {
			t.Fatalf("Probe() error = %v", err)
		}
		if len(samples) != 0 {
			t.Fatalf("samples = %+v, want no fabricated window", samples)
		}
		if got := seen.Get("x-dsh-auth-token"); got != "grant" {
			t.Fatalf("probe header = %q, want the grant in the harness header", got)
		}
	})

	t.Run("a refused grant is reported as rejected", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		}))
		t.Cleanup(server.Close)
		prober := quota.NewDeepSeekAccountProber(quota.ProberOptions{Endpoint: server.URL})
		_, err := prober.Probe(context.Background(),
			activity.Credential{ID: "c1", AccessToken: "stale"})
		if !errors.Is(err, activity.ErrProbeRejected) {
			t.Fatalf("Probe() error = %v, want %v", err, activity.ErrProbeRejected)
		}
	})

	t.Run("no stored grant is named", func(t *testing.T) {
		prober := quota.NewDeepSeekAccountProber(quota.ProberOptions{})
		if _, err := prober.Probe(context.Background(), activity.Credential{ID: "c1"}); !errors.Is(err, activity.ErrNoCredential) {
			t.Fatalf("Probe() error = %v, want %v", err, activity.ErrNoCredential)
		}
	})
}
