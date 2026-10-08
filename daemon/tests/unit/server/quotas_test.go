package server_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/jonaskahn/relo/internal/account"
	"github.com/jonaskahn/relo/internal/activity"
	"github.com/jonaskahn/relo/internal/platform"
	"github.com/jonaskahn/relo/internal/server"
)

// recordingQuota stands in for the quota cache the daemon wires, so a test
// can see what a live response reported and to whom.
type recordingQuota struct {
	mu      sync.Mutex
	calls   []quotaCall
	failure error
}

type quotaCall struct {
	credentialID string
	windows      []activity.WindowSample
}

func (r *recordingQuota) Record(_ context.Context, credentialID string, windows []activity.WindowSample) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.failure != nil {
		return r.failure
	}
	r.calls = append(r.calls, quotaCall{credentialID: credentialID, windows: windows})
	return nil
}

func (r *recordingQuota) recorded() []quotaCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]quotaCall{}, r.calls...)
}

// staleQuota reports the credentials a failed probe left behind.
type staleQuota struct {
	mu  sync.Mutex
	ids map[string]bool
}

func (s *staleQuota) Stale(credentialID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ids[credentialID]
}

func (s *staleQuota) NoteFresh(credentialID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.ids, credentialID)
}

func TestQuotaRecording(t *testing.T) {
	t.Run("the windows a response reports are stored against the account that served it", func(t *testing.T) {
		h := newHarness(t)
		quota := &recordingQuota{}
		h.upstream.setHeaders(map[string]string{
			"x-codex-primary-used-percent":   "42",
			"x-codex-primary-window-minutes": "300",
			"x-codex-primary-reset-at":       "1784817996",
		})
		h.server = h.newServerWithOptions(h.cfg, []account.PoolEntry{defaultEntry()}, func(options *server.Options) {
			options.Quota = quota
		})
		recorder := h.dataPlane(http.MethodPost, "/v1/chat/completions", dataPlaneToken, strings.NewReader(streamingBody()))
		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, want the relayed answer", recorder.Code)
		}
		calls := quota.recorded()
		if len(calls) != 1 {
			t.Fatalf("recorded = %+v, want one reading", calls)
		}
		if calls[0].credentialID != "one" {
			t.Fatalf("credential = %q, want the account that served the request", calls[0].credentialID)
		}
		if len(calls[0].windows) != 1 || calls[0].windows[0].Window != activity.WindowFiveHours ||
			calls[0].windows[0].UsedPercent != 42 {
			t.Fatalf("windows = %+v, want the five-hour window at 42%%", calls[0].windows)
		}
	})

	t.Run("a response without quota headers records nothing", func(t *testing.T) {
		h := newHarness(t)
		quota := &recordingQuota{}
		h.server = h.newServerWithOptions(h.cfg, []account.PoolEntry{defaultEntry()}, func(options *server.Options) {
			options.Quota = quota
		})
		if recorder := h.dataPlane(http.MethodPost, "/v1/chat/completions", dataPlaneToken, strings.NewReader(streamingBody())); recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, want the relayed answer", recorder.Code)
		}
		if calls := quota.recorded(); len(calls) != 0 {
			t.Fatalf("recorded = %+v, want nothing", calls)
		}
	})

	t.Run("a quota store that fails does not fail the request", func(t *testing.T) {
		h := newHarness(t)
		quota := &recordingQuota{failure: errors.New("the quota table is gone")}
		h.upstream.setHeaders(map[string]string{"x-codex-primary-used-percent": "10"})
		h.server = h.newServerWithOptions(h.cfg, []account.PoolEntry{defaultEntry()}, func(options *server.Options) {
			options.Quota = quota
		})
		if recorder := h.dataPlane(http.MethodPost, "/v1/chat/completions", dataPlaneToken, strings.NewReader(streamingBody())); recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, want the client to see its answer anyway", recorder.Code)
		}
		if calls := quota.recorded(); len(calls) != 0 {
			t.Fatalf("recorded = %+v, want the failed write to be dropped", calls)
		}
		// The failure is worth telling the operator about, never worth
		// hiding from the log.
		if !strings.Contains(h.logBuffer.String(), "record the quota a response reported") {
			t.Fatalf("log = %q, want the quota write failure", h.logBuffer.String())
		}
	})
}

func TestQuotaEndpoint(t *testing.T) {
	t.Run("the stored windows are listed with their account and length", func(t *testing.T) {
		h := newHarness(t)
		if err := platform.NewQuotaStore(h.db).UpsertSnapshots(context.Background(), []activity.Snapshot{{
			CredentialID: "one", Window: activity.WindowFiveHours, UsedPercent: 42,
			Seconds: 5 * 60 * 60, Source: "probe", UpdatedAt: h.clock.Now(),
		}}); err != nil {
			t.Fatalf("UpsertSnapshots() error = %v", err)
		}
		h.server = h.newServerWithOptions(h.cfg, []account.PoolEntry{defaultEntry()}, func(options *server.Options) {
			options.QuotaFreshness = &staleQuota{ids: map[string]bool{"one": true}}
		})
		var payload struct {
			Items []struct {
				CredentialID string  `json:"credential_id"`
				ConnectionID string  `json:"connection_id"`
				Label        string  `json:"label"`
				Window       string  `json:"window"`
				Seconds      int64   `json:"window_seconds"`
				UsedPercent  float64 `json:"used_percent"`
				Source       string  `json:"source"`
				Stale        bool    `json:"stale"`
			} `json:"items"`
		}
		if status := h.managementJSON(t, http.MethodGet, "/api/v1/activity/quota", "", &payload); status != http.StatusOK {
			t.Fatalf("status = %d, want 200", status)
		}
		if len(payload.Items) != 1 {
			t.Fatalf("items = %+v, want the stored window", payload.Items)
		}
		window := payload.Items[0]
		if window.CredentialID != "one" || window.ConnectionID != "openai" || window.Label != "default" {
			t.Fatalf("window = %+v, want the account that reported it", window)
		}
		if window.Window != activity.WindowFiveHours || window.Seconds != 5*60*60 || window.UsedPercent != 42 {
			t.Fatalf("window = %+v, want the five-hour reading", window)
		}
		if !window.Stale {
			t.Fatal("a credential whose last probe failed was not marked stale")
		}
	})

	t.Run("money readings include their amount and currency", func(t *testing.T) {
		h := newHarness(t)
		if err := platform.NewQuotaStore(h.db).UpsertSnapshots(context.Background(), []activity.Snapshot{{
			CredentialID: "one", Window: "balance", Amount: new(12.5), Currency: "USD",
			Source: "probe", UpdatedAt: h.clock.Now(),
		}}); err != nil {
			t.Fatalf("UpsertSnapshots() error = %v", err)
		}
		var payload struct {
			Items []struct {
				Amount   *float64 `json:"amount"`
				Currency string   `json:"currency"`
			} `json:"items"`
		}
		if status := h.managementJSON(t, http.MethodGet, "/api/v1/activity/quota", "", &payload); status != http.StatusOK {
			t.Fatalf("status = %d, want 200", status)
		}
		if len(payload.Items) != 1 || payload.Items[0].Amount == nil ||
			*payload.Items[0].Amount != 12.5 || payload.Items[0].Currency != "USD" {
			t.Fatalf("items = %+v, want the money reading", payload.Items)
		}
	})

	t.Run("a live reading clears an older probe", func(t *testing.T) {
		h := newHarness(t)
		if err := platform.NewQuotaStore(h.db).UpsertSnapshots(context.Background(), []activity.Snapshot{{
			CredentialID: "one", Window: activity.WindowFiveHours, UsedPercent: 42,
			Seconds: 5 * 60 * 60, Source: "probe", UpdatedAt: h.clock.Now(),
		}}); err != nil {
			t.Fatalf("UpsertSnapshots() error = %v", err)
		}
		freshness := &staleQuota{ids: map[string]bool{"one": true}}
		h.upstream.setHeaders(map[string]string{"x-codex-primary-used-percent": "10"})
		h.server = h.newServerWithOptions(h.cfg, []account.PoolEntry{defaultEntry()}, func(options *server.Options) {
			options.Quota = &recordingQuota{}
			options.QuotaFreshness = freshness
		})
		if recorder := h.dataPlane(http.MethodPost, "/v1/chat/completions", dataPlaneToken, strings.NewReader(streamingBody())); recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, want the relayed answer", recorder.Code)
		}
		var payload struct {
			Items []struct {
				Stale bool `json:"stale"`
			} `json:"items"`
		}
		if status := h.managementJSON(t, http.MethodGet, "/api/v1/activity/quota", "", &payload); status != http.StatusOK {
			t.Fatalf("status = %d, want 200", status)
		}
		if len(payload.Items) != 1 || payload.Items[0].Stale {
			t.Fatalf("items = %+v, want the live reading to clear the older probe", payload.Items)
		}
	})

	t.Run("a build without a service reports the API as unavailable", func(t *testing.T) {
		h := newHarness(t)
		served := serverWithoutService(h)
		recorder := h.managementOn(served, http.MethodGet, "/api/v1/activity/quota", adminToken, nil)
		if recorder.Code != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, want 503", recorder.Code)
		}
	})
}

// recordingRefresh stands in for the quota worker so a test can see which
// connection a pane refresh asked to read again.
type recordingRefresh struct {
	mu        sync.Mutex
	providers []string
}

func (r *recordingRefresh) ProbeCredential(context.Context, string) ([]activity.Snapshot, error) {
	return nil, nil
}

func (r *recordingRefresh) ProbeProvider(_ context.Context, providerID string) ([]activity.Snapshot, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers = append(r.providers, providerID)
	return nil, nil
}

func (r *recordingRefresh) probed() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string{}, r.providers...)
}

func TestRefreshQuotaRoute(t *testing.T) {
	t.Run("a connection quota refresh is a known route", func(t *testing.T) {
		h := newHarness(t)
		refresh := &recordingRefresh{}
		h.server = h.newServerWithOptions(h.cfg, []account.PoolEntry{defaultEntry()}, func(options *server.Options) {
			options.QuotaRefresh = refresh
		})
		recorder := h.management(http.MethodPost, "/api/v1/connections/openai/quota/refresh", adminToken, nil)
		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body %s)", recorder.Code, recorder.Body.String())
		}
		if got := refresh.probed(); len(got) != 1 || got[0] != "openai" {
			t.Fatalf("probed = %v, want this connection", got)
		}
	})

	t.Run("a model refresh also reads this connection's quota", func(t *testing.T) {
		h := newHarness(t)
		refresh := &recordingRefresh{}
		h.server = h.newServerWithOptions(h.cfg, []account.PoolEntry{defaultEntry()}, func(options *server.Options) {
			options.QuotaRefresh = refresh
		})
		recorder := h.management(http.MethodPost, "/api/v1/connections/openai/models/refresh", adminToken, nil)
		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body %s)", recorder.Code, recorder.Body.String())
		}
		if got := refresh.probed(); len(got) != 1 || got[0] != "openai" {
			t.Fatalf("probed = %v, want the pane refresh to read quota too", got)
		}
	})
}
