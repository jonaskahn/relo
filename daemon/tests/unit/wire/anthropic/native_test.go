package anthropic_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/wire"
)

func TestNativeBodyReuse(t *testing.T) {
	native := `{"model":"claude-sonnet-5-5","max_tokens":128000,"stream":true,` +
		`"system":[{"type":"text","text":"x-anthropic-billing-header: cc_version=2.1.296.abc;"}],` +
		`"tools":[{"name":"Read","description":"read","input_schema":{"type":"object"}}],` +
		`"context_management":{"edits":[{"type":"clear_thinking_20251015"}]},` +
		`"metadata":{"user_id":"{\"session_id\":\"caller-session\"}"},` +
		`"messages":[{"role":"user","content":[{"type":"text","text":"hi","cache_control":{"type":"ephemeral"}}]}]}`

	t.Run("a routed native body keeps its wire shape", func(t *testing.T) {
		request := newTestRequest(t, canonicalRequest(true), wire.CodecOpts{
			CredentialRef: "oauth-token", AuthMethod: "oauth",
			AccountID: "acct-9", DeviceID: "device-1", SessionID: "sess-uuid-1",
			NativeBody: []byte(native),
		})
		raw := rawBody(t, request)
		if strings.Contains(raw, "You are Claude Code, Anthropic's official CLI for Claude.") {
			t.Fatalf("body rebuilt the identity block instead of preserving it")
		}
		if !strings.Contains(raw, "clear_thinking_20251015") {
			t.Fatalf("body = %s, want the native context management", raw)
		}
		var body map[string]any
		if err := json.Unmarshal([]byte(raw), &body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		tools, _ := body["tools"].([]any)
		if len(tools) != 1 || tools[0].(map[string]any)["name"] != "Read" {
			t.Fatalf("tools = %v, want the native tool names", body["tools"])
		}
		metadata, _ := body["metadata"].(map[string]any)
		var user map[string]any
		if err := json.Unmarshal([]byte(metadata["user_id"].(string)), &user); err != nil {
			t.Fatalf("metadata user_id = %v, want a json identity", metadata["user_id"])
		}
		if user["account_uuid"] != "acct-9" || user["session_id"] != "sess-uuid-1" {
			t.Fatalf("metadata user = %v, want the selected account", user)
		}
		if strings.Contains(metadata["user_id"].(string), "caller-session") {
			t.Fatalf("metadata user = %v, want no caller attribution", user)
		}
		if request.Header.Get("X-Claude-Code-Session-Id") != "sess-uuid-1" {
			t.Fatalf("session header = %q, want the metadata session", request.Header.Get("X-Claude-Code-Session-Id"))
		}
	})

	t.Run("a foreign body falls back to the canonical encoder", func(t *testing.T) {
		body := encodeBody(t, canonicalRequest(true), wire.CodecOpts{
			CredentialRef: "oauth-token", AuthMethod: "oauth",
			AccountID: "acct-9", SessionID: "sess-uuid-1",
			NativeBody: []byte(`{"chat":"not messages"}`),
		})
		system, _ := body["system"].([]any)
		if len(system) == 0 {
			t.Fatalf("system = %v, want the generated system blocks", body["system"])
		}
	})

	t.Run("an off-official attempt never forwards the native body", func(t *testing.T) {
		raw := rawBody(t, newTestRequest(t, canonicalRequest(true), wire.CodecOpts{
			BaseURL: "http://127.0.0.1:9000", CredentialRef: "oauth-token", AuthMethod: "oauth",
			AccountID: "acct-9", SessionID: "sess-uuid-1",
			NativeBody: []byte(native),
		}))
		if strings.Contains(raw, "clear_thinking_20251015") {
			t.Fatalf("body = %s, want the canonical request off the official host", raw)
		}
	})
}
