// Replacing a foreign client's identity before the signed-out OpenCode lane.
package wire

import (
	"regexp"
	"strings"

	"github.com/jonaskahn/relo/internal/inference"
)

// freeLaneIdentity is the neutral self-description the signed-out gateway
// accepts. A Claude Code client leads its system prompt with its own identity
// and an Anthropic billing stamp; relaying either earns the lane's generic
// FreeUsageLimitError, which reads as a spent quota whatever the lane's
// allowance is.
const freeLaneIdentity = "You are a coding assistant."

// freeBillingHeader is the Anthropic subscription stamp a Claude client leads
// its system prompt with. Its fields are semicolon-terminated and the identity
// sentence follows the last one without a separator.
var freeBillingHeader = regexp.MustCompile(`^\s*x-anthropic-billing-header:(?:[^;\n]*;)+\s*`)

// freeForeignIdentities are the client self-descriptions a prompt may open
// with. Matching is case-insensitive and whitespace-tolerant because a client
// wraps or re-cases the sentence it stamps.
var freeForeignIdentities = []*regexp.Regexp{
	regexp.MustCompile(`(?i)you\s+are\s+claude\s+code\b[^\n]*`),
	regexp.MustCompile(`(?i)you\s+are\s+codex\b[^\n]*`),
	regexp.MustCompile(`(?i)you\s+are\s+opencode\b[^\n]*`),
	regexp.MustCompile(`(?i)you\s+are\s+cursor\b[^\n]*`),
	regexp.MustCompile(`(?i)you\s+are\s+cline\b[^\n]*`),
	regexp.MustCompile(`(?i)you\s+are\s+kilo\s+code\b[^\n]*`),
	regexp.MustCompile(`(?i)you\s+are\s+windsurf\b[^\n]*`),
	regexp.MustCompile(`(?i)you\s+are\s+roo\s+code\b[^\n]*`),
	regexp.MustCompile(`(?i)you\s+are\s+aider\b[^\n]*`),
	regexp.MustCompile(`(?i)you\s+are\s+gemini\s+cli\b[^\n]*`),
	regexp.MustCompile(`(?i)you\s+are\s+qwen\s+code\b[^\n]*`),
}

// withFreeLaneIdentity returns the messages with every foreign agent claim
// and Anthropic billing stamp replaced by the lane's own identity. The
// client's task, harness instructions, and non-system turns travel unchanged.
func withFreeLaneIdentity(messages []inference.Message) []inference.Message {
	if len(messages) == 0 {
		return messages
	}
	copied := append([]inference.Message(nil), messages...)
	for index := range copied {
		if copied[index].Role != inference.RoleSystem {
			continue
		}
		parts := append([]inference.ContentPart(nil), copied[index].Content...)
		for part := range parts {
			if parts[part].Type != inference.ContentTypeText {
				continue
			}
			parts[part].Text = freeLaneText(parts[part].Text)
		}
		copied[index].Content = parts
	}
	return copied
}

// freeLaneText strips the billing stamp and replaces a foreign identity
// sentence. A system block that held only those two becomes the neutral
// identity rather than an empty message.
func freeLaneText(text string) string {
	stripped := freeBillingHeader.ReplaceAllString(text, "")
	for _, identity := range freeForeignIdentities {
		stripped = identity.ReplaceAllString(stripped, freeLaneIdentity)
	}
	if strings.TrimSpace(stripped) == "" {
		return freeLaneIdentity
	}
	return stripped
}

// IsBillingStampOnly reports a system prompt that carries nothing but the
// Anthropic subscription stamp. A client that continues a server-side thread
// sends that stamp alone, which must not replace the thread's own prompt.
func IsBillingStampOnly(text string) bool {
	return strings.TrimSpace(freeBillingHeader.ReplaceAllString(text, "")) == ""
}
