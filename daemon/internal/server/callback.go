// OAuth callback registration and page rendering for provider redirects.
package server

import (
	"bytes"
	"net/http"
	"strings"
)

// The query parameters one provider redirect carries, and the ticket that
// replaces them once the grant has been read.
const (
	callbackCodeQuery   = "code"
	callbackStateQuery  = "state"
	callbackTicketQuery = "ticket"
)

func (s *Server) registerCallbacks(mux *http.ServeMux) {
	mux.HandleFunc("GET "+CallbackPrefix+"{provider}", s.handleCallback)
	mux.HandleFunc("GET "+CallbackPrefix+"{provider}/status", s.handleCallbackStatus)
}

func (s *Server) handleCallback(w http.ResponseWriter, r *http.Request) {
	provider := r.PathValue("provider")
	query := r.URL.Query()
	if ticket := strings.TrimSpace(query.Get(callbackTicketQuery)); ticket != "" {
		s.renderCallback(w, r, provider, ticket)
		return
	}
	ticket, err := s.opts.Callbacks.Deliver(provider, query)
	if err != nil && ticket == "" {
		s.renderCallbackRefusal(w, r, provider)
		return
	}
	// The page lives on the same listener the redirect arrived on, so a
	// relative address keeps whichever host name the browser used.
	redirect := CallbackPrefix + provider + "?" + callbackTicketQuery + "=" + ticket
	http.Redirect(w, r, redirect, http.StatusSeeOther)
}

func (s *Server) handleCallbackStatus(w http.ResponseWriter, r *http.Request) {
	status, found := s.opts.Callbacks.Status(strings.TrimSpace(r.URL.Query().Get(callbackTicketQuery)))
	if !found {
		writeError(w, http.StatusNotFound, "not_found", s.translator(r).Text("api.callback.expired", nil))
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) renderCallback(w http.ResponseWriter, r *http.Request, provider, ticket string) {
	view := s.callbackView(r, provider, ticket)
	status, found := s.opts.Callbacks.Status(ticket)
	if !found {
		s.expireCallback(&view)
		s.writeCallbackPage(w, view, http.StatusOK)
		return
	}
	switch status.Phase {
	case CallbackConnected:
		view.Connected = true
		view.Account = status.Account
		view.Heading = view.Wording.ConnectedTitle
		view.Summary = view.Wording.ConnectedBody
		view.Marker = view.Wording.MarkerConnected
	case CallbackFailed:
		view.Failed = true
		view.Heading = view.Wording.FailedTitle
		view.Summary = callbackFailureBody(view.Wording, CallbackOutcome(status.Error))
		view.Marker = view.Wording.MarkerFailed
	}
	s.writeCallbackPage(w, view, http.StatusOK)
}

func (s *Server) renderCallbackRefusal(w http.ResponseWriter, r *http.Request, provider string) {
	view := s.callbackView(r, provider, "")
	s.expireCallback(&view)
	s.writeCallbackPage(w, view, http.StatusBadRequest)
}

func (s *Server) expireCallback(view *callbackView) {
	view.Expired = true
	view.Failed = true
	view.StatusURL = ""
	view.Heading = view.Wording.ExpiredTitle
	view.Summary = view.Wording.ExpiredBody
	view.Marker = view.Wording.MarkerFailed
}

func (s *Server) writeCallbackPage(w http.ResponseWriter, view callbackView, status int) {
	// The icon travels as a placeholder the page fills in after rendering:
	// the template sanitizer rewrites any data URI it is given, while the
	// document has always carried the raw brand icon.
	var rendered bytes.Buffer
	if err := callbackTemplate.Execute(&rendered, view); err != nil {
		s.opts.Logger.Error("render the callback page", "error", err)
	}
	setCallbackPageHeaders(w)
	w.WriteHeader(status)
	_, _ = w.Write(bytes.ReplaceAll(rendered.Bytes(), []byte(callbackIconPlaceholder), []byte(s.opts.CallbackIcon)))
}

func setCallbackPageHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Content-Security-Policy",
		"default-src 'none'; base-uri 'none'; form-action 'none'; img-src 'self' data:; "+
			"style-src 'unsafe-inline'; script-src 'unsafe-inline'; connect-src 'self'")
}
