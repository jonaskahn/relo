package quota_test

import (
	"context"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/activity"
	"github.com/jonaskahn/relo/internal/adapters/antigravity"
	"github.com/jonaskahn/relo/internal/adapters/quota"
	"github.com/jonaskahn/relo/tests/testkit"
)

func TestAntigravityProber(t *testing.T) {
	t.Run("windows are derived from the fixture", func(t *testing.T) {
		upstream := testkit.MockUpstream(t, map[string]string{
			"/v1internal:fetchAvailableModels": "google/antigravity_models.json",
		})
		// The summary is not served here, so the account falls back to the
		// per-model listing.
		prober := quota.NewAntigravityProber(quota.ProberOptions{BaseURL: upstream.URL})
		windows, err := prober.Probe(context.Background(), activity.Credential{ID: "credential-1", AccessToken: "token"})
		if err != nil {
			t.Fatalf("Probe() error = %v", err)
		}
		if len(windows) != 2 {
			t.Fatalf("windows = %+v, want the Gem and Cla windows", windows)
		}
		if windows[0].Window != "Cla" || windows[0].UsedPercent != 75 {
			t.Fatalf("first window = %+v, want Cla at 75%% used", windows[0])
		}
		if windows[0].ResetAt == 0 {
			t.Fatalf("window = %+v, want the reset time parsed", windows[0])
		}
		if windows[1].Window != "Gem" || windows[1].UsedPercent != 20 {
			t.Fatalf("second window = %+v, want Gem at 20%% used", windows[1])
		}
		for _, window := range windows {
			if window.Seconds != 5*60*60 {
				t.Fatalf("window = %+v, want the session length the listing reports", window)
			}
		}
	})

	t.Run("probe failures are typed", func(t *testing.T) {
		cases := []struct {
			name   string
			status int
			body   string
			token  string
			want   error
		}{
			{"refused", http.StatusUnauthorized, `{}`, "token", activity.ErrProbeRejected},
			{"forbidden", http.StatusForbidden, `{}`, "token", activity.ErrProbeRejected},
			{"server error", http.StatusInternalServerError, `{}`, "token", activity.ErrProbeResponse},
			{"no windows", http.StatusOK, `{"models":{}}`, "token", activity.ErrProbeResponse},
			{"malformed", http.StatusOK, `not json`, "token", nil},
			{"no token", http.StatusOK, `{}`, "", activity.ErrNoCredential},
		}
		for _, tt := range cases {
			t.Run(tt.name, func(t *testing.T) {
				server := httptest.NewServer(&statusServer{status: tt.status, body: tt.body})
				t.Cleanup(server.Close)
				prober := quota.NewAntigravityProber(quota.ProberOptions{BaseURL: server.URL})
				_, err := prober.Probe(context.Background(), activity.Credential{ID: "credential-1", AccessToken: tt.token})
				if err == nil {
					t.Fatal("Probe() error = nil, want a failure")
				}
				if tt.want != nil && !errors.Is(err, tt.want) {
					t.Fatalf("Probe() error = %v, want %v", err, tt.want)
				}
			})
		}
	})

	t.Run("a transport failure is reported", func(t *testing.T) {
		prober := quota.NewAntigravityProber(quota.ProberOptions{
			HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return nil, errors.New("connection refused")
			})},
			Endpoint: "https://cloudcode.test/probe",
		})
		if _, err := prober.Probe(context.Background(), activity.Credential{ID: "c", AccessToken: "token"}); err == nil {
			t.Fatal("Probe() error = nil, want the transport failure")
		}
	})

	t.Run("the default endpoint is the cloud code assist api", func(t *testing.T) {
		prober := quota.NewAntigravityProber(quota.ProberOptions{})
		if prober == nil {
			t.Fatal("NewAntigravityProber() = nil")
		}
	})

	t.Run("the summary reports both pools with both windows", func(t *testing.T) {
		server := httptest.NewServer(jsonBodyHandler(antigravitySummary))
		t.Cleanup(server.Close)
		prober := quota.NewAntigravityProber(quota.ProberOptions{BaseURL: server.URL})
		windows, err := prober.Probe(context.Background(), activity.Credential{ID: "credential-1", AccessToken: "token"})
		if err != nil {
			t.Fatalf("Probe() error = %v", err)
		}
		want := []struct {
			window  string
			used    float64
			seconds int64
		}{
			{"Gem", 20, 5 * 60 * 60},
			{"Gem (Weekly)", 50, 7 * 24 * 60 * 60},
			{"Cla", 60, 5 * 60 * 60},
			{"Cla (Weekly)", 0, 7 * 24 * 60 * 60},
		}
		if len(windows) != len(want) {
			t.Fatalf("windows = %+v, want the four pools", windows)
		}
		for index, expected := range want {
			window := windows[index]
			if window.Window != expected.window || math.Abs(window.UsedPercent-expected.used) > 0.001 || window.Seconds != expected.seconds {
				t.Fatalf("window %d = %+v, want %+v", index, window, expected)
			}
		}
		if windows[0].ResetAt != time.Date(2026, 7, 2, 16, 0, 0, 0, time.UTC).Unix() {
			t.Fatalf("reset = %d, want the summary's reset time", windows[0].ResetAt)
		}
	})

	t.Run("the summary is read from either envelope", func(t *testing.T) {
		server := httptest.NewServer(jsonBodyHandler(`{"response":{"groups":[{"buckets":[
			{"bucketId":"gemini-5h","remainingFraction":0.9},
			{"bucketId":"3p-weekly","remainingFraction":0.25}]}]}}`))
		t.Cleanup(server.Close)
		prober := quota.NewAntigravityProber(quota.ProberOptions{BaseURL: server.URL})
		windows, err := prober.Probe(context.Background(), activity.Credential{ID: "credential-1", AccessToken: "token"})
		if err != nil {
			t.Fatalf("Probe() error = %v", err)
		}
		if len(windows) != 2 || windows[0].Window != "Gem" || windows[1].Window != "Cla (Weekly)" {
			t.Fatalf("windows = %+v, want the two windows the envelope names", windows)
		}
	})

	t.Run("an unknown or unusable bucket drops only its own window", func(t *testing.T) {
		server := httptest.NewServer(jsonBodyHandler(`{"groups":[{"buckets":[
			{"bucketId":"gemini-image-5h","remainingFraction":0.1},
			{"bucketId":"3p-5h","remainingFraction":"lots"},
			{"bucketId":"gemini-5h","remainingFraction":0.25}]}]}`))
		t.Cleanup(server.Close)
		prober := quota.NewAntigravityProber(quota.ProberOptions{BaseURL: server.URL})
		windows, err := prober.Probe(context.Background(), activity.Credential{ID: "credential-1", AccessToken: "token"})
		if err != nil {
			t.Fatalf("Probe() error = %v", err)
		}
		if len(windows) != 1 || windows[0].Window != "Gem" || windows[0].UsedPercent != 75 {
			t.Fatalf("windows = %+v, want only the readable known bucket", windows)
		}
	})

	t.Run("both calls carry the fingerprint and the billing project", func(t *testing.T) {
		requests := &recordedProbes{}
		server := httptest.NewServer(requests.handler(`{"groups":[{"buckets":
			[{"bucketId":"gemini-5h","remainingFraction":0.5}]}]}`))
		t.Cleanup(server.Close)
		prober := quota.NewAntigravityProber(quota.ProberOptions{BaseURL: server.URL})
		credential := activity.Credential{
			ID: "credential-1", AccessToken: "token", Extra: map[string]string{"projectId": "project-1"},
		}
		if _, err := prober.Probe(context.Background(), credential); err != nil {
			t.Fatalf("Probe() error = %v", err)
		}
		summary := requests.last("/v1internal:retrieveUserQuotaSummary")
		if summary.body != `{"project":"project-1"}` {
			t.Fatalf("summary body = %q, want the billing project", summary.body)
		}
		if summary.userAgent != antigravity.UserAgent() || summary.authorization != "Bearer token" {
			t.Fatalf("summary = %+v, want the fingerprint and the credential", summary)
		}
		if requests.count("/v1internal:fetchAvailableModels") != 0 {
			t.Fatal("the listing ran even though the summary answered")
		}

		// An account with no project still gets its session pools, and the
		// request stays an empty object rather than naming no project.
		requests.reset()
		fallback := httptest.NewServer(requests.handler(`{"models":{"gemini-3.8-flash":
			{"displayName":"Gemini 3.8 Flash","quotaInfo":{"remainingFraction":0.75}}}}`))
		t.Cleanup(fallback.Close)
		anonymous := quota.NewAntigravityProber(quota.ProberOptions{BaseURL: fallback.URL})
		windows, err := anonymous.Probe(context.Background(), activity.Credential{ID: "credential-2", AccessToken: "token"})
		if err != nil {
			t.Fatalf("Probe() error = %v", err)
		}
		if len(windows) != 1 || windows[0].Window != "Gem" || windows[0].UsedPercent != 25 {
			t.Fatalf("windows = %+v, want the session pool the listing reports", windows)
		}
		for _, request := range requests.all() {
			if request.body != "{}" {
				t.Fatalf("body = %q, want an empty object without a project", request.body)
			}
		}
	})

	t.Run("a refused credential is reported without a listing call", func(t *testing.T) {
		requests := &recordedProbes{}
		server := httptest.NewServer(requests.handler(`{}`, http.StatusForbidden))
		t.Cleanup(server.Close)
		prober := quota.NewAntigravityProber(quota.ProberOptions{BaseURL: server.URL})
		_, err := prober.Probe(context.Background(), activity.Credential{ID: "credential-1", AccessToken: "token"})
		if !errors.Is(err, activity.ErrProbeRejected) {
			t.Fatalf("Probe() error = %v, want %v", err, activity.ErrProbeRejected)
		}
		if requests.count("/v1internal:fetchAvailableModels") != 0 {
			t.Fatal("the listing ran with a refused credential")
		}
	})
}

type statusServer struct {
	status int
	body   string
}

// antigravitySummary is a retrieveUserQuotaSummary body naming both pools
// with their session and weekly windows.
const antigravitySummary = `{"groups":[{"buckets":[
	{"bucketId":"gemini-5h","remainingFraction":0.8,"resetTime":"2026-07-02T16:00:00Z"},
	{"bucketId":"gemini-weekly","remainingFraction":0.5,"resetTime":"2026-07-06T07:00:00Z"},
	{"bucketId":"3p-5h","remainingFraction":0.4,"resetTime":"2026-07-02T15:30:00Z"},
	{"bucketId":"3p-weekly","remainingFraction":1,"resetTime":"2026-07-06T07:00:00Z"}]}]}`

// jsonBodyHandler answers every request with one JSON body.
func jsonBodyHandler(body string, status ...int) http.HandlerFunc {
	code := http.StatusOK
	if len(status) > 0 {
		code = status[0]
	}
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_, _ = io.WriteString(w, body)
	}
}

// probeRequest is one quota call as the account saw it.
type probeRequest struct {
	path          string
	userAgent     string
	authorization string
	body          string
}

// recordedProbes answers every quota call with one body and keeps what each
// call carried.
type recordedProbes struct {
	mu       sync.Mutex
	requests []probeRequest
	counts   map[string]int
}

func (r *recordedProbes) handler(body string, status ...int) http.HandlerFunc {
	answer := jsonBodyHandler(body, status...)
	return func(w http.ResponseWriter, request *http.Request) {
		raw, _ := io.ReadAll(request.Body)
		r.mu.Lock()
		r.requests = append(r.requests, probeRequest{
			path: request.URL.Path, userAgent: request.Header.Get("User-Agent"),
			authorization: request.Header.Get("Authorization"), body: string(raw),
		})
		if r.counts == nil {
			r.counts = map[string]int{}
		}
		r.counts[request.URL.Path]++
		r.mu.Unlock()
		answer(w, request)
	}
}

func (r *recordedProbes) reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.requests = nil
	r.counts = map[string]int{}
}

func (r *recordedProbes) count(path string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.counts[path]
}

func (r *recordedProbes) all() []probeRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]probeRequest(nil), r.requests...)
}

func (r *recordedProbes) last(path string) probeRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	for index := len(r.requests) - 1; index >= 0; index-- {
		if r.requests[index].path == path {
			return r.requests[index]
		}
	}
	return probeRequest{}
}

func (s *statusServer) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(s.status)
	_, _ = w.Write([]byte(s.body))
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}
