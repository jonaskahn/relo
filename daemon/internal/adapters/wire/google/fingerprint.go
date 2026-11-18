// Replacing a foreign client's identity before a Cloud Code Assist turn.
package google

import "regexp"

// The endpoint reads the system prompt, not just the headers, and refuses a
// turn whose prompt claims another vendor's agent. A Claude client leads with
// its own identity sentence, so relaying that prompt unchanged earns a 429
// that reads as a spent quota whatever the account's quota is; a Codex,
// OpenCode, Cursor, or Cline prompt carries the same admission. Every such
// claim is rewritten to the identity this transport serves before the turn
// leaves, so the prompt a client wrote never reaches the endpoint as-is.
const antigravityIdentity = "You are Antigravity, a powerful agentic AI assistant designed by the Google Deepmind team."

// foreignIdentities are the client self-descriptions a prompt may open with.
// Matching is case-insensitive and whitespace-tolerant because a client wraps,
// indents, or re-cases the sentence it stamps; the endpoint normalises it the
// same way before it decides, which is why an exact string compare here would
// let a padded variant through.
var foreignIdentities = []*regexp.Regexp{
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

// scrubIdentity rewrites every foreign agent claim in a system prompt to this
// transport's own identity. The billing stamp is scrubbed first, because it is
// led by the same block that carries the identity and would otherwise survive
// as an unattributed prefix.
func scrubIdentity(system *content) {
	if system == nil {
		return
	}
	for index := range system.Parts {
		text := billingHeader.ReplaceAllString(system.Parts[index].Text, "")
		for _, identity := range foreignIdentities {
			text = identity.ReplaceAllString(text, antigravityIdentity)
		}
		system.Parts[index].Text = text
	}
}
