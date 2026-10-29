package inference

import (
	"strings"
	"testing"
)

func TestTextualViewDropsImagePayloads(t *testing.T) {
	payload := "data:image/png;base64," + strings.Repeat("A", 4096)
	request := &Request{Messages: []Message{
		{Role: RoleUser, Content: []ContentPart{{Type: ContentTypeText, Text: "look"}}},
		{Role: RoleUser, Content: []ContentPart{{Type: ContentTypeImage, ImageURL: payload}}},
	}}

	view, images := TextualView(request)

	if images != 1 {
		t.Fatalf("images = %d, want 1", images)
	}
	if view.Messages[1].Content[0].ImageURL != "" {
		t.Fatalf("image payload = %q, want it dropped from the view", view.Messages[1].Content[0].ImageURL)
	}
	if view.Messages[0].Content[0].Text != "look" {
		t.Fatalf("text = %q, want the text kept", view.Messages[0].Content[0].Text)
	}
	if request.Messages[1].Content[0].ImageURL != payload {
		t.Fatal("TextualView modified the caller's request")
	}
}

func TestTextualViewOfNothing(t *testing.T) {
	view, images := TextualView(nil)
	if view != nil || images != 0 {
		t.Fatalf("TextualView(nil) = %v, %d, want nil, 0", view, images)
	}
}
