// The management API for Claude Desktop's own model picker.
package server

import (
	"context"
	"errors"
	"net/http"

	appintegration "github.com/jonaskahn/relo/internal/application/integration"
)

type pickerResponse struct {
	Supported   bool   `json:"supported"`
	Enabled     bool   `json:"enabled"`
	Armed       bool   `json:"armed"`
	Trust       string `json:"trust"`
	Profile     string `json:"profile"`
	Fingerprint string `json:"fingerprint"`
	Address     string `json:"address"`
	Reason      string `json:"reason"`
}

type pickerRequest struct {
	Enabled bool `json:"enabled"`
}

func toPickerResponse(view appintegration.PickerView) pickerResponse {
	return pickerResponse{
		Supported: view.Supported, Enabled: view.Enabled, Armed: view.Armed, Trust: view.Trust,
		Profile: view.Profile, Fingerprint: view.Fingerprint, Address: view.Address, Reason: string(view.Reason),
	}
}

// picker answers the picker, or reports the build has none.
func (s *Server) picker(w http.ResponseWriter, r *http.Request) (appintegration.DesktopPicker, bool) {
	manager, ok := s.integrationsOrUnavailable(w, r)
	if !ok {
		return nil, false
	}
	picker := manager.Picker()
	if picker == nil {
		s.fail(w, r, refusal{
			Status: http.StatusServiceUnavailable, Code: "service_unavailable", Message: "api.service.unavailable",
		})
		return nil, false
	}
	return picker, true
}

func (s *Server) handleGetPicker(w http.ResponseWriter, r *http.Request) {
	picker, ok := s.picker(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, toPickerResponse(picker.View(r.Context())))
}

// handleSetPicker turns the picker on or off. Turning it on is the one request
// that may make the operating system ask the operator to trust Relo's root, so
// it is a deliberate PUT and nothing else does it.
func (s *Server) handleSetPicker(w http.ResponseWriter, r *http.Request) {
	picker, ok := s.picker(w, r)
	if !ok {
		return
	}
	var request pickerRequest
	if !s.decodeJSON(w, r, &request) {
		return
	}
	action := picker.Disable
	if request.Enabled {
		action = picker.Enable
	}
	s.answerPicker(w, r, action)
}

func (s *Server) handleRemovePickerTrust(w http.ResponseWriter, r *http.Request) {
	picker, ok := s.picker(w, r)
	if !ok {
		return
	}
	s.answerPicker(w, r, picker.RemoveTrust)
}

// answerPicker writes the picker as it stands after an action, and a refusal
// with the reason and the picker beside it so the console can show both.
func (s *Server) answerPicker(w http.ResponseWriter, r *http.Request, action func(ctx context.Context) (appintegration.PickerView, error)) {
	view, err := action(r.Context())
	if err == nil {
		writeJSON(w, http.StatusOK, toPickerResponse(view))
		return
	}
	var refused *appintegration.PickerError
	if !errors.As(err, &refused) {
		s.writeIntegrationError(w, r, err)
		return
	}
	writeJSON(w, http.StatusConflict, map[string]any{
		"error":  map[string]string{"code": string(refused.Reason), "message": err.Error()},
		"picker": toPickerResponse(view),
	})
}
