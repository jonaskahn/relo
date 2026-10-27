package openai_responses_test

import (
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/wire"
	"github.com/jonaskahn/relo/internal/adapters/wire/openaichat"
	"github.com/jonaskahn/relo/internal/adapters/wire/openairesponses"
)

// TestXiaomiFreeformTurnReplay replays the captured MiMo stream through the
// whole Codex path: a Responses request with a freeform exec tool, the
// upstream chat stream that split the call across a structured delta and
// template text, and the Responses frames Codex would run.
func TestXiaomiFreeformTurnReplay(t *testing.T) {
	body := []byte(`{"model":"gpt-5","stream":true,
		"tools":[{"type":"custom","name":"exec","description":"run"}],
		"input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"go"}]}]}`)
	inbound := openairesponses.NewInbound()
	request, err := decodeInboundWith(inbound, body)
	if err != nil {
		t.Fatalf("DecodeRequest() error = %v", err)
	}

	codec := openaichat.NewCodec("").Bind(request, wire.CodecOpts{ToolCallDialect: "qwen-text"}).(wire.StreamCodec)
	events := pushStream(t, codec.NewStreamDecoder(), "openai/xiaomi_mimo_exec_stream.txt")

	var frames []wire.SSEEvent
	for _, event := range events {
		encoded, err := inbound.EncodeResponseEvent(event)
		if err != nil {
			t.Fatalf("EncodeResponseEvent(%s) error = %v", event.Kind, err)
		}
		frames = append(frames, encoded...)
	}

	done := frameNamed(frames, "response.output_item.done")
	for _, want := range []string{`"type":"custom_tool_call"`, `"name":"exec"`, `"input":"const r = await tools.exec_command`} {
		if !strings.Contains(done, want) {
			t.Fatalf("custom item = %s, want %s", done, want)
		}
	}
	if !strings.Contains(done, `;text(r.output)"`) {
		t.Fatalf("custom item = %s, want the whole recovered body", done)
	}
	for _, frame := range frames {
		if frame.Name == "response.output_text.delta" && strings.Contains(frame.Data, "<tool_call>") {
			t.Fatalf("text frame = %s, want no template text streamed as prose", frame.Data)
		}
	}
}
