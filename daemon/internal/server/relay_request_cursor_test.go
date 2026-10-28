package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	wireformats "github.com/jonaskahn/relo/internal/adapters/wire/formats"
	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/internal/inference"
	"github.com/jonaskahn/relo/internal/routing"
)

// cursorExchangeSpec builds the smallest attempt on the Cursor agent format:
// the exchange only has to resolve its headers and its codec options.
func cursorExchangeSpec(anchor string) attemptSpec {
	request := httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	if anchor != "" {
		request.Header.Set("session-id", anchor)
	}
	return attemptSpec{
		request: request,
		candidate: routing.Candidate{
			ProviderID: "cursor", ModelID: "sonnet",
			Model: catalog.Model{ID: "sonnet", UpstreamID: "sonnet", APIFormat: catalog.FormatCursorAgent},
		},
		outcome: &requestOutcome{
			requestID: "req-1", surface: string(inference.SurfaceMessages),
			request: &inference.Request{Model: "sonnet"},
		},
		host:       catalog.Provider{ID: "cursor", Auth: catalog.AuthAPIKey, KeyHeader: catalog.KeyHeaderBearer},
		authorized: catalog.Authorization{CredentialID: "cred-a", Label: "a", Token: "token"},
	}
}

// TestBuildAttemptExchangeWiresTheCursorHandoff pins the loop that keeps a
// Cursor conversation resumable: a handoff rotates the wire generation the
// next request derives its conversation id from.
func TestBuildAttemptExchangeWiresTheCursorHandoff(t *testing.T) {
	server := &Server{opts: Options{Formats: wireformats.New(), CaptureRedactor: testRedactor{}}}
	spec := cursorExchangeSpec("session-1")
	module, ok := wireformats.New().Outbound(catalog.FormatCursorAgent)
	if !ok {
		t.Fatal("the registry has no Cursor codec")
	}
	exchange, err := server.buildAttemptExchange(spec, module, false)
	if err != nil {
		t.Fatalf("buildAttemptExchange() error = %v", err)
	}
	if exchange.OnHandoff == nil {
		t.Fatal("exchange carries no handoff callback")
	}
	before := server.cursorWireID("cursor", "cred-a", "session-1")
	exchange.OnHandoff()
	if after := server.cursorWireID("cursor", "cred-a", "session-1"); after == before {
		t.Fatal("the handoff left the next request on the same wire conversation")
	}
}

// TestBuildAttemptExchangeLeavesOtherFormatsAlone pins the other half: an
// attempt that is not a Cursor agent turn rotates nothing.
func TestBuildAttemptExchangeLeavesOtherFormatsAlone(t *testing.T) {
	server := &Server{opts: Options{Formats: wireformats.New(), CaptureRedactor: testRedactor{}}}
	spec := cursorExchangeSpec("session-1")
	spec.candidate.Model.APIFormat = catalog.FormatOpenAIChat
	module, ok := wireformats.New().Outbound(catalog.FormatOpenAIChat)
	if !ok {
		t.Fatal("the registry has no Chat codec")
	}
	exchange, err := server.buildAttemptExchange(spec, module, false)
	if err != nil {
		t.Fatalf("buildAttemptExchange() error = %v", err)
	}
	if exchange.OnHandoff != nil {
		t.Fatal("a non-Cursor attempt carries a handoff callback")
	}
}
