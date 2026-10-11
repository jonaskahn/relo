// Session leases: the stable identity a request with no named conversation
// still sends, so a provider never sees a fresh session per request.
package account

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"

	"github.com/jonaskahn/relo/internal/clock"
)

const (
	sessionLeaseTTL = 30 * time.Minute
	// sessionLeaseMaxEntries bounds the store, so a client that never names a
	// conversation cannot grow it without limit.
	sessionLeaseMaxEntries = 10000
	// sessionLeaseIDBytes is the entropy one synthetic session id carries.
	sessionLeaseIDBytes = 16
)

// SessionKey identifies the client a lease belongs to: which client, which of
// its client keys, and which upstream credential it is talking to.
type SessionKey struct {
	ClientKind string
	ClientKey  string
	Credential string
}

// SessionLeases hands out one synthetic session id per (client, key,
// credential) triple, held while a client keeps using it. A provider that
// sees a new session per request can flag the traffic as disposable; a lease
// keeps an unlabelled conversation looking like the same one.
type SessionLeases struct {
	mu      sync.Mutex
	clock   clock.Clock
	ttl     time.Duration
	max     int
	entries map[SessionKey]sessionLease
}

type sessionLease struct {
	id        string
	expiresAt time.Time
}

// NewSessionLeases returns a lease store bound to maxEntries leases and a ttl
// per lease. Zero values keep the documented defaults.
func NewSessionLeases(maxEntries int, ttl time.Duration, clk clock.Clock) *SessionLeases {
	if clk == nil {
		clk = clock.New()
	}
	if maxEntries <= 0 {
		maxEntries = sessionLeaseMaxEntries
	}
	if ttl <= 0 {
		ttl = sessionLeaseTTL
	}
	return &SessionLeases{
		clock: clk, ttl: ttl, max: maxEntries,
		entries: map[SessionKey]sessionLease{},
	}
}

// Lease returns the session id for a key, minting one when no live lease is
// held. Refreshing the deadline is what keeps one conversation on one id
// across separate requests, and an expired lease is replaced by a fresh id.
func (l *SessionLeases) Lease(key SessionKey) string {
	if l == nil {
		return newSessionID()
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.clock.Now()
	if entry, found := l.entries[key]; found && now.Before(entry.expiresAt) {
		entry.expiresAt = now.Add(l.ttl)
		l.entries[key] = entry
		return entry.id
	}
	l.evictOverflow()
	id := newSessionID()
	l.entries[key] = sessionLease{id: id, expiresAt: now.Add(l.ttl)}
	return id
}

// Evict forgets one client's lease. A client that stops talking to a
// credential gets a fresh session the next time, rather than the old id.
func (l *SessionLeases) Evict(key SessionKey) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.entries, key)
}

// Len returns how many leases are held right now.
func (l *SessionLeases) Len() int {
	if l == nil {
		return 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.entries)
}

func (l *SessionLeases) evictOverflow() {
	for len(l.entries) >= l.max {
		var oldestKey SessionKey
		var oldestAt time.Time
		for key, entry := range l.entries {
			if oldestAt.IsZero() || entry.expiresAt.Before(oldestAt) {
				oldestKey, oldestAt = key, entry.expiresAt
			}
		}
		if oldestAt.IsZero() {
			return
		}
		delete(l.entries, oldestKey)
	}
}

// newSessionID mints an unguessable session id in the UUID shape the Claude
// and Codex protocols expect.
func newSessionID() string {
	var raw [sessionLeaseIDBytes]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return hex.EncodeToString(raw[:])
	}
	raw[6] = (raw[6] & 0x0f) | 0x40
	raw[8] = (raw[8] & 0x3f) | 0x80
	return formatSessionID(raw)
}

func formatSessionID(raw [sessionLeaseIDBytes]byte) string {
	const hexDigits = "0123456789abcdef"
	out := make([]byte, 0, 36)
	for index, value := range raw {
		if index == 4 || index == 6 || index == 8 || index == 10 {
			out = append(out, '-')
		}
		out = append(out, hexDigits[value>>4], hexDigits[value&0x0f])
	}
	return string(out)
}
