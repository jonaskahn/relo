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

	t.Run("a reused body drops a choice with no tools", func(t *testing.T) {
		body := `{"model":"m","tools":[],"tool_choice":{"type":"auto"},` +
			`"messages":[{"role":"user","content":[{"type":"text","text":"hi"}]}]}`
		encoded := encodeBody(t, canonicalRequest(true), wire.CodecOpts{
			CredentialRef: "oauth-token", AuthMethod: "oauth", NativeBody: []byte(body),
		})
		if _, present := encoded["tool_choice"]; present {
			t.Fatalf("body = %v, want no tool_choice with no tools", encoded)
		}
	})
}

// TestNativeBodyKeepsTheClientsBytes pins that retargeting rewrites only the
// model and the account attribution. Everything else — the prompt text Claude
// Code writes with angle brackets, numbers too large for a float, and the
// member order the client chose — must reach the upstream as the client sent it.
func TestNativeBodyKeepsTheClientsBytes(t *testing.T) {
	parts := `{"model":"claude-relo-claude--claude-sonnet-4","max_tokens":128000,` +
		`"system":[{"type":"text","text":"<system-reminder>keep</system-reminder>"}],` +
		`"tools":[{"name":"Run","input_schema":{"type":"object","properties":{"limit":{"const":9007199254740993}}}}],` +
		`"context_management":{"edits":[{"type":"clear_thinking_20251015","keep":"all"}]},` +
		`"thinking":{"type":"adaptive","display":"updates"},` +
		`"metadata":{"user_id":"{\"account_uuid\":\"caller\"}"},` +
		`"messages":[{"role":"user","content":[{"type":"text","text":"hi"}]}]}`

	raw := rawBody(t, newTestRequest(t, canonicalRequest(true), wire.CodecOpts{
		CredentialRef: "oauth-token", AuthMethod: "oauth",
		AccountID: "acct-9", DeviceID: "device-1", SessionID: "sess-1",
		NativeBody: []byte(parts),
	}))

	if !strings.Contains(raw, "<system-reminder>keep</system-reminder>") {
		t.Fatalf("body = %s, want the client's own text kept as bytes", raw)
	}
	if !strings.Contains(raw, "9007199254740993") {
		t.Fatalf("body = %s, want the client's own number", raw)
	}
	if strings.Contains(raw, `"display":"updates"`) {
		t.Fatalf("body = %s, want display:updates normalized away", raw)
	}
	if !strings.Contains(raw, `"type":"adaptive"`) {
		t.Fatalf("body = %s, want the client's own thinking type kept", raw)
	}
	if !strings.Contains(raw, `"model":"claude-sonnet-4"`) {
		t.Fatalf("body = %s, want the selected upstream model", raw)
	}

	order := []string{`"max_tokens"`, `"system"`, `"tools"`, `"context_management"`, `"thinking"`, `"metadata"`, `"messages"`}
	previous := -1
	for _, name := range order {
		at := strings.Index(raw, name)
		if at < 0 {
			t.Fatalf("body = %s, want %s kept", raw, name)
		}
		if at < previous {
			t.Fatalf("body = %s, want the client's member order kept", raw)
		}
		previous = at
	}
}
