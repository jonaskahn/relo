// Which client sent a request: the terminal's Claude Code, or a process Claude
// Desktop started. Both reach the vendor through the one intercept, so the
// user-agent is what tells their traffic apart.
package proxy

import "regexp"

// The clients the intercept tells apart. A client's name is its integration's
// id, which is where its bindings and its Relo key are kept.
const (
	ClientCode    = "claude-code"
	ClientDesktop = "claude-desktop"
)

// desktopEntrypoint matches the processes Claude Desktop starts. They share the
// settings file and the catalog file a plain `claude` reads, but they are a
// different integration with their own mode, bindings and key.
var desktopEntrypoint = regexp.MustCompile(`^claude-cli/\S+ \(external, (claude-desktop|claude-desktop-3p|local-agent)[,)]`)

// ClientOf names the client a user-agent belongs to. Anything that is not a
// process Claude Desktop started is Claude Code's.
func ClientOf(userAgent string) string {
	if desktopEntrypoint.MatchString(userAgent) {
		return ClientDesktop
	}
	return ClientCode
}
