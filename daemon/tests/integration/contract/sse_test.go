package contract_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/inference"
)

func TestInferenceContract(t *testing.T) {
	h := newHarness(t)
	dir := fixtureDir(t)

	h.upstream.setFixture("openai/chat_streaming.txt", true)
	chat := h.dataPlaneOn(inference.ProtocolOpenAI, http.MethodPost, "/v1/chat/completions", h.keyToken,
		strings.NewReader(`{"model":"gpt-4o","stream":true,"messages":[{"role":"user","content":"hi"}]}`))
	if chat.Code != http.StatusOK {
		t.Fatalf("POST /v1/chat/completions = %d, want 200 (body %s)", chat.Code, chat.Body.String())
	}
	assertStream(t, "chat-completions", chat.Body.Bytes())
	compareFixture(t, dir, "sse-chat-completions.txt", h.normalizeStream(chat.Body.Bytes()))

	h.upstream.setFixture("openai/responses_streaming.txt", true)
	responses := h.dataPlaneOn(inference.ProtocolOpenAI, http.MethodPost, "/v1/responses", h.keyToken,
		strings.NewReader(`{"model":"`+responsesModel+`","stream":true,"input":"hi"}`))
	if responses.Code != http.StatusOK {
		t.Fatalf("POST /v1/responses = %d, want 200 (body %s)", responses.Code, responses.Body.String())
	}
	assertStream(t, "responses", responses.Body.Bytes())
	compareFixture(t, dir, "sse-responses.txt", h.normalizeStream(responses.Body.Bytes()))

	h.upstream.setFixture("anthropic/messages_streaming.txt", true)
	messages := h.dataPlaneOn(inference.ProtocolAnthropic, http.MethodPost, "/v1/messages", h.keyToken,
		strings.NewReader(`{"model":"`+anthropicModel+`","max_tokens":16,"stream":true,"messages":[{"role":"user","content":"hi"}]}`))
	if messages.Code != http.StatusOK {
		t.Fatalf("POST /v1/messages = %d, want 200 (body %s)", messages.Code, messages.Body.String())
	}
	assertStream(t, "messages", messages.Body.Bytes())
	compareFixture(t, dir, "sse-messages.txt", h.normalizeStream(messages.Body.Bytes()))

	models := h.dataPlaneOn(inference.ProtocolOpenAI, http.MethodGet, "/v1/models", "", nil)
	if models.Code != http.StatusOK {
		t.Fatalf("GET /v1/models = %d, want 200 (body %s)", models.Code, models.Body.String())
	}
	compareFixture(t, dir, "dataplane-models.json", h.normalize(models.Body.Bytes()))

	health := h.dataPlaneOn(inference.ProtocolOpenAI, http.MethodGet, "/healthz", "", nil)
	if health.Code != http.StatusOK {
		t.Fatalf("GET /healthz = %d, want 200 (body %s)", health.Code, health.Body.String())
	}
	compareFixture(t, dir, "dataplane-healthz.json", h.normalize(health.Body.Bytes()))
}

// TestEventsContract pins the event-stream framing: the unknown channel
// refusal and the head of a live stream behind the same frozen clock.
func TestEventsContract(t *testing.T) {
	h := newHarness(t)
	dir := fixtureDir(t)

	unknown := h.management(http.MethodGet, "/api/v1/events/unknown", adminToken, nil)
	if unknown.Code != http.StatusNotFound {
		t.Fatalf("GET /api/v1/events/unknown = %d, want 404 (body %s)", unknown.Code, unknown.Body.String())
	}
	compareFixture(t, dir, "events-unknown.json", h.normalize(unknown.Body.Bytes()))

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/events/status", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req = req.WithContext(ctx)
	recorder := &signalRecorder{ResponseRecorder: httptest.NewRecorder(), wrote: make(chan struct{})}
	done := make(chan struct{})
	go func() {
		defer close(done)
		h.server.Handler().ServeHTTP(recorder, req)
	}()
	select {
	case <-recorder.wrote:
	case <-time.After(2 * time.Second):
		t.Fatalf("the event stream wrote nothing within 2s")
	}
	cancel()
	<-done
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/events/status = %d, want 200 (body %s)", recorder.Code, recorder.Body.String())
	}
	head := recorder.Body.Bytes()
	if !strings.HasPrefix(string(head), "retry: 3000\n\n") {
		t.Fatalf("event stream head = %q, want the retry preamble first", head)
	}
	compareFixture(t, dir, "events-status-head.txt", head)
}

// signalRecorder reports the stream's first write, so the test stops the
// stream without ever reading the recorder while the handler writes it.
type signalRecorder struct {
	*httptest.ResponseRecorder
	wrote chan struct{}
	once  sync.Once
}

// Write reports the first byte the stream produces, then records as usual.
func (w *signalRecorder) Write(body []byte) (int, error) {
	w.once.Do(func() { close(w.wrote) })
	return w.ResponseRecorder.Write(body)
}

// assertStream refuses to pin an error envelope as a golden stream, so a
// broken relay fails loudly instead of freezing the failure into fixtures.
func assertStream(t *testing.T, surface string, body []byte) {
	t.Helper()
	if !strings.Contains(string(body), "data:") {
		t.Fatalf("%s stream carries no SSE data frames (body %s)", surface, body)
	}
}
