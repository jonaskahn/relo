// Package openai_responses translates between the canonical format and the
// OpenAI Responses API that Codex speaks, in both directions.
package openairesponses

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/jonaskahn/relo/internal/inference"
	"net/http"
	"strings"

	"github.com/jonaskahn/relo/internal/adapters/wire"
)

// Responses wire identity is what OpenAI accepts: the family name and the
// base address.
const (
	// ID names the wire family in configuration and logs.
	ID = "openai-responses"

	DefaultBaseURL = "https://api.openai.com/v1"
	responsesPath  = "/responses"
)

// Codec speaks the OpenAI Responses API. One codec serves every request;
// each response gets its own stream decoder.
type Codec struct {
	baseURL string
}

var _ wire.CodecModule = (*Codec)(nil)

// Module returns the Responses codec the composition root registers.
func Module() wire.CodecModule {
	return NewCodec(DefaultBaseURL)
}

// NewCodec returns a codec whose endpoint can be overridden per request
// through CodecOpts.
func NewCodec(baseURL string) *Codec {
	return &Codec{baseURL: baseURL}
}

// EncodeRequest builds the upstream Responses request. It maps messages to
// input items, tools to function definitions, and reasoning to parameters,
// and always asks the upstream not to store the response.
func (c *Codec) EncodeRequest(req *inference.Request, opts wire.CodecOpts) (*http.Request, error) {
	if opts.CredentialRef == "" && opts.AuthMethod != wire.AuthNone {
		return nil, wire.ErrMissingCredential
	}
	payload := newPayload(req, opts)
	// The ChatGPT Codex backend has no max-output ceiling parameter, so the
	// choice is left out rather than sent and refused.
	if opts.RefusesMaxOutputTokens {
		payload.MaxOutputTokens = 0
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode responses request: %w", err)
	}
	target := c.endpoint(opts)
	request, err := http.NewRequest(http.MethodPost, target, bytes.NewReader(encoded))
	if err != nil {
		return nil, fmt.Errorf("build responses request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	wire.ApplyCredential(request, opts)
	for name, value := range opts.ExtraHeaders {
		request.Header.Set(name, value)
	}
	if isCodexBackend(target) {
		applyCodexHeaders(request, req, opts)
	}
	return request, nil
}

// DecodeResponseEvent translates one frame in isolation. A whole stream
// needs NewStreamDecoder, because items span several frames.
func (c *Codec) DecodeResponseEvent(event wire.SSEEvent) ([]inference.Event, error) {
	return c.NewStreamDecoder().Push(event)
}

// DecodeResponse translates a completed response object into canonical events.
func (c *Codec) DecodeResponse(body []byte) ([]inference.Event, error) {
	var response responseBody
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("%w: %v", wire.ErrInvalidResponse, err)
	}
	decoder := c.NewStreamDecoder().(*streamDecoder)
	events := decoder.absorbResponse(&response)
	closing, err := decoder.Finish()
	if err != nil {
		return nil, err
	}
	return append(events, closing...), nil
}

// DecodeError translates an upstream error body into a canonical error.
func (c *Codec) DecodeError(status int, body []byte) *inference.ErrorInfo {
	var envelope responseBody
	if err := json.Unmarshal(body, &envelope); err != nil || envelope.Error == nil {
		return &inference.ErrorInfo{
			Code:    http.StatusText(status),
			Message: strings.TrimSpace(string(body)),
			Status:  status,
		}
	}
	return envelope.Error.info(status)
}

func (c *Codec) endpoint(opts wire.CodecOpts) string {
	base := c.baseURL
	if opts.BaseURL != "" {
		base = opts.BaseURL
	}
	return strings.TrimSuffix(base, "/") + responsesPath
}
