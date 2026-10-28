package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jonaskahn/relo/internal/routing"
)

// TestCursorWireWithoutATrackerIsInert covers a server built without the wire
// tracker: the generation stays zero and a rotation asks for nothing, which is
// what a transport with no state yet does.
func TestCursorWireWithoutATrackerIsInert(t *testing.T) {
	var wire *cursorWire
	if generation := wire.generation("cursor", "cred-a", "session-1"); generation != 0 {
		t.Fatalf("generation = %d, want zero", generation)
	}
	wire.rotate("cursor", "cred-a", "session-1")
}

// TestCursorWireGenerationSuffix pins the seed each generation adds: the first
// keeps the historical id, and later ones never collide with it.
func TestCursorWireGenerationSuffix(t *testing.T) {
	if suffix := generationSuffix(0); suffix != "" {
		t.Fatalf("generationSuffix(0) = %q, want none", suffix)
	}
	if suffix := generationSuffix(-1); suffix != "" {
		t.Fatalf("generationSuffix(-1) = %q, want none", suffix)
	}
	first, second := generationSuffix(1), generationSuffix(2)
	if first == second || first == "" || second == "" {
		t.Fatalf("suffixes = %q %q, want two distinct generations", first, second)
	}
}

// TestRotateWireFallsBackToTheCredential pins the account a rotation is filed
// under when the attempt reported only its label: the label is the key the
// next request derives its generation from.
func TestRotateWireFallsBackToTheCredential(t *testing.T) {
	server := &Server{}
	request := httptest.NewRequest(http.MethodPost, "/", nil)
	request.Header.Set("session-id", "session-1")
	driver := &sendDriver{s: server, r: request, outcome: &requestOutcome{requestID: "req-1"}}
	failed := failedAttempt{
		candidate: routing.Candidate{ProviderID: "cursor"},
		result:    Result{Credential: "label-a", RotateWire: true},
	}
	before := server.cursorWireID("cursor", "label-a", "session-1")
	driver.rotateWire(failed)
	if after := server.cursorWireID("cursor", "label-a", "session-1"); after == before {
		t.Fatal("the rotation was filed under a different account")
	}
}
