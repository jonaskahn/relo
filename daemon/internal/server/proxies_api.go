// Managed outbound proxy pool API.
package server

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	appproxy "github.com/jonaskahn/relo/internal/proxy"
)

type proxyResponse struct {
	ID           string  `json:"id"`
	Label        string  `json:"label"`
	Scheme       string  `json:"scheme"`
	Host         string  `json:"host"`
	Port         int     `json:"port"`
	Username     string  `json:"username"`
	Enabled      bool    `json:"enabled"`
	Rank         int     `json:"rank"`
	HasPassword  bool    `json:"has_password"`
	State        string  `json:"state"`
	CoolingUntil *string `json:"cooling_until,omitempty"`
}

type proxyListResponse struct {
	Items      []proxyResponse `json:"items"`
	NextCursor string          `json:"next_cursor"`
}

type createProxyRequest struct {
	Label    string `json:"label"`
	Scheme   string `json:"scheme"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
	Enabled  *bool  `json:"enabled"`
	Rank     int    `json:"rank"`
}

type importProxyRequest struct {
	Text string `json:"text"`
}

type importProxyResult struct {
	Line   int            `json:"line"`
	Raw    string         `json:"raw"`
	Status string         `json:"status"`
	Reason string         `json:"reason,omitempty"`
	Item   *proxyResponse `json:"item,omitempty"`
}

type importProxyResponse struct {
	Results []importProxyResult `json:"results"`
}

type patchProxyRequest struct {
	Label    *string `json:"label"`
	Scheme   *string `json:"scheme"`
	Host     *string `json:"host"`
	Port     *int    `json:"port"`
	Username *string `json:"username"`
	Password *string `json:"password"`
	Enabled  *bool   `json:"enabled"`
	Rank     *int    `json:"rank"`
}

func toProxyResponse(item appproxy.Item) proxyResponse {
	response := proxyResponse{
		ID: item.ID, Label: item.Label, Scheme: item.Scheme, Host: item.Host, Port: item.Port,
		Username: item.Username, Enabled: item.Enabled, Rank: item.Rank,
		HasPassword: item.HasPassword, State: string(item.State),
	}
	if item.CoolingUntil != nil {
		until := item.CoolingUntil.UTC().Format(time.RFC3339)
		response.CoolingUntil = &until
	}
	return response
}

func (s *Server) proxiesOrUnavailable(w http.ResponseWriter, r *http.Request) (*appproxy.Service, bool) {
	if s.opts.Proxies == nil {
		s.fail(w, r, refusal{
			Status: http.StatusServiceUnavailable, Code: "service_unavailable", Message: "api.service.unavailable",
		})
		return nil, false
	}
	return s.opts.Proxies, true
}

func (s *Server) handleListProxies(w http.ResponseWriter, r *http.Request) {
	proxies, ok := s.proxiesOrUnavailable(w, r)
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset := 0
	if cursor := strings.TrimSpace(r.URL.Query().Get("cursor")); cursor != "" {
		offset, _ = strconv.Atoi(cursor)
	}
	items, next, err := proxies.Page(r.Context(), appproxy.Query{
		Search: r.URL.Query().Get("q"), Status: r.URL.Query().Get("status"),
		Offset: offset, Limit: limit,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	response := proxyListResponse{Items: make([]proxyResponse, 0, len(items))}
	for _, item := range items {
		response.Items = append(response.Items, toProxyResponse(item))
	}
	if next > 0 {
		response.NextCursor = strconv.Itoa(next)
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) handleCreateProxy(w http.ResponseWriter, r *http.Request) {
	proxies, ok := s.proxiesOrUnavailable(w, r)
	if !ok {
		return
	}
	var request createProxyRequest
	if !s.decodeJSON(w, r, &request) {
		return
	}
	enabled := true
	if request.Enabled != nil {
		enabled = *request.Enabled
	}
	created, err := proxies.Create(r.Context(), appproxy.Draft{
		Label: request.Label, Scheme: request.Scheme, Host: request.Host, Port: request.Port,
		Username: request.Username, Password: request.Password, Enabled: enabled, Rank: request.Rank,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toProxyResponse(created))
}

func (s *Server) handleImportProxies(w http.ResponseWriter, r *http.Request) {
	proxies, ok := s.proxiesOrUnavailable(w, r)
	if !ok {
		return
	}
	var request importProxyRequest
	if !s.decodeJSON(w, r, &request) {
		return
	}
	results, err := proxies.Import(r.Context(), request.Text)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	response := importProxyResponse{Results: make([]importProxyResult, 0, len(results))}
	for _, result := range results {
		row := importProxyResult{Line: result.Line, Raw: result.Raw, Status: result.Status, Reason: result.Reason}
		if result.Item != nil {
			item := toProxyResponse(*result.Item)
			row.Item = &item
		}
		response.Results = append(response.Results, row)
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) handlePatchProxy(w http.ResponseWriter, r *http.Request) {
	proxies, ok := s.proxiesOrUnavailable(w, r)
	if !ok {
		return
	}
	var request patchProxyRequest
	if !s.decodeJSON(w, r, &request) {
		return
	}
	updated, err := proxies.Update(r.Context(), r.PathValue("id"), appproxy.Patch{
		Label: request.Label, Scheme: request.Scheme, Host: request.Host, Port: request.Port,
		Username: request.Username, Password: request.Password, Enabled: request.Enabled, Rank: request.Rank,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toProxyResponse(updated))
}

func (s *Server) handleDeleteProxy(w http.ResponseWriter, r *http.Request) {
	proxies, ok := s.proxiesOrUnavailable(w, r)
	if !ok {
		return
	}
	if err := proxies.Delete(r.Context(), r.PathValue("id")); err != nil {
		writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleTestProxy(w http.ResponseWriter, r *http.Request) {
	proxies, ok := s.proxiesOrUnavailable(w, r)
	if !ok {
		return
	}
	tested, err := proxies.Test(r.Context(), r.PathValue("id"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toProxyResponse(tested))
}
