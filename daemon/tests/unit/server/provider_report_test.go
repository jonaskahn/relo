package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/sqlite"
)

func seedProviderReport(t *testing.T, h *harness, status int) int64 {
	t.Helper()
	ctx := context.Background()
	eventID, err := h.usage.AppendEvent(ctx, sqlite.UsageEvent{
		RequestID: "provider-error", Timestamp: 1_700_000_000_000, Provider: "openai", Model: "gpt-4o",
		CredentialLabel: "private-account", Status: 503, Attempts: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	for ordinal := range 2 {
		if err := h.usage.AppendAttempt(ctx, sqlite.UsageAttempt{
			EventID: eventID, Ordinal: ordinal, Provider: "openai", Model: "gpt-4o", Status: status,
			CredentialLabel: "private-account", CredentialID: "private-id", ErrorCode: "private-code", DurationMs: 12,
		}); err != nil {
			t.Fatal(err)
		}
	}
	ordinal, success := 0, 1
	legacy := strings.Repeat("h", 8192) + strings.Repeat("t", 8192)
	captures := []sqlite.Capture{
		{Kind: sqlite.CaptureAgentRequest, Body: []byte("private-agent-prompt")},
		{Ordinal: &ordinal, Kind: sqlite.CaptureProviderRequest, Method: "POST", URL: "https://api.test/path?key=private-query", Headers: map[string][]string{"Authorization": {"Bearer private-auth"}, "X-Api-Key": {"private-key"}}, Body: []byte(legacy)},
		{Ordinal: &ordinal, Kind: sqlite.CaptureProviderResponse, Status: status, Headers: map[string][]string{"X-Api-Key": {"private-response-key"}}, Body: []byte("{\"api_key\":\"private-body-key\",\"error\":\"rate limited\"}"), Truncated: true},
		{Ordinal: &success, Kind: sqlite.CaptureProviderRequest, Body: []byte("successful-attempt")},
		{Ordinal: &success, Kind: sqlite.CaptureProviderResponse, Status: 200, Body: []byte("successful-reply")},
	}
	if err := sqlite.NewCaptureStore(h.db).Append(ctx, eventID, captures); err != nil {
		t.Fatal(err)
	}
	return eventID
}

func TestProviderReportMasksAndBoundsFailedAttempts(t *testing.T) {
	h := newHarness(t)
	eventID := seedProviderReport(t, h, 429)
	response := h.management(http.MethodGet, "/api/v1/activity/requests/"+strconv.FormatInt(eventID, 10)+"/provider-report", adminToken, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", response.Code, response.Body.String())
	}
	raw := response.Body.String()
	for _, secret := range []string{"private-", "successful-", "credential", "error_code", "agent_request", "?key="} {
		if strings.Contains(raw, secret) {
			t.Errorf("report contains excluded text %q", secret)
		}
	}
	var report map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if string(report["request_id"]) != "\"provider-error\"" || string(report["relo_version"]) != "\""+testVersion+"\"" {
		t.Fatalf("identity = %s", raw[:100])
	}
	var attempts []struct {
		Ordinal  int
		Provider string
		Model    string
		Status   int
		Request  struct {
			URL       string
			Body      string
			Truncated bool
			Headers   map[string][]string
		}
		Response struct {
			Body      string
			Truncated bool
		}
	}
	if err := json.Unmarshal(report["attempts"], &attempts); err != nil {
		t.Fatal(err)
	}
	if len(attempts) != 1 {
		t.Fatalf("attempts = %d", len(attempts))
	}
	attempt := attempts[0]
	if attempt.Status != 429 || attempt.Provider != "openai" || attempt.Model != "gpt-4o" {
		t.Fatalf("attempt = %+v", attempt)
	}
	if attempt.Request.URL != "https://api.test/path" {
		t.Fatal(attempt.Request.URL)
	}
	if attempt.Request.Body != strings.Repeat("h", 4096)+strings.Repeat("t", 4096) || !attempt.Request.Truncated {
		t.Fatal("legacy body not bounded")
	}
	if !attempt.Response.Truncated || strings.Contains(attempt.Response.Body, "private-body-key") {
		t.Fatal("response not masked or truncation lost")
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("report can be cached")
	}
}

func TestProviderReportRequiresProviderError(t *testing.T) {
	for _, status := range []int{200, 0} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			h := newHarness(t)
			eventID := seedProviderReport(t, h, status)
			path := "/api/v1/activity/requests/" + strconv.FormatInt(eventID, 10) + "/provider-report"
			if code := h.managementJSON(t, http.MethodGet, path, "", nil); code != 404 {
				t.Fatalf("status = %d", code)
			}
		})
	}
	h := newHarness(t)
	eventID, _ := seedCapturedRequest(t, h)
	for _, id := range []string{strconv.FormatInt(eventID, 10), "999999"} {
		if code := h.managementJSON(t, http.MethodGet, "/api/v1/activity/requests/"+id+"/provider-report", "", nil); code != 404 {
			t.Fatalf("%s status = %d", id, code)
		}
	}
	if code := h.managementJSON(t, http.MethodGet, "/api/v1/activity/requests/bad/provider-report", "", nil); code != 400 {
		t.Fatalf("bad id status = %d", code)
	}
}
