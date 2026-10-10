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

// freeMinMaxTokens is the smallest output ceiling the gateway accepts. A client
// that asks for less to keep a probe cheap is refused, so a stated ceiling is
// raised to this floor and no further: it is the smallest value that passes,
// not a recommended one.
const freeMinMaxTokens = 16

// freeRejectedKeywords are the schema keywords the gateway refuses. It answers
// "Invalid JSON schema ... not valid under any of the schemas listed in the
// 'anyOf' keyword" naming the offending fragment, so this is the refusals
// observed rather than a dialect the gateway publishes. Every entry is either
// a validation annotation that narrows nothing a tool may legitimately be asked
// to do, or a dialect declaration with no runtime meaning. A keyword that
// shapes what a valid call is — oneOf, allOf, additionalProperties — is not
// listed: dropping those would leave the model calling a tool wrongly rather
// than fix a refusal. Widen this only when an upstream error names a keyword.
var freeRejectedKeywords = map[string]bool{
	"pattern": true, "minLength": true, "maxLength": true,
	"$schema": true,
}

// dropRejectedKeywords returns the schema the gateway accepts. A schema that
// does not decode, or that decodes but will not re-encode, arrives as it came,
// because a schema Relo cannot parse is one it must not alter. One that needs
// no change arrives as it came too: a coding turn declares a hundred tools, and
// re-marshalling every schema it already satisfies would cost more than the
// refusals it prevents.
func dropRejectedKeywords(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return raw
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return raw
	}
	if !carriesRejectedKeyword(decoded) {
		return raw
	}
	cleaned, err := json.Marshal(withoutRejectedKeywords(decoded))
	if err != nil {
		return raw
	}
	return cleaned
}

func carriesRejectedKeyword(node any) bool {
	switch value := node.(type) {
	case map[string]any:
		for key, child := range value {
			if freeRejectedKeywords[key] || carriesRejectedKeyword(child) {
				return true
			}
		}
	case []any:
		for _, child := range value {
			if carriesRejectedKeyword(child) {
				return true
			}
		}
	}
	return false
}

func withoutRejectedKeywords(node any) any {
	switch value := node.(type) {
	case map[string]any:
		for key := range value {
			if freeRejectedKeywords[key] {
				delete(value, key)
				continue
			}
			value[key] = withoutRejectedKeywords(value[key])
		}
		return value
	case []any:
		for i, child := range value {
			value[i] = withoutRejectedKeywords(child)
		}
		return value
	default:
		return node
	}
}

// freeToolNameLimit is the longest tool name the gateway accepts. A client that
// names a tool past it is refused with "`name` must be at most 64 characters",
// which a coding session reaches as soon as it loads an MCP server.
const freeToolNameLimit = 64

// freeToolAliasDigest is the digest bytes that keep two long names apart, so a
// name that shares a head with another still shortens to its own alias.
const freeToolAliasDigest = 6

// freeToolAlias returns the name the gateway accepts for one tool. A name
// within the limit is returned unchanged; a longer one keeps a readable head
// and gains a digest of the whole name, which the answer is read back through.
func freeToolAlias(name string) string {
	if len(name) <= freeToolNameLimit {
		return name
	}
	digest := sha256.Sum256([]byte(name))
	suffix := "_" + hex.EncodeToString(digest[:freeToolAliasDigest])
	return name[:freeToolNameLimit-len(suffix)] + suffix
}

func carriesLongToolName(req *inference.Request) bool {
	for _, tool := range req.Tools {
		if len(tool.Name) > freeToolNameLimit {
			return true
		}
	}
	if req.ToolChoice != nil && len(req.ToolChoice.Name) > freeToolNameLimit {
		return true
	}
	for _, message := range req.Messages {
		for _, call := range message.ToolCalls {
			if len(call.Name) > freeToolNameLimit {
				return true
			}
		}
	}
	return false
}

// shortToolNames rewrites every name the gateway would refuse on the copy the
// caller owns, and returns the map that names each alias again on the answer.
// A request whose names all fit is left whole, so an ordinary turn pays
// nothing. A name that appears only in a replayed call still gets the alias its
// declaration got, because the same name always shortens the same way.
func shortToolNames(req *inference.Request) map[string]string {
	if !carriesLongToolName(req) {
		return nil
	}
	originals := map[string]string{}
	alias := func(name string) string {
		short := freeToolAlias(name)
		if short != name {
			originals[short] = name
		}
		return short
	}
	for index := range req.Tools {
		req.Tools[index].Name = alias(req.Tools[index].Name)
	}
	if req.ToolChoice != nil {
		req.ToolChoice = &inference.ToolChoice{
			Mode: req.ToolChoice.Mode, Name: alias(req.ToolChoice.Name),
		}
	}
	shortReplayedCalls(req, alias)
	return originals
}

// shortReplayedCalls renames the tools a conversation already called, on fresh
// slices, because a route candidate shares the message array it arrived in.
func shortReplayedCalls(req *inference.Request, alias func(string) string) {
	req.Messages = append([]inference.Message(nil), req.Messages...)
	for index := range req.Messages {
		calls := req.Messages[index].ToolCalls
		if len(calls) == 0 {
			continue
		}
		renamed := append([]inference.ToolCall(nil), calls...)
		for call := range renamed {
			renamed[call].Name = alias(renamed[call].Name)
		}
		req.Messages[index].ToolCalls = renamed
	}
}

// OpenCodeFreeRequest returns the request the signed-out gateway accepts. A
// turn without the declarations it inspects is refused, so missing names are
// appended as unavailable tools on a copy; the caller's request is never
// mutated because it is shared between route candidates. Only the exact
// lowercase names satisfy the lane, so differently-cased client tools stay
// and the missing lowercase declarations are still appended. Real tools and
// an explicit tool choice are left alone, and a tool-free chat turn forbids
// tool calls the way the working client does. A schema the gateway refuses, an
// output ceiling below its floor, and a tool name past its limit are corrected
// on the same copy. The returned map names each shortened tool again on the
// answer, and is empty when the gateway accepts every name.
func OpenCodeFreeRequest(req *inference.Request, format catalog.APIFormat) (*inference.Request, map[string]string) {
	if req == nil {
		return nil, nil
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
	// The schemas are cleaned after the declarations are appended, so a lane tool
	// added later is cleaned as well as one a client sent.
	for index := range copied.Tools {
		copied.Tools[index].Parameters = dropRejectedKeywords(copied.Tools[index].Parameters)
	}
	if copied.MaxTokens > 0 && copied.MaxTokens < freeMinMaxTokens {
		copied.MaxTokens = freeMinMaxTokens
	}
	if len(req.Tools) == 0 && len(copied.Tools) > 0 && req.ToolChoice == nil &&
		format == catalog.FormatOpenAIChat {
		copied.ToolChoice = &inference.ToolChoice{Mode: inference.ToolChoiceNone}
	}
	return &copied, shortToolNames(&copied)
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
