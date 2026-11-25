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

// A system block that held nothing but the billing stamp also becomes the
// lane's neutral identity rather than an empty message.
func TestOpenCodeFreeReplacesAStampOnlySystemBlock(t *testing.T) {
	original := &inference.Request{Messages: []inference.Message{
		{Role: inference.RoleSystem, Content: []inference.ContentPart{{
			Type: inference.ContentTypeText,
			Text: "x-anthropic-billing-header: cc_version=1;",
		}}},
	}}
	got, _ := wire.OpenCodeFreeRequest(original, catalog.FormatOpenAIChat)
	if system := got.Messages[0].Content[0].Text; system != "You are a coding assistant." {
		t.Fatalf("system = %q, want the neutral identity", system)
	}
}

// A system block with no text part and a text part that loses only the
// billing stamp keep their shape: the blank lookup returns before any copy,
// the non-text part travels untouched, and a residual instruction is not
// replaced by the neutral identity.
func TestOpenCodeFreeKeepsUnrelatedAndNonTextSystemContent(t *testing.T) {
	original := &inference.Request{Messages: []inference.Message{
		{Role: inference.RoleSystem},
		{Role: inference.RoleSystem, Content: []inference.ContentPart{
			{Type: inference.ContentTypeImage, ImageURL: "data:image/png;base64,AA=="},
			{Type: inference.ContentTypeText, Text: "keep this rule"},
		}},
	}}
	got, _ := wire.OpenCodeFreeRequest(original, catalog.FormatOpenAIChat)
	if len(got.Messages) != len(original.Messages) {
		t.Fatalf("messages = %d, want %d", len(got.Messages), len(original.Messages))
	}
	if got.Messages[1].Content[0].Type != inference.ContentTypeImage || got.Messages[1].Content[0].ImageURL == "" {
		t.Fatalf("part = %+v, want the image untouched", got.Messages[1].Content[0])
	}
	if got.Messages[1].Content[1].Text != "keep this rule" {
		t.Fatalf("text = %q, want the instruction kept", got.Messages[1].Content[1].Text)
	}
}

// The neutral identity is shared by every client name, so each pattern is
// exercised through the request path rather than left as a separate literal.
func TestOpenCodeFreeReplacesEveryForeignIdentity(t *testing.T) {
	for _, identity := range []string{
		"You are Codex, a coding agent.",
		"You are OpenCode, the open-source agent.",
		"You are Cursor.",
		"You are Cline.",
		"You are Kilo Code.",
		"You are Windsurf.",
		"You are Roo Code.",
		"You are Aider.",
		"You are Gemini CLI.",
		"You are Qwen Code.",
	} {
		t.Run(identity, func(t *testing.T) {
			original := &inference.Request{Messages: []inference.Message{{
				Role:    inference.RoleSystem,
				Content: []inference.ContentPart{{Type: inference.ContentTypeText, Text: identity + "\nKeep the task."}},
			}}}
			got, _ := wire.OpenCodeFreeRequest(original, catalog.FormatOpenAIChat)
			text := got.Messages[0].Content[0].Text
			if strings.Contains(text, identity) || !strings.HasPrefix(text, "You are a coding assistant.") {
				t.Fatalf("system = %q, want the neutral identity", text)
			}
			if !strings.Contains(text, "Keep the task.") {
				t.Fatalf("system = %q, want the task kept", text)
			}
		})
	}
}

// The billing stamp is anchored to the system block's start, so a stamp a
// user pasted later in the prompt is not deleted.
func TestOpenCodeFreeLeavesALaterBillingStampAlone(t *testing.T) {
	const text = "Rules first. x-anthropic-billing-header: cc_version=1;"
	original := &inference.Request{Messages: []inference.Message{{
		Role:    inference.RoleSystem,
		Content: []inference.ContentPart{{Type: inference.ContentTypeText, Text: text}},
	}}}
	got, _ := wire.OpenCodeFreeRequest(original, catalog.FormatOpenAIChat)
	if got.Messages[0].Content[0].Text != text {
		t.Fatalf("system = %q, want an inner stamp kept", got.Messages[0].Content[0].Text)
	}
}

// A continued thread sends the subscription stamp alone as its system prompt,
// which is not a prompt: recognizing it is what keeps the thread's own prompt.
func TestBillingStampOnly(t *testing.T) {
	const stamp = "x-anthropic-billing-header: cc_version=2.1.327; cc_entrypoint=cli; cch=84b7b;"
	if !wire.IsBillingStampOnly(stamp) {
		t.Fatalf("IsBillingStampOnly(%q) = false, want the stamp alone recognized", stamp)
	}
	if !wire.IsBillingStampOnly("  \n" + stamp + "\n ") {
		t.Fatal("IsBillingStampOnly() = false, want the surrounding space ignored")
	}
	if wire.IsBillingStampOnly(stamp + "Answer in French.") {
		t.Fatal("IsBillingStampOnly() = true, want a prompt after the stamp kept")
	}
	if wire.IsBillingStampOnly("Answer in French.") {
		t.Fatal("IsBillingStampOnly() = true, want any prompt kept")
	}
}
