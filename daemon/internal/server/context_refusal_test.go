package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/wire/formats"
)

// TestContextRefusalReachesAnOpenAIClientAsItsOwnCode covers the code a
// coding client acts on: an overflow Relo decided itself is a request error,
// not an upstream failure, so the client can compact instead of retrying.
func TestContextRefusalReachesAnOpenAIClientAsItsOwnCode(t *testing.T) {
	server := New(Options{})
	recorder := httptest.NewRecorder()
	driver := &sendDriver{s: server, w: recorder, r: httptest.NewRequest(http.MethodPost, "/", nil)}
	cause := &contextLimitError{Model: "deepseek-v4.1-flash", Estimated: 1_300_698, Limit: 1_000_000}

	server.writeUnwritten(driver, 0, cause)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}
	var payload struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("error body is not JSON: %v", err)
	}
	if payload.Error.Code != "context_length_exceeded" {
		t.Fatalf("code = %q, want context_length_exceeded", payload.Error.Code)
	}
}

// TestContextRefusalReachesAClaudeClientAsAPromptTooLong covers the other
// half of the contract: Claude Code compacts on invalid_request_error whose
// message says the prompt is too long, and does nothing for api_error.
func TestContextRefusalReachesAClaudeClientAsAPromptTooLong(t *testing.T) {
	server := New(Options{Formats: formats.New()})
	recorder := httptest.NewRecorder()
	driver := &sendDriver{
		s: server, w: recorder, r: httptest.NewRequest(http.MethodPost, "/", nil), anthropic: true,
	}
	cause := &contextLimitError{Model: "claude-sonnet-5-5", Estimated: 1_300_698, Limit: 1_000_000}

	server.writeUnwritten(driver, 0, cause)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}
	var payload struct {
		Type  string `json:"type"`
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("error body is not JSON: %v", err)
	}
	if payload.Type != "error" || payload.Error.Type != "invalid_request_error" {
		t.Fatalf("error = %+v, want an invalid_request_error envelope", payload)
	}
	want := "prompt is too long: 1300698 tokens > 1000000 maximum"
	if payload.Error.Message != want {
		t.Fatalf("message = %q, want %q", payload.Error.Message, want)
	}
}

// TestAnthropicRefusalLeavesOtherFailuresAlone keeps the vendor's own error
// vocabulary intact for a failure Relo did not decide.
func TestAnthropicRefusalLeavesOtherFailuresAlone(t *testing.T) {
	server := New(Options{Formats: formats.New()})
	recorder := httptest.NewRecorder()

	server.writeAnthropicRefusal(recorder, http.StatusServiceUnavailable, errors.New("the provider is unreachable"))

	var payload struct {
		Error struct {
			Type string `json:"type"`
		} `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("error body is not JSON: %v", err)
	}
	if payload.Error.Type != "overloaded_error" {
		t.Fatalf("type = %q, want overloaded_error", payload.Error.Type)
	}
}

// TestUpstreamRefusalKeepsItsOwnCode keeps the local vocabulary off a failure
// the provider actually reported: an upstream 400 is not Relo's overflow.
func TestUpstreamRefusalKeepsItsOwnCode(t *testing.T) {
	server := New(Options{})
	recorder := httptest.NewRecorder()
	driver := &sendDriver{s: server, w: recorder, r: httptest.NewRequest(http.MethodPost, "/", nil)}

	server.writeUnwritten(driver, http.StatusBadRequest, &contextLimitError{
		Model: "m", Estimated: 10, Limit: 5,
	})

	var payload struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("error body is not JSON: %v", err)
	}
	if payload.Error.Code != "upstream_error" {
		t.Fatalf("code = %q, want upstream_error for a provider answer", payload.Error.Code)
	}
}
