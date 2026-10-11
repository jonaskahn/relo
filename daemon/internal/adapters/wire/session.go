// Session anchor: the header binding a request to its session.
package wire

import (
	"encoding/json"
	"net/http"
	"strings"
)

// SessionAnchor returns the conversation identity an inbound client named.
//
// Codex's parent thread and its own thread are one identity: the parent
// alone is shared by every parallel child, and the child alone collides
// across parents. A root that sends only its own thread id is left on
// whatever session header it also sent, so an existing conversation does
// not move to a new anchor. OpenCode names the conversation with
// x-opencode-session. Everyone else uses a session or conversation header.
func SessionAnchor(header http.Header) string {
	if header == nil {
		return ""
	}
	parent := strings.TrimSpace(header.Get("x-codex-parent-thread-id"))
	own := strings.TrimSpace(header.Get("thread-id"))
	switch {
	case parent != "" && own != "":
		return "codex-thread:" + parent + "\x00" + own
	case parent != "":
		return "codex-thread:" + parent
	}
	for _, name := range []string{
		"x-opencode-session",
		"session-id",
		"session_id",
		"x-session-id",
		"conversation_id",
		"x-conversation-id",
		"x-claude-code-session-id",
		"x-session-affinity",
		"x-opencode-session-id",
	} {
		if value := strings.TrimSpace(header.Get(name)); value != "" {
			return value
		}
	}
	if session := codexTurnSession(header.Get("x-codex-turn-metadata")); session != "" {
		return "codex-turn:" + session
	}
	return ""
}

// codexTurnSession reads the conversation id a Codex turn carries in its
// metadata envelope. A malformed envelope is not a refusal: the turn keeps
// whatever other anchor named its conversation.
func codexTurnSession(metadata string) string {
	var envelope struct {
		SessionID string `json:"session_id"`
	}
	if err := json.Unmarshal([]byte(metadata), &envelope); err != nil {
		return ""
	}
	return strings.TrimSpace(envelope.SessionID)
}

// ClientKind names the family an inbound request came from, which is what
// separates one client's unlabelled conversations from another's. The header
// a client always sends is read before its user agent, because an agent that
// carries its own identity leaves a shape every request repeats.
func ClientKind(header http.Header) string {
	if header == nil {
		return ""
	}
	if strings.TrimSpace(header.Get("x-opencode-session")) != "" {
		return "opencode"
	}
	if strings.TrimSpace(header.Get("x-opencode-client")) != "" {
		return "opencode"
	}
	if strings.Contains(strings.ToLower(header.Get("user-agent")), "opencode") {
		return "opencode"
	}
	if strings.HasPrefix(strings.ToLower(header.Get("user-agent")), "claude-cli") {
		return "claude-code"
	}
	for _, name := range []string{
		"x-claude-code-session-id",
		"originator",
		"thread-id",
		"x-codex-parent-thread-id",
	} {
		if strings.TrimSpace(header.Get(name)) != "" {
			return "codex"
		}
	}
	if strings.TrimSpace(header.Get("user-agent")) != "" {
		return "other"
	}
	return ""
}
