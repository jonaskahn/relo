package dispatch_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/upstream"
	"github.com/jonaskahn/relo/internal/adapters/wire"
	"github.com/jonaskahn/relo/internal/adapters/wire/cursor"
	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/internal/inference"
)

type cursorStubCodec struct {
	calls  int
	events []inference.Event
	err    error
	bodies [][]inference.Event
}

func (c *cursorStubCodec) RunBidirectional(_ context.Context, call wire.BidirectionalCall) ([]inference.Event, error) {
	c.calls++
	if call.Emit != nil {
		call.Emit(c.events)
	}
	c.bodies = append(c.bodies, c.events)
	return c.events, c.err
}

func (c *cursorStubCodec) EncodeRequest(_ *inference.Request, _ wire.CodecOpts) (*http.Request, error) {
	return nil, errors.New("bidirectional only")
}

func (c *cursorStubCodec) DecodeResponseEvent(_ wire.SSEEvent) ([]inference.Event, error) {
	return nil, nil
}

func (c *cursorStubCodec) DecodeResponse(_ []byte) ([]inference.Event, error) {
	return c.events, nil
}

func (c *cursorStubCodec) NewStreamDecoder() wire.StreamDecoder { return cursorStubDecoder{} }

func (c *cursorStubCodec) DecodeError(status int, body []byte) *inference.ErrorInfo {
	return &inference.ErrorInfo{Code: http.StatusText(status), Message: string(body), Status: status}
}

type cursorStubDecoder struct{}

func (cursorStubDecoder) Push(_ wire.SSEEvent) ([]inference.Event, error) { return nil, nil }
func (cursorStubDecoder) Finish() ([]inference.Event, error)              { return nil, nil }

func cursorExchange(codec *cursorStubCodec, stream bool) upstream.Exchange {
	exchange := completeExchange("https://cursor.test")
	exchange.ProviderID = "cursor"
	exchange.Codec = codec
	exchange.Request = &inference.Request{
		Model: "sonnet",
		Messages: []inference.Message{
			{Role: inference.RoleUser, Content: []inference.ContentPart{{Type: inference.ContentTypeText, Text: "hi"}}},
		},
		Stream: stream,
	}
	exchange.Options.Cursor = &wire.CursorRequest{
		ConversationID: "conversation-1", ModelID: "sonnet", DetailsID: "sonnet-low",
	}
	return exchange
}

func cursorTextEvents() []inference.Event {
	usage := inference.UsageReport{InputTokens: 10, OutputTokens: 5}
	return []inference.Event{
		{Kind: inference.EventTextDelta, Text: "hello"},
		{Kind: inference.EventUsage, Usage: &usage},
		{Kind: inference.EventTerminal, Terminal: &inference.TerminalInfo{Reason: inference.EventReasonStop}},
	}
}

func TestExecuteBidirectional(t *testing.T) {
	t.Run("a complete turn answers with one body", func(t *testing.T) {
		codec := &cursorStubCodec{events: cursorTextEvents()}
		recorder := httptest.NewRecorder()
		result, err := newExecutor(t).Execute(context.Background(), cursorExchange(codec, false), recorder)
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
		if result.Status != http.StatusOK || !result.Wrote || !result.RelayedBody {
			t.Fatalf("result = %+v, want a written answer", result)
		}
		if result.Usage.InputTokens != 10 || result.Usage.OutputTokens != 5 {
			t.Fatalf("usage = %+v", result.Usage)
		}
		if body := recorder.Body.String(); !strings.Contains(body, "hello") {
			t.Fatalf("body = %q, want the answer", body)
		}
	})

	t.Run("a streamed turn relays frames as they arrive", func(t *testing.T) {
		codec := &cursorStubCodec{events: cursorTextEvents()}
		recorder := httptest.NewRecorder()
		result, err := newExecutor(t).Execute(context.Background(), cursorExchange(codec, true), recorder)
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
		if result.Status != http.StatusOK || !result.Wrote {
			t.Fatalf("result = %+v, want a written stream", result)
		}
		body := recorder.Body.String()
		if !strings.Contains(body, "hello") || !strings.Contains(body, "data: [DONE]") {
			t.Fatalf("body = %q, want the chunks and the sentinel", body)
		}
		if len(codec.bodies) != 1 {
			t.Fatalf("emit batches = %d, want the turn streamed through emit", len(codec.bodies))
		}
	})

	t.Run("a tool-use turn without usage is not an empty completion", func(t *testing.T) {
		codec := &cursorStubCodec{events: []inference.Event{
			{Kind: inference.EventToolCallStart, ToolCall: &inference.ToolCallDelta{Index: 0, ID: "call-1", Name: "read"}},
			{Kind: inference.EventToolCallEnd, ToolCall: &inference.ToolCallDelta{Index: 0, ID: "call-1"}},
			{Kind: inference.EventTerminal, Terminal: &inference.TerminalInfo{Reason: inference.EventReasonToolUse}},
		}}
		recorder := httptest.NewRecorder()
		exchange := cursorExchange(codec, false)
		exchange.Final = false
		result, err := newExecutor(t).Execute(context.Background(), exchange, recorder)
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
		if result.ZeroTokens {
			t.Fatalf("result = %+v, want no empty-completion verdict on a tool turn", result)
		}
	})

	t.Run("a refused turn is retryable before the last attempt", func(t *testing.T) {
		codec := &cursorStubCodec{err: errors.New("cursor ended the stream without finishing the turn")}
		recorder := httptest.NewRecorder()
		exchange := cursorExchange(codec, false)
		exchange.Final = false
		result, err := newExecutor(t).Execute(context.Background(), exchange, recorder)
		if err == nil {
			t.Fatal("Execute() succeeded on a failed turn")
		}
		if !result.Retryable || result.Wrote {
			t.Fatalf("result = %+v, want an unwritten retryable failure", result)
		}
	})

	t.Run("an unauthorized turn refreshes once", func(t *testing.T) {
		codec := &cursorStubCodec{events: cursorTextEvents(), err: cursor.ErrUnauthorized}
		recorder := httptest.NewRecorder()
		exchange := cursorExchange(codec, false)
		refreshed := false
		exchange.OnUnauthorized = func(context.Context) (catalog.Authorization, error) {
			refreshed = true
			codec.err = nil
			return catalog.Authorization{Token: "fresh"}, nil
		}
		result, err := newExecutor(t).Execute(context.Background(), exchange, recorder)
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
		if !refreshed || codec.calls != 2 {
			t.Fatalf("refreshed = %v calls = %d, want one renewal and resend", refreshed, codec.calls)
		}
		if result.Status != http.StatusOK {
			t.Fatalf("result = %+v, want the resent answer", result)
		}
	})
}
