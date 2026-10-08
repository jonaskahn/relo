package codec_test

import (
	"bytes"
	"encoding/json"
	"github.com/jonaskahn/relo/internal/inference"
	"os"
	"reflect"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/wire"
	"github.com/jonaskahn/relo/tests/testkit"
)

func TestCanonicalRequestRoundTrip(t *testing.T) {
	t.Run("fixture decodes and re-encodes identically", func(t *testing.T) {
		body, err := os.ReadFile(testkit.FixturePath("codec", "canonical_request.json"))
		if err != nil {
			t.Fatalf("read fixture: %v", err)
		}
		var request inference.Request
		if err := json.Unmarshal(body, &request); err != nil {
			t.Fatalf("decode fixture: %v", err)
		}
		if request.Model != "gpt-4o" || len(request.Messages) != 4 || len(request.Tools) != 1 {
			t.Fatalf("decoded request = %+v", request)
		}
		if !request.Stream || request.MaxTokens != 256 || request.Temperature == nil {
			t.Fatalf("decoded options = %+v", request)
		}
		if request.Reasoning == nil || request.Reasoning.Effort != "low" {
			t.Fatalf("decoded reasoning = %+v", request.Reasoning)
		}
		if request.Metadata["surface"] != "chat-completions" {
			t.Fatalf("decoded metadata = %+v", request.Metadata)
		}
		encoded, err := json.Marshal(request)
		if err != nil {
			t.Fatalf("encode request: %v", err)
		}
		var again inference.Request
		if err := json.Unmarshal(encoded, &again); err != nil {
			t.Fatalf("decode re-encoded request: %v", err)
		}
		compactToolParameters(&request)
		compactToolParameters(&again)
		if !reflect.DeepEqual(request, again) {
			t.Fatalf("round trip changed the request: %+v != %+v", request, again)
		}
	})

	t.Run("messages with text and image content round-trip", func(t *testing.T) {
		request := inference.Request{
			Model: "gpt-4o",
			Messages: []inference.Message{{
				Role: inference.RoleUser,
				Content: []inference.ContentPart{
					{Type: inference.ContentTypeText, Text: "look"},
					{Type: inference.ContentTypeImage, ImageURL: "https://example.invalid/a.png"},
				},
			}},
		}
		assertRoundTrip(t, request)
	})

	t.Run("a thinking part keeps its signature", func(t *testing.T) {
		request := inference.Request{
			Model: "claude-sonnet-4",
			Messages: []inference.Message{{Role: inference.RoleAssistant, Content: []inference.ContentPart{
				{Type: inference.ContentTypeThinking, Text: "weighing options", Signature: "sig-value"},
			}}},
		}
		assertRoundTrip(t, request)
	})

	t.Run("tool calls round-trip", func(t *testing.T) {
		request := inference.Request{
			Model: "gpt-4o",
			Messages: []inference.Message{{
				Role:       inference.RoleAssistant,
				ToolCalls:  []inference.ToolCall{{ID: "call_1", Name: "lookup", Arguments: "{\"q\":1}"}},
				ToolCallID: "call_1",
				Name:       "lookup",
			}},
			Tools: []inference.Tool{{Name: "lookup", Description: "look it up", Parameters: json.RawMessage("{}")}},
		}
		assertRoundTrip(t, request)
	})
}

func TestCanonicalEventRoundTrip(t *testing.T) {
	tests := []struct {
		name  string
		event inference.Event
	}{
		{"text delta", inference.Event{Kind: inference.EventTextDelta, Text: "hello"}},
		{"reasoning delta", inference.Event{Kind: inference.EventReasoningDelta, Text: "thinking"}},
		{"reasoning text with family state", inference.Event{Kind: inference.EventReasoningDelta, Reasoning: &inference.ReasoningConfig{ID: "rs_1", Signature: "sig", EncryptedContent: []byte{0x01}}}},
		{"tool call start", inference.Event{Kind: inference.EventToolCallStart, ToolCall: &inference.ToolCallDelta{Index: 0, ID: "call_1", Name: "lookup"}}},
		{"tool call delta", inference.Event{Kind: inference.EventToolCallDelta, ToolCall: &inference.ToolCallDelta{Index: 0, Arguments: "{}"}}},
		{"tool call end", inference.Event{Kind: inference.EventToolCallEnd, ToolCall: &inference.ToolCallDelta{Index: 0, ID: "call_1"}}},
		{"usage", inference.Event{Kind: inference.EventUsage, Usage: &inference.UsageReport{InputTokens: 10, OutputTokens: 4, CacheReadTokens: 2, CacheWriteTokens: 1}}},
		{"terminal", inference.Event{Kind: inference.EventTerminal, Terminal: &inference.TerminalInfo{Reason: inference.EventReasonToolUse}}},
		{"error", inference.Event{Kind: inference.EventError, Error: &inference.ErrorInfo{Code: "bad_gateway", Message: "upstream is down", Status: 502}}},
	}
	for _, tt := range tests {
		t.Run(tt.name+" round-trip", func(t *testing.T) {
			assertRoundTrip(t, tt.event)
		})
	}

	t.Run("reasoning encrypted content is preserved", func(t *testing.T) {
		request := inference.Request{
			Model:     "gpt-4o",
			Reasoning: &inference.ReasoningConfig{Effort: "high", EncryptedContent: []byte{0x00, 0xff, 0x7f}},
		}
		assertRoundTrip(t, request)
	})
}

func TestEventKindString(t *testing.T) {
	tests := []struct {
		kind inference.EventKind
		want string
	}{
		{inference.EventTextDelta, "text_delta"},
		{inference.EventToolCallStart, "tool_call_start"},
		{inference.EventToolCallDelta, "tool_call_delta"},
		{inference.EventToolCallEnd, "tool_call_end"},
		{inference.EventReasoningDelta, "reasoning_delta"},
		{inference.EventUsage, "usage"},
		{inference.EventTerminal, "terminal"},
		{inference.EventError, "error"},
		{inference.EventKind(42), "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.kind.String(); got != tt.want {
				t.Fatalf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSSEEventFields(t *testing.T) {
	event := wire.SSEEvent{Name: "message", Data: "payload", ID: "7"}
	encoded, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("encode SSE event: %v", err)
	}
	var again wire.SSEEvent
	if err := json.Unmarshal(encoded, &again); err != nil {
		t.Fatalf("decode SSE event: %v", err)
	}
	if again != event {
		t.Fatalf("round trip = %+v, want %+v", again, event)
	}
}

func assertRoundTrip[T any](t *testing.T, value T) {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var decoded T
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !reflect.DeepEqual(value, decoded) {
		t.Fatalf("round trip = %+v, want %+v", decoded, value)
	}
}

// compactToolParameters normalizes raw JSON so a round trip can be
// compared field by field instead of byte by byte.
func compactToolParameters(request *inference.Request) {
	for index := range request.Tools {
		compacted := &bytes.Buffer{}
		if err := json.Compact(compacted, request.Tools[index].Parameters); err != nil {
			continue
		}
		request.Tools[index].Parameters = json.RawMessage(compacted.Bytes())
	}
}
