// OpenCode headers preserve the gateway's conversation identity.
package wire

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

type openCodeIdentifiers struct {
	mu        sync.Mutex
	sessions  map[string]string
	timestamp int64
	counter   uint64
}

// newIdentifiers opens an empty lane-to-session table.
func newIdentifiers() *openCodeIdentifiers {
	return &openCodeIdentifiers{sessions: make(map[string]string)}
}

// freeIdentifiers is the process-wide lane table the gateway headers read.
// All access goes through its methods, so the lock discipline lives in one place.
var freeIdentifiers = newIdentifiers()

var openCodeSession = regexp.MustCompile(`^ses_[0-9a-f]{12}[0-9A-Za-z]{14}$`)

// OpenCodeFreeHeaders returns the signed-out CLI headers, with one session
// per conversation lane and a fresh message identifier for each attempt.
func OpenCodeFreeHeaders(lane, inboundSession string) (map[string]string, error) {
	return freeIdentifiers.headers(lane, inboundSession)
}

// headers resolves one lane to its session and mints its request identifier.
func (ids *openCodeIdentifiers) headers(lane, inboundSession string) (map[string]string, error) {
	ids.mu.Lock()
	defer ids.mu.Unlock()
	session := inboundSession
	if !openCodeSession.MatchString(session) {
		session = ids.sessions[lane]
		if session == "" {
			var err error
			session, err = ids.identifier("ses_", true)
			if err != nil {
				return nil, err
			}
			ids.sessions[lane] = session
		}
	}
	request, err := ids.identifier("msg_", false)
	if err != nil {
		return nil, err
	}
	return map[string]string{
		"Authorization":      "Bearer public",
		"User-Agent":         "opencode/1.18.33",
		"x-opencode-client":  "cli",
		"x-opencode-project": "global",
		"x-opencode-session": session,
		"x-opencode-request": request,
	}, nil
}

func (ids *openCodeIdentifiers) identifier(prefix string, descending bool) (string, error) {
	now := time.Now().UnixMilli()
	if now != ids.timestamp {
		ids.timestamp, ids.counter = now, 0
	}
	ids.counter++
	value := uint64(now)*0x1000 + ids.counter
	if descending {
		value = ^value
	}
	var random [14]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", fmt.Errorf("create an OpenCode identifier: %w", err)
	}
	const alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	for i, b := range random {
		random[i] = alphabet[int(b)%len(alphabet)]
	}
	return fmt.Sprintf("%s%012x%s", prefix, value&0xffffffffffff, random[:]), nil
}

// OpenCodeGoSessionHeader is the header the OpenCode Go gateway requires
// before it will route a request to a session.
const OpenCodeGoSessionHeader = "x-opencode-session"

// OpenCodeGo reports whether baseURL is the OpenCode Go gateway. The
// gateway rejects a request that arrives without a session header.
func OpenCodeGo(baseURL string) bool {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || parsed.Hostname() == "" {
		return false
	}
	if !strings.EqualFold(parsed.Hostname(), "opencode.ai") {
		return false
	}
	return strings.Contains(strings.ToLower(parsed.EscapedPath()), "/zen/go")
}

// OpenCodeFree reports whether baseURL is the signed-out OpenCode Zen
// gateway, which keeps no account and prices nothing. The Go path is
// OpenCodeGo; a custom URL on the same gateway counts the same way.
func OpenCodeFree(baseURL string) bool {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || parsed.Hostname() == "" {
		return false
	}
	if !strings.EqualFold(parsed.Hostname(), "opencode.ai") {
		return false
	}
	path := strings.ToLower(parsed.EscapedPath())
	return strings.Contains(path, "/zen") && !strings.Contains(path, "/zen/go")
}

// WithOpenCodeGoSession returns headers with a stable session for one
// OpenCode Go destination. An operator-set session is left alone, and any
// other host is returned unchanged. An empty lane is left for the caller
// to fill, because minting one here would give each retry a new value.
func WithOpenCodeGoSession(headers map[string]string, baseURL, wireFormat, lane string) map[string]string {
	if !OpenCodeGo(baseURL) || lane == "" || hasHeader(headers, OpenCodeGoSessionHeader) {
		return headers
	}
	copied := copyHeaders(headers)
	copied[OpenCodeGoSessionHeader] = OpenCodeGoSessionID(lane, wireFormat)
	return copied
}

// OpenCodeGoSessionID returns the opaque session the Go gateway keeps a
// conversation on. The same lane and wire format always produce the same
// value, and a different lane or format produces a different one.
func OpenCodeGoSessionID(lane, wireFormat string) string {
	digest := sha256.New()
	_, _ = digest.Write([]byte("relo/opencode-go/session/v1\x00"))
	_, _ = digest.Write([]byte(wireFormat))
	_, _ = digest.Write([]byte{0})
	_, _ = digest.Write([]byte(lane))
	return "relo_" + hex.EncodeToString(digest.Sum(nil)[:16])
}

func hasHeader(headers map[string]string, name string) bool {
	for existing := range headers {
		if strings.EqualFold(existing, name) {
			return true
		}
	}
	return false
}

func copyHeaders(headers map[string]string) map[string]string {
	copied := make(map[string]string, len(headers)+1)
	for name, value := range headers {
		copied[name] = value
	}
	return copied
}
