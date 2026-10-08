// Catalog DTOs: the connection, model, and template shapes the API serves.
package server

import (
	appcatalog "github.com/jonaskahn/relo/internal/application/catalog"
	"github.com/jonaskahn/relo/internal/catalog"
)

type pricesResponse struct {
	Input         *int64 `json:"input"`
	Output        *int64 `json:"output"`
	CacheRead     *int64 `json:"cache_read"`
	CacheWrite    *int64 `json:"cache_write"`
	ExtThreshold  *int64 `json:"ext_threshold"`
	ExtInput      *int64 `json:"ext_input"`
	ExtOutput     *int64 `json:"ext_output"`
	ExtCacheRead  *int64 `json:"ext_cache_read"`
	ExtCacheWrite *int64 `json:"ext_cache_write"`
}

func toPricesResponse(prices appcatalog.Prices) pricesResponse {
	return pricesResponse{
		Input: prices.Input, Output: prices.Output,
		CacheRead: prices.CacheRead, CacheWrite: prices.CacheWrite,
		ExtThreshold: prices.ExtThreshold, ExtInput: prices.ExtInput,
		ExtOutput: prices.ExtOutput, ExtCacheRead: prices.ExtCacheRead,
		ExtCacheWrite: prices.ExtCacheWrite,
	}
}

type priceSourceMapResponse struct {
	Input         string `json:"input,omitempty"`
	Output        string `json:"output,omitempty"`
	CacheRead     string `json:"cache_read,omitempty"`
	CacheWrite    string `json:"cache_write,omitempty"`
	ExtThreshold  string `json:"ext_threshold,omitempty"`
	ExtInput      string `json:"ext_input,omitempty"`
	ExtOutput     string `json:"ext_output,omitempty"`
	ExtCacheRead  string `json:"ext_cache_read,omitempty"`
	ExtCacheWrite string `json:"ext_cache_write,omitempty"`
}

func toPriceSourceMapResponse(sources appcatalog.PriceSourceMap) priceSourceMapResponse {
	return priceSourceMapResponse{
		Input: sources.Input, Output: sources.Output,
		CacheRead: sources.CacheRead, CacheWrite: sources.CacheWrite,
		ExtThreshold: sources.ExtThreshold, ExtInput: sources.ExtInput,
		ExtOutput: sources.ExtOutput, ExtCacheRead: sources.ExtCacheRead,
		ExtCacheWrite: sources.ExtCacheWrite,
	}
}

type providerCountsResponse struct {
	Models          int `json:"models"`
	EnabledModels   int `json:"enabled_models"`
	AvailableModels int `json:"available_models"`
	UnpricedModels  int `json:"unpriced_models"`
	Accounts        int `json:"accounts"`
	ActiveAccounts  int `json:"active_accounts"`
	PausedAccounts  int `json:"paused_accounts"`
	ReauthAccounts  int `json:"reauth_accounts"`
}

func toProviderCountsResponse(counts appcatalog.ProviderCounts) providerCountsResponse {
	return providerCountsResponse{
		Models: counts.Models, EnabledModels: counts.EnabledModels,
		AvailableModels: counts.AvailableModels, UnpricedModels: counts.UnpricedModels,
		Accounts: counts.Accounts, ActiveAccounts: counts.ActiveAccounts,
		PausedAccounts: counts.PausedAccounts, ReauthAccounts: counts.ReauthAccounts,
	}
}

type formatOptionResponse struct {
	Format         catalog.APIFormat    `json:"format"`
	DefaultBaseURL string               `json:"default_base_url"`
	KeyHeader      catalog.KeyHeader    `json:"key_header"`
	ModelsFormat   catalog.ModelsFormat `json:"models_format"`
	Label          string               `json:"label"`
}

func toFormatOptionResponse(option catalog.FormatOption) formatOptionResponse {
	return formatOptionResponse{
		Format: option.Format, DefaultBaseURL: option.DefaultBaseURL,
		KeyHeader: option.KeyHeader, ModelsFormat: option.ModelsFormat,
		Label: option.Label,
	}
}

func toFormatOptionList(options []catalog.FormatOption) []formatOptionResponse {
	if options == nil {
		return nil
	}
	listed := make([]formatOptionResponse, 0, len(options))
	for _, option := range options {
		listed = append(listed, toFormatOptionResponse(option))
	}
	return listed
}

type variableResponse struct {
	Name        string   `json:"name"`
	Label       string   `json:"label"`
	Placeholder string   `json:"placeholder"`
	Required    bool     `json:"required"`
	Options     []string `json:"options,omitempty"`
}

func toVariableResponse(variable catalog.Variable) variableResponse {
	return variableResponse{
		Name: variable.Name, Label: variable.Label,
		Placeholder: variable.Placeholder, Required: variable.Required,
		Options: variable.Options,
	}
}

func toVariableList(variables []catalog.Variable) []variableResponse {
	if variables == nil {
		return nil
	}
	listed := make([]variableResponse, 0, len(variables))
	for _, variable := range variables {
		listed = append(listed, toVariableResponse(variable))
	}
	return listed
}

type loginMethodResponse struct {
	Flow string            `json:"flow"`
	Kind catalog.LoginKind `json:"kind"`
}

func toLoginMethodResponse(method catalog.LoginMethod) loginMethodResponse {
	return loginMethodResponse{Flow: method.Flow, Kind: method.Kind}
}

func toLoginMethodList(methods []catalog.LoginMethod) []loginMethodResponse {
	if methods == nil {
		return nil
	}
	listed := make([]loginMethodResponse, 0, len(methods))
	for _, method := range methods {
		listed = append(listed, toLoginMethodResponse(method))
	}
	return listed
}

type providerResponse struct {
	ID                  string                 `json:"id"`
	TemplateID          string                 `json:"template_id"`
	Label               string                 `json:"label"`
	Kind                string                 `json:"kind"`
	AvailableFormats    []formatOptionResponse `json:"available_formats"`
	VariableDefs        []variableResponse     `json:"variable_defs"`
	Origin              string                 `json:"origin"`
	Auth                string                 `json:"auth"`
	APIFormat           string                 `json:"api_format"`
	APIFormats          []string               `json:"api_formats"`
	KeyHeader           string                 `json:"key_header"`
	ModelsSource        string                 `json:"models_source"`
	ModelsFormat        string                 `json:"models_format"`
	ModelsPerAccount    bool                   `json:"models_per_account"`
	ModelsDevProviderID string                 `json:"modelsdev_provider_id"`
	BaseURL             string                 `json:"base_url"`
	DocURL              string                 `json:"doc_url"`
	KeyEnv              []string               `json:"key_env"`
	LoginFlows          []string               `json:"login_flows"`
	LoginMethods        []loginMethodResponse  `json:"login_methods,omitempty"`
	Headers             map[string]string      `json:"headers"`
	Variables           map[string]string      `json:"variables"`
	NeedsSetup          []string               `json:"needs_setup"`
	Routable            bool                   `json:"routable"`
	UnroutableReason    string                 `json:"unroutable_reason"`
	Enabled             bool                   `json:"enabled"`
	UseProxy            bool                   `json:"use_proxy"`
	TimeoutSeconds      *int                   `json:"timeout_seconds"`
	RetryBackoff        [][2]int               `json:"retry_backoff"`
	SwitchOn4xx         bool                   `json:"switch_on_4xx"`
	SwitchOn5xx         bool                   `json:"switch_on_5xx"`
	Rank                int                    `json:"rank"`
	PoolStrategy        string                 `json:"pool_strategy"`
	Configured          bool                   `json:"configured"`
	LastRefreshedAtMs   *int64                 `json:"last_refreshed_at_ms,omitempty"`
	LastRefreshError    string                 `json:"last_refresh_error,omitempty"`
	Counts              providerCountsResponse `json:"counts"`
	CreatedAtMs         int64                  `json:"created_at_ms"`
	UpdatedAtMs         int64                  `json:"updated_at_ms"`
}

func toProviderResponse(provider appcatalog.Provider) providerResponse {
	return providerResponse{
		ID: provider.ID, TemplateID: provider.TemplateID, Label: provider.Label,
		Kind: provider.Kind, AvailableFormats: toFormatOptionList(provider.AvailableFormats),
		VariableDefs: toVariableList(provider.VariableDefs),
		Origin:       provider.Origin, Auth: provider.Auth,
		APIFormat: provider.APIFormat, APIFormats: provider.APIFormats,
		KeyHeader: provider.KeyHeader, ModelsSource: provider.ModelsSource,
		ModelsFormat: provider.ModelsFormat, ModelsPerAccount: provider.ModelsPerAccount,
		ModelsDevProviderID: provider.ModelsDevProviderID, BaseURL: provider.BaseURL,
		DocURL: provider.DocURL, KeyEnv: provider.KeyEnv, LoginFlows: provider.LoginFlows,
		LoginMethods: toLoginMethodList(provider.LoginMethods),
		Headers:      provider.Headers, Variables: provider.Variables,
		NeedsSetup: provider.NeedsSetup, Routable: provider.Routable,
		UnroutableReason: provider.UnroutableReason, Enabled: provider.Enabled,
		UseProxy: provider.UseProxy, TimeoutSeconds: provider.TimeoutSeconds,
		RetryBackoff: provider.RetryBackoff,
		SwitchOn4xx:  provider.SwitchOn4xx, SwitchOn5xx: provider.SwitchOn5xx,
		Rank: provider.Rank, PoolStrategy: provider.PoolStrategy,
		Configured:        provider.Configured,
		LastRefreshedAtMs: provider.LastRefreshedAtMs,
		LastRefreshError:  provider.LastRefreshError,
		Counts:            toProviderCountsResponse(provider.Counts),
		CreatedAtMs:       provider.CreatedAtMs, UpdatedAtMs: provider.UpdatedAtMs,
	}
}

func toProviderList(providers []appcatalog.Provider) []providerResponse {
	listed := make([]providerResponse, 0, len(providers))
	for _, provider := range providers {
		listed = append(listed, toProviderResponse(provider))
	}
	return listed
}

type capabilitiesResponse struct {
	Tools     *bool `json:"tools"`
	Reasoning *bool `json:"reasoning"`
	Vision    *bool `json:"vision"`
}

func toCapabilitiesResponse(flags appcatalog.Capabilities) capabilitiesResponse {
	return capabilitiesResponse{
		Tools: flags.Tools, Reasoning: flags.Reasoning, Vision: flags.Vision,
	}
}

type detailLayerResponse struct {
	Name          *string `json:"name"`
	Description   *string `json:"description"`
	Family        *string `json:"family"`
	Category      *string `json:"category"`
	ContextWindow *int64  `json:"context_window"`
	MaxInput      *int64  `json:"max_input"`
	MaxOutput     *int64  `json:"max_output"`
	Tools         *bool   `json:"tools"`
	Reasoning     *bool   `json:"reasoning"`
	Vision        *bool   `json:"vision"`
	Status        *string `json:"status"`
	ReleaseDate   *string `json:"release_date"`
}

func toDetailLayerResponse(layer appcatalog.DetailLayer) detailLayerResponse {
	return detailLayerResponse{
		Name: layer.Name, Description: layer.Description,
		Family: layer.Family, Category: layer.Category,
		ContextWindow: layer.ContextWindow, MaxInput: layer.MaxInput,
		MaxOutput: layer.MaxOutput, Tools: layer.Tools,
		Reasoning: layer.Reasoning, Vision: layer.Vision,
		Status: layer.Status, ReleaseDate: layer.ReleaseDate,
	}
}

type detailSourceMapResponse struct {
	Name          string `json:"name,omitempty"`
	Description   string `json:"description,omitempty"`
	Family        string `json:"family,omitempty"`
	Category      string `json:"category,omitempty"`
	ContextWindow string `json:"context_window,omitempty"`
	MaxInput      string `json:"max_input,omitempty"`
	MaxOutput     string `json:"max_output,omitempty"`
	Tools         string `json:"tools,omitempty"`
	Reasoning     string `json:"reasoning,omitempty"`
	Vision        string `json:"vision,omitempty"`
	Status        string `json:"status,omitempty"`
	ReleaseDate   string `json:"release_date,omitempty"`
}

func toDetailSourceMapResponse(sources appcatalog.DetailSourceMap) detailSourceMapResponse {
	return detailSourceMapResponse{
		Name: sources.Name, Description: sources.Description,
		Family: sources.Family, Category: sources.Category,
		ContextWindow: sources.ContextWindow, MaxInput: sources.MaxInput,
		MaxOutput: sources.MaxOutput, Tools: sources.Tools,
		Reasoning: sources.Reasoning, Vision: sources.Vision,
		Status: sources.Status, ReleaseDate: sources.ReleaseDate,
	}
}

type modelDetailsResponse struct {
	Override        detailLayerResponse     `json:"override"`
	Provider        detailLayerResponse     `json:"provider"`
	ModelsDev       detailLayerResponse     `json:"modelsdev"`
	EffectiveSource detailSourceMapResponse `json:"effective_source"`
}

func toModelDetailsResponse(details appcatalog.ModelDetails) modelDetailsResponse {
	return modelDetailsResponse{
		Override:        toDetailLayerResponse(details.Override),
		Provider:        toDetailLayerResponse(details.Provider),
		ModelsDev:       toDetailLayerResponse(details.ModelsDev),
		EffectiveSource: toDetailSourceMapResponse(details.EffectiveSource),
	}
}

func toModelDetailsOrNil(details *appcatalog.ModelDetails) *modelDetailsResponse {
	if details == nil {
		return nil
	}
	mapped := toModelDetailsResponse(*details)
	return &mapped
}

type modelPricesResponse struct {
	Override        pricesResponse         `json:"override"`
	Provider        pricesResponse         `json:"provider"`
	ModelsDev       pricesResponse         `json:"modelsdev"`
	Effective       pricesResponse         `json:"effective"`
	EffectiveSource priceSourceMapResponse `json:"effective_source"`
}

func toModelPricesResponse(prices appcatalog.ModelPrices) modelPricesResponse {
	return modelPricesResponse{
		Override:        toPricesResponse(prices.Override),
		Provider:        toPricesResponse(prices.Provider),
		ModelsDev:       toPricesResponse(prices.ModelsDev),
		Effective:       toPricesResponse(prices.Effective),
		EffectiveSource: toPriceSourceMapResponse(prices.EffectiveSource),
	}
}

type contextLayersResponse struct {
	Override  *int64 `json:"override"`
	Provider  *int64 `json:"provider"`
	ModelsDev *int64 `json:"modelsdev"`
}

func toContextLayersResponse(layers appcatalog.ContextLayers) contextLayersResponse {
	return contextLayersResponse{
		Override: layers.Override, Provider: layers.Provider,
		ModelsDev: layers.ModelsDev,
	}
}

type modelResponse struct {
	ProviderID         string                `json:"provider_id"`
	ModelID            string                `json:"model_id"`
	UpstreamModelID    string                `json:"upstream_model_id"`
	ClonedFrom         string                `json:"cloned_from"`
	Source             string                `json:"source"`
	Name               string                `json:"name"`
	Description        string                `json:"description"`
	Family             string                `json:"family"`
	Category           string                `json:"category"`
	APIFormat          string                `json:"api_format"`
	BaseURL            string                `json:"base_url"`
	ModelsDevRef       string                `json:"modelsdev_ref"`
	Match              string                `json:"match"`
	ContextWindow      *int64                `json:"context_window"`
	MaxInput           *int64                `json:"max_input"`
	MaxOutput          *int64                `json:"max_output"`
	ContextLayers      contextLayersResponse `json:"context_layers"`
	ContextSource      string                `json:"context_source"`
	MaxOutputLayers    contextLayersResponse `json:"max_output_layers"`
	MaxOutputSource    string                `json:"max_output_source"`
	Capabilities       capabilitiesResponse  `json:"capabilities"`
	CapabilityOverride capabilitiesResponse  `json:"capability_override"`
	Status             string                `json:"status"`
	ReleaseDate        string                `json:"release_date"`
	Enabled            bool                  `json:"enabled"`
	Available          *bool                 `json:"available"`
	Routable           bool                  `json:"routable"`
	UnroutableReason   string                `json:"unroutable_reason"`
	Prices             modelPricesResponse   `json:"prices"`
	GroupRefs          []string              `json:"group_refs"`
	Overridden         bool                  `json:"overridden"`
	Details            *modelDetailsResponse `json:"details,omitempty"`
	ListedAtMs         *int64                `json:"listed_at_ms,omitempty"`
	UpdatedAtMs        int64                 `json:"updated_at_ms"`
	ServingAccounts    int                   `json:"serving_accounts"`
	ActiveAccounts     int                   `json:"active_accounts"`
}

func toModelResponse(model appcatalog.Model) modelResponse {
	return modelResponse{
		ProviderID: model.ProviderID, ModelID: model.ModelID,
		UpstreamModelID: model.UpstreamModelID, ClonedFrom: model.ClonedFrom,
		Source: model.Source, Name: model.Name, Description: model.Description,
		Family: model.Family, Category: model.Category,
		APIFormat: model.APIFormat, BaseURL: model.BaseURL,
		ModelsDevRef: model.ModelsDevRef, Match: model.Match,
		ContextWindow: model.ContextWindow, MaxInput: model.MaxInput,
		MaxOutput:          model.MaxOutput,
		ContextLayers:      toContextLayersResponse(model.ContextLayers),
		ContextSource:      model.ContextSource,
		MaxOutputLayers:    toContextLayersResponse(model.MaxOutputLayers),
		MaxOutputSource:    model.MaxOutputSource,
		Capabilities:       toCapabilitiesResponse(model.Capabilities),
		CapabilityOverride: toCapabilitiesResponse(model.CapabilityOverride),
		Status:             model.Status, ReleaseDate: model.ReleaseDate,
		Enabled: model.Enabled, Available: model.Available,
		Routable: model.Routable, UnroutableReason: model.UnroutableReason,
		Prices: toModelPricesResponse(model.Prices), GroupRefs: model.GroupRefs,
		Overridden: model.Overridden,
		Details:    toModelDetailsOrNil(model.Details),
		ListedAtMs: model.ListedAtMs, UpdatedAtMs: model.UpdatedAtMs,
		ServingAccounts: model.ServingAccounts, ActiveAccounts: model.ActiveAccounts,
	}
}

func toModelList(models []appcatalog.Model) []modelResponse {
	listed := make([]modelResponse, 0, len(models))
	for _, model := range models {
		listed = append(listed, toModelResponse(model))
	}
	return listed
}

type groupMemberResponse struct {
	ProviderID      string `json:"provider_id"`
	ModelID         string `json:"model_id"`
	Kind            string `json:"kind"`
	Weight          int    `json:"weight"`
	Enabled         bool   `json:"enabled"`
	Eligible        bool   `json:"eligible"`
	Reason          string `json:"reason,omitempty"`
	ServingAccounts int    `json:"serving_accounts"`
	ActiveAccounts  int    `json:"active_accounts"`
}

func toGroupMemberResponse(member appcatalog.GroupMember) groupMemberResponse {
	return groupMemberResponse{
		ProviderID: member.ProviderID, ModelID: member.ModelID,
		Kind: member.Kind, Weight: member.Weight, Enabled: member.Enabled,
		Eligible: member.Eligible, Reason: member.Reason,
		ServingAccounts: member.ServingAccounts, ActiveAccounts: member.ActiveAccounts,
	}
}

func toGroupMemberList(members []appcatalog.GroupMember) []groupMemberResponse {
	if members == nil {
		return nil
	}
	listed := make([]groupMemberResponse, 0, len(members))
	for _, member := range members {
		listed = append(listed, toGroupMemberResponse(member))
	}
	return listed
}

type groupResponse struct {
	ID                 string                `json:"id"`
	Label              string                `json:"label"`
	Strategy           string                `json:"strategy"`
	Enabled            bool                  `json:"enabled"`
	Listed             bool                  `json:"listed"`
	ClientID           string                `json:"client_id"`
	ShadowsModel       bool                  `json:"shadows_model"`
	Members            []groupMemberResponse `json:"members"`
	SwitchOn4xx        bool                  `json:"switch_on_4xx"`
	SwitchOn5xx        bool                  `json:"switch_on_5xx"`
	ContextWindow      *int64                `json:"context_window"`
	MaxOutput          *int64                `json:"max_output"`
	CapabilityWarnings []string              `json:"capability_warnings"`
	SupportsTools      *bool                 `json:"supports_tools"`
	SupportsReasoning  *bool                 `json:"supports_reasoning"`
	SupportsVision     *bool                 `json:"supports_vision"`
	CreatedAtMs        int64                 `json:"created_at_ms"`
	UpdatedAtMs        int64                 `json:"updated_at_ms"`
}

func toGroupResponse(group appcatalog.Group) groupResponse {
	return groupResponse{
		ID: group.ID, Label: group.Label, Strategy: group.Strategy,
		Enabled: group.Enabled, Listed: group.Listed,
		ClientID: group.ClientID, ShadowsModel: group.ShadowsModel,
		Members:     toGroupMemberList(group.Members),
		SwitchOn4xx: group.SwitchOn4xx, SwitchOn5xx: group.SwitchOn5xx,
		ContextWindow: group.ContextWindow, MaxOutput: group.MaxOutput,
		CapabilityWarnings: group.CapabilityWarnings,
		SupportsTools:      group.SupportsTools,
		SupportsReasoning:  group.SupportsReasoning,
		SupportsVision:     group.SupportsVision,
		CreatedAtMs:        group.CreatedAtMs, UpdatedAtMs: group.UpdatedAtMs,
	}
}

func toGroupList(groups []appcatalog.Group) []groupResponse {
	listed := make([]groupResponse, 0, len(groups))
	for _, group := range groups {
		listed = append(listed, toGroupResponse(group))
	}
	return listed
}

type templateResponse struct {
	ID                     string                 `json:"id"`
	Label                  string                 `json:"label"`
	Kind                   catalog.Kind           `json:"kind"`
	Origin                 catalog.Origin         `json:"origin"`
	Auth                   catalog.Auth           `json:"auth"`
	KeyHeader              catalog.KeyHeader      `json:"key_header"`
	DefaultFormat          catalog.APIFormat      `json:"default_format"`
	AvailableFormats       []formatOptionResponse `json:"available_formats"`
	DefaultBaseURL         string                 `json:"default_base_url"`
	RequiresStream         bool                   `json:"requires_stream,omitempty"`
	RefusesMaxOutputTokens bool                   `json:"refuses_max_output_tokens,omitempty"`
	Variables              []variableResponse     `json:"variables,omitempty"`
	ModelsSource           string                 `json:"models_source"`
	ModelsFormat           catalog.ModelsFormat   `json:"models_format"`
	ModelsDevProviderID    string                 `json:"modelsdev_provider_id,omitempty"`
	DocURL                 string                 `json:"doc_url,omitempty"`
	KeyEnv                 []string               `json:"key_env,omitempty"`
	LoginFlows             []string               `json:"login_flows,omitempty"`
	LoginMethods           []loginMethodResponse  `json:"login_methods,omitempty"`
	Headers                map[string]string      `json:"headers,omitempty"`
	UnsupportedReason      string                 `json:"unsupported_reason,omitempty"`
	ModelsDevModels        int                    `json:"modelsdev_models"`
}

func toTemplateResponse(template catalog.Template) templateResponse {
	return templateResponse{
		ID: template.ID, Label: template.Label, Kind: template.Kind,
		Origin: template.Origin, Auth: template.Auth,
		KeyHeader: template.KeyHeader, DefaultFormat: template.DefaultFormat,
		AvailableFormats:       toFormatOptionList(template.AvailableFormats),
		DefaultBaseURL:         template.DefaultBaseURL,
		RequiresStream:         template.RequiresStream,
		RefusesMaxOutputTokens: template.RefusesMaxOutputTokens,
		Variables:              toVariableList(template.Variables),
		ModelsSource:           template.ModelsSource, ModelsFormat: template.ModelsFormat,
		ModelsDevProviderID: template.ModelsDevProviderID,
		DocURL:              template.DocURL, KeyEnv: template.KeyEnv,
		LoginFlows:        template.LoginFlows,
		LoginMethods:      toLoginMethodList(template.LoginMethods),
		Headers:           template.Headers,
		UnsupportedReason: template.UnsupportedReason,
		ModelsDevModels:   template.ModelsDevModels,
	}
}

func toTemplateList(templates []catalog.Template) []templateResponse {
	if templates == nil {
		return nil
	}
	listed := make([]templateResponse, 0, len(templates))
	for _, template := range templates {
		listed = append(listed, toTemplateResponse(template))
	}
	return listed
}

type modelsDevStateResponse struct {
	SourceURL       string `json:"source_url"`
	FetchedAtMs     int64  `json:"fetched_at_ms"`
	Stale           bool   `json:"stale"`
	Available       bool   `json:"available"`
	ETag            string `json:"etag,omitempty"`
	LastModified    string `json:"last_modified,omitempty"`
	ProvidersCount  int    `json:"providers_count"`
	ModelsCount     int    `json:"models_count"`
	LastAttemptAtMs int64  `json:"last_attempt_at_ms"`
	LastError       string `json:"last_error,omitempty"`
}

func toModelsDevStateResponse(state catalog.ModelsDevState) modelsDevStateResponse {
	return modelsDevStateResponse{
		SourceURL: state.SourceURL, FetchedAtMs: state.FetchedAtMs,
		Stale: state.Stale, Available: state.Available,
		ETag: state.ETag, LastModified: state.LastModified,
		ProvidersCount: state.ProvidersCount, ModelsCount: state.ModelsCount,
		LastAttemptAtMs: state.LastAttemptAtMs, LastError: state.LastError,
	}
}

type modelsDevSearchHitResponse struct {
	Ref           string         `json:"ref"`
	ProviderID    string         `json:"provider_id"`
	ProviderName  string         `json:"provider_name"`
	ModelID       string         `json:"model_id"`
	Name          string         `json:"name"`
	ContextWindow *int64         `json:"context_window"`
	Prices        pricesResponse `json:"prices"`
}

func toModelsDevSearchHitResponse(hit appcatalog.ModelsDevSearchHit) modelsDevSearchHitResponse {
	return modelsDevSearchHitResponse{
		Ref: hit.Ref, ProviderID: hit.ProviderID,
		ProviderName: hit.ProviderName, ModelID: hit.ModelID, Name: hit.Name,
		ContextWindow: hit.ContextWindow, Prices: toPricesResponse(hit.Prices),
	}
}

func toModelsDevSearchHitList(hits []appcatalog.ModelsDevSearchHit) []modelsDevSearchHitResponse {
	if hits == nil {
		return nil
	}
	listed := make([]modelsDevSearchHitResponse, 0, len(hits))
	for _, hit := range hits {
		listed = append(listed, toModelsDevSearchHitResponse(hit))
	}
	return listed
}

type pricesReportResponse struct {
	FetchedAtMs     int64                  `json:"fetched_at_ms"`
	Providers       int                    `json:"providers"`
	ModelsMatched   int                    `json:"models_matched"`
	PricesChanged   int                    `json:"prices_changed"`
	ModelsUnmatched int                    `json:"models_unmatched"`
	ModelsDev       modelsDevStateResponse `json:"modelsdev"`
}

func toPricesReportResponse(report appcatalog.PricesReport) pricesReportResponse {
	return pricesReportResponse{
		FetchedAtMs: report.FetchedAtMs, Providers: report.Providers,
		ModelsMatched: report.ModelsMatched, PricesChanged: report.PricesChanged,
		ModelsUnmatched: report.ModelsUnmatched,
		ModelsDev:       toModelsDevStateResponse(report.ModelsDev),
	}
}

type pricesRequest struct {
	Input         *int64 `json:"input"`
	Output        *int64 `json:"output"`
	CacheRead     *int64 `json:"cache_read"`
	CacheWrite    *int64 `json:"cache_write"`
	ExtThreshold  *int64 `json:"ext_threshold"`
	ExtInput      *int64 `json:"ext_input"`
	ExtOutput     *int64 `json:"ext_output"`
	ExtCacheRead  *int64 `json:"ext_cache_read"`
	ExtCacheWrite *int64 `json:"ext_cache_write"`
}

func toPricesInput(request *pricesRequest) *appcatalog.Prices {
	if request == nil {
		return nil
	}
	return &appcatalog.Prices{
		Input: request.Input, Output: request.Output,
		CacheRead: request.CacheRead, CacheWrite: request.CacheWrite,
		ExtThreshold: request.ExtThreshold, ExtInput: request.ExtInput,
		ExtOutput: request.ExtOutput, ExtCacheRead: request.ExtCacheRead,
		ExtCacheWrite: request.ExtCacheWrite,
	}
}

type modelOverrideRequest struct {
	Name          *string        `json:"name"`
	Description   *string        `json:"description"`
	Category      *string        `json:"category"`
	ContextWindow *int64         `json:"context_window"`
	MaxInput      *int64         `json:"max_input"`
	MaxOutput     *int64         `json:"max_output"`
	Tools         *bool          `json:"tools"`
	Reasoning     *bool          `json:"reasoning"`
	Vision        *bool          `json:"vision"`
	Prices        *pricesRequest `json:"prices"`
}

func toModelOverrideInput(request *modelOverrideRequest) *appcatalog.ModelOverride {
	if request == nil {
		return nil
	}
	return &appcatalog.ModelOverride{
		Name: request.Name, Description: request.Description,
		Category: request.Category, ContextWindow: request.ContextWindow,
		MaxInput: request.MaxInput, MaxOutput: request.MaxOutput,
		Tools: request.Tools, Reasoning: request.Reasoning,
		Vision: request.Vision, Prices: toPricesInput(request.Prices),
	}
}
