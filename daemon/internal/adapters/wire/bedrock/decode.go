// Bedrock Converse response decoding.
package bedrock

import (
	"encoding/json"
	"fmt"
	"github.com/jonaskahn/relo/internal/inference"
	"net/http"
	"strings"

	"github.com/jonaskahn/relo/internal/adapters/wire"
)

type converseOutput struct {
	Output struct {
		Message struct {
			Role    string         `json:"role"`
			Content []conversePart `json:"content"`
		} `json:"message"`
	} `json:"output"`
	StopReason string        `json:"stopReason"`
	Usage      converseUsage `json:"usage"`
	Message    string        `json:"message,omitempty"`
}

type conversePart struct {
	Text    string `json:"text,omitempty"`
	ToolUse *struct {
		ToolUseID string `json:"toolUseId"`
		Name      string `json:"name"`
		Input     any    `json:"input"`
	} `json:"toolUse,omitempty"`
}

type converseUsage struct {
	InputTokens  int `json:"inputTokens"`
	OutputTokens int `json:"outputTokens"`
}

// DecodeResponse decodes a complete Bedrock converse response.
func (c *Codec) DecodeResponse(body []byte) ([]inference.Event, error) {
	var resp converseOutput
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode bedrock response: %w", err)
	}

	if resp.Message != "" && len(resp.Output.Message.Content) == 0 {
		return bedrockFailureEvents(resp.Message), nil
	}

	events := decodeBedrockContent(resp.Output.Message.Content)
	events = append(events, decodeBedrockUsage(resp.Usage)...)
	return append(events, inference.Event{
		Kind: inference.EventTerminal,
		Terminal: &inference.TerminalInfo{
			Reason: bedrockFinishReason(resp.StopReason),
		},
	}), nil
}

func bedrockFailureEvents(message string) []inference.Event {
	return []inference.Event{
		{
			Kind: inference.EventError,
			Error: &inference.ErrorInfo{
				Code:    "upstream_error",
				Message: message,
				Status:  http.StatusBadGateway,
			},
		},
		{
			Kind: inference.EventTerminal,
			Terminal: &inference.TerminalInfo{
				Reason: inference.EventReasonError,
			},
		},
	}
}

func decodeBedrockContent(content []conversePart) []inference.Event {
	var events []inference.Event
	toolIdx := 0
	for _, part := range content {
		if part.Text != "" {
			events = append(events, inference.Event{
				Kind: inference.EventTextDelta,
				Text: part.Text,
			})
		}
		if part.ToolUse != nil {
			events = append(events, bedrockToolCallEvents(toolIdx, part)...)
			toolIdx++
		}
	}
	return events
}

func bedrockToolCallEvents(toolIdx int, part conversePart) []inference.Event {
	argsJSON, _ := json.Marshal(part.ToolUse.Input)
	return []inference.Event{
		{
			Kind: inference.EventToolCallStart,
			ToolCall: &inference.ToolCallDelta{
				Index: toolIdx,
				ID:    part.ToolUse.ToolUseID,
				Name:  part.ToolUse.Name,
			},
		},
		{
			Kind: inference.EventToolCallDelta,
			ToolCall: &inference.ToolCallDelta{
				Index:     toolIdx,
				ID:        part.ToolUse.ToolUseID,
				Arguments: string(argsJSON),
			},
		},
		{
			Kind: inference.EventToolCallEnd,
			ToolCall: &inference.ToolCallDelta{
				Index: toolIdx,
				ID:    part.ToolUse.ToolUseID,
			},
		},
	}
}

func decodeBedrockUsage(usage converseUsage) []inference.Event {
	if usage.InputTokens <= 0 && usage.OutputTokens <= 0 {
		return nil
	}
	return []inference.Event{
		{
			Kind: inference.EventUsage,
			Usage: &inference.UsageReport{
				InputTokens:  usage.InputTokens,
				OutputTokens: usage.OutputTokens,
			},
		},
	}
}

func bedrockFinishReason(stopReason string) string {
	switch stopReason {
	case "tool_use":
		return inference.EventReasonToolUse
	case "max_tokens":
		return inference.EventReasonLength
	default:
		return inference.EventReasonStop
	}
}

// DecodeResponseEvent decodes a single event.
func (c *Codec) DecodeResponseEvent(event wire.SSEEvent) ([]inference.Event, error) {
	return c.NewStreamDecoder().Push(event)
}

// DecodeError translates an upstream error body into a canonical error.
func (c *Codec) DecodeError(status int, body []byte) *inference.ErrorInfo {
	var resp struct {
		Message string `json:"message"`
		Type    string `json:"__type"`
	}
	if err := json.Unmarshal(body, &resp); err == nil && resp.Message != "" {
		code := resp.Type
		if code == "" {
			code = http.StatusText(status)
		}
		return &inference.ErrorInfo{
			Code:    code,
			Message: resp.Message,
			Status:  status,
		}
	}

	return &inference.ErrorInfo{
		Code:    http.StatusText(status),
		Message: strings.TrimSpace(string(body)),
		Status:  status,
	}
}

// NewStreamDecoder opens a Bedrock stream session, holding the usage the
// vendor reports until the stream ends.
func (c *Codec) NewStreamDecoder() wire.StreamDecoder {
	return &streamDecoder{
		toolCalls: make(map[int]string),
	}
}
