package cursor

import (
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/wire"
	"github.com/jonaskahn/relo/internal/inference"
)

func validSelection() *wire.CursorRequest {
	return &wire.CursorRequest{
		ConversationID: "conversation-1",
		ModelID:        "sonnet",
		DetailsID:      "sonnet-low",
	}
}

func TestValidate(t *testing.T) {
	base := &inference.Request{
		Model: "sonnet",
		Messages: []inference.Message{
			{Role: inference.RoleUser, Content: []inference.ContentPart{{Type: inference.ContentTypeText, Text: "hi"}}},
		},
	}
	if err := Validate(base, validSelection()); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if err := Validate(base, nil); err == nil {
		t.Fatal("Validate() accepted a missing selection")
	}
	imaged := *base
	imaged.Messages = []inference.Message{
		{Role: inference.RoleUser, Content: []inference.ContentPart{{Type: inference.ContentTypeImage, ImageURL: "data:"}}},
	}
	if err := Validate(&imaged, validSelection()); err == nil {
		t.Fatal("Validate() accepted image content")
	}
	limited := *base
	limited.MaxTokens = 100
	if err := Validate(&limited, validSelection()); err == nil {
		t.Fatal("Validate() accepted a max-output ceiling")
	}
	temperature := 0.5
	heated := *base
	heated.Temperature = &temperature
	if err := Validate(&heated, validSelection()); err == nil {
		t.Fatal("Validate() accepted a temperature")
	}
	forced := *base
	forced.ToolChoice = &inference.ToolChoice{Mode: inference.ToolChoiceRequired}
	if err := Validate(&forced, validSelection()); err == nil {
		t.Fatal("Validate() accepted a forced tool choice")
	}
	unknown := *base
	unknown.Reasoning = &inference.ReasoningConfig{Effort: "nonsense"}
	if err := Validate(&unknown, validSelection()); err == nil {
		t.Fatal("Validate() accepted an unknown effort")
	}
	effort := *base
	effort.Reasoning = &inference.ReasoningConfig{Effort: "high"}
	if err := Validate(&effort, validSelection()); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}
