package proxy_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/sqlite"
)

// capturedRow is one stored capture as a test reads it back.
type capturedRow struct {
	Kind      string
	Ordinal   *int
	Method    string
	URL       string
	Status    int
	Headers   string
	Body      []byte
	BodyBytes int64
	Truncated bool
}

// capturesOf reads what was stored beside one request, oldest first.
func capturesOf(t *testing.T, daemon daemon, requestID string) []capturedRow {
	t.Helper()
	rows, err := daemon.db.SQL().QueryContext(context.Background(),
		`SELECT c.kind, c.ordinal, c.method, c.url, c.status, c.headers, c.body, c.body_bytes, c.truncated
		FROM usage_captures c JOIN usage_events e ON e.id = c.event_id
		WHERE e.request_id = ?
		ORDER BY CASE c.kind WHEN 'agent_request' THEN 0 WHEN 'agent_response' THEN 1
			WHEN 'provider_request' THEN 2 ELSE 3 END, c.id`, requestID)
	if err != nil {
		t.Fatalf("read the captures: %v", err)
	}
	defer func() { _ = rows.Close() }()
	captures := make([]capturedRow, 0, 3)
	for rows.Next() {
		var (
			capture capturedRow
			ordinal *int64
			trunc   int
		)
		if err := rows.Scan(&capture.Kind, &ordinal, &capture.Method, &capture.URL, &capture.Status,
			&capture.Headers, &capture.Body, &capture.BodyBytes, &trunc); err != nil {
			t.Fatalf("scan a capture: %v", err)
		}
		if ordinal != nil {
			value := int(*ordinal)
			capture.Ordinal = &value
		}
		capture.Truncated = trunc != 0
		captures = append(captures, capture)
	}
	return captures
}

func findCapture(t *testing.T, captures []capturedRow, kind string) capturedRow {
	t.Helper()
	for _, capture := range captures {
		if capture.Kind == kind {
			return capture
		}
	}
	t.Fatalf("no %s capture among %+v", kind, captures)
	return capturedRow{}
}

// TestCaptureKeepsEverySideOfARequest covers the promise of the log's raw
// view: the request the agent sent, the answer it read, the request Relo sent
// to the account, and what that account answered, all kept beside one usage
// row.
func TestCaptureKeepsEverySideOfARequest(t *testing.T) {
	origin := newUpstream(t, "openai/chat_streaming.txt", true)
	daemon := startDaemon(t, origin.URL())
	token := dataPlaneToken
	origin.respond("openai/chat_nonstreaming.json", false)

	body := completeRequest()
	response := send(t, daemon.dataPlane, "/v1/chat/completions", token, body)
	defer func() { _ = response.Body.Close() }()
	read, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read the answer: %v", err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want the relayed answer", response.StatusCode)
	}

	event := daemon.lastUsageEvent(t)
	captures := capturesOf(t, daemon, event.RequestID)
	if len(captures) != 4 {
		t.Fatalf("captures = %d, want the agent's own exchange and the provider's", len(captures))
	}

	agent := findCapture(t, captures, sqlite.CaptureAgentRequest)
	if agent.Method != http.MethodPost || agent.URL != "/v1/chat/completions" {
		t.Fatalf("agent capture = %+v, want the client's own request line", agent)
	}
	if string(agent.Body) != body {
		t.Fatalf("captured agent body = %q, want %q", agent.Body, body)
	}
	// The client key is a live credential: the log keeps the scheme it was
	// presented in and nothing behind it.
	if strings.Contains(agent.Headers, token) {
		t.Fatalf("agent headers = %s, want the key the client presented hidden", agent.Headers)
	}
	if !strings.Contains(agent.Headers, "Bearer ••••") {
		t.Fatalf("agent headers = %s, want the scheme kept beside the mask", agent.Headers)
	}
	if agent.Ordinal != nil {
		t.Fatalf("agent capture ordinal = %d, want none", *agent.Ordinal)
	}

	answer := findCapture(t, captures, sqlite.CaptureAgentResponse)
	if answer.Status != http.StatusOK {
		t.Fatalf("agent answer status = %d, want the status the client read", answer.Status)
	}
	if answer.Ordinal != nil {
		t.Fatalf("agent answer ordinal = %d, want none", *answer.Ordinal)
	}
	if string(answer.Body) != string(read) {
		t.Fatalf("captured answer = %q, want the bytes the client read %q", answer.Body, read)
	}
	if !strings.Contains(answer.Headers, "application/json") {
		t.Fatalf("agent answer headers = %s, want the reply's own headers", answer.Headers)
	}

	provider := findCapture(t, captures, sqlite.CaptureProviderRequest)
	if !strings.HasPrefix(provider.URL, origin.URL()) {
		t.Fatalf("provider capture URL = %q, want the account's endpoint", provider.URL)
	}
	if strings.Contains(provider.Headers, "sk-test") {
		t.Fatalf("provider headers = %s, want the account credential hidden", provider.Headers)
	}
	if !strings.Contains(provider.Headers, "Bearer ••••") {
		t.Fatalf("provider headers = %s, want the scheme kept beside the mask", provider.Headers)
	}
	if got := origin.lastAuthorization(); got != "Bearer sk-test" {
		t.Fatalf("upstream Authorization = %q, want the credential itself still sent", got)
	}
	if provider.Ordinal == nil || *provider.Ordinal != 0 {
		t.Fatalf("provider capture ordinal = %v, want the first attempt", provider.Ordinal)
	}
	if !strings.Contains(string(provider.Body), "\"model\":\"gpt-4o\"") {
		t.Fatalf("provider body = %q, want the encoded request", provider.Body)
	}

	reply := findCapture(t, captures, sqlite.CaptureProviderResponse)
	if reply.Status != http.StatusOK {
		t.Fatalf("reply status = %d, want the upstream status", reply.Status)
	}
	if !strings.Contains(string(reply.Body), "chat.completion") {
		t.Fatalf("reply body = %q, want the upstream answer", reply.Body)
	}
	if reply.Truncated || provider.Truncated || agent.Truncated || answer.Truncated {
		t.Fatalf("a small exchange was reported truncated: %+v", captures)
	}
}

// TestCaptureKeepsAStreamedReply covers the reply that never becomes one
// body: the frames a client reads are the bytes the capture holds, and the
// client still sees them as they arrive.
func TestCaptureKeepsAStreamedReply(t *testing.T) {
	origin := newUpstream(t, "openai/chat_streaming.txt", true)
	daemon := startDaemon(t, origin.URL())
	token := dataPlaneToken

	response := send(t, daemon.dataPlane, "/v1/chat/completions", token, streamingRequest())
	defer func() { _ = response.Body.Close() }()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read the stream: %v", err)
	}
	for _, frame := range []string{"chat.completion.chunk", "Hello", "[DONE]"} {
		if !strings.Contains(string(body), frame) {
			t.Fatalf("client stream = %q, want %q", body, frame)
		}
	}

	event := daemon.lastUsageEvent(t)
	captures := capturesOf(t, daemon, event.RequestID)
	reply := findCapture(t, captures, sqlite.CaptureProviderResponse)
	for _, frame := range []string{"chat.completion.chunk", "Hello", "[DONE]"} {
		if !strings.Contains(string(reply.Body), frame) {
			t.Fatalf("captured stream = %q, want %q", reply.Body, frame)
		}
	}
	if len(reply.Body) < len(body) {
		t.Fatalf("captured %d bytes of a stream the client saw %d of", len(reply.Body), len(body))
	}
	// The other half of the exchange is what the agent read: the same frames,
	// because a streamed answer is relayed frame for frame.
	answer := findCapture(t, captures, sqlite.CaptureAgentResponse)
	for _, frame := range []string{"chat.completion.chunk", "Hello", "[DONE]"} {
		if !strings.Contains(string(answer.Body), frame) {
			t.Fatalf("captured answer = %q, want %q", answer.Body, frame)
		}
	}
	if string(answer.Body) != string(body) {
		t.Fatalf("captured answer = %q, want the frames the client read %q", answer.Body, body)
	}
}

// TestCaptureKeepsAFailedAttempt covers the exchange an operator debugs most:
// a refusal still keeps the request that was sent and the reply that refused
// it.
func TestCaptureKeepsAFailedAttempt(t *testing.T) {
	origin := newUpstream(t, "openai/chat_error.json", false)
	daemon := startDaemon(t, origin.URL())
	token := dataPlaneToken
	origin.setStatus(http.StatusTooManyRequests)

	response := send(t, daemon.dataPlane, "/v1/chat/completions", token, completeRequest())
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want the upstream refusal", response.StatusCode)
	}

	event := daemon.lastUsageEvent(t)
	captures := capturesOf(t, daemon, event.RequestID)
	provider := findCapture(t, captures, sqlite.CaptureProviderRequest)
	if !strings.Contains(string(provider.Body), "gpt-4o") {
		t.Fatalf("provider body = %q, want the request that was refused", provider.Body)
	}
	reply := findCapture(t, captures, sqlite.CaptureProviderResponse)
	if reply.Status != http.StatusTooManyRequests {
		t.Fatalf("reply status = %d, want the refusal", reply.Status)
	}
	if !strings.Contains(string(reply.Body), "Incorrect API key") {
		t.Fatalf("reply body = %q, want the upstream error", reply.Body)
	}
}

// TestCaptureKeepsARequestBodyReloCouldNotRead covers the request an operator
// most wants to see: the log keeps the bytes of a body Relo refused, with no
// provider and no attempt behind it.
func TestCaptureKeepsARequestBodyReloCouldNotRead(t *testing.T) {
	origin := newUpstream(t, "openai/chat_nonstreaming.json", false)
	daemon := startDaemon(t, origin.URL())
	token := dataPlaneToken

	broken := `{"model":"gpt-4o","messages":[`
	response := send(t, daemon.dataPlane, "/v1/chat/completions", token, broken)
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want the decoding refusal", response.StatusCode)
	}
	if origin.requestCount() != 0 {
		t.Fatal("a body Relo could not read was still sent upstream")
	}

	event := daemon.lastUsageEvent(t)
	if event.Status != http.StatusBadRequest {
		t.Fatalf("recorded status = %d, want the refusal", event.Status)
	}
	captures := capturesOf(t, daemon, event.RequestID)
	if len(captures) != 2 {
		t.Fatalf("captures = %+v, want the agent's request and the refusal it read", captures)
	}
	if string(captures[0].Body) != broken {
		t.Fatalf("captured body = %q, want the bytes the agent sent", captures[0].Body)
	}
	refusal := findCapture(t, captures, sqlite.CaptureAgentResponse)
	if refusal.Status != http.StatusBadRequest {
		t.Fatalf("refusal status = %d, want the status the agent read", refusal.Status)
	}
	if !strings.Contains(string(refusal.Body), "bad_request") {
		t.Fatalf("refusal body = %q, want the refusal the agent read", refusal.Body)
	}
}
