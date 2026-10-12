// Account request configurations: one account's exact request tuple per
// grouped model and selection.
package sqlite

import (
	"context"
	"fmt"

	"github.com/jonaskahn/relo/internal/catalog"
)

const (
	listRequestConfigsSQL = `SELECT model_id, selection, effort, is_default,
		api_format, wire_config, context_window, images, reasoning
	FROM model_request_configs WHERE provider_id = ? AND credential_id = ?
	ORDER BY model_id, selection`
	deleteRequestConfigsSQL = `DELETE FROM model_request_configs WHERE credential_id = ?`
	insertRequestConfigSQL  = `INSERT INTO model_request_configs
		(provider_id, credential_id, model_id, selection, effort, is_default,
		 api_format, wire_config, context_window, images, reasoning)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
)

// RequestConfigRepo stores the exact request tuples behind grouped models,
// so selection resolves the requested selection per account. The wire
// payload belongs to the owning codec; this repo never reads inside it.
type RequestConfigRepo struct {
	db *DB
}

// NewRequestConfigRepo returns a repository over the given database.
func NewRequestConfigRepo(db *DB) *RequestConfigRepo {
	return &RequestConfigRepo{db: db}
}

// Save replaces one account's configurations for one connection.
func (r *RequestConfigRepo) Save(ctx context.Context, providerID, credentialID string, configs []catalog.ModelRequestConfig) error {
	tx, err := r.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin request config write: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	save := requestConfigSave{providerID: providerID, credentialID: credentialID, configs: configs}
	if err := save.exec(ctx, tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit request config write: %w", err)
	}
	return nil
}

type requestConfigSave struct {
	providerID   string
	credentialID string
	configs      []catalog.ModelRequestConfig
}

func (s requestConfigSave) exec(ctx context.Context, handle execer) error {
	if _, err := handle.ExecContext(ctx, `DELETE FROM model_request_configs WHERE provider_id = ? AND credential_id = ?`, s.providerID, s.credentialID); err != nil {
		return fmt.Errorf("clear request configs: %w", err)
	}
	seen := map[string]bool{}
	for _, config := range s.configs {
		if err := s.insert(ctx, handle, seen, config); err != nil {
			return err
		}
	}
	return nil
}

func (s requestConfigSave) insert(ctx context.Context, handle execer, seen map[string]bool, config catalog.ModelRequestConfig) error {
	if config.ModelID == "" || config.Selection == "" {
		return nil
	}
	key := config.ModelID + "\x00" + config.Selection
	if seen[key] {
		return nil
	}
	seen[key] = true
	wire := config.WireConfig
	if wire == "" {
		wire = "{}"
	}
	if _, err := handle.ExecContext(ctx, insertRequestConfigSQL,
		s.providerID, s.credentialID, config.ModelID, config.Selection, config.Effort,
		boolInt(config.Default), string(config.APIFormat), wire,
		config.Context, boolInt(config.Images), boolInt(config.Reasoning)); err != nil {
		return fmt.Errorf("write request config %s: %w", config.ModelID, err)
	}
	return nil
}

// Load returns one account's configurations for one connection.
func (r *RequestConfigRepo) Load(ctx context.Context, providerID, credentialID string) ([]catalog.ModelRequestConfig, error) {
	rows, err := r.db.sql.QueryContext(ctx, listRequestConfigsSQL, providerID, credentialID)
	if err != nil {
		return nil, fmt.Errorf("list request configs: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var configs []catalog.ModelRequestConfig
	for rows.Next() {
		var config catalog.ModelRequestConfig
		var apiFormat string
		var isDefault, images, reasoning int
		if err := rows.Scan(&config.ModelID, &config.Selection, &config.Effort,
			&isDefault, &apiFormat, &config.WireConfig,
			&config.Context, &images, &reasoning); err != nil {
			return nil, fmt.Errorf("scan request config: %w", err)
		}
		config.Default = isDefault != 0
		config.APIFormat = catalog.APIFormat(apiFormat)
		config.Images = images != 0
		config.Reasoning = reasoning != 0
		configs = append(configs, config)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate request configs: %w", err)
	}
	return configs, nil
}

// Forget drops everything one credential's listing published, which is what
// removing the account requires.
func (r *RequestConfigRepo) Forget(ctx context.Context, credentialID string) error {
	return r.forget(ctx, r.db.sql, credentialID)
}

func (r *RequestConfigRepo) forget(ctx context.Context, handle execer, credentialID string) error {
	if _, err := handle.ExecContext(ctx, deleteRequestConfigsSQL, credentialID); err != nil {
		return fmt.Errorf("forget request configs: %w", err)
	}
	return nil
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
