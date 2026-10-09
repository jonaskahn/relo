// OpenCode headers preserve the gateway's conversation identity.
package wire

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/internal/inference"
)

var openCodeSession = regexp.MustCompile(`^ses_[0-9a-f]{12}[0-9A-Za-z]{14}$`)

// freeIdentifierAlphabet is the character set the gateway accepts in the tail
// of an identifier.
const freeIdentifierAlphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// freeSessionDomain separates Relo's session names from any other producer's,
// so two clients that name the same conversation cannot land on one session.
const freeSessionDomain = "relo/opencode-free/session/v1\x00"

// OpenCodeFreeHeaders returns the signed-out CLI headers, with one session
// per conversation lane and a fresh message identifier for each attempt.
func OpenCodeFreeHeaders(lane, inboundSession string) (map[string]string, error) {
	request, err := freeRequestID()
	if err != nil {
		return nil, err
	}
	return map[string]string{
		"Authorization":      "Bearer public",
		"User-Agent":         "opencode/1.18.33",
		"x-opencode-client":  "cli",
		"x-opencode-project": "global",
		"x-opencode-session": freeSessionID(lane, inboundSession),
		"x-opencode-request": request,
	}, nil
}

// freeSessionID is the session one conversation is sent on. It is derived from
// the lane rather than remembered for it, because a daemon that outlives many
// conversations would otherwise hold a session for every one it ever served.
func freeSessionID(lane, inboundSession string) string {
	if openCodeSession.MatchString(inboundSession) {
		return inboundSession
	}
	digest := sha256.Sum256([]byte(freeSessionDomain + lane))
	return "ses_" + hex.EncodeToString(digest[:6]) + freeIdentifierTail(digest[6:20])
}

// freeRequestID names one attempt. Every attempt gets its own, so a retry is
// never mistaken for the request it replaces.
func freeRequestID() (string, error) {
	var random [20]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", fmt.Errorf("create an OpenCode identifier: %w", err)
	}
	return "msg_" + hex.EncodeToString(random[:6]) + freeIdentifierTail(random[6:20]), nil
}

func freeIdentifierTail(digest []byte) string {
	var out strings.Builder
	out.Grow(len(digest))
	for _, b := range digest {
		out.WriteByte(freeIdentifierAlphabet[int(b)%len(freeIdentifierAlphabet)])
	}
	return out.String()
}

// freeLaneTools are the declarations the signed-out gateway inspects on every
// model except the one it answers without tools. A plain chat turn carries
// none of them, so the adaptation below declares exactly these.
var freeLaneTools = []string{"bash", "glob", "grep", "read"}

// OpenCodeFreeRequest returns the request the signed-out gateway accepts. A
// turn without the declarations it inspects is refused, so missing names are
// appended as unavailable tools on a copy; the caller's request is never
// mutated because it is shared between route candidates. Only the exact
// lowercase names satisfy the lane, so differently-cased client tools stay
// and the missing lowercase declarations are still appended. Real tools and
// an explicit tool choice are left alone, and a tool-free chat turn forbids
// tool calls the way the working client does.
func OpenCodeFreeRequest(req *inference.Request, format catalog.APIFormat) *inference.Request {
	if req == nil {
		return nil
	}
	copied := *req
	copied.Tools = append([]inference.Tool(nil), req.Tools...)
	present := make(map[string]bool, len(req.Tools))
	for _, tool := range req.Tools {
		present[tool.Name] = true
	}
	for _, want := range freeLaneTools {
		if present[want] {
			continue
		}
		copied.Tools = append(copied.Tools, inference.Tool{
			Name:        want,
			Description: "This tool is currently unavailable and must not be used.",
			Parameters:  json.RawMessage(`{"type":"object","properties":{}}`),
		})
	}
	if len(req.Tools) == 0 && len(copied.Tools) > 0 && req.ToolChoice == nil &&
		format == catalog.FormatOpenAIChat {
		copied.ToolChoice = &inference.ToolChoice{Mode: inference.ToolChoiceNone}
	}
	return &copied
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
