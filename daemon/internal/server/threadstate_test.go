package server

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jonaskahn/relo/internal/routing"
)

// TestLostThreadEndsTheWalk pins the routing rule: a refusal of the thread's
// state cannot be cured by another account, so the client reads it and the
// request does not walk the rest of the connection.
func TestLostThreadEndsTheWalk(t *testing.T) {
	served := New(Options{})
	driver := &sendDriver{
		s: served, r: httptest.NewRequest(http.MethodPost, "/", nil),
		outcome: &requestOutcome{requestID: "req-thread"},
	}
	err := errors.New("upstream rejected the request: 404 thread state")
	action := driver.handleAttemptFailure(failedAttempt{
		candidate: routing.Candidate{ProviderID: "anthropic", ModelID: "claude-sonnet-5-5"},
		result:    Result{Status: http.StatusNotFound, ThreadState: true, Wrote: true},
	}, err)
	if action != walkAnswered {
		t.Fatalf("action = %v, want the walk answered", action)
	}
	if !errors.Is(driver.last, err) {
		t.Fatalf("last = %v, want the refusal kept", driver.last)
	}
}
