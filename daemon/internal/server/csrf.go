// Write authorization: the CSRF, origin, and session checks writes pass.
package server

import (
	"crypto/subtle"
	"errors"
	"net/http"
)

const (
	// CSRFCookieName is the readable double-submit cookie the dashboard
	// echoes back in the CSRF header.
	CSRFCookieName = "relo_csrf"
	// CSRFHeaderName is the header a state-changing dashboard request must
	// carry.
	CSRFHeaderName = "X-CSRF-Token"
	// CSRFOriginHeader is the header a browser sets on a cross-origin
	// request, which the dashboard refuses.
	CSRFOriginHeader = "Origin"
)

var (
	// ErrMissingCSRF reports a state-changing request that did not echo the
	// double-submit cookie.
	ErrMissingCSRF = errors.New("the CSRF token is missing or does not match")
	// ErrWrongOrigin reports a request a browser sent from another site.
	ErrWrongOrigin = errors.New("the request came from another origin")
	// ErrWrongHost reports a request whose Host names a site other than the
	// one serving it.
	ErrWrongHost = errors.New("the request Host does not name this machine")
)

func authorizeWrite(r *http.Request, session Session) error {
	if err := checkOrigin(r); err != nil {
		return err
	}
	if isSafeMethod(r.Method) {
		return nil
	}
	return checkCSRF(r, session)
}

func isSafeMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	default:
		return false
	}
}

func checkOpenWrite(r *http.Request) error {
	if isSafeMethod(r.Method) {
		return nil
	}
	if r.Header.Get(CSRFOriginHeader) == "null" {
		return ErrWrongOrigin
	}
	return checkOrigin(r)
}

func checkOrigin(r *http.Request) error {
	origin := r.Header.Get(CSRFOriginHeader)
	if origin == "" || origin == "null" {
		return nil
	}
	if !sameHost(originHost(origin), r.Host) {
		return ErrWrongOrigin
	}
	return nil
}

func checkCSRF(r *http.Request, session Session) error {
	provided := r.Header.Get(CSRFHeaderName)
	if provided == "" {
		provided = r.FormValue(csrfFieldName)
	}
	if provided == "" {
		return ErrMissingCSRF
	}
	expected := session.CSRFToken
	if expected == "" {
		expected = cookieValue(r, CSRFCookieName)
	}
	if subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) != 1 {
		return ErrMissingCSRF
	}
	return nil
}

const csrfFieldName = "csrf_token"

func sameHost(origin, host string) bool {
	if origin == "" || host == "" {
		return false
	}
	return origin == host
}
