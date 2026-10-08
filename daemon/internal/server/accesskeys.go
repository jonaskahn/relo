// Client-key gate: the data-plane guard and the identity it carries.
package server

import (
	"context"
	"crypto/subtle"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/jonaskahn/relo/internal/access"
	appaccess "github.com/jonaskahn/relo/internal/application/access"
	"github.com/jonaskahn/relo/internal/clock"
)

const (
	// accessKeyCacheTTL is how long a verification is trusted without asking
	// the store again. A key created in another process — a CLI run against a
	// running daemon — becomes visible within this window.
	accessKeyCacheTTL = 5 * time.Second
	// accessKeyCacheMax bounds the verification cache, so a client presenting
	// invented identifiers cannot grow it.
	accessKeyCacheMax      = 256
	accessKeyTouchThrottle = time.Minute
	apiKeyHeaderName       = "x-api-key"
)

// ErrInvalidAPIKey reports a presented client key that does not
// authenticate a request.
var ErrInvalidAPIKey = errors.New("the API key is not valid")

// AccessKeySource is the store behind the data-plane gate.
type AccessKeySource interface {
	LookupAccessKey(ctx context.Context, id string) (appaccess.AccessKeyRecord, bool, error)
	MarkAccessKeyUsed(ctx context.Context, id string) error
}

type clientIdentity struct {
	ID     string
	Name   string
	Kind   string
	Client string
}

type identityKey struct{}

func withClientIdentity(ctx context.Context, identity clientIdentity) context.Context {
	return context.WithValue(ctx, identityKey{}, identity)
}

func clientIdentityFrom(ctx context.Context) (clientIdentity, bool) {
	identity, found := ctx.Value(identityKey{}).(clientIdentity)
	return identity, found
}

// AccessKeyGate authenticates inbound inference requests against the stored
// client keys. It compares digests in constant time, and only against the
// key the presented token names, so a wrong secret never matches another key.
type AccessKeyGate struct {
	mu       sync.Mutex
	source   AccessKeySource
	now      func() time.Time
	logger   *slog.Logger
	ttl      time.Duration
	throttle time.Duration
	cache    map[string]gateEntry
	touched  map[string]time.Time
}

type gateEntry struct {
	record appaccess.AccessKeyRecord
	found  bool
	at     time.Time
}

// NewAccessKeyGate returns a gate over the given key store. A nil source
// leaves the gate refusing every key, which is what a build without a
// service does.
func NewAccessKeyGate(source AccessKeySource, clk clock.Clock, logger *slog.Logger) *AccessKeyGate {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	if clk == nil {
		clk = clock.New()
	}
	return &AccessKeyGate{
		source: source, now: clk.Now, logger: logger,
		ttl: accessKeyCacheTTL, throttle: accessKeyTouchThrottle,
		cache: map[string]gateEntry{}, touched: map[string]time.Time{},
	}
}

// Invalidate forgets one cached key, which is how a revocation a caller just
// made takes effect on the next request instead of within the cache window.
func (g *AccessKeyGate) Invalidate(id string) {
	if g == nil || id == "" {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.cache, id)
	delete(g.touched, id)
}

// Verify reports which key presented a token, or ErrInvalidAPIKey when no
// live key matches it.
func (g *AccessKeyGate) Verify(ctx context.Context, token string) (clientIdentity, error) {
	if g == nil || g.source == nil {
		return clientIdentity{}, ErrInvalidAPIKey
	}
	id, secret, err := access.Parse(token)
	if err != nil {
		return clientIdentity{}, ErrInvalidAPIKey
	}
	record, found, err := g.record(ctx, id)
	if err != nil {
		return clientIdentity{}, err
	}
	if !found {
		return clientIdentity{}, ErrInvalidAPIKey
	}
	if subtle.ConstantTimeCompare([]byte(access.Digest(secret)), []byte(record.Digest)) != 1 {
		return clientIdentity{}, ErrInvalidAPIKey
	}
	now := g.now()
	if record.RevokedAtMs > 0 {
		return clientIdentity{}, ErrInvalidAPIKey
	}
	if record.ExpiresAtMs > 0 && !now.Before(time.UnixMilli(record.ExpiresAtMs)) {
		return clientIdentity{}, ErrInvalidAPIKey
	}
	g.markUsed(ctx, record.ID, now)
	return clientIdentity{
		ID: record.ID, Name: record.Name, Kind: record.Kind, Client: record.Client,
	}, nil
}

func (g *AccessKeyGate) record(ctx context.Context, id string) (appaccess.AccessKeyRecord, bool, error) {
	now := g.now()
	g.mu.Lock()
	entry, cached := g.cache[id]
	if cached && now.Sub(entry.at) < g.ttl {
		g.mu.Unlock()
		return entry.record, entry.found, nil
	}
	g.mu.Unlock()
	record, found, err := g.source.LookupAccessKey(ctx, id)
	if err != nil {
		return appaccess.AccessKeyRecord{}, false, err
	}
	g.mu.Lock()
	if len(g.cache) >= accessKeyCacheMax {
		g.cache = map[string]gateEntry{}
	}
	g.cache[id] = gateEntry{record: record, found: found, at: now}
	g.mu.Unlock()
	return record, found, nil
}

func (g *AccessKeyGate) markUsed(ctx context.Context, id string, now time.Time) {
	g.mu.Lock()
	last, seen := g.touched[id]
	if seen && now.Sub(last) < g.throttle {
		g.mu.Unlock()
		return
	}
	g.touched[id] = now
	g.mu.Unlock()
	if err := g.source.MarkAccessKeyUsed(context.WithoutCancel(ctx), id); err != nil {
		g.logger.Debug("record access key use", "key", id, "error", err)
	}
}

func describeClient(ctx context.Context) (id, name, client string) {
	identity, found := clientIdentityFrom(ctx)
	if !found {
		return "", "", ""
	}
	return identity.ID, identity.Name, identity.Client
}

func presentedKey(r *http.Request) (string, bool) {
	if provided, found := bearerToken(r); found {
		return string(provided), true
	}
	if value := r.Header.Get(apiKeyHeaderName); value != "" {
		return value, true
	}
	return "", false
}

func (s *Server) apiKeyGuard(next http.Handler) http.Handler {
	return s.keyGuard(next, false)
}

func (s *Server) anthropicGuard(next http.Handler) http.Handler {
	return s.keyGuard(next, true)
}

func (s *Server) keyGuard(next http.Handler, allowClaudeLogin bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, found := presentedKey(r)
		if !found {
			s.guardMissingKey(w, r, next, allowClaudeLogin)
			return
		}
		s.guardPresentedKey(w, r, next, token, allowClaudeLogin)
	})
}

func (s *Server) guardMissingKey(w http.ResponseWriter, r *http.Request, next http.Handler, allowClaudeLogin bool) {
	if allowClaudeLogin && isLoopback(r.RemoteAddr) {
		next.ServeHTTP(w, r.WithContext(withClaudeLogin(r.Context())))
		return
	}
	s.fail(w, r, refusal{
		Status: http.StatusUnauthorized, Code: "unauthorized", Message: "api.keys.missing_header",
	})
}

func (s *Server) guardPresentedKey(w http.ResponseWriter, r *http.Request, next http.Handler, token string, allowClaudeLogin bool) {
	identity, err := s.gate.Verify(r.Context(), token)
	switch {
	case err == nil:
		next.ServeHTTP(w, r.WithContext(withClientIdentity(r.Context(), identity)))
	case errors.Is(err, ErrInvalidAPIKey) && allowClaudeLogin && isLoopback(r.RemoteAddr):
		next.ServeHTTP(w, r.WithContext(withClaudeLogin(r.Context())))
	case errors.Is(err, ErrInvalidAPIKey):
		s.fail(w, r, refusal{
			Status: http.StatusUnauthorized, Code: "unauthorized", Message: "api.keys.invalid",
		})
	default:
		s.opts.Logger.Error("verify an API key", "error", err)
		s.fail(w, r, refusal{
			Status: http.StatusServiceUnavailable, Code: "service_unavailable", Message: "api.keys.store_unavailable",
		})
	}
}
