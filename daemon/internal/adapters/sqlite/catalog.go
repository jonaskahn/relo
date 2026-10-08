// Catalog row aliases shared by readers and writers.
package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jonaskahn/relo/internal/catalog"
)

// Catalog errors reuse the feature's own sentinels, so storage and use
// cases answer a missing row with one vocabulary.
var (
	ErrProviderNotFound = catalog.ErrProviderNotFound
	ErrModelNotFound    = catalog.ErrModelNotFound
	ErrGroupNotFound    = catalog.ErrGroupNotFound
	ErrConflict         = catalog.ErrConflict
)

const providerColumns = "id, template_id, origin, label, auth, api_format, key_header, base_url, " +
	"models_source, models_format, modelsdev_provider_id, headers, doc_url, key_env, login_flows, " +
	"variables, enabled, rank, pool_strategy, last_refreshed_at, last_refresh_error, created_at, updated_at, use_proxy, " +
	"timeout_seconds, retry_backoff, switch_on_4xx, switch_on_5xx"

const modelColumns = "provider_id, model_id, source, api_format, base_url, modelsdev_ref, " +
	"match, enabled, available, listed_at, updated_at, upstream_model_id, cloned_from"

const modelFactsColumns = "provider_id, model_id, layer, name, description, family, category, " +
	"context_window, max_input, max_output, supports_tools, supports_reasoning, supports_vision, " +
	"status, release_date, input_price, output_price, cache_read_price, cache_write_price, " +
	"ext_threshold, ext_input_price, ext_output_price, ext_cache_read_price, ext_cache_write_price, " +
	"reasoning_efforts, reasoning_toggle, reasoning_budget, reasoning_budget_min, reasoning_budget_max"

// ProviderRow is one row of the providers table.
type ProviderRow = catalog.ConnectionRecord

// ModelRow is one row of the models table.
type ModelRow = catalog.ModelRecord

// ModelFactsRow represents one fact layer for a model.
type ModelFactsRow = catalog.FactsRecord

// PriceRow is one set of rates in integer USD micros per million tokens.
type PriceRow = catalog.PriceRecord

// GroupRow is one row of the model_groups table with its members.
type GroupRow struct {
	ID          string
	Label       string
	Strategy    string
	Enabled     bool
	Listed      bool
	Members     []GroupMemberRow
	CreatedAtMs int64
	UpdatedAtMs int64
	SwitchOn4xx *bool
	SwitchOn5xx *bool
}

// GroupMemberRow is one member of a group.
type GroupMemberRow struct {
	Position   int
	ProviderID string
	ModelID    string
	// Kind is how the member names its model: one connection's own row, or a
	// bare identifier the router resolves against every connection serving it.
	Kind    string
	Weight  int
	Enabled bool
}

// ModelsDevStateRow holds the last fetch status of models.dev.
type ModelsDevStateRow = catalog.ModelsDevStateRecord

// CatalogRepo is the SQLite-backed catalog store.
type CatalogRepo struct {
	db *DB
}

// NewCatalogRepo returns a repository over the given database.
func NewCatalogRepo(db *DB) *CatalogRepo {
	return &CatalogRepo{db: db}
}

// ListProviders returns every provider, ordered by identifier.
func (r *CatalogRepo) ListProviders(ctx context.Context) ([]ProviderRow, error) {
	rows, err := r.db.sql.QueryContext(ctx, "SELECT "+providerColumns+" FROM providers ORDER BY id")
	if err != nil {
		return nil, fmt.Errorf("list providers: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanProviders(rows)
}

// GetProvider returns one provider row.
func (r *CatalogRepo) GetProvider(ctx context.Context, id string) (ProviderRow, error) {
	row := r.db.sql.QueryRowContext(ctx, "SELECT "+providerColumns+" FROM providers WHERE id = ?", id)
	provider, err := scanProvider(row)
	if errors.Is(err, sql.ErrNoRows) {
		return ProviderRow{}, fmt.Errorf("%s: %w", id, ErrProviderNotFound)
	}
	if err != nil {
		return ProviderRow{}, fmt.Errorf("read provider %s: %w", id, err)
	}
	return provider, nil
}

// ListModels returns every model, ordered by provider and identifier.
func (r *CatalogRepo) ListModels(ctx context.Context) ([]ModelRow, error) {
	rows, err := r.db.sql.QueryContext(ctx, "SELECT "+modelColumns+" FROM models ORDER BY provider_id, model_id")
	if err != nil {
		return nil, fmt.Errorf("list models: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanModels(rows)
}

// GetModel returns one model.
func (r *CatalogRepo) GetModel(ctx context.Context, providerID, modelID string) (ModelRow, error) {
	row := r.db.sql.QueryRowContext(ctx,
		"SELECT "+modelColumns+" FROM models WHERE provider_id = ? AND model_id = ?", providerID, modelID)
	model, err := scanModel(row)
	if errors.Is(err, sql.ErrNoRows) {
		return ModelRow{}, fmt.Errorf("%s/%s: %w", providerID, modelID, ErrModelNotFound)
	}
	if err != nil {
		return ModelRow{}, fmt.Errorf("read model %s/%s: %w", providerID, modelID, err)
	}
	return model, nil
}

// ListAllModelFacts returns all facts rows for all models.
func (r *CatalogRepo) ListAllModelFacts(ctx context.Context) ([]ModelFactsRow, error) {
	rows, err := r.db.sql.QueryContext(ctx, "SELECT "+modelFactsColumns+" FROM model_facts ORDER BY provider_id, model_id, layer")
	if err != nil {
		return nil, fmt.Errorf("list model facts: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanModelFacts(rows)
}

// ListModelFacts returns all fact layers for one model.
func (r *CatalogRepo) ListModelFacts(ctx context.Context, providerID, modelID string) ([]ModelFactsRow, error) {
	rows, err := r.db.sql.QueryContext(ctx,
		"SELECT "+modelFactsColumns+" FROM model_facts WHERE provider_id = ? AND model_id = ? ORDER BY layer",
		providerID, modelID)
	if err != nil {
		return nil, fmt.Errorf("list facts for %s/%s: %w", providerID, modelID, err)
	}
	defer func() { _ = rows.Close() }()
	return scanModelFacts(rows)
}

// GetModelsDevState returns the recorded models.dev fetch state.
func (r *CatalogRepo) GetModelsDevState(ctx context.Context) (ModelsDevStateRow, bool, error) {
	var s ModelsDevStateRow
	err := r.db.sql.QueryRowContext(ctx, "SELECT source_url, fetched_at, etag, last_modified, "+
		"providers_count, models_count, last_attempt_at, last_error FROM modelsdev_state WHERE id = 1").
		Scan(&s.SourceURL, &s.FetchedAtMs, &s.ETag, &s.LastModified,
			&s.ProvidersCount, &s.ModelsCount, &s.LastAttemptAtMs, &s.LastError)
	if errors.Is(err, sql.ErrNoRows) {
		return ModelsDevStateRow{}, false, nil
	}
	if err != nil {
		return ModelsDevStateRow{}, false, fmt.Errorf("read modelsdev state: %w", err)
	}
	return s, true, nil
}

// ListGroups returns every group with its members.
func (r *CatalogRepo) ListGroups(ctx context.Context) ([]GroupRow, error) {
	rows, err := r.db.sql.QueryContext(ctx, "SELECT id, label, strategy, enabled, listed, created_at, updated_at, "+
		"switch_on_4xx, switch_on_5xx "+
		"FROM model_groups ORDER BY id")
	if err != nil {
		return nil, fmt.Errorf("list groups: %w", err)
	}
	defer func() { _ = rows.Close() }()
	groups, err := scanGroups(rows)
	if err != nil {
		return nil, err
	}
	for i := range groups {
		members, err := r.members(ctx, groups[i].ID)
		if err != nil {
			return nil, err
		}
		groups[i].Members = members
	}
	return groups, nil
}

// GetGroup returns one group with its members.
func (r *CatalogRepo) GetGroup(ctx context.Context, id string) (GroupRow, error) {
	row := r.db.sql.QueryRowContext(ctx, "SELECT id, label, strategy, enabled, listed, created_at, updated_at, "+
		"switch_on_4xx, switch_on_5xx "+
		"FROM model_groups WHERE id = ?", id)
	group, err := scanGroup(row)
	if errors.Is(err, sql.ErrNoRows) {
		return GroupRow{}, fmt.Errorf("%s: %w", id, ErrGroupNotFound)
	}
	if err != nil {
		return GroupRow{}, fmt.Errorf("read group %s: %w", id, err)
	}
	if group.Members, err = r.members(ctx, id); err != nil {
		return GroupRow{}, err
	}
	return group, nil
}

func (r *CatalogRepo) members(ctx context.Context, groupID string) ([]GroupMemberRow, error) {
	rows, err := r.db.sql.QueryContext(ctx, "SELECT position, provider_id, model_id, kind, weight, enabled "+
		"FROM model_group_members WHERE group_id = ? ORDER BY position", groupID)
	if err != nil {
		return nil, fmt.Errorf("list the members of %s: %w", groupID, err)
	}
	defer func() { _ = rows.Close() }()
	members := make([]GroupMemberRow, 0, 4)
	for rows.Next() {
		var m GroupMemberRow
		if err := rows.Scan(&m.Position, &m.ProviderID, &m.ModelID, &m.Kind, &m.Weight, &m.Enabled); err != nil {
			return nil, fmt.Errorf("scan group member: %w", err)
		}
		members = append(members, m)
	}
	return members, rows.Err()
}

func scanProviders(rows *sql.Rows) ([]ProviderRow, error) {
	var list []ProviderRow
	for rows.Next() {
		p, err := scanProvider(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, p)
	}
	return list, rows.Err()
}

func scanProvider(row rowScanner) (ProviderRow, error) {
	var (
		p                ProviderRow
		headersStr       string
		keyEnvStr        string
		loginFlowsStr    string
		variablesStr     string
		lastRefreshedAt  sql.NullInt64
		useProxy         int
		timeoutSeconds   sql.NullInt64
		retryBackoffText sql.NullString
		switchOn4xx      sql.NullInt64
		switchOn5xx      sql.NullInt64
	)

	err := row.Scan(&p.ID, &p.TemplateID, &p.Origin, &p.Label, &p.Auth,
		&p.APIFormat, &p.KeyHeader, &p.BaseURL, &p.ModelsSource, &p.ModelsFormat,
		&p.ModelsDevProviderID, &headersStr, &p.DocURL, &keyEnvStr, &loginFlowsStr,
		&variablesStr, &p.Enabled, &p.Rank, &p.PoolStrategy, &lastRefreshedAt,
		&p.LastRefreshError, &p.CreatedAtMs, &p.UpdatedAtMs, &useProxy,
		&timeoutSeconds, &retryBackoffText, &switchOn4xx, &switchOn5xx)
	if err != nil {
		return ProviderRow{}, err
	}

	return finishProviderRow(p, headersStr, keyEnvStr, loginFlowsStr, variablesStr,
		lastRefreshedAt, useProxy, timeoutSeconds, retryBackoffText, switchOn4xx, switchOn5xx), nil
}

func finishProviderRow(p ProviderRow, headersStr, keyEnvStr, loginFlowsStr, variablesStr string, lastRefreshedAt sql.NullInt64, useProxy int, timeoutSeconds sql.NullInt64, retryBackoffText sql.NullString, switchOn4xx, switchOn5xx sql.NullInt64) ProviderRow {
	p.Headers = decodeStringMap(headersStr)
	p.KeyEnv = decodeStrings(keyEnvStr)
	p.LoginFlows = decodeStrings(loginFlowsStr)
	p.Variables = decodeStringMap(variablesStr)
	if lastRefreshedAt.Valid {
		val := lastRefreshedAt.Int64
		p.LastRefreshedAtMs = &val
	}
	p.UseProxy = useProxy != 0
	if timeoutSeconds.Valid {
		val := int(timeoutSeconds.Int64)
		p.TimeoutSeconds = &val
	}
	if retryBackoffText.Valid {
		p.RetryBackoff = decodeRetryBackoff(retryBackoffText.String)
	}
	p.SwitchOn4xx = boolPtr(switchOn4xx)
	p.SwitchOn5xx = boolPtr(switchOn5xx)
	return p
}

func scanModels(rows *sql.Rows) ([]ModelRow, error) {
	var list []ModelRow
	for rows.Next() {
		m, err := scanModel(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, m)
	}
	return list, rows.Err()
}

func scanModel(row rowScanner) (ModelRow, error) {
	var (
		m         ModelRow
		available sql.NullInt64
		listedAt  sql.NullInt64
	)
	err := row.Scan(&m.ProviderID, &m.ModelID, &m.Source, &m.APIFormat,
		&m.BaseURL, &m.ModelsDevRef, &m.Match, &m.Enabled, &available, &listedAt, &m.UpdatedAtMs,
		&m.UpstreamModelID, &m.ClonedFrom)
	if err != nil {
		return ModelRow{}, err
	}
	m.Available = boolPtr(available)
	m.ListedAtMs = intPtr(listedAt)
	return m, nil
}

func scanModelFacts(rows *sql.Rows) ([]ModelFactsRow, error) {
	var list []ModelFactsRow
	for rows.Next() {
		f, err := scanFactsRow(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, f)
	}
	return list, rows.Err()
}

func scanFactsRow(rows *sql.Rows) (ModelFactsRow, error) {
	var scan factsScan
	err := rows.Scan(&scan.f.ProviderID, &scan.f.ModelID, &scan.f.Layer,
		&scan.name, &scan.desc, &scan.fam, &scan.cat, &scan.ctxWindow, &scan.maxIn, &scan.maxOut,
		&scan.tools, &scan.reason, &scan.vision, &scan.status, &scan.relDate,
		&scan.inPrice, &scan.outPrice, &scan.crPrice, &scan.cwPrice,
		&scan.extThresh, &scan.extInPrice, &scan.extOutPrice, &scan.extCrPrice, &scan.extCwPrice,
		&scan.efforts, &scan.toggle, &scan.budget, &scan.budgetMin, &scan.budgetMax)
	if err != nil {
		return ModelFactsRow{}, err
	}
	return scan.finish(), nil
}

type factsScan struct {
	f                                     ModelFactsRow
	name, desc, fam, cat, status, relDate sql.NullString
	efforts                               sql.NullString
	ctxWindow, maxIn, maxOut              sql.NullInt64
	tools, reason, vision                 sql.NullInt64
	inPrice, outPrice, crPrice, cwPrice   sql.NullInt64
	extThresh, extInPrice, extOutPrice    sql.NullInt64
	extCrPrice, extCwPrice                sql.NullInt64
	toggle, budget                        sql.NullInt64
	budgetMin, budgetMax                  sql.NullInt64
}

func (s *factsScan) finish() ModelFactsRow {
	s.f.Name = stringPtr(s.name)
	s.f.Description = stringPtr(s.desc)
	s.f.Family = stringPtr(s.fam)
	s.f.Category = stringPtr(s.cat)
	s.f.Status = stringPtr(s.status)
	s.f.ReleaseDate = stringPtr(s.relDate)
	s.f.ContextWindow = intPtr(s.ctxWindow)
	s.f.MaxInput = intPtr(s.maxIn)
	s.f.MaxOutput = intPtr(s.maxOut)
	s.f.SupportsTools = boolPtr(s.tools)
	s.f.SupportsReasoning = boolPtr(s.reason)
	s.f.SupportsVision = boolPtr(s.vision)
	s.f.ReasoningEfforts = stringListPtr(s.efforts)
	s.f.ReasoningToggle = boolPtr(s.toggle)
	s.f.ReasoningBudget = boolPtr(s.budget)
	s.f.ReasoningBudgetMin = intPtr(s.budgetMin)
	s.f.ReasoningBudgetMax = intPtr(s.budgetMax)
	s.f.Prices = s.prices()
	return s.f
}

func (s *factsScan) prices() PriceRow {
	return PriceRow{
		Input:         intPtr(s.inPrice),
		Output:        intPtr(s.outPrice),
		CacheRead:     intPtr(s.crPrice),
		CacheWrite:    intPtr(s.cwPrice),
		ExtThreshold:  intPtr(s.extThresh),
		ExtInput:      intPtr(s.extInPrice),
		ExtOutput:     intPtr(s.extOutPrice),
		ExtCacheRead:  intPtr(s.extCrPrice),
		ExtCacheWrite: intPtr(s.extCwPrice),
	}
}

func scanGroups(rows *sql.Rows) ([]GroupRow, error) {
	var list []GroupRow
	for rows.Next() {
		g, err := scanGroup(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, g)
	}
	return list, rows.Err()
}

func scanGroup(row rowScanner) (GroupRow, error) {
	var (
		g           GroupRow
		switchOn4xx sql.NullInt64
		switchOn5xx sql.NullInt64
	)
	err := row.Scan(&g.ID, &g.Label, &g.Strategy, &g.Enabled, &g.Listed, &g.CreatedAtMs, &g.UpdatedAtMs,
		&switchOn4xx, &switchOn5xx)
	if err != nil {
		return GroupRow{}, err
	}
	g.SwitchOn4xx = boolPtr(switchOn4xx)
	g.SwitchOn5xx = boolPtr(switchOn5xx)
	return g, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func encodeStringMap(m map[string]string) (string, error) {
	if len(m) == 0 {
		return "{}", nil
	}
	b, err := json.Marshal(m)
	return string(b), err
}

func encodeStrings(s []string) (string, error) {
	if len(s) == 0 {
		return "[]", nil
	}
	b, err := json.Marshal(s)
	return string(b), err
}

func decodeStringMap(raw string) map[string]string {
	res := make(map[string]string)
	_ = json.Unmarshal([]byte(raw), &res)
	return res
}

func decodeStrings(raw string) []string {
	var res []string
	_ = json.Unmarshal([]byte(raw), &res)
	return res
}

func nullInt(value *int64) sql.NullInt64 {
	if value == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *value, Valid: true}
}

func nullIntValue(value *int) sql.NullInt64 {
	if value == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(*value), Valid: true}
}

func nullRetryBackoff(windows [][2]int) sql.NullString {
	if windows == nil {
		return sql.NullString{}
	}
	encoded, err := json.Marshal(windows)
	if err != nil {
		return sql.NullString{}
	}
	return sql.NullString{String: string(encoded), Valid: true}
}

func decodeRetryBackoff(raw string) [][2]int {
	var windows [][2]int
	if err := json.Unmarshal([]byte(raw), &windows); err != nil {
		return nil
	}
	return windows
}

func bit(value bool) int {
	if value {
		return 1
	}
	return 0
}

func nullBool(value *bool) sql.NullInt64 {
	if value == nil {
		return sql.NullInt64{}
	}
	if *value {
		return sql.NullInt64{Int64: 1, Valid: true}
	}
	return sql.NullInt64{Int64: 0, Valid: true}
}

func nullString(s *string) sql.NullString {
	if s == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *s, Valid: true}
}

func nullStringList(values []string) sql.NullString {
	if values == nil {
		return sql.NullString{}
	}
	encoded, err := json.Marshal(values)
	if err != nil {
		return sql.NullString{}
	}
	return sql.NullString{String: string(encoded), Valid: true}
}

func stringListPtr(v sql.NullString) []string {
	if !v.Valid {
		return nil
	}
	var values []string
	if err := json.Unmarshal([]byte(v.String), &values); err != nil {
		return []string{}
	}
	return values
}

func intPtr(v sql.NullInt64) *int64 {
	if !v.Valid {
		return nil
	}
	val := v.Int64
	return &val
}

func boolPtr(v sql.NullInt64) *bool {
	if !v.Valid {
		return nil
	}
	val := v.Int64 != 0
	return &val
}

func stringPtr(v sql.NullString) *string {
	if !v.Valid {
		return nil
	}
	val := v.String
	return &val
}

func nowMs() int64 {
	return time.Now().UnixMilli()
}
