// Account model handlers: rosters and context windows.
package server

import "net/http"

func (s *Server) handleGetAccountModels(w http.ResponseWriter, r *http.Request) {
	accounts, ok := s.accountsOrUnavailable(w, r)
	if !ok {
		return
	}
	models, err := accounts.AccountModels(r.Context(), r.PathValue("id"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toAccountModelsResponse(models))
}

func (s *Server) handleRefreshAccountModels(w http.ResponseWriter, r *http.Request) {
	accounts, ok := s.accountsOrUnavailable(w, r)
	if !ok {
		return
	}
	models, err := accounts.RefreshAccountModels(r.Context(), r.PathValue("id"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toAccountModelsResponse(models))
}

func (s *Server) handleGetAccountContext(w http.ResponseWriter, r *http.Request) {
	accounts, ok := s.accountsOrUnavailable(w, r)
	if !ok {
		return
	}
	contexts, err := accounts.AccountContext(r.Context(), r.PathValue("id"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toAccountContextResponse(contexts))
}

func (s *Server) handleSetAccountContext(w http.ResponseWriter, r *http.Request) {
	accounts, ok := s.accountsOrUnavailable(w, r)
	if !ok {
		return
	}
	var req struct {
		ModelIDs      []string `json:"model_ids"`
		ContextWindow *int64   `json:"context_window"`
	}
	if !s.decodeJSON(w, r, &req) {
		return
	}
	result, err := accounts.SetAccountModelsContext(r.Context(), r.PathValue("id"), req.ModelIDs, req.ContextWindow)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toAccountContextWriteResponse(result))
}
