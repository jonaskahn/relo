// Package formats is the one registry of upstream wire formats. It is built
// explicitly by the composition root, so a build cannot advertise a format it
// has no codec for.
package formats

import (
	"net/http"

	"github.com/jonaskahn/relo/internal/adapters/wire"
	"github.com/jonaskahn/relo/internal/adapters/wire/anthropic"
	"github.com/jonaskahn/relo/internal/adapters/wire/bedrock"
	"github.com/jonaskahn/relo/internal/adapters/wire/google"
	"github.com/jonaskahn/relo/internal/adapters/wire/kiro"
	"github.com/jonaskahn/relo/internal/adapters/wire/openaichat"
	"github.com/jonaskahn/relo/internal/adapters/wire/openairesponses"
	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/internal/inference"
)

// Registry resolves the codec of every format Relo implements.
type Registry struct{}

// New returns the registry of every format this build speaks.
func New() *Registry {
	return &Registry{}
}

var _ wire.FormatSource = (*Registry)(nil)

// Outbound returns the codec one format speaks, and whether Relo implements
// that format at all.
func (r *Registry) Outbound(format catalog.APIFormat) (wire.CodecModule, bool) {
	switch format {
	case catalog.FormatOpenAIChat:
		return openaichat.NewCodec(""), true
	case catalog.FormatOpenAIResp:
		return openairesponses.NewCodec(""), true
	case catalog.FormatAnthropic:
		return anthropic.NewCodec(""), true
	case catalog.FormatVertexAnthropic:
		return anthropic.NewVertexCodec(""), true
	case catalog.FormatGemini:
		return google.NewCodec(google.Config{Mode: google.ModeAIStudio}), true
	case catalog.FormatVertex:
		return google.NewCodec(google.Config{Mode: google.ModeVertex}), true
	case catalog.FormatCloudCode:
		return google.NewCodec(google.Config{Mode: google.ModeCloudCodeAssist}), true
	case catalog.FormatBedrockConverse:
		return bedrock.NewCodec(""), true
	case catalog.FormatKiro:
		return kiro.NewCodec(""), true
	default:
		return nil, false
	}
}

// Supports reports whether Relo implements a format at all.
func (r *Registry) Supports(format catalog.APIFormat) bool {
	_, supported := r.Outbound(format)
	return supported
}

// Streams reports whether a format answers with a stream of events rather
// than one complete body.
func (r *Registry) Streams(format catalog.APIFormat) bool {
	switch format {
	case catalog.FormatUnsupported:
		return false
	default:
		return r.Supports(format)
	}
}

// GoogleMode returns the endpoint shape a Google format speaks, so the Google
// codec never guesses from a connection identifier.
func (r *Registry) GoogleMode(format catalog.APIFormat) google.Mode {
	switch format {
	case catalog.FormatVertex:
		return google.ModeVertex
	case catalog.FormatCloudCode:
		return google.ModeCloudCodeAssist
	default:
		return google.ModeAIStudio
	}
}

// ModeFor names the endpoint shape one format speaks, which is what a caller
// outside the codecs records on a request.
func (r *Registry) ModeFor(format catalog.APIFormat) string {
	return string(r.GoogleMode(format))
}

// Inbound returns the codec one client surface speaks, and whether Relo
// implements that surface at all.
func (r *Registry) Inbound(surface string) (wire.InboundCodec, bool) {
	switch surface {
	case inference.SurfaceChatCompletions:
		return openaichat.NewChatCompletionsCodec(), true
	case inference.SurfaceResponses:
		return openairesponses.NewInbound(), true
	case inference.SurfaceMessages:
		return anthropic.NewInbound(), true
	default:
		return nil, false
	}
}

// FailureDocument renders a refusal a client of the Messages surface reads.
// It lives here so the transport never imports a codec family directly.
func (r *Registry) FailureDocument(status int, message string) []byte {
	return anthropic.FailureDocument(status, message)
}

// IsAnthropicRequest reports whether a request carries the header an
// Anthropic client sends, which is how the model list chooses its shape.
func (r *Registry) IsAnthropicRequest(header http.Header) bool {
	return header.Get(anthropic.VersionHeader) != ""
}
