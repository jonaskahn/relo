// The management API for a Claude Code integration's model bindings.
package server

import (
	"net/http"

	"github.com/jonaskahn/relo/internal/application/claudebindings"
	appintegration "github.com/jonaskahn/relo/internal/application/integration"
)

// bindingsIntegration is the one integration that has bindings: Claude Code,
// the client proxy mode wires.
const bindingsIntegration = "claude-code"

type bindingTargetDTO struct {
	Kind       string `json:"kind"`
	GroupID    string `json:"group_id,omitempty"`
	ProviderID string `json:"provider_id,omitempty"`
	ModelID    string `json:"model_id,omitempty"`
}

type bindingDTO struct {
	NativeID  string           `json:"native_id"`
	Target    bindingTargetDTO `json:"target"`
	Direction string           `json:"direction"`
	OnFailure string           `json:"on_failure"`
	Enabled   bool             `json:"enabled"`
}

type bindingEntryDTO struct {
	bindingDTO
	State string `json:"state"`
}

type officialModelDTO struct {
	ID     string `json:"id"`
	Family string `json:"family"`
	Source string `json:"source"`
}

type bindingsResponse struct {
	Bindings []bindingEntryDTO  `json:"bindings"`
	Official []officialModelDTO `json:"official"`
}

type bindingsRequest struct {
	Set    []bindingDTO `json:"set"`
	Remove []string     `json:"remove"`
}

func toBindingsResponse(view claudebindings.View) bindingsResponse {
	response := bindingsResponse{
		Bindings: make([]bindingEntryDTO, 0, len(view.Entries)),
		Official: make([]officialModelDTO, 0, len(view.Official)),
	}
	for _, entry := range view.Entries {
		response.Bindings = append(response.Bindings, bindingEntryDTO{
			bindingDTO: toBindingDTO(entry.Binding), State: string(entry.State),
		})
	}
	for _, official := range view.Official {
		response.Official = append(response.Official, officialModelDTO{
			ID: official.ID, Family: official.Family, Source: official.Source,
		})
	}
	return response
}

func toBindingDTO(binding claudebindings.Binding) bindingDTO {
	return bindingDTO{
		NativeID: binding.NativeID, Direction: binding.Direction, OnFailure: binding.OnFailure, Enabled: binding.Enabled,
		Target: bindingTargetDTO{
			Kind: binding.Target.Kind, GroupID: binding.Target.GroupID,
			ProviderID: binding.Target.ProviderID, ModelID: binding.Target.ModelID,
		},
	}
}

func toChange(request bindingsRequest) claudebindings.Change {
	change := claudebindings.Change{Remove: request.Remove}
	for _, binding := range request.Set {
		change.Set = append(change.Set, claudebindings.Binding{
			NativeID: binding.NativeID, Direction: binding.Direction, OnFailure: binding.OnFailure, Enabled: binding.Enabled,
			Target: claudebindings.Target{
				Kind: binding.Target.Kind, GroupID: binding.Target.GroupID,
				ProviderID: binding.Target.ProviderID, ModelID: binding.Target.ModelID,
			},
		})
	}
	return change
}

// bindingsOrRefuse answers the service when the request names the integration
// that has bindings, and refuses it otherwise.
func (s *Server) bindingsOrRefuse(w http.ResponseWriter, r *http.Request) (*claudebindings.Service, bool) {
	if s.opts.ClaudeBindings == nil {
		s.fail(w, r, refusal{
			Status: http.StatusServiceUnavailable, Code: "service_unavailable", Message: "api.service.unavailable",
		})
		return nil, false
	}
	if r.PathValue("id") != bindingsIntegration {
		s.writeIntegrationError(w, r, appintegration.ErrUnknownIntegration)
		return nil, false
	}
	return s.opts.ClaudeBindings, true
}

func (s *Server) handleGetBindings(w http.ResponseWriter, r *http.Request) {
	bindings, ok := s.bindingsOrRefuse(w, r)
	if !ok {
		return
	}
	view, err := bindings.View(r.Context(), bindingsIntegration)
	if err != nil {
		s.writeIntegrationError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toBindingsResponse(view))
}

func (s *Server) handleSetBindings(w http.ResponseWriter, r *http.Request) {
	bindings, ok := s.bindingsOrRefuse(w, r)
	if !ok {
		return
	}
	if !s.requireProxyMode(w, r) {
		return
	}
	var request bindingsRequest
	if !s.decodeJSON(w, r, &request) {
		return
	}
	if err := bindings.Apply(r.Context(), bindingsIntegration, toChange(request)); err != nil {
		s.writeIntegrationError(w, r, err)
		return
	}
	view, err := bindings.View(r.Context(), bindingsIntegration)
	if err != nil {
		s.writeIntegrationError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toBindingsResponse(view))
}

// requireProxyMode refuses a write while Claude Code is not wired through the
// intercept. A binding only means something there, so writing one in settings
// mode is refused rather than stored where nothing reads it.
func (s *Server) requireProxyMode(w http.ResponseWriter, r *http.Request) bool {
	manager, ok := s.integrationsOrUnavailable(w, r)
	if !ok {
		return false
	}
	integration, err := manager.Integration(r.Context(), bindingsIntegration)
	if err != nil {
		s.writeIntegrationError(w, r, err)
		return false
	}
	if !integration.Enabled || integration.Mode != appintegration.ModeProxy {
		writeError(w, http.StatusConflict, "conflict", "model bindings apply to Claude Code in proxy mode only")
		return false
	}
	return true
}
