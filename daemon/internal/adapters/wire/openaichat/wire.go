// Chat wire shapes: requests, messages, and tool calls.
package openaichat

import (
	"encoding/json"
	"fmt"
	"github.com/jonaskahn/relo/internal/adapters/wire"
	"github.com/jonaskahn/relo/internal/inference"
)

type chatWireRequest struct {
	Model               string             `json:"model"`
	Messages            []chatWireMessage  `json:"messages"`
	Tools               []chatWireTool     `json:"tools"`
	Stream              bool               `json:"stream"`
	MaxTokens           int                `json:"max_tokens"`
	MaxCompletionTokens int                `json:"max_completion_tokens"`
	Temperature         *float64           `json:"temperature"`
	Reasoning           *chatWireReasoning `json:"reasoning"`
	ReasoningEffort     string             `json:"reasoning_effort"`
}

type chatWireMessage struct {
	Role             string             `json:"role"`
	Content          json.RawMessage    `json:"content"`
	ToolCalls        []chatWireToolCall `json:"tool_calls"`
	ToolCallID       string             `json:"tool_call_id"`
	Name             string             `json:"name"`
	ReasoningContent string             `json:"reasoning_content"`
}

type chatWireToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type chatWireTool struct {
	Type     string `json:"type"`
	Function struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Parameters  json.RawMessage `json:"parameters"`
	} `json:"function"`
}

type chatWirePart struct {
	Type     string `json:"type"`
	Text     string `json:"text"`
	ImageURL struct {
		URL string `json:"url"`
	} `json:"image_url"`
}

type chatWireReasoning struct {
	Effort string `json:"effort"`
}

func (w chatWireRequest) toCanonical() (*inference.Request, error) {
	if w.Model == "" {
		return nil, fmt.Errorf("%w: model is required", wire.ErrInvalidRequest)
	}
	messages, err := decodeMessages(w.Messages)
	if err != nil {
		return nil, err
	}
	return &inference.Request{
		Model:       w.Model,
		Messages:    messages,
		Tools:       decodeTools(w.Tools),
		Stream:      w.Stream,
		MaxTokens:   w.maxTokens(),
		Temperature: w.Temperature,
		Reasoning:   w.reasoning(),
	}, nil
}

func (w chatWireRequest) maxTokens() int {
	if w.MaxCompletionTokens > 0 {
		return w.MaxCompletionTokens
	}
	return w.MaxTokens
}

func decodeMessages(messages []chatWireMessage) ([]inference.Message, error) {
	decoded := make([]inference.Message, 0, len(messages))
	for _, message := range messages {
		content, err := decodeContent(message.Content)
		if err != nil {
			return nil, err
		}
		if message.ReasoningContent != "" {
			content = append([]inference.ContentPart{{
				Type: inference.ContentTypeThinking,
				Text: message.ReasoningContent,
			}}, content...)
		}
		decoded = append(decoded, inference.Message{
			Role:       message.Role,
			Content:    content,
			ToolCalls:  decodeToolCalls(message.ToolCalls),
			ToolCallID: message.ToolCallID,
			Name:       message.Name,
		})
	}
	return decoded, nil
}

func decodeContent(raw json.RawMessage) ([]inference.ContentPart, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return []inference.ContentPart{{Type: inference.ContentTypeText, Text: text}}, nil
	}
	var parts []chatWirePart
	if err := json.Unmarshal(raw, &parts); err != nil {
		return nil, fmt.Errorf("%w: content must be a string or an array of parts", wire.ErrInvalidRequest)
	}
	return decodeParts(parts)
}

func decodeParts(parts []chatWirePart) ([]inference.ContentPart, error) {
	decoded := make([]inference.ContentPart, 0, len(parts))
	for _, part := range parts {
		switch part.Type {
		case inference.ContentTypeText:
			decoded = append(decoded, inference.ContentPart{Type: inference.ContentTypeText, Text: part.Text})
		case "image_url":
			decoded = append(decoded, inference.ContentPart{Type: inference.ContentTypeImage, ImageURL: part.ImageURL.URL})
		default:
			return nil, fmt.Errorf("%w: unsupported content part %q", wire.ErrInvalidRequest, part.Type)
		}
	}
	return decoded, nil
}

func decodeToolCalls(calls []chatWireToolCall) []inference.ToolCall {
	decoded := make([]inference.ToolCall, 0, len(calls))
	for _, call := range calls {
		decoded = append(decoded, inference.ToolCall{
			ID:        call.ID,
			Name:      call.Function.Name,
			Arguments: call.Function.Arguments,
		})
	}
	return decoded
}

func decodeTools(tools []chatWireTool) []inference.Tool {
	decoded := make([]inference.Tool, 0, len(tools))
	for _, tool := range tools {
		decoded = append(decoded, inference.Tool{
			Name:        tool.Function.Name,
			Description: tool.Function.Description,
			Parameters:  tool.Function.Parameters,
		})
	}
	return decoded
}

func (w chatWireRequest) reasoning() *inference.ReasoningConfig {
	if w.Reasoning != nil {
		return &inference.ReasoningConfig{Effort: w.Reasoning.Effort}
	}
	if w.ReasoningEffort != "" {
		return &inference.ReasoningConfig{Effort: w.ReasoningEffort}
	}
	return nil
}
