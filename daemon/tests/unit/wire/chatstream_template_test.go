package codec_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/wire/openaichat"
	"github.com/jonaskahn/relo/internal/inference"
)

// TestChatStreamTemplateCall covers the Qwen / MiMo family, which renders a
// tool call as literal text beside an empty structured call. Fix A recovers
// the body; Fix B fails the turn over when only the text arrives.
func TestChatStreamTemplateCall(t *testing.T) {
	t.Run("a structured call with an empty body is repaired from template text", func(t *testing.T) {
		decoder := openaichat.Module().NewStreamDecoder()
		var events []inference.Event
		events = append(events, pushChatData(t, decoder, `{"id":"chatcmpl-1","choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"exec","arguments":""}}]},"finish_reason":null}]}`)...)
		// The template body arrives split across two deltas, tags included.
		events = append(events, pushChatData(t, decoder, `{"id":"chatcmpl-1","choices":[{"index":0,"delta":{"content":"\u003ctool_call\u003e\u003cfunction=exec\u003econst r = await tools.exec_command({cmd:\"ls\"});text(r.output)"},"finish_reason":null}]}`)...)
		events = append(events, pushChatData(t, decoder, `{"id":"chatcmpl-1","choices":[{"index":0,"delta":{"content":"\u003c/parameter\u003e\u003c/function\u003e\u003c/tool_call\u003e"},"finish_reason":null}]}`)...)
		events = append(events, pushChatData(t, decoder, `{"id":"chatcmpl-1","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{}"}}]},"finish_reason":null}]}`)...)
		events = append(events, pushChatData(t, decoder, `{"id":"chatcmpl-1","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`)...)

		closing, err := decoder.Finish()
		if err != nil {
			t.Fatalf("Finish() error = %v", err)
		}
		events = append(events, closing...)

		if failure := chatStreamFailure(t, events); failure != nil {
			t.Fatalf("failure = %+v, want the template call recovered, not failed", failure)
		}
		if reason := chatStreamTerminalReason(t, events); reason != inference.EventReasonToolUse {
			t.Fatalf("terminal reason = %q, want tool use", reason)
		}
		want := `{"input":"const r = await tools.exec_command({cmd:\"ls\"});text(r.output)"}`
		if got := chatStreamCallArguments(t, events); got != want {
			t.Fatalf("call arguments = %q, want the recovered body %q", got, want)
		}
		if text := chatStreamText(t, events); strings.Contains(text, "<tool_call>") {
			t.Fatalf("text = %q, want the template call suppressed", text)
		}
	})

	t.Run("template text with no structured call fails the turn", func(t *testing.T) {
		decoder := openaichat.Module().NewStreamDecoder()
		events := pushChatData(t, decoder, `{"id":"chatcmpl-1","choices":[{"index":0,"delta":{"content":"\u003ctool_call\u003e\u003cfunction=exec\u003egrep -rn foo\u003c/parameter\u003e\u003c/function\u003e\u003c/tool_call\u003e"},"finish_reason":"stop"}]}`)
		closing, err := decoder.Finish()
		if err != nil {
			t.Fatalf("Finish() error = %v", err)
		}
		events = append(events, closing...)

		failure := chatStreamFailure(t, events)
		if failure == nil || failure.Code != "leaked_tool_call" || failure.Status != http.StatusBadGateway {
			t.Fatalf("failure = %+v, want the template call reported as an upstream fault", failure)
		}
		if text := chatStreamText(t, events); strings.Contains(text, "<function=") {
			t.Fatalf("text = %q, want the template call suppressed even on failure", text)
		}
	})

	t.Run("a template marker beside a real call still ends cleanly", func(t *testing.T) {
		decoder := openaichat.Module().NewStreamDecoder()
		var events []inference.Event
		events = append(events, pushChatData(t, decoder, `{"id":"chatcmpl-1","choices":[{"index":0,"delta":{"content":"Here is the transcript: \u003ctool_call\u003e\u003cfunction=read\u003e{\"path\":\"/x\"}\u003c/parameter\u003e\u003c/function\u003e\u003c/tool_call\u003e"},"finish_reason":null}]}`)...)
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
		if got := chatStreamCallArguments(t, events); got != `{"command":"cd /tmp"}` {
			t.Fatalf("call arguments = %q, want the real call untouched", got)
		}
	})
}

// chatStreamCallArguments returns the arguments of the one terminal tool call.
func chatStreamCallArguments(t *testing.T, events []inference.Event) string {
	t.Helper()
	var arguments string
	for _, event := range events {
		if event.Kind == inference.EventToolCallEnd && event.ToolCall != nil {
			arguments = event.ToolCall.Arguments
		}
	}
	return arguments
}

// chatStreamText joins every forwarded text delta.
func chatStreamText(t *testing.T, events []inference.Event) string {
	t.Helper()
	var builder strings.Builder
	for _, event := range events {
		if event.Kind == inference.EventTextDelta {
			builder.WriteString(event.Text)
		}
	}
	return builder.String()
}
