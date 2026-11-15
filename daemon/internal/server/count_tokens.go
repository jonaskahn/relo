// Anthropic token counting: the pre-flight a Messages client runs before it
// sends. The surface answers from the catalog's own estimator, so a client
// that asks what its prompt costs reads the same number Relo would charge it.
package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	"github.com/jonaskahn/relo/internal/catalog"
)

const countTokensPath = messagesPath + "/count_tokens"

// handleCountTokens answers the Messages token counter. A Claude-login request
// for a native model is forwarded like its messages sibling, because only
// Anthropic can count the parts Relo never decodes.
func (s *Server) handleCountTokens(w http.ResponseWriter, r *http.Request) {
	if !s.relayReady() {
		s.handleNotImplemented(w, r)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 32<<20+1))
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "read the count request")
		return
	}
	if len(body) > 32<<20 {
		writeError(w, http.StatusRequestEntityTooLarge, "bad_request", "the request is too large")
		return
	}
	if claudeLoginFrom(r.Context()) && catalog.NativeClaudeID(countModel(body)) {
		s.forwardNativeClaude(w, r, body)
		return
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	inbound, found := s.inboundFor(messagesSurface())
	if !found {
		s.handleNotImplemented(w, r)
		return
	}
	request, err := inbound.DecodeRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"input_tokens": s.estimateRequestTokens(request)})
}

func countModel(body []byte) string {
	var probe struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(body, &probe); err != nil {
		return ""
	}
	return probe.Model
}
