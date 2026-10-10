package codec

import (
	"net/http"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/wire"
	"github.com/jonaskahn/relo/internal/inference"
)

// namedCodec answers every path with one tool call under the name it was
// handed, which is the shape a lane's upstream alias arrives in.
type namedCodec struct{ name string }

func (c namedCodec) EncodeRequest(*inference.Request, wire.CodecOpts) (*http.Request, error) {
	return nil, nil
}

func (c namedCodec) DecodeResponseEvent(wire.SSEEvent) ([]inference.Event, error) {
	return []inference.Event{c.call()}, nil
}

func (c namedCodec) DecodeResponse([]byte) ([]inference.Event, error) {
	return []inference.Event{c.call()}, nil
}

func (c namedCodec) DecodeError(int, []byte) *inference.ErrorInfo { return nil }

func (c namedCodec) NewStreamDecoder() wire.StreamDecoder { return namedStream(c) }

func (c namedCodec) call() inference.Event {
	return inference.Event{
		Kind:     inference.EventToolCallDelta,
		ToolCall: &inference.ToolCallDelta{Index: 0, Name: c.name},
	}
}

type namedStream struct{ name string }

func (s namedStream) Push(wire.SSEEvent) ([]inference.Event, error) {
	return namedCodec(s).DecodeResponseEvent(wire.SSEEvent{})
}

func (s namedStream) Finish() ([]inference.Event, error) {
	return namedCodec(s).DecodeResponseEvent(wire.SSEEvent{})
}

// A tool call reaches the client on whichever answer path the attempt took, so
// every one of them has to name the tool the client declared.
func TestRestoreToolNamesNamesTheToolOnEveryAnswerPath(t *testing.T) {
	const alias, original = "mcp__claude_ai_Cloudflare_Developer_P_7f3a91b2c4d5", "mcp__claude_ai_Cloudflare_Developer_Platform__complete_authentication"
	module := wire.RestoreToolNames(namedCodec{name: alias}, map[string]string{alias: original})

	decoder := module.NewStreamDecoder()
	pushed, err := decoder.Push(wire.SSEEvent{})
	if err != nil {
		t.Fatalf("push: %v", err)
	}
	finished, err := decoder.Finish()
	if err != nil {
		t.Fatalf("finish: %v", err)
	}
	complete, err := module.DecodeResponse(nil)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	single, err := module.DecodeResponseEvent(wire.SSEEvent{})
	if err != nil {
		t.Fatalf("decode event: %v", err)
	}
	for path, events := range map[string][]inference.Event{
		"push": pushed, "finish": finished, "complete": complete, "event": single,
	} {
		if len(events) != 1 || events[0].ToolCall == nil {
			t.Fatalf("%s = %+v, want the tool call it answered with", path, events)
		}
		if events[0].ToolCall.Name != original {
			t.Errorf("%s named %q, want the name the client declared", path, events[0].ToolCall.Name)
		}
	}
}

// A lane that shortened nothing keeps its own codec, so an ordinary attempt
// carries no extra layer and nothing to undo.
func TestRestoreToolNamesPassesThroughALaneThatShortenedNothing(t *testing.T) {
	inner := namedCodec{name: "read"}
	if wire.RestoreToolNames(inner, nil) != wire.CodecModule(inner) {
		t.Fatal("nil originals wrapped the codec, want it passed through")
	}
	if wire.RestoreToolNames(inner, map[string]string{}) != wire.CodecModule(inner) {
		t.Fatal("empty originals wrapped the codec, want it passed through")
	}
}
