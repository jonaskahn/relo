// ChatGPT subscription requests carry the identity expected by the Codex
// backend without changing ordinary OpenAI Responses API calls.
package openairesponses

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/url"
	"strings"

	"github.com/jonaskahn/relo/internal/adapters/codex"
	"github.com/jonaskahn/relo/internal/adapters/wire"
	"github.com/jonaskahn/relo/internal/inference"
)

const codexHost = "chatgpt.com"

func isCodexBackend(target string) bool {
	parsed, err := url.Parse(target)
	return err == nil && strings.EqualFold(parsed.Hostname(), codexHost)
}

func applyCodexHeaders(request *http.Request, req *inference.Request, opts wire.CodecOpts) {
	residency := request.Header.Get("x-openai-internal-codex-residency")
	request.Header.Del("x-api-key")
	request.Header.Del("x-codex-installation-id")
	request.Header.Del("OpenAI-Beta")

	request.Header.Set("Authorization", "Bearer "+opts.CredentialRef)
	request.Header.Set("originator", codex.Originator)
	request.Header.Set("version", codex.Version)
	request.Header.Set("User-Agent", codex.UserAgent)
	request.Header.Set("OpenAI-Beta", codex.OpenAIBeta)
	request.Header.Set("Accept", "text/event-stream")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("x-codex-routing-hint", "model="+req.Model)

	if session := codexSession(opts); session != "" {
		for _, name := range []string{"session_id", "conversation_id", "x-client-request-id", "session-id"} {
			request.Header.Set(name, session)
		}
	}
	if residency == "" {
		residency = codex.ResidencyFromToken(opts.CredentialRef)
	}
	if residency != "" {
		request.Header.Set("x-openai-internal-codex-residency", residency)
	}
}

func codexSession(opts wire.CodecOpts) string {
	session := opts.SessionAnchor
	if session == "" {
		session = opts.RequestID
	}
	if strings.IndexFunc(session, func(char rune) bool { return char < 0x20 || char == 0x7f }) < 0 {
		return session
	}
	sum := sha256.Sum256([]byte(session))
	return hex.EncodeToString(sum[:16])
}
