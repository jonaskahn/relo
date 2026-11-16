// Document store wiring for the application collections.
package platform

import (
	"context"

	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	"github.com/jonaskahn/relo/internal/application/docstore"
)

// documentStore adapts the document table to the docstore contract.
type documentStore struct {
	repo *sqlite.Documents
}

var _ docstore.Store = (*documentStore)(nil)

// newDocumentStore returns the document store over the given database.
func newDocumentStore(db *sqlite.DB) *documentStore {
	return &documentStore{repo: sqlite.NewDocuments(db)}
}

func (s *documentStore) List(ctx context.Context, namespace string) ([]docstore.Row, error) {
	rows, err := s.repo.List(ctx, namespace)
	if err != nil {
		return nil, err
	}
	listed := make([]docstore.Row, 0, len(rows))
	for _, row := range rows {
		listed = append(listed, docstore.Row{ID: row.ID, Version: row.Version, Body: row.Body})
	}
	return listed, nil
}

func (s *documentStore) Put(ctx context.Context, namespace string, row docstore.Row, nowMs int64) error {
	return s.repo.Put(ctx, namespace, sqlite.DocumentRow{ID: row.ID, Version: row.Version, Body: row.Body}, nowMs)
}

func (s *documentStore) Delete(ctx context.Context, namespace, id string) error {
	return s.repo.Delete(ctx, namespace, id)
}
