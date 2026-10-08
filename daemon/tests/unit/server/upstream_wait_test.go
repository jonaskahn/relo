package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/inference"
)

// wireProvider carries the two per-connection overrides the API reports,
// decoded from the wire rather than the application type.
type wireProvider struct {
	TimeoutSeconds *int     `json:"timeout_seconds"`
	RetryBackoff   [][2]int `json:"retry_backoff"`
}

// TestConnectionUpstreamWaitRoundTrip covers the two per-connection override
// fields: a PATCH stores the call wait and the retry windows, the response
// reports them, and a present null clears each back to the global value.
func TestConnectionUpstreamWaitRoundTrip(t *testing.T) {
	harness := newHarness(t)

	patched := harness.management(http.MethodPatch, "/api/v1/connections/openai", adminToken,
		strings.NewReader(`{"timeout_seconds":180,"retry_backoff":[[2,4],[4,6],[6,8]]}`))
	if patched.Code != http.StatusOK {
		t.Fatalf("patch status = %d, body = %s", patched.Code, patched.Body.String())
	}
	var provider wireProvider
	if err := json.Unmarshal(patched.Body.Bytes(), &provider); err != nil {
		t.Fatalf("decode patch: %v", err)
	}
	if provider.TimeoutSeconds == nil || *provider.TimeoutSeconds != 180 {
		t.Fatalf("timeout_seconds = %v, want 180", provider.TimeoutSeconds)
	}
	if len(provider.RetryBackoff) != 3 || provider.RetryBackoff[1] != [2]int{4, 6} {
		t.Fatalf("retry_backoff = %v, want the patched windows", provider.RetryBackoff)
	}

	read := harness.management(http.MethodGet, "/api/v1/connections/openai", adminToken, nil)
	if read.Code != http.StatusOK {
		t.Fatalf("get status = %d, body = %s", read.Code, read.Body.String())
	}
	var reloaded wireProvider
	if err := json.Unmarshal(read.Body.Bytes(), &reloaded); err != nil {
		t.Fatalf("decode get: %v", err)
	}
	if reloaded.TimeoutSeconds == nil || *reloaded.TimeoutSeconds != 180 ||
		len(reloaded.RetryBackoff) != 3 || reloaded.RetryBackoff[2] != [2]int{6, 8} {
		t.Fatalf("read back = %+v, want the stored overrides", reloaded)
	}

	cleared := harness.management(http.MethodPatch, "/api/v1/connections/openai", adminToken,
		strings.NewReader(`{"timeout_seconds":null,"retry_backoff":null}`))
	if cleared.Code != http.StatusOK {
		t.Fatalf("clear status = %d, body = %s", cleared.Code, cleared.Body.String())
	}
	var backToGlobal wireProvider
	if err := json.Unmarshal(cleared.Body.Bytes(), &backToGlobal); err != nil {
		t.Fatalf("decode clear: %v", err)
	}
	if backToGlobal.TimeoutSeconds != nil || backToGlobal.RetryBackoff != nil {
		t.Fatalf("read back = %+v, want the overrides cleared to the global values", backToGlobal)
	}
}

// TestConnectionUpstreamWaitRefusals covers a call wait outside the presets
// and retry windows the relay cannot draw from: the API answers 400 and the
// stored connection is left alone.
func TestConnectionUpstreamWaitRefusals(t *testing.T) {
	harness := newHarness(t)

	for name, body := range map[string]string{
		"retired preset": `{"timeout_seconds":30}`,
		"reversed range": `{"retry_backoff":[[2,4],[6,4],[6,8]]}`,
		"two ranges":     `{"retry_backoff":[[2,4],[4,6]]}`,
	} {
		recorder := harness.management(http.MethodPatch, "/api/v1/connections/openai", adminToken,
			strings.NewReader(body))
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("%s: status = %d, body = %s, want 400", name, recorder.Code, recorder.Body.String())
		}
	}
}

// TestClientCancelMidStreamAbandonsTheAttempt covers a client that goes away
// while its stream is still arriving: the relay abandons the single attempt
// rather than retrying it, and the answer keeps no fabricated completion.
func TestClientCancelMidStreamAbandonsTheAttempt(t *testing.T) {
	harness := newHarness(t)
	harness.upstream.fixture = "openai/chat_streaming_no_usage.txt"
	harness.upstream.sse = true
	harness.upstream.delayBy(500 * time.Millisecond)

	handler, found := harness.server.DataPlaneHandler(inference.ProtocolOpenAI)
	if !found {
		t.Fatal("the server serves no openai port")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(streamingBody())).
		WithContext(ctx)
	request.Header.Set("Authorization", "Bearer "+dataPlaneToken)
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if got := harness.upstream.requestCount(); got != 1 {
		t.Fatalf("upstream requests = %d, want the cancelled client to stop after the first", got)
	}
	if strings.Contains(recorder.Body.String(), `"finish_reason"`) {
		t.Fatalf("body = %q, want no fabricated completion", recorder.Body.String())
	}
}
