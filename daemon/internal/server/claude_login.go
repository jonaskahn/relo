// Claude native login: detecting it and forwarding its credential.
package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/jonaskahn/relo/internal/adapters/wire"
	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/internal/inference"
)

const nativeAnthropicDefault = "https://api.anthropic.com"

type claudeLoginKey struct{}

func claudeLoginModel(body []byte) bool {
	var probe struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(body, &probe); err != nil {
		return false
	}
	model := strings.TrimSpace(probe.Model)
	return catalog.ClaudeAlias(model) || catalog.NativeClaudeID(model)
}

func withClaudeLogin(ctx context.Context) context.Context {
	return context.WithValue(ctx, claudeLoginKey{}, true)
}

func claudeLoginFrom(ctx context.Context) bool {
	value, _ := ctx.Value(claudeLoginKey{}).(bool)
	return value
}

func (s *Server) serveClaudeLogin(w http.ResponseWriter, r *http.Request, surface inferenceSurface) {
	body, ok := s.readClaudeLoginBody(w, r)
	if !ok {
		return
	}
	if !claudeLoginModel(body) {
		s.rejectClaudeLogin(w, r)
		return
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	inbound, found := s.inboundFor(surface)
	if !found {
		s.handleNotImplemented(w, r)
		return
	}
	request, err := s.decodeClaudeLogin(w, r, inbound)
	if err != nil {
		return
	}
	if catalog.ClaudeAlias(request.Model) {
		s.relayClaudeAlias(w, r, surface, inbound, request, body)
		return
	}
	if catalog.NativeClaudeID(request.Model) {
		s.forwardNativeClaude(w, r, body)
		return
	}
	s.rejectClaudeLogin(w, r)
}

func (s *Server) rejectClaudeLogin(w http.ResponseWriter, r *http.Request) {
	s.fail(w, r, refusal{
		Status: http.StatusUnauthorized, Code: "unauthorized", Message: "api.keys.missing_header",
	})
}

func (s *Server) readClaudeLoginBody(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 32<<20+1))
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "read messages request")
		return nil, false
	}
	if len(body) > 32<<20 {
		writeError(w, http.StatusRequestEntityTooLarge, "bad_request", "the request is too large")
		return nil, false
	}
	return body, true
}

func (s *Server) decodeClaudeLogin(w http.ResponseWriter, r *http.Request, inbound wire.InboundCodec) (*inference.Request, error) {
	request, err := inbound.DecodeRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return nil, err
	}
	return request, nil
}

func (s *Server) relayClaudeAlias(w http.ResponseWriter, r *http.Request, surface inferenceSurface, inbound wire.InboundCodec, request *inference.Request, body []byte) {
	r.Header.Del("Authorization")
	r.Header.Del(apiKeyHeaderName)
	r.Body = io.NopCloser(bytes.NewReader(body))
	ctx := r.Context()
	if identity, ok := s.claudeCodeIdentity(ctx); ok {
		ctx = withClientIdentity(ctx, identity)
	}
	r = r.WithContext(ctx)
	captured := captureInbound(s.opts.CaptureRedactor, r)
	s.relay(relaySpec{writer: w, request: r, surface: surface, inbound: inbound, canonical: request, origin: inference.OriginExternal, captured: captured})
}

func (s *Server) claudeCodeIdentity(ctx context.Context) (clientIdentity, bool) {
	if s.opts.Keys == nil {
		return clientIdentity{}, false
	}
	key, found, err := s.opts.Keys.OwnedKey(ctx, "claude-code")
	if err != nil || !found {
		return clientIdentity{}, false
	}
	return clientIdentity{ID: key.ID, Name: key.Name, Kind: key.Kind, Client: key.Client}, true
}

func (s *Server) forwardNativeClaude(w http.ResponseWriter, r *http.Request, body []byte) {
	target := strings.TrimRight(s.opts.NativeAnthropicURL, "/") + r.URL.RequestURI()
	outbound, err := http.NewRequestWithContext(r.Context(), r.Method, target, bytes.NewReader(body))
	if err != nil {
		writeError(w, http.StatusBadGateway, "bad_gateway", "the upstream is unavailable")
		return
	}
	copyEndToEndHeaders(outbound.Header, r.Header)
	response, err := s.nativeClient.Do(outbound)
	if err != nil {
		writeError(w, http.StatusBadGateway, "bad_gateway", "the upstream is unavailable")
		return
	}
	defer func() { _ = response.Body.Close() }()
	copyEndToEndHeaders(w.Header(), response.Header)
	w.WriteHeader(response.StatusCode)
	writeUpstream(w, response.Body)
}

func writeUpstream(w http.ResponseWriter, body io.Reader) {
	flusher, canFlush := w.(http.Flusher)
	buffer := make([]byte, 32<<10)
	for {
		read, err := body.Read(buffer)
		if read > 0 {
			if _, writeErr := w.Write(buffer[:read]); writeErr != nil {
				return
			}
			if canFlush {
				flusher.Flush()
			}
		}
		if err != nil {
			return
		}
	}
}

func copyEndToEndHeaders(dst, src http.Header) {
	for name, values := range src {
		if hopByHop(name) {
			continue
		}
		for _, value := range values {
			dst.Add(name, value)
		}
	}
}

func hopByHop(name string) bool {
	switch strings.ToLower(name) {
	case "connection", "keep-alive", "proxy-authenticate", "proxy-authorization",
		"te", "trailer", "transfer-encoding", "upgrade", "host", "content-length":
		return true
	default:
		return false
	}
}
