package codec

import (
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/wire"
	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/internal/inference"
)

// The signed-out lane refuses a request that still carries another client's
// identity. A Claude Code prompt leads with an Anthropic billing stamp and the
// Claude Code sentence, so relaying it unchanged earns the lane's generic
// FreeUsageLimitError, which reads as a spent quota.
func TestOpenCodeFreeReplacesAForeignClientIdentity(t *testing.T) {
	const billing = "x-anthropic-billing-header: cc_version=2.1.327.1ce; cc_entrypoint=cli; cch=84b7b;"
	original := &inference.Request{
		WorkingDirectory: "/client",
		Messages: []inference.Message{
			{Role: inference.RoleSystem, Content: []inference.ContentPart{{
				Type: inference.ContentTypeText,
				Text: billing + "You are Claude Code, Anthropic's official CLI for Claude.\nFollow the client's rules.",
			}}},
			{Role: inference.RoleUser, Content: []inference.ContentPart{{Type: inference.ContentTypeText, Text: "hello"}}},
		},
	}
	got, _ := wire.OpenCodeFreeRequest(original, catalog.FormatOpenAIChat)
	system := got.Messages[0].Content[0].Text
	if strings.Contains(system, "x-anthropic-billing-header") || strings.Contains(system, "You are Claude Code") {
		t.Fatalf("system = %q, want the foreign identity gone", system)
	}
	if !strings.Contains(system, "Follow the client's rules.") {
		t.Fatalf("system = %q, want the client's own instructions kept", system)
	}
	if got.Messages[1].Content[0].Text != "hello" {
		t.Fatalf("user message = %q, want the task unchanged", got.Messages[1].Content[0].Text)
	}
	if strings.Contains(original.Messages[0].Content[0].Text, "You are a coding assistant.") {
		t.Fatal("mutated the caller's request")
	}
}

// A system block holding only the stamp and the identity becomes the lane's
// neutral identity rather than an empty message.
func TestOpenCodeFreeReplacesAnIdentityOnlySystemBlock(t *testing.T) {
	original := &inference.Request{Messages: []inference.Message{
		{Role: inference.RoleSystem, Content: []inference.ContentPart{{
			Type: inference.ContentTypeText,
			Text: "x-anthropic-billing-header: cc_version=1;\nYou are Claude Code, Anthropic's official CLI for Claude.",
		}}},
	}}
	got, _ := wire.OpenCodeFreeRequest(original, catalog.FormatOpenAIChat)
	if system := got.Messages[0].Content[0].Text; !strings.HasPrefix(system, "You are a coding assistant.") {
		t.Fatalf("system = %q, want the neutral identity", system)
	}
}
