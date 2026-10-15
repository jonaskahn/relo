// Native opencode identifiers, forged the way the client mints them.
//
// The gateway reads the client's clock out of an identifier's hex prefix, so
// a session that only has the shape is answered 403. This ports
// packages/schema/src/identifier.ts create(): 12 hex digits holding
// ~((milliseconds * 0x1000) + counter) for sessions (descending) or the plain
// value for request ids (ascending), plus 14 random characters drawn % 62
// from the client's own alphabet.
package wire

import (
	"crypto/rand"
	"encoding/hex"
	mathrand "math/rand"
	"strings"
	"sync"
	"time"
)

// nativeIdentifierAlphabet is the character set the gateway accepts in the
// tail of an identifier, byte for byte the client's own.
const nativeIdentifierAlphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// nativeIdentifierTail is the random tail length every identifier carries.
const nativeIdentifierTail = 14

var nativeClock = struct {
	sync.Mutex
	last    int64
	counter int64
}{}

// nativeIdentifierBody renders one identifier body for an explicit clock, so
// tests pin it against the TypeScript formula. Production passes the current
// time through nativeMint.
func nativeIdentifierBody(descending bool, ms, seq int64, tail []byte) string {
	current := ms*0x1000 + seq
	var bits uint64
	if descending {
		bits = uint64(^current)
	} else {
		bits = uint64(current)
	}
	var prefix [6]byte
	for i := range 6 {
		prefix[i] = byte(bits >> (40 - 8*i))
	}
	out := make([]byte, 0, 12+nativeIdentifierTail)
	out = append(out, hex.EncodeToString(prefix[:])...)
	for _, b := range tail {
		out = append(out, nativeIdentifierAlphabet[int(b)%len(nativeIdentifierAlphabet)])
	}
	return string(out)
}

var fallbackRand = struct {
	sync.Mutex
	source *mathrand.Rand
}{source: mathrand.New(mathrand.NewSource(time.Now().UnixNano()))}

// nativeMint mints one identifier body with the current clock. Crypto
// randomness is effectively infallible; a failure still returns a
// shape-correct body from the fallback source rather than no identifier.
func nativeMint(descending bool) string {
	nativeClock.Lock()
	ms := time.Now().UnixMilli()
	if ms != nativeClock.last {
		nativeClock.last = ms
		nativeClock.counter = 0
	}
	nativeClock.counter++
	seq := nativeClock.counter
	nativeClock.Unlock()

	tail := make([]byte, nativeIdentifierTail)
	if _, err := rand.Read(tail); err != nil {
		fallbackRand.Lock()
		for i := range tail {
			tail[i] = byte(fallbackRand.source.Intn(256))
		}
		fallbackRand.Unlock()
	}
	return nativeIdentifierBody(descending, ms, seq, tail)
}

// NativeSessionID mints a session the way a native conversation start does:
// descending, newest first.
func NativeSessionID() string {
	return "ses_" + nativeMint(true)
}

// NativeRequestID mints the message identifier one attempt carries:
// ascending, oldest first.
func NativeRequestID() string {
	return "msg_" + nativeMint(false)
}

// projectForgeMin and projectForgeSpan bound the rotation of a forged
// project id: every epoch lasts between three and twenty minutes, and each
// epoch draws its own successor, so there is no fixed cadence to fingerprint.
const (
	projectForgeMin  = 3 * time.Minute
	projectForgeSpan = 17 * time.Minute
)

// projectForgeIDLength is the SHA-1 hex length a native project id carries.
const projectForgeIDLength = 40

// ProjectForge mints the project id sent when the client named none. The
// value rotates on a random 3-20 minute schedule; the zero value is usable
// and mints on first use. Inject now in tests to steer the clock.
type ProjectForge struct {
	mu       sync.Mutex
	current  string
	rotateAt time.Time
	now      func() time.Time
}

// clock reports the forge's time, real unless a test steers it.
func (p *ProjectForge) clock() time.Time {
	if p.now != nil {
		return p.now()
	}
	return time.Now()
}

// Get returns the current forged id, rotating it when its epoch expired.
func (p *ProjectForge) Get() string {
	now := p.clock()
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.current == "" || !now.Before(p.rotateAt) {
		raw := make([]byte, projectForgeIDLength/2)
		if _, err := rand.Read(raw); err != nil {
			return p.current
		}
		p.current = hex.EncodeToString(raw)
		p.rotateAt = now.Add(projectForgeMin + time.Duration(randInt63(int64(projectForgeSpan/time.Second)))*time.Second)
	}
	return p.current
}

// randInt63 draws [0, max) from crypto randomness, falling back to the
// middle of the range when randomness is unavailable.
func randInt63(max int64) int64 {
	raw := make([]byte, 8)
	if _, err := rand.Read(raw); err != nil {
		return max / 2
	}
	var value int64
	for _, b := range raw {
		value = (value << 8) | int64(b)
	}
	if value < 0 {
		value = ^value
	}
	return value % max
}

// InboundIdentity carries what the inbound client said about itself, so the
// free lane relays the client's own identity instead of inventing one.
// Session is the conversation session the caller resolved for the lane:
// the inbound affinity when the client named its conversation, a forged
// native session the caller keeps per lane, or "" to derive one from the
// lane for callers without a conversation.
type InboundIdentity struct {
	Session         string
	Affinity        string
	UserAgent       string
	Project         string
	ParentSessionID string
}

// hasOpenCodeMarker reports whether the agent identifies as an opencode
// client, the only identity the gateway reads the agent for.
func hasOpenCodeMarker(agent string) bool {
	return strings.Contains(strings.ToLower(agent), "opencode/")
}

// ValidSessionID reports whether the value has the session shape the
// gateway accepts.
func ValidSessionID(value string) bool {
	return openCodeSession.MatchString(value)
}
