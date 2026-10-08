// Responses request encoding.
package openairesponses

import (
	"encoding/json"
	"strings"

	"github.com/jonaskahn/relo/internal/adapters/wire"
	"github.com/jonaskahn/relo/internal/inference"
)

const (
	itemMessage            = "message"
	itemFunctionCall       = "function_call"
	itemFunctionCallOutput = "function_call_output"
	itemCustomToolCall     = "custom_tool_call"
	itemCustomCallOutput   = "custom_tool_call_output"
	itemReasoning          = "reasoning"

	contentInputText  = "input_text"
	contentOutputText = "output_text"
	contentInputImage = "input_image"

	toolTypeFunction      = "function"
	toolTypeCustom        = "custom"
	summaryAuto           = "auto"
	summaryText           = "summary_text"
	encryptedReasoningKey = "reasoning.encrypted_content"
)

const instructionSeparator = `` + "\n\n" + ``

type requestPayload struct {
	Model           string        `json:"model"`
	Input           []inputItem   `json:"input"`
	Instructions    string        `json:"instructions,omitempty"`
	Tools           []toolDef     `json:"tools,omitempty"`
	Stream          bool          `json:"stream,omitempty"`
	Store           bool          `json:"store"`
	MaxOutputTokens int           `json:"max_output_tokens,omitempty"`
	Temperature     *float64      `json:"temperature,omitempty"`
	Reasoning       *reasoningDef `json:"reasoning,omitempty"`
	Include         []string      `json:"include,omitempty"`
}

type inputItem struct {
	Type             string        `json:"type,omitempty"`
	Role             string        `json:"role,omitempty"`
	Content          []contentItem `json:"content,omitempty"`
	CallID           string        `json:"call_id,omitempty"`
	Name             string        `json:"name,omitempty"`
	Arguments        string        `json:"arguments,omitempty"`
	Input            string        `json:"input,omitempty"`
	Output           toolOutput    `json:"output,omitempty"`
	ID               string        `json:"id,omitempty"`
	Status           string        `json:"status,omitempty"`
	Summary          []summaryPart `json:"summary,omitempty"`
	EncryptedContent string        `json:"encrypted_content,omitempty"`
}

// MarshalJSON keeps summary on a reasoning item. Codex rejects the request
// when that field is omitted, including when the list is empty.
func (item inputItem) MarshalJSON() ([]byte, error) {
	type encoded inputItem
	if item.Type != itemReasoning {
		return json.Marshal(encoded(item))
	}
	if item.Summary == nil {
		item.Summary = []summaryPart{}
	}
	return json.Marshal(struct {
		encoded
		Summary []summaryPart `json:"summary"`
	}{encoded: encoded(item), Summary: item.Summary})
}

type contentItem struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	ImageURL string `json:"image_url,omitempty"`
}

type summaryPart struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type reasoningDef struct {
	Effort  string `json:"effort,omitempty"`
	Summary string `json:"summary,omitempty"`
}

type toolDef struct {
	Type        string          `json:"type"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

func newPayload(req *inference.Request, opts wire.CodecOpts) requestPayload {
	instructions, items := encodeInput(req.Messages)
	payload := requestPayload{
		Model:           req.Model,
		Input:           placeReasoning(items, req.Reasoning),
		Instructions:    instructions,
		Tools:           encodeTools(req.Tools),
		Stream:          req.Stream,
		Store:           false,
		MaxOutputTokens: req.MaxTokens,
		Temperature:     req.Temperature,
	}
	payload.applyReasoning(req.Reasoning, opts)
	return payload
}

func (p *requestPayload) applyReasoning(reasoning *inference.ReasoningConfig, opts wire.CodecOpts) {
	if reasoning == nil {
		return
	}
	level, off := inference.ChooseThinking(reasoning.Effort, opts.ReasoningEfforts, opts.ReasoningToggle)
	if off {
		level = "none"
	}
	if level == "" && len(reasoning.EncryptedContent) == 0 {
		return
	}
	p.Reasoning = &reasoningDef{Effort: level, Summary: summaryAuto}
	p.Include = []string{encryptedReasoningKey}
}

func placeReasoning(items []inputItem, reasoning *inference.ReasoningConfig) []inputItem {
	if reasoning == nil || len(reasoning.EncryptedContent) == 0 {
		return items
	}
	item := inputItem{
		Type:             itemReasoning,
		ID:               reasoning.ID,
		EncryptedContent: string(reasoning.EncryptedContent),
		Summary:          []summaryPart{},
	}
	for index := range items {
		if items[index].Type == itemFunctionCall {
			return insertItem(items, index, item)
		}
	}
	return append(items, item)
}

func insertItem(items []inputItem, index int, item inputItem) []inputItem {
	items = append(items, inputItem{})
	copy(items[index+1:], items[index:])
	items[index] = item
	return items
}

func encodeInput(messages []inference.Message) (instructions string, items []inputItem) {
	for _, message := range messages {
		if message.Role == inference.RoleSystem {
			instructions = joinInstruction(instructions, textOf(message.Content))
			continue
		}
		items = append(items, encodeMessage(message)...)
	}
	return instructions, items
}

func joinInstruction(existing, text string) string {
	if text == "" {
		return existing
	}
	if existing == "" {
		return text
	}
	return existing + instructionSeparator + text
}

func encodeMessage(message inference.Message) []inputItem {
	if message.Role == inference.RoleTool {
		return []inputItem{{Type: itemFunctionCallOutput, CallID: message.ToolCallID, Output: toolOutput(textOf(message.Content))}}
	}
	return append(messageItems(message), functionCallItems(message.ToolCalls)...)
}

func messageItems(message inference.Message) []inputItem {
	content := encodeContent(message.Role, message.Content)
	if len(content) == 0 {
		return nil
	}
	return []inputItem{{Type: itemMessage, Role: message.Role, Content: content}}
}

func encodeContent(role string, parts []inference.ContentPart) []contentItem {
	textType := contentInputText
	if role == inference.RoleAssistant {
		textType = contentOutputText
	}
	encoded := make([]contentItem, 0, len(parts))
	for _, part := range parts {
		switch part.Type {
		case inference.ContentTypeText:
			spoken := inference.SpokenText([]inference.ContentPart{part})
			if len(spoken) == 0 {
				continue
			}
			encoded = append(encoded, contentItem{Type: textType, Text: spoken[0].Text})
		case inference.ContentTypeImage:
			encoded = append(encoded, contentItem{Type: contentInputImage, ImageURL: part.ImageURL})
		}
	}
	return encoded
}

func functionCallItems(calls []inference.ToolCall) []inputItem {
	items := make([]inputItem, 0, len(calls))
	for _, call := range calls {
		items = append(items, inputItem{Type: itemFunctionCall, CallID: call.ID, Name: call.Name, Arguments: call.Arguments})
	}
	return items
}

func encodeTools(tools []inference.Tool) []toolDef {
	encoded := make([]toolDef, 0, len(tools))
	for _, tool := range tools {
		encoded = append(encoded, toolDef{
			Type:        toolTypeFunction,
			Name:        tool.Name,
			Description: tool.Description,
			Parameters:  tool.Parameters,
		})
	}
	return encoded
}

func textOf(parts []inference.ContentPart) string {
	var builder strings.Builder
	for _, part := range parts {
		if part.Type == inference.ContentTypeText {
			builder.WriteString(part.Text)
		}
	}
	return builder.String()
}
