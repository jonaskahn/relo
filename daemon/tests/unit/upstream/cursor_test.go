package dispatch_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	cursorproto "github.com/jonaskahn/relo/internal/adapters/cursor"
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
	if call.Record != nil {
		call.Record([]byte("framed-run-request"))
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

	t.Run("a named refusal classifies as itself instead of retrying", func(t *testing.T) {
		refusal := cursorproto.EndStream{Name: "BAD_MODEL_NAME", Message: "no such model", Status: http.StatusNotFound}
		codec := &cursorStubCodec{err: refusal}
		recorder := httptest.NewRecorder()
		exchange := cursorExchange(codec, false)
		exchange.Final = false
		result, err := newExecutor(t).Execute(context.Background(), exchange, recorder)
		if err == nil {
			t.Fatal("Execute() succeeded on a refused turn")
		}
		if result.Retryable {
			t.Fatalf("result = %+v, want a refusal the caller has to fix", result)
		}
		if result.Status != http.StatusNotFound {
			t.Fatalf("status = %d, want the refusal's own status", result.Status)
		}
	})

	t.Run("a refusal the service calls transient stays retryable", func(t *testing.T) {
		refusal := cursorproto.EndStream{
			Name: "INTERNAL", Message: "try again", Status: http.StatusServiceUnavailable,
		}
		codec := &cursorStubCodec{err: refusal}
		recorder := httptest.NewRecorder()
		exchange := cursorExchange(codec, false)
		exchange.Final = false
		result, err := newExecutor(t).Execute(context.Background(), exchange, recorder)
		if err == nil || !result.Retryable {
			t.Fatalf("result = %+v %v, want a retryable refusal", result, err)
		}
	})

	t.Run("a duplex attempt is captured", func(t *testing.T) {
		codec := &cursorStubCodec{events: cursorTextEvents()}
		result, err := newExecutor(t).Execute(context.Background(),
			cursorExchange(codec, false), httptest.NewRecorder())
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
		if result.Capture == nil || result.Capture.Request == nil {
			t.Fatalf("capture = %+v, want the attempt recorded", result.Capture)
		}
		if !strings.HasSuffix(result.Capture.Request.URL, "/agent.v1.AgentService/Run") {
			t.Fatalf("url = %q, want the run target", result.Capture.Request.URL)
		}
		if got := result.Capture.Request.Headers["Authorization"]; strings.Contains(strings.Join(got, ""), "token") {
			t.Fatalf("Authorization = %q, want the credential masked", got)
		}
		if body := string(result.Capture.Request.Body); body != "framed-run-request" {
			t.Fatalf("body = %q, want the framed bytes the codec wrote", body)
		}
		if _, fabricated := result.Capture.Request.Headers["X-Request-Id"]; fabricated {
			t.Fatalf("headers = %v, want no header the turn never sent", result.Capture.Request.Headers)
		}
	})

	t.Run("a request the codec cannot express fails fast", func(t *testing.T) {
		codec := &cursorStubCodec{err: fmt.Errorf("cursor has no temperature control: %w", wire.ErrUnsupportedFeature)}
		recorder := httptest.NewRecorder()
		exchange := cursorExchange(codec, false)
		exchange.Final = false
		result, err := newExecutor(t).Execute(context.Background(), exchange, recorder)
		if err == nil {
			t.Fatal("Execute() succeeded on an inexpressible request")
		}
		if result.Retryable {
			t.Fatalf("result = %+v, want no retry of a request no account can take", result)
		}
		if result.Status != http.StatusBadRequest {
			t.Fatalf("status = %d, want the caller's failure to fix", result.Status)
		}
		// A pre-flight refusal is the caller's to fix, not a network fault:
		// reporting it as one misnames a local verdict.
		if errors.Is(err, upstream.ErrUpstreamFailed) {
			t.Fatalf("error = %v, want no upstream-failure wrapper on a local refusal", err)
		}
		if !errors.Is(err, wire.ErrUnsupportedFeature) {
			t.Fatalf("error = %v, want the codec's refusal", err)
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
