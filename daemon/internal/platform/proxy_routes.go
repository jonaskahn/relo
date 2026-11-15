// Proxy mode's routing claim: the models this daemon answers itself, so the
// intercept knows which requests to route and which to hand to the vendor.
package platform

import (
	"github.com/jonaskahn/relo/internal/catalog"
)

// proxyRouteClaim reports whether the intercept answers a model rather than
// relaying it. A Relo-spelled model is always ours, and so is any id the
// catalog resolves to a provider model; a native Claude id is the vendor's
// own and stays with the vendor.
func proxyRouteClaim(models *catalog.Catalog) func(model string) bool {
	if models == nil {
		return func(string) bool { return false }
	}
	return func(model string) bool {
		if catalog.ClaudeAlias(model) {
			return true
		}
		snapshot, found := models.Snapshot()
		if !found {
			return false
		}
		if _, resolved := snapshot.ResolveClientID(model); resolved {
			return true
		}
		return false
	}
}
