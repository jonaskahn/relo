// Session anchor: the header binding a request to its session.
package wire

import (
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
	} {
		if value := strings.TrimSpace(header.Get(name)); value != "" {
			return value
		}
	}
	return ""
}
