// Daemon lifecycle routes: stop, restart, and shutdown from the console.
package server

import (
	"net/http"
	"strconv"
)

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
	// Flush the receipt before Stop closes this connection.
	writeDaemonReceipt(w, "stopping")
	flushStopReceipt(w)
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
	writeDaemonReceipt(w, "restarting")
	flushStopReceipt(w)
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
	writeDaemonReceipt(w, "stopping")
	flushStopReceipt(w)
	go stop()
}

func flushStopReceipt(w http.ResponseWriter) {
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
}

func writeDaemonReceipt(w http.ResponseWriter, status string) {
	body := "{\"status\":\"" + status + "\"}\n"
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(http.StatusAccepted)
	_, _ = w.Write([]byte(body))
}
