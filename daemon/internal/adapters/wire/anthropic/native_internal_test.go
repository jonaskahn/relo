package anthropic

import "testing"

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
