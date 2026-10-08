// Account fact rows and the catalog repository writing them.
package sqlite

import (
	"context"
	"database/sql"
	"fmt"
)

const (
	upsertAccountFactSQL = `INSERT INTO account_model_facts
	(credential_id, provider_id, model_id, context_window, updated_at)
	VALUES (?, ?, ?, ?, ?)
	ON CONFLICT (credential_id, provider_id, model_id)
	DO UPDATE SET context_window = excluded.context_window, updated_at = excluded.updated_at`
	deleteAccountFactSQL = `DELETE FROM account_model_facts
	WHERE credential_id = ? AND provider_id = ? AND model_id = ?`
	listAccountFactsSQL = `SELECT model_id, context_window, updated_at
	FROM account_model_facts WHERE credential_id = ? ORDER BY model_id`
	listProviderAccountFactsSQL = `SELECT credential_id, model_id, context_window, updated_at
	FROM account_model_facts WHERE provider_id = ? ORDER BY credential_id, model_id`
)

// AccountFactRow is one account's stored override for one model.
type AccountFactRow struct {
	CredentialID  string
	ProviderID    string
	ModelID       string
	ContextWindow *int64
	UpdatedAtMs   int64
}

// SaveAccountModelFact writes one account's context override for one model. A
// nil window stores nothing usable, so the row is removed rather than kept as
// an empty statement of intent.
func (r *CatalogRepo) SaveAccountModelFact(ctx context.Context, row AccountFactRow) error {
	if row.ContextWindow == nil {
		_, err := r.db.sql.ExecContext(ctx, deleteAccountFactSQL, row.CredentialID, row.ProviderID, row.ModelID)
		if err != nil {
			return fmt.Errorf("clear the account context of %s/%s: %w", row.ProviderID, row.ModelID, err)
		}
		return nil
	}
	if _, err := r.db.sql.ExecContext(ctx, upsertAccountFactSQL,
		row.CredentialID, row.ProviderID, row.ModelID, *row.ContextWindow, row.UpdatedAtMs); err != nil {
		return fmt.Errorf("save the account context of %s/%s: %w", row.ProviderID, row.ModelID, err)
	}
	return nil
}

// DeleteAccountFacts removes one account's overrides, which is what removing
// the account does so a recycled identifier never inherits one.
func (r *CatalogRepo) DeleteAccountFacts(ctx context.Context, credentialID string) error {
	if _, err := r.db.sql.ExecContext(ctx,
		"DELETE FROM account_model_facts WHERE credential_id = ?", credentialID); err != nil {
		return fmt.Errorf("delete the account context overrides of %s: %w", credentialID, err)
	}
	return nil
}

// ListAccountFacts returns one account's stored overrides.
func (r *CatalogRepo) ListAccountFacts(ctx context.Context, credentialID string) ([]AccountFactRow, error) {
	rows, err := r.db.sql.QueryContext(ctx, listAccountFactsSQL, credentialID)
	if err != nil {
		return nil, fmt.Errorf("list the account context overrides of %s: %w", credentialID, err)
	}
	defer func() { _ = rows.Close() }()
	return scanAccountFacts(rows, credentialID, "")
}

// ListProviderAccountFacts returns every account override of one connection,
// which is what publishes the smallest eligible window for a shared model.
func (r *CatalogRepo) ListProviderAccountFacts(ctx context.Context, providerID string) ([]AccountFactRow, error) {
	rows, err := r.db.sql.QueryContext(ctx, listProviderAccountFactsSQL, providerID)
	if err != nil {
		return nil, fmt.Errorf("list the account context overrides of %s: %w", providerID, err)
	}
	defer func() { _ = rows.Close() }()
	return scanAccountFacts(rows, "", providerID)
}

func scanAccountFacts(rows *sql.Rows, credentialID, providerID string) ([]AccountFactRow, error) {
	facts := make([]AccountFactRow, 0, 8)
	for rows.Next() {
		row := AccountFactRow{CredentialID: credentialID, ProviderID: providerID}
		var window sql.NullInt64
		if providerID == "" {
			if err := rows.Scan(&row.ModelID, &window, &row.UpdatedAtMs); err != nil {
				return nil, fmt.Errorf("scan an account context override: %w", err)
			}
		} else {
			if err := rows.Scan(&row.CredentialID, &row.ModelID, &window, &row.UpdatedAtMs); err != nil {
				return nil, fmt.Errorf("scan an account context override: %w", err)
			}
		}
		if window.Valid {
			value := window.Int64
			row.ContextWindow = &value
		}
		facts = append(facts, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate account context overrides: %w", err)
	}
	return facts, nil
}
