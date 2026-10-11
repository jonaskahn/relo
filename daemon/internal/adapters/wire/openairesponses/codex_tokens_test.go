// Codex encoding carries no context-mode switch: a small and a large
// estimate encode the same headers, so staying in the base tier costs no
// extra upstream overhead.
package openairesponses

import (
	"slices"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/wire"
	"github.com/jonaskahn/relo/internal/inference"
)

func TestPromptTokensCarryNoContextMode(t *testing.T) {
	request := &inference.Request{
		Model:  "gpt-6.1-sol",
		Stream: true,
		Messages: []inference.Message{{
			Role:    inference.RoleUser,
			Content: []inference.ContentPart{{Type: inference.ContentTypeText, Text: "hi"}},
		}},
	}
	optsFor := func(tokens int64) wire.CodecOpts {
		return wire.CodecOpts{
			BaseURL: "https://chatgpt.com/backend-api/codex", CredentialRef: "tok",
			AuthMethod: wire.AuthOAuth, RequestID: "req-1", PromptTokens: tokens,
		}
	}
	small, err := NewCodec("").EncodeRequest(request, optsFor(10_000))
	if err != nil {
		t.Fatalf("encode small: %v", err)
	}
	large, err := NewCodec("").EncodeRequest(request, optsFor(900_000))
	if err != nil {
		t.Fatalf("encode large: %v", err)
	}
	smallHeaders, largeHeaders := small.Header, large.Header
	if len(smallHeaders) != len(largeHeaders) {
		t.Fatalf("headers = %v vs %v, want the same set", smallHeaders, largeHeaders)
	}
	for name, values := range smallHeaders {
		if !slices.Equal(values, largeHeaders[name]) {
			t.Fatalf("header %s = %v vs %v, want no context-mode switch", name, values, largeHeaders[name])
		}
	}
	for _, name := range []string{"anthropic-beta", "x-codex-context-window", "openai-context-window"} {
		if value := small.Header.Get(name); value != "" {
			t.Fatalf("header %s = %q, want no context-mode header", name, value)
		}
	}
}
