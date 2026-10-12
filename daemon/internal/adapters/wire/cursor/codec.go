// Cursor codec: the agent wire family the relay dispatches through.
package cursor

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/jonaskahn/relo/internal/adapters/wire"
	"github.com/jonaskahn/relo/internal/inference"
)

// Codec speaks the Cursor agent protocol over one duplex stream.
type Codec struct {
	baseURL string
}

var (
	_ wire.CodecModule        = (*Codec)(nil)
	_ wire.BidirectionalCodec = (*Codec)(nil)
)

// NewCodec returns the Cursor codec over the given base URL.
func NewCodec(baseURL string) *Codec {
	return &Codec{baseURL: baseURL}
}

// EncodeRequest refuses the single-shot path: a Cursor turn converses over
// a duplex stream through RunBidirectional instead.
func (c *Codec) EncodeRequest(_ *inference.Request, _ wire.CodecOpts) (*http.Request, error) {
	return nil, fmt.Errorf("cursor needs a bidirectional run: %w", wire.ErrUnsupportedFeature)
}

// DecodeResponseEvent refuses the single-shot path like EncodeRequest does.
func (c *Codec) DecodeResponseEvent(_ wire.SSEEvent) ([]inference.Event, error) {
	return nil, fmt.Errorf("cursor needs a bidirectional run: %w", wire.ErrUnsupportedFeature)
}

// DecodeResponse refuses the single-shot path like EncodeRequest does.
func (c *Codec) DecodeResponse(_ []byte) ([]inference.Event, error) {
	return nil, fmt.Errorf("cursor needs a bidirectional run: %w", wire.ErrUnsupportedFeature)
}

// NewStreamDecoder refuses the single-shot path like EncodeRequest does.
func (c *Codec) NewStreamDecoder() wire.StreamDecoder {
	return unsupportedDecoder{}
}

type unsupportedDecoder struct{}

func (unsupportedDecoder) Push(_ wire.SSEEvent) ([]inference.Event, error) {
	return nil, fmt.Errorf("cursor needs a bidirectional run: %w", wire.ErrUnsupportedFeature)
}

func (unsupportedDecoder) Finish() ([]inference.Event, error) {
	return nil, fmt.Errorf("cursor needs a bidirectional run: %w", wire.ErrUnsupportedFeature)
}

// DecodeError translates a Connect error body into a canonical error.
func (c *Codec) DecodeError(status int, body []byte) *inference.ErrorInfo {
	var envelope struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		message := strings.TrimSpace(string(body))
		if message == "" {
			message = http.StatusText(status)
		}
		return &inference.ErrorInfo{Code: http.StatusText(status), Message: message, Status: status}
	}
	code := envelope.Code
	if code == "" {
		code = http.StatusText(status)
	}
	message := envelope.Message
	if message == "" {
		message = strings.TrimSpace(string(body))
	}
	return &inference.ErrorInfo{Code: code, Message: message, Status: status}
}

// RunBidirectional converses with the Cursor agent service until the turn
// ends, emitting canonical events as they arrive.
func (c *Codec) RunBidirectional(ctx context.Context, call wire.BidirectionalCall) ([]inference.Event, error) {
	if call.Opts.Cursor == nil {
		return nil, fmt.Errorf("cursor selection: %w", wire.ErrInvalidRequest)
	}
	runner := &Runner{
		Client:    http2Client(call.Client),
		BaseURL:   firstNonEmpty(call.Opts.BaseURL, c.baseURL),
		Token:     call.Opts.CredentialRef,
		Request:   call.Request,
		Selection: call.Opts.Cursor,
		Emit:      call.Emit,
		ConnectError: func(status int, body []byte) *inference.ErrorInfo {
			return c.DecodeError(status, body)
		},
	}
	return runner.Run(ctx)
}

// http2Client ensures the duplex turn runs over HTTP/2: a stock HTTP/1.1
// connection writes the request body to EOF before reading the response,
// which would deadlock the conversation. Cloning keeps the caller's proxy
// and timeout behavior.
func http2Client(client *http.Client) *http.Client {
	if client == nil {
		return &http.Client{}
	}
	transport, ok := client.Transport.(*http.Transport)
	if !ok || transport == nil || transport.ForceAttemptHTTP2 {
		return client
	}
	clone := transport.Clone()
	clone.ForceAttemptHTTP2 = true
	duplicated := *client
	duplicated.Transport = clone
	return &duplicated
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
