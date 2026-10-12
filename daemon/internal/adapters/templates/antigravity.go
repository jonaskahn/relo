// Antigravity connection policy: the families its roster keeps.
package templates

import (
	"github.com/jonaskahn/relo/internal/adapters/antigravity"
	"github.com/jonaskahn/relo/internal/catalog"
)

// AntigravityFilter keeps what the vendor serves to a coding agent: the model
// families an operator routes to, without the ids the vendor retired. An
// operator narrows the families further, but never resurrects a retired id.
func AntigravityFilter(entry catalog.Listed) bool {
	return antigravity.Serves(entry, antigravity.DefaultPrefixes())
}
