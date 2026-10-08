// Sign-in API: the login, logout, and session handlers the console reads.
package server

import (
	"net/http"
	"strings"
)

// The console's sign-in routes. The console trades the admin token for a
// session cookie on the first one, and every request afterwards carries
// that cookie.
const (
	authLoginPath   = apiPrefix + "auth/login"
	authLogoutPath  = apiPrefix + "auth/logout"
	authSessionPath = apiPrefix + "auth/session"
)

type authLoginRequest struct {
	AdminToken string `json:"admin_token"`
}

type authLoginResponse struct {
	CSRFToken string `json:"csrf_token"`
}

type authSessionResponse struct {
	Authenticated bool `json:"authenticated"`
	LoginRequired bool `json:"login_required"`
}

func (s *Server) handleAPILogin(w http.ResponseWriter, r *http.Request) {
	if !s.SameOrigin(r) {
		s.fail(w, r, refusal{Status: http.StatusForbidden, Code: "forbidden", Detail: ErrWrongOrigin})
		return
	}
	if !s.IsLoopback(r) && !s.externalAccess() {
		s.fail(w, r, refusal{
			Status: http.StatusForbidden, Code: "forbidden", Message: "api.auth.loopback_only",
		})
		return
	}
	var request authLoginRequest
	if !s.decodeJSON(w, r, &request) {
		return
	}
	if !s.VerifyAdminToken(strings.TrimSpace(request.AdminToken)) {
		s.fail(w, r, refusal{
			Status: http.StatusUnauthorized, Code: "unauthorized", Message: "api.auth.invalid_token",
		})
		return
	}
	csrf, err := s.startLoginSession(w, r)
	if err != nil {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, authLoginResponse{CSRFToken: csrf})
}

func (s *Server) startLoginSession(w http.ResponseWriter, r *http.Request) (string, error) {
	csrf, err := s.StartSession(w)
	if err != nil {
		s.opts.Logger.Error("start a console session", "error", err)
		s.fail(w, r, refusal{
			Status: http.StatusInternalServerError, Code: "internal_error", Message: "api.auth.session_failed",
		})
		return "", err
	}
	return csrf, nil
}

func (s *Server) handleAPILogout(w http.ResponseWriter, r *http.Request) {
	s.EndSession(w, r)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAPISession(w http.ResponseWriter, r *http.Request) {
	if !s.loginRequired() && s.consoleLocal(r) {
		writeJSON(w, http.StatusOK, authSessionResponse{Authenticated: true})
		return
	}
	if !s.loginRequired() && !s.externalAccess() {
		s.refuseOpen(w, r)
		return
	}
	authenticated := false
	if s.sessions != nil {
		_, authenticated = s.sessions.Lookup(cookieValue(r, SessionCookieName))
	}
	writeJSON(w, http.StatusOK, authSessionResponse{Authenticated: authenticated, LoginRequired: true})
}
