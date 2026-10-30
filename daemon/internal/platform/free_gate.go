package platform

import (
	"context"
	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	"github.com/jonaskahn/relo/internal/catalog"
)

// freeGate leaves stored rows untouched. The routing snapshot keeps disabled
// free connections so a configured name reaches the consent refusal; the
// management projections and the public model list hide them instead.
type freeGate struct {
	*sqlite.CatalogReader
}

func (r freeGate) ListConnections(ctx context.Context) ([]catalog.ConnectionRecord, error) {
	return r.CatalogReader.ListConnections(ctx)
}
