// Antigravity roster policy: what the vendor lists but a connection never serves.
package antigravity

import (
	"github.com/jonaskahn/relo/internal/catalog"
)

// DefaultPrefixes are the model families a Google Antigravity connection
// serves: Gemini models with a thinking effort and Claude models without one.
// The vendor lists tab, commit, and command helpers under the same document,
// and none of them is a model a coding agent routes to.
func DefaultPrefixes() []string {
	return []string{"claude", "gemini"}
}

// Serves reports whether one listed entry belongs on the roster.
func Serves(entry catalog.Listed, prefixes []string) bool {
	if catalog.RetiredAntigravity(entry.ID) {
		return false
	}
	return catalog.MatchesPrefix(entry.ID, prefixes)
}
