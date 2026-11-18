package anthropic

import (
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/wire"
	"github.com/jonaskahn/relo/internal/inference"
)

// TestInboundEchoesTheUpstreamMessageID pins the identity a client continues.
// Claude Code sends the message id it was answered with as
// thread.previous_message_id, so a synthetic id makes the next turn ask for a
// thread the upstream never opened.
func TestInboundEchoesTheUpstreamMessageID(t *testing.T) {
	inbound := NewInbound()
	if _, err := inbound.EncodeResponseEvent(inference.Event{
		Kind: inference.EventMessageStart, MessageID: "msg_011ChAHLY3ndrYTtPPeJo9H9",
	}); err != nil {
		t.Fatalf("EncodeResponseEvent() error = %v", err)
	}
	frames, err := inbound.EncodeResponseEvent(inference.Event{
		Kind: inference.EventTerminal, Terminal: &inference.TerminalInfo{Reason: inference.EventReasonStop},
	})
	if err != nil {
		t.Fatalf("EncodeResponseEvent() error = %v", err)
	}
	start := frameNamed(frames, "message_start")
	if !strings.Contains(start, "msg_011ChAHLY3ndrYTtPPeJo9H9") {
		t.Fatalf("message_start = %s, want the upstream identity", start)
	}
}

// TestInboundMintsAnIdentityWithoutAnUpstreamOne keeps a local answer
// readable: a terminal with no message identity still opens a stream.
func TestInboundMintsAnIdentityWithoutAnUpstreamOne(t *testing.T) {
	frames, err := NewInbound().EncodeResponseEvent(inference.Event{
		Kind: inference.EventTerminal, Terminal: &inference.TerminalInfo{Reason: inference.EventReasonStop},
	})
	if err != nil {
		t.Fatalf("EncodeResponseEvent() error = %v", err)
	}
	if start := frameNamed(frames, "message_start"); !strings.Contains(start, `"id":"msg_`) {
		t.Fatalf("message_start = %s, want a minted identity", start)
	}
}

func frameNamed(frames []wire.SSEEvent, name string) string {
	for _, frame := range frames {
		if frame.Name == name {
			return frame.Data
		}
	}
	return ""
}
