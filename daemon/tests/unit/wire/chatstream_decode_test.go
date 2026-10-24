package codec_test

import (
	"net/http"
	"strings"
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

func TestChatStreamLeakedToolCall(t *testing.T) {
	t.Run("a leaked call split across deltas fails the turn", func(t *testing.T) {
		decoder := openaichat.Module().NewStreamDecoder()
		var events []inference.Event
		events = append(events, pushChatData(t, decoder, `{"id":"chatcmpl-1","choices":[{"index":0,"delta":{"reasoning_content":"Now Commit 4. [Assistant tool call]: read({\"path\":\"/x\"})\n\u003c/rfl:"},"finish_reason":null}]}`)...)
		events = append(events, pushChatData(t, decoder, `{"id":"chatcmpl-1","choices":[{"index":0,"delta":{"reasoning_content":"parameter\u003e\u003c|start|\u003e"},"finish_reason":null}]}`)...)
		events = append(events, pushChatData(t, decoder, `{"id":"chatcmpl-1","choices":[{"index":0,"delta":{"content":"Let me read it."},"finish_reason":"stop"}]}`)...)
		events = append(events, pushChatData(t, decoder, `{"id":"chatcmpl-1","choices":[],"usage":{"prompt_tokens":120,"completion_tokens":7}}`)...)

		closing, err := decoder.Finish()
		if err != nil {
			t.Fatalf("Finish() error = %v", err)
		}
		events = append(events, closing...)

		failure := chatStreamFailure(t, events)
		if failure == nil || failure.Code != "leaked_tool_call" || failure.Status != http.StatusBadGateway {
			t.Fatalf("failure = %+v, want the leaked tool call reported as an upstream fault", failure)
		}
		if reason := chatStreamTerminalReason(t, events); reason != inference.EventReasonError {
			t.Fatalf("terminal reason = %q, want error", reason)
		}
		assertLeakEncoded(t, events)
	})

	t.Run("a marker beside a real tool call still ends cleanly", func(t *testing.T) {
		decoder := openaichat.Module().NewStreamDecoder()
		var events []inference.Event
		events = append(events, pushChatData(t, decoder, `{"id":"chatcmpl-1","choices":[{"index":0,"delta":{"reasoning_content":"\u003cconstrain|xml|message|\u003erfl:parameter name=\"command\"\u003ecd /tmp\u003c/rfl:parameter\u003e"},"finish_reason":null}]}`)...)
		events = append(events, pushChatData(t, decoder, `{"id":"chatcmpl-1","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"shell","arguments":"{\"command\":\"cd /tmp\"}"}}]},"finish_reason":null}]}`)...)
		events = append(events, pushChatData(t, decoder, `{"id":"chatcmpl-1","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`)...)

		closing, err := decoder.Finish()
		if err != nil {
			t.Fatalf("Finish() error = %v", err)
		}
		events = append(events, closing...)

		if failure := chatStreamFailure(t, events); failure != nil {
			t.Fatalf("failure = %+v, want a clean tool call", failure)
		}
		if reason := chatStreamTerminalReason(t, events); reason != inference.EventReasonToolUse {
			t.Fatalf("terminal reason = %q, want tool use", reason)
		}
	})

	t.Run("an in-band error finish keeps its own failure", func(t *testing.T) {
		decoder := openaichat.Module().NewStreamDecoder()
		events := pushChatData(t, decoder, `{"id":"chatcmpl-1","choices":[{"index":0,"delta":{"reasoning_content":"\u003c/rfl:parameter\u003e"},"finish_reason":"error"}]}`)
		closing, err := decoder.Finish()
		if err != nil {
			t.Fatalf("Finish() error = %v", err)
		}
		failure := chatStreamFailure(t, append(events, closing...))
		if failure == nil || failure.Code != "upstream_error" {
			t.Fatalf("failure = %+v, want the finish's own error", failure)
		}
	})

	t.Run("a non-streaming leaked call also fails", func(t *testing.T) {
		events, err := openaichat.Module().DecodeResponse([]byte(`{"id":"chatcmpl-1","choices":[{"index":0,"message":{"role":"assistant","reasoning_content":"\u003c/rfl:parameter\u003e","content":"done"},"finish_reason":"stop"}]}`))
		if err != nil {
			t.Fatalf("DecodeResponse() error = %v", err)
		}
		if failure := chatStreamFailure(t, events); failure == nil || failure.Code != "leaked_tool_call" {
			t.Fatalf("failure = %+v, want the leaked tool call reported", failure)
		}
	})
}

// chatStreamFailure returns the one error event among events, if any.
func chatStreamFailure(t *testing.T, events []inference.Event) *inference.ErrorInfo {
	t.Helper()
	var failure *inference.ErrorInfo
	for _, event := range events {
		if event.Kind != inference.EventError {
			continue
		}
		if failure != nil {
			t.Fatalf("events = %+v, want one error event", events)
		}
		failure = event.Error
	}
	return failure
}

// chatStreamTerminalReason returns the reason of the one terminal event.
func chatStreamTerminalReason(t *testing.T, events []inference.Event) string {
	t.Helper()
	var reason string
	for _, event := range events {
		if event.Kind != inference.EventTerminal {
			continue
		}
		if reason != "" {
			t.Fatalf("events = %+v, want one terminal event", events)
		}
		if event.Terminal != nil {
			reason = event.Terminal.Reason
		}
	}
	if reason == "" {
		t.Fatalf("events = %+v, want a terminal event", events)
	}
	return reason
}

// assertLeakEncoded checks the client sees the fault instead of a stop.
func assertLeakEncoded(t *testing.T, events []inference.Event) {
	t.Helper()
	codec := openaichat.NewChatCompletionsCodec()
	var stream strings.Builder
	for _, event := range events {
		frames, err := codec.EncodeResponseEvent(event)
		if err != nil {
			t.Fatalf("EncodeResponseEvent(%s) error = %v", event.Kind, err)
		}
		for _, frame := range frames {
			stream.WriteString("data: " + frame.Data + "\n\n")
		}
	}
	body := stream.String()
	if !strings.Contains(body, `"type":"leaked_tool_call"`) {
		t.Fatalf("stream = %q, want the leaked call reported", body)
	}
	if strings.Contains(body, `"finish_reason":"stop"`) {
		t.Fatalf("stream = %q, want no clean stop after a fault", body)
	}
	if !strings.HasSuffix(strings.TrimSpace(body), "data: [DONE]") {
		t.Fatalf("stream = %q, want the sentinel to close it", body)
	}
}
