// Package discovery reads the model-list dialects providers publish, so a
// connection serves exactly the roster its provider answers with.
package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/jonaskahn/relo/internal/adapters/antigravity"
	"github.com/jonaskahn/relo/internal/adapters/codex"
	"github.com/jonaskahn/relo/internal/adapters/wire/anthropic"
	"github.com/jonaskahn/relo/internal/adapters/wire/google"
	"github.com/jonaskahn/relo/internal/catalog"
)

type openAIModels struct {
	Data []openAIModelEntry `json:"data"`
}

type openAIModelEntry struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	ContextLength *int64 `json:"context_length"`
	ContextWindow *int64 `json:"context_window"`
	MaxOutput     *int64 `json:"max_output_tokens"`
	IsFree        *bool  `json:"isFree"`
	Pricing       *struct {
		Prompt          string `json:"prompt"`
		Completion      string `json:"completion"`
		InputCacheRead  string `json:"input_cache_read"`
		InputCacheWrite string `json:"input_cache_write"`
	} `json:"pricing"`
	TopProvider *struct {
		MaxCompletionTokens *int64 `json:"max_completion_tokens"`
	} `json:"top_provider"`
}

func (l *Lister) listOpenAI(ctx context.Context, target Target) ([]Listed, error) {
	body, err := l.get(ctx, target, "/models")
	if err != nil {
		return nil, err
	}
	var document openAIModels
	if err := decode(body, &document); err != nil {
		return nil, err
	}
	models := make([]Listed, 0, len(document.Data))
	for _, entry := range document.Data {
		if listed, ok := openAIModel(entry); ok {
			models = append(models, listed)
		}
	}
	return truncate(models), nil
}

func openAIModel(entry openAIModelEntry) (Listed, bool) {
	if strings.TrimSpace(entry.ID) == "" {
		return Listed{}, false
	}
	maxOutput := entry.MaxOutput
	if maxOutput == nil && entry.TopProvider != nil {
		maxOutput = entry.TopProvider.MaxCompletionTokens
	}
	return Listed{
		ID:            entry.ID,
		Name:          entry.Name,
		ContextWindow: firstOf(entry.ContextWindow, entry.ContextLength),
		MaxOutput:     maxOutput,
		Prices:        openAIModelPrices(entry),
		IsFree:        entry.IsFree,
	}, true
}

func openAIModelPrices(entry openAIModelEntry) *catalog.Prices {
	if entry.Pricing == nil {
		return nil
	}
	p := parseOpenRouterPricing(entry.Pricing.Prompt, entry.Pricing.Completion,
		entry.Pricing.InputCacheRead, entry.Pricing.InputCacheWrite)
	if !p.Known() {
		return nil
	}
	return &p
}

func parseOpenRouterPricing(prompt, completion, cacheRead, cacheWrite string) catalog.Prices {
	return catalog.Prices{
		Input:      parsePerTokenUSD(prompt),
		Output:     parsePerTokenUSD(completion),
		CacheRead:  parsePerTokenUSD(cacheRead),
		CacheWrite: parsePerTokenUSD(cacheWrite),
	}
}

func parsePerTokenUSD(val string) *int64 {
	val = strings.TrimSpace(val)
	if val == "" {
		return nil
	}
	f, err := strconv.ParseFloat(val, 64)
	if err != nil || f < 0 {
		return nil
	}
	// USD per token -> micro-dollars per 1M tokens: f * 1e6 (dollars/1M) * 1e6 (micros/dollar) = f * 1e12
	micros := int64(math.Round(f * 1e12))
	return &micros
}

type anthropicModels struct {
	Data []struct {
		ID          string `json:"id"`
		DisplayName string `json:"display_name"`
	} `json:"data"`
	HasMore bool   `json:"has_more"`
	LastID  string `json:"last_id"`
}

func (l *Lister) listAnthropic(ctx context.Context, target Target) ([]Listed, error) {
	headers := anthropicHeaders(target)
	models := []Listed{}
	after := ""
	for page := 0; page <= maxModels/pageSize; page++ {
		values := url.Values{}
		values.Set("limit", strconv.Itoa(pageSize))
		if after != "" {
			values.Set("after_id", after)
		}
		body, err := l.getWith(ctx, target, withQuery("/models", values), headers)
		if err != nil {
			return nil, err
		}
		var document anthropicModels
		if err := decode(body, &document); err != nil {
			return nil, err
		}
		for _, entry := range document.Data {
			models = append(models, Listed{ID: entry.ID, Name: entry.DisplayName})
		}
		if !document.HasMore || document.LastID == "" {
			break
		}
		after = document.LastID
	}
	return truncate(models), nil
}

func anthropicHeaders(target Target) map[string]string {
	headers := map[string]string{anthropic.VersionHeader: anthropic.APIVersion}
	switch {
	case target.Auth.Auth == catalog.AuthOAuth && target.Auth.Token != "":
		headers["Authorization"] = "Bearer " + target.Auth.Token
		headers[anthropic.BetaHeader] = anthropic.OAuthBetaValue
	case target.Auth.Token != "":
		headers[anthropic.APIKeyHeader] = target.Auth.Token
	}
	return headers
}

type geminiModels struct {
	Models []struct {
		Name                       string   `json:"name"`
		DisplayName                string   `json:"displayName"`
		InputTokenLimit            *int64   `json:"inputTokenLimit"`
		OutputTokenLimit           *int64   `json:"outputTokenLimit"`
		SupportedGenerationMethods []string `json:"supportedGenerationMethods"`
	} `json:"models"`
	NextPageToken string `json:"nextPageToken"`
}

func (l *Lister) listGemini(ctx context.Context, target Target) ([]Listed, error) {
	models := []Listed{}
	if err := l.collectGeminiPages(ctx, target, &models); err != nil {
		return nil, err
	}
	return truncate(models), nil
}

func (l *Lister) collectGeminiPages(ctx context.Context, target Target, models *[]Listed) error {
	pageToken := ""
	for page := 0; page <= maxModels/pageSize; page++ {
		next, err := l.collectGeminiPage(ctx, target, models, pageToken)
		if err != nil {
			return err
		}
		if next == "" {
			return nil
		}
		pageToken = next
	}
	return nil
}

func (l *Lister) collectGeminiPage(ctx context.Context, target Target, models *[]Listed, pageToken string) (string, error) {
	body, err := l.getGeminiPage(ctx, target, pageToken)
	if err != nil {
		return "", err
	}
	var document geminiModels
	if err := decode(body, &document); err != nil {
		return "", err
	}
	for _, entry := range document.Models {
		if !supportsGenerateContent(entry.SupportedGenerationMethods) {
			continue
		}
		*models = append(*models, Listed{
			ID:            strings.TrimPrefix(entry.Name, "models/"),
			Name:          entry.DisplayName,
			ContextWindow: entry.InputTokenLimit,
			MaxOutput:     entry.OutputTokenLimit,
		})
	}
	return document.NextPageToken, nil
}

func (l *Lister) getGeminiPage(ctx context.Context, target Target, pageToken string) ([]byte, error) {
	values := url.Values{}
	values.Set("pageSize", strconv.Itoa(pageSize))
	if pageToken != "" {
		values.Set("pageToken", pageToken)
	}
	headers := map[string]string{}
	if target.Auth.Token != "" {
		headers[google.APIKeyHeader] = target.Auth.Token
	}
	return l.getWith(ctx, target, withQuery("/models", values), headers)
}

func supportsGenerateContent(methods []string) bool {
	for _, method := range methods {
		if method == "generateContent" {
			return true
		}
	}
	return false
}

type antigravityModels struct {
	Models map[string]struct {
		DisplayName     string `json:"displayName"`
		MaxTokens       *int64 `json:"maxTokens"`
		MaxOutputTokens *int64 `json:"maxOutputTokens"`
		Model           string `json:"model"`
		ThinkingBudget  *int   `json:"thinkingBudget"`
	} `json:"models"`
	AgentModelSorts []struct {
		Groups []struct {
			ModelIDs []string `json:"modelIds"`
		} `json:"groups"`
	} `json:"agentModelSorts"`
	TieredModelIDs             map[string][]string        `json:"tieredModelIds"`
	ImageGenerationModelIDs    []string                   `json:"imageGenerationModelIds"`
	DefaultAgentModelID        string                     `json:"defaultAgentModelId"`
	TabModelIDs                []string                   `json:"tabModelIds"`
	CommandModelIDs            []string                   `json:"commandModelIds"`
	CommitMessageModelIDs      []string                   `json:"commitMessageModelIds"`
	MqueryModelIDs             []string                   `json:"mqueryModelIds"`
	WebSearchModelIDs          []string                   `json:"webSearchModelIds"`
	AudioTranscriptionModelIDs []string                   `json:"audioTranscriptionModelIds"`
	DeprecatedModelIDs         map[string]json.RawMessage `json:"deprecatedModelIds"`
}

func (d antigravityModels) agentModelIDs() map[string]bool {
	ids := make(map[string]bool, len(d.Models))
	for _, sort := range d.AgentModelSorts {
		for _, group := range sort.Groups {
			for _, id := range group.ModelIDs {
				ids[id] = true
			}
		}
	}
	for _, tiered := range d.TieredModelIDs {
		for _, id := range tiered {
			ids[id] = true
		}
	}
	for _, id := range d.ImageGenerationModelIDs {
		ids[id] = true
	}
	if d.DefaultAgentModelID != "" {
		ids[d.DefaultAgentModelID] = true
	}
	return ids
}

func (d antigravityModels) nonAgentModelIDs() map[string]bool {
	ids := make(map[string]bool, 16)
	for _, bucket := range [][]string{
		d.TabModelIDs, d.CommandModelIDs, d.CommitMessageModelIDs,
		d.MqueryModelIDs, d.WebSearchModelIDs, d.AudioTranscriptionModelIDs,
	} {
		for _, id := range bucket {
			ids[id] = true
		}
	}
	return ids
}

func antigravityServes(id string, agents, others map[string]bool, deprecated map[string]json.RawMessage) bool {
	switch {
	case catalog.RetiredAntigravity(id):
		return false
	case deprecated != nil:
		if _, found := deprecated[id]; found {
			return false
		}
	}
	return agents[id] || !others[id]
}

func antigravityBody(project string) (string, error) {
	payload := map[string]string{}
	if project != "" {
		payload["project"] = project
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode the model list request: %w", err)
	}
	return string(body), nil
}

func antigravityRefusal(status int, code, detail string) error {
	return fmt.Errorf("%w: %s", ErrListRejected, antigravity.Reason(status, code, detail))
}

func (l *Lister) listAntigravity(ctx context.Context, target Target) ([]Listed, error) {
	response, err := l.postAntigravityList(ctx, target)
	if err != nil {
		return nil, err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		refusal, _ := io.ReadAll(io.LimitReader(response.Body, refusalBodyLimit))
		code, detail := antigravity.Refusal(refusal)
		return nil, antigravityRefusal(response.StatusCode, code, detail)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		// Any other refusal is the endpoint failing, not the credential: it
		// must not read as a rejection, or a transient outage would retire a
		// healthy account and wipe the roster it had.
		return nil, fmt.Errorf("%w %d", ErrListStatus, response.StatusCode)
	}
	var document antigravityModels
	if err := json.NewDecoder(response.Body).Decode(&document); err != nil {
		return nil, err
	}
	return truncate(antigravityModelsOf(document)), nil
}

func (l *Lister) postAntigravityList(ctx context.Context, target Target) (*http.Response, error) {
	body, err := antigravityBody(target.Auth.Project)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimSuffix(target.BaseURL, "/")+antigravityPath, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	if target.Auth.Token != "" {
		request.Header.Set("Authorization", "Bearer "+target.Auth.Token)
	}
	applyHeaders(request, target.Headers)
	return l.client.Do(request)
}

type antigravityServed struct {
	id     string
	name   string
	ctx    *int64
	max    *int64
	enum   string
	budget *int
}

func antigravityModelsOf(document antigravityModels) []Listed {
	served := collectAntigravityServed(document)
	grouped, singletons := groupAntigravityServed(served)
	models := make([]Listed, 0, len(grouped)+len(singletons))
	for logical, members := range grouped {
		models = append(models, antigravityGrouped(logical, members))
	}
	return append(models, singletons...)
}

func collectAntigravityServed(document antigravityModels) map[string]antigravityServed {
	agents := document.agentModelIDs()
	others := document.nonAgentModelIDs()
	served := make(map[string]antigravityServed, len(document.Models))
	for id, entry := range document.Models {
		if !antigravityServes(id, agents, others, document.DeprecatedModelIDs) {
			continue
		}
		item := antigravityServed{id: id, name: entry.DisplayName}
		if entry.MaxTokens != nil && *entry.MaxTokens > 0 {
			budget := *entry.MaxTokens
			item.ctx = &budget
		}
		if entry.MaxOutputTokens != nil && *entry.MaxOutputTokens > 0 {
			budget := *entry.MaxOutputTokens
			item.max = &budget
		}
		item.enum = entry.Model
		if entry.ThinkingBudget != nil {
			budget := *entry.ThinkingBudget
			item.budget = &budget
		}
		served[id] = item
	}
	return served
}

func groupAntigravityServed(served map[string]antigravityServed) (map[string][]antigravityServed, []Listed) {
	grouped := make(map[string][]antigravityServed)
	singletons := make([]Listed, 0, len(served))
	for _, item := range served {
		logical, found := antigravity.Logical(item.id)
		if !found && antigravity.Variants(item.id) != nil {
			logical, found = item.id, true
		}
		if !found || !isGeminiLogical(logical) {
			itemListed := Listed{ID: item.id, Name: item.name, ContextWindow: item.ctx, MaxOutput: item.max}
			if isClaudeModel(item.id) {
				noReasoning := false
				itemListed.SupportsReasoning = &noReasoning
				itemListed.ReasoningEfforts = []string{}
			}
			singletons = append(singletons, itemListed)
			continue
		}
		grouped[logical] = append(grouped[logical], item)
	}
	return grouped, singletons
}

func isGeminiLogical(logical string) bool {
	return len(logical) >= 6 && logical[:6] == "gemini"
}

func isClaudeModel(id string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(id)), "claude")
}

func antigravityGrouped(logical string, members []antigravityServed) Listed {
	listed := Listed{ID: logical, Name: antigravityGroupName(logical, members)}
	listed.ContextWindow, listed.MaxOutput = antigravityWidest(members)
	listed.RequestConfigs, listed.ReasoningEfforts = antigravityBandConfigs(logical, members)
	supported := true
	listed.SupportsReasoning = &supported
	return listed
}

func antigravityGroupName(logical string, members []antigravityServed) string {
	for _, member := range members {
		if member.name == "" {
			continue
		}
		if name := catalog.CleanAntigravityGeminiName(logical, member.name); name != "" {
			return name
		}
	}
	return catalog.CleanAntigravityGeminiName(logical, logical)
}

func antigravityWidest(members []antigravityServed) (*int64, *int64) {
	var widestCtx, widestMax *int64
	for _, member := range members {
		if member.ctx != nil && (widestCtx == nil || *member.ctx > *widestCtx) {
			budget := *member.ctx
			widestCtx = &budget
		}
		if member.max != nil && (widestMax == nil || *member.max > *widestMax) {
			budget := *member.max
			widestMax = &budget
		}
	}
	return widestCtx, widestMax
}

func antigravityBandConfigs(logical string, members []antigravityServed) ([]catalog.ModelRequestConfig, []string) {
	rows := antigravity.Variants(logical)
	var configs []catalog.ModelRequestConfig
	var efforts []string
	for _, band := range catalog.AntigravityBands() {
		variant, found := rows[band]
		if !found {
			continue
		}
		present := antigravityBandMember(members, variant.WireModel)
		if present == nil {
			continue
		}
		var ctx int64
		if present.ctx != nil {
			ctx = *present.ctx
		}
		enum := variant.ModelEnum
		if present.enum != "" {
			enum = present.enum
		}
		efforts = append(efforts, band)
		configs = append(configs, catalog.EncodeAntigravityConfig(catalog.AntigravityVariant{
			LogicalID: logical, Selection: band, Effort: band, Default: band == catalog.AntigravityBandMedium,
			WireModel: variant.WireModel, ModelEnum: enum, MaxOutput: variant.MaxOutput,
			ThinkingBudget: present.budget, Context: ctx, Reasoning: true,
		}))
	}
	return configs, efforts
}

func antigravityBandMember(members []antigravityServed, wire string) *antigravityServed {
	for i := range members {
		if members[i].id == wire {
			return &members[i]
		}
	}
	return nil
}

func (l *Lister) listBedrock(ctx context.Context, target Target) ([]Listed, error) {
	body, err := l.get(ctx, target, "/foundation-models?byOutputModality=TEXT")
	if err != nil {
		return nil, err
	}
	var doc struct {
		ModelSummaries []struct {
			ModelID   string `json:"modelId"`
			ModelName string `json:"modelName"`
		} `json:"modelSummaries"`
	}
	if err := decode(body, &doc); err != nil {
		return nil, err
	}
	models := make([]Listed, 0, len(doc.ModelSummaries))
	for _, m := range doc.ModelSummaries {
		models = append(models, Listed{
			ID:   m.ModelID,
			Name: m.ModelName,
		})
	}
	return truncate(models), nil
}

func (l *Lister) listKiro(_ context.Context, _ Target) ([]Listed, error) {
	return []Listed{
		{ID: "auto", Name: "Kiro Assistant"},
	}, nil
}

type codexModels struct {
	Models []struct {
		Slug           string `json:"slug"`
		DisplayName    string `json:"display_name"`
		ContextWindow  *int64 `json:"context_window"`
		SupportedInAPI bool   `json:"supported_in_api"`
		Visibility     string `json:"visibility"`
	} `json:"models"`
}

func (l *Lister) listCodex(ctx context.Context, target Target) ([]Listed, error) {
	values := url.Values{}
	values.Set("client_version", codex.Version)
	body, err := l.getWith(ctx, target, withQuery("/models", values), map[string]string{
		"OpenAI-Beta": codex.OpenAIBeta,
		"originator":  codex.Originator,
		"version":     codex.Version,
	})
	if err != nil {
		return nil, err
	}
	var document codexModels
	if err := decode(body, &document); err != nil {
		return nil, err
	}
	return truncate(codexModelsOf(document)), nil
}

func codexModelsOf(document codexModels) []Listed {
	models := make([]Listed, 0, len(document.Models))
	for _, entry := range document.Models {
		if strings.TrimSpace(entry.Slug) == "" || !entry.SupportedInAPI {
			continue
		}
		if strings.EqualFold(entry.Visibility, "hide") {
			continue
		}
		models = append(models, Listed{
			ID:            entry.Slug,
			Name:          entry.DisplayName,
			ContextWindow: entry.ContextWindow,
		})
	}
	return models
}

func (l *Lister) getWith(ctx context.Context, target Target, path string, headers map[string]string) ([]byte, error) {
	merged := Target{Format: target.Format, BaseURL: target.BaseURL, Auth: target.Auth,
		Headers: map[string]string{}}
	for name, value := range target.Headers {
		merged.Headers[name] = value
	}
	for name, value := range headers {
		merged.Headers[name] = value
	}
	return l.get(ctx, merged, path)
}

func firstOf(values ...*int64) *int64 {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}
