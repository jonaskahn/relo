package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/sqlite"
)

const (
	agentBody    = "agent request body"
	providerBody = "provider request body"
	replyBody    = "provider reply body"
)

// seedCapturedRequest writes one usage row with the three messages a request
// keeps, and returns the event identifier and the stored capture identifiers.
func seedCapturedRequest(t *testing.T, harness *harness) (int64, []int64) {
	t.Helper()
	ctx := context.Background()
	eventID, err := harness.usage.AppendEvent(ctx, sqlite.UsageEvent{
		RequestID: "captured-1", Timestamp: 1_700_000_000_000, Provider: "openai",
		Model: "gpt-4o", CredentialLabel: "work", Surface: "chat-completions",
		Status: 200, DurationMs: 12, InputTokens: 5, OutputTokens: 2, Attempts: 1,
	})
	if err != nil {
		t.Fatalf("AppendEvent() error = %v", err)
	}
	store := sqlite.NewCaptureStore(harness.db)
	ordinal := 0
	agent := sqlite.Capture{Kind: sqlite.CaptureAgentRequest, Method: http.MethodPost,
		URL: "/v1/chat/completions", Headers: map[string][]string{"Authorization": {"Bearer rlo_ak_test"}},
		Body: []byte(agentBody)}
	provider := sqlite.Capture{Ordinal: &ordinal, Kind: sqlite.CaptureProviderRequest, Method: http.MethodPost,
		URL: "https://api.openai.test/v1/chat/completions", Headers: map[string][]string{"Authorization": {"Bearer sk-test"}},
		Body: []byte(providerBody)}
	reply := sqlite.Capture{Ordinal: &ordinal, Kind: sqlite.CaptureProviderResponse, Status: 200,
		Headers: map[string][]string{"Content-Type": {"application/json"}},
		Body:    []byte(replyBody), Truncated: true}
	if err := store.Append(ctx, eventID, []sqlite.Capture{agent, provider, reply}); err != nil {
		t.Fatalf("Append() error = %v", err)
	}
	manifest, err := store.Manifest(ctx, eventID)
	if err != nil {
		t.Fatalf("Manifest() error = %v", err)
	}
	ids := make([]int64, 0, len(manifest))
	for _, capture := range manifest {
		ids = append(ids, capture.ID)
	}
	return eventID, ids
}

// TestCapturesListAndStreamBodies covers the two reads the log's raw view
// makes: a manifest without bodies, and one body at a time on request.
func TestCapturesListAndStreamBodies(t *testing.T) {
	harness := newHarness(t)
	eventID, ids := seedCapturedRequest(t, harness)
	event := strconv.FormatInt(eventID, 10)

	t.Run("the manifest names every message without a body", func(t *testing.T) {
		response := harness.management(http.MethodGet, "/api/v1/activity/requests/"+event+"/captures", adminToken, nil)
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (%s)", response.Code, response.Body.String())
		}
		var body struct {
			Items []struct {
				ID        int64               `json:"id"`
				Kind      string              `json:"kind"`
				Ordinal   *int                `json:"ordinal"`
				Method    string              `json:"method"`
				URL       string              `json:"url"`
				Status    int                 `json:"status"`
				Headers   map[string][]string `json:"headers"`
				BodyBytes int64               `json:"body_bytes"`
				Truncated bool                `json:"truncated"`
			}
		}
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode the response: %v", err)
		}
		if len(body.Items) != 3 {
			t.Fatalf("items = %+v, want the three captured messages", body.Items)
		}
		if body.Items[0].Kind != sqlite.CaptureAgentRequest || body.Items[0].Ordinal != nil {
			t.Fatalf("first item = %+v, want the agent's own request first", body.Items[0])
		}
		if body.Items[0].Headers["Authorization"][0] != "Bearer rlo_ak_test" {
			t.Fatalf("headers = %v, want the client's own headers", body.Items[0].Headers)
		}
		if body.Items[0].BodyBytes != int64(len(agentBody)) {
			t.Fatalf("body_bytes = %d, want the stored size", body.Items[0].BodyBytes)
		}
		if !body.Items[2].Truncated {
			t.Fatalf("third item = %+v, want the truncated reply marked", body.Items[2])
		}
		if strings.Contains(response.Body.String(), replyBody) {
			t.Fatal("the manifest carried a body")
		}
	})

	t.Run("one body is streamed on request", func(t *testing.T) {
		path := "/api/v1/activity/requests/" + event + "/captures/" + strconv.FormatInt(ids[2], 10) + "/body"
		response := harness.management(http.MethodGet, path, adminToken, nil)
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (%s)", response.Code, response.Body.String())
		}
		if response.Body.String() != replyBody {
			t.Fatalf("body = %q, want the stored reply", response.Body.String())
		}
		if got := response.Header().Get("Cache-Control"); got != "no-store" {
			t.Fatalf("Cache-Control = %q, want no-store", got)
		}
		if got := response.Header().Get("X-Relo-Truncated"); got != "true" {
			t.Fatalf("X-Relo-Truncated = %q, want the truncation reported", got)
		}
	})

	t.Run("a capture of another request is not readable", func(t *testing.T) {
		path := "/api/v1/activity/requests/" + strconv.FormatInt(eventID+1, 10) +
			"/captures/" + strconv.FormatInt(ids[0], 10) + "/body"
		response := harness.management(http.MethodGet, path, adminToken, nil)
		if response.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404 (%s)", response.Code, response.Body.String())
		}
	})

	t.Run("an identifier that is not a number is refused", func(t *testing.T) {
		response := harness.management(http.MethodGet, "/api/v1/activity/requests/not-a-number/captures", adminToken, nil)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400 (%s)", response.Code, response.Body.String())
		}
	})
}

// TestUsageMetricsAPI covers the read and the write the usage page's picker
// makes: the choice lives in the shared startup file, and a set the overview
// cannot render is refused.
func TestUsageMetricsAPI(t *testing.T) {
	harness := newHarness(t)

	t.Run("the defaults are what a fresh install reads", func(t *testing.T) {
		var body struct {
			Metrics   []string `json:"metrics"`
			Available []string `json:"available"`
			Min       int      `json:"min"`
			Max       int      `json:"max"`
		}
		if code := harness.managementJSON(t, http.MethodGet, "/api/v1/ui/usage-metrics", "", &body); code != http.StatusOK {
			t.Fatalf("status = %d, want 200", code)
		}
		if len(body.Metrics) != 9 {
			t.Fatalf("metrics = %v, want the nine defaults", body.Metrics)
		}
		if !slices.Contains(body.Metrics, "plan_usage") {
			t.Fatalf("metrics = %v, want plan_usage beside API spend", body.Metrics)
		}
		if len(body.Available) < 20 {
			t.Fatalf("available = %v, want every metric the page may show", body.Available)
		}
		if body.Min != 6 || body.Max != 10 {
			t.Fatalf("bounds = %d..%d, want 6..10", body.Min, body.Max)
		}
	})

	t.Run("a chosen set is stored and read back", func(t *testing.T) {
		chosen := []string{"requests", "errors", "spend", "duration_avg", "attempts", "cache_read"}
		payload, err := json.Marshal(map[string]any{"metrics": chosen})
		if err != nil {
			t.Fatalf("encode the request: %v", err)
		}
		var saved struct {
			Metrics []string `json:"metrics"`
		}
		if code := harness.managementJSON(t, http.MethodPut, "/api/v1/ui/usage-metrics", string(payload), &saved); code != http.StatusOK {
			t.Fatalf("status = %d, want 200", code)
		}
		if len(saved.Metrics) != len(chosen) {
			t.Fatalf("saved = %v, want %v", saved.Metrics, chosen)
		}
		var read struct {
			Metrics []string `json:"metrics"`
		}
		if code := harness.managementJSON(t, http.MethodGet, "/api/v1/ui/usage-metrics", "", &read); code != http.StatusOK {
			t.Fatalf("status = %d, want 200", code)
		}
		for index, metric := range chosen {
			if read.Metrics[index] != metric {
				t.Fatalf("metrics = %v, want %v", read.Metrics, chosen)
			}
		}
	})

	t.Run("a set outside the window is refused", func(t *testing.T) {
		payload, err := json.Marshal(map[string]any{"metrics": []string{"requests", "errors"}})
		if err != nil {
			t.Fatalf("encode the request: %v", err)
		}
		if code := harness.managementJSON(t, http.MethodPut, "/api/v1/ui/usage-metrics", string(payload), nil); code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", code)
		}
	})

	t.Run("a metric this build does not know is refused", func(t *testing.T) {
		payload, err := json.Marshal(map[string]any{"metrics": []string{
			"requests", "errors", "spend", "attempts", "retried", "invented"}})
		if err != nil {
			t.Fatalf("encode the request: %v", err)
		}
		if code := harness.managementJSON(t, http.MethodPut, "/api/v1/ui/usage-metrics", string(payload), nil); code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", code)
		}
	})
}
