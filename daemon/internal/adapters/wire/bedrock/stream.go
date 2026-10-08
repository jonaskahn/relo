// Bedrock stream decoding into canonical events.
package bedrock

import (
	"encoding/json"
	"github.com/jonaskahn/relo/internal/inference"

	"github.com/jonaskahn/relo/internal/adapters/wire"
)

type streamDecoder struct {
	toolCalls map[int]string
	usage     *inference.UsageReport
	finished  bool
}

type streamChunk struct {
	ContentBlockStart *struct {
		Start *struct {
			ToolUse *struct {
				ToolUseID string `json:"toolUseId"`
				Name      string `json:"name"`
			} `json:"toolUse,omitempty"`
		} `json:"start,omitempty"`
		ContentBlockIndex int `json:"contentBlockIndex"`
	} `json:"contentBlockStart,omitempty"`
	ContentBlockDelta *struct {
		Delta *struct {
			Text    string `json:"text,omitempty"`
			ToolUse *struct {
				Input string `json:"input,omitempty"`
			} `json:"toolUse,omitempty"`
		} `json:"delta,omitempty"`
		ContentBlockIndex int `json:"contentBlockIndex"`
	} `json:"contentBlockDelta,omitempty"`
	ContentBlockStop *struct {
		ContentBlockIndex int `json:"contentBlockIndex"`
	} `json:"contentBlockStop,omitempty"`
	MessageStop *struct {
		StopReason string `json:"stopReason"`
	} `json:"messageStop,omitempty"`
	Metadata *struct {
		Usage *struct {
			InputTokens  int `json:"inputTokens"`
			OutputTokens int `json:"outputTokens"`
		} `json:"usage,omitempty"`
	} `json:"metadata,omitempty"`
}

// Push translates one Bedrock stream frame into canonical events, skipping
// heartbeats and undecodable frames the relay must not fail on.
func (s *streamDecoder) Push(event wire.SSEEvent) ([]inference.Event, error) {
	if event.Data == "" || event.Data == "[DONE]" {
		return nil, nil
	}

	var chunk streamChunk
	if err := json.Unmarshal([]byte(event.Data), &chunk); err != nil {
		return nil, nil
	}

	var events []inference.Event
	events = append(events, s.pushBlockStart(chunk)...)
	events = append(events, s.pushBlockDelta(chunk)...)
	events = append(events, s.pushBlockStop(chunk)...)
	events = append(events, s.pushStreamUsage(chunk)...)
	events = append(events, s.pushMessageStop(chunk)...)

	return events, nil
}

func (s *streamDecoder) pushBlockStart(chunk streamChunk) []inference.Event {
	if chunk.ContentBlockStart == nil || chunk.ContentBlockStart.Start == nil {
		return nil
	}
	if tu := chunk.ContentBlockStart.Start.ToolUse; tu != nil {
		idx := chunk.ContentBlockStart.ContentBlockIndex
		s.toolCalls[idx] = tu.ToolUseID
		return []inference.Event{
			{
				Kind: inference.EventToolCallStart,
				ToolCall: &inference.ToolCallDelta{
					Index: idx,
					ID:    tu.ToolUseID,
					Name:  tu.Name,
				},
			},
		}
	}
	return nil
}

func (s *streamDecoder) pushBlockDelta(chunk streamChunk) []inference.Event {
	if chunk.ContentBlockDelta == nil || chunk.ContentBlockDelta.Delta == nil {
		return nil
	}
	var events []inference.Event
	delta := chunk.ContentBlockDelta.Delta
	idx := chunk.ContentBlockDelta.ContentBlockIndex
	if delta.Text != "" {
		events = append(events, inference.Event{
			Kind: inference.EventTextDelta,
			Text: delta.Text,
		})
	}
	if delta.ToolUse != nil && delta.ToolUse.Input != "" {
		id := s.toolCalls[idx]
		events = append(events, inference.Event{
			Kind: inference.EventToolCallDelta,
			ToolCall: &inference.ToolCallDelta{
				Index:     idx,
				ID:        id,
				Arguments: delta.ToolUse.Input,
			},
		})
	}
	return events
}

func (s *streamDecoder) pushBlockStop(chunk streamChunk) []inference.Event {
	if chunk.ContentBlockStop == nil {
		return nil
	}
	idx := chunk.ContentBlockStop.ContentBlockIndex
	if id, ok := s.toolCalls[idx]; ok {
		return []inference.Event{
			{
				Kind: inference.EventToolCallEnd,
				ToolCall: &inference.ToolCallDelta{
					Index: idx,
					ID:    id,
				},
			},
		}
	}
	return nil
}

func (s *streamDecoder) pushStreamUsage(chunk streamChunk) []inference.Event {
	if chunk.Metadata == nil || chunk.Metadata.Usage == nil {
		return nil
	}
	s.usage = &inference.UsageReport{
		InputTokens:  chunk.Metadata.Usage.InputTokens,
		OutputTokens: chunk.Metadata.Usage.OutputTokens,
	}
	return []inference.Event{
		{
			Kind:  inference.EventUsage,
			Usage: s.usage,
		},
	}
}

func (s *streamDecoder) pushMessageStop(chunk streamChunk) []inference.Event {
	if chunk.MessageStop == nil {
		return nil
	}
	s.finished = true
	return []inference.Event{
		{
			Kind: inference.EventTerminal,
			Terminal: &inference.TerminalInfo{
				Reason: bedrockFinishReason(chunk.MessageStop.StopReason),
			},
		},
	}
}

// Finish emits the usage the stream carried, once, when the vendor ends it.
func (s *streamDecoder) Finish() ([]inference.Event, error) {
	if s.finished {
		return nil, nil
	}
	s.finished = true
	var events []inference.Event
	if s.usage != nil {
		events = append(events, inference.Event{
			Kind:  inference.EventUsage,
			Usage: s.usage,
		})
	}
	events = append(events, inference.Event{
		Kind: inference.EventTerminal,
		Terminal: &inference.TerminalInfo{
			Reason: inference.EventReasonStop,
		},
	})
	return events, nil
}
