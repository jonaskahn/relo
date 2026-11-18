package upstream

import (
	"net/http"
	"testing"

	"github.com/jonaskahn/relo/internal/inference"
)

// A conversation-state 404 names the thread, not the model, so it must not be
// recorded as model access and must not send the request to another account.
func TestThreadStateRefusalIsNotModelAccess(t *testing.T) {
	info := &inference.ErrorInfo{
		Code:    "not_found_error",
		Message: "No thread state was found for the requested `previous_message_id`.",
		Status:  http.StatusNotFound,
	}
	if modelAccessRefusal(http.StatusNotFound, info, "claude-sonnet-5-5") {
		t.Fatal("thread-state 404 reported as model access")
	}
	if !threadStateRefusal(http.StatusNotFound, info) {
		t.Fatal("thread-state 404 was not recognised")
	}
}

// A 404 that names the model is still a model refusal.
func TestModelNotFoundIsStillModelAccess(t *testing.T) {
	info := &inference.ErrorInfo{
		Code:    "not_found_error",
		Message: "model claude-sonnet-5-5 not found",
		Status:  http.StatusNotFound,
	}
	if !modelAccessRefusal(http.StatusNotFound, info, "claude-sonnet-5-5") {
		t.Fatal("model 404 was not reported as model access")
	}
	if threadStateRefusal(http.StatusNotFound, info) {
		t.Fatal("model 404 reported as a thread-state refusal")
	}
}
