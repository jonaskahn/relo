// Account-discovered capability facts and manual context completions.
package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jonaskahn/relo/internal/catalog"
)

const (
	listAccountCapabilitiesSQL = `SELECT credential_id, provider_id, model_id, source,
		context_window, max_input, max_output, supports_tools, supports_reasoning,
		supports_vision, reasoning_efforts, reasoning_toggle, reasoning_budget,
		reasoning_budget_min, reasoning_budget_max, observed_at
	FROM account_model_capabilities ORDER BY credential_id, provider_id, model_id`
	listManualAccountContextsSQL = `SELECT credential_id, provider_id, model_id,
		context_window, updated_at FROM account_model_facts
	WHERE context_window IS NOT NULL ORDER BY credential_id, provider_id, model_id`
	insertAccountCapabilitySQL = `INSERT INTO account_model_capabilities
		(credential_id, provider_id, model_id, source, context_window, max_input,
		 max_output, supports_tools, supports_reasoning, supports_vision,
		 reasoning_efforts, reasoning_toggle, reasoning_budget,
		 reasoning_budget_min, reasoning_budget_max, observed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (credential_id, provider_id, model_id) DO UPDATE SET
		 source = excluded.source, context_window = excluded.context_window,
		 max_input = excluded.max_input, max_output = excluded.max_output,
		 supports_tools = excluded.supports_tools,
		 supports_reasoning = excluded.supports_reasoning,
		 supports_vision = excluded.supports_vision,
		 reasoning_efforts = excluded.reasoning_efforts,
		 reasoning_toggle = excluded.reasoning_toggle,
		 reasoning_budget = excluded.reasoning_budget,
		 reasoning_budget_min = excluded.reasoning_budget_min,
		 reasoning_budget_max = excluded.reasoning_budget_max,
		 observed_at = excluded.observed_at`
)

// ListAccountCapabilities returns every discovered account capability plus the
// manual context completions in the same vocabulary. A manual row is marked
// with source "manual" so the resolver can keep it subordinate.
func (r *CatalogRepo) ListAccountCapabilities(ctx context.Context) ([]catalog.AccountFactsRecord, error) {
	rows, err := r.db.sql.QueryContext(ctx, listAccountCapabilitiesSQL)
	if err != nil {
		return nil, fmt.Errorf("list account capabilities: %w", err)
	}
	defer func() { _ = rows.Close() }()
	facts, err := scanAccountCapabilities(rows)
	if err != nil {
		return nil, err
	}
	manual, err := r.listManualAccountContexts(ctx)
	if err != nil {
		return nil, err
	}
	return append(facts, manual...), nil
}

func (r *CatalogRepo) listManualAccountContexts(ctx context.Context) ([]catalog.AccountFactsRecord, error) {
	rows, err := r.db.sql.QueryContext(ctx, listManualAccountContextsSQL)
	if err != nil {
		return nil, fmt.Errorf("list account context completions: %w", err)
	}
	defer func() { _ = rows.Close() }()
	facts := make([]catalog.AccountFactsRecord, 0, 8)
	for rows.Next() {
		var fact catalog.AccountFactsRecord
		var window int64
		if err := rows.Scan(&fact.CredentialID, &fact.ProviderID, &fact.ModelID, &window, &fact.ObservedAtMs); err != nil {
			return nil, fmt.Errorf("scan account context completion: %w", err)
		}
		fact.Source = "manual"
		fact.ContextWindow = &window
		facts = append(facts, fact)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate account context completions: %w", err)
	}
	return facts, nil
}

// SaveAccountCapabilities replaces one account's discovered facts for one
// connection. A successful empty roster clears the previous observations.
func (r *CatalogRepo) SaveAccountCapabilities(ctx context.Context, providerID, credentialID string, facts []catalog.AccountFactsRecord) error {
	tx, err := r.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin account capability write: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM account_model_capabilities WHERE provider_id = ? AND credential_id = ?`, providerID, credentialID); err != nil {
		return fmt.Errorf("clear account capabilities: %w", err)
	}
	for _, fact := range facts {
		if fact.ModelID == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx, insertAccountCapabilitySQL,
			credentialID, providerID, fact.ModelID, firstAccountSource(fact.Source),
			nullInt(fact.ContextWindow), nullInt(fact.MaxInput), nullInt(fact.MaxOutput),
			nullBool(fact.SupportsTools), nullBool(fact.SupportsReasoning), nullBool(fact.SupportsVision),
			nullStringList(fact.ReasoningEfforts), nullBool(fact.ReasoningToggle),
			nullBool(fact.ReasoningBudget), nullInt(fact.ReasoningBudgetMin), nullInt(fact.ReasoningBudgetMax),
			fact.ObservedAtMs); err != nil {
			return fmt.Errorf("write account capability %s: %w", fact.ModelID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit account capability write: %w", err)
	}
	return nil
}

func firstAccountSource(source string) string {
	if source == "" {
		return "listing"
	}
	return source
}

func scanAccountCapabilities(rows *sql.Rows) ([]catalog.AccountFactsRecord, error) {
	facts := make([]catalog.AccountFactsRecord, 0, 16)
	for rows.Next() {
		var fact catalog.AccountFactsRecord
		var contextWindow, maxInput, maxOutput sql.NullInt64
		var tools, reasoning, vision, toggle, budget, budgetMin, budgetMax sql.NullInt64
		var efforts sql.NullString
		if err := rows.Scan(&fact.CredentialID, &fact.ProviderID, &fact.ModelID, &fact.Source,
			&contextWindow, &maxInput, &maxOutput, &tools, &reasoning, &vision, &efforts,
			&toggle, &budget, &budgetMin, &budgetMax, &fact.ObservedAtMs); err != nil {
			return nil, fmt.Errorf("scan account capability: %w", err)
		}
		fact.ContextWindow = intPtr(contextWindow)
		fact.MaxInput = intPtr(maxInput)
		fact.MaxOutput = intPtr(maxOutput)
		fact.SupportsTools = boolPtr(tools)
		fact.SupportsReasoning = boolPtr(reasoning)
		fact.SupportsVision = boolPtr(vision)
		fact.ReasoningEfforts = stringListPtr(efforts)
		fact.ReasoningToggle = boolPtr(toggle)
		fact.ReasoningBudget = boolPtr(budget)
		fact.ReasoningBudgetMin = intPtr(budgetMin)
		fact.ReasoningBudgetMax = intPtr(budgetMax)
		facts = append(facts, fact)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate account capabilities: %w", err)
	}
	return facts, nil
}

// DeleteAccountCapabilities removes discovered facts for one account.
func (r *CatalogRepo) DeleteAccountCapabilities(ctx context.Context, credentialID string) error {
	if _, err := r.db.sql.ExecContext(ctx, `DELETE FROM account_model_capabilities WHERE credential_id = ?`, credentialID); err != nil {
		return fmt.Errorf("delete account capabilities of %s: %w", credentialID, err)
	}
	return nil
}
