package anthropic_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/inference"

	"github.com/jonaskahn/relo/internal/adapters/wire"
	"github.com/jonaskahn/relo/internal/adapters/wire/anthropic"
	"github.com/jonaskahn/relo/internal/adapters/wire/sse"
	"github.com/jonaskahn/relo/tests/testkit"
)

const (
	apiVersion  = "2023-06-01"
	thinkingSig = "sig-abc-123"
)

func TestEncodeRequest(t *testing.T) {
	t.Run("the system prompt is a top-level field and max_tokens is set", func(t *testing.T) {
		body := encodeBody(t, canonicalRequest(true), wire.CodecOpts{CredentialRef: "sk-ant"})
		if body["model"] != "claude-sonnet-4" || body["max_tokens"] != float64(256) || body["stream"] != true {
			t.Fatalf("body = %v, want the request options", body)
		}
		if body["system"] != "be brief" {
			t.Fatalf("system = %v, want the system prompt", body["system"])
		}
		messages := body["messages"].([]any)
		if len(messages) != 1 || messages[0].(map[string]any)["content"].([]any)[0].(map[string]any)["text"] != "hello" {
			t.Fatalf("messages = %v, want the system prompt kept out of the turns", messages)
		}
	})

	t.Run("a missing max_tokens falls back to the default", func(t *testing.T) {
		request := canonicalRequest(false)
		request.MaxTokens = 0
		if body := encodeBody(t, request, wire.CodecOpts{CredentialRef: "sk-ant"}); body["max_tokens"] != float64(4096) {
			t.Fatalf("max_tokens = %v, want the default", body["max_tokens"])
		}
	})

	t.Run("API-key credential: no beta header, no prefix", func(t *testing.T) {
		request := newTestRequest(t, canonicalRequest(true), wire.CodecOpts{CredentialRef: "sk-ant"})
		if got := request.Header.Get("x-api-key"); got != "sk-ant" {
			t.Fatalf("x-api-key = %q, want the credential", got)
		}
		if got := request.Header.Get("anthropic-version"); got != apiVersion {
			t.Fatalf("anthropic-version = %q, want %q", got, apiVersion)
		}
		if got := request.Header.Get("anthropic-beta"); got != "" {
			t.Fatalf("anthropic-beta = %q, want none", got)
		}
		if got := request.Header.Get("Authorization"); got != "" {
			t.Fatalf("Authorization = %q, want none", got)
		}
		if names := encodedToolNames(t, request); names[0] != "get_weather" {
			t.Fatalf("tools = %v, want unprefixed names", names)
		}
	})

	t.Run("OAuth beta header present once, with the tool prefix", func(t *testing.T) {
		request := newTestRequest(t, canonicalRequest(true), wire.CodecOpts{CredentialRef: "oauth-token", AuthMethod: "oauth"})
		if got := request.Header.Get("anthropic-beta"); got != anthropic.ClaudeCodeBeta {
			t.Fatalf("anthropic-beta = %q, want %q", got, anthropic.ClaudeCodeBeta)
		}
		if got := request.Header.Values("anthropic-beta"); len(got) != 1 {
			t.Fatalf("anthropic-beta = %v, want exactly one value", got)
		}
		if got := request.Header.Get("Authorization"); got != "Bearer oauth-token" {
			t.Fatalf("Authorization = %q, want the bearer token", got)
		}
		if got := request.Header.Get("x-api-key"); got != "" {
			t.Fatalf("x-api-key = %q, want none in OAuth mode", got)
		}
		if names := encodedToolNames(t, request); names[0] != "custom_get_weather" {
			t.Fatalf("tools = %v, want the OAuth prefix", names)
		}
	})

	t.Run("an OAuth request leads the system with the Claude Code identity", func(t *testing.T) {
		body := encodeBody(t, canonicalRequest(false), wire.CodecOpts{CredentialRef: "oauth-token", AuthMethod: "oauth"})
		blocks, _ := body["system"].([]any)
		if len(blocks) != 2 {
			t.Fatalf("system = %v, want the identity then the prompt", body["system"])
		}
		first := blocks[0].(map[string]any)
		text, _ := first["text"].(string)
		if first["type"] != "text" || !strings.HasPrefix(text, "x-anthropic-billing-header:") ||
			!strings.Contains(text, "You are Claude Code, Anthropic's official CLI for Claude.") {
			t.Fatalf("first system block = %v, want the billing line and the Claude Code identity", first)
		}
		if blocks[1].(map[string]any)["text"] != "be brief" {
			t.Fatalf("second system block = %v, want the request prompt", blocks[1])
		}
	})

	t.Run("an OAuth request that already names Claude Code is not prefixed twice", func(t *testing.T) {
		request := canonicalRequest(false)
		request.Messages[0].Content[0].Text = "You are Claude Code, Anthropic's official CLI for Claude.\n\nbe brief"
		body := encodeBody(t, request, wire.CodecOpts{CredentialRef: "oauth-token", AuthMethod: "oauth"})
		blocks, _ := body["system"].([]any)
		if len(blocks) != 1 {
			t.Fatalf("system = %v, want a single block", body["system"])
		}
		text, _ := blocks[0].(map[string]any)["text"].(string)
		if strings.Count(text, "You are Claude Code, Anthropic's official CLI for Claude.") != 1 ||
			!strings.Contains(text, request.Messages[0].Content[0].Text) {
			t.Fatalf("system = %v, want the prompt once, under the billing line", blocks[0])
		}
	})

	t.Run("an API-key request keeps the system prompt as a string", func(t *testing.T) {
		body := encodeBody(t, canonicalRequest(false), wire.CodecOpts{CredentialRef: "sk-ant"})
		if body["system"] != "be brief" {
			t.Fatalf("system = %v, want the string form", body["system"])
		}
	})

	t.Run("builtin tools keep their names in OAuth mode", func(t *testing.T) {
		request := canonicalRequest(true)
		request.Tools = []inference.Tool{{Name: "web_search"}, {Name: "custom_own"}}
		names := encodedToolNames(t, newTestRequest(t, request, wire.CodecOpts{CredentialRef: "oauth-token", AuthMethod: "oauth"}))
		if names[0] != "web_search" || names[1] != "custom_own" {
			t.Fatalf("tools = %v, want the builtins and already prefixed names unchanged", names)
		}
	})

	t.Run("multiple tool results batch into one user turn", func(t *testing.T) {
		request := canonicalRequest(false)
		request.Messages = append(request.Messages, toolResult("call_1", "31C"), toolResult("call_2", "sunny"))
		messages := encodeBody(t, request, wire.CodecOpts{CredentialRef: "sk-ant"})["messages"].([]any)
		results := messages[len(messages)-1].(map[string]any)
		if results["role"] != inference.RoleUser || len(results["content"].([]any)) != 2 {
			t.Fatalf("last turn = %v, want one user turn with both results", results)
		}
		if results["content"].([]any)[0].(map[string]any)["type"] != "tool_result" {
			t.Fatalf("turn = %v, want tool result blocks", results)
		}
	})

	t.Run("thinking effort becomes a budget inside the limit", func(t *testing.T) {
		request := canonicalRequest(false)
		request.Reasoning = &inference.ReasoningConfig{Effort: "high"}
		thinking := encodeBody(t, request, wire.CodecOpts{CredentialRef: "sk-ant"})["thinking"].(map[string]any)
		if thinking["type"] != "enabled" || thinking["budget_tokens"] != float64(128) {
			t.Fatalf("thinking = %v, want the budget capped by max_tokens", thinking)
		}
	})

	t.Run("an empty text part is left out", func(t *testing.T) {
		request := canonicalRequest(false)
		request.Messages = append(request.Messages, inference.Message{
			Role: inference.RoleAssistant,
			Content: []inference.ContentPart{
				{Type: inference.ContentTypeThinking, Signature: "sig-1"},
				{Type: inference.ContentTypeText},
				{Type: inference.ContentTypeText, Text: "visible"},
			},
		})
		blocks := encodeBody(t, request, wire.CodecOpts{CredentialRef: "sk-ant"})["messages"].([]any)
		assistant := blocks[len(blocks)-2].(map[string]any)["content"].([]any)
		if len(assistant) != 2 || assistant[0].(map[string]any)["signature"] != "sig-1" || assistant[1].(map[string]any)["text"] != "visible" {
			t.Fatalf("assistant = %v, want the thinking block and the visible sentence", assistant)
		}
	})

	t.Run("thinking parts keep their signature and tool calls their input", func(t *testing.T) {
		request := canonicalRequest(false)
		request.Messages = append(request.Messages, inference.Message{
			Role: inference.RoleAssistant,
			Content: []inference.ContentPart{
				{Type: inference.ContentTypeThinking, Text: "weighing", Signature: "sig-1"},
			},
			ToolCalls: []inference.ToolCall{{ID: "toolu_1", Name: "get_weather", Arguments: `{"city":"Hanoi"}`}},
		})
		messages := encodeBody(t, request, wire.CodecOpts{CredentialRef: "sk-ant"})["messages"].([]any)
		assistant := messages[len(messages)-2].(map[string]any)["content"].([]any)
		if assistant[0].(map[string]any)["signature"] != "sig-1" {
			t.Fatalf("assistant = %v, want the thinking signature", assistant)
		}
		input := assistant[1].(map[string]any)["input"].(map[string]any)
		if input["city"] != "Hanoi" {
			t.Fatalf("input = %v, want the decoded tool arguments", input)
		}
	})

	t.Run("an unsigned thinking part is left out", func(t *testing.T) {
		request := canonicalRequest(false)
		request.Messages = append(request.Messages, inference.Message{
			Role: inference.RoleAssistant,
			Content: []inference.ContentPart{
				{Type: inference.ContentTypeThinking, Text: "weighing"},
			},
			ToolCalls: []inference.ToolCall{{ID: "toolu_1", Name: "get_weather", Arguments: `{"city":"Hanoi"}`}},
		})
		messages := encodeBody(t, request, wire.CodecOpts{CredentialRef: "sk-ant"})["messages"].([]any)
		assistant := messages[len(messages)-2].(map[string]any)["content"].([]any)
		if len(assistant) != 1 || assistant[0].(map[string]any)["type"] != "tool_use" {
			t.Fatalf("assistant = %v, want the thinking block dropped and the call kept", assistant)
		}
	})

	t.Run("an unsigned thinking part leaves a text-only turn with no blocks", func(t *testing.T) {
		request := canonicalRequest(false)
		request.Messages = append(request.Messages, inference.Message{
			Role:    inference.RoleAssistant,
			Content: []inference.ContentPart{{Type: inference.ContentTypeThinking, Text: "weighing"}},
		})
		messages := encodeBody(t, request, wire.CodecOpts{CredentialRef: "sk-ant"})["messages"].([]any)
		for _, message := range messages {
			for _, block := range message.(map[string]any)["content"].([]any) {
				if block.(map[string]any)["type"] == "thinking" {
					t.Fatalf("messages = %v, want no thinking block", messages)
				}
			}
		}
	})

	t.Run("unparseable tool arguments become an empty object", func(t *testing.T) {
		request := canonicalRequest(false)
		request.Messages = append(request.Messages, inference.Message{
			Role:      inference.RoleAssistant,
			ToolCalls: []inference.ToolCall{{ID: "toolu_1", Name: "get_weather", Arguments: "{not json"}},
		})
		conversation := encodeBody(t, request, wire.CodecOpts{CredentialRef: "sk-ant"})["messages"].([]any)
		input := conversation[len(conversation)-2].(map[string]any)["content"].([]any)[0].(map[string]any)["input"].(map[string]any)
		if len(input) != 0 {
			t.Fatalf("input = %v, want an empty object", input)
		}
	})

	t.Run("an image part becomes a url source", func(t *testing.T) {
		request := canonicalRequest(false)
		request.Messages = append(request.Messages, inference.Message{Role: inference.RoleUser, Content: []inference.ContentPart{
			{Type: inference.ContentTypeImage, ImageURL: "https://example.invalid/a.png"},
		}})
		source := lastMessageBlocks(t, request)[0].(map[string]any)["source"].(map[string]any)
		if source["type"] != "url" || source["url"] != "https://example.invalid/a.png" {
			t.Fatalf("source = %v, want the url source", source)
		}
	})

	t.Run("missing credential is rejected", func(t *testing.T) {
		_, err := anthropic.Module().EncodeRequest(canonicalRequest(true), wire.CodecOpts{})
		if !errors.Is(err, wire.ErrMissingCredential) {
			t.Fatalf("EncodeRequest() error = %v, want the missing credential sentinel", err)
		}
	})

	t.Run("a per-request base URL overrides the default", func(t *testing.T) {
		request := newTestRequest(t, canonicalRequest(false), wire.CodecOpts{BaseURL: "http://127.0.0.1:9000/", CredentialRef: "sk-ant"})
		if request.URL.String() != "http://127.0.0.1:9000/messages" {
			t.Fatalf("url = %s, want the override", request.URL)
		}
	})

	t.Run("an API-key request carries none of the Claude Code fingerprint", func(t *testing.T) {
		request := newTestRequest(t, canonicalRequest(true), wire.CodecOpts{CredentialRef: "sk-ant"})
		if request.URL.RawQuery != "" || request.Header.Get("User-Agent") != "" || request.Header.Get("X-Stainless-Lang") != "" {
			t.Fatalf("url = %s, user-agent = %q, stainless = %q, want a plain API-key call",
				request.URL, request.Header.Get("User-Agent"), request.Header.Get("X-Stainless-Lang"))
		}
		if strings.Contains(rawBody(t, request), "x-anthropic-billing-header") {
			t.Fatal("API-key body contains the billing header")
		}
	})
}

// TestLongContextBetaIsTheSameProfile pins that a paired 1M request and a
// natively million-token model advertise the same Claude Code beta profile.
func TestLongContextBetaIsTheSameProfile(t *testing.T) {
	opts := wire.CodecOpts{
		CredentialRef: "oauth-token", AuthMethod: "oauth", SessionAnchor: "sess-1",
		ExtraHeaders: map[string]string{"X-Api-Key": "sk-caller"},
	}

	paired := newTestRequest(t, canonicalRequest(true), opts)
	if got := paired.Header.Get("anthropic-beta"); got != anthropic.ClaudeCodeBeta {
		t.Fatalf("beta = %q, want the Claude Code profile", got)
	}

	long := newTestRequest(t, canonicalRequest(true), wire.CodecOpts{
		CredentialRef: "oauth-token", AuthMethod: "oauth", SessionAnchor: "sess-1",
		ExtraHeaders: map[string]string{"X-Api-Key": "sk-caller"},
		LongContext:  true,
	})
	if got := long.Header.Get("anthropic-beta"); got != anthropic.ClaudeCodeBeta || retiredBeta(got) {
		t.Fatalf("beta = %q, want the same profile for a paired 1M request", got)
	}

	native := newTestRequest(t, canonicalRequest(true), wire.CodecOpts{
		CredentialRef: "oauth-token", AuthMethod: "oauth", SessionAnchor: "sess-1",
		ExtraHeaders: map[string]string{"X-Api-Key": "sk-caller"},
		LongContext:  true, NativeMillion: true,
	})
	if got := native.Header.Get("anthropic-beta"); got != anthropic.ClaudeCodeBeta || retiredBeta(got) {
		t.Fatalf("beta = %q, want the same profile for a natively 1M model", got)
	}
}

func retiredBeta(value string) bool {
	return strings.Contains(value, "context-1m-2025-08-07") || strings.Contains(value, "afk-mode-2026-01-31")
}

func TestClaudeCodeOAuthFingerprint(t *testing.T) {
	opts := wire.CodecOpts{
		CredentialRef: "oauth-token", AuthMethod: "oauth", SessionAnchor: "sess-1",
		ExtraHeaders: map[string]string{"X-Api-Key": "sk-caller"},
	}
	request := newTestRequest(t, canonicalRequest(true), opts)
	if request.URL.String() != "https://api.anthropic.com/v1/messages?beta=true" {
		t.Fatalf("url = %s, want the official messages endpoint", request.URL)
	}
	if request.Header.Get("Accept") != "application/json" {
		t.Fatalf("accept = %q, want application/json while streaming", request.Header.Get("Accept"))
	}
	if request.Header.Get("Authorization") != "Bearer oauth-token" {
		t.Fatalf("authorization = %q", request.Header.Get("Authorization"))
	}
	if got := request.Header.Get("anthropic-beta"); got != anthropic.ClaudeCodeBeta || retiredBeta(got) {
		t.Fatalf("beta = %q, want the Claude Code profile", got)
	}
	if request.Header.Get("User-Agent") != anthropic.CLIUserAgent || request.Header.Get("x-app") != "cli" {
		t.Fatalf("user-agent = %q, x-app = %q", request.Header.Get("User-Agent"), request.Header.Get("x-app"))
	}
	if request.Header.Get("anthropic-dangerous-direct-browser-access") != "true" {
		t.Fatal("missing the direct browser access header")
	}
	if request.Header.Get("Connection") != "keep-alive" || request.Header.Get("Accept-Encoding") != "gzip, deflate, br, zstd" {
		t.Fatalf("connection = %q, accept-encoding = %q", request.Header.Get("Connection"), request.Header.Get("Accept-Encoding"))
	}
	if request.Header.Get("X-Claude-Code-Session-Id") != "sess-1" || request.Header.Get("x-api-key") != "sk-caller" {
		t.Fatalf("session = %q, api key = %q", request.Header.Get("X-Claude-Code-Session-Id"), request.Header.Get("x-api-key"))
	}
	if request.Header.Get("X-Stainless-Lang") != "js" || request.Header.Get("X-Stainless-Package-Version") != "0.128.0" ||
		request.Header.Get("X-Stainless-Runtime") != "node" || request.Header.Get("X-Stainless-Runtime-Version") != "v26.3.0" ||
		request.Header.Get("X-Stainless-Retry-Count") != "0" || request.Header.Get("X-Stainless-Timeout") != "600" ||
		request.Header.Get("X-Stainless-OS") != stainlessOSName(runtime.GOOS) ||
		request.Header.Get("X-Stainless-Arch") != stainlessArchName(runtime.GOARCH) {
		t.Fatalf("stainless = %v", request.Header)
	}
	body := rawBody(t, request)
	sum := sha256.Sum256([]byte("59cf53e54c78o2.1.294"))
	suffix := hex.EncodeToString(sum[:])[:3]
	if !strings.Contains(body, "cc_version=2.1.294."+suffix) || strings.Contains(body, "cch=00000") {
		t.Fatalf("body missing the patched billing header: %s", body)
	}
	again := rawBody(t, newTestRequest(t, canonicalRequest(true), opts))
	if body != again {
		t.Fatal("billing checksum changed between two encodes of the same request")
	}

	t.Run("a caller authorization is dropped and a claude-cli user agent is forwarded", func(t *testing.T) {
		forwarded := newTestRequest(t, canonicalRequest(false), wire.CodecOpts{
			CredentialRef: "oauth-token", AuthMethod: "oauth",
			ExtraHeaders: map[string]string{
				"Authorization":  "Bearer caller",
				"User-Agent":     "claude-cli/9.9.9 (external, cli)",
				"anthropic-beta": "context-1m-2025-08-07",
				"X-Custom":       "keep",
			},
		})
		if forwarded.Header.Get("Authorization") != "Bearer oauth-token" {
			t.Fatalf("authorization = %q, want the OAuth credential", forwarded.Header.Get("Authorization"))
		}
		if forwarded.Header.Get("User-Agent") != "claude-cli/9.9.9 (external, cli)" {
			t.Fatalf("user-agent = %q, want the caller claude-cli agent", forwarded.Header.Get("User-Agent"))
		}
		if forwarded.Header.Get("anthropic-beta") != anthropic.ClaudeCodeBeta || forwarded.Header.Get("X-Custom") != "keep" {
			t.Fatalf("beta = %q, custom = %q", forwarded.Header.Get("anthropic-beta"), forwarded.Header.Get("X-Custom"))
		}
	})

	t.Run("a non-official host may override the client identity", func(t *testing.T) {
		proxied := newTestRequest(t, canonicalRequest(false), wire.CodecOpts{
			BaseURL: "http://127.0.0.1:9000", CredentialRef: "oauth-token", AuthMethod: "oauth",
			ExtraHeaders: map[string]string{
				"Authorization":    "Bearer caller",
				"anthropic-beta":   "custom-beta",
				"User-Agent":       "proxy",
				"x-app":            "sdk",
				"X-Stainless-Lang": "go",
			},
		})
		if proxied.URL.String() != "http://127.0.0.1:9000/messages" {
			t.Fatalf("url = %s, want no beta query", proxied.URL)
		}
		if proxied.Header.Get("Authorization") != "Bearer oauth-token" || proxied.Header.Get("Accept") != "application/json" {
			t.Fatalf("authorization = %q, accept = %q", proxied.Header.Get("Authorization"), proxied.Header.Get("Accept"))
		}
		if proxied.Header.Get("anthropic-beta") != "custom-beta" || proxied.Header.Get("User-Agent") != "proxy" ||
			proxied.Header.Get("x-app") != "sdk" || proxied.Header.Get("X-Stainless-Lang") != "go" {
			t.Fatalf("overrides did not apply: %v", proxied.Header)
		}
	})

	t.Run("a missing session anchor falls back to the request id", func(t *testing.T) {
		named := newTestRequest(t, canonicalRequest(false), wire.CodecOpts{
			CredentialRef: "oauth-token", AuthMethod: "oauth", RequestID: "req-9",
		})
		if named.Header.Get("X-Claude-Code-Session-Id") != "req-9" {
			t.Fatalf("session = %q, want the request id", named.Header.Get("X-Claude-Code-Session-Id"))
		}
	})
}

func stainlessOSName(goos string) string {
	switch goos {
	case "darwin":
		return "MacOS"
	case "linux":
		return "Linux"
	case "windows":
		return "Windows"
	default:
		return goos
	}
}

func stainlessArchName(arch string) string {
	switch arch {
	case "amd64":
		return "x64"
	case "386":
		return "x86"
	default:
		return arch
	}
}

func rawBody(t *testing.T, request *http.Request) string {
	t.Helper()
	body, err := io.ReadAll(request.Body)
	if err != nil {
		t.Fatalf("read request body: %v", err)
	}
	return string(body)
}

func TestStreamDecoding(t *testing.T) {
	t.Run("text stream correct event sequence", func(t *testing.T) {
		events := pushStream(t, anthropic.Module().NewStreamDecoder(), "anthropic/messages_streaming.txt")
		if got := textOf(events); got != "Hello world" {
			t.Fatalf("text = %q, want the concatenated deltas", got)
		}
		if countKind(events, inference.EventTerminal) != 1 || terminalReason(events) != inference.EventReasonStop {
			t.Fatalf("events = %+v, want one stop terminal", events)
		}
	})

	t.Run("usage no double count", func(t *testing.T) {
		events := pushStream(t, anthropic.Module().NewStreamDecoder(), "anthropic/messages_streaming.txt")
		if countKind(events, inference.EventUsage) != 1 {
			t.Fatalf("usage events = %d, want one", countKind(events, inference.EventUsage))
		}
		usage := lastUsage(events)
		if usage == nil || usage.InputTokens != 11 || usage.OutputTokens != 2 {
			t.Fatalf("usage = %+v, want both sides of the report", usage)
		}
		if usage.CacheReadTokens != 3 || usage.CacheWriteTokens != 2 {
			t.Fatalf("usage = %+v, want the cache counts", usage)
		}
	})

	t.Run("thinking block signature preserved byte-for-byte", func(t *testing.T) {
		events := pushStream(t, anthropic.Module().NewStreamDecoder(), "anthropic/messages_thinking.txt")
		if got := reasoningTextOf(events); got != "Let me check" {
			t.Fatalf("reasoning = %q, want the thinking text", got)
		}
		if got := reasoningSignature(events); got != thinkingSig {
			t.Fatalf("signature = %q, want %q", got, thinkingSig)
		}
	})

	t.Run("tool call with custom_ prefix applied and stripped", func(t *testing.T) {
		events := pushStream(t, anthropic.Module().NewStreamDecoder(), "anthropic/messages_thinking.txt")
		calls := assembledCalls(events)
		if len(calls) != 1 {
			t.Fatalf("calls = %+v, want one call", calls)
		}
		if calls[0].ID != "toolu_1" || calls[0].Name != "get_weather" {
			t.Fatalf("call = %+v, want the caller's tool name", calls[0])
		}
		if calls[0].Arguments != `{"city":"Hanoi"}` {
			t.Fatalf("arguments = %q, want the streamed JSON", calls[0].Arguments)
		}
		if terminalReason(events) != inference.EventReasonToolUse {
			t.Fatalf("terminal = %q, want tool_use", terminalReason(events))
		}
	})

	t.Run("a tool call that never streams fragments keeps its input", func(t *testing.T) {
		decoder := anthropic.Module().NewStreamDecoder()
		pushStreamData(t, decoder, `{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_2","name":"lookup","input":{"q":1}}}`)
		events := pushStreamData(t, decoder, `{"type":"content_block_stop","index":0}`)
		if len(events) != 1 || events[0].ToolCall.Arguments != `{"q":1}` {
			t.Fatalf("events = %+v, want the block input as the arguments", events)
		}
	})

	t.Run("an interrupted stream is closed with a terminal", func(t *testing.T) {
		decoder := anthropic.Module().NewStreamDecoder()
		pushStreamData(t, decoder, `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"partial"}}`)
		closing, err := decoder.Finish()
		if err != nil {
			t.Fatalf("Finish() error = %v", err)
		}
		if len(closing) != 1 || closing[0].Kind != inference.EventTerminal || closing[0].Terminal.Reason != inference.EventReasonStop {
			t.Fatalf("Finish() = %+v, want a synthesized terminal", closing)
		}
		if again, err := decoder.Finish(); err != nil || len(again) != 0 {
			t.Fatalf("second Finish() = %+v, %v, want nothing", again, err)
		}
	})

	t.Run("a max_tokens stop becomes a length terminal", func(t *testing.T) {
		decoder := anthropic.Module().NewStreamDecoder()
		pushStreamData(t, decoder, `{"type":"message_delta","delta":{"stop_reason":"max_tokens"},"usage":{"output_tokens":5}}`)
		closing, err := decoder.Finish()
		if err != nil {
			t.Fatalf("Finish() error = %v", err)
		}
		if reason := terminalReason(closing); reason != inference.EventReasonLength {
			t.Fatalf("Finish() = %+v, want a length terminal", closing)
		}
		if closing[0].Kind != inference.EventUsage || closing[0].Usage.OutputTokens != 5 {
			t.Fatalf("Finish() = %+v, want the usage before the terminal", closing)
		}
	})

	t.Run("an upstream error becomes an error event", func(t *testing.T) {
		decoder := anthropic.Module().NewStreamDecoder()
		events := pushStreamData(t, decoder, `{"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}`)
		if len(events) != 1 || events[0].Kind != inference.EventError || events[0].Error.Code != "overloaded_error" {
			t.Fatalf("events = %+v, want the upstream failure", events)
		}
		closing, err := decoder.Finish()
		if err != nil {
			t.Fatalf("Finish() error = %v", err)
		}
		if closing[0].Terminal.Reason != inference.EventReasonError {
			t.Fatalf("terminal = %+v, want error", closing[0])
		}
	})

	t.Run("empty and unknown frames are skipped", func(t *testing.T) {
		decoder := anthropic.Module().NewStreamDecoder()
		for _, data := range []string{"", "   ", `{"type":"ping"}`, `{"type":"message_stop"}`, `{"type":"future_event"}`} {
			events, err := decoder.Push(wire.SSEEvent{Data: data})
			if err != nil {
				t.Fatalf("Push(%q) error = %v", data, err)
			}
			if len(events) != 0 {
				t.Fatalf("Push(%q) = %+v, want nothing", data, events)
			}
		}
	})

	t.Run("fragments for unknown blocks are ignored", func(t *testing.T) {
		decoder := anthropic.Module().NewStreamDecoder()
		frames := []string{
			`{"type":"content_block_delta","index":7,"delta":{"type":"input_json_delta","partial_json":"{}"}}`,
			`{"type":"content_block_delta","index":7,"delta":{"type":"unknown_delta"}}`,
			`{"type":"content_block_stop","index":7}`,
		}
		for _, data := range frames {
			if events := pushStreamData(t, decoder, data); len(events) != 0 {
				t.Fatalf("push(%s) = %+v, want nothing", data, events)
			}
		}
	})

	t.Run("a malformed frame is rejected", func(t *testing.T) {
		_, err := anthropic.Module().NewStreamDecoder().Push(wire.SSEEvent{Data: "{not json"})
		if !errors.Is(err, wire.ErrMalformedEvent) {
			t.Fatalf("Push() error = %v, want the malformed event sentinel", err)
		}
	})

	t.Run("the SSE event field names the frame", func(t *testing.T) {
		events, err := anthropic.Module().DecodeResponseEvent(wire.SSEEvent{
			Name: "content_block_delta",
			Data: `{"index":0,"delta":{"type":"text_delta","text":"named"}}`,
		})
		if err != nil {
			t.Fatalf("DecodeResponseEvent() error = %v", err)
		}
		if len(events) != 1 || events[0].Text != "named" {
			t.Fatalf("events = %+v, want the delta", events)
		}
	})
}

func TestDecodeResponse(t *testing.T) {
	t.Run("non-streaming response", func(t *testing.T) {
		events, err := anthropic.Module().DecodeResponse(readFixture(t, "anthropic/messages_nonstreaming.json"))
		if err != nil {
			t.Fatalf("DecodeResponse() error = %v", err)
		}
		if got := textOf(events); got != "Hi there" {
			t.Fatalf("text = %q, want the message text", got)
		}
		if got := reasoningSignature(events); got != "sig-xyz" {
			t.Fatalf("signature = %q, want the thinking signature", got)
		}
		if calls := assembledCalls(events); len(calls) != 1 || calls[0].ID != "toolu_9" || !sameJSON(calls[0].Arguments, `{"city":"Hanoi"}`) {
			t.Fatalf("calls = %+v, want the tool call with its input", calls)
		}
		if usage := lastUsage(events); usage == nil || usage.InputTokens != 20 || usage.OutputTokens != 6 || usage.CacheReadTokens != 4 {
			t.Fatalf("usage = %+v, want the reported counts", usage)
		}
		if terminalReason(events) != inference.EventReasonToolUse {
			t.Fatalf("terminal = %q, want tool_use", terminalReason(events))
		}
	})

	t.Run("a redacted thinking block keeps its data as the signature", func(t *testing.T) {
		body := []byte(`{"id":"msg_9","content":[{"type":"redacted_thinking","data":"opaque-blob"}]}`)
		events, err := anthropic.Module().DecodeResponse(body)
		if err != nil {
			t.Fatalf("DecodeResponse() error = %v", err)
		}
		if got := reasoningSignature(events); got != "opaque-blob" {
			t.Fatalf("signature = %q, want the redacted payload", got)
		}
	})

	t.Run("a message carrying an error becomes an error event", func(t *testing.T) {
		body := []byte(`{"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}`)
		events, err := anthropic.Module().DecodeResponse(body)
		if err != nil {
			t.Fatalf("DecodeResponse() error = %v", err)
		}
		if countKind(events, inference.EventError) != 1 || terminalReason(events) != inference.EventReasonError {
			t.Fatalf("events = %+v, want the failure and an error terminal", events)
		}
	})

	t.Run("a malformed message is rejected", func(t *testing.T) {
		_, err := anthropic.Module().DecodeResponse([]byte("{not json"))
		if !errors.Is(err, wire.ErrInvalidResponse) {
			t.Fatalf("DecodeResponse() error = %v, want the invalid response sentinel", err)
		}
	})
}

func TestDecodeError(t *testing.T) {
	t.Run("an upstream error body is mapped", func(t *testing.T) {
		body := []byte(`{"type":"error","error":{"type":"authentication_error","message":"invalid x-api-key"}}`)
		info := anthropic.Module().DecodeError(401, body)
		if info.Status != 401 || info.Code != "authentication_error" || info.Message != "invalid x-api-key" {
			t.Fatalf("info = %+v, want the upstream failure", info)
		}
	})

	t.Run("an error body without a type uses the default code", func(t *testing.T) {
		info := anthropic.Module().DecodeError(500, []byte(`{"error":{"message":"boom"}}`))
		if info.Code != "api_error" {
			t.Fatalf("info = %+v, want the default code", info)
		}
	})

	t.Run("a non-json error body falls back to the status text", func(t *testing.T) {
		info := anthropic.Module().DecodeError(503, []byte("service unavailable"))
		if info.Code != "Service Unavailable" || info.Status != 503 {
			t.Fatalf("info = %+v, want the status text", info)
		}
	})
}

func TestInboundDecode(t *testing.T) {
	t.Run("the client request becomes canonical", func(t *testing.T) {
		request := inboundRequest(t, readFixture(t, "anthropic/messages_request.json"))
		if request.Model != "claude-sonnet-4" || !request.Stream || request.MaxTokens != 512 {
			t.Fatalf("request = %+v, want the client options", request)
		}
		if request.Messages[0].Role != inference.RoleSystem || request.Messages[0].Content[0].Text != "be brief" {
			t.Fatalf("messages = %+v, want the system prompt first", request.Messages)
		}
		user := request.Messages[1]
		if user.Content[1].Type != inference.ContentTypeImage || !strings.HasPrefix(user.Content[1].ImageURL, "data:image/png;base64,") {
			t.Fatalf("user message = %+v, want the image as a data URL", user)
		}
		assistant := request.Messages[2]
		if assistant.Content[0].Signature != "sig-1" || assistant.ToolCalls[0].Name != "get_weather" {
			t.Fatalf("assistant message = %+v, want thinking and the tool call", assistant)
		}
		if !sameJSON(assistant.ToolCalls[0].Arguments, `{"city":"Hanoi"}`) {
			t.Fatalf("arguments = %q, want the tool input", assistant.ToolCalls[0].Arguments)
		}
		results := messagesWithRole(request.Messages, inference.RoleTool)
		if len(results) != 2 || results[0].ToolCallID != "toolu_1" || results[1].Content[0].Text != "sunny" {
			t.Fatalf("tool messages = %+v, want both results", results)
		}
		if request.Reasoning == nil || request.Reasoning.Effort != "high" {
			t.Fatalf("reasoning = %+v, want the thinking budget as effort", request.Reasoning)
		}
	})

	t.Run("content may be a plain string", func(t *testing.T) {
		body := []byte(`{"model":"claude-sonnet-4","max_tokens":8,"system":"rules","messages":[{"role":"user","content":"hello"}]}`)
		request := inboundRequest(t, body)
		if request.Messages[0].Content[0].Text != "rules" || request.Messages[1].Content[0].Text != "hello" {
			t.Fatalf("messages = %+v, want both prompts", request.Messages)
		}
	})

	t.Run("a user turn may mix text and tool results", func(t *testing.T) {
		body := []byte(`{"model":"claude-sonnet-4","max_tokens":8,"messages":[{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_1","content":"31C"},{"type":"text","text":"and now?"}]}]}`)
		request := inboundRequest(t, body)
		if len(request.Messages) != 2 || request.Messages[0].Role != inference.RoleTool || request.Messages[1].Role != inference.RoleUser {
			t.Fatalf("messages = %+v, want the result before the turn", request.Messages)
		}
	})

	t.Run("a request without a model is rejected", func(t *testing.T) {
		_, err := decodeInbound([]byte(`{"messages":[]}`))
		if !errors.Is(err, wire.ErrInvalidRequest) {
			t.Fatalf("DecodeRequest() error = %v, want the invalid request sentinel", err)
		}
	})

	t.Run("malformed content and malformed bodies are rejected", func(t *testing.T) {
		cases := []string{
			"{not json",
			`{"model":"claude-sonnet-4","messages":[{"role":"user","content":5}]}`,
			`{"model":"claude-sonnet-4","system":5,"messages":[]}`,
		}
		for _, body := range cases {
			if _, err := decodeInbound([]byte(body)); !errors.Is(err, wire.ErrInvalidRequest) {
				t.Fatalf("DecodeRequest(%s) error = %v, want the invalid request sentinel", body, err)
			}
		}
	})

	t.Run("disabled thinking is an off effort", func(t *testing.T) {
		body := []byte(`{"model":"claude-sonnet-4","max_tokens":8,"thinking":{"type":"disabled"},"messages":[]}`)
		if request := inboundRequest(t, body); request.Reasoning == nil || request.Reasoning.Effort != "none" {
			t.Fatalf("reasoning = %+v, want effort none", request.Reasoning)
		}
	})

	t.Run("adaptive thinking keeps the output effort", func(t *testing.T) {
		body := []byte(`{"model":"claude-sonnet-5","max_tokens":8,"thinking":{"type":"adaptive","display":"updates"},"output_config":{"effort":"max"},"messages":[]}`)
		if request := inboundRequest(t, body); request.Reasoning == nil || request.Reasoning.Effort != "max" {
			t.Fatalf("reasoning = %+v, want effort max", request.Reasoning)
		}
	})

	t.Run("the tool list is preserved", func(t *testing.T) {
		request := inboundRequest(t, readFixture(t, "anthropic/messages_request.json"))
		if len(request.Tools) != 1 || request.Tools[0].Name != "get_weather" || request.Tools[0].Description != "look it up" {
			t.Fatalf("tools = %+v, want the declared tool", request.Tools)
		}
	})
}

func TestInboundEncode(t *testing.T) {
	t.Run("the canonical stream becomes Messages frames", func(t *testing.T) {
		frames := encodeEvents(t, anthropic.NewInbound(), conversationEvents())
		names := frameNames(frames)
		if names[0] != "message_start" {
			t.Fatalf("frames = %v, want the message identity first", names)
		}
		if names[len(names)-1] != "message_stop" {
			t.Fatalf("frames = %v, want the stream closed", names)
		}
		if countName(frames, "content_block_delta") != 5 {
			t.Fatalf("frames = %v, want text, thinking, signature, and argument deltas", names)
		}
		if countName(frames, "content_block_stop") != countName(frames, "content_block_start") {
			t.Fatalf("frames = %v, want every block closed", names)
		}
		if !strings.Contains(frameData(t, frames, "message_delta"), `"stop_reason":"tool_use"`) {
			t.Fatalf("message_delta = %s, want the stop reason", frameData(t, frames, "message_delta"))
		}
	})

	t.Run("the thinking signature is rendered as its own delta", func(t *testing.T) {
		events := []inference.Event{
			{Kind: inference.EventReasoningDelta, Text: "weighing"},
			{Kind: inference.EventReasoningDelta, Reasoning: &inference.ReasoningConfig{Signature: "sig-1"}},
			{Kind: inference.EventTerminal, Terminal: &inference.TerminalInfo{Reason: inference.EventReasonStop}},
		}
		frames := encodeEvents(t, anthropic.NewInbound(), events)
		if !framesContain(frames, "content_block_delta", "sig-1") {
			t.Fatalf("frames = %v, want the signature delta", frameNames(frames))
		}
	})

	t.Run("the non-streaming shape carries the assembled message", func(t *testing.T) {
		payload, err := anthropic.NewInbound().EncodeResponse(conversationEvents())
		if err != nil {
			t.Fatalf("EncodeResponse() error = %v", err)
		}
		var message map[string]any
		if err := json.Unmarshal(payload, &message); err != nil {
			t.Fatalf("decode message: %v", err)
		}
		if message["type"] != "message" || message["stop_reason"] != "tool_use" {
			t.Fatalf("message = %v, want a stopped message", message)
		}
		blocks := message["content"].([]any)
		types := blockTypes(blocks)
		if !reflect.DeepEqual(types, []string{"text", "thinking", "tool_use"}) {
			t.Fatalf("content = %v, want text, thinking, then the tool call", types)
		}
		if blocks[1].(map[string]any)["signature"] != "sig-1" {
			t.Fatalf("thinking block = %v, want the signature", blocks[1])
		}
		usage := message["usage"].(map[string]any)
		if usage["input_tokens"] != float64(7) || usage["output_tokens"] != float64(3) {
			t.Fatalf("usage = %v, want the token counts", usage)
		}
	})

	t.Run("a relay failure closes the stream with an error", func(t *testing.T) {
		frames := anthropic.NewInbound().EncodeError(errors.New("upstream went away"))
		names := frameNames(frames)
		if names[len(names)-1] != "message_stop" || countName(frames, "error") != 1 {
			t.Fatalf("frames = %v, want the error and the close", names)
		}
	})

	t.Run("an error terminal reports the failure and stops", func(t *testing.T) {
		events := []inference.Event{
			{Kind: inference.EventError, Error: &inference.ErrorInfo{Code: "stream_error", Message: "gone", Status: 502}},
			{Kind: inference.EventTerminal, Terminal: &inference.TerminalInfo{Reason: inference.EventReasonError}},
		}
		frames := encodeEvents(t, anthropic.NewInbound(), events)
		names := frameNames(frames)
		if countName(frames, "message_delta") != 0 || names[len(names)-1] != "message_stop" {
			t.Fatalf("frames = %v, want the error to end the stream", names)
		}
	})

	t.Run("deltas without an open tool block are ignored", func(t *testing.T) {
		inbound := anthropic.NewInbound()
		frames, err := inbound.EncodeResponseEvent(inference.Event{Kind: inference.EventToolCallDelta, ToolCall: &inference.ToolCallDelta{Index: 0, Arguments: "{}"}})
		if err != nil {
			t.Fatalf("EncodeResponseEvent() error = %v", err)
		}
		if len(frames) != 0 {
			t.Fatalf("frames = %v, want nothing", frameNames(frames))
		}
		if ends, err := inbound.EncodeResponseEvent(inference.Event{Kind: inference.EventToolCallEnd}); err != nil || len(ends) != 0 {
			t.Fatalf("EncodeResponseEvent() = %v, %v, want nothing", frameNames(ends), err)
		}
	})

	t.Run("a failure event without detail still opens the stream", func(t *testing.T) {
		frames, err := anthropic.NewInbound().EncodeResponseEvent(inference.Event{Kind: inference.EventError})
		if err != nil {
			t.Fatalf("EncodeResponseEvent() error = %v", err)
		}
		if len(frames) != 1 || frames[0].Name != "message_start" {
			t.Fatalf("frames = %v, want the opening frame only", frameNames(frames))
		}
	})

	t.Run("an unknown canonical event is rejected", func(t *testing.T) {
		_, err := anthropic.NewInbound().EncodeResponseEvent(inference.Event{Kind: inference.EventKind(99)})
		if !errors.Is(err, wire.ErrUnknownEvent) {
			t.Fatalf("EncodeResponseEvent() error = %v, want the unknown event sentinel", err)
		}
	})
}

func TestModule(t *testing.T) {
	t.Run("the module satisfies the codec contract", func(t *testing.T) {
		var module wire.Codec = anthropic.Module()
		events, err := module.DecodeResponse(readFixture(t, "anthropic/messages_nonstreaming.json"))
		if err != nil || len(events) == 0 {
			t.Fatalf("DecodeResponse() = %d events, %v, want the decoded message", len(events), err)
		}
		if streamer, ok := module.(wire.StreamCodec); !ok || streamer.NewStreamDecoder() == nil {
			t.Fatal("the module cannot decode a stream")
		}
		if _, ok := module.(wire.ErrorDecoder); !ok {
			t.Fatal("the module cannot decode an upstream error")
		}
	})
}

func canonicalRequest(stream bool) *inference.Request {
	return &inference.Request{
		Model:     "claude-sonnet-4",
		Stream:    stream,
		MaxTokens: 256,
		Messages: []inference.Message{
			{Role: inference.RoleSystem, Content: []inference.ContentPart{{Type: inference.ContentTypeText, Text: "be brief"}}},
			{Role: inference.RoleUser, Content: []inference.ContentPart{{Type: inference.ContentTypeText, Text: "hello"}}},
		},
		Tools: []inference.Tool{{Name: "get_weather", Description: "look it up", Parameters: json.RawMessage(`{"type":"object"}`)}},
	}
}

func toolResult(callID, text string) inference.Message {
	return inference.Message{Role: inference.RoleTool, ToolCallID: callID, Content: []inference.ContentPart{{Type: inference.ContentTypeText, Text: text}}}
}

func conversationEvents() []inference.Event {
	return []inference.Event{
		{Kind: inference.EventTextDelta, Text: "Hel"},
		{Kind: inference.EventTextDelta, Text: "lo"},
		{Kind: inference.EventReasoningDelta, Text: "weighing"},
		{Kind: inference.EventReasoningDelta, Reasoning: &inference.ReasoningConfig{Signature: "sig-1"}},
		{Kind: inference.EventToolCallStart, ToolCall: &inference.ToolCallDelta{Index: 0, ID: "toolu_1", Name: "get_weather"}},
		{Kind: inference.EventToolCallDelta, ToolCall: &inference.ToolCallDelta{Index: 0, Arguments: `{"city":"Hanoi"}`}},
		{Kind: inference.EventToolCallEnd, ToolCall: &inference.ToolCallDelta{Index: 0, ID: "toolu_1", Name: "get_weather", Arguments: `{"city":"Hanoi"}`}},
		{Kind: inference.EventUsage, Usage: &inference.UsageReport{InputTokens: 7, OutputTokens: 3, CacheReadTokens: 1}},
		{Kind: inference.EventTerminal, Terminal: &inference.TerminalInfo{Reason: inference.EventReasonToolUse}},
	}
}

func encodeBody(t *testing.T, request *inference.Request, opts wire.CodecOpts) map[string]any {
	t.Helper()
	return decodeBody(t, newTestRequest(t, request, opts))
}

func newTestRequest(t *testing.T, request *inference.Request, opts wire.CodecOpts) *http.Request {
	t.Helper()
	encoded, err := anthropic.Module().EncodeRequest(request, opts)
	if err != nil {
		t.Fatalf("EncodeRequest() error = %v", err)
	}
	return encoded
}

func decodeBody(t *testing.T, request *http.Request) map[string]any {
	t.Helper()
	body, err := io.ReadAll(request.Body)
	if err != nil {
		t.Fatalf("read request body: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode request body: %v", err)
	}
	return payload
}

func encodedToolNames(t *testing.T, request *http.Request) []string {
	t.Helper()
	tools, _ := decodeBody(t, request)["tools"].([]any)
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.(map[string]any)["name"].(string))
	}
	return names
}

func lastMessageBlocks(t *testing.T, request *inference.Request) []any {
	t.Helper()
	messages := encodeBody(t, request, wire.CodecOpts{CredentialRef: "sk-ant"})["messages"].([]any)
	last := messages[len(messages)-1].(map[string]any)
	return last["content"].([]any)
}

func decodeInbound(body []byte) (*inference.Request, error) {
	request, err := http.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	return anthropic.NewInbound().DecodeRequest(request)
}

func inboundRequest(t *testing.T, body []byte) *inference.Request {
	t.Helper()
	request, err := decodeInbound(body)
	if err != nil {
		t.Fatalf("DecodeRequest() error = %v", err)
	}
	return request
}

func encodeEvents(t *testing.T, inbound wire.InboundCodec, events []inference.Event) []wire.SSEEvent {
	t.Helper()
	var frames []wire.SSEEvent
	for _, event := range events {
		encoded, err := inbound.EncodeResponseEvent(event)
		if err != nil {
			t.Fatalf("EncodeResponseEvent(%s) error = %v", event.Kind, err)
		}
		frames = append(frames, encoded...)
	}
	return frames
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	body, err := os.ReadFile(testkit.FixturePath(name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return body
}

// pushStream feeds every frame of an SSE fixture through a decoder and
// adds the events Finish() produces.
func pushStream(t *testing.T, decoder wire.StreamDecoder, fixture string) []inference.Event {
	t.Helper()
	var events []inference.Event
	reader := sse.NewReader(bytes.NewReader(readFixture(t, fixture)))
	for {
		frame, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("read fixture %s: %v", fixture, err)
		}
		events = append(events, pushStreamData(t, decoder, frame.Data)...)
	}
	closing, err := decoder.Finish()
	if err != nil {
		t.Fatalf("Finish() error = %v", err)
	}
	return append(events, closing...)
}

func pushStreamData(t *testing.T, decoder wire.StreamDecoder, data string) []inference.Event {
	t.Helper()
	events, err := decoder.Push(wire.SSEEvent{Data: data})
	if err != nil {
		t.Fatalf("Push() error = %v", err)
	}
	return events
}

func textOf(events []inference.Event) string {
	var builder strings.Builder
	for _, event := range events {
		if event.Kind == inference.EventTextDelta {
			builder.WriteString(event.Text)
		}
	}
	return builder.String()
}

func reasoningTextOf(events []inference.Event) string {
	var builder strings.Builder
	for _, event := range events {
		if event.Kind == inference.EventReasoningDelta {
			builder.WriteString(event.Text)
		}
	}
	return builder.String()
}

func reasoningSignature(events []inference.Event) string {
	for _, event := range events {
		if event.Reasoning != nil && event.Reasoning.Signature != "" {
			return event.Reasoning.Signature
		}
	}
	return ""
}

// assembledCalls folds tool call fragments into complete calls, the way a
// consumer of the canonical events has to.
func assembledCalls(events []inference.Event) []*inference.ToolCallDelta {
	calls := map[int]*inference.ToolCallDelta{}
	var order []int
	for _, event := range events {
		switch event.Kind {
		case inference.EventToolCallStart:
			calls[event.ToolCall.Index] = &inference.ToolCallDelta{Index: event.ToolCall.Index, ID: event.ToolCall.ID, Name: event.ToolCall.Name}
			order = append(order, event.ToolCall.Index)
		case inference.EventToolCallDelta:
			calls[event.ToolCall.Index].Arguments += event.ToolCall.Arguments
		case inference.EventToolCallEnd:
			calls[event.ToolCall.Index].Arguments = event.ToolCall.Arguments
		}
	}
	assembled := make([]*inference.ToolCallDelta, 0, len(order))
	for _, index := range order {
		assembled = append(assembled, calls[index])
	}
	return assembled
}

func terminalReason(events []inference.Event) string {
	for _, event := range events {
		if event.Kind == inference.EventTerminal {
			return event.Terminal.Reason
		}
	}
	return ""
}

func lastUsage(events []inference.Event) *inference.UsageReport {
	for index := len(events) - 1; index >= 0; index-- {
		if events[index].Usage != nil {
			return events[index].Usage
		}
	}
	return nil
}

func countKind(events []inference.Event, kind inference.EventKind) int {
	count := 0
	for _, event := range events {
		if event.Kind == kind {
			count++
		}
	}
	return count
}

func countName(frames []wire.SSEEvent, name string) int {
	count := 0
	for _, frame := range frames {
		if frame.Name == name {
			count++
		}
	}
	return count
}

func frameNames(frames []wire.SSEEvent) []string {
	names := make([]string, 0, len(frames))
	for _, frame := range frames {
		names = append(names, frame.Name)
	}
	return names
}

func frameData(t *testing.T, frames []wire.SSEEvent, name string) string {
	t.Helper()
	for _, frame := range frames {
		if frame.Name == name {
			return frame.Data
		}
	}
	t.Fatalf("no %s frame in %v", name, frameNames(frames))
	return ""
}

// sameJSON reports whether two documents describe the same JSON value.
func sameJSON(left, right string) bool {
	var decoded, expected any
	if json.Unmarshal([]byte(left), &decoded) != nil || json.Unmarshal([]byte(right), &expected) != nil {
		return false
	}
	return reflect.DeepEqual(decoded, expected)
}

func framesContain(frames []wire.SSEEvent, name, fragment string) bool {
	for _, frame := range frames {
		if frame.Name == name && strings.Contains(frame.Data, fragment) {
			return true
		}
	}
	return false
}

func blockTypes(blocks []any) []string {
	types := make([]string, 0, len(blocks))
	for _, block := range blocks {
		types = append(types, block.(map[string]any)["type"].(string))
	}
	return types
}

func messagesWithRole(messages []inference.Message, role string) []inference.Message {
	var found []inference.Message
	for _, message := range messages {
		if message.Role == role {
			found = append(found, message)
		}
	}
	return found
}

func TestEncodeRequestDetails(t *testing.T) {
	t.Run("an unknown effort is omitted", func(t *testing.T) {
		request := canonicalRequest(false)
		request.MaxTokens = 8192
		request.Reasoning = &inference.ReasoningConfig{Effort: "extreme"}
		if thinking := encodeBody(t, request, wire.CodecOpts{CredentialRef: "sk-ant"})["thinking"]; thinking != nil {
			t.Fatalf("thinking = %v, want the field omitted", thinking)
		}
	})

	t.Run("xhigh is a budget on an older claude and an effort on a newer one", func(t *testing.T) {
		older := canonicalRequest(false)
		older.MaxTokens = 64000
		older.Reasoning = &inference.ReasoningConfig{Effort: "xhigh"}
		thinking := encodeBody(t, older, wire.CodecOpts{CredentialRef: "sk-ant"})["thinking"].(map[string]any)
		if thinking["budget_tokens"] != float64(24576) {
			t.Fatalf("thinking = %v, want the xhigh budget", thinking)
		}
		newer := canonicalRequest(false)
		newer.Model = "claude-sonnet-5"
		newer.MaxTokens = 64000
		newer.Reasoning = &inference.ReasoningConfig{Effort: "xhigh"}
		body := encodeBody(t, newer, wire.CodecOpts{CredentialRef: "sk-ant"})
		thinking, _ = body["thinking"].(map[string]any)
		if thinking["type"] != "adaptive" || thinking["budget_tokens"] != nil {
			t.Fatalf("thinking = %v, want adaptive thinking and no token budget", body["thinking"])
		}
		effort := body["output_config"].(map[string]any)["effort"]
		if effort != "xhigh" {
			t.Fatalf("output_config = %v, want xhigh", body["output_config"])
		}
		opus := canonicalRequest(false)
		opus.Model = "claude-opus-4-7"
		opus.Reasoning = &inference.ReasoningConfig{Effort: "max"}
		if encodeBody(t, opus, wire.CodecOpts{CredentialRef: "sk-ant"})["output_config"].(map[string]any)["effort"] != "max" {
			t.Fatal("opus 4.7 did not take the effort name")
		}
	})

	t.Run("a tool call without arguments sends an empty object", func(t *testing.T) {
		request := canonicalRequest(false)
		request.Messages = append(request.Messages, inference.Message{
			Role:      inference.RoleAssistant,
			ToolCalls: []inference.ToolCall{{ID: "toolu_1", Name: "get_weather"}},
		})
		conversation := encodeBody(t, request, wire.CodecOpts{CredentialRef: "sk-ant"})["messages"].([]any)
		input := conversation[len(conversation)-2].(map[string]any)["content"].([]any)[0].(map[string]any)["input"].(map[string]any)
		if len(input) != 0 {
			t.Fatalf("input = %v, want an empty object", input)
		}
	})

	t.Run("an unsupported content part is left out", func(t *testing.T) {
		request := canonicalRequest(false)
		request.Messages = append(request.Messages, inference.Message{
			Role:    inference.RoleUser,
			Content: []inference.ContentPart{{Type: "audio"}},
		})
		messages := encodeBody(t, request, wire.CodecOpts{CredentialRef: "sk-ant"})["messages"].([]any)
		if len(messages) != 1 {
			t.Fatalf("messages = %v, want the turn with no representable part to disappear", messages)
		}
	})
}

func TestOmittedLimitHeadroom(t *testing.T) {
	maxOutput := func(value int64) *int64 { return &value }

	t.Run("an omitted limit covers the thinking budget plus the reply", func(t *testing.T) {
		request := canonicalRequest(false)
		request.Model = "claude-sonnet-5-5"
		request.MaxTokens = 0
		request.Reasoning = &inference.ReasoningConfig{Effort: "xhigh"}
		body := encodeBody(t, request, wire.CodecOpts{CredentialRef: "sk-ant", MaxOutput: maxOutput(128000)})
		if body["max_tokens"] != float64(28672) {
			t.Fatalf("max_tokens = %v, want the xhigh budget plus the reply headroom", body["max_tokens"])
		}
		if effort := body["output_config"].(map[string]any)["effort"]; effort != "xhigh" {
			t.Fatalf("effort = %v, want xhigh", effort)
		}
	})

	t.Run("a ceiling below the budget steps the effort down", func(t *testing.T) {
		request := canonicalRequest(false)
		request.Model = "claude-sonnet-5-5"
		request.MaxTokens = 0
		request.Reasoning = &inference.ReasoningConfig{Effort: "xhigh"}
		body := encodeBody(t, request, wire.CodecOpts{CredentialRef: "sk-ant", MaxOutput: maxOutput(8192)})
		if body["max_tokens"] != float64(8192) {
			t.Fatalf("max_tokens = %v, want the model ceiling", body["max_tokens"])
		}
		if effort := body["output_config"].(map[string]any)["effort"]; effort != "low" {
			t.Fatalf("effort = %v, want the highest effort the ceiling holds", effort)
		}
	})

	t.Run("an older claude carries the full budget inside the raised limit", func(t *testing.T) {
		request := canonicalRequest(false)
		request.Model = "claude-sonnet-4"
		request.MaxTokens = 0
		request.Reasoning = &inference.ReasoningConfig{Effort: "high"}
		body := encodeBody(t, request, wire.CodecOpts{CredentialRef: "sk-ant", MaxOutput: maxOutput(64000)})
		if body["max_tokens"] != float64(20480) {
			t.Fatalf("max_tokens = %v, want the high budget plus the reply headroom", body["max_tokens"])
		}
		thinking := body["thinking"].(map[string]any)
		if thinking["type"] != "enabled" || thinking["budget_tokens"] != float64(16384) {
			t.Fatalf("thinking = %v, want the full high budget", thinking)
		}
	})

	t.Run("an explicit limit is kept", func(t *testing.T) {
		request := canonicalRequest(false)
		request.Model = "claude-sonnet-5-5"
		request.MaxTokens = 999
		request.Reasoning = &inference.ReasoningConfig{Effort: "xhigh"}
		body := encodeBody(t, request, wire.CodecOpts{CredentialRef: "sk-ant", MaxOutput: maxOutput(128000)})
		if body["max_tokens"] != float64(999) {
			t.Fatalf("max_tokens = %v, want the client limit unchanged", body["max_tokens"])
		}
	})

	t.Run("an omitted limit without thinking keeps the default", func(t *testing.T) {
		request := canonicalRequest(false)
		request.Model = "claude-sonnet-5-5"
		request.MaxTokens = 0
		body := encodeBody(t, request, wire.CodecOpts{CredentialRef: "sk-ant", MaxOutput: maxOutput(128000)})
		if body["max_tokens"] != float64(4096) {
			t.Fatalf("max_tokens = %v, want the default", body["max_tokens"])
		}
	})
}

func TestStreamDecodingDetails(t *testing.T) {
	t.Run("empty deltas are ignored", func(t *testing.T) {
		decoder := anthropic.Module().NewStreamDecoder()
		frames := []string{
			`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":""}}`,
			`{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":""}}`,
			`{"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":""}}`,
			`{"type":"content_block_delta","index":0}`,
		}
		for _, data := range frames {
			if events := pushStreamData(t, decoder, data); len(events) != 0 {
				t.Fatalf("push(%s) = %+v, want nothing", data, events)
			}
		}
	})

	t.Run("a message start without a body is ignored", func(t *testing.T) {
		if events := pushStreamData(t, anthropic.Module().NewStreamDecoder(), `{"type":"message_start"}`); len(events) != 0 {
			t.Fatalf("events = %+v, want nothing", events)
		}
	})

	t.Run("a call left open is closed by Finish", func(t *testing.T) {
		decoder := anthropic.Module().NewStreamDecoder()
		data := `{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_3","name":"lookup","input":{"q":1}}}`
		pushStreamData(t, decoder, data)
		closing, err := decoder.Finish()
		if err != nil {
			t.Fatalf("Finish() error = %v", err)
		}
		if len(closing) != 2 || closing[0].Kind != inference.EventToolCallEnd || closing[1].Kind != inference.EventTerminal {
			t.Fatalf("Finish() = %+v, want the call closed before the terminal", closing)
		}
		if closing[0].ToolCall.Arguments != `{"q":1}` || closing[1].Terminal.Reason != inference.EventReasonToolUse {
			t.Fatalf("Finish() = %+v, want the call and a tool_use terminal", closing)
		}
	})

	t.Run("unknown content blocks are skipped", func(t *testing.T) {
		events, err := anthropic.Module().DecodeResponse([]byte(`{"id":"msg_9","content":[{"type":"future_block"}]}`))
		if err != nil {
			t.Fatalf("DecodeResponse() error = %v", err)
		}
		if countKind(events, inference.EventTextDelta)+countKind(events, inference.EventToolCallStart) != 0 {
			t.Fatalf("events = %+v, want only the terminal", events)
		}
	})
}

func TestInboundEncodeDetails(t *testing.T) {
	t.Run("a call start with arguments streams them", func(t *testing.T) {
		events := []inference.Event{
			{Kind: inference.EventToolCallStart, ToolCall: &inference.ToolCallDelta{Index: 0, ID: "toolu_1", Name: "lookup", Arguments: `{"q":1}`}},
			{Kind: inference.EventToolCallEnd, ToolCall: &inference.ToolCallDelta{Index: 0, Arguments: `{"q":1}`}},
		}
		frames := encodeEvents(t, anthropic.NewInbound(), events)
		if !framesContain(frames, "content_block_delta", "partial_json") {
			t.Fatalf("frames = %v, want the arguments streamed", frameNames(frames))
		}
		if !framesContain(frames, "content_block_start", "toolu_1") {
			t.Fatalf("frames = %v, want the call identity", frameNames(frames))
		}
	})

	t.Run("a tool call start without detail is ignored", func(t *testing.T) {
		frames, err := anthropic.NewInbound().EncodeResponseEvent(inference.Event{Kind: inference.EventToolCallStart})
		if err != nil || len(frames) != 0 {
			t.Fatalf("EncodeResponseEvent() = %v, %v, want nothing", frameNames(frames), err)
		}
	})

	t.Run("a length terminal reports max_tokens", func(t *testing.T) {
		events := []inference.Event{{Kind: inference.EventTerminal, Terminal: &inference.TerminalInfo{Reason: inference.EventReasonLength}}}
		frames := encodeEvents(t, anthropic.NewInbound(), events)
		if !framesContain(frames, "message_delta", "max_tokens") {
			t.Fatalf("frames = %v, want the length stop reason", frameNames(frames))
		}
	})

	t.Run("a failed response carries the error", func(t *testing.T) {
		events := []inference.Event{
			{Kind: inference.EventError, Error: &inference.ErrorInfo{Code: "stream_error", Message: "gone", Status: 502}},
			{Kind: inference.EventTerminal, Terminal: &inference.TerminalInfo{Reason: inference.EventReasonError}},
		}
		payload, err := anthropic.NewInbound().EncodeResponse(events)
		if err != nil {
			t.Fatalf("EncodeResponse() error = %v", err)
		}
		var message map[string]any
		if err := json.Unmarshal(payload, &message); err != nil {
			t.Fatalf("decode message: %v", err)
		}
		if message["error"].(map[string]any)["type"] != "stream_error" {
			t.Fatalf("message = %v, want the failure", message)
		}
	})
}

func TestInboundDecodeDetails(t *testing.T) {
	t.Run("image sources and missing sources are handled", func(t *testing.T) {
		body := []byte(`{"model":"claude-sonnet-4","max_tokens":8,"messages":[{"role":"user","content":[{"type":"image","source":{"type":"url","url":"https://example.invalid/a.png"}},{"type":"image"}]}]}`)
		request := inboundRequest(t, body)
		content := request.Messages[0].Content
		if content[0].ImageURL != "https://example.invalid/a.png" || content[1].ImageURL != "" {
			t.Fatalf("content = %+v, want the url source and an empty one", content)
		}
	})

	t.Run("thinking budgets map back to effort levels", func(t *testing.T) {
		cases := map[int]string{32000: "max", 24576: "xhigh", 16384: "high", 8192: "medium", 4096: "low", 1024: "low"}
		for budget, effort := range cases {
			body := []byte(fmt.Sprintf(`{"model":"claude-sonnet-4","max_tokens":8,"thinking":{"type":"enabled","budget_tokens":%d},"messages":[]}`, budget))
			if request := inboundRequest(t, body); request.Reasoning.Effort != effort {
				t.Fatalf("budget %d = %q, want %q", budget, request.Reasoning.Effort, effort)
			}
		}
	})

	t.Run("a tool result that is not a block or text keeps its raw content", func(t *testing.T) {
		body := []byte(`{"model":"claude-sonnet-4","max_tokens":8,"messages":[{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_1","content":5}]}]}`)
		request := inboundRequest(t, body)
		if len(request.Messages) != 1 || request.Messages[0].Content[0].Text != "5" {
			t.Fatalf("messages = %+v, want the raw result", request.Messages)
		}
	})
}

func TestContinueTurn(t *testing.T) {
	lastRole := func(t *testing.T, request *inference.Request) string {
		t.Helper()
		messages := encodeBody(t, request, wire.CodecOpts{CredentialRef: "sk-ant"})["messages"].([]any)
		return messages[len(messages)-1].(map[string]any)["role"].(string)
	}

	t.Run("a conversation ending on the assistant gets a user turn", func(t *testing.T) {
		request := canonicalRequest(false)
		request.Messages = append(request.Messages, inference.Message{
			Role:    inference.RoleAssistant,
			Content: []inference.ContentPart{{Type: inference.ContentTypeText, Text: "I'll look into it."}},
		})
		if role := lastRole(t, request); role != inference.RoleUser {
			t.Fatalf("last role = %q, want a user turn handing the turn back", role)
		}
		messages := encodeBody(t, request, wire.CodecOpts{CredentialRef: "sk-ant"})["messages"].([]any)
		last := messages[len(messages)-1].(map[string]any)
		if last["content"].([]any)[0].(map[string]any)["text"] != "(continue)" {
			t.Fatalf("last turn = %v, want the continuation prompt", last)
		}
	})

	t.Run("a conversation ending on a user turn is left alone", func(t *testing.T) {
		if role := lastRole(t, canonicalRequest(false)); role != inference.RoleUser {
			t.Fatalf("last role = %q, want the request untouched", role)
		}
	})

	t.Run("a trailing tool result is not appended after", func(t *testing.T) {
		request := canonicalRequest(false)
		request.Messages = append(request.Messages,
			inference.Message{
				Role:      inference.RoleAssistant,
				ToolCalls: []inference.ToolCall{{ID: "toolu_1", Name: "get_weather", Arguments: `{"city":"Oslo"}`}},
			},
			toolResult("toolu_1", "12 degrees"),
		)
		if role := lastRole(t, request); role != inference.RoleUser {
			t.Fatalf("last role = %q, want the flushed tool result to close the turn", role)
		}
		messages := encodeBody(t, request, wire.CodecOpts{CredentialRef: "sk-ant"})["messages"].([]any)
		if len(messages) != 3 {
			t.Fatalf("messages = %d, want user, assistant, and the result batch", len(messages))
		}
	})

	t.Run("a system-only conversation stays empty", func(t *testing.T) {
		request := canonicalRequest(false)
		request.Messages = request.Messages[:1]
		if messages := encodeBody(t, request, wire.CodecOpts{CredentialRef: "sk-ant"})["messages"]; messages != nil {
			t.Fatalf("messages = %v, want none", messages)
		}
	})
}
