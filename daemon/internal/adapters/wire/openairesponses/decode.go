// Responses decoding, including streams.
package openairesponses

import (
	"encoding/json"
	"fmt"
	"github.com/jonaskahn/relo/internal/inference"
	"net/http"
	"strings"

	"github.com/jonaskahn/relo/internal/adapters/wire"
)

// The events the Responses API streams. Decoding prefers the SSE event
// field and falls back to the type inside the payload.
const (
	eventCreated            = "response.created"
	eventInProgress         = "response.in_progress"
	eventOutputTextDelta    = "response.output_text.delta"
	eventReasoningDelta     = "response.reasoning_summary_text.delta"
	eventReasoningPartAdded = "response.reasoning_summary_part.added"
	eventReasoningTextDone  = "response.reasoning_summary_text.done"
	eventReasoningPartDone  = "response.reasoning_summary_part.done"
	eventOutputItemAdded    = "response.output_item.added"
	eventOutputItemDone     = "response.output_item.done"
	eventArgumentsDelta     = "response.function_call_arguments.delta"
	eventArgumentsDone      = "response.function_call_arguments.done"
	eventCustomInputDelta   = "response.custom_tool_call_input.delta"
	eventCustomInputDone    = "response.custom_tool_call_input.done"
	eventCompleted          = "response.completed"
	eventIncomplete         = "response.incomplete"
	eventFailed             = "response.failed"
	eventError              = "error"

	statusCompleted  = "completed"
	statusIncomplete = "incomplete"
	statusFailed     = "failed"
	reasonMaxOutput  = "max_output_tokens"
)

type streamDecoder struct {
	calls      map[string]*pendingCall
	order      []string
	nextIndex  int
	sawToolUse bool
	usage      *inference.UsageReport
	usageSent  bool
	failure    *inference.ErrorInfo
	truncated  bool
	terminated bool
}

type pendingCall struct {
	key   string
	index int
	id    string
	name  string
	args  strings.Builder
	done  bool
}

// NewStreamDecoder returns a decoder for one upstream response.
func (c *Codec) NewStreamDecoder() wire.StreamDecoder {
	return &streamDecoder{calls: map[string]*pendingCall{}}
}

type streamFrame struct {
	Type      string        `json:"type"`
	Delta     string        `json:"delta"`
	ItemID    string        `json:"item_id"`
	Arguments string        `json:"arguments"`
	Item      *inputItem    `json:"item"`
	Response  *responseBody `json:"response"`
	Error     *errorBody    `json:"error"`
	Code      string        `json:"code"`
	Message   string        `json:"message"`
}

func (f *streamFrame) failure() *errorBody {
	if f.Error != nil {
		return f.Error
	}
	if f.Code == "" && f.Message == "" {
		return nil
	}
	return &errorBody{Code: f.Code, Message: f.Message}
}

type responseBody struct {
	ID                string          `json:"id"`
	Model             string          `json:"model"`
	Status            string          `json:"status"`
	Output            []inputItem     `json:"output"`
	Usage             *usageBody      `json:"usage"`
	Error             *errorBody      `json:"error"`
	IncompleteDetails *incompleteBody `json:"incomplete_details"`
}

type incompleteBody struct {
	Reason string `json:"reason"`
}

type usageBody struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	InputDetails struct {
		CachedTokens int `json:"cached_tokens"`
	} `json:"input_tokens_details"`
}

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Type    string `json:"type"`
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
	terminal := inference.Event{Kind: inference.EventTerminal, Terminal: &inference.TerminalInfo{Reason: d.terminalReason()}}
	return append(events, terminal), nil
}

func nameOf(event wire.SSEEvent, frame *streamFrame) string {
	if event.Name != "" {
		return event.Name
	}
	return frame.Type
}

func (d *streamDecoder) absorb(name string, frame *streamFrame) []inference.Event {
	switch name {
	case eventOutputTextDelta:
		return textEvent(frame.Delta)
	case eventReasoningDelta:
		return reasoningTextEvent(frame.Delta)
	case eventOutputItemAdded:
		return d.itemAdded(frame.Item)
	case eventArgumentsDelta:
		return d.argumentDelta(frame)
	case eventArgumentsDone:
		return d.argumentDone(frame)
	case eventOutputItemDone:
		return d.itemDone(frame.Item)
	case eventCompleted:
		return d.completed(frame.Response, false)
	case eventIncomplete:
		return d.completed(frame.Response, true)
	case eventFailed:
		return d.failed(frame.Response)
	case eventError:
		return d.failureEvent(frame.failure())
	default:
		return nil
	}
}

func textEvent(text string) []inference.Event {
	if text == "" {
		return nil
	}
	return []inference.Event{{Kind: inference.EventTextDelta, Text: text}}
}

func reasoningTextEvent(text string) []inference.Event {
	if text == "" {
		return nil
	}
	return []inference.Event{{Kind: inference.EventReasoningDelta, Text: text}}
}

func (d *streamDecoder) itemAdded(item *inputItem) []inference.Event {
	if item == nil || item.Type != itemFunctionCall {
		return nil
	}
	return []inference.Event{{Kind: inference.EventToolCallStart, ToolCall: d.call(item).snapshot()}}
}

func (d *streamDecoder) argumentDelta(frame *streamFrame) []inference.Event {
	call, found := d.calls[frame.ItemID]
	if !found {
		return nil
	}
	call.args.WriteString(frame.Delta)
	return []inference.Event{{
		Kind:     inference.EventToolCallDelta,
		ToolCall: &inference.ToolCallDelta{Index: call.index, Arguments: frame.Delta},
	}}
}

func (d *streamDecoder) argumentDone(frame *streamFrame) []inference.Event {
	call, found := d.calls[frame.ItemID]
	if !found || frame.Arguments == "" {
		return nil
	}
	call.args.Reset()
	call.args.WriteString(frame.Arguments)
	return nil
}

func (d *streamDecoder) itemDone(item *inputItem) []inference.Event {
	if item == nil {
		return nil
	}
	if item.Type == itemReasoning {
		return encryptedReasoning(item)
	}
	if item.Type != itemFunctionCall {
		return nil
	}
	call := d.call(item)
	call.done = true
	return []inference.Event{{Kind: inference.EventToolCallEnd, ToolCall: call.snapshot()}}
}

func (d *streamDecoder) call(item *inputItem) *pendingCall {
	key := itemKey(item)
	if existing, found := d.calls[key]; found {
		existing.absorb(item)
		return existing
	}
	call := &pendingCall{key: key, index: d.nextIndex, id: callID(item), name: item.Name}
	call.args.WriteString(item.Arguments)
	d.nextIndex++
	d.sawToolUse = true
	d.calls[key] = call
	d.order = append(d.order, key)
	return call
}

func (c *pendingCall) absorb(item *inputItem) {
	if item.Name != "" {
		c.name = item.Name
	}
	if item.Arguments != "" {
		c.args.Reset()
		c.args.WriteString(item.Arguments)
	}
}

func (c *pendingCall) snapshot() *inference.ToolCallDelta {
	return &inference.ToolCallDelta{Index: c.index, ID: c.id, Name: c.name, Arguments: c.args.String()}
}

func itemKey(item *inputItem) string {
	if item.ID != "" {
		return item.ID
	}
	return item.CallID
}

func callID(item *inputItem) string {
	if item.CallID != "" {
		return item.CallID
	}
	return item.ID
}

func (d *streamDecoder) toolCallEnds() []inference.Event {
	var events []inference.Event
	for _, key := range d.order {
		call := d.calls[key]
		if call.done {
			continue
		}
		call.done = true
		events = append(events, inference.Event{Kind: inference.EventToolCallEnd, ToolCall: call.snapshot()})
	}
	return events
}

func encryptedReasoning(item *inputItem) []inference.Event {
	if item.EncryptedContent == "" {
		return nil
	}
	return []inference.Event{{
		Kind:      inference.EventReasoningDelta,
		Reasoning: &inference.ReasoningConfig{ID: item.ID, EncryptedContent: []byte(item.EncryptedContent)},
	}}
}

func (d *streamDecoder) completed(response *responseBody, incomplete bool) []inference.Event {
	if response == nil {
		return nil
	}
	d.usage = reportOf(response.Usage)
	if incomplete || response.Status == statusIncomplete || incompleteReason(response) == reasonMaxOutput {
		d.truncated = true
	}
	return d.usageEvent()
}

func (d *streamDecoder) failed(response *responseBody) []inference.Event {
	if response == nil || response.Error == nil {
		return nil
	}
	d.usage = reportOf(response.Usage)
	return append(d.usageEvent(), d.failureEvent(response.Error)...)
}

func (d *streamDecoder) failureEvent(failure *errorBody) []inference.Event {
	if failure == nil {
		return nil
	}
	d.failure = failure.info(http.StatusBadGateway)
	return []inference.Event{{Kind: inference.EventError, Error: d.failure}}
}

func (d *streamDecoder) usageEvent() []inference.Event {
	if d.usage == nil || d.usageSent {
		return nil
	}
	d.usageSent = true
	return []inference.Event{{Kind: inference.EventUsage, Usage: d.usage}}
}

func (d *streamDecoder) terminalReason() string {
	switch {
	case d.failure != nil:
		return inference.EventReasonError
	case d.truncated:
		return inference.EventReasonLength
	case d.sawToolUse:
		return inference.EventReasonToolUse
	default:
		return inference.EventReasonStop
	}
}

func (d *streamDecoder) absorbResponse(response *responseBody) []inference.Event {
	var events []inference.Event
	for index := range response.Output {
		events = append(events, d.outputEvents(&response.Output[index])...)
	}
	d.usage = reportOf(response.Usage)
	events = append(events, d.usageEvent()...)
	if response.Status == statusIncomplete || incompleteReason(response) == reasonMaxOutput {
		d.truncated = true
	}
	return append(events, d.failureEvent(response.Error)...)
}

func (d *streamDecoder) outputEvents(item *inputItem) []inference.Event {
	switch item.Type {
	case itemFunctionCall:
		return d.completeCall(item)
	case itemReasoning:
		return reasoningItem(item)
	case itemMessage:
		return messageEvents(item)
	default:
		return nil
	}
}

func (d *streamDecoder) completeCall(item *inputItem) []inference.Event {
	call := d.call(item)
	call.done = true
	return []inference.Event{
		{Kind: inference.EventToolCallStart, ToolCall: call.snapshot()},
		{Kind: inference.EventToolCallEnd, ToolCall: call.snapshot()},
	}
}

func messageEvents(item *inputItem) []inference.Event {
	var events []inference.Event
	for _, part := range item.Content {
		if part.Type == contentOutputText {
			events = append(events, textEvent(part.Text)...)
		}
	}
	return events
}

func reasoningItem(item *inputItem) []inference.Event {
	var events []inference.Event
	for _, part := range item.Summary {
		events = append(events, reasoningTextEvent(part.Text)...)
	}
	return append(events, encryptedReasoning(item)...)
}

func incompleteReason(response *responseBody) string {
	if response.IncompleteDetails == nil {
		return ""
	}
	return response.IncompleteDetails.Reason
}

func reportOf(usage *usageBody) *inference.UsageReport {
	if usage == nil {
		return nil
	}
	return &inference.UsageReport{
		InputTokens:     usage.InputTokens,
		OutputTokens:    usage.OutputTokens,
		CacheReadTokens: usage.InputDetails.CachedTokens,
	}
}

func (e *errorBody) info(status int) *inference.ErrorInfo {
	return &inference.ErrorInfo{Code: e.codeName(status), Message: e.Message, Status: status}
}

func (e *errorBody) codeName(status int) string {
	if e.Code != "" {
		return e.Code
	}
	if e.Type != "" {
		return e.Type
	}
	return http.StatusText(status)
}
