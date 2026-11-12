// OpenCode headers preserve the gateway's conversation identity.
package wire

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"hash/fnv"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/internal/inference"
)

var openCodeSession = regexp.MustCompile(`^ses_[0-9a-f]{12}[0-9A-Za-z]{14}$`)

// freeClientUserAgent is the fallback identity for callers that are not an
// opencode client. The gateway refuses a turn whose User-Agent carries no
// "opencode/" marker, so a request that arrives without one cannot go out
// without one either; a real client's own agent is always preferred.
const freeClientUserAgent = "opencode/latest/2.0.27/cli"

// OpenCodeFreeHeaders returns the signed-out CLI headers, with one session
// per conversation lane and a fresh message identifier for each attempt. The
// inbound identity is relayed, never invented: the client's own agent,
// affinity, project, and parent travel when present, and only the gaps are
// filled — the agent name from the client or the fallback above, the project
// omitted, the session from the caller's forged value or a fresh clock-based
// identifier. The affinity spellings carry the conversation's root session, while
// x-opencode-session-id keeps the client's own session, the way the native
// client writes a fork.
func OpenCodeFreeHeaders(lane string, id InboundIdentity) map[string]string {
	root := freeRootSession(lane, id)
	own := id.Session
	if !ValidSessionID(own) {
		own = root
	}
	request := NativeRequestID()
	headers := map[string]string{
		"Authorization":         "Bearer public",
		"User-Agent":            freeAgent(id.UserAgent),
		"x-opencode-client":     freeClient(id.Client),
		"x-opencode-session":    root,
		"x-opencode-session-id": own,
		"x-session-affinity":    root,
		"x-session-id":          root,
		"x-opencode-request":    request,
		"Accept":                "*/*",
	}
	// The project travels only when known: inbound, or operator-configured.
	// A forged rotating id fills the gap upstream of this function; a
	// hardcoded one would impersonate a stranger's repository.
	if project := strings.TrimSpace(id.Project); project != "" {
		headers["x-opencode-project"] = project
	}
	// Forks share the parent's affinity: both spellings travel when the
	// client named its parent, the way the native client sends them.
	if parent := strings.TrimSpace(id.ParentSessionID); parent != "" {
		headers["x-opencode-parent-session-id"] = parent
		headers["x-parent-session-id"] = parent
	}
	return headers
}

// freeRootSession is the conversation session every affinity spelling carries:
// the client's root, its own session when it named no root, or the lane's
// freshly minted session for a caller that named nothing.
func freeRootSession(lane string, id InboundIdentity) string {
	if ValidSessionID(id.Affinity) {
		return id.Affinity
	}
	if ValidSessionID(id.Session) {
		return id.Session
	}
	return freeSessionID(lane, "")
}

// freeClient names the agent that sent the turn. A client that labels itself
// keeps its name; anything else is the signed-out CLI's own name.
func freeClient(inbound string) string {
	if client := strings.TrimSpace(inbound); client != "" {
		return client
	}
	return "cli"
}

// freeAgent prefers the inbound agent when it identifies as an opencode
// client, and falls back to the pinned one otherwise.
func freeAgent(inbound string) string {
	if hasOpenCodeMarker(inbound) {
		return inbound
	}
	return freeClientUserAgent
}

func freeSessionID(_ string, inboundSession string) string {
	if openCodeSession.MatchString(inboundSession) {
		return inboundSession
	}
	return NativeSessionID()
}

// The free gateway refused a tool-bearing turn that named none of these
// declarations, so they are added beside a tool-bearing request. A tool-free
// turn gains nothing: a captured native title request that carried them was
// refused, and the same body without them was answered.
var freeLaneTools = []string{"bash", "glob", "grep", "read"}

// freeToolLimit is the most tool declarations the gateway accepts in one
// request. The free lane's own declarations count toward it.
const freeToolLimit = 128

const (
	toolPriorityRequired = iota
	toolPriorityNative
	toolPriorityCode
	toolPriorityOther
)

// codeToolPrefixes name the code-intelligence servers a coding session keeps
// ahead of unrelated MCP tools when a tool list has to be trimmed.
var codeToolPrefixes = []string{"mcp__codegraph__", "mcp__gitnexus__"}

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

// dropRejectedKeywords returns the schema the gateway accepts. A keyword that
// names an argument or a definition is kept: only keywords inside a schema
// object state a rule the gateway refuses. A schema that
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

// schemaNameMaps hold argument and definition names rather than schema
// keywords, so a key inside one names what a valid call carries instead of
// stating a rule the gateway refuses.
var schemaNameMaps = map[string]bool{
	"properties": true, "patternProperties": true,
	"$defs": true, "definitions": true,
}

func carriesRejectedKeyword(node any) bool {
	switch value := node.(type) {
	case map[string]any:
		for key, child := range value {
			if schemaNameMaps[key] {
				if named, ok := child.(map[string]any); ok {
					for _, schema := range named {
						if carriesRejectedKeyword(schema) {
							return true
						}
					}
				}
				continue
			}
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
			if schemaNameMaps[key] {
				if named, ok := value[key].(map[string]any); ok {
					for name := range named {
						named[name] = withoutRejectedKeywords(named[name])
					}
				}
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
// turn that declares tools gains the lane declarations it misses as unavailable
// tools on a copy; the caller's request is never mutated because it is shared
// between route candidates. Only the exact lowercase names satisfy the lane, so
// differently-cased client tools stay and the missing lowercase declarations are
// still appended. Real tools and an explicit tool choice are left alone. A turn
// that brought no tools keeps none: the gateway's demand for declarations is not
// general, and appending placeholders to a tool-free turn is what turned a
// native title request into a refusal. A schema the gateway refuses, an output
// ceiling below its floor, and a tool name past its limit are corrected on the
// same copy. The returned map names each shortened tool again on the answer, and
// is empty when the gateway accepts every name.
func OpenCodeFreeRequest(req *inference.Request, format catalog.APIFormat) (*inference.Request, map[string]string) {
	return openCodeFreeRequest(req, format, false, "")
}

// OpenCodeFreeRequestWithToolCutoff returns the request the signed-out gateway
// accepts, trimming tool declarations the gateway would refuse. The seed keeps
// the random fill stable for one conversation.
func OpenCodeFreeRequestWithToolCutoff(req *inference.Request, format catalog.APIFormat, seed string) (*inference.Request, map[string]string) {
	return openCodeFreeRequest(req, format, true, seed)
}

func openCodeFreeRequest(req *inference.Request, format catalog.APIFormat, cutTools bool, seed string) (*inference.Request, map[string]string) {
	if req == nil {
		return nil, nil
	}
	copied := *req
	// Only the environment instruction is required; preserve the client's task.
	if !HasOpenCodeSystemPrompt(req) {
		copied.Messages = withOpenCodeSystemPrompt(copied.Messages, copied.WorkingDirectory)
	}
	if len(req.Tools) > 0 {
		copied.Tools = withFreeLaneTools(req.Tools)
	}
	if cutTools {
		copied.Tools = cutFreeTools(copied.Tools, copied.ToolChoice, seed)
	}
	// The schemas are cleaned on the copy the attempt sends, so a lane tool
	// added above is cleaned as well as one a client sent.
	for index := range copied.Tools {
		copied.Tools[index].Parameters = dropRejectedKeywords(copied.Tools[index].Parameters)
	}
	if copied.MaxTokens > 0 && copied.MaxTokens < freeMinMaxTokens {
		copied.MaxTokens = freeMinMaxTokens
	}
	return &copied, shortToolNames(&copied)
}

type toolCandidate struct {
	index    int
	priority int
	key      uint64
}

func cutFreeTools(tools []inference.Tool, choice *inference.ToolChoice, seed string) []inference.Tool {
	if len(tools) <= freeToolLimit {
		return tools
	}
	candidates := freeToolCandidates(tools, freeRequiredTools(choice), seed)
	sort.Slice(candidates, func(i, j int) bool { return toolCandidateLess(candidates[i], candidates[j]) })
	return keepFreeTools(tools, candidates[:freeToolLimit])
}

func freeRequiredTools(choice *inference.ToolChoice) map[string]bool {
	required := make(map[string]bool, len(freeLaneTools)+1)
	for _, name := range freeLaneTools {
		required[name] = true
	}
	if choice != nil && choice.Name != "" {
		required[choice.Name] = true
	}
	return required
}

func freeToolCandidates(tools []inference.Tool, required map[string]bool, seed string) []toolCandidate {
	candidates := make([]toolCandidate, len(tools))
	for index, tool := range tools {
		candidates[index] = toolCandidate{
			index: index, priority: toolPriority(tool.Name, required), key: toolCutoffKey(seed, tool.Name),
		}
	}
	return candidates
}

func toolCandidateLess(left, right toolCandidate) bool {
	if left.priority != right.priority {
		return left.priority < right.priority
	}
	if left.key != right.key {
		return left.key < right.key
	}
	return left.index < right.index
}

func keepFreeTools(tools []inference.Tool, candidates []toolCandidate) []inference.Tool {
	kept := make([]bool, len(tools))
	for _, candidate := range candidates {
		kept[candidate.index] = true
	}
	out := make([]inference.Tool, 0, freeToolLimit)
	for index, tool := range tools {
		if kept[index] {
			out = append(out, tool)
		}
	}
	return out
}

func toolPriority(name string, required map[string]bool) int {
	if required[name] {
		return toolPriorityRequired
	}
	if !strings.HasPrefix(name, "mcp__") {
		return toolPriorityNative
	}
	for _, prefix := range codeToolPrefixes {
		if strings.HasPrefix(name, prefix) {
			return toolPriorityCode
		}
	}
	return toolPriorityOther
}

func toolCutoffKey(seed, name string) uint64 {
	digest := fnv.New64a()
	_, _ = digest.Write([]byte(seed))
	_, _ = digest.Write([]byte{0})
	_, _ = digest.Write([]byte(name))
	return digest.Sum64()
}

func withFreeLaneTools(tools []inference.Tool) []inference.Tool {
	out := append([]inference.Tool(nil), tools...)
	present := make(map[string]bool, len(tools))
	for _, tool := range tools {
		present[tool.Name] = true
	}
	for _, want := range freeLaneTools {
		if present[want] {
			continue
		}
		out = append(out, inference.Tool{
			Name:        want,
			Description: "This tool is currently unavailable and must not be used.",
			Parameters:  json.RawMessage(`{"type":"object","properties":{}}`),
		})
	}
	return out
}

// OpenCodeGateway reports whether baseURL reaches an OpenCode gateway lane.
// The signed-out Zen lane and the paid Go lane share the gateway's contract,
// including the limit it puts on a tool name, and both the older /zen paths
// and the current /inference ones count.
func OpenCodeGateway(baseURL string) bool {
	path, ok := gatewayPath(baseURL)
	if !ok {
		return false
	}
	return strings.Contains(path, "/zen") || strings.Contains(path, "/inference")
}

// OpenCodeGatewayRequest returns the request an OpenCode gateway lane accepts,
// with the map that names each shortened tool again on the answer. The gateway
// refuses a tool name past its limit, and a coding session passes it as soon as
// it loads an MCP server, so the names go out under aliases. A paid lane keeps
// the client's request whole apart from the names: unlike the signed-out lane
// it demands no particular tools and refuses no schema keyword. A request whose
// names all fit is returned unchanged.
func OpenCodeGatewayRequest(req *inference.Request) (*inference.Request, map[string]string) {
	if req == nil {
		return nil, nil
	}
	copied := *req
	copied.Tools = append([]inference.Tool(nil), req.Tools...)
	return &copied, shortToolNames(&copied)
}

// OpenCodeGoSessionHeader is the header the OpenCode Go gateway requires
// before it will route a request to a session.
const OpenCodeGoSessionHeader = "x-opencode-session"

// OpenCodeGo reports whether baseURL is the OpenCode Go gateway. This is the
// lane that rejects a request arriving without a session header, and it counts
// on both the older /zen path and the current /inference one, because a
// connection keeps whichever base URL it was created with.
func OpenCodeGo(baseURL string) bool {
	path, ok := gatewayPath(baseURL)
	if !ok {
		return false
	}
	return strings.Contains(path, "/zen/go") || strings.Contains(path, "/inference/go")
}

// OpenCodeFree reports whether baseURL is the signed-out OpenCode gateway,
// which keeps no account and prices nothing. The older /zen path and the
// current /inference one are the same lane, and the Go lane is OpenCodeGo; a
// custom URL on the same gateway counts the same way.
func OpenCodeFree(baseURL string) bool {
	path, ok := gatewayPath(baseURL)
	if !ok {
		return false
	}
	return (strings.Contains(path, "/zen") && !strings.Contains(path, "/zen/go")) ||
		(strings.Contains(path, "/inference") && !strings.Contains(path, "/inference/go"))
}

// gatewayPath returns the lowercased path of a URL on the OpenCode gateway,
// and whether the address is on that host at all, so no lane can match someone
// else's address.
func gatewayPath(baseURL string) (string, bool) {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || parsed.Hostname() == "" {
		return "", false
	}
	if !strings.EqualFold(parsed.Hostname(), "opencode.ai") {
		return "", false
	}
	return strings.ToLower(parsed.EscapedPath()), true
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
