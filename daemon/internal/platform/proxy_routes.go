// Proxy mode's routing claim: the models this daemon answers itself, so the
// intercept knows which requests to route and which to hand to the vendor.
package platform

import (
	"github.com/jonaskahn/relo/internal/adapters/proxy"
	"github.com/jonaskahn/relo/internal/application/claudebindings"
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

// proxyBinding hands the intercept what a native model id is bound to. A model
// no binding applies to is left for the alias claim and then the vendor.
func proxyBinding(bindings *claudebindings.Service) func(integrationID, model string) (proxy.Binding, bool) {
	return func(integrationID, model string) (proxy.Binding, bool) {
		resolution := bindings.Resolve(integrationID, model)
		if !resolution.Matched {
			return proxy.Binding{}, false
		}
		return proxy.Binding{
			Rewrite:        resolution.Rewrite,
			VendorFirst:    resolution.Direction == claudebindings.DirectionFallback,
			VendorFallback: resolution.OnFailure == claudebindings.OnFailureNative,
			Reason:         resolution.Reason,
		}, true
	}
}
