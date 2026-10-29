package platform

import (
	"context"
	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/internal/config"
)

// freeGate leaves stored rows untouched; embedding retains capability reads.
type freeGate struct {
	*sqlite.CatalogReader
	configPath string
}

func (r freeGate) ListConnections(ctx context.Context) ([]catalog.ConnectionRecord, error) {
	rows, err := r.CatalogReader.ListConnections(ctx)
	if err != nil || config.ReadFreeConnections(r.configPath) {
		return rows, err
	}
	kept := make([]catalog.ConnectionRecord, 0, len(rows))
	for _, row := range rows {
		if !catalog.IsFreeTemplate(row.TemplateID) {
			kept = append(kept, row)
		}
	}
	return kept, nil
}
