// Integration store wiring for the application service.
package platform

import (
	"context"

	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	"github.com/jonaskahn/relo/internal/application/integration"
)

// IntegrationStore adapts the integration tables to the integration
// package's store contract.
type IntegrationStore struct {
	repo *sqlite.IntegrationRepo
}

// NewIntegrationStore returns a store over the given database.
func NewIntegrationStore(db *sqlite.DB) *IntegrationStore {
	return &IntegrationStore{repo: sqlite.NewIntegrationRepo(db)}
}

// Integration returns one stored integration.
func (s *IntegrationStore) Integration(ctx context.Context, id string) (integration.Record, bool, error) {
	row, found, err := s.repo.Get(ctx, id)
	if err != nil || !found {
		return integration.Record{}, found, err
	}
	return integration.Record{
		ID: row.ID, Enabled: row.Enabled, KeyID: row.KeyID,
		State: row.State, LastError: row.LastError, UpdatedAtMs: row.UpdatedAtMs,
		ModelsDigest: row.ModelsDigest, Context1M: row.Context1M,
	}, true, nil
}

// SaveIntegration stores one integration.
func (s *IntegrationStore) SaveIntegration(ctx context.Context, record integration.Record) error {
	return s.repo.Save(ctx, sqlite.IntegrationRow{
		ID: record.ID, Enabled: record.Enabled, KeyID: record.KeyID,
		State: record.State, LastError: record.LastError, UpdatedAtMs: record.UpdatedAtMs,
		ModelsDigest: record.ModelsDigest, Context1M: record.Context1M,
	})
}

// Files returns every file one integration owns.
func (s *IntegrationStore) Files(ctx context.Context, id string) ([]integration.FileRecord, error) {
	rows, err := s.repo.Files(ctx, id)
	if err != nil {
		return nil, err
	}
	files := make([]integration.FileRecord, 0, len(rows))
	for _, row := range rows {
		files = append(files, integration.FileRecord{
			IntegrationID: row.IntegrationID, Kind: row.Kind, Path: row.Path,
			Digest: row.Digest, SnapshotPath: row.SnapshotPath, UpdatedAtMs: row.UpdatedAtMs,
		})
	}
	return files, nil
}

// SaveFile records one file Relo wrote.
func (s *IntegrationStore) SaveFile(ctx context.Context, record integration.FileRecord) error {
	return s.repo.SaveFile(ctx, sqlite.IntegrationFileRow{
		IntegrationID: record.IntegrationID, Kind: record.Kind, Path: record.Path,
		Digest: record.Digest, SnapshotPath: record.SnapshotPath, UpdatedAtMs: record.UpdatedAtMs,
	})
}

// DeleteFiles forgets every file one integration owned.
func (s *IntegrationStore) DeleteFiles(ctx context.Context, id string) error {
	return s.repo.DeleteFiles(ctx, id)
}

// AppendOp records one action.
func (s *IntegrationStore) AppendOp(ctx context.Context, op integration.OpRecord) error {
	return s.repo.AppendOp(ctx, sqlite.IntegrationOpRow{
		ID: op.ID, IntegrationID: op.IntegrationID, Action: op.Action,
		Detail: op.Detail, CreatedAtMs: op.CreatedAtMs,
	})
}
