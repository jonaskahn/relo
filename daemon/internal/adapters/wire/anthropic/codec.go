// Package anthropic translates between the canonical format and the
// Anthropic Messages API that Claude Code speaks, in both directions.
package anthropic

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/jonaskahn/relo/internal/adapters/wire"
	"github.com/jonaskahn/relo/internal/inference"
)

// Anthropic wire identity is what its API accepts: the family name, the
// base address, the version header, and the beta the relay speaks.
const (
	// ID names the wire family in configuration and logs.
	ID = "anthropic-messages"

	DefaultBaseURL = "https://api.anthropic.com/v1"
	messagesPath   = "/messages"

	// VersionHeader carries APIVersion on every request.
	VersionHeader = "anthropic-version"
	APIVersion    = "2023-06-01"

	// BetaHeader carries a beta profile. OAuthBetaValue is the short pair a
	// model listing sends. Inference and usage send ClaudeCodeBeta.
	BetaHeader     = "anthropic-beta"
	OAuthBetaValue = "claude-code-20250219,oauth-2025-04-20"

	// APIKeyHeader is where a plain API key travels, which a sign-in token
	// never uses.
	APIKeyHeader = "x-api-key"

	customToolPrefix = "custom_"
)

// Codec speaks the Anthropic Messages API. One codec serves every request;
// each response gets its own stream decoder.
type Codec struct {
	baseURL string
	vertex  bool
}

var _ wire.CodecModule = (*Codec)(nil)

// Module returns the Messages codec the composition root registers.
func Module() wire.CodecModule {
	return NewCodec(DefaultBaseURL)
}

// NewCodec returns a codec whose endpoint can be overridden per request
// through CodecOpts.
func NewCodec(baseURL string) *Codec {
	return &Codec{baseURL: baseURL, vertex: false}
}

// NewVertexCodec returns a codec configured for Vertex Anthropic endpoint.
func NewVertexCodec(baseURL string) *Codec {
	return &Codec{baseURL: baseURL, vertex: true}
}

// EncodeRequest builds the upstream Messages request. The system prompt
// becomes a top-level field, tool results batch into one user turn, and
// OAuth credentials add the beta headers and tool namespace Claude Code
// expects.
func (c *Codec) EncodeRequest(req *inference.Request, opts wire.CodecOpts) (*http.Request, error) {
	if opts.CredentialRef == "" && opts.AuthMethod != wire.AuthNone {
		return nil, wire.ErrMissingCredential
	}
	oauth := opts.AuthMethod == wire.AuthOAuth
	var payload []byte
	var err error
	if c.vertex {
		payload, err = json.Marshal(newVertexPayload(req, oauth, opts))
	} else {
		payload, err = json.Marshal(newPayload(req, oauth, opts))
	}
	if err != nil {
		return nil, fmt.Errorf("encode messages request: %w", err)
	}
	if oauth && !c.vertex {
		payload = patchBillingCCH(payload)
	}
	request, err := http.NewRequest(http.MethodPost, c.endpoint(opts, req, oauth), bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("build messages request: %w", err)
	}
	c.authorize(request, opts, oauth)
	return request, nil
}

func (c *Codec) authorize(request *http.Request, opts wire.CodecOpts, oauth bool) {
	request.Header.Set("Content-Type", "application/json")
	if c.vertex {
		if opts.CredentialRef != "" {
			request.Header.Set("Authorization", "Bearer "+opts.CredentialRef)
		}
		for name, value := range opts.ExtraHeaders {
			request.Header.Set(name, value)
		}
		return
	}
	request.Header.Set(VersionHeader, APIVersion)
	if oauth {
		c.authorizeClaudeCode(request, opts)
		return
	}
	switch {
	case opts.KeyHeader != "":
		wire.ApplyCredential(request, opts)
	case opts.CredentialRef != "":
		request.Header.Set(APIKeyHeader, opts.CredentialRef)
	}
	for name, value := range opts.ExtraHeaders {
		request.Header.Set(name, value)
	}
}

// DecodeResponseEvent translates one frame in isolation. A whole stream
// needs NewStreamDecoder, because content blocks span several frames.
func (c *Codec) DecodeResponseEvent(event wire.SSEEvent) ([]inference.Event, error) {
	return c.NewStreamDecoder().Push(event)
}

// DecodeResponse translates a completed message object into canonical events.
func (c *Codec) DecodeResponse(body []byte) ([]inference.Event, error) {
	var message messageBody
	if err := json.Unmarshal(body, &message); err != nil {
		return nil, fmt.Errorf("%w: %v", wire.ErrInvalidResponse, err)
	}
	if message.Error != nil {
		events := []inference.Event{{Kind: inference.EventError, Error: message.Error.info()}}
		return append(events, terminalEvent(inference.EventReasonError)), nil
	}
	decoder := c.NewStreamDecoder().(*streamDecoder)
	events := decoder.absorbMessage(&message)
	closing, err := decoder.Finish()
	if err != nil {
		return nil, err
	}
	return append(events, closing...), nil
}

// DecodeError translates an upstream error body into a canonical error.
func (c *Codec) DecodeError(status int, body []byte) *inference.ErrorInfo {
	var envelope messageBody
	if err := json.Unmarshal(body, &envelope); err != nil || envelope.Error == nil {
		return &inference.ErrorInfo{
			Code:    http.StatusText(status),
			Message: strings.TrimSpace(string(body)),
			Status:  status,
		}
	}
	info := envelope.Error.info()
	info.Status = status
	return info
}

func (c *Codec) endpoint(opts wire.CodecOpts, req *inference.Request, oauth bool) string {
	base := strings.TrimSuffix(requestBase(c, opts), "/")
	if c.vertex {
		action := "rawPredict"
		if req != nil && req.Stream {
			action = "streamRawPredict"
		}
		return base + "/publishers/anthropic/models/" + req.Model + ":" + action
	}
	target := base + messagesPath
	if oauth && officialAnthropicHost(base) {
		return withBeta(target)
	}
	return target
}
