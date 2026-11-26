package muse_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/activity"
	"github.com/jonaskahn/relo/internal/adapters/muse"
	"github.com/jonaskahn/relo/tests/testkit"
)

func quotaEndpoint(t *testing.T, status int, body string) (*httptest.Server, *probeRecorder) {
	t.Helper()
	recorder := &probeRecorder{status: status, body: body}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recorder.record(r)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(recorder.status)
		_, _ = w.Write([]byte(recorder.body))
	}))
	t.Cleanup(server.Close)
	return server, recorder
}

type probeRecorder struct {
	mu      sync.Mutex
	status  int
	body    string
	calls   int
	request *http.Request
}

func (r *probeRecorder) record(request *http.Request) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	r.request = request.Clone(request.Context())
}

func (r *probeRecorder) snapshot() (int, *http.Request) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls, r.request
}

func TestQuotaProberReportsMetaWindows(t *testing.T) {
	const body = `{"api_key":"LLM|key","is_subs_active":true,` +
		`"subs_usage":{"window":{"used_percent":42,"resets_at":1800000000,"window_duration_mins":300},` +
		`"weekly":{"used_percent":75,"resets_at":"2030-01-08T00:00:00Z"}}}`
	server, recorder := quotaEndpoint(t, http.StatusOK, body)
	prober := muse.NewQuotaProber(muse.QuotaProberOptions{Endpoint: server.URL})
	windows, err := prober.Probe(context.Background(), activity.Credential{
		ID: "cred-1", Extra: map[string]string{muse.AccessTokenExtraKey: "account-token"},
	})
	if err != nil {
		t.Fatalf("Probe() error = %v", err)
	}
	if len(windows) != 2 {
		t.Fatalf("windows = %+v, want the rolling and weekly windows", windows)
	}
	if windows[0].Window != "5h" || windows[0].UsedPercent != 42 || windows[0].ResetAt != 1800000000 || windows[0].Seconds != 300*60 {
		t.Fatalf("rolling window = %+v", windows[0])
	}
	if windows[1].Window != activity.WindowSevenDays || windows[1].UsedPercent != 75 || windows[1].Seconds != 7*24*60*60 {
		t.Fatalf("weekly window = %+v", windows[1])
	}
	_, request := recorder.snapshot()
	if request.Method != http.MethodPost || request.Header.Get("Authorization") != "Bearer account-token" {
		t.Fatalf("request = %s %v, want the account token in a POST", request.Method, request.Header)
	}
	if request.Header.Get("x-api-version") != "1.0.0" {
		t.Fatalf("version header = %q, want the vendor version", request.Header.Get("x-api-version"))
	}
}

func TestQuotaProberNeedsAnAccountToken(t *testing.T) {
	prober := muse.NewQuotaProber(muse.QuotaProberOptions{})
	_, err := prober.Probe(context.Background(), activity.Credential{ID: "cred-1"})
	if !errors.Is(err, activity.ErrNoCredential) {
		t.Fatalf("Probe() error = %v, want %v", err, activity.ErrNoCredential)
	}
}

func TestQuotaProberNeverInventsMissingUsage(t *testing.T) {
	for _, body := range []string{
		`{"is_subs_active":true}`,
		`{"is_subs_active":false,"subs_usage":{"weekly":{"used_percent":5}}}`,
		`{"require_payment":true,"subs_usage":{"weekly":{"used_percent":5}}}`,
		`{"is_subs_active":true,"subs_usage":{"weekly":{"used_percent":null}}}`,
	} {
		server, _ := quotaEndpoint(t, http.StatusOK, body)
		prober := muse.NewQuotaProber(muse.QuotaProberOptions{Endpoint: server.URL})
		if _, err := prober.Probe(context.Background(), activity.Credential{
			ID: "cred-1", Extra: map[string]string{muse.AccessTokenExtraKey: "token"},
		}); err == nil {
			t.Errorf("Probe(%s) error = nil, want unavailable", body)
		}
	}
}

func TestQuotaProberReauthAndBackoff(t *testing.T) {
	t.Run("a refusal asks for a new login", func(t *testing.T) {
		server, _ := quotaEndpoint(t, http.StatusForbidden, `{}`)
		prober := muse.NewQuotaProber(muse.QuotaProberOptions{Endpoint: server.URL})
		_, err := prober.Probe(context.Background(), activity.Credential{
			ID: "cred-1", Extra: map[string]string{muse.AccessTokenExtraKey: "token"},
		})
		if !errors.Is(err, activity.ErrProbeRejected) {
			t.Fatalf("Probe() error = %v, want %v", err, activity.ErrProbeRejected)
		}
	})
	t.Run("a refused token backs off before asking again", func(t *testing.T) {
		clk := testkit.NewFakeClock(time.Now())
		server, recorder := quotaEndpoint(t, http.StatusForbidden, `{}`)
		prober := muse.NewQuotaProber(muse.QuotaProberOptions{Endpoint: server.URL, Now: clk.Now})
		credential := activity.Credential{ID: "cred-1", Extra: map[string]string{muse.AccessTokenExtraKey: "token"}}
		for range 2 {
			if _, err := prober.Probe(context.Background(), credential); err == nil {
				t.Fatal("Probe() error = nil, want the refusal")
			}
		}
		calls, _ := recorder.snapshot()
		if calls != 1 {
			t.Fatalf("calls = %d, want one request inside the backoff", calls)
		}
		clk.Add(5 * time.Minute)
		if _, err := prober.Probe(context.Background(), credential); err == nil {
			t.Fatal("Probe() error = nil, want a second refusal")
		}
		calls, _ = recorder.snapshot()
		if calls != 2 {
			t.Fatalf("calls = %d, want a retry after the backoff", calls)
		}
	})
}
