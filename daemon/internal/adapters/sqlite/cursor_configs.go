// Cursor request configurations: one account's exact request tuple per
// group and selection.
package sqlite

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jonaskahn/relo/internal/catalog"
)

const (
	listCursorConfigsSQL = `SELECT group_id, selection, effort, is_default,
		model_id, details_id, max_mode, params, context_window, images, reasoning
	FROM cursor_model_configs WHERE provider_id = ? AND credential_id = ?
	ORDER BY group_id, selection`
	deleteCursorConfigsSQL = `DELETE FROM cursor_model_configs WHERE credential_id = ?`
	insertCursorConfigSQL  = `INSERT INTO cursor_model_configs
		(provider_id, credential_id, group_id, selection, effort, is_default,
		 model_id, details_id, max_mode, params, context_window, images, reasoning)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
)

// CursorConfigRepo stores the exact Cursor request tuples behind grouped
// models, so selection resolves the requested effort per account.
type CursorConfigRepo struct {
	db *DB
}

// NewCursorConfigRepo returns a repository over the given database.
func NewCursorConfigRepo(db *DB) *CursorConfigRepo {
	return &CursorConfigRepo{db: db}
}

// Save replaces one account's configurations for one connection.
func (r *CursorConfigRepo) Save(ctx context.Context, providerID, credentialID string, configs []catalog.CursorVariant) error {
	tx, err := r.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin cursor config write: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	save := cursorSave{providerID: providerID, credentialID: credentialID, configs: configs}
	if err := save.exec(ctx, tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit cursor config write: %w", err)
	}
	return nil
}

// cursorSave writes one account's configurations inside a transaction.
type cursorSave struct {
	providerID   string
	credentialID string
	configs      []catalog.CursorVariant
}

func (s cursorSave) exec(ctx context.Context, handle execer) error {
	if _, err := handle.ExecContext(ctx, `DELETE FROM cursor_model_configs WHERE provider_id = ? AND credential_id = ?`, s.providerID, s.credentialID); err != nil {
		return fmt.Errorf("clear cursor configs: %w", err)
	}
	seen := map[string]bool{}
	for _, config := range s.configs {
		if err := s.insert(ctx, handle, seen, config); err != nil {
			return err
		}
	}
	return nil
}

func (s cursorSave) insert(ctx context.Context, handle execer, seen map[string]bool, config catalog.CursorVariant) error {
	if config.GroupID == "" || config.Selection == "" {
		return nil
	}
	key := config.GroupID + "\x00" + config.Selection
	if seen[key] {
		return nil
	}
	seen[key] = true
	params, err := json.Marshal(config.Params)
	if err != nil {
		return fmt.Errorf("encode cursor params: %w", err)
	}
	if _, err := handle.ExecContext(ctx, insertCursorConfigSQL,
		s.providerID, s.credentialID, config.GroupID, config.Selection, config.Effort,
		boolInt(config.Default), config.ModelID, config.DetailsID, boolInt(config.MaxMode),
		string(params), config.Context, boolInt(config.Images), boolInt(config.Reasoning)); err != nil {
		return fmt.Errorf("write cursor config %s: %w", config.GroupID, err)
	}
	return nil
}

// Load returns one account's configurations for one connection.
func (r *CursorConfigRepo) Load(ctx context.Context, providerID, credentialID string) ([]catalog.CursorVariant, error) {
	rows, err := r.db.sql.QueryContext(ctx, listCursorConfigsSQL, providerID, credentialID)
	if err != nil {
		return nil, fmt.Errorf("list cursor configs: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var configs []catalog.CursorVariant
	for rows.Next() {
		var config catalog.CursorVariant
		var isDefault, maxMode, images, reasoning int
		var params string
		if err := rows.Scan(&config.GroupID, &config.Selection, &config.Effort,
			&isDefault, &config.ModelID, &config.DetailsID, &maxMode,
			&params, &config.Context, &images, &reasoning); err != nil {
			return nil, fmt.Errorf("scan cursor config: %w", err)
		}
		config.Default = isDefault != 0
		config.MaxMode = maxMode != 0
		config.Images = images != 0
		config.Reasoning = reasoning != 0
		if err := json.Unmarshal([]byte(params), &config.Params); err != nil {
			return nil, fmt.Errorf("decode cursor params: %w", err)
		}
		configs = append(configs, config)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate cursor configs: %w", err)
	}
	return configs, nil
}

// Forget drops everything one credential's Cursor listing published, which
// is what removing the account requires.
func (r *CursorConfigRepo) Forget(ctx context.Context, credentialID string) error {
	return r.forget(ctx, r.db.sql, credentialID)
}

func (r *CursorConfigRepo) forget(ctx context.Context, handle execer, credentialID string) error {
	if _, err := handle.ExecContext(ctx, deleteCursorConfigsSQL, credentialID); err != nil {
		return fmt.Errorf("forget cursor configs: %w", err)
	}
	return nil
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
