// Access key DTOs and handlers: the client keys agents authenticate with.
package server

import (
	"net/http"

	appaccess "github.com/jonaskahn/relo/internal/application/access"
)

type accessKeysResponse struct {
	Items []accessKeyResponse `json:"items"`
}

type issuedAccessKeyResponse struct {
	Key   accessKeyResponse `json:"key"`
	Token string            `json:"token"`
}

type createAccessKeyRequest struct {
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	Client      string `json:"client"`
	ExpiresAtMs int64  `json:"expires_at_ms"`
}

type updateAccessKeyRequest struct {
	Name        *string `json:"name"`
	ExpiresAtMs *int64  `json:"expires_at_ms"`
}

func (s *Server) handleListAccessKeys(w http.ResponseWriter, r *http.Request) {
	keys, ok := s.keysOrUnavailable(w, r)
	if !ok {
		return
	}
	listed, err := keys.AccessKeys(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, accessKeysResponse{Items: toAccessKeyList(listed)})
}

func (s *Server) handleCreateAccessKey(w http.ResponseWriter, r *http.Request) {
	keys, ok := s.keysOrUnavailable(w, r)
	if !ok {
		return
	}
	var request createAccessKeyRequest
	if !s.decodeJSON(w, r, &request) {
		return
	}
	issued, err := keys.CreateAccessKey(r.Context(), appaccess.NewAccessKey{
		Name: request.Name, Kind: request.Kind, Client: request.Client,
		ExpiresAtMs: request.ExpiresAtMs,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, issuedAccessKeyResponse{Key: toAccessKeyResponse(issued.AccessKey), Token: issued.Token})
}

func (s *Server) handleUpdateAccessKey(w http.ResponseWriter, r *http.Request) {
	keys, ok := s.keysOrUnavailable(w, r)
	if !ok {
		return
	}
	var request updateAccessKeyRequest
	if !s.decodeJSON(w, r, &request) {
		return
	}
	current, err := keys.AccessKey(r.Context(), r.PathValue("id"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	update := appaccess.AccessKeyUpdate{Name: current.Name, ExpiresAtMs: current.ExpiresAtMs}
	if request.Name != nil {
		update.Name = *request.Name
	}
	if request.ExpiresAtMs != nil {
		update.ExpiresAtMs = *request.ExpiresAtMs
	}
	key, err := keys.UpdateAccessKey(r.Context(), current.ID, update)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	s.InvalidateAccessKey(current.ID)
	writeJSON(w, http.StatusOK, toAccessKeyResponse(key))
}

func (s *Server) handleRotateAccessKey(w http.ResponseWriter, r *http.Request) {
	keys, ok := s.keysOrUnavailable(w, r)
	if !ok {
		return
	}
	issued, err := keys.RotateAccessKey(r.Context(), r.PathValue("id"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	s.InvalidateAccessKey(issued.ID)
	writeJSON(w, http.StatusOK, issuedAccessKeyResponse{Key: toAccessKeyResponse(issued.AccessKey), Token: issued.Token})
}

func (s *Server) handleDeleteAccessKey(w http.ResponseWriter, r *http.Request) {
	keys, ok := s.keysOrUnavailable(w, r)
	if !ok {
		return
	}
	key, err := keys.AccessKey(r.Context(), r.PathValue("id"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	if err := keys.RevokeAccessKey(r.Context(), key.ID); err != nil {
		writeServiceError(w, err)
		return
	}
	s.InvalidateAccessKey(key.ID)
	w.WriteHeader(http.StatusNoContent)
}

type deleteExpiredKeysRequest struct {
	IDs []string `json:"ids"`
}

type deleteExpiredKeysResponse struct {
	Deleted int `json:"deleted"`
}

func (s *Server) handleDeleteExpiredAccessKeys(w http.ResponseWriter, r *http.Request) {
	keys, ok := s.keysOrUnavailable(w, r)
	if !ok {
		return
	}
	var request deleteExpiredKeysRequest
	if !s.decodeJSON(w, r, &request) {
		return
	}
	deleted, err := keys.DeleteExpiredKeys(r.Context(), request.IDs)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	for _, id := range deleted {
		s.InvalidateAccessKey(id)
	}
	writeJSON(w, http.StatusOK, deleteExpiredKeysResponse{Deleted: len(deleted)})
}
