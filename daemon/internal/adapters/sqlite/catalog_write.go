// Catalog writes: providers, models, and their facts.
package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// SaveProvider writes or updates a provider row.
func (r *CatalogRepo) SaveProvider(ctx context.Context, p ProviderRow) error {
	headers, keyEnv, loginFlows, variables, err := encodeProviderJSON(p)
	if err != nil {
		return err
	}
	p = normalizeProviderRow(p)

	query := providerUpsertQuery()

	_, err = r.db.sql.ExecContext(ctx, query,
		p.ID, p.TemplateID, p.Origin, p.Label, p.Auth, p.APIFormat, p.KeyHeader,
		p.BaseURL, p.ModelsSource, p.ModelsFormat, p.ModelsDevProviderID, headers, p.DocURL,
		keyEnv, loginFlows, variables, p.Enabled, p.Rank, p.PoolStrategy,
		nullInt(p.LastRefreshedAtMs), p.LastRefreshError, p.CreatedAtMs, p.UpdatedAtMs, bit(p.UseProxy),
		nullIntValue(p.TimeoutSeconds), nullRetryBackoff(p.RetryBackoff),
		nullBool(p.SwitchOn4xx), nullBool(p.SwitchOn5xx))
	if err != nil {
		return fmt.Errorf("save provider %s: %w", p.ID, err)
	}
	return nil
}

func encodeProviderJSON(p ProviderRow) (headers, keyEnv, loginFlows, variables string, err error) {
	if headers, err = encodeStringMap(p.Headers); err != nil {
		return "", "", "", "", err
	}
	if keyEnv, err = encodeStrings(p.KeyEnv); err != nil {
		return "", "", "", "", err
	}
	if loginFlows, err = encodeStrings(p.LoginFlows); err != nil {
		return "", "", "", "", err
	}
	if variables, err = encodeStringMap(p.Variables); err != nil {
		return "", "", "", "", err
	}
	return headers, keyEnv, loginFlows, variables, nil
}

func normalizeProviderRow(p ProviderRow) ProviderRow {
	if p.CreatedAtMs == 0 {
		p.CreatedAtMs = nowMs()
	}
	if p.UpdatedAtMs == 0 {
		p.UpdatedAtMs = nowMs()
	}
	if p.KeyHeader == "" {
		p.KeyHeader = "bearer"
	}
	if p.ModelsSource == "" {
		p.ModelsSource = "listing"
	}
	if p.ModelsFormat == "" {
		p.ModelsFormat = "none"
	}
	if p.PoolStrategy == "" {
		p.PoolStrategy = StrategyLeastLoaded
	}
	if p.Headers == nil {
		p.Headers = map[string]string{}
	}
	if p.Variables == nil {
		p.Variables = map[string]string{}
	}
	return p
}

// DeleteProvider deletes a provider. Refuses if referenced by group members.
func (r *CatalogRepo) DeleteProvider(ctx context.Context, id string) error {
	tx, err := r.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin provider delete %s: %w", id, err)
	}
	defer func() { _ = tx.Rollback() }()
	var count int
	if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM model_group_members WHERE provider_id = ?", id).Scan(&count); err != nil {
		return fmt.Errorf("check if provider %s is in use: %w", id, err)
	}
	if count > 0 {
		return fmt.Errorf("%s: %w (a group routes to one of its models)", id, ErrConflict)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM providers WHERE id = ?", id); err != nil {
		return fmt.Errorf("delete provider %s: %w", id, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit provider delete %s: %w", id, err)
	}
	return nil
}

func providerUpsertQuery() string {
	return "INSERT INTO providers (" + providerColumns + ") VALUES (" +
		placeholders(28) + ") ON CONFLICT (id) DO UPDATE SET " +
		"template_id = excluded.template_id, origin = excluded.origin, label = excluded.label, " +
		"auth = excluded.auth, api_format = excluded.api_format, key_header = excluded.key_header, " +
		"base_url = excluded.base_url, models_source = excluded.models_source, " +
		"models_format = excluded.models_format, modelsdev_provider_id = excluded.modelsdev_provider_id, " +
		"headers = excluded.headers, doc_url = excluded.doc_url, key_env = excluded.key_env, " +
		"login_flows = excluded.login_flows, variables = excluded.variables, enabled = excluded.enabled, " +
		"rank = excluded.rank, pool_strategy = excluded.pool_strategy, " +
		"last_refreshed_at = excluded.last_refreshed_at, last_refresh_error = excluded.last_refresh_error, " +
		"updated_at = excluded.updated_at, use_proxy = excluded.use_proxy, " +
		"timeout_seconds = excluded.timeout_seconds, retry_backoff = excluded.retry_backoff, " +
		"switch_on_4xx = excluded.switch_on_4xx, switch_on_5xx = excluded.switch_on_5xx"
}

func modelFactsQuery() string {
	return "INSERT INTO model_facts (" + modelFactsColumns + ") VALUES (" +
		placeholders(29) + ") ON CONFLICT (provider_id, model_id, layer) DO UPDATE SET " +
		"name = excluded.name, description = excluded.description, family = excluded.family, " +
		"category = excluded.category, context_window = excluded.context_window, " +
		"max_input = excluded.max_input, max_output = excluded.max_output, " +
		"supports_tools = excluded.supports_tools, supports_reasoning = excluded.supports_reasoning, " +
		"supports_vision = excluded.supports_vision, status = excluded.status, " +
		"release_date = excluded.release_date, input_price = excluded.input_price, " +
		"output_price = excluded.output_price, cache_read_price = excluded.cache_read_price, " +
		"cache_write_price = excluded.cache_write_price, ext_threshold = excluded.ext_threshold, " +
		"ext_input_price = excluded.ext_input_price, ext_output_price = excluded.ext_output_price, " +
		"ext_cache_read_price = excluded.ext_cache_read_price, ext_cache_write_price = excluded.ext_cache_write_price, " +
		"reasoning_efforts = excluded.reasoning_efforts, " +
		"reasoning_toggle = excluded.reasoning_toggle, reasoning_budget = excluded.reasoning_budget, " +
		"reasoning_budget_min = excluded.reasoning_budget_min, reasoning_budget_max = excluded.reasoning_budget_max"
}

// SaveModel inserts or updates a model.
func (r *CatalogRepo) SaveModel(ctx context.Context, m ModelRow) error {
	if m.UpdatedAtMs == 0 {
		m.UpdatedAtMs = nowMs()
	}
	if m.Source == "" {
		m.Source = "manual"
	}
	if m.Match == "" {
		m.Match = "none"
	}
	query := "INSERT INTO models (" + modelColumns + ") VALUES (" +
		placeholders(13) + ") ON CONFLICT (provider_id, model_id) DO UPDATE SET " +
		"source = excluded.source, api_format = excluded.api_format, base_url = excluded.base_url, " +
		"modelsdev_ref = excluded.modelsdev_ref, match = excluded.match, enabled = excluded.enabled, " +
		"available = excluded.available, listed_at = excluded.listed_at, updated_at = excluded.updated_at, " +
		"upstream_model_id = excluded.upstream_model_id, cloned_from = excluded.cloned_from"

	_, err := r.db.sql.ExecContext(ctx, query,
		m.ProviderID, m.ModelID, m.Source, m.APIFormat, m.BaseURL, m.ModelsDevRef,
		m.Match, m.Enabled, nullBool(m.Available), nullInt(m.ListedAtMs), m.UpdatedAtMs,
		m.UpstreamModelID, m.ClonedFrom)
	if err != nil {
		return fmt.Errorf("save model %s/%s: %w", m.ProviderID, m.ModelID, err)
	}
	return nil
}

// SaveModelFacts inserts or updates a model fact layer.
func (r *CatalogRepo) SaveModelFacts(ctx context.Context, f ModelFactsRow) error {
	query := modelFactsQuery()

	_, err := r.db.sql.ExecContext(ctx, query,
		f.ProviderID, f.ModelID, f.Layer, nullString(f.Name), nullString(f.Description),
		nullString(f.Family), nullString(f.Category), nullInt(f.ContextWindow), nullInt(f.MaxInput),
		nullInt(f.MaxOutput), nullBool(f.SupportsTools), nullBool(f.SupportsReasoning),
		nullBool(f.SupportsVision), nullString(f.Status), nullString(f.ReleaseDate),
		nullInt(f.Prices.Input), nullInt(f.Prices.Output), nullInt(f.Prices.CacheRead),
		nullInt(f.Prices.CacheWrite), nullInt(f.Prices.ExtThreshold), nullInt(f.Prices.ExtInput),
		nullInt(f.Prices.ExtOutput), nullInt(f.Prices.ExtCacheRead), nullInt(f.Prices.ExtCacheWrite),
		nullStringList(f.ReasoningEfforts),
		nullBool(f.ReasoningToggle), nullBool(f.ReasoningBudget),
		nullInt(f.ReasoningBudgetMin), nullInt(f.ReasoningBudgetMax))
	if err != nil {
		return fmt.Errorf("save facts for %s/%s (%s): %w", f.ProviderID, f.ModelID, f.Layer, err)
	}
	return nil
}

// DeleteModelRow deletes a model. Refuses if referenced by a group.
func (r *CatalogRepo) DeleteModelRow(ctx context.Context, providerID, modelID string) error {
	var count int
	err := r.db.sql.QueryRowContext(ctx,
		"SELECT count(*) FROM model_group_members WHERE provider_id = ? AND model_id = ?",
		providerID, modelID).Scan(&count)
	if err != nil {
		return fmt.Errorf("check if model %s/%s in use: %w", providerID, modelID, err)
	}
	if count > 0 {
		return fmt.Errorf("%s/%s: %w (a group routes to this model)", providerID, modelID, ErrConflict)
	}
	if _, err := r.db.sql.ExecContext(ctx,
		"DELETE FROM models WHERE provider_id = ? AND model_id = ?", providerID, modelID); err != nil {
		return fmt.Errorf("delete model %s/%s: %w", providerID, modelID, err)
	}
	return nil
}

// DeleteModelFactsLayer removes one fact layer of a model, which is what
// clearing an override or losing a models.dev match does.
func (r *CatalogRepo) DeleteModelFactsLayer(ctx context.Context, providerID, modelID, layer string) error {
	if _, err := r.db.sql.ExecContext(ctx,
		"DELETE FROM model_facts WHERE provider_id = ? AND model_id = ? AND layer = ?",
		providerID, modelID, layer); err != nil {
		return fmt.Errorf("delete the %s facts of %s/%s: %w", layer, providerID, modelID, err)
	}
	return nil
}

// SaveGroup saves a group and replaces its members in a transaction.
func (r *CatalogRepo) SaveGroup(ctx context.Context, group GroupRow) error {
	tx, err := r.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin save group: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if group.CreatedAtMs == 0 {
		group.CreatedAtMs = nowMs()
	}
	if group.UpdatedAtMs == 0 {
		group.UpdatedAtMs = nowMs()
	}

	if err := saveGroupRow(ctx, tx, group); err != nil {
		return err
	}
	if err := replaceGroupMembers(ctx, tx, group); err != nil {
		return err
	}

	return tx.Commit()
}

func saveGroupRow(ctx context.Context, tx *sql.Tx, group GroupRow) error {
	_, err := tx.ExecContext(ctx,
		"INSERT INTO model_groups (id, label, strategy, enabled, listed, created_at, updated_at, "+
			"switch_on_4xx, switch_on_5xx) "+
			"VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT (id) DO UPDATE SET "+
			"label = excluded.label, strategy = excluded.strategy, enabled = excluded.enabled, "+
			"listed = excluded.listed, updated_at = excluded.updated_at, "+
			"switch_on_4xx = excluded.switch_on_4xx, switch_on_5xx = excluded.switch_on_5xx",
		group.ID, group.Label, group.Strategy, group.Enabled, group.Listed, group.CreatedAtMs, group.UpdatedAtMs,
		nullBool(group.SwitchOn4xx), nullBool(group.SwitchOn5xx))
	if err != nil {
		return fmt.Errorf("save group %s: %w", group.ID, err)
	}
	return nil
}

func replaceGroupMembers(ctx context.Context, tx *sql.Tx, group GroupRow) error {
	if _, err := tx.ExecContext(ctx, "DELETE FROM model_group_members WHERE group_id = ?", group.ID); err != nil {
		return fmt.Errorf("clear group members %s: %w", group.ID, err)
	}

	for _, m := range group.Members {
		_, err := tx.ExecContext(ctx,
			"INSERT INTO model_group_members (group_id, position, provider_id, model_id, kind, weight, enabled) "+
				"VALUES (?, ?, ?, ?, ?, ?, ?)",
			group.ID, m.Position, m.ProviderID, m.ModelID, memberKindOf(m.Kind), m.Weight, m.Enabled)
		if err != nil {
			return fmt.Errorf("insert member %s into group %s: %w", m.ModelID, group.ID, err)
		}
	}
	return nil
}

func memberKindOf(kind string) string {
	if kind == "auto" {
		return "auto"
	}
	return "model"
}

// DeleteGroup removes a group.
func (r *CatalogRepo) DeleteGroup(ctx context.Context, id string) error {
	if _, err := r.db.sql.ExecContext(ctx, "DELETE FROM model_groups WHERE id = ?", id); err != nil {
		return fmt.Errorf("delete group %s: %w", id, err)
	}
	return nil
}

// SaveModelsDevState updates the models.dev fetch status.
func (r *CatalogRepo) SaveModelsDevState(ctx context.Context, s ModelsDevStateRow) error {
	query := "INSERT INTO modelsdev_state (id, source_url, fetched_at, etag, last_modified, " +
		"providers_count, models_count, last_attempt_at, last_error) VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?) " +
		"ON CONFLICT (id) DO UPDATE SET source_url = excluded.source_url, fetched_at = excluded.fetched_at, " +
		"etag = excluded.etag, last_modified = excluded.last_modified, " +
		"providers_count = excluded.providers_count, models_count = excluded.models_count, " +
		"last_attempt_at = excluded.last_attempt_at, last_error = excluded.last_error"

	_, err := r.db.sql.ExecContext(ctx, query,
		s.SourceURL, s.FetchedAtMs, s.ETag, s.LastModified,
		s.ProvidersCount, s.ModelsCount, s.LastAttemptAtMs, s.LastError)
	if err != nil {
		return fmt.Errorf("save modelsdev state: %w", err)
	}
	return nil
}

// CommitProbe saves a tested provider, credential, models, and facts in a single transaction.
func (r *CatalogRepo) CommitProbe(ctx context.Context, p ProviderRow, cred CredentialRow, models []ModelRow, facts []ModelFactsRow) error {
	tx, err := r.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin commit probe: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := saveProbeProvider(ctx, tx, p); err != nil {
		return err
	}
	if err := saveProbeCredential(ctx, tx, cred); err != nil {
		return err
	}
	if err := saveProbeModels(ctx, tx, models); err != nil {
		return err
	}
	if err := saveFactRows(ctx, tx, facts); err != nil {
		return err
	}

	return tx.Commit()
}

func saveProbeProvider(ctx context.Context, tx *sql.Tx, p ProviderRow) error {
	headers, _ := encodeStringMap(p.Headers)
	keyEnv, _ := encodeStrings(p.KeyEnv)
	loginFlows, _ := encodeStrings(p.LoginFlows)
	variables, _ := encodeStringMap(p.Variables)
	if p.CreatedAtMs == 0 {
		p.CreatedAtMs = nowMs()
	}
	if p.UpdatedAtMs == 0 {
		p.UpdatedAtMs = nowMs()
	}

	pQuery := providerUpsertQuery()

	if _, err := tx.ExecContext(ctx, pQuery,
		p.ID, p.TemplateID, p.Origin, p.Label, p.Auth, p.APIFormat, p.KeyHeader,
		p.BaseURL, p.ModelsSource, p.ModelsFormat, p.ModelsDevProviderID, headers, p.DocURL,
		keyEnv, loginFlows, variables, p.Enabled, p.Rank, p.PoolStrategy,
		nullInt(p.LastRefreshedAtMs), p.LastRefreshError, p.CreatedAtMs, p.UpdatedAtMs, bit(p.UseProxy),
		nullIntValue(p.TimeoutSeconds), nullRetryBackoff(p.RetryBackoff),
		nullBool(p.SwitchOn4xx), nullBool(p.SwitchOn5xx)); err != nil {
		return fmt.Errorf("save provider: %w", err)
	}
	return nil
}

func saveProbeCredential(ctx context.Context, tx *sql.Tx, cred CredentialRow) error {
	if cred.ID == "" {
		return nil
	}
	nowSec := nowMs() / 1000
	cQuery := "INSERT INTO credentials (id, provider_id, kind, label, secret_ref, status, priority, generation, created_at, updated_at) " +
		"VALUES (?, ?, ?, ?, ?, ?, ?, 0, ?, ?) ON CONFLICT (id) DO UPDATE SET " +
		"label = excluded.label, secret_ref = excluded.secret_ref, status = excluded.status, " +
		"priority = excluded.priority, updated_at = excluded.updated_at"
	if _, err := tx.ExecContext(ctx, cQuery,
		cred.ID, cred.ProviderID, cred.Kind, cred.Label, cred.SecretRef, cred.Status, cred.Priority, nowSec, nowSec); err != nil {
		return fmt.Errorf("save credential: %w", err)
	}
	return nil
}

func saveProbeModels(ctx context.Context, tx *sql.Tx, models []ModelRow) error {
	mQuery := "INSERT INTO models (" + modelColumns + ") VALUES (" +
		placeholders(13) + ") ON CONFLICT (provider_id, model_id) DO UPDATE SET " +
		"source = excluded.source, api_format = excluded.api_format, base_url = excluded.base_url, " +
		"modelsdev_ref = excluded.modelsdev_ref, match = excluded.match, enabled = excluded.enabled, " +
		"available = excluded.available, listed_at = excluded.listed_at, updated_at = excluded.updated_at, " +
		"upstream_model_id = excluded.upstream_model_id, cloned_from = excluded.cloned_from"

	for _, m := range models {
		if m.UpdatedAtMs == 0 {
			m.UpdatedAtMs = nowMs()
		}
		if _, err := tx.ExecContext(ctx, mQuery,
			m.ProviderID, m.ModelID, m.Source, m.APIFormat, m.BaseURL, m.ModelsDevRef,
			m.Match, m.Enabled, nullBool(m.Available), nullInt(m.ListedAtMs), m.UpdatedAtMs,
			m.UpstreamModelID, m.ClonedFrom); err != nil {
			return fmt.Errorf("save model %s: %w", m.ModelID, err)
		}
	}
	return nil
}

func saveFactRows(ctx context.Context, tx *sql.Tx, facts []ModelFactsRow) error {
	fQuery := modelFactsQuery()

	for _, f := range facts {
		if _, err := tx.ExecContext(ctx, fQuery,
			f.ProviderID, f.ModelID, f.Layer, nullString(f.Name), nullString(f.Description),
			nullString(f.Family), nullString(f.Category), nullInt(f.ContextWindow), nullInt(f.MaxInput),
			nullInt(f.MaxOutput), nullBool(f.SupportsTools), nullBool(f.SupportsReasoning),
			nullBool(f.SupportsVision), nullString(f.Status), nullString(f.ReleaseDate),
			nullInt(f.Prices.Input), nullInt(f.Prices.Output), nullInt(f.Prices.CacheRead),
			nullInt(f.Prices.CacheWrite), nullInt(f.Prices.ExtThreshold), nullInt(f.Prices.ExtInput),
			nullInt(f.Prices.ExtOutput), nullInt(f.Prices.ExtCacheRead), nullInt(f.Prices.ExtCacheWrite),
			nullStringList(f.ReasoningEfforts),
			nullBool(f.ReasoningToggle), nullBool(f.ReasoningBudget),
			nullInt(f.ReasoningBudgetMin), nullInt(f.ReasoningBudgetMax)); err != nil {
			return fmt.Errorf("save fact for %s (%s): %w", f.ModelID, f.Layer, err)
		}
	}
	return nil
}

// RecordRefresh notes a refresh attempt that could not read a model list. The
// operator asked for the refresh, so the attempt is recorded either way and the
// page can show why the roster did not change.
func (r *CatalogRepo) RecordRefresh(ctx context.Context, providerID string, atMs int64, refreshErr string) error {
	_, err := r.db.sql.ExecContext(ctx,
		"UPDATE providers SET last_refreshed_at = ?, last_refresh_error = ?, updated_at = ? WHERE id = ?",
		atMs, refreshErr, atMs, providerID)
	if err != nil {
		return fmt.Errorf("record the model refresh: %w", err)
	}
	return nil
}

// RefreshProviderModels updates a provider's models, marks missing models as unavailable, and updates fact layers.
func (r *CatalogRepo) RefreshProviderModels(ctx context.Context, providerID string, models []ModelRow, facts []ModelFactsRow, unavailableIDs []string, atMs int64, refreshErr string) error {
	tx, err := r.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin refresh models: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := stampRefreshProvider(ctx, tx, providerID, atMs, refreshErr); err != nil {
		return err
	}
	if err := clearProviderFacts(ctx, tx, providerID); err != nil {
		return err
	}
	if err := saveRefreshModels(ctx, tx, providerID, models, atMs); err != nil {
		return err
	}
	if err := markModelsUnavailable(ctx, tx, providerID, unavailableIDs, atMs); err != nil {
		return err
	}
	if err := saveFactRows(ctx, tx, facts); err != nil {
		return err
	}

	return tx.Commit()
}

func stampRefreshProvider(ctx context.Context, tx *sql.Tx, providerID string, atMs int64, refreshErr string) error {
	_, err := tx.ExecContext(ctx,
		"UPDATE providers SET last_refreshed_at = ?, last_refresh_error = ?, updated_at = ? WHERE id = ?",
		atMs, refreshErr, atMs, providerID)
	if err != nil {
		return fmt.Errorf("update provider refresh time: %w", err)
	}
	return nil
}

func clearProviderFacts(ctx context.Context, tx *sql.Tx, providerID string) error {
	// The provider layer is rewritten from scratch so a model that lost its
	// provider values does not keep stale ones. The override layer is untouched.
	if _, err := tx.ExecContext(ctx,
		"DELETE FROM model_facts WHERE provider_id = ? AND layer = 'provider'", providerID); err != nil {
		return fmt.Errorf("clear the provider fact layer: %w", err)
	}
	return nil
}

func saveRefreshModels(ctx context.Context, tx *sql.Tx, providerID string, models []ModelRow, atMs int64) error {
	mQuery := "INSERT INTO models (" + modelColumns + ") VALUES (" +
		placeholders(13) + ") ON CONFLICT (provider_id, model_id) DO UPDATE SET " +
		"source = excluded.source, api_format = excluded.api_format, base_url = excluded.base_url, " +
		"modelsdev_ref = excluded.modelsdev_ref, match = excluded.match, " +
		"available = excluded.available, listed_at = excluded.listed_at, updated_at = excluded.updated_at, " +
		"upstream_model_id = excluded.upstream_model_id, cloned_from = excluded.cloned_from"

	for _, m := range models {
		if _, err := tx.ExecContext(ctx, mQuery,
			m.ProviderID, m.ModelID, m.Source, m.APIFormat, m.BaseURL, m.ModelsDevRef,
			m.Match, m.Enabled, nullBool(m.Available), atMs, atMs,
			m.UpstreamModelID, m.ClonedFrom); err != nil {
			return fmt.Errorf("save model %s: %w", m.ModelID, err)
		}
	}
	return nil
}

func markModelsUnavailable(ctx context.Context, tx *sql.Tx, providerID string, unavailableIDs []string, atMs int64) error {
	for _, unavailID := range unavailableIDs {
		if _, err := tx.ExecContext(ctx,
			"UPDATE models SET available = 0, updated_at = ? WHERE provider_id = ? AND model_id = ?",
			atMs, providerID, unavailID); err != nil {
			return fmt.Errorf("mark model %s unavailable: %w", unavailID, err)
		}
	}
	return nil
}

// SaveModelsDevFacts writes the rows the downloaded catalog produced in one
// transaction, and records the fetch state with them. A model the catalog does
// not describe keeps the layer it already holds, so nothing here deletes: a
// catalog that stopped naming a model is not evidence about that model.
func (r *CatalogRepo) SaveModelsDevFacts(ctx context.Context, facts []ModelFactsRow, state ModelsDevStateRow) error {
	tx, err := r.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin update modelsdev facts: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := replaceModelsDevFacts(ctx, tx, facts); err != nil {
		return err
	}
	if err := saveModelsDevState(ctx, tx, state); err != nil {
		return err
	}

	return tx.Commit()
}

func replaceModelsDevFacts(ctx context.Context, tx *sql.Tx, facts []ModelFactsRow) error {
	fQuery := modelFactsQuery()

	for _, f := range facts {
		if _, err := tx.ExecContext(ctx, fQuery,
			f.ProviderID, f.ModelID, f.Layer, nullString(f.Name), nullString(f.Description),
			nullString(f.Family), nullString(f.Category), nullInt(f.ContextWindow), nullInt(f.MaxInput),
			nullInt(f.MaxOutput), nullBool(f.SupportsTools), nullBool(f.SupportsReasoning),
			nullBool(f.SupportsVision), nullString(f.Status), nullString(f.ReleaseDate),
			nullInt(f.Prices.Input), nullInt(f.Prices.Output), nullInt(f.Prices.CacheRead),
			nullInt(f.Prices.CacheWrite), nullInt(f.Prices.ExtThreshold), nullInt(f.Prices.ExtInput),
			nullInt(f.Prices.ExtOutput), nullInt(f.Prices.ExtCacheRead), nullInt(f.Prices.ExtCacheWrite),
			nullStringList(f.ReasoningEfforts),
			nullBool(f.ReasoningToggle), nullBool(f.ReasoningBudget),
			nullInt(f.ReasoningBudgetMin), nullInt(f.ReasoningBudgetMax)); err != nil {
			return fmt.Errorf("replace modelsdev fact for %s (%s): %w", f.ModelID, f.Layer, err)
		}
	}
	return nil
}

func saveModelsDevState(ctx context.Context, tx *sql.Tx, state ModelsDevStateRow) error {
	stateQuery := "INSERT INTO modelsdev_state (id, source_url, fetched_at, etag, last_modified, " +
		"providers_count, models_count, last_attempt_at, last_error) VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?) " +
		"ON CONFLICT (id) DO UPDATE SET source_url = excluded.source_url, fetched_at = excluded.fetched_at, " +
		"etag = excluded.etag, last_modified = excluded.last_modified, " +
		"providers_count = excluded.providers_count, models_count = excluded.models_count, " +
		"last_attempt_at = excluded.last_attempt_at, last_error = excluded.last_error"

	if _, err := tx.ExecContext(ctx, stateQuery,
		state.SourceURL, state.FetchedAtMs, state.ETag, state.LastModified,
		state.ProvidersCount, state.ModelsCount, state.LastAttemptAtMs, state.LastError); err != nil {
		return fmt.Errorf("save modelsdev state: %w", err)
	}
	return nil
}

// CountProviders returns total count of providers.
func (r *CatalogRepo) CountProviders(ctx context.Context) (int, error) {
	return r.count(ctx, "SELECT count(*) FROM providers")
}

// CountModels returns total count of models.
func (r *CatalogRepo) CountModels(ctx context.Context) (int, error) {
	return r.count(ctx, "SELECT count(*) FROM models")
}

// CountGroups returns total count of groups.
func (r *CatalogRepo) CountGroups(ctx context.Context) (int, error) {
	return r.count(ctx, "SELECT count(*) FROM model_groups")
}

func (r *CatalogRepo) count(ctx context.Context, query string) (int, error) {
	var count int
	if err := r.db.sql.QueryRowContext(ctx, query).Scan(&count); err != nil {
		return 0, fmt.Errorf("count: %w", err)
	}
	return count, nil
}

func placeholders(count int) string {
	if count <= 0 {
		return ""
	}
	return strings.TrimSuffix(strings.Repeat("?, ", count), ", ")
}
