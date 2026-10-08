// Subscription calls identify themselves as Claude Code on the official API.
package anthropic

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"runtime"
	"strings"

	"github.com/jonaskahn/relo/internal/adapters/wire"
	"github.com/jonaskahn/relo/internal/inference"
)

const (
	// CLIVersion is the Claude Code release a subscription request claims to be.
	CLIVersion = "2.1.294"

	// CLIUserAgent is the User-Agent a subscription request sends when the
	// caller did not already name a claude-cli agent.
	CLIUserAgent = "claude-cli/" + CLIVersion + " (external, cli)"

	// ClaudeCodeBeta is the beta profile a subscription inference or usage
	// call advertises. One profile covers a paired 1M request and a model
	// that is a million tokens on its own.
	ClaudeCodeBeta = "claude-code-20250219,oauth-2025-04-20," +
		"interleaved-thinking-2025-05-14,thinking-token-count-2026-05-13," +
		"context-management-2025-06-27,prompt-caching-scope-2026-01-05," +
		"mid-conversation-system-2026-04-07,per-turn-control-2026-07-01," +
		"mid-conversation-tool-changes-2026-07-01,inline-tools-2026-09-15," +
		"advisor-tool-2026-03-01,effort-2025-11-24," +
		"dangerous-tool-use-2026-09-03,thinking-display-updates-2026-08-18," +
		"extended-cache-ttl-2025-04-11"

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
)

func (c *Codec) authorizeClaudeCode(request *http.Request, opts wire.CodecOpts) {
	official := officialAnthropicHost(requestBase(c, opts))
	copyHeaders(request, opts.ExtraHeaders, notAuthorization)
	applyClaudeCodeHeaders(request, opts)
	if !official {
		copyHeaders(request, opts.ExtraHeaders, overridableOffOfficial)
	}
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
	request.Header.Set(BetaHeader, ClaudeCodeBeta)
	request.Header.Set("x-app", "cli")
	request.Header.Set("User-Agent", claudeUserAgent(opts.ExtraHeaders))
	if session := claudeSession(opts); session != "" {
		request.Header.Set("X-Claude-Code-Session-Id", session)
	}
	if key := headerValue(opts.ExtraHeaders, APIKeyHeader); key != "" {
		request.Header.Set(APIKeyHeader, key)
	}
	applyStainless(request)
}

func claudeUserAgent(headers map[string]string) string {
	agent := headerValue(headers, "User-Agent")
	if strings.HasPrefix(strings.ToLower(agent), "claude-cli") {
		return agent
	}
	return CLIUserAgent
}

func claudeSession(opts wire.CodecOpts) string {
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
	case "anthropic-beta", "user-agent", "x-app":
		return true
	default:
		return strings.HasPrefix(strings.ToLower(name), "x-stainless-")
	}
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
		"; cc_entrypoint=cli; " + cchPlaceholder + ";\n"
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
