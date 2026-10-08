package kiro

import (
	"encoding/json"
	"github.com/jonaskahn/relo/internal/inference"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/wire"
)

func TestEncodeRequest(t *testing.T) {
	c := NewCodec("")
	req := &inference.Request{
		Messages: []inference.Message{
			{
				Role: "user",
				Content: []inference.ContentPart{
					{Type: inference.ContentTypeText, Text: "Explain recursion"},
				},
			},
		},
		Reasoning: &inference.ReasoningConfig{Effort: "high"},
	}

	httpReq, err := c.EncodeRequest(req, wire.CodecOpts{Project: "arn:aws:kiro:profile:123"})
	if err != nil {
		t.Fatal(err)
	}

	if httpReq.URL.Path != "/generateAssistantResponse" {
		t.Fatalf("unexpected path: %s", httpReq.URL.Path)
	}
	var body map[string]any
	if err := json.NewDecoder(httpReq.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if _, present := body["thinking"]; present {
		t.Fatalf("body = %v, want no thinking field", body)
	}
	if _, present := body["reasoning_effort"]; present {
		t.Fatalf("body = %v, want no reasoning effort", body)
	}
}

func TestDecodeResponse(t *testing.T) {
	c := NewCodec("")
	body := []byte(`{"content": "Recursion is when a function calls itself."}`)

	events, err := c.DecodeResponse(body)
	if err != nil {
		t.Fatal(err)
	}

	if len(events) != 2 || events[0].Text != "Recursion is when a function calls itself." {
		t.Fatalf("unexpected events: %+v", events)
	}
}

func TestStreamDecoder(t *testing.T) {
	c := NewCodec("")
	dec := c.NewStreamDecoder()

	chunk := map[string]any{
		"assistantResponseEvent": map[string]any{
			"content": "Hello",
		},
	}
	b, _ := json.Marshal(chunk)
	events, err := dec.Push(wire.SSEEvent{Data: string(b)})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Text != "Hello" {
		t.Fatalf("unexpected events: %+v", events)
	}

	finishEvents, err := dec.Finish()
	if err != nil {
		t.Fatal(err)
	}
	if len(finishEvents) != 1 || finishEvents[0].Kind != inference.EventTerminal {
		t.Fatalf("unexpected finish events: %+v", finishEvents)
	}
}
