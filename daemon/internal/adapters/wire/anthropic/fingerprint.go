// Subscription calls identify themselves as Claude Code on the official API.
package anthropic

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"github.com/jonaskahn/relo/internal/adapters/wire"
	"github.com/jonaskahn/relo/internal/inference"
)

const (
	// CLIVersion is the Claude Code release a subscription request claims to be.
	CLIVersion = "2.1.299"

	// CLIUserAgent is the User-Agent a subscription request sends when the
	// caller did not already name a claude-cli agent.
	CLIUserAgent = "claude-cli/" + CLIVersion + " (external, sdk-cli)"

	// ClaudeCodeBetaShort is the beta profile a subscription call advertises
	// while the caller did not name the million-token entry. It matches the
	// profile the installed Claude Code release sends, so a subscription
	// request is shaped like the client's own.
	ClaudeCodeBetaShort = "claude-code-20250219,oauth-2025-04-20," +
		"interleaved-thinking-2025-05-14,thinking-token-count-2026-05-13," +
		"context-management-2025-06-27,prompt-caching-scope-2026-01-05," +
		"advisor-tool-2026-03-01,advanced-tool-use-2025-11-20,effort-2025-11-24," +
		"dangerous-tool-use-2026-09-03,thinking-binding-controls-2026-08-01," +
		"afk-mode-2026-01-31,extended-cache-ttl-2025-04-11," +
		"cache-diagnosis-2026-04-07,message-threads-2026-08-12"

	// ClaudeCodeBeta is the profile a call that named a separate million-token
	// entry advertises: the same list plus the long-context beta, which is an
	// opt-in the client selects rather than a size the prompt crosses. A model
	// whose window is a million tokens on its own never sends it.
	ClaudeCodeBeta = ClaudeCodeBetaShort + "," + contextBeta

	// ClaudeCodeUsageBeta is the beta profile a subscription usage call
	// advertises. Usage reads the account's windows rather than running
	// the agent, so it leaves the agent-mode profile off the request.
	ClaudeCodeUsageBeta = "claude-code-20250219,oauth-2025-04-20," +
		"interleaved-thinking-2025-05-14,thinking-token-count-2026-05-13," +
		"context-management-2025-06-27,prompt-caching-scope-2026-01-05," +
		"mid-conversation-system-2026-04-07,per-turn-control-2026-07-01," +
		"mid-conversation-tool-changes-2026-07-01,inline-tools-2026-09-15," +
		"advisor-tool-2026-03-01,effort-2025-11-24," +
		"dangerous-tool-use-2026-09-03,extended-cache-ttl-2025-04-11"

	// contextBeta is the long-context opt-in Claude Code attaches when the
	// caller selects a separate million-token entry.
	contextBeta = "context-1m-2025-08-07"

	claudeAcceptEncoding = "gzip, deflate, br, zstd"
	stainlessLang        = "js"
	stainlessPackage     = "0.128.0"
	stainlessRuntime     = "node"
	stainlessNode        = "v26.3.0"
	stainlessTimeout     = "600"
	cchPlaceholder       = "cch=00000"
	cchSeed              = 0x4d659218e32a3268
	fingerprintSalt      = "59cf53e54c78"
	officialHost         = "api.anthropic.com"
	// dispatchID is the dispatch tag every native subscription call sends.
	dispatchID = "v2d"
	// requestClassMain is the request class a main-turn call advertises.
	requestClassMain = "main"
)

var modelVersion = regexp.MustCompile(`-(\d+)-(\d+)`)

func (c *Codec) authorizeClaudeCode(request *http.Request, opts wire.CodecOpts) {
	official := officialAnthropicHost(requestBase(c, opts))
	copyHeaders(request, opts.ExtraHeaders, notAuthorization)
	applyClaudeCodeHeaders(request, opts)
	if !official {
		copyHeaders(request, opts.ExtraHeaders, overridableOffOfficial)
		return
	}
	copyHeaders(request, opts.ExtraHeaders, overridableOnOfficial)
}

func requestBase(codec *Codec, opts wire.CodecOpts) string {
	if opts.BaseURL != "" {
		return opts.BaseURL
	}
	return codec.baseURL
}

func officialAnthropicHost(base string) bool {
	parsed, err := url.Parse(base)
	if err != nil {
		return false
	}
	return strings.EqualFold(parsed.Hostname(), officialHost)
}

func withBeta(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return raw + "?beta=true"
	}
	query := parsed.Query()
	query.Set("beta", "true")
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func applyClaudeCodeHeaders(request *http.Request, opts wire.CodecOpts) {
	request.Header.Set("Authorization", "Bearer "+opts.CredentialRef)
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(VersionHeader, APIVersion)
	request.Header.Set("anthropic-dangerous-direct-browser-access", "true")
	request.Header.Set("Connection", "keep-alive")
	request.Header.Set("Accept-Encoding", claudeAcceptEncoding)
	request.Header.Set(BetaHeader, claudeBetaFor(opts))
	request.Header.Set("x-app", "cli")
	request.Header.Set("User-Agent", claudeUserAgent(opts.ExtraHeaders))
	request.Header.Set("anthropic-dispatch-id", dispatchID)
	request.Header.Set("x-claude-code-request-class", requestClassMain)
	request.Header.Set("x-client-request-id", newRequestUUID())
	request.Header.Set("x-claude-code-prompt-id", newRequestUUID())
	if session := claudeSession(opts); session != "" {
		request.Header.Set("X-Claude-Code-Session-Id", session)
	}
	if key := headerValue(opts.ExtraHeaders, APIKeyHeader); key != "" {
		request.Header.Set(APIKeyHeader, key)
	}
	applyStainless(request)
}

func claudeBetaFor(opts wire.CodecOpts) string {
	if opts.LongContext && !opts.NativeMillion && contextBetaApplies(opts.UpstreamModel) {
		return ClaudeCodeBeta
	}
	return ClaudeCodeBetaShort
}

// contextBetaApplies reports whether the upstream model takes the opt-in
// long-context beta. Only opus and sonnet at 4.6 or older keep it; an empty
// or unparseable model id keeps the legacy behavior of sending it.
func contextBetaApplies(model string) bool {
	version := modelVersion.FindStringSubmatch(model)
	if version == nil {
		return true
	}
	major, err := strconv.Atoi(version[1])
	if err != nil {
		return true
	}
	minor, err := strconv.Atoi(version[2])
	if err != nil {
		return true
	}
	return major < 4 || (major == 4 && minor <= 6)
}

// newRequestUUID returns a random v4 UUID for a per-attempt correlation
// header. It IDs one attempt, so retries of a conversation never share one.
func newRequestUUID() string {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return ""
	}
	id[6] = id[6]&0x0f | 0x40
	id[8] = id[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", id[0:4], id[4:6], id[6:8], id[8:10], id[10:16])
}

func claudeUserAgent(headers map[string]string) string {
	agent := headerValue(headers, "User-Agent")
	if strings.HasPrefix(strings.ToLower(agent), "claude-cli") {
		return agent
	}
	return CLIUserAgent
}

func claudeSession(opts wire.CodecOpts) string {
	if opts.SessionID != "" {
		return opts.SessionID
	}
	if opts.SessionAnchor != "" {
		return opts.SessionAnchor
	}
	return opts.RequestID
}

func applyStainless(request *http.Request) {
	request.Header.Set("X-Stainless-Arch", stainlessArch(runtime.GOARCH))
	request.Header.Set("X-Stainless-Lang", stainlessLang)
	request.Header.Set("X-Stainless-OS", stainlessOS(runtime.GOOS))
	request.Header.Set("X-Stainless-Package-Version", stainlessPackage)
	request.Header.Set("X-Stainless-Retry-Count", "0")
	request.Header.Set("X-Stainless-Runtime", stainlessRuntime)
	request.Header.Set("X-Stainless-Runtime-Version", stainlessNode)
	request.Header.Set("X-Stainless-Timeout", stainlessTimeout)
}

func stainlessArch(arch string) string {
	switch arch {
	case "amd64":
		return "x64"
	case "386":
		return "x86"
	default:
		return arch
	}
}

func stainlessOS(goos string) string {
	switch goos {
	case "darwin":
		return "MacOS"
	case "linux":
		return "Linux"
	case "windows":
		return "Windows"
	case "freebsd":
		return "FreeBSD"
	case "android":
		return "Android"
	default:
		return goos
	}
}

func copyHeaders(request *http.Request, headers map[string]string, allow func(string) bool) {
	for name, value := range headers {
		if allow != nil && !allow(name) {
			continue
		}
		request.Header.Set(name, value)
	}
}

func notAuthorization(name string) bool {
	return !strings.EqualFold(name, "Authorization")
}

func overridableOffOfficial(name string) bool {
	switch strings.ToLower(name) {
	case "anthropic-beta", "user-agent", "x-app",
		"anthropic-dispatch-id", "x-claude-code-prompt-id",
		"x-claude-code-request-class", "x-client-request-id":
		return true
	default:
		return strings.HasPrefix(strings.ToLower(name), "x-stainless-")
	}
}

// overridableOnOfficial names the fingerprint headers an operator may pin
// through ExtraHeaders on the official host. Authorization is never on the
// list: the selected credential owns it.
func overridableOnOfficial(name string) bool {
	return overridableOffOfficial(name)
}

func headerValue(headers map[string]string, name string) string {
	for key, value := range headers {
		if strings.EqualFold(key, name) {
			return value
		}
	}
	return ""
}

func billingHeader(firstUser string) string {
	return "x-anthropic-billing-header: cc_version=" + CLIVersion + "." + versionSuffix(firstUser) +
		"; cc_entrypoint=sdk-cli; " + cchPlaceholder + ";\n"
}

func versionSuffix(firstUser string) string {
	material := fingerprintSalt + indexChar(firstUser, 4) + indexChar(firstUser, 7) + indexChar(firstUser, 20) + CLIVersion
	sum := sha256.Sum256([]byte(material))
	return hex.EncodeToString(sum[:])[:3]
}

func indexChar(text string, index int) string {
	n := 0
	for _, char := range text {
		if n == index {
			return string(char)
		}
		n++
	}
	return ""
}

func firstUserText(messages []inference.Message) string {
	for _, message := range messages {
		if message.Role == inference.RoleUser {
			return textOf(message.Content)
		}
	}
	return ""
}

func patchBillingCCH(body []byte) []byte {
	needle := []byte(cchPlaceholder)
	if !bytes.Contains(body, needle) {
		return body
	}
	token := fmt.Sprintf("cch=%05x", xxh64(body, cchSeed)&0xFFFFF)
	return bytes.Replace(body, needle, []byte(token), 1)
}
