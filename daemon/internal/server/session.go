// Dashboard sessions: minting, lookup, and revocation.
package server

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/jonaskahn/relo/internal/clock"
)

const (
	// SessionCookieName is the cookie a dashboard session lives in.
	SessionCookieName = "relo_session"
	// SessionTokenBytes is the entropy of a session or CSRF token.
	SessionTokenBytes = 32
	// MaxSessions bounds the sessions one process keeps, so a browser that
	// never comes back cannot grow the map forever.
	MaxSessions = 64
)

var (
	// ErrNoSessions reports an operation that needs the session store the
	// process was not built with.
	ErrNoSessions = errors.New("this server has no dashboard sessions")
)

const sessionTTL = 12 * time.Hour

// Session is one dashboard login: the cookie value, the CSRF token that
// cookie must echo back, and when it expires.
type Session struct {
	Token     string
	CSRFToken string
	ExpiresAt time.Time
}

// Sessions holds the dashboard sessions one process minted. They live in
// memory only, so a restart logs every browser out.
type Sessions struct {
	mu      sync.Mutex
	clock   clock.Clock
	ttl     time.Duration
	max     int
	entries map[string]Session
}

// NewSessions returns an empty session store driven by the given clock.
func NewSessions(clk clock.Clock) *Sessions {
	if clk == nil {
		clk = clock.New()
	}
	return &Sessions{clock: clk, ttl: sessionTTL, max: MaxSessions, entries: map[string]Session{}}
}

// Mint issues one session, dropping whatever expired in the meantime.
func (s *Sessions) Mint() (Session, error) {
	token, err := sessionToken()
	if err != nil {
		return Session{}, err
	}
	csrf, err := sessionToken()
	if err != nil {
		return Session{}, err
	}
	session := Session{Token: token, CSRFToken: csrf, ExpiresAt: s.clock.Now().Add(s.ttl)}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prune()
	s.entries[sessionKey(token)] = session
	s.evictOverflow()
	return session, nil
}

// Lookup returns the session a cookie token names, and whether it is still
// valid. The lookup is keyed by the token digest, so the map never holds the
// cookie value itself.
func (s *Sessions) Lookup(token string) (Session, bool) {
	if token == "" {
		return Session{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	session, found := s.entries[sessionKey(token)]
	if !found {
		return Session{}, false
	}
	if !s.clock.Now().Before(session.ExpiresAt) {
		delete(s.entries, sessionKey(token))
		return Session{}, false
	}
	return session, true
}

// Revoke drops one session.
func (s *Sessions) Revoke(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.entries, sessionKey(token))
}

// Count returns how many sessions are live right now.
func (s *Sessions) Count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prune()
	return len(s.entries)
}

func (s *Sessions) prune() {
	for key, session := range s.entries {
		if !s.clock.Now().Before(session.ExpiresAt) {
			delete(s.entries, key)
		}
	}
}

func (s *Sessions) evictOverflow() {
	for len(s.entries) > s.max {
		oldestKey, oldest := "", time.Time{}
		for key, session := range s.entries {
			if oldest.IsZero() || session.ExpiresAt.Before(oldest) {
				oldestKey, oldest = key, session.ExpiresAt
			}
		}
		delete(s.entries, oldestKey)
	}
}

func sessionKey(token string) string {
	// The cookie value is never stored, and a digest of a 256-bit random
	// token is not guessable from the map.
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func sessionToken() (string, error) {
	raw := make([]byte, SessionTokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate a session token: %w", err)
	}
	return hex.EncodeToString(raw), nil
}

type guardOptions struct {
	server     *Server
	adminToken string
	sessions   *Sessions
}

// LoginPath is the page a browser signs in at. It is the one management
// route that answers without a session.
const LoginPath = "/login"

func (s *Server) sessionGuard(next http.Handler) http.Handler {
	options := guardOptions{server: s, adminToken: s.opts.AdminToken, sessions: s.sessions}
	guarded := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if options.tokenMatches(r) {
			next.ServeHTTP(w, r)
			return
		}
		options.serveSession(w, r, next)
	})
	open := s.openGuard(next)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.loginRequired() {
			guarded.ServeHTTP(w, r)
			return
		}
		if s.consoleLocal(r) {
			open.ServeHTTP(w, r)
			return
		}
		if s.externalAccess() {
			guarded.ServeHTTP(w, r)
			return
		}
		s.refuseOpen(w, r)
	})
}

func (s *Server) openGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.consoleLocal(r) {
			s.refuseOpen(w, r)
			return
		}
		if err := checkOpenWrite(r); err != nil {
			s.fail(w, r, refusal{Status: http.StatusForbidden, Code: "forbidden", Detail: err})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) refuseOpen(w http.ResponseWriter, r *http.Request) {
	if !isLocalAddress(remoteIP(r.RemoteAddr)) {
		s.fail(w, r, refusal{
			Status: http.StatusForbidden, Code: "forbidden", Message: "api.auth.loopback_only",
		})
		return
	}
	s.fail(w, r, refusal{
		Status: http.StatusForbidden, Code: "forbidden", Detail: ErrWrongHost,
	})
}

func (s *Server) loginRequired() bool {
	return s.opts.Config.Admin.Login
}

func (s *Server) externalAccess() bool {
	if s.opts.Settings == nil {
		return false
	}
	enabled, err := s.opts.Settings.AllowExternal()
	if err != nil {
		s.opts.Logger.Warn("read the external-access setting", "error", err)
		return false
	}
	return enabled
}

func (o guardOptions) serveSession(w http.ResponseWriter, r *http.Request, next http.Handler) {
	if o.sessions == nil {
		o.unauthorized(w, r)
		return
	}
	token := cookieValue(r, SessionCookieName)
	session, found := o.sessions.Lookup(token)
	if !found {
		o.unauthorized(w, r)
		return
	}
	if err := authorizeWrite(r, session); err != nil {
		o.server.fail(w, r, refusal{Status: http.StatusForbidden, Code: "forbidden", Detail: err})
		return
	}
	next.ServeHTTP(w, r)
}

func (o guardOptions) unauthorized(w http.ResponseWriter, r *http.Request) {
	if isAPIRequest(r) {
		o.server.fail(w, r, refusal{
			Status: http.StatusUnauthorized, Code: "unauthorized", Message: "api.auth.session_missing",
		})
		return
	}
	http.Redirect(w, r, loginRedirect(r), http.StatusSeeOther)
}

func isAPIRequest(r *http.Request) bool {
	return strings.HasPrefix(r.URL.Path, apiPrefix)
}

func loginRedirect(r *http.Request) string {
	target := safeReturnTarget(r.URL.RequestURI())
	if target == "" {
		return LoginPath
	}
	return LoginPath + "?" + url.Values{"return_to": {target}}.Encode()
}

func safeReturnTarget(raw string) string {
	trimmed := strings.TrimSpace(raw)
	switch {
	case trimmed == "", !strings.HasPrefix(trimmed, "/"), strings.HasPrefix(trimmed, "//"):
		return ""
	case strings.ContainsAny(trimmed, "\\\r\n"):
		return ""
	case strings.HasPrefix(trimmed, LoginPath):
		return ""
	default:
		return trimmed
	}
}

func (o guardOptions) tokenMatches(r *http.Request) bool {
	expected := []byte(o.adminToken)
	provided, found := bearerToken(r)
	if !found || len(provided) == 0 {
		return false
	}
	return subtle.ConstantTimeCompare(provided, expected) == 1
}

// VerifyAdminToken reports whether a presented string is the admin token.
// The comparison takes constant time, so a wrong guess is not timed apart
// from a short one.
func (s *Server) VerifyAdminToken(token string) bool {
	expected := []byte(s.opts.AdminToken)
	if len(expected) == 0 || token == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(token), expected) == 1
}

// IsLoopback reports whether a request came from this machine, which is the
// only place the dashboard may mint a session.
func (s *Server) IsLoopback(r *http.Request) bool {
	return isLoopback(r.RemoteAddr)
}

// SameOrigin reports whether a request came from the site that served it.
func (s *Server) SameOrigin(r *http.Request) bool {
	return checkOrigin(r) == nil
}

// StartSession mints one session and sets the cookies that carry it.
func (s *Server) StartSession(w http.ResponseWriter) (string, error) {
	if s.sessions == nil {
		return "", ErrNoSessions
	}
	session, err := s.sessions.Mint()
	if err != nil {
		return "", err
	}
	setSessionCookies(w, session)
	return session.CSRFToken, nil
}

// EndSession revokes the session of one request and clears its cookies.
func (s *Server) EndSession(w http.ResponseWriter, r *http.Request) {
	if s.sessions != nil {
		s.sessions.Revoke(cookieValue(r, SessionCookieName))
	}
	clearSessionCookies(w)
}

func setSessionCookies(w http.ResponseWriter, session Session) {
	http.SetCookie(w, &http.Cookie{
		Name: SessionCookieName, Value: session.Token, Path: "/",
		HttpOnly: true, SameSite: http.SameSiteStrictMode,
		Expires: session.ExpiresAt, MaxAge: int(sessionTTL.Seconds()),
	})
	http.SetCookie(w, &http.Cookie{
		Name: CSRFCookieName, Value: session.CSRFToken, Path: "/",
		SameSite: http.SameSiteStrictMode,
		Expires:  session.ExpiresAt, MaxAge: int(sessionTTL.Seconds()),
	})
}

func clearSessionCookies(w http.ResponseWriter) {
	for _, name := range []string{SessionCookieName, CSRFCookieName} {
		http.SetCookie(w, &http.Cookie{
			Name: name, Value: "", Path: "/", HttpOnly: name == SessionCookieName,
			SameSite: http.SameSiteStrictMode, MaxAge: -1, Expires: time.Unix(0, 0),
		})
	}
}

func cookieValue(r *http.Request, name string) string {
	cookie, err := r.Cookie(name)
	if err != nil {
		return ""
	}
	return cookie.Value
}

func isLoopback(remoteAddr string) bool {
	return remoteIP(remoteAddr).IsLoopback()
}

func isLocalHost(hostPort string) bool {
	host, _, err := net.SplitHostPort(hostPort)
	if err != nil {
		host = hostPort
	}
	host = strings.Trim(strings.TrimSpace(host), "[]")
	if strings.EqualFold(host, "localhost") {
		return true
	}
	return isLocalAddress(net.ParseIP(host))
}

func originHost(raw string) string {
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return parsed.Host
}
