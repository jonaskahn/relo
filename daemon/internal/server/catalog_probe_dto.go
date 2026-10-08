// Catalog probe DTOs: the verification shapes the setup flow serves.
package server

import (
	appcatalog "github.com/jonaskahn/relo/internal/application/catalog"
)

type probeCheckResponse struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail"`
}

func toProbeCheckResponse(check appcatalog.ProbeCheck) probeCheckResponse {
	return probeCheckResponse{
		Name: check.Name, Status: check.Status, Detail: check.Detail,
	}
}

func toProbeCheckList(checks []appcatalog.ProbeCheck) []probeCheckResponse {
	if checks == nil {
		return nil
	}
	listed := make([]probeCheckResponse, 0, len(checks))
	for _, check := range checks {
		listed = append(listed, toProbeCheckResponse(check))
	}
	return listed
}

type probeModelResponse struct {
	ID            string              `json:"id"`
	Name          string              `json:"name"`
	ContextWindow *int64              `json:"context_window,omitempty"`
	MaxOutput     *int64              `json:"max_output,omitempty"`
	Match         string              `json:"match"`
	ModelsDevRef  string              `json:"modelsdev_ref"`
	Source        string              `json:"source"`
	Prices        modelPricesResponse `json:"prices"`
	Enabled       bool                `json:"enabled"`
}

func toProbeModelResponse(model appcatalog.ProbeModel) probeModelResponse {
	return probeModelResponse{
		ID: model.ID, Name: model.Name,
		ContextWindow: model.ContextWindow, MaxOutput: model.MaxOutput,
		Match: model.Match, ModelsDevRef: model.ModelsDevRef,
		Source: model.Source, Prices: toModelPricesResponse(model.Prices),
		Enabled: model.Enabled,
	}
}

func toProbeModelList(models []appcatalog.ProbeModel) []probeModelResponse {
	if models == nil {
		return nil
	}
	listed := make([]probeModelResponse, 0, len(models))
	for _, model := range models {
		listed = append(listed, toProbeModelResponse(model))
	}
	return listed
}

type manualModelRequest struct {
	ModelID  string `json:"model_id"`
	PricedAs string `json:"priced_as,omitempty"`
}

func toManualModelInput(request manualModelRequest) appcatalog.ManualModel {
	return appcatalog.ManualModel{
		ModelID: request.ModelID, PricedAs: request.PricedAs,
	}
}

func toManualModelList(requests []manualModelRequest) []appcatalog.ManualModel {
	if requests == nil {
		return nil
	}
	models := make([]appcatalog.ManualModel, 0, len(requests))
	for _, request := range requests {
		models = append(models, toManualModelInput(request))
	}
	return models
}

type probeCredentialRequest struct {
	Kind     string `json:"kind"`
	Secret   string `json:"secret"`
	Priority int    `json:"priority"`
}

type probeRequest struct {
	TemplateID   string                 `json:"template_id,omitempty"`
	ProviderID   string                 `json:"provider_id,omitempty"`
	CustomID     string                 `json:"custom_id,omitempty"`
	Label        string                 `json:"label,omitempty"`
	APIFormat    string                 `json:"api_format,omitempty"`
	BaseURL      string                 `json:"base_url,omitempty"`
	KeyHeader    string                 `json:"key_header,omitempty"`
	ModelsFormat string                 `json:"models_format,omitempty"`
	Variables    map[string]string      `json:"variables,omitempty"`
	Headers      map[string]string      `json:"headers,omitempty"`
	Credential   probeCredentialRequest `json:"credential"`
	ManualModels []manualModelRequest   `json:"manual_models,omitempty"`
}

func toProbeInput(request probeRequest) appcatalog.ProbeRequest {
	input := appcatalog.ProbeRequest{
		TemplateID: request.TemplateID, ProviderID: request.ProviderID,
		CustomID: request.CustomID, Label: request.Label,
		APIFormat: request.APIFormat, BaseURL: request.BaseURL,
		KeyHeader: request.KeyHeader, ModelsFormat: request.ModelsFormat,
		Variables: request.Variables, Headers: request.Headers,
		ManualModels: toManualModelList(request.ManualModels),
	}
	applyProbeCredential(&input, request.Credential)
	return input
}

func applyProbeCredential(input *appcatalog.ProbeRequest, credential probeCredentialRequest) {
	input.Credential.Kind = credential.Kind
	input.Credential.Secret = credential.Secret
	input.Credential.Priority = credential.Priority
}

type probeCountsResponse struct {
	Listed      int `json:"listed"`
	Matched     int `json:"matched"`
	Priced      int `json:"priced"`
	FromListing int `json:"from_listing"`
	FromManual  int `json:"from_manual"`
}

type probeResultResponse struct {
	ProbeID          string                 `json:"probe_id"`
	ExpiresAtMs      int64                  `json:"expires_at_ms"`
	Checks           []probeCheckResponse   `json:"checks"`
	Counts           probeCountsResponse    `json:"counts"`
	Models           []probeModelResponse   `json:"models"`
	ModelsDevState   modelsDevStateResponse `json:"modelsdev_state"`
	TargetProviderID string                 `json:"target_provider_id,omitempty"`
}

func toProbeResultResponse(result appcatalog.ProbeResult) probeResultResponse {
	return probeResultResponse{
		ProbeID: result.ProbeID, ExpiresAtMs: result.ExpiresAtMs,
		Checks: toProbeCheckList(result.Checks),
		Counts: probeCountsResponse{
			Listed: result.Counts.Listed, Matched: result.Counts.Matched,
			Priced: result.Counts.Priced, FromListing: result.Counts.FromListing,
			FromManual: result.Counts.FromManual,
		},
		Models:           toProbeModelList(result.Models),
		ModelsDevState:   toModelsDevStateResponse(result.ModelsDevState),
		TargetProviderID: result.TargetProviderID,
	}
}

type commitProbeRequest struct {
	ProviderID     string    `json:"provider_id"`
	Label          string    `json:"label"`
	AccountLabel   string    `json:"account_label"`
	DisabledModels *[]string `json:"disabled_models"`
}

func toCommitProbeInput(request commitProbeRequest) appcatalog.CommitProbeRequest {
	return appcatalog.CommitProbeRequest{
		ProviderID: request.ProviderID, Label: request.Label,
		AccountLabel: request.AccountLabel, DisabledModels: request.DisabledModels,
	}
}

type refreshResultResponse struct {
	Status          string                 `json:"status"`
	Listed          int                    `json:"listed"`
	Added           int                    `json:"added"`
	Unavailable     int                    `json:"unavailable"`
	Matched         int                    `json:"matched"`
	Priced          int                    `json:"priced"`
	Accounts        int                    `json:"accounts,omitempty"`
	AccountFailures []string               `json:"account_failures,omitempty"`
	ModelsDev       modelsDevStateResponse `json:"modelsdev"`
}

func toRefreshResultResponse(result appcatalog.RefreshResult) refreshResultResponse {
	return refreshResultResponse{
		Status: result.Status, Listed: result.Listed, Added: result.Added,
		Unavailable: result.Unavailable, Matched: result.Matched,
		Priced: result.Priced, Accounts: result.Accounts,
		AccountFailures: result.AccountFailures,
		ModelsDev:       toModelsDevStateResponse(result.ModelsDev),
	}
}

type catalogRefreshOutcomeResponse struct {
	ProviderID      string   `json:"provider_id"`
	Label           string   `json:"label"`
	Status          string   `json:"status"`
	Detail          string   `json:"detail,omitempty"`
	Listed          int      `json:"listed"`
	Added           int      `json:"added"`
	Unavailable     int      `json:"unavailable"`
	Matched         int      `json:"matched"`
	Priced          int      `json:"priced"`
	Accounts        int      `json:"accounts,omitempty"`
	AccountFailures []string `json:"account_failures,omitempty"`
}

func toCatalogRefreshOutcomeResponse(outcome appcatalog.CatalogRefreshOutcome) catalogRefreshOutcomeResponse {
	return catalogRefreshOutcomeResponse{
		ProviderID: outcome.ProviderID, Label: outcome.Label,
		Status: outcome.Status, Detail: outcome.Detail,
		Listed: outcome.Listed, Added: outcome.Added,
		Unavailable: outcome.Unavailable, Matched: outcome.Matched,
		Priced: outcome.Priced, Accounts: outcome.Accounts,
		AccountFailures: outcome.AccountFailures,
	}
}

func toCatalogRefreshOutcomeList(outcomes []appcatalog.CatalogRefreshOutcome) []catalogRefreshOutcomeResponse {
	if outcomes == nil {
		return nil
	}
	listed := make([]catalogRefreshOutcomeResponse, 0, len(outcomes))
	for _, outcome := range outcomes {
		listed = append(listed, toCatalogRefreshOutcomeResponse(outcome))
	}
	return listed
}

type catalogRefreshResultResponse struct {
	Metadata      pricesReportResponse            `json:"metadata"`
	MetadataError string                          `json:"metadata_error,omitempty"`
	Providers     []catalogRefreshOutcomeResponse `json:"providers"`
	Updated       int                             `json:"updated"`
	Skipped       int                             `json:"skipped"`
	Failed        int                             `json:"failed"`
}

func toCatalogRefreshResultResponse(result appcatalog.CatalogRefreshResult) catalogRefreshResultResponse {
	return catalogRefreshResultResponse{
		Metadata:      toPricesReportResponse(result.Metadata),
		MetadataError: result.MetadataError,
		Providers:     toCatalogRefreshOutcomeList(result.Providers),
		Updated:       result.Updated, Skipped: result.Skipped, Failed: result.Failed,
	}
}
