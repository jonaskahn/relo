// Application documents: raw JSON bodies addressed by namespace and id.
package sqlite

import (
	"context"
	"fmt"
)

// DocumentRow is one stored document: its id within the namespace, the schema
// version its owner stamped, and the raw JSON body.
type DocumentRow struct {
	ID      string
	Version int
	Body    []byte
}

// Documents stores application documents. It returns raw JSON and keeps no
// cache: the owning package decodes a body, and the collection above holds the
// decoded form in memory.
type Documents struct {
	db *DB
}

// NewDocuments returns the document store over the database.
func NewDocuments(db *DB) *Documents {
	return &Documents{db: db}
}

// List returns every document of one namespace.
func (s *Documents) List(ctx context.Context, namespace string) ([]DocumentRow, error) {
	rows, err := s.db.sql.QueryContext(ctx,
		"SELECT id, schema_version, body FROM app_documents WHERE namespace = ? ORDER BY id", namespace)
	if err != nil {
		return nil, fmt.Errorf("list %s documents: %w", namespace, err)
	}
	defer func() { _ = rows.Close() }()
	var found []DocumentRow
	for rows.Next() {
		var row DocumentRow
		var body string
		if err := rows.Scan(&row.ID, &row.Version, &body); err != nil {
			return nil, fmt.Errorf("read %s document: %w", namespace, err)
		}
		row.Body = []byte(body)
		found = append(found, row)
	}
	return found, rows.Err()
}

// Put replaces one document.
func (s *Documents) Put(ctx context.Context, namespace string, row DocumentRow, nowMs int64) error {
	if _, err := s.db.sql.ExecContext(ctx,
		"INSERT INTO app_documents (namespace, id, schema_version, body, updated_at) VALUES (?, ?, ?, ?, ?)"+
			" ON CONFLICT (namespace, id) DO UPDATE SET schema_version = excluded.schema_version,"+
			" body = excluded.body, updated_at = excluded.updated_at",
		namespace, row.ID, row.Version, string(row.Body), nowMs); err != nil {
		return fmt.Errorf("store %s document %s: %w", namespace, row.ID, err)
	}
	return nil
}

// Delete removes one document. A missing document is not an error.
func (s *Documents) Delete(ctx context.Context, namespace, id string) error {
	if _, err := s.db.sql.ExecContext(ctx,
		"DELETE FROM app_documents WHERE namespace = ? AND id = ?", namespace, id); err != nil {
		return fmt.Errorf("delete %s document %s: %w", namespace, id, err)
	}
	return nil
}
