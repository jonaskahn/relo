// Package wire holds the wire-protocol vocabulary both sides translate
// through: the codec interfaces and the shared inbound shapes.
package wire

import (
	"context"
	"errors"
	"net/http"

	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/internal/inference"
)

// ErrInvalidRequest reports a client request the surface cannot decode.
var ErrInvalidRequest = errors.New("invalid client request")

// ErrUnknownEvent reports a canonical event kind a surface cannot render.
var ErrUnknownEvent = errors.New("unknown canonical event kind")

// ErrMalformedEvent reports an upstream frame the family decoder cannot parse.
var ErrMalformedEvent = errors.New("malformed upstream SSE event")

// ErrInvalidResponse reports an upstream body the family decoder cannot parse.
var ErrInvalidResponse = errors.New("malformed upstream response body")

// ErrMissingCredential reports a request that reached a codec without the
// credential the codec applies to the upstream request.
var ErrMissingCredential = errors.New("request requires a credential and none was resolved")

// ErrUnsupportedFeature reports a request a wire family cannot express, so a
// field is never silently dropped.
var ErrUnsupportedFeature = errors.New("wire family cannot represent this feature")

// Auth names how one attempt authenticates its upstream request. A codec
// reads the name through CodecOpts, so a keyless provider is a supported
// configuration rather than a missing credential.
const (
	AuthOAuth  = "oauth"
	AuthAPIKey = "api_key"
	AuthAWS    = "aws"
	AuthGCP    = "gcp"
	AuthNone   = "none"
)

// SSEEvent is one server-sent event.
type SSEEvent struct {
	Name string
	Data string
	ID   string
}

// CodecOpts carries the per-request settings a codec needs. It never
// carries routing decisions, only protocol configuration.
type CodecOpts struct {
	BaseURL       string
	CredentialRef string
	AuthMethod    string
	KeyHeader     string
	Mode          string
	Project       string
	// AccountID is the subscription account the selected credential belongs
	// to. A codec that attributes usage to an account sends it; empty means
	// the attempt carries no account attribution.
	AccountID string
	// DeviceID is the installation's stable device id for AccountID. A codec
	// that attributes usage to a device sends it beside the account.
	DeviceID string
	// SessionID is the resolved upstream conversation id for this attempt.
	// It stays stable across retries of one conversation; empty leaves the
	// dialect's own fallback in place.
	SessionID string
	// NativeBody is the original client body a same-family surface decoded.
	// A codec that speaks the client's own wire reuses its shape for an
	// attempt the route keeps on the requested model. Empty means the codec
	// always builds the body from the canonical request.
	NativeBody []byte
	// RequestID names the request in Relo's own log, which a dialect that
	// carries a correlation identifier of its own puts on the wire.
	RequestID string
	// SessionAnchor is the conversation identity the inbound client named.
	// A dialect that keeps per-conversation state hashes it into its own
	// session id. Empty means the dialect falls back to the first user text.
	SessionAnchor string
	ExtraHeaders  map[string]string
	Signer        func(req *http.Request) error
	// RefusesMaxOutputTokens reports an upstream dialect without a
	// max-output ceiling parameter, which the codec leaves out of the body.
	RefusesMaxOutputTokens bool
	// MaxOutput is the routed model's output ceiling from the catalog. A
	// codec that injects a default limit never sends one above it.
	MaxOutput *int64
	// ReasoningEfforts are the effort values the routed model accepts, read
	// from the catalog. Nil means no layer stated a ladder, which leaves a
	// codec's own pass-through rule alone; an empty slice means the model
	// stated no effort list.
	ReasoningEfforts []string
	// ReasoningToggle reports that the model can turn thinking on or off
	// without an effort name.
	ReasoningToggle bool
	// ReasoningBudget reports that the model takes a token budget. The
	// bounds are nil when the model stated a budget without a limit.
	ReasoningBudget    bool
	ReasoningBudgetMin *int64
	ReasoningBudgetMax *int64
	// TemplateID is the connection template. A chat codec uses it when that
	// template's API names thinking with its own fields.
	TemplateID string
	// ToolCallDialect names a serving template that renders tool calls as text
	// rather than structured deltas, which a chat decoder uses to recover the
	// call from that text. Empty leaves the decoder's content-shape detection.
	ToolCallDialect string
	// LongContext reports that the caller asked for the million-token entry,
	// which the Claude subscription profile marks with its own beta.
	LongContext bool
	// NativeMillion reports that the named entry is a million tokens on its own,
	// so the long-context beta does not apply and must be left off the request.
	NativeMillion bool
	// UpstreamModel is the identifier sent to the provider, which gates
	// version-dependent wire details such as the long-context beta. Empty
	// keeps the legacy behavior.
	UpstreamModel string
	// PromptTokens estimates the request size the codec will encode, which a
	// codec uses for the capacity a target promises rather than for the beta
	// a caller explicitly named.
	PromptTokens int64
	// Cursor carries the exact Cursor request tuple one attempt sends. Nil
	// on every other format.
	Cursor *CursorRequest
	// Antigravity carries the exact Cloud Code SKU one attempt sends. Nil
	// on every other format.
	Antigravity *AntigravityRequest
}

// AntigravityRequest is the exact Cloud Code request tuple one attempt sends:
// the wire SKU the account's discovery published for the requested band.
type AntigravityRequest struct {
	WireModel      string
	ModelEnum      string
	MaxOutput      int
	ThinkingBudget *int
}

// CursorParam is one Cursor model parameter pair.
type CursorParam struct {
	ID    string
	Value string
}

// CursorRequest is the exact Cursor request tuple one attempt sends: the
// model triple the account's discovery published for the requested group
// and effort.
type CursorRequest struct {
	ConversationID string
	ModelID        string
	DetailsID      string
	MaxMode        bool
	Params         []CursorParam
}

// ApplyCredential writes the credential onto the request according to KeyHeader.
func ApplyCredential(req *http.Request, opts CodecOpts) {
	if opts.CredentialRef == "" || opts.AuthMethod == AuthNone || opts.KeyHeader == "none" {
		return
	}
	switch opts.KeyHeader {
	case "x-api-key":
		req.Header.Set("x-api-key", opts.CredentialRef)
	case "x-goog-api-key":
		req.Header.Set("x-goog-api-key", opts.CredentialRef)
	case "api-key":
		req.Header.Set("api-key", opts.CredentialRef)
	case "bearer", "":
		req.Header.Set("Authorization", "Bearer "+opts.CredentialRef)
	default:
		req.Header.Set(opts.KeyHeader, opts.CredentialRef)
	}
}

// Codec translates canonical requests into a provider's wire format and
// that provider's responses back into canonical events.
type Codec interface {
	EncodeRequest(req *inference.Request, opts CodecOpts) (*http.Request, error)
	DecodeResponseEvent(event SSEEvent) ([]inference.Event, error)
	DecodeResponse(body []byte) ([]inference.Event, error)
}

// StreamDecoder translates a provider's SSE stream into canonical events,
// keeping the state one response needs: tool call accumulation, usage
// assembly, and terminal repair.
type StreamDecoder interface {
	Push(event SSEEvent) ([]inference.Event, error)
	Finish() ([]inference.Event, error)
}

// StreamCodec is a Codec whose streaming translation is stateful, so each
// upstream response gets its own decoder.
type StreamCodec interface {
	Codec
	NewStreamDecoder() StreamDecoder
}

// ErrorDecoder translates an upstream error body into a canonical error.
type ErrorDecoder interface {
	DecodeError(status int, body []byte) *inference.ErrorInfo
}

// ErrorEncoder renders a relay failure as frames the client can parse.
type ErrorEncoder interface {
	EncodeError(err error) []SSEEvent
}

// FormatSource resolves the codec one upstream format speaks. It is declared
// here, where the codec contract lives, so a consumer depends on the contract
// rather than on the family packages.
type FormatSource interface {
	Outbound(format catalog.APIFormat) (CodecModule, bool)
	Streams(format catalog.APIFormat) bool
}

// CodecModule is one wire family: request encoding, per-response stream
// decoding, and upstream failure translation.
type CodecModule interface {
	Codec
	StreamCodec
	ErrorDecoder
}

// RequestBinder scopes a codec to one attempt. A family that remembers an
// upstream answer uses the bound codec so the memory is filed under the
// conversation that produced it.
type RequestBinder interface {
	Bind(req *inference.Request, opts CodecOpts) CodecModule
}

// BidirectionalCall carries one duplex attempt: what to send, how to send
// it, and where to emit events as they arrive. Record keeps the framed
// bytes the codec wrote, so the traffic log can show what the upstream
// parsed; it is nil when the caller keeps nothing.
type BidirectionalCall struct {
	Client  *http.Client
	Request *inference.Request
	Opts    CodecOpts
	Emit    func([]inference.Event)
	Record  func(frame []byte)
	// Handoff reports that the codec handed a native execution to the
	// calling client. The upstream conversation keeps a pending native call,
	// so the next request belongs on a fresh upstream conversation.
	Handoff func()
}

// BidirectionalCodec is a CodecModule that converses over one duplex
// stream: it answers the upstream's own requests while reading its
// responses, instead of sending one request and reading one response.
type BidirectionalCodec interface {
	CodecModule
	// RunBidirectional converses until the turn ends, emitting canonical
	// events as they arrive and returning them all once the turn ends.
	RunBidirectional(ctx context.Context, call BidirectionalCall) ([]inference.Event, error)
}

// InboundCodec translates one client-facing surface into canonical form
// and back. One instance serves one request.
type InboundCodec interface {
	DecodeRequest(r *http.Request) (*inference.Request, error)
	EncodeResponseEvent(event inference.Event) ([]SSEEvent, error)
	EncodeResponse(events []inference.Event) ([]byte, error)
}

// NativeBodyCarrier is an inbound codec that kept the client body it decoded,
// so an attempt on the client's own wire family reuses the shape the client
// built instead of a rebuild that drops its native controls.
type NativeBodyCarrier interface {
	NativeBody() []byte
}
