package codec_test

import (
	"net/http"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/wire"
	"github.com/jonaskahn/relo/internal/adapters/wire/openaichat"
	"github.com/jonaskahn/relo/internal/inference"
)

func TestChatStreamErrorFinish(t *testing.T) {
	t.Run("an in-band error finish becomes an error event and terminal", func(t *testing.T) {
		decoder := openaichat.Module().NewStreamDecoder()
		events := pushChatData(t, decoder, `{"id":"chatcmpl-1","choices":[{"index":0,"delta":{"role":"assistant","content":""},"finish_reason":"error","native_finish_reason":"error"}]}`)
		if len(events) != 1 || events[0].Kind != inference.EventError {
			t.Fatalf("Push() = %+v, want one error event", events)
		}
		failure := events[0].Error
		if failure.Status != http.StatusBadGateway || failure.Message == "" {
			t.Fatalf("failure = %+v, want a retryable upstream failure", failure)
		}
		closing, err := decoder.Finish()
		if err != nil {
			t.Fatalf("Finish() error = %v", err)
		}
		if len(closing) != 1 || closing[0].Kind != inference.EventTerminal || closing[0].Terminal.Reason != inference.EventReasonError {
			t.Fatalf("Finish() = %+v, want an error terminal, not end_turn", closing)
		}
	})

	t.Run("an error finish without a message body is still reported", func(t *testing.T) {
		decoder := openaichat.Module().NewStreamDecoder()
		events := pushChatData(t, decoder, `{"id":"chatcmpl-1","choices":[{"index":0,"finish_reason":"error"}]}`)
		if len(events) != 1 || events[0].Kind != inference.EventError {
			t.Fatalf("Push() = %+v, want one error event", events)
		}
	})

	t.Run("a stop finish still ends cleanly", func(t *testing.T) {
		decoder := openaichat.Module().NewStreamDecoder()
		pushChatData(t, decoder, `{"id":"chatcmpl-1","choices":[{"index":0,"delta":{"role":"assistant","content":"hi"},"finish_reason":"stop"}]}`)
		closing, err := decoder.Finish()
		if err != nil {
			t.Fatalf("Finish() error = %v", err)
		}
		if len(closing) != 1 || closing[0].Terminal.Reason != inference.EventReasonStop {
			t.Fatalf("Finish() = %+v, want a stop terminal", closing)
		}
	})

	t.Run("a non-streaming error finish carries the failure", func(t *testing.T) {
		events, err := openaichat.Module().DecodeResponse([]byte(`{"id":"chatcmpl-1","choices":[{"index":0,"message":{"role":"assistant","content":""},"finish_reason":"error"}]}`))
		if err != nil {
			t.Fatalf("DecodeResponse() error = %v", err)
		}
		var seenError, seenTerminal bool
		for _, event := range events {
			if event.Kind == inference.EventError {
				seenError = true
			}
			if event.Kind == inference.EventTerminal {
				seenTerminal = true
				if event.Terminal.Reason != inference.EventReasonError {
					t.Fatalf("terminal = %+v, want error", event.Terminal)
				}
			}
		}
		if !seenError || !seenTerminal {
			t.Fatalf("events = %+v, want the failure and an error terminal", events)
		}
	})
}

func pushChatData(t *testing.T, decoder wire.StreamDecoder, data string) []inference.Event {
	t.Helper()
	events, err := decoder.Push(wire.SSEEvent{Data: data})
	if err != nil {
		t.Fatalf("Push() error = %v", err)
	}
	return events
}
