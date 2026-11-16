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

// entrypointToken reads the entrypoint a Claude Code user-agent names, the word
// in `(external, <entrypoint>)`. Only that short token is ever kept.
var entrypointToken = regexp.MustCompile(`^claude-cli/\S+ \(external, ([a-z0-9-]{1,32})[,)]`)

// EntrypointOf is the entrypoint word a Claude Code user-agent carries, such as
// `cli` or `local-agent`, and "unknown" for any other shape. It is what an
// operator reads to learn which client a request came from, so it never carries
// more of the user-agent than that one word.
func EntrypointOf(userAgent string) string {
	if match := entrypointToken.FindStringSubmatch(userAgent); match != nil {
		return match[1]
	}
	return "unknown"
}

// ClientOf names the client a user-agent belongs to. Anything that is not a
// process Claude Desktop started is Claude Code's.
func ClientOf(userAgent string) string {
	if desktopEntrypoint.MatchString(userAgent) {
		return ClientDesktop
	}
	return ClientCode
}
