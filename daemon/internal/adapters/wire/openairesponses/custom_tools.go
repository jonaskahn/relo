// Custom tool decoding for the responses surface.
package openairesponses

import (
	"encoding/json"
	"strings"

	"github.com/jonaskahn/relo/internal/adapters/wire"
	"github.com/jonaskahn/relo/internal/inference"
)

const customInputKey = "input"

const (
	emptyObjectSchema = `{"type":"object","properties":{}}`
	customInputSchema = `{"type":"object","properties":{"input":{"type":"string"}},"required":["input"]}`
)

func decodeTools(tools []toolDef) ([]inference.Tool, map[string]bool) {
	decoded := make([]inference.Tool, 0, len(tools))
	custom := map[string]bool{}
	for _, tool := range tools {
		if tool.Name == "" {
			continue
		}
		switch tool.Type {
		case "", toolTypeFunction:
			decoded = append(decoded, inference.Tool{
				Name: tool.Name, Description: tool.Description, Parameters: schemaOrEmpty(tool.Parameters),
			})
		case toolTypeCustom:
			custom[tool.Name] = true
			decoded = append(decoded, inference.Tool{
				Name: tool.Name, Description: tool.Description, Parameters: json.RawMessage(customInputSchema),
			})
		}
	}
	return decoded, custom
}

// decodeToolChoice reads the Responses wire's tool choice, which is either one
// of its own names or an object naming a tool. A shape this wire does not
// define leaves the choice unset rather than guessing at what the client meant.
func decodeToolChoice(raw json.RawMessage) *inference.ToolChoice {
	var name string
	if err := json.Unmarshal(raw, &name); err == nil {
		return namedToolChoice(name)
	}
	var selector struct {
		Type string `json:"type"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(raw, &selector); err != nil {
		return nil
	}
	if selector.Type == toolTypeFunction {
		return &inference.ToolChoice{Mode: inference.ToolChoiceTool, Name: selector.Name}
	}
	return namedToolChoice(selector.Type)
}

func namedToolChoice(name string) *inference.ToolChoice {
	switch name {
	case "":
		return nil
	case inference.ToolChoiceAuto, inference.ToolChoiceNone, inference.ToolChoiceRequired:
		return &inference.ToolChoice{Mode: name}
	default:
		return &inference.ToolChoice{Mode: inference.ToolChoiceTool, Name: name}
	}
}

func schemaOrEmpty(schema json.RawMessage) json.RawMessage {
	if len(schema) == 0 || string(schema) == "null" {
		return json.RawMessage(emptyObjectSchema)
	}
	return schema
}

func customArguments(input string) string {
	encoded, err := json.Marshal(map[string]string{customInputKey: input})
	if err != nil {
		return "{}"
	}
	return string(encoded)
}

func customInput(arguments string) string {
	var wrapped map[string]any
	if err := json.Unmarshal([]byte(arguments), &wrapped); err != nil {
		return arguments
	}
	if text, ok := wrapped[customInputKey].(string); ok {
		return text
	}
	return arguments
}

func (i *Inbound) customCallItem(call *callState, status string) map[string]any {
	item := map[string]any{
		"type":    itemCustomToolCall,
		"id":      call.id,
		"call_id": call.id,
		"name":    call.name,
		"input":   "",
		"status":  status,
	}
	if status == statusCompleted {
		item["input"] = customInput(call.args.String())
	}
	return item
}

func (i *Inbound) customEndFrames(call *callState) []wire.SSEEvent {
	item := i.customCallItem(call, statusCompleted)
	i.items[call.index] = item
	input, _ := item["input"].(string)
	frames := i.startedFrames()
	if input != "" {
		frames = append(frames, i.frame(eventCustomInputDelta, map[string]any{
			"item_id": call.id, "output_index": call.index, "delta": input,
		})...)
	}
	frames = append(frames, i.frame(eventCustomInputDone, map[string]any{
		"item_id": call.id, "output_index": call.index, "input": input,
	})...)
	return append(frames, i.frame(eventOutputItemDone, map[string]any{
		"output_index": call.index, "item": item,
	})...)
}

type toolOutput string

// UnmarshalJSON accepts either shape a client sends a result in.
func (o *toolOutput) UnmarshalJSON(data []byte) error {
	var text string
	if err := json.Unmarshal(data, &text); err == nil {
		*o = toolOutput(text)
		return nil
	}
	var parts []contentItem
	if err := json.Unmarshal(data, &parts); err != nil {
		return err
	}
	var joined strings.Builder
	for _, part := range parts {
		joined.WriteString(part.Text)
	}
	*o = toolOutput(joined.String())
	return nil
}
