package quota_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/activity"
	"github.com/jonaskahn/relo/internal/adapters/quota"
)

const openCodeStatusBody = `{"access":{"endsAt":"2026-08-04T11:18:32.662Z","meters":{
	"fiveHour":{"resetsAt":"2026-07-12T13:30:00.662Z","limitMicroCents":"1000000","usedMicroCents":"120000"},
	"week":{"resetsAt":"2026-07-13T00:00:00.662Z","limitMicroCents":100,"usedMicroCents":8},
	"month":{"limitMicroCents":"50","usedMicroCents":"50"}
}}}`

func TestOpenCodeGoProberReadsTheThreeWindows(t *testing.T) {
	server, requests := signInEndpoint(t, http.StatusOK, openCodeStatusBody)
	prober := quota.NewOpenCodeGoProber(quota.ProberOptions{Endpoint: server.URL})
	windows, err := prober.Probe(context.Background(), activity.Credential{ID: "go", AccessToken: "oc-key"})
	if err != nil {
		t.Fatalf("Probe() error = %v", err)
	}
	request := <-requests
	if request.Header.Get("Authorization") != "Bearer oc-key" {
		t.Fatalf("authorization = %q", request.Header.Get("Authorization"))
	}
	if len(windows) != 3 {
		t.Fatalf("windows = %d, want 3", len(windows))
	}
	if windows[0].Window != activity.WindowFiveHours || windows[0].UsedPercent != 12 || windows[0].Seconds != 5*60*60 {
		t.Fatalf("session = %+v", windows[0])
	}
	if windows[1].Window != activity.WindowSevenDays || windows[1].UsedPercent != 8 {
		t.Fatalf("weekly = %+v", windows[1])
	}
	if windows[2].Window != activity.WindowThirtyDays || windows[2].UsedPercent != 100 || windows[2].Seconds != 30*24*60*60 {
		t.Fatalf("monthly = %+v", windows[2])
	}
	reset := time.Unix(windows[0].ResetAt, 0).UTC()
	if reset.Format(time.RFC3339) != "2026-07-12T13:30:00Z" {
		t.Fatalf("session reset = %s", reset)
	}
	monthly := time.Unix(windows[2].ResetAt, 0).UTC()
	if monthly.Format(time.RFC3339) != "2026-08-04T11:18:32Z" {
		t.Fatalf("monthly reset = %s", monthly)
	}
}

func TestOpenCodeGoProberUsesTheConsoleStatusURL(t *testing.T) {
	var got string
	client := &http.Client{Transport: openCodeRoundTrip(func(request *http.Request) (*http.Response, error) {
		got = request.URL.String()
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(openCodeStatusBody)),
			Header:     make(http.Header),
			Request:    request,
		}, nil
	})}
	prober := quota.NewOpenCodeGoProber(quota.ProberOptions{
		HTTPClient: client,
		BaseURL:    "https://opencode.ai/zen/go/v1",
	})
	_, err := prober.Probe(context.Background(), activity.Credential{
		ID: "go", AccessToken: "oc-key", BaseURL: "https://opencode.ai/zen/go/v1",
	})
	if err != nil {
		t.Fatalf("Probe() error = %v", err)
	}
	if got != "https://opencode.ai/console/api/go/status" {
		t.Fatalf("url = %q", got)
	}
}

func TestOpenCodeGoProberDistinguishesAMissingSubscription(t *testing.T) {
	prober := func(status int, body string) error {
		server, _ := signInEndpoint(t, status, body)
		probe := quota.NewOpenCodeGoProber(quota.ProberOptions{Endpoint: server.URL})
		_, err := probe.Probe(context.Background(), activity.Credential{ID: "go", AccessToken: "oc-key"})
		return err
	}
	if err := prober(http.StatusUnauthorized, `{"_tag":"Unauthorized"}`); !errors.Is(err, activity.ErrProbeRejected) {
		t.Fatalf("unauthorized error = %v, want a refused probe", err)
	}
	err := prober(http.StatusForbidden, `{"error":"forbidden"}`)
	if !errors.Is(err, activity.ErrProbeResponse) || errors.Is(err, activity.ErrProbeRejected) {
		t.Fatalf("forbidden error = %v, want an unreadable meter", err)
	}
	err = prober(http.StatusOK, `{"access":null}`)
	if !errors.Is(err, activity.ErrProbeResponse) || errors.Is(err, activity.ErrProbeRejected) {
		t.Fatalf("missing meters error = %v, want an unreadable meter", err)
	}
}

type openCodeRoundTrip func(*http.Request) (*http.Response, error)

func (trip openCodeRoundTrip) RoundTrip(request *http.Request) (*http.Response, error) {
	return trip(request)
}
