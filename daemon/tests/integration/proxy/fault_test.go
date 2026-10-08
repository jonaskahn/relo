package proxy_test

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/catalog"
)

const (
	usageWait  = 3 * time.Second
	probeLimit = 2 * time.Second
	streamPath = "/v1/chat/completions"
)

// upstreamFamily is one upstream protocol a fault is run against. The codec
// is the one the catalog selects for the format, so nothing here reaches for
// a concrete family.
type upstreamFamily struct {
	name   string
	format catalog.APIFormat
}

func upstreamFamilies() []upstreamFamily {
	return []upstreamFamily{
		{name: "openai-chat", format: catalog.FormatOpenAIChat},
		{name: "openai-responses", format: catalog.FormatOpenAIResp},
		{name: "anthropic-messages", format: catalog.FormatAnthropic},
		{name: "google-gemini", format: catalog.FormatGemini},
	}
}

// faultScenario is one upstream failure and what the client must see.
type faultScenario struct {
	name   string
	model  string
	verify func(t *testing.T, body string)
}

func faultScenarios() []faultScenario {
	return []faultScenario{
		{name: "mid-stream disconnect", model: scenarioDisconnect, verify: verifyFailureReported},
		{name: "absent sentinel", model: scenarioNoSentinel, verify: verifyRelayed},
		{name: "split UTF-8", model: scenarioRuneSplit, verify: verifyMultibyte},
		{name: "split tool JSON", model: scenarioToolJSON, verify: verifyToolCall},
		{name: "post-content 429", model: scenarioError, verify: verifyFailureReported},
		{name: "empty response", model: scenarioEmpty, verify: verifyEmpty},
		{name: "no retry after output", model: scenarioError, verify: verifyNoReplay},
	}
}

// TestStreamingFaults runs every fault against every wire family through a
// real listener: the client stream always ends, content is never truncated
// without a report, and one client request makes one upstream attempt.
func TestStreamingFaults(t *testing.T) {
	for _, family := range upstreamFamilies() {
		t.Run(family.name, func(t *testing.T) {
			upstream := newFaultUpstream(t, family.format)
			daemon := startDaemonWith(t, family.format, upstream.URL(), faultModelNames()...)
			for _, scenario := range faultScenarios() {
				t.Run(scenario.name, func(t *testing.T) {
					runFault(t, daemon, upstream, scenario)
				})
			}
			t.Run("client cancellation mid-stream", func(t *testing.T) {
				verifyCancellation(t, daemon, upstream)
			})
			t.Run("partial-response timeout", func(t *testing.T) {
				verifyTimeout(t, daemon, upstream)
			})
		})
	}
}

func runFault(t *testing.T, daemon daemon, upstream *faultUpstream, scenario faultScenario) {
	t.Helper()
	usageBefore, requestsBefore := daemon.usageCount(t), upstream.requestCount()
	response := sendStream(t, context.Background(), daemon.dataPlane, scenario.model)
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.StatusCode)
	}
	if got := response.Header.Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("Content-Type = %q, want text/event-stream", got)
	}
	body := readStream(t, response)
	if !strings.HasSuffix(strings.TrimSpace(body), "data: [DONE]") {
		t.Fatalf("body = %q, want the stream closed with the sentinel", body)
	}
	scenario.verify(t, body)
	if got := upstream.requestCount(); got != requestsBefore+1 {
		t.Fatalf("upstream requests = %d, want exactly one attempt", got-requestsBefore)
	}
	waitForUsage(t, daemon, usageBefore+1)
}

// verifyRelayed checks that every delta the upstream sent reached the client
// and that the terminal was synthesized for the missing one.
func verifyRelayed(t *testing.T, body string) {
	t.Helper()
	if !strings.Contains(body, "Hello") || !strings.Contains(body, "world") {
		t.Fatalf("body = %q, want every delta relayed", body)
	}
	if !strings.Contains(body, `finish_reason":"stop"`) {
		t.Fatalf("body = %q, want a synthesized stop terminal", body)
	}
}

// verifyFailureReported checks that a stream that died mid-flight is
// reported instead of looking complete.
func verifyFailureReported(t *testing.T, body string) {
	t.Helper()
	if !strings.Contains(body, "Hello") {
		t.Fatalf("body = %q, want the content that arrived first", body)
	}
	if !strings.Contains(body, "error") {
		t.Fatalf("body = %q, want the failure reported", body)
	}
	if strings.Contains(body, `"finish_reason":"`) {
		t.Fatalf("body = %q, want no completion terminal for a failed stream", body)
	}
}

func verifyMultibyte(t *testing.T, body string) {
	t.Helper()
	if !strings.Contains(body, "Xin chào") {
		t.Fatalf("body = %q, want the multi-byte character intact", body)
	}
}

func verifyToolCall(t *testing.T, body string) {
	t.Helper()
	arguments := clientToolArguments(t, body)
	if !json.Valid([]byte(arguments)) {
		t.Fatalf("tool arguments = %q, want valid JSON", arguments)
	}
	if !strings.Contains(arguments, "Hanoi") {
		t.Fatalf("tool arguments = %q, want the argument fragments", arguments)
	}
}

func verifyEmpty(t *testing.T, body string) {
	t.Helper()
	if !strings.Contains(body, "finish_reason") {
		t.Fatalf("body = %q, want a terminal even for an empty response", body)
	}
	if strings.Contains(body, "Hello") {
		t.Fatalf("body = %q, want no content", body)
	}
}

// verifyNoReplay checks that content which already reached the client is
// neither sent again nor retried after the stream failed.
func verifyNoReplay(t *testing.T, body string) {
	t.Helper()
	if strings.Count(body, "Hello") != 1 {
		t.Fatalf("body = %q, want the delivered content exactly once", body)
	}
	if !strings.Contains(body, "error") {
		t.Fatalf("body = %q, want the failure reported", body)
	}
}

// verifyCancellation stops reading mid-stream and proves the daemon keeps
// serving and still records the cancelled request.
func verifyCancellation(t *testing.T, daemon daemon, upstream *faultUpstream) {
	t.Helper()
	usageBefore := daemon.usageCount(t)
	ctx, cancel := context.WithCancel(context.Background())
	response := sendStream(t, ctx, daemon.dataPlane, scenarioStall)
	defer func() { _ = response.Body.Close() }()
	if _, err := bufio.NewReader(response.Body).ReadString('\n'); err != nil {
		t.Fatalf("read first frame: %v", err)
	}
	cancel()
	waitForUsage(t, daemon, usageBefore+1)
	probe := sendStream(t, context.Background(), daemon.dataPlane, scenarioComplete)
	defer func() { _ = probe.Body.Close() }()
	if probe.StatusCode != http.StatusOK || !strings.Contains(readStream(t, probe), "world") {
		t.Fatalf("status = %d, want the daemon to keep serving", probe.StatusCode)
	}
}

// verifyTimeout lets the client's own deadline fire while the upstream
// stalls, then proves the daemon survives the abandoned stream.
func verifyTimeout(t *testing.T, daemon daemon, upstream *faultUpstream) {
	t.Helper()
	usageBefore := daemon.usageCount(t)
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	response := sendStream(t, ctx, daemon.dataPlane, scenarioStall)
	defer func() { _ = response.Body.Close() }()
	if _, err := bufio.NewReader(response.Body).ReadString('\n'); err != nil {
		t.Fatalf("read first frame: %v", err)
	}
	if _, err := io.ReadAll(response.Body); err == nil {
		t.Fatal("the stalled stream ended without an error, want the deadline to fire")
	}
	waitForUsage(t, daemon, usageBefore+1)
	probe := sendStream(t, context.Background(), daemon.dataPlane, scenarioComplete)
	defer func() { _ = probe.Body.Close() }()
	if probe.StatusCode != http.StatusOK || !strings.Contains(readStream(t, probe), "world") {
		t.Fatalf("status = %d, want the daemon to keep serving after the timeout", probe.StatusCode)
	}
}

// clientToolArguments folds the tool call fragments the client received
// into the arguments a client would reassemble.
func clientToolArguments(t *testing.T, body string) string {
	t.Helper()
	var builder strings.Builder
	for _, line := range strings.Split(body, "\n") {
		data, found := strings.CutPrefix(line, "data: ")
		if !found || data == "[DONE]" {
			continue
		}
		for _, fragment := range deltaArguments(data) {
			builder.WriteString(fragment)
		}
	}
	return builder.String()
}

func deltaArguments(data string) []string {
	var chunk struct {
		Choices []struct {
			Delta struct {
				ToolCalls []struct {
					Function struct {
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"delta"`
		} `json:"choices"`
	}
	if err := json.Unmarshal([]byte(data), &chunk); err != nil {
		return nil
	}
	var fragments []string
	for _, choice := range chunk.Choices {
		for _, call := range choice.Delta.ToolCalls {
			fragments = append(fragments, call.Function.Arguments)
		}
	}
	return fragments
}

func sendStream(t *testing.T, ctx context.Context, addr, model string) *http.Response {
	t.Helper()
	body := `{"model":"` + model + `","stream":true,"messages":[{"role":"user","content":"hello"}]}`
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+addr+streamPath, strings.NewReader(body))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+dataPlaneToken)
	client := &http.Client{Timeout: probeLimit}
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	return response
}

func readStream(t *testing.T, response *http.Response) string {
	t.Helper()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read stream: %v", err)
	}
	return string(body)
}

func waitForUsage(t *testing.T, daemon daemon, want int) {
	t.Helper()
	deadline := time.Now().Add(usageWait)
	for time.Now().Before(deadline) {
		if daemon.usageCount(t) >= want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("usage rows = %d, want %d recorded", daemon.usageCount(t), want)
}
