// Integration DTOs: the coding-client shapes the management API serves.
package server

import (
	appintegration "github.com/jonaskahn/relo/internal/application/integration"
)

type integrationKeyResponse struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Hint         string `json:"token_hint"`
	Status       string `json:"status"`
	CreatedAtMs  int64  `json:"created_at_ms"`
	LastUsedAtMs int64  `json:"last_used_at_ms,omitempty"`
}

func toIntegrationKeyResponse(key appintegration.IntegrationKey) integrationKeyResponse {
	return integrationKeyResponse{
		ID: key.ID, Name: key.Name, Hint: key.Hint, Status: key.Status,
		CreatedAtMs: key.CreatedAtMs, LastUsedAtMs: key.LastUsedAtMs,
	}
}

func toIntegrationKeyOrNil(key *appintegration.IntegrationKey) *integrationKeyResponse {
	if key == nil {
		return nil
	}
	mapped := toIntegrationKeyResponse(*key)
	return &mapped
}

type fileViewResponse struct {
	Kind     string `json:"kind"`
	Path     string `json:"path"`
	Present  bool   `json:"present"`
	Drifted  bool   `json:"drifted"`
	Snapshot string `json:"snapshot_path,omitempty"`
}

func toFileViewResponse(file appintegration.FileView) fileViewResponse {
	return fileViewResponse{
		Kind: file.Kind, Path: file.Path, Present: file.Present,
		Drifted: file.Drifted, Snapshot: file.Snapshot,
	}
}

func toFileViewList(files []appintegration.FileView) []fileViewResponse {
	if files == nil {
		return nil
	}
	listed := make([]fileViewResponse, 0, len(files))
	for _, file := range files {
		listed = append(listed, toFileViewResponse(file))
	}
	return listed
}

type plannedFileResponse struct {
	Kind     string `json:"kind"`
	Path     string `json:"path"`
	Fragment string `json:"fragment,omitempty"`
	Refused  bool   `json:"refused,omitempty"`
	Reason   string `json:"reason,omitempty"`
	Code     string `json:"code,omitempty"`
}

func toPlannedFileResponse(planned appintegration.PlannedFile) plannedFileResponse {
	return plannedFileResponse{
		Kind: planned.Kind, Path: planned.Path, Fragment: planned.Fragment,
		Refused: planned.Refused, Reason: planned.Reason, Code: planned.Code,
	}
}

func toPlannedFileList(planned []appintegration.PlannedFile) []plannedFileResponse {
	if planned == nil {
		return nil
	}
	listed := make([]plannedFileResponse, 0, len(planned))
	for _, file := range planned {
		listed = append(listed, toPlannedFileResponse(file))
	}
	return listed
}

func toStringList(values []string) []string {
	if values == nil {
		return nil
	}
	return append([]string(nil), values...)
}

type integrationViewResponse struct {
	ID            string                  `json:"id"`
	Client        string                  `json:"client"`
	Summary       string                  `json:"summary"`
	Protocol      string                  `json:"protocol"`
	BaseURL       string                  `json:"base_url,omitempty"`
	ManagesFiles  bool                    `json:"manages_files"`
	Enabled       bool                    `json:"enabled"`
	State         string                  `json:"state"`
	LastError     string                  `json:"last_error,omitempty"`
	Key           *integrationKeyResponse `json:"key,omitempty"`
	Files         []fileViewResponse      `json:"files"`
	Preview       []plannedFileResponse   `json:"preview,omitempty"`
	Steps         []string                `json:"steps,omitempty"`
	UpdatedAtMs   int64                   `json:"updated_at_ms,omitempty"`
	ModelsStale   bool                    `json:"models_stale,omitempty"`
	RestartNeeded bool                    `json:"restart_needed,omitempty"`
	EnvKey        string                  `json:"env_key,omitempty"`
	Context1M     *bool                   `json:"context_1m,omitempty"`
}

func toIntegrationViewResponse(view appintegration.IntegrationView) integrationViewResponse {
	return integrationViewResponse{
		ID: view.ID, Client: view.Client, Summary: view.Summary,
		Protocol: view.Protocol, BaseURL: view.BaseURL,
		ManagesFiles: view.ManagesFiles, Enabled: view.Enabled,
		State: view.State, LastError: view.LastError,
		Key: toIntegrationKeyOrNil(view.Key), Files: toFileViewList(view.Files),
		Preview: toPlannedFileList(view.Preview), Steps: toStringList(view.Steps),
		UpdatedAtMs: view.UpdatedAtMs, ModelsStale: view.ModelsStale,
		RestartNeeded: view.RestartNeeded, EnvKey: view.EnvKey,
		Context1M: view.Context1M,
	}
}

func toIntegrationViewList(views []appintegration.IntegrationView) []integrationViewResponse {
	listed := make([]integrationViewResponse, 0, len(views))
	for _, view := range views {
		listed = append(listed, toIntegrationViewResponse(view))
	}
	return listed
}

type integrationResultResponse struct {
	Integration integrationViewResponse `json:"integration"`
	Token       string                  `json:"token,omitempty"`
}

func toIntegrationResultResponse(result appintegration.IntegrationResult) integrationResultResponse {
	return integrationResultResponse{
		Integration: toIntegrationViewResponse(result.Integration),
		Token:       result.Token,
	}
}

type verifyCheckResponse struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail,omitempty"`
	Fixed  bool   `json:"fixed,omitempty"`
}

func toVerifyCheckResponse(check appintegration.VerifyCheck) verifyCheckResponse {
	return verifyCheckResponse{
		Name: check.Name, OK: check.OK,
		Detail: check.Detail, Fixed: check.Fixed,
	}
}

func toVerifyCheckList(checks []appintegration.VerifyCheck) []verifyCheckResponse {
	if checks == nil {
		return nil
	}
	listed := make([]verifyCheckResponse, 0, len(checks))
	for _, check := range checks {
		listed = append(listed, toVerifyCheckResponse(check))
	}
	return listed
}

type integrationVerifyResponse struct {
	OK      bool                  `json:"ok"`
	URL     string                `json:"url"`
	Status  int                   `json:"status"`
	Models  int                   `json:"models"`
	Message string                `json:"message"`
	Config  bool                  `json:"config,omitempty"`
	Checks  []verifyCheckResponse `json:"checks,omitempty"`
}

func toIntegrationVerifyResponse(verify appintegration.IntegrationVerify) integrationVerifyResponse {
	return integrationVerifyResponse{
		OK: verify.OK, URL: verify.URL, Status: verify.Status,
		Models: verify.Models, Message: verify.Message, Config: verify.Config,
		Checks: toVerifyCheckList(verify.Checks),
	}
}

type integrationModelResponse struct {
	ID            string `json:"id"`
	Name          string `json:"name,omitempty"`
	ContextWindow *int64 `json:"context_window,omitempty"`
	ProviderID    string `json:"provider_id,omitempty"`
	SourceModelID string `json:"source_model_id,omitempty"`
}

func toIntegrationModelResponse(model appintegration.IntegrationModel) integrationModelResponse {
	return integrationModelResponse{
		ID: model.ID, Name: model.Name, ContextWindow: model.ContextWindow,
		ProviderID: model.ProviderID, SourceModelID: model.SourceModelID,
	}
}

func toIntegrationModelList(models []appintegration.IntegrationModel) []integrationModelResponse {
	if models == nil {
		return nil
	}
	listed := make([]integrationModelResponse, 0, len(models))
	for _, model := range models {
		listed = append(listed, toIntegrationModelResponse(model))
	}
	return listed
}

type integrationModelsResponse struct {
	OK      bool                       `json:"ok"`
	URL     string                     `json:"url"`
	Status  int                        `json:"status"`
	Models  []integrationModelResponse `json:"models"`
	Message string                     `json:"message"`
}

func toIntegrationModelsResponse(read appintegration.IntegrationModels) integrationModelsResponse {
	return integrationModelsResponse{
		OK: read.OK, URL: read.URL, Status: read.Status,
		Models: toIntegrationModelList(read.Models), Message: read.Message,
	}
}

type integrationChatResponse struct {
	OK         bool     `json:"ok"`
	Status     int      `json:"status"`
	Model      string   `json:"model"`
	Text       string   `json:"text"`
	Error      string   `json:"error,omitempty"`
	DurationMs int64    `json:"duration_ms"`
	Warnings   []string `json:"warnings,omitempty"`
}

func toIntegrationChatResponse(answer appintegration.IntegrationChat) integrationChatResponse {
	return integrationChatResponse{
		OK: answer.OK, Status: answer.Status, Model: answer.Model,
		Text: answer.Text, Error: answer.Error, DurationMs: answer.DurationMs,
		Warnings: toStringList(answer.Warnings),
	}
}

type integrationChatRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
}

func toIntegrationChatQuery(request integrationChatRequest) appintegration.IntegrationChatRequest {
	return appintegration.IntegrationChatRequest{
		Model: request.Model, Prompt: request.Prompt,
	}
}
