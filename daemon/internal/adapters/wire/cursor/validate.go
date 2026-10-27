// Cursor request validation: refusing what the protocol cannot express.
package cursor

import (
	"fmt"

	"github.com/jonaskahn/relo/internal/adapters/wire"
	"github.com/jonaskahn/relo/internal/inference"
)

// Validate refuses a canonical request the Cursor protocol cannot express,
// so a field is never silently dropped.
func Validate(req *inference.Request, selection *wire.CursorRequest) error {
	if selection == nil || selection.ConversationID == "" || selection.ModelID == "" {
		return fmt.Errorf("cursor selection: %w", wire.ErrInvalidRequest)
	}
	for _, message := range req.Messages {
		for _, part := range message.Content {
			if part.Type == inference.ContentTypeImage {
				if _, _, ok := splitDataURL(part.ImageURL); !ok {
					return fmt.Errorf("cursor takes inline image bytes, not %q: %w",
						part.ImageURL, wire.ErrUnsupportedFeature)
				}
			}
		}
	}
	if req.MaxTokens != 0 {
		return fmt.Errorf("cursor has no max-output ceiling: %w", wire.ErrUnsupportedFeature)
	}
	if req.Temperature != nil {
		return fmt.Errorf("cursor has no temperature control: %w", wire.ErrUnsupportedFeature)
	}
	if req.ToolChoice != nil && req.ToolChoice.Mode != "" && req.ToolChoice.Mode != inference.ToolChoiceAuto {
		return fmt.Errorf("cursor has no tool-choice control: %w", wire.ErrUnsupportedFeature)
	}
	if req.Reasoning != nil && req.Reasoning.Effort != "" {
		if _, ok := NormalizeEffortRequest(req.Reasoning.Effort); !ok {
			return fmt.Errorf("cursor effort %q: %w", req.Reasoning.Effort, wire.ErrUnsupportedFeature)
		}
	}
	return nil
}

// NormalizeEffortRequest maps a requested effort onto a Cursor selection.
func NormalizeEffortRequest(effort string) (string, bool) {
	switch effort {
	case "off", "toggle", "default":
		return effort, true
	case "none":
		return "off", true
	default:
		name, ok := inference.NormalizeEffort(effort)
		return name, ok
	}
}
