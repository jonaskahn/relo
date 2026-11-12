// Package wire embeds OpenCode's own system prompt, which the free lane
// expects every request to carry. A non-OpenCode client (Codex, Claude Code,
// anything else) brings its own prompt, which the gateway refuses, so Relo
// prepends OpenCode's captured prompt before sending the turn upstream.
package wire

import (
	_ "embed"
	"strings"

	"github.com/jonaskahn/relo/internal/inference"
)

// opencodeSystemPrompt is the complete system prompt captured from a native
// OpenCode client. The free lane refuses any request whose first system
// message is not OpenCode's own prompt, so Relo embeds it whole and prepends
// it to turns that did not come from OpenCode. The asset is shipped untrimmed:
// a truncated prompt is refused, so every byte matters.
//
//go:embed assets/opencode-system-prompt.txt
var opencodeSystemPrompt string

// openCodeSystemPromptMarker is the opening line OpenCode's client writes as
// its system prompt. A request whose first system message starts with it is
// native and is left untouched.
const openCodeSystemPromptMarker = "You are an AI agent running in OpenCode, a coding agent harness."

// HasOpenCodeSystemPrompt reports whether req already carries OpenCode's own
// system prompt, so the forge does not add a second copy to a native request.
// The gateway reads the body, not the User-Agent — a non-native client may send
// OpenCode's User-Agent because Relo rewrites it — so the body is the only
// signal that survives.
func HasOpenCodeSystemPrompt(req *inference.Request) bool {
	return hasOpenCodeSystemPromptMessages(req.Messages)
}

func hasOpenCodeSystemPromptMessages(messages []inference.Message) bool {
	for _, m := range messages {
		if m.Role != inference.RoleSystem {
			continue
		}
		text := spokenText(m.Content)
		return strings.HasPrefix(text, openCodeSystemPromptMarker) || text == opencodeSystemPrompt
	}
	return false
}

// spokenText returns the concatenated text parts of a message, so a prompt
// carried as content parts is read the same way as one carried as a string.
func spokenText(parts []inference.ContentPart) string {
	var b strings.Builder
	for _, p := range parts {
		if p.Type == inference.ContentTypeText {
			b.WriteString(p.Text)
		}
	}
	return b.String()
}

// withOpenCodeSystemPrompt returns messages with OpenCode's prompt prepended
// as a new system message at index 0, keeping the caller's own messages behind
// it. The input slice is not mutated: a fresh slice is returned.
func withOpenCodeSystemPrompt(messages []inference.Message) []inference.Message {
	copied := make([]inference.Message, 0, len(messages)+1)
	copied = append(copied, inference.Message{
		Role: inference.RoleSystem,
		Content: []inference.ContentPart{{
			Type: inference.ContentTypeText,
			Text: opencodeSystemPrompt,
		}},
	})
	copied = append(copied, messages...)
	return copied
}
