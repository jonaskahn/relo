// Package google translates between the canonical format and Google's
// GenerateContent API, across AI Studio, Vertex AI, and Cloud Code Assist.
package google

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jonaskahn/relo/internal/inference"
	"net/http"
	"strings"

	"github.com/jonaskahn/relo/internal/adapters/antigravity"
	"github.com/jonaskahn/relo/internal/adapters/wire"
)

// ErrInvalidGoogleMode reports a mode no endpoint shape matches.
var ErrInvalidGoogleMode = errors.New("unsupported google mode")

// ErrMissingProject reports a Google mode that needs a project identifier.
var ErrMissingProject = errors.New("google mode requires a project")

// Mode names the Google endpoint shape a request uses.
type Mode string

// Gemini wire identity is what Google accepts: the modes, the family name,
// the key header, and the addresses each mode answers on.
const (
	// ModeAIStudio speaks generativelanguage.googleapis.com with an API key.
	ModeAIStudio Mode = "ai-studio"

	// ModeVertex speaks aiplatform.googleapis.com for one project.
	ModeVertex Mode = "vertex"

	// ModeCloudCodeAssist speaks the internal endpoint Code Assist clients
	// use, with the project and model around the request.
	ModeCloudCodeAssist Mode = "cloud-code-assist"

	// ID names the wire family in configuration and logs.
	ID = "google-gemini"

	// APIKeyHeader is where a Google API key travels. The key never appears
	// in a URL, so it cannot leak through a log line or a proxy cache.
	APIKeyHeader = "x-goog-api-key"

	// DefaultAIStudioURL and DefaultVertexURL carry the version segment, so
	// a codec only appends the resource a request names.
	DefaultAIStudioURL = "https://generativelanguage.googleapis.com/v1beta"
	DefaultVertexURL   = "https://aiplatform.googleapis.com/v1"

	aiStudioHost        = DefaultAIStudioURL
	cloudCodeAssistHost = "https://daily-cloudcode-pa.googleapis.com"

	// cloudCodeUserAgent and cloudCodeRequestType are the protocol constants
	// the first-party Antigravity client puts in the Cloud Code Assist
	// envelope, beside the versioned User-Agent header.
	cloudCodeUserAgent   = "antigravity"
	cloudCodeRequestType = "agent"
)

// Config selects the endpoint a codec talks to and the project it bills.
type Config struct {
	Mode    Mode
	Project string
	BaseURL string
}

// Codec speaks GenerateContent. One codec serves every request; each
// response gets its own stream decoder. replay is set only on the copy
// Bind returns for one Cloud Code Assist attempt.
type Codec struct {
	cfg    Config
	replay replayScope
}

var _ wire.CodecModule = (*Codec)(nil)

// Module returns the AI Studio codec the composition root registers.
func Module() wire.CodecModule {
	return NewCodec(Config{Mode: ModeAIStudio})
}

// NewCodec returns a codec for the given endpoint configuration. A request
// may override the mode, project, location, and base URL through CodecOpts.
func NewCodec(cfg Config) *Codec {
	return &Codec{cfg: cfg}
}

// EncodeRequest builds the upstream GenerateContent request, sanitizing
// every tool schema on the way out.
func (c *Codec) EncodeRequest(req *inference.Request, opts wire.CodecOpts) (*http.Request, error) {
	if opts.CredentialRef == "" && opts.AuthMethod != wire.AuthNone {
		return nil, wire.ErrMissingCredential
	}
	mode, err := c.mode(opts)
	if err != nil {
		return nil, err
	}
	settings := c.settings(opts)
	if err := settings.check(mode); err != nil {
		return nil, err
	}
	payload, err := settings.body(req, mode, opts)
	if err != nil {
		return nil, err
	}
	endpoint, err := settings.endpoint(req, mode, opts)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("build generate content request: %w", err)
	}
	c.prepareGoogleRequest(request, mode, opts)
	return request, nil
}

func (c *Codec) prepareGoogleRequest(request *http.Request, mode Mode, opts wire.CodecOpts) {
	request.Header.Set("Content-Type", "application/json")
	c.authorize(request, opts)
	for name, value := range opts.ExtraHeaders {
		request.Header.Set(name, value)
	}
	if mode == ModeCloudCodeAssist {
		// The connection template still carries the IDE fingerprint used for
		// listing. Chat replaces it with the CLI fingerprint, and leaves the
		// body length unset so the transport writes a chunked body.
		request.Header.Set("User-Agent", antigravity.CLIUserAgent())
		request.Header.Set("Accept-Encoding", "gzip")
		request.ContentLength = -1
	}
}

// Bind returns a codec that files this attempt's answer under its session,
// so the next turn can replay a thought signature the client did not send
// back. Models that do not use that cache keep the shared codec.
func (c *Codec) Bind(req *inference.Request, opts wire.CodecOpts) wire.CodecModule {
	mode, err := c.mode(opts)
	if err != nil || mode != ModeCloudCodeAssist || req == nil || !geminiWire(req.Model) {
		return c
	}
	bound := *c
	bound.cfg.Mode = ModeCloudCodeAssist
	bound.replay = replayScope{
		model:   req.Model,
		session: sessionID(sessionPreimage(opts.SessionAnchor, req)),
		record:  true,
	}
	return &bound
}

func (c *Codec) authorize(request *http.Request, opts wire.CodecOpts) {
	switch {
	case opts.CredentialRef == "":
	case opts.AuthMethod == wire.AuthOAuth:
		request.Header.Set("Authorization", "Bearer "+opts.CredentialRef)
	default:
		request.Header.Set(APIKeyHeader, opts.CredentialRef)
	}
}

// DecodeResponseEvent translates one frame in isolation.
func (c *Codec) DecodeResponseEvent(event wire.SSEEvent) ([]inference.Event, error) {
	return c.NewStreamDecoder().Push(event)
}

// DecodeResponse translates a completed GenerateContent response.
func (c *Codec) DecodeResponse(body []byte) ([]inference.Event, error) {
	var chunk streamChunk
	if err := json.Unmarshal(body, &chunk); err != nil {
		return nil, fmt.Errorf("%w: %v", wire.ErrInvalidResponse, err)
	}
	chunk = unwrapCloudCode(c.cfg.Mode, string(body), chunk)
	decoder := c.NewStreamDecoder().(*streamDecoder)
	events := decoder.absorb(&chunk)
	closing, err := decoder.Finish()
	if err != nil {
		return nil, err
	}
	return append(events, closing...), nil
}

// DecodeError translates an upstream error body into a canonical error.
func (c *Codec) DecodeError(status int, body []byte) *inference.ErrorInfo {
	var envelope errorEnvelope
	if err := json.Unmarshal(body, &envelope); err == nil && envelope.Error != nil {
		return envelope.Error.info(status)
	}
	// A Cloud Code Assist refusal arrives inside the same wrapper its answers
	// use, so the wrapped error is read when none sits on top.
	if c.cfg.Mode == ModeCloudCodeAssist {
		var wrapped ccaEnvelope
		if err := json.Unmarshal(body, &wrapped); err == nil && wrapped.Response != nil && wrapped.Response.Error != nil {
			return wrapped.Response.Error.info(status)
		}
	}
	return &inference.ErrorInfo{
		Code:    http.StatusText(status),
		Message: strings.TrimSpace(string(body)),
		Status:  status,
	}
}

type settings struct {
	project       string
	baseURL       string
	requestID     string
	sessionAnchor string
}

func (c *Codec) settings(opts wire.CodecOpts) settings {
	return settings{
		project:       firstOf(opts.Project, c.cfg.Project),
		baseURL:       firstOf(opts.BaseURL, c.cfg.BaseURL),
		requestID:     opts.RequestID,
		sessionAnchor: opts.SessionAnchor,
	}
}

func firstOf(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

func (c *Codec) mode(opts wire.CodecOpts) (Mode, error) {
	mode := c.cfg.Mode
	if opts.Mode != "" {
		mode = Mode(opts.Mode)
	}
	switch mode {
	case "":
		return ModeAIStudio, nil
	case ModeAIStudio, ModeVertex, ModeCloudCodeAssist:
		return mode, nil
	default:
		return "", fmt.Errorf("%w: %q", ErrInvalidGoogleMode, mode)
	}
}

func (s settings) check(mode Mode) error {
	switch {
	case mode == ModeCloudCodeAssist && s.project == "":
		return ErrMissingProject
	default:
		return nil
	}
}

func (s settings) body(req *inference.Request, mode Mode, opts wire.CodecOpts) ([]byte, error) {
	assist := mode == ModeCloudCodeAssist
	payload, err := newPayload(req, assist, s.sessionAnchor, opts)
	if err != nil {
		return nil, err
	}
	if !assist {
		return json.Marshal(payload)
	}
	trajectory := finishAssist(payload, req, s.sessionAnchor, s.requestID)
	return json.Marshal(envelope{
		Project: s.project, RequestID: assistRequestID(trajectory, userSteps(payload.Contents)),
		Request: *payload, Model: req.Model,
		UserAgent: cloudCodeUserAgent, RequestType: cloudCodeRequestType,
	})
}

func (s settings) endpoint(req *inference.Request, mode Mode, opts wire.CodecOpts) (string, error) {
	action := methodName(req.Stream)
	switch mode {
	case ModeVertex:
		return s.base(mode) + "/publishers/google/models/" + req.Model + ":" + action + query(req.Stream), nil
	case ModeCloudCodeAssist:
		return s.base(mode) + "/v1internal:" + action + query(req.Stream), nil
	default:
		return s.base(mode) + "/models/" + req.Model + ":" + action + query(req.Stream), nil
	}
}

func (s settings) base(mode Mode) string {
	if s.baseURL != "" {
		return strings.TrimSuffix(s.baseURL, "/")
	}
	switch mode {
	case ModeVertex:
		return DefaultVertexURL
	case ModeCloudCodeAssist:
		return cloudCodeAssistHost
	default:
		return aiStudioHost
	}
}

func methodName(stream bool) string {
	if stream {
		return "streamGenerateContent"
	}
	return "generateContent"
}

func query(stream bool) string {
	if !stream {
		return ""
	}
	return "?alt=sse"
}
