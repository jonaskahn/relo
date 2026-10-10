// Chat stream decoding for response-family models.
package openaichat

import (
	"encoding/json"
	"fmt"
	"github.com/jonaskahn/relo/internal/inference"
	"net/http"
	"strings"

	"github.com/jonaskahn/relo/internal/adapters/wire"
)

type streamDecoder struct {
	pending      map[int]*inference.ToolCallDelta
	order        []int
	finishReason string
	failure      *inference.ErrorInfo
	terminated   bool
}

const (
	finishError       = "error"
	upstreamErrorCode = "upstream_error"
)

// NewStreamDecoder returns a decoder for one upstream response.
func (c *Codec) NewStreamDecoder() wire.StreamDecoder {
	return &streamDecoder{pending: map[int]*inference.ToolCallDelta{}}
}

// Push translates one upstream frame into canonical events.
func (d *streamDecoder) Push(event wire.SSEEvent) ([]inference.Event, error) {
	if strings.TrimSpace(event.Data) == "" {
		return nil, nil
	}
	var envelope chatEnvelope
	if err := json.Unmarshal([]byte(event.Data), &envelope); err != nil {
		return nil, fmt.Errorf("decode chat chunk: %w", err)
	}
	if envelope.Error != nil {
		return []inference.Event{failureEvent(envelope.Error, http.StatusBadGateway)}, nil
	}
	return d.absorb(&envelope), nil
}

// Finish closes the stream. It always emits exactly one terminal event,
// synthesizing one when the upstream never sent it.
func (d *streamDecoder) Finish() ([]inference.Event, error) {
	if d.terminated {
		return nil, nil
	}
	d.terminated = true
	terminal := inference.Event{
		Kind:     inference.EventTerminal,
		Terminal: &inference.TerminalInfo{Reason: d.terminalReason()},
	}
	return append(d.toolCallEnds(), terminal), nil
}

func (d *streamDecoder) absorb(envelope *chatEnvelope) []inference.Event {
	var events []inference.Event
	for index := range envelope.Choices {
		events = append(events, d.choiceEvents(&envelope.Choices[index])...)
	}
	if envelope.Usage == nil {
		return events
	}
	report := envelope.Usage.report()
	return append(events, inference.Event{Kind: inference.EventUsage, Usage: report})
}

func (d *streamDecoder) choiceEvents(choice *chatChoice) []inference.Event {
	message := choice.Delta
	if message == nil {
		message = choice.Message
	}
	if message == nil {
		return d.finishEvents(choice.FinishReason)
	}
	events := append(d.textEvents(message), d.toolCallEvents(message.ToolCalls)...)
	if choice.FinishReason == "" {
		return events
	}
	d.finishReason = choice.FinishReason
	return append(events, append(d.finishEvents(choice.FinishReason), d.toolCallEnds()...)...)
}

func (d *streamDecoder) finishEvents(reason string) []inference.Event {
	if reason != finishError {
		return nil
	}
	// Some gateways end an empty stream in-band with finish_reason "error"
	// while keeping HTTP 200, which must surface as an upstream failure
	// rather than a clean stop so the relay fails over instead of logging
	// a successful empty turn.
	d.failure = &inference.ErrorInfo{Code: upstreamErrorCode, Message: `upstream reported finish_reason "error"`, Status: http.StatusBadGateway}
	return []inference.Event{{Kind: inference.EventError, Error: d.failure}}
}

func (d *streamDecoder) textEvents(message *chatMessageBody) []inference.Event {
	var events []inference.Event
	if message.Content != "" {
		events = append(events, inference.Event{Kind: inference.EventTextDelta, Text: message.Content})
	}
	if reasoning := message.reasoningText(); reasoning != "" {
		events = append(events, inference.Event{Kind: inference.EventReasoningDelta, Text: reasoning})
	}
	return events
}

func (d *streamDecoder) toolCallEvents(calls []chatToolCallBody) []inference.Event {
	events := make([]inference.Event, 0, len(calls))
	for _, call := range calls {
		if pending, seen := d.pending[call.Index]; seen {
			pending.Arguments += call.Function.Arguments
			events = append(events, fragmentEvent(call))
			continue
		}
		d.startToolCall(call)
		events = append(events, inference.Event{
			Kind:     inference.EventToolCallStart,
			ToolCall: snapshot(d.pending[call.Index]),
		})
	}
	return events
}

func (d *streamDecoder) startToolCall(call chatToolCallBody) {
	d.pending[call.Index] = &inference.ToolCallDelta{
		Index:     call.Index,
		ID:        call.ID,
		Name:      call.Function.Name,
		Arguments: call.Function.Arguments,
	}
	d.order = append(d.order, call.Index)
}

func (d *streamDecoder) toolCallEnds() []inference.Event {
	if len(d.order) == 0 {
		return nil
	}
	events := make([]inference.Event, 0, len(d.order))
	for _, index := range d.order {
		events = append(events, inference.Event{Kind: inference.EventToolCallEnd, ToolCall: snapshot(d.pending[index])})
	}
	d.pending = map[int]*inference.ToolCallDelta{}
	d.order = nil
	return events
}

func snapshot(delta *inference.ToolCallDelta) *inference.ToolCallDelta {
	copied := *delta
	return &copied
}

func (d *streamDecoder) terminalReason() string {
	switch {
	case d.failure != nil:
		return inference.EventReasonError
	case d.finishReason == "tool_calls" || d.finishReason == "function_call":
		return inference.EventReasonToolUse
	case d.finishReason == "length":
		return inference.EventReasonLength
	default:
		return inference.EventReasonStop
	}
}

func fragmentEvent(call chatToolCallBody) inference.Event {
	return inference.Event{
		Kind: inference.EventToolCallDelta,
		ToolCall: &inference.ToolCallDelta{
			Index:     call.Index,
			Arguments: call.Function.Arguments,
		},
	}
}

func failureEvent(failure *chatErrorBody, status int) inference.Event {
	return inference.Event{Kind: inference.EventError, Error: failure.info(status)}
}

type chatEnvelope struct {
	ID      string         `json:"id"`
	Model   string         `json:"model"`
	Choices []chatChoice   `json:"choices"`
	Usage   *chatUsageBody `json:"usage"`
	Error   *chatErrorBody `json:"error"`
}

type chatChoice struct {
	Index        int              `json:"index"`
	Delta        *chatMessageBody `json:"delta"`
	Message      *chatMessageBody `json:"message"`
	FinishReason string           `json:"finish_reason"`
}

type chatMessageBody struct {
	Content          string             `json:"content"`
	ReasoningContent string             `json:"reasoning_content"`
	Reasoning        string             `json:"reasoning"`
	ToolCalls        []chatToolCallBody `json:"tool_calls"`
}

type chatToolCallBody struct {
	Index    int    `json:"index"`
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type chatUsageBody struct {
	PromptTokens        int `json:"prompt_tokens"`
	CompletionTokens    int `json:"completion_tokens"`
	PromptTokensDetails struct {
		CachedTokens int `json:"cached_tokens"`
	} `json:"prompt_tokens_details"`
}

type chatErrorBody struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code"`
}

func (m *chatMessageBody) reasoningText() string {
	if m.ReasoningContent != "" {
		return m.ReasoningContent
	}
	return m.Reasoning
}

func (u *chatUsageBody) report() *inference.UsageReport {
	return &inference.UsageReport{
		InputTokens:     u.PromptTokens,
		OutputTokens:    u.CompletionTokens,
		CacheReadTokens: u.PromptTokensDetails.CachedTokens,
	}
}

func (e *chatErrorBody) info(status int) *inference.ErrorInfo {
	return &inference.ErrorInfo{Code: e.codeName(status), Message: e.Message, Status: status}
}

func (e *chatErrorBody) codeName(status int) string {
	if e.Code != "" {
		return e.Code
	}
	if e.Type != "" {
		return e.Type
	}
	return http.StatusText(status)
}
