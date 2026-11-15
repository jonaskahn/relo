package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/inference"
)

// TestCountTokensAnswersTheMessagesSurface pins the pre-flight route: a
// Messages client counts its prompt on the same port it sends it, and the
// count is the one Relo's own estimator reports.
func TestCountTokensAnswersTheMessagesSurface(t *testing.T) {
	harness := newHarness(t)
	handler, found := harness.server.DataPlaneHandler(inference.ProtocolAnthropic)
	if !found {
		t.Fatal("the server serves no anthropic port")
	}
	body := `{"model":"claude-relo-openai-gpt-4o","messages":[{"role":"user","content":"hello there"}]}`
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, anthropicRequest(http.MethodPost, "/v1/messages/count_tokens", dataPlaneToken, body))
	if recorder.Code != http.StatusOK {
		t.Fatalf("count_tokens = %d, want 200 (body %s)", recorder.Code, recorder.Body.String())
	}
	var answer struct {
		InputTokens int `json:"input_tokens"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &answer); err != nil {
		t.Fatalf("decode the count: %v", err)
	}
	if answer.InputTokens <= 0 {
		t.Fatalf("input_tokens = %d, want a positive estimate", answer.InputTokens)
	}
}

// TestCountTokensRefusesAnUnreadableBody keeps the surface honest: a body the
// Messages decoder cannot read is a bad request, not an estimate of zero.
func TestCountTokensRefusesAnUnreadableBody(t *testing.T) {
	harness := newHarness(t)
	handler, found := harness.server.DataPlaneHandler(inference.ProtocolAnthropic)
	if !found {
		t.Fatal("the server serves no anthropic port")
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, anthropicRequest(http.MethodPost, "/v1/messages/count_tokens", dataPlaneToken, `{"messages":[]}`))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("count_tokens without a model = %d, want 400 (body %s)", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "bad_request") {
		t.Fatalf("count_tokens refusal = %s, want the bad_request code", recorder.Body.String())
	}
}

// TestCountTokensIsNotOnTheOpenAIPort pins the protocol boundary: the counter
// is a Messages route, so the OpenAI-compatible port does not answer it.
func TestCountTokensIsNotOnTheOpenAIPort(t *testing.T) {
	harness := newHarness(t)
	handler, found := harness.server.DataPlaneHandler(inference.ProtocolOpenAI)
	if !found {
		t.Fatal("the server serves no openai port")
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, anthropicRequest(http.MethodPost, "/v1/messages/count_tokens", dataPlaneToken, `{"model":"m","messages":[]}`))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("count_tokens on the openai port = %d, want 404", recorder.Code)
	}
}
