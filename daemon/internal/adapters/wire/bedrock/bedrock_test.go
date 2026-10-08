package bedrock

import (
	"encoding/json"
	"github.com/jonaskahn/relo/internal/inference"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/wire"
)

func TestEncodeRequest(t *testing.T) {
	c := NewCodec("https://bedrock-runtime.us-east-1.amazonaws.com")
	req := &inference.Request{
		Model: "anthropic.claude-3-5-sonnet-20240620-v1:0",
		Messages: []inference.Message{
			{
				Role: "system",
				Content: []inference.ContentPart{
					{Type: inference.ContentTypeText, Text: "You are helpful"},
				},
			},
			{
				Role: "user",
				Content: []inference.ContentPart{
					{Type: inference.ContentTypeText, Text: "Hello"},
				},
			},
		},
		MaxTokens: 1000,
	}

	httpReq, err := c.EncodeRequest(req, wire.CodecOpts{})
	if err != nil {
		t.Fatal(err)
	}

	if httpReq.URL.String() != "https://bedrock-runtime.us-east-1.amazonaws.com/model/anthropic.claude-3-5-sonnet-20240620-v1:0/converse" {
		t.Fatalf("unexpected URL: %s", httpReq.URL.String())
	}
}

func TestEncodeRequestDropsEmptyText(t *testing.T) {
	c := NewCodec("https://bedrock-runtime.us-east-1.amazonaws.com")
	req := &inference.Request{
		Model: "anthropic.claude-3-5-sonnet-20240620-v1:0",
		Messages: []inference.Message{{
			Role: "user",
			Content: []inference.ContentPart{
				{Type: inference.ContentTypeText},
				{Type: inference.ContentTypeText, Text: "visible"},
			},
		}},
	}
	httpReq, err := c.EncodeRequest(req, wire.CodecOpts{})
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.NewDecoder(httpReq.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	content := body["messages"].([]any)[0].(map[string]any)["content"].([]any)
	if len(content) != 1 || content[0].(map[string]any)["text"] != "visible" {
		t.Fatalf("content = %v, want only the visible sentence", content)
	}
}

func TestEncodeRequestCarriesThinking(t *testing.T) {
	c := NewCodec("https://bedrock-runtime.us-east-1.amazonaws.com")
	req := &inference.Request{
		Model:     "anthropic.claude-opus-4-7",
		Messages:  []inference.Message{{Role: "user", Content: []inference.ContentPart{{Type: inference.ContentTypeText, Text: "Hello"}}}},
		Reasoning: &inference.ReasoningConfig{Effort: "xhigh"},
	}
	httpReq, err := c.EncodeRequest(req, wire.CodecOpts{ReasoningEfforts: []string{"low", "medium", "high", "max"}})
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.NewDecoder(httpReq.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	fields, _ := body["additionalModelRequestFields"].(map[string]any)
	output, _ := fields["output_config"].(map[string]any)
	if output["effort"] != "max" {
		t.Fatalf("fields = %v, want effort max", fields)
	}

	plain := &inference.Request{
		Model:     "amazon.nova-pro",
		Messages:  req.Messages,
		Reasoning: &inference.ReasoningConfig{Effort: "high"},
	}
	httpReq, err = c.EncodeRequest(plain, wire.CodecOpts{ReasoningEfforts: []string{"low", "high"}})
	if err != nil {
		t.Fatal(err)
	}
	body = map[string]any{}
	if err := json.NewDecoder(httpReq.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	fields, _ = body["additionalModelRequestFields"].(map[string]any)
	if fields["reasoning_effort"] != "high" {
		t.Fatalf("fields = %v, want reasoning_effort high", fields)
	}
}

func TestDecodeResponse(t *testing.T) {
	c := NewCodec("")
	body := []byte(`{
		"output": {
			"message": {
				"role": "assistant",
				"content": [
					{"text": "Hello there!"}
				]
			}
		},
		"stopReason": "end_turn",
		"usage": {
			"inputTokens": 10,
			"outputTokens": 5
		}
	}`)

	events, err := c.DecodeResponse(body)
	if err != nil {
		t.Fatal(err)
	}

	if len(events) < 3 {
		t.Fatalf("expected at least 3 events, got %d", len(events))
	}

	hasText := false
	for _, e := range events {
		if e.Kind == inference.EventTextDelta && e.Text == "Hello there!" {
			hasText = true
		}
	}
	if !hasText {
		t.Errorf("missing text delta event")
	}
}

func TestStreamDecoder(t *testing.T) {
	c := NewCodec("")
	decoder := c.NewStreamDecoder()

	chunk1 := map[string]any{
		"contentBlockDelta": map[string]any{
			"contentBlockIndex": 0,
			"delta": map[string]any{
				"text": "Hello",
			},
		},
	}
	b1, _ := json.Marshal(chunk1)
	events1, err := decoder.Push(wire.SSEEvent{Data: string(b1)})
	if err != nil {
		t.Fatal(err)
	}
	if len(events1) != 1 || events1[0].Text != "Hello" {
		t.Fatalf("unexpected events: %+v", events1)
	}

	chunk2 := map[string]any{
		"messageStop": map[string]any{
			"stopReason": "end_turn",
		},
	}
	b2, _ := json.Marshal(chunk2)
	events2, err := decoder.Push(wire.SSEEvent{Data: string(b2)})
	if err != nil {
		t.Fatal(err)
	}
	if len(events2) != 1 || events2[0].Kind != inference.EventTerminal {
		t.Fatalf("unexpected finish event: %+v", events2)
	}
}
