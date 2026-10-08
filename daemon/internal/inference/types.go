// Package codec holds the canonical request and event format that every
// wire family translates to and from.
package inference

import (
	"encoding/json"
	"strings"
)

// Message roles and content kinds name who said what in a turn, in the one
// vocabulary every wire format translates through.
const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleTool      = "tool"

	ContentTypeText     = "text"
	ContentTypeImage    = "image"
	ContentTypeThinking = "thinking"
)

// SpokenText returns the text parts a provider can send. An empty or
// whitespace-only part is not a valid text block on the wires Relo speaks,
// and a thinking part stays with the family that produced it.
func SpokenText(parts []ContentPart) []ContentPart {
	spoken := make([]ContentPart, 0, len(parts))
	for _, part := range parts {
		if part.Type != ContentTypeText || strings.TrimSpace(part.Text) == "" {
			continue
		}
		spoken = append(spoken, part)
	}
	return spoken
}

// Request is the internal representation of any LLM request.
type Request struct {
	Model       string           `json:"model"`
	Messages    []Message        `json:"messages"`
	Tools       []Tool           `json:"tools,omitempty"`
	Stream      bool             `json:"stream"`
	MaxTokens   int              `json:"max_tokens,omitempty"`
	Temperature *float64         `json:"temperature,omitempty"`
	Reasoning   *ReasoningConfig `json:"reasoning,omitempty"`
	Metadata    map[string]any   `json:"metadata,omitempty"`
}

// Message is one turn of the conversation.
type Message struct {
	Role       string        `json:"role"`
	Content    []ContentPart `json:"content"`
	ToolCalls  []ToolCall    `json:"tool_calls,omitempty"`
	ToolCallID string        `json:"tool_call_id,omitempty"`
	Name       string        `json:"name,omitempty"`
}

// ContentPart is one piece of message content, described by Type. Signature
// carries the opaque marker a family needs to replay a thinking part.
type ContentPart struct {
	Type      string `json:"type"`
	Text      string `json:"text,omitempty"`
	ImageURL  string `json:"image_url,omitempty"`
	Signature string `json:"signature,omitempty"`
}

// Tool describes a function the model may call.
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// ToolCall is one model-initiated tool invocation, arguments included.
type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ReasoningConfig carries the reasoning controls a surface exposes and the
// opaque state one family needs to replay its reasoning items. The ID,
// signature, and encrypted content are family state: only the family that
// produced them can interpret them, and they travel unchanged.
type ReasoningConfig struct {
	Effort           string `json:"effort,omitempty"`
	ID               string `json:"id,omitempty"`
	Signature        string `json:"signature,omitempty"`
	EncryptedContent []byte `json:"encrypted_content,omitempty"`
}

// NormalizeEffort maps a client effort onto the ladder a provider wire can
// carry. An unrecognised effort omits the field. ultra is sent as max.
// none is a real level meaning thinking off. minimal stays distinct so a
// token budget can give it its own size.
func NormalizeEffort(effort string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(effort)) {
	case "ultra":
		return "max", true
	case "none", "minimal", "low", "medium", "high", "xhigh", "max":
		return strings.ToLower(strings.TrimSpace(effort)), true
	default:
		return "", false
	}
}

// WireEffort is NormalizeEffort with minimal sent as low, which is the string
// an OpenAI effort field accepts.
func WireEffort(effort string) (string, bool) {
	name, ok := NormalizeEffort(effort)
	if !ok {
		return "", false
	}
	if name == "minimal" {
		return "low", true
	}
	return name, true
}
