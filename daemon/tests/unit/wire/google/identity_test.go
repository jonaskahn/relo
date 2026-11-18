package google_test

import (
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/wire"
	"github.com/jonaskahn/relo/internal/inference"
)

// The endpoint reads the system prompt, not only the headers, and refuses a
// turn whose prompt claims another vendor's agent. Each of these is a client
// identity that a coding agent stamps on the prompt it sends, and each one
// earned a 429 that reads as a spent quota before it was rewritten.

// identitySystem returns the system instruction an encoded turn carries.
func identitySystem(t *testing.T, messages []inference.Message) string {
	t.Helper()
	request := &inference.Request{Model: "gemini-2.0-flash", Messages: messages}
	body := assistBody(t, request, wire.CodecOpts{CredentialRef: "token", Project: "p"})
	instruction, found := body["systemInstruction"].(map[string]any)
	if !found {
		return ""
	}
	parts, _ := instruction["parts"].([]any)
	if len(parts) == 0 {
		return ""
	}
	text, _ := parts[0].(map[string]any)["text"].(string)
	return text
}

func systemMessage(text string) []inference.Message {
	return []inference.Message{{
		Role:    inference.RoleSystem,
		Content: []inference.ContentPart{{Type: inference.ContentTypeText, Text: text}},
	}}
}

// TestAssistRewritesAForeignIdentity covers every client identity Relo relays.
func TestAssistRewritesAForeignIdentity(t *testing.T) {
	for _, prompt := range []string{
		"You are Claude Code, Anthropic's official CLI for Claude.",
		"you are claude code, anthropic's official cli",
		"YOU ARE CLAUDE CODE",
		"You are Claude  Code",
		"You are\nClaude Code",
		"You are Codex, a coding agent based on GPT-6.",
		"You are OpenCode, an agent working with the user.",
		"You are Cursor, an AI code editor.",
		"You are Cline, a coding agent.",
		"You are Kilo Code, an agent.",
	} {
		t.Run(prompt, func(t *testing.T) {
			system := identitySystem(t, systemMessage(prompt))
			if strings.Contains(strings.ToLower(system), "claude code") ||
				strings.Contains(strings.ToLower(system), "you are codex") ||
				strings.Contains(strings.ToLower(system), "you are opencode") ||
				strings.Contains(strings.ToLower(system), "you are cursor") ||
				strings.Contains(strings.ToLower(system), "you are cline") ||
				strings.Contains(strings.ToLower(system), "you are kilo") {
				t.Fatalf("systemInstruction = %q, want the foreign identity gone", system)
			}
			if !strings.Contains(system, "You are Antigravity") {
				t.Fatalf("systemInstruction = %q, want this transport's own identity", system)
			}
		})
	}
}

// TestAssistKeepsTheHarnessAfterTheIdentity asserts the rewrite replaces the
// claim rather than the prompt: the rules behind the identity still travel.
func TestAssistKeepsTheHarnessAfterTheIdentity(t *testing.T) {
	const harness = "\n# Harness\n - Text you output is displayed as markdown.\n"
	system := identitySystem(t, systemMessage("You are Claude Code, Anthropic's official CLI for Claude."+harness))
	if !strings.Contains(system, "# Harness") || !strings.Contains(system, "displayed as markdown") {
		t.Fatalf("systemInstruction = %q, want the harness kept", system)
	}
}

// TestAssistScrubsTheBillingStamp covers the other subscription stamp: the
// Anthropic billing header leads the same block and is refused on its own.
func TestAssistScrubsTheBillingStamp(t *testing.T) {
	const stamp = "x-anthropic-billing-header: cc_version=2.1.327.1ce; cc_entrypoint=cli; cch=d2a72;"
	system := identitySystem(t, systemMessage(stamp+"\nYou are Claude Code, Anthropic's official CLI for Claude.\nDo the task."))
	if strings.Contains(system, "x-anthropic-billing-header") {
		t.Fatalf("systemInstruction = %q, want the billing stamp gone", system)
	}
	if !strings.Contains(system, "Do the task.") {
		t.Fatalf("systemInstruction = %q, want the task kept", system)
	}
}

// TestAssistLeavesItsOwnIdentityAlone guards the rewrite from mangling a
// prompt that is already this transport's.
func TestAssistLeavesItsOwnIdentityAlone(t *testing.T) {
	const prompt = "You are Antigravity, a powerful agentic AI assistant."
	if got := identitySystem(t, systemMessage(prompt)); got != prompt {
		t.Fatalf("systemInstruction = %q, want it unchanged", got)
	}
}
