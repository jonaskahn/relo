package dispatch_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/upstream"
	"github.com/jonaskahn/relo/internal/adapters/wire"
	"github.com/jonaskahn/relo/internal/adapters/wire/openaichat"
	"github.com/jonaskahn/relo/internal/inference"
)

// plainInbound is an inbound codec that renders no error frames of its own, so
// the relay has to write the generic stream failure. It forwards the three
// methods the port declares and nothing else, which is what keeps it out of
// wire.ErrorEncoder.
type plainInbound struct{ inbound wire.InboundCodec }

func (p plainInbound) DecodeRequest(r *http.Request) (*inference.Request, error) {
	return p.inbound.DecodeRequest(r)
}

func (p plainInbound) EncodeResponseEvent(event inference.Event) ([]wire.SSEEvent, error) {
	return p.inbound.EncodeResponseEvent(event)
}

func (p plainInbound) EncodeResponse(events []inference.Event) ([]byte, error) {
	return p.inbound.EncodeResponse(events)
}

// TestFailedStartedStreamWritesAGenericFailure covers the surface with no
// error encoder of its own: the client still gets a frame it can parse.
func TestFailedStartedStreamWritesAGenericFailure(t *testing.T) {
	codec := &cursorStubCodec{
		events: []inference.Event{{Kind: inference.EventTextDelta, Text: "partial"}},
		err:    errors.New(`the turn broke "here"`),
	}
	recorder := httptest.NewRecorder()
	exchange := cursorExchange(codec, true)
	exchange.Final = true
	exchange.Inbound = plainInbound{inbound: chatInbound()}
	if _, err := newExecutor(t).Execute(context.Background(), exchange, recorder); err == nil {
		t.Fatal("Execute() succeeded on a failed stream")
	}
	body := recorder.Body.String()
	if !strings.Contains(body, "event: error") {
		t.Fatalf("body = %q, want an error frame", body)
	}
	frame := ""
	for _, line := range strings.Split(body, "\n") {
		if strings.Contains(line, "stream_error") {
			frame = strings.TrimPrefix(line, "data: ")
		}
	}
	decoded := map[string]string{}
	if err := json.Unmarshal([]byte(frame), &decoded); err != nil {
		t.Fatalf("error frame %q: %v", frame, err)
	}
	if decoded["error"] != "stream_error" {
		t.Fatalf("error = %q, want the stream error code", decoded["error"])
	}
	if decoded["message"] != `the turn broke "here"` {
		t.Fatalf("message = %q, want the failure text quoted once", decoded["message"])
	}
}

// TestFailedStartedStreamUsesTheSurfaceErrorFrames covers a surface that knows
// how to render its own failure: the relay defers to it.
func TestFailedStartedStreamUsesTheSurfaceErrorFrames(t *testing.T) {
	codec := &cursorStubCodec{
		events: []inference.Event{{Kind: inference.EventTextDelta, Text: "partial"}},
		err:    errors.New("the turn broke"),
	}
	recorder := httptest.NewRecorder()
	exchange := cursorExchange(codec, true)
	exchange.Final = true
	if _, err := newExecutor(t).Execute(context.Background(), exchange, recorder); err == nil {
		t.Fatal("Execute() succeeded on a failed stream")
	}
	body := recorder.Body.String()
	if !strings.Contains(body, `"error"`) || !strings.Contains(body, "the turn broke") {
		t.Fatalf("body = %q, want the surface's own error frame", body)
	}
}

// TestFailedStartedStreamReportsAWriteFailure covers a stream whose buffered
// answer could not be written at all: the relay names the write, because the
// client never saw the frames the failure would have followed.
func TestFailedStartedStreamReportsAWriteFailure(t *testing.T) {
	codec := &cursorStubCodec{
		events: []inference.Event{{Kind: inference.EventTextDelta, Text: "partial"}},
		err:    errors.New("the turn broke"),
	}
	exchange := cursorExchange(codec, true)
	exchange.Final = true
	result, err := newExecutor(t).Execute(context.Background(), exchange, abandonedClient{})
	if err == nil {
		t.Fatal("Execute() succeeded on a client that refused the answer")
	}
	if !errors.Is(err, upstream.ErrRelayFailed) {
		t.Fatalf("error = %v, want the relay failure", err)
	}
	if result.Status != http.StatusOK {
		t.Fatalf("status = %d, want the attempt's own answer", result.Status)
	}
}

// flakyClient accepts a budget of writes and then refuses the rest, the way a
// client that disconnects partway through a stream behaves.
type flakyClient struct {
	budget   int
	accepted int
}

func (c *flakyClient) Header() http.Header { return http.Header{} }

func (c *flakyClient) Write(data []byte) (int, error) {
	if c.accepted >= c.budget {
		return 0, errors.New("client is gone")
	}
	c.accepted++
	return len(data), nil
}

func (*flakyClient) WriteHeader(int) {}
func (*flakyClient) Flush()          {}

// TestFailedStartedStreamReportsAFailureToClose covers a client that read the
// answer and then went away: the failure frame never arrived, so the relay
// reports the write instead of pretending the stream was closed.
func TestFailedStartedStreamReportsAFailureToClose(t *testing.T) {
	codec := &cursorStubCodec{
		events: []inference.Event{{Kind: inference.EventTextDelta, Text: "partial"}},
		err:    errors.New("the turn broke"),
	}
	exchange := cursorExchange(codec, true)
	exchange.Final = true
	// The partial answer takes two writes and arrives; the failure frame that
	// follows is the one the client refuses.
	client := &flakyClient{budget: 2}
	result, err := newExecutor(t).Execute(context.Background(), exchange, client)
	if !errors.Is(err, upstream.ErrRelayFailed) {
		t.Fatalf("Execute() error = %v, want the relay failure", err)
	}
	if client.accepted != 2 {
		t.Fatalf("accepted = %d writes, want the partial answer", client.accepted)
	}
	if result.Status != http.StatusOK {
		t.Fatalf("status = %d, want the attempt's own answer", result.Status)
	}
}

// TestFailedStartedStreamLeavesAnAbandonedClientAlone covers the caller that
// went away after reading part of the answer: there is no stream left to close
// and nothing more to say about it.
func TestFailedStartedStreamLeavesAnAbandonedClientAlone(t *testing.T) {
	codec := &cursorStubCodec{
		events: []inference.Event{{Kind: inference.EventTextDelta, Text: "partial"}},
		err:    context.Canceled,
	}
	recorder := httptest.NewRecorder()
	exchange := cursorExchange(codec, true)
	exchange.Final = true
	result, err := newExecutor(t).Execute(context.Background(), exchange, recorder)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Execute() error = %v, want the cancellation", err)
	}
	if result.Status != upstream.StatusClientAborted {
		t.Fatalf("status = %d, want the aborted status", result.Status)
	}
	if body := recorder.Body.String(); strings.Contains(body, "event: error") {
		t.Fatalf("body = %q, want no error frame for a gone caller", body)
	}
}

// chatInbound is the Chat surface, which the plain codec defers every frame it
// cannot render itself to.
func chatInbound() wire.InboundCodec {
	return openaichat.NewChatCompletionsCodec()
}
