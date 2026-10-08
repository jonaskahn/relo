// Daemon lifecycle routes: stop, restart, and shutdown from the console.
package server

import "net/http"

type daemonStopRequest struct {
	InstanceID string `json:"instance_id"`
}

func (s *Server) handleDaemonStop(w http.ResponseWriter, r *http.Request) {
	if s.opts.Shutdown == nil || s.opts.InstanceID == "" {
		s.fail(w, r, refusal{
			Status: http.StatusNotImplemented, Code: "not_implemented", Message: "api.daemon.not_headless",
		})
		return
	}
	if !s.IsLoopback(r) {
		s.fail(w, r, refusal{
			Status: http.StatusForbidden, Code: "forbidden", Message: "api.auth.loopback_only",
		})
		return
	}
	if !s.verifyStopToken(w, r) {
		return
	}
	var request daemonStopRequest
	if !s.decodeJSON(w, r, &request) {
		return
	}
	if request.InstanceID != s.opts.InstanceID {
		s.fail(w, r, refusal{
			Status: http.StatusConflict, Code: "conflict", Message: "api.daemon.instance_mismatch",
		})
		return
	}
	s.answerStopReceipt(w)
}

func (s *Server) answerStopReceipt(w http.ResponseWriter) {
	stop := s.opts.Shutdown
	// The answer leaves before the daemon drains, so the caller reads a
	// receipt rather than a connection that closed under it.
	writeJSON(w, http.StatusAccepted, map[string]any{"status": "stopping"})
	s.opts.Logger.Info("daemon stopping", "cause", "stop request")
	go stop()
}

func (s *Server) verifyStopToken(w http.ResponseWriter, r *http.Request) bool {
	presented, found := bearerToken(r)
	if !found || !s.VerifyAdminToken(string(presented)) {
		s.fail(w, r, refusal{
			Status: http.StatusUnauthorized, Code: "unauthorized", Message: "api.auth.invalid_token",
		})
		return false
	}
	return true
}

func (s *Server) handleDaemonRestart(w http.ResponseWriter, r *http.Request) {
	s.acceptDaemonRestart(w, r, false)
}

func (s *Server) handleDaemonForceRestart(w http.ResponseWriter, r *http.Request) {
	s.acceptDaemonRestart(w, r, true)
}

func (s *Server) acceptDaemonRestart(w http.ResponseWriter, r *http.Request, force bool) {
	if s.opts.Restart == nil {
		s.fail(w, r, refusal{
			Status: http.StatusNotImplemented, Code: "not_implemented", Message: "api.daemon.not_headless",
		})
		return
	}
	if !s.IsLoopback(r) {
		s.fail(w, r, refusal{
			Status: http.StatusForbidden, Code: "forbidden", Message: "api.auth.loopback_only",
		})
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"status": "restarting"})
	s.opts.Logger.Info("daemon restarting", "cause", "restart request", "force", force)
	go s.opts.Restart(force)
}

func (s *Server) handleDaemonShutdown(w http.ResponseWriter, r *http.Request) {
	if s.opts.Shutdown == nil {
		s.fail(w, r, refusal{
			Status: http.StatusNotImplemented, Code: "not_implemented", Message: "api.daemon.not_headless",
		})
		return
	}
	if !s.IsLoopback(r) {
		s.fail(w, r, refusal{
			Status: http.StatusForbidden, Code: "forbidden", Message: "api.auth.loopback_only",
		})
		return
	}
	stop := s.opts.Shutdown
	writeJSON(w, http.StatusAccepted, map[string]any{"status": "stopping"})
	s.opts.Logger.Info("daemon stopping", "cause", "shutdown request")
	go stop()
}
