// Coding-client integration handlers: setup, repair, rotation, and restore.
package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	appintegration "github.com/jonaskahn/relo/internal/application/integration"
)

type integrationsResponse struct {
	Items []integrationViewResponse `json:"items"`
}

func (s *Server) handleListIntegrations(w http.ResponseWriter, r *http.Request) {
	manager, ok := s.integrationsOrUnavailable(w, r)
	if !ok {
		return
	}
	items, err := manager.Integrations(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, integrationsResponse{Items: toIntegrationViewList(items)})
}

func (s *Server) handleGetIntegration(w http.ResponseWriter, r *http.Request) {
	manager, ok := s.integrationsOrUnavailable(w, r)
	if !ok {
		return
	}
	view, err := manager.Integration(r.Context(), r.PathValue("id"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toIntegrationViewResponse(view))
}

type integrationTokenResponse struct {
	Token string `json:"token"`
}

func (s *Server) handleGetIntegrationToken(w http.ResponseWriter, r *http.Request) {
	manager, ok := s.integrationsOrUnavailable(w, r)
	if !ok {
		return
	}
	token, err := manager.IntegrationToken(r.Context(), r.PathValue("id"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, integrationTokenResponse{Token: token})
}

type enableIntegrationRequest struct {
	Overwrite bool   `json:"overwrite"`
	Force     bool   `json:"force"`
	Versions  string `json:"versions,omitempty"`
}

func (s *Server) handleEnableIntegration(w http.ResponseWriter, r *http.Request) {
	manager, ok := s.integrationsOrUnavailable(w, r)
	if !ok {
		return
	}
	request := enableIntegrationRequest{}
	if !s.actionRequest(w, r, &request) {
		return
	}
	versions, ok := s.openCodeVersions(w, r, request.Versions)
	if !ok {
		return
	}
	var result appintegration.IntegrationResult
	var err error
	if request.Overwrite {
		result, err = manager.EnableIntegrationOverwrite(r.Context(), r.PathValue("id"), versions)
	} else {
		result, err = manager.EnableIntegration(r.Context(), r.PathValue("id"), versions)
	}
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toIntegrationResultResponse(result))
}

// openCodeVersions keeps the generations setup writes: the console sends one
// of v1, v2, or v1+v2 for OpenCode and anything else is a bad request, while
// an empty value writes the current generation.
func (s *Server) openCodeVersions(w http.ResponseWriter, r *http.Request, versions string) (string, bool) {
	switch versions {
	case "", "v1", "v2", "v1+v2":
		if versions == "" {
			return "v2", true
		}
		return versions, true
	default:
		s.fail(w, r, refusal{
			Status: http.StatusBadRequest, Code: "bad_request", Message: "api.body.invalid",
			Data: map[string]any{"Detail": "versions must be v1, v2, or v1+v2"},
		})
		return "", false
	}
}

func (s *Server) handleDisableIntegration(w http.ResponseWriter, r *http.Request) {
	manager, ok := s.integrationsOrUnavailable(w, r)
	if !ok {
		return
	}
	request := enableIntegrationRequest{}
	if !s.actionRequest(w, r, &request) {
		return
	}
	if err := manager.DisableIntegration(r.Context(), r.PathValue("id"), request.Force); err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "disabled"})
}

func (s *Server) actionRequest(w http.ResponseWriter, r *http.Request, request *enableIntegrationRequest) bool {
	if r.Body == nil {
		return true
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxManagementBody))
	if err != nil {
		s.fail(w, r, refusal{
			Status: http.StatusBadRequest, Code: "bad_request", Message: "api.body.invalid",
		})
		return false
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return true
	}
	return s.decodeActionBody(w, r, body, request)
}

func (s *Server) decodeActionBody(w http.ResponseWriter, r *http.Request, body []byte, request *enableIntegrationRequest) bool {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(request); err != nil {
		s.fail(w, r, refusal{
			Status: http.StatusBadRequest, Code: "bad_request", Message: "api.body.invalid",
			Data: map[string]any{"Detail": err.Error()},
		})
		return false
	}
	if err := decoder.Decode(new(struct{})); !errors.Is(err, io.EOF) {
		s.fail(w, r, refusal{
			Status: http.StatusBadRequest, Code: "bad_request", Message: "api.body.single_object",
		})
		return false
	}
	return true
}

func (s *Server) handleRotateIntegration(w http.ResponseWriter, r *http.Request) {
	manager, ok := s.integrationsOrUnavailable(w, r)
	if !ok {
		return
	}
	result, err := manager.RotateIntegrationKey(r.Context(), r.PathValue("id"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toIntegrationResultResponse(result))
}

func (s *Server) handleRestartIntegration(w http.ResponseWriter, r *http.Request) {
	manager, ok := s.integrationsOrUnavailable(w, r)
	if !ok {
		return
	}
	view, err := manager.RestartIntegration(r.Context(), r.PathValue("id"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toIntegrationResultResponse(appintegration.IntegrationResult{Integration: view}))
}

type codexContextRequest struct {
	Enabled bool `json:"enabled"`
}

func (s *Server) handleCodexContext(w http.ResponseWriter, r *http.Request) {
	manager, ok := s.integrationsOrUnavailable(w, r)
	if !ok {
		return
	}
	body, ok := s.readCodexContextBody(w, r)
	if !ok {
		return
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var request codexContextRequest
	if err := decoder.Decode(&request); err != nil {
		s.fail(w, r, refusal{
			Status: http.StatusBadRequest, Code: "bad_request", Message: "api.body.invalid",
			Data: map[string]any{"Detail": err.Error()},
		})
		return
	}
	view, err := s.setCodexContext(r, manager, request)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toIntegrationViewResponse(view))
}

func (s *Server) readCodexContextBody(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	if r.Body == nil {
		s.fail(w, r, refusal{
			Status: http.StatusBadRequest, Code: "bad_request", Message: "api.body.invalid",
		})
		return nil, false
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxManagementBody))
	if err != nil {
		s.fail(w, r, refusal{
			Status: http.StatusBadRequest, Code: "bad_request", Message: "api.body.invalid",
		})
		return nil, false
	}
	return body, true
}

func (s *Server) setCodexContext(r *http.Request, manager *appintegration.Service, request codexContextRequest) (appintegration.IntegrationView, error) {
	return manager.SetCodexContext(r.Context(), r.PathValue("id"), request.Enabled)
}

func (s *Server) handleRepairIntegration(w http.ResponseWriter, r *http.Request) {
	manager, ok := s.integrationsOrUnavailable(w, r)
	if !ok {
		return
	}
	result, err := manager.RepairIntegration(r.Context(), r.PathValue("id"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toIntegrationResultResponse(result))
}

func (s *Server) handleRestoreIntegration(w http.ResponseWriter, r *http.Request) {
	manager, ok := s.integrationsOrUnavailable(w, r)
	if !ok {
		return
	}
	if err := manager.RestoreIntegration(r.Context(), r.PathValue("id")); err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "restored"})
}

func (s *Server) handleVerifyIntegration(w http.ResponseWriter, r *http.Request) {
	manager, ok := s.integrationsOrUnavailable(w, r)
	if !ok {
		return
	}
	verify, err := manager.VerifyIntegration(r.Context(), r.PathValue("id"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toIntegrationVerifyResponse(verify))
}

func (s *Server) handleIntegrationModels(w http.ResponseWriter, r *http.Request) {
	manager, ok := s.integrationsOrUnavailable(w, r)
	if !ok {
		return
	}
	models, err := manager.IntegrationModels(r.Context(), r.PathValue("id"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toIntegrationModelsResponse(models))
}

func (s *Server) handleIntegrationChatTurn(w http.ResponseWriter, r *http.Request) {
	manager, ok := s.integrationsOrUnavailable(w, r)
	if !ok {
		return
	}
	var request integrationChatRequest
	if !s.decodeJSON(w, r, &request) {
		return
	}
	answer, err := manager.IntegrationChat(r.Context(), r.PathValue("id"), toIntegrationChatQuery(request))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toIntegrationChatResponse(answer))
}
