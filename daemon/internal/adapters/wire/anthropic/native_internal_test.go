package anthropic

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestRebaseNativeBodyFallsBackWithoutMessages keeps the reuse honest: only an
// object holding at least one message is a request this codec may retarget.
func TestRebaseNativeBodyFallsBackWithoutMessages(t *testing.T) {
	cases := map[string]string{
		"an empty message list": `{"messages":[]}`,
		"no message list":       `{"model":"m","system":"s"}`,
		"not an object":         `[1,2,3]`,
		"not json":              `{`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			if _, ok := rebaseNativeBody([]byte(body), "claude-sonnet-4", nil); ok {
				t.Fatalf("body = %s, want the canonical encoder", body)
			}
		})
	}
}

// TestRebaseNativeBodyAddsTheModelItWasNotGiven pins that a client which sent
// no model still gets one: the attempt runs against a model of Relo's choosing,
// so the body has to name it even though the client did not.
func TestRebaseNativeBodyAddsTheModelItWasNotGiven(t *testing.T) {
	body := `{"messages":[{"role":"user","content":[]}],"max_tokens":16}`
	rebuilt, ok := rebaseNativeBody([]byte(body), "claude-sonnet-4", nil)
	if !ok {
		t.Fatal("want the body retargeted")
	}
	if string(rebuilt) != `{"messages":[{"role":"user","content":[]}],"max_tokens":16,"model":"claude-sonnet-4"}` {
		t.Fatalf("body = %s, want the model appended", rebuilt)
	}
}

// TestInboundKeepsTheClientsOwnBody pins that the body the client sent is the
// body an attempt on the Messages wire reuses, byte for byte.
func TestInboundKeepsTheClientsOwnBody(t *testing.T) {
	body := `{"model":"claude-sonnet-4","messages":[{"role":"user","content":"hi"}]}`
	inbound := NewInbound()
	request := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	if _, err := inbound.DecodeRequest(request); err != nil {
		t.Fatalf("DecodeRequest() error = %v", err)
	}
	if string(inbound.NativeBody()) != body {
		t.Fatalf("native body = %s, want the client's own bytes", inbound.NativeBody())
	}
}

// TestRebaseNativeBodyDropsMetadataWithoutAnAccount pins that an attempt with
// no account attribution carries none, rather than the client's own.
func TestRebaseNativeBodyDropsMetadataWithoutAnAccount(t *testing.T) {
	body := `{"model":"m","metadata":{"user_id":"caller"},"messages":[{"role":"user","content":[]}]}`
	rebuilt, ok := rebaseNativeBody([]byte(body), "claude-sonnet-4", nil)
	if !ok {
		t.Fatal("want the body retargeted")
	}
	if string(rebuilt) != `{"model":"claude-sonnet-4","messages":[{"role":"user","content":[]}]}` {
		t.Fatalf("body = %s, want the caller's attribution dropped", rebuilt)
	}
}
