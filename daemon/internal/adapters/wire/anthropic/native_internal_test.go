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

// TestRebaseNativeBodyNormalizesThinkingDisplay pins the one client control
// retargeting does rewrite. Claude Code asks for thinking updates, a value the
// public Messages API refuses with a 400, so an unknown display becomes the
// nearest value this API accepts. A display it does accept is the client's own.
func TestRebaseNativeBodyNormalizesThinkingDisplay(t *testing.T) {
	cases := []struct {
		name     string
		thinking string
		want     string
	}{
		{name: "an unknown display becomes omitted", thinking: `{"type":"adaptive","display":"updates"}`,
			want: `{"type":"adaptive","display":"omitted"}`},
		{name: "summarized is kept", thinking: `{"type":"adaptive","display":"summarized"}`,
			want: `{"type":"adaptive","display":"summarized"}`},
		{name: "omitted is kept", thinking: `{"type":"adaptive","display":"omitted"}`,
			want: `{"type":"adaptive","display":"omitted"}`},
		{name: "no display is left alone", thinking: `{"type":"enabled","budget_tokens":1024}`,
			want: `{"type":"enabled","budget_tokens":1024}`},
		{name: "a thinking field that is not an object is left alone", thinking: `"adaptive"`,
			want: `"adaptive"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := `{"model":"m","thinking":` + tc.thinking + `,"messages":[{"role":"user","content":[]}]}`
			rebuilt, ok := rebaseNativeBody([]byte(body), "claude-sonnet-4", nil)
			if !ok {
				t.Fatal("want the body retargeted")
			}
			if want := `"thinking":` + tc.want; !strings.Contains(string(rebuilt), want) {
				t.Fatalf("body = %s, want %s", rebuilt, want)
			}
		})
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
