// Status, health, and update reporting for supervisors and consoles.
package server

import (
	"context"
	"net/http"

	"github.com/jonaskahn/relo/internal/access"
)

type statusResponse struct {
	Status        string `json:"status"`
	Version       string `json:"version"`
	UptimeSeconds int64  `json:"uptime_seconds"`
	Addr          string `json:"addr"`
	SecretMode    string `json:"secret_mode"`
	SchemaVersion int    `json:"schema_version"`
	ClientKeys    int    `json:"client_keys"`
	ActiveKeys    int    `json:"active_client_keys"`
	// DataPlane names the address of every protocol a client can point at,
	// which is what a console page, the setup steps, and a script all read
	// instead of writing an address of their own.
	DataPlane []dataPlaneAddress `json:"data_plane"`
	// Language is the operator's language, which every client reads to
	// follow the setting rather than the request that asked for it.
	Language string `json:"language"`
}

type dataPlaneAddress struct {
	Protocol string `json:"protocol"`
	BaseURL  string `json:"base_url"`
}

func (s *Server) handleUpdates(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.opts.Updates.Status(r.Context()))
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.statusReport(r.Context()))
}

func (s *Server) statusReport(ctx context.Context) statusResponse {
	keys, active := s.clientKeyCounts(ctx)
	return statusResponse{
		Status:        "running",
		Version:       s.opts.Version,
		Language:      s.operatorLanguage(ctx),
		UptimeSeconds: int64(s.opts.Clock.Now().Sub(s.startedAt).Seconds()),
		Addr:          s.Addr(),
		SecretMode:    s.secretMode(),
		SchemaVersion: s.schemaVersion(),
		ClientKeys:    keys,
		ActiveKeys:    active,
		DataPlane:     s.dataPlaneAddresses(),
	}
}

func (s *Server) dataPlaneAddresses() []dataPlaneAddress {
	served := s.servedProtocols()
	bound := s.DataPlaneAddrs()
	addresses := make([]dataPlaneAddress, 0, len(served))
	for _, protocol := range served {
		addr := bound[protocol.id]
		if addr == "" {
			addr = s.opts.Config.Server.DataPlaneAddr(protocol.id)
		}
		addresses = append(addresses, dataPlaneAddress{Protocol: protocol.id, BaseURL: "http://" + addr})
	}
	return addresses
}

func (s *Server) clientKeyCounts(ctx context.Context) (total, active int) {
	if s.opts.Keys == nil {
		return 0, 0
	}
	keys, err := s.opts.Keys.AccessKeys(ctx)
	if err != nil {
		s.opts.Logger.Warn("read client keys for status", "error", err)
		return 0, 0
	}
	for _, key := range keys {
		if key.Status == access.StatusActive {
			active++
		}
	}
	return len(keys), active
}

const secretModeNone = "none"

func (s *Server) secretMode() string {
	if s.opts.Secrets == nil {
		// The status shape names the backend even when there is none, so an
		// absent store still answers the frozen "none" the console reads.
		return secretModeNone
	}
	return s.opts.Secrets.Mode()
}

func (s *Server) schemaVersion() int {
	if s.opts.SchemaVersion == nil {
		return 0
	}
	version, err := s.opts.SchemaVersion.SchemaVersion()
	if err != nil {
		s.opts.Logger.Warn("read schema version", "error", err)
		return 0
	}
	return version
}
