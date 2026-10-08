// Anthropic response decoding, including streams.
package anthropic

import (
	"encoding/json"
	"fmt"
	"github.com/jonaskahn/relo/internal/inference"
	"net/http"
	"strings"

	"github.com/jonaskahn/relo/internal/adapters/wire"
)

// The events the Messages API streams.
const (
	eventMessageStart      = "message_start"
	eventContentBlockStart = "content_block_start"
	eventContentBlockDelta = "content_block_delta"
	eventContentBlockStop  = "content_block_stop"
	eventMessageDelta      = "message_delta"
	eventMessageStop       = "message_stop"
	eventPing              = "ping"
	eventError             = "error"

	deltaText      = "text_delta"
	deltaThinking  = "thinking_delta"
	deltaSignature = "signature_delta"
	deltaJSON      = "input_json_delta"

	stopEndTurn   = "end_turn"
	stopMaxTokens = "max_tokens"
	stopToolUse   = "tool_use"

	defaultErrorCode = "api_error"
)

type streamDecoder struct {
	calls      map[int]*pendingCall
	order      []int
	usage      inference.UsageReport
	usageSeen  bool
	usageSent  bool
	stopReason string
	sawToolUse bool
	failure    *inference.ErrorInfo
	terminated bool
}

type pendingCall struct {
	index    int
	id       string
	name     string
	input    string
	args     strings.Builder
	streamed bool
	done     bool
}

// NewStreamDecoder returns a decoder for one upstream response.
func (c *Codec) NewStreamDecoder() wire.StreamDecoder {
	return &streamDecoder{calls: map[int]*pendingCall{}}
}

type streamFrame struct {
	Type         string       `json:"type"`
	Index        int          `json:"index"`
	Message      *messageBody `json:"message"`
	ContentBlock *blockBody   `json:"content_block"`
	Delta        *deltaBody   `json:"delta"`
	Usage        *usageBody   `json:"usage"`
	Error        *errorBody   `json:"error"`
}

type messageBody struct {
	ID         string      `json:"id"`
	Type       string      `json:"type"`
	Role       string      `json:"role"`
	Model      string      `json:"model"`
	Content    []blockBody `json:"content"`
	StopReason string      `json:"stop_reason"`
	Usage      *usageBody  `json:"usage"`
	Error      *errorBody  `json:"error"`
}

type blockBody struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	Thinking  string          `json:"thinking"`
	Signature string          `json:"signature"`
	Data      string          `json:"data"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
}

type deltaBody struct {
	Type        string `json:"type"`
	Text        string `json:"text"`
	Thinking    string `json:"thinking"`
	Signature   string `json:"signature"`
	PartialJSON string `json:"partial_json"`
	StopReason  string `json:"stop_reason"`
}

type usageBody struct {
	InputTokens   int `json:"input_tokens"`
	OutputTokens  int `json:"output_tokens"`
	CacheCreation int `json:"cache_creation_input_tokens"`
	CacheRead     int `json:"cache_read_input_tokens"`
}

type errorBody struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// Push translates one upstream frame into canonical events.
func (d *streamDecoder) Push(event wire.SSEEvent) ([]inference.Event, error) {
	if strings.TrimSpace(event.Data) == "" {
		return nil, nil
	}
	var frame streamFrame
	if err := json.Unmarshal([]byte(event.Data), &frame); err != nil {
		return nil, fmt.Errorf("%w: %v", wire.ErrMalformedEvent, err)
	}
	return d.absorb(nameOf(event, &frame), &frame), nil
}

// Finish closes the stream. It always emits exactly one terminal event,
// synthesizing one when the upstream never sent it.
func (d *streamDecoder) Finish() ([]inference.Event, error) {
	if d.terminated {
		return nil, nil
	}
	d.terminated = true
	events := d.toolCallEnds()
	events = append(events, d.usageEvent()...)
	return append(events, terminalEvent(d.terminalReason())), nil
}

func nameOf(event wire.SSEEvent, frame *streamFrame) string {
	if event.Name != "" {
		return event.Name
	}
	return frame.Type
}

func (d *streamDecoder) absorb(name string, frame *streamFrame) []inference.Event {
	switch name {
	case eventMessageStart:
		return d.messageStart(frame.Message)
	case eventContentBlockStart:
		return d.blockStart(frame)
	case eventContentBlockDelta:
		return d.blockDelta(frame)
	case eventContentBlockStop:
		return d.blockStop(frame.Index)
	case eventMessageDelta:
		return d.messageDelta(frame)
	case eventError:
		return d.failureEvent(frame.Error)
	default:
		return nil
	}
}

func (d *streamDecoder) messageStart(message *messageBody) []inference.Event {
	if message == nil {
		return nil
	}
	d.mergeUsage(message.Usage)
	return nil
}

func (d *streamDecoder) messageDelta(frame *streamFrame) []inference.Event {
	if frame.Delta != nil {
		d.stopReason = frame.Delta.StopReason
	}
	d.mergeUsage(frame.Usage)
	return nil
}

func (d *streamDecoder) blockStart(frame *streamFrame) []inference.Event {
	block := frame.ContentBlock
	if block == nil || block.Type != blockToolUse {
		return nil
	}
	call := &pendingCall{index: frame.Index, id: block.ID, name: strippedToolName(block.Name), input: string(block.Input)}
	d.calls[frame.Index] = call
	d.order = append(d.order, frame.Index)
	d.sawToolUse = true
	return []inference.Event{{Kind: inference.EventToolCallStart, ToolCall: call.start()}}
}

func (d *streamDecoder) blockDelta(frame *streamFrame) []inference.Event {
	if frame.Delta == nil {
		return nil
	}
	switch frame.Delta.Type {
	case deltaText:
		return textEvent(frame.Delta.Text)
	case deltaThinking:
		return reasoningEvent("", frame.Delta.Thinking)
	case deltaSignature:
		return reasoningEvent(frame.Delta.Signature, "")
	case deltaJSON:
		return d.argumentDelta(frame)
	default:
		return nil
	}
}

func (d *streamDecoder) argumentDelta(frame *streamFrame) []inference.Event {
	call, found := d.calls[frame.Index]
	if !found || frame.Delta.PartialJSON == "" {
		return nil
	}
	call.streamed = true
	call.args.WriteString(frame.Delta.PartialJSON)
	fragment := &inference.ToolCallDelta{Index: call.index, Arguments: frame.Delta.PartialJSON}
	return []inference.Event{{Kind: inference.EventToolCallDelta, ToolCall: fragment}}
}

func (d *streamDecoder) blockStop(index int) []inference.Event {
	call, found := d.calls[index]
	if !found || call.done {
		return nil
	}
	call.done = true
	return []inference.Event{{Kind: inference.EventToolCallEnd, ToolCall: call.end()}}
}

func (d *streamDecoder) toolCallEnds() []inference.Event {
	var events []inference.Event
	for _, index := range d.order {
		call := d.calls[index]
		if call.done {
			continue
		}
		call.done = true
		events = append(events, inference.Event{Kind: inference.EventToolCallEnd, ToolCall: call.end()})
	}
	return events
}

func (c *pendingCall) start() *inference.ToolCallDelta {
	return &inference.ToolCallDelta{Index: c.index, ID: c.id, Name: c.name}
}

func (c *pendingCall) end() *inference.ToolCallDelta {
	arguments := c.args.String()
	if !c.streamed {
		arguments = c.input
	}
	return &inference.ToolCallDelta{Index: c.index, ID: c.id, Name: c.name, Arguments: arguments}
}

func textEvent(text string) []inference.Event {
	if text == "" {
		return nil
	}
	return []inference.Event{{Kind: inference.EventTextDelta, Text: text}}
}

func reasoningEvent(signature, text string) []inference.Event {
	if text == "" && signature == "" {
		return nil
	}
	event := inference.Event{Kind: inference.EventReasoningDelta, Text: text}
	if signature != "" {
		event.Reasoning = &inference.ReasoningConfig{Signature: signature}
	}
	return []inference.Event{event}
}

func (d *streamDecoder) mergeUsage(usage *usageBody) {
	if usage == nil {
		return
	}
	d.usageSeen = true
	d.usage = inference.UsageReport{
		InputTokens:      newest(d.usage.InputTokens, usage.InputTokens),
		OutputTokens:     newest(d.usage.OutputTokens, usage.OutputTokens),
		CacheReadTokens:  newest(d.usage.CacheReadTokens, usage.CacheRead),
		CacheWriteTokens: newest(d.usage.CacheWriteTokens, usage.CacheCreation),
	}
}

func newest(current, reported int) int {
	if reported > 0 {
		return reported
	}
	return current
}

func (d *streamDecoder) usageEvent() []inference.Event {
	if !d.usageSeen || d.usageSent {
		return nil
	}
	d.usageSent = true
	report := d.usage
	return []inference.Event{{Kind: inference.EventUsage, Usage: &report}}
}

func (d *streamDecoder) failureEvent(failure *errorBody) []inference.Event {
	if failure == nil {
		return nil
	}
	d.failure = failure.info()
	return []inference.Event{{Kind: inference.EventError, Error: d.failure}}
}

func (d *streamDecoder) terminalReason() string {
	switch {
	case d.failure != nil:
		return inference.EventReasonError
	case d.stopReason == stopMaxTokens:
		return inference.EventReasonLength
	case d.stopReason == stopToolUse, d.sawToolUse:
		return inference.EventReasonToolUse
	default:
		return inference.EventReasonStop
	}
}

func (d *streamDecoder) absorbMessage(message *messageBody) []inference.Event {
	var events []inference.Event
	for index := range message.Content {
		events = append(events, d.blockEvents(&message.Content[index], index)...)
	}
	d.mergeUsage(message.Usage)
	d.stopReason = message.StopReason
	return append(events, d.failureEvent(message.Error)...)
}

func (d *streamDecoder) blockEvents(block *blockBody, index int) []inference.Event {
	switch block.Type {
	case blockText:
		return textEvent(block.Text)
	case blockThinking:
		return reasoningEvent(block.Signature, block.Thinking)
	case "redacted_thinking":
		return reasoningEvent(block.Data, "")
	case blockToolUse:
		return d.completeCall(block, index)
	default:
		return nil
	}
}

func (d *streamDecoder) completeCall(block *blockBody, index int) []inference.Event {
	call := &pendingCall{index: index, id: block.ID, name: strippedToolName(block.Name), input: string(block.Input), done: true}
	d.calls[index] = call
	d.order = append(d.order, index)
	d.sawToolUse = true
	return []inference.Event{
		{Kind: inference.EventToolCallStart, ToolCall: call.start()},
		{Kind: inference.EventToolCallEnd, ToolCall: call.end()},
	}
}

func strippedToolName(name string) string {
	rest, found := strings.CutPrefix(name, customToolPrefix)
	if !found || rest == "" {
		return name
	}
	return rest
}

func terminalEvent(reason string) inference.Event {
	return inference.Event{Kind: inference.EventTerminal, Terminal: &inference.TerminalInfo{Reason: reason}}
}

func (e *errorBody) info() *inference.ErrorInfo {
	code := e.Type
	if code == "" {
		code = defaultErrorCode
	}
	return &inference.ErrorInfo{Code: code, Message: e.Message, Status: http.StatusBadGateway}
}
