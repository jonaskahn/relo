// Account DTOs: the credential shapes the management API serves.
package server

import (
	appaccount "github.com/jonaskahn/relo/internal/application/account"
)

type accountResponse struct {
	ID             string `json:"id"`
	ProviderID     string `json:"provider_id"`
	Kind           string `json:"kind"`
	Label          string `json:"label"`
	Status         string `json:"status"`
	Priority       int    `json:"priority"`
	SecretMask     string `json:"secret_mask,omitempty"`
	LimitState     string `json:"limit_state"`
	LimitedUntilMs int64  `json:"limited_until_ms,omitempty"`
	ModelsKnown    bool   `json:"models_known"`
	ModelsCount    int    `json:"models_count"`
}

func toAccountResponse(account appaccount.Account) accountResponse {
	return accountResponse{
		ID: account.ID, ProviderID: account.ProviderID, Kind: account.Kind,
		Label: account.Label, Status: account.Status, Priority: account.Priority,
		SecretMask: account.SecretMask, LimitState: account.LimitState,
		LimitedUntilMs: account.LimitedUntilMs, ModelsKnown: account.ModelsKnown,
		ModelsCount: account.ModelsCount,
	}
}

func toAccountList(accounts []appaccount.Account) []accountResponse {
	listed := make([]accountResponse, 0, len(accounts))
	for _, account := range accounts {
		listed = append(listed, toAccountResponse(account))
	}
	return listed
}

type accountModelsResponse struct {
	CredentialID string   `json:"credential_id"`
	ProviderID   string   `json:"provider_id"`
	Known        bool     `json:"known"`
	Source       string   `json:"source,omitempty"`
	ObservedAtMs int64    `json:"observed_at_ms,omitempty"`
	Models       []string `json:"models"`
}

func toAccountModelsResponse(models appaccount.AccountModels) accountModelsResponse {
	return accountModelsResponse{
		CredentialID: models.CredentialID, ProviderID: models.ProviderID,
		Known: models.Known, Source: models.Source,
		ObservedAtMs: models.ObservedAtMs, Models: models.Models,
	}
}

type accountContextModelResponse struct {
	ModelID   string `json:"model_id"`
	Effective *int64 `json:"context_window"`
	Override  *int64 `json:"override"`
	Inherited *int64 `json:"inherited"`
	MaxInput  *int64 `json:"max_input"`
	Listed    bool   `json:"listed"`
}

type accountContextResponse struct {
	CredentialID string                        `json:"credential_id"`
	ProviderID   string                        `json:"provider_id"`
	Models       []accountContextModelResponse `json:"models"`
}

func toAccountContextResponse(context appaccount.AccountContext) accountContextResponse {
	models := make([]accountContextModelResponse, 0, len(context.Models))
	for _, model := range context.Models {
		models = append(models, accountContextModelResponse{
			ModelID: model.ModelID, Effective: model.Effective,
			Override: model.Override, Inherited: model.Inherited,
			MaxInput: model.MaxInput, Listed: model.Listed,
		})
	}
	return accountContextResponse{
		CredentialID: context.CredentialID, ProviderID: context.ProviderID,
		Models: models,
	}
}

type accountContextSkipResponse struct {
	ModelID string `json:"model_id"`
	Reason  string `json:"reason"`
}

type accountContextWriteResponse struct {
	Applied []string                     `json:"applied"`
	Skipped []accountContextSkipResponse `json:"skipped"`
}

func toAccountContextWriteResponse(write appaccount.AccountContextWrite) accountContextWriteResponse {
	skipped := make([]accountContextSkipResponse, 0, len(write.Skipped))
	for _, skip := range write.Skipped {
		skipped = append(skipped, accountContextSkipResponse{
			ModelID: skip.ModelID, Reason: skip.Reason,
		})
	}
	return accountContextWriteResponse{Applied: write.Applied, Skipped: skipped}
}
