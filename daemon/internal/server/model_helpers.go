// Small model and catalog mapping helpers the handlers share.
package server

import (
	"net/http"

	appcatalog "github.com/jonaskahn/relo/internal/application/catalog"
	"github.com/jonaskahn/relo/internal/catalog"
)

const (
	defaultModelLimit           = 100
	defaultModelsDevSearchLimit = 20
	// maxModelsDevSearchLimit bounds one search, so a picker stays a short
	// list rather than the whole catalog.
	maxModelsDevSearchLimit = 50
)

func pricesFrom(prices catalog.Prices) appcatalog.Prices {
	return appcatalog.Prices{
		Input: prices.Input, Output: prices.Output, CacheRead: prices.CacheRead,
		CacheWrite: prices.CacheWrite, ExtThreshold: prices.ExtThreshold,
		ExtInput: prices.ExtInput, ExtOutput: prices.ExtOutput,
		ExtCacheRead: prices.ExtCacheRead, ExtCacheWrite: prices.ExtCacheWrite,
	}
}

func boolOr(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
}

func queryInt64Value(r *http.Request, name string) int64 {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return 0
	}
	value, err := queryInt64(r, name, 0)
	if err != nil {
		return 0
	}
	return value
}
