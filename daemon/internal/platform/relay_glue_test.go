package platform

import (
	"context"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/upstream"
	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/internal/inference"
	"github.com/jonaskahn/relo/internal/server"
)

// TestRelayExchangeCarriesTheCursorCallbacks pins the two attempt hooks the
// transport hands the relay: a refused credential to renew, and a Cursor
// handoff that moves the next request onto a fresh conversation.
func TestRelayExchangeCarriesTheCursorCallbacks(t *testing.T) {
	handoffs, refreshed := 0, 0
	exchange := server.Exchange{
		ProviderID: "cursor",
		Request:    &inference.Request{Model: "sonnet"},
		Policy:     server.Policy{SwitchOn4xx: true},
		OnHandoff:  func() { handoffs++ },
		OnUnauthorized: func(context.Context) (catalog.Authorization, error) {
			refreshed++
			return catalog.Authorization{}, nil
		},
	}
	converted := relayExchange(exchange)
	if converted.ProviderID != "cursor" || !converted.Policy.SwitchOn4xx {
		t.Fatalf("exchange = %+v, want the connection and its policy", converted)
	}
	if converted.OnHandoff == nil {
		t.Fatal("the handoff callback did not cross the glue")
	}
	converted.OnHandoff()
	if handoffs != 1 {
		t.Fatalf("handoffs = %d, want the callback forwarded once", handoffs)
	}
	if converted.OnUnauthorized == nil {
		t.Fatal("the credential renewal did not cross the glue")
	}
	if _, err := converted.OnUnauthorized(context.Background()); err != nil {
		t.Fatalf("OnUnauthorized() error = %v", err)
	}
	if refreshed != 1 {
		t.Fatalf("refreshes = %d, want the callback forwarded once", refreshed)
	}
}

// TestRelayResultCarriesTheWireRotation pins the verdict the walk reads: a
// refusal that belongs to the conversation rather than the request has to
// survive the translation, or the next attempt is replayed into the same one.
func TestRelayResultCarriesTheWireRotation(t *testing.T) {
	result := relayResult(upstream.Result{
		Status: 200, CredentialID: "cred-a", RefreshSettled: true, RotateWire: true,
		Usage: inference.UsageReport{InputTokens: 3, OutputTokens: 4},
	})
	if !result.RotateWire {
		t.Fatal("the wire rotation did not cross the glue")
	}
	if !result.RefreshSettled || result.CredentialID != "cred-a" {
		t.Fatalf("result = %+v, want the attempt's own verdicts", result)
	}
	if result.Usage.InputTokens != 3 || result.Usage.OutputTokens != 4 {
		t.Fatalf("usage = %+v, want the counted tokens", result.Usage)
	}
}

// TestRelayResultTranslatesTheCapture pins the exchange the log keeps: both
// halves cross the glue with their bodies intact.
func TestRelayResultTranslatesTheCapture(t *testing.T) {
	result := relayResult(upstream.Result{Capture: &upstream.Capture{
		Request:  &upstream.CapturedMessage{Method: "POST", URL: "https://cursor.test/run", Body: []byte("sent")},
		Response: &upstream.CapturedMessage{Status: 200, Body: []byte("answered"), Truncated: true},
	}})
	if result.Capture == nil || result.Capture.Request == nil || result.Capture.Response == nil {
		t.Fatalf("capture = %+v, want both halves", result.Capture)
	}
	if string(result.Capture.Request.Body) != "sent" || result.Capture.Request.URL != "https://cursor.test/run" {
		t.Fatalf("request capture = %+v, want what the attempt sent", result.Capture.Request)
	}
	if string(result.Capture.Response.Body) != "answered" || !result.Capture.Response.Truncated {
		t.Fatalf("response capture = %+v, want what the provider answered", result.Capture.Response)
	}
	if relayResult(upstream.Result{}).Capture != nil {
		t.Fatal("an uncaptured attempt invented a capture")
	}
}
