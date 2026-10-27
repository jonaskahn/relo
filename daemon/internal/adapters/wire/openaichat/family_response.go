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
	// reasoningTail holds the end of the reasoning seen so far, so a marker
	// the stream split across two deltas is still read.
	reasoningTail string
	// sawToolCall records a real tool_calls delta, which clears a marker: the
	// fault is a call that never arrived, not the text around it.
	sawToolCall bool
	// leakedToolCall records template text that means the model formatted a
	// call as prose.
	leakedToolCall bool
	// textTail holds a trailing partial tag, so a template that a delta split
	// is not forwarded as prose before it is recognised.
	textTail string
	// inTemplate reports that content is inside a template call being
	// suppressed rather than streamed.
	inTemplate bool
	// templateBuf accumulates the suppressed template call until its close.
	templateBuf strings.Builder
	// leakedCall is the call recovered from template text, if the turn wrote
	// one. An empty structured call is repaired from it.
	leakedCall *templateCall
}

// templateCall is a tool call the serving template wrote as text instead of
// through structured tool_calls.
type templateCall struct {
	name string
	body string
}

const (
	finishError       = "error"
	upstreamErrorCode = "upstream_error"

	// leakedToolCallCode names a turn that meant to call a tool but wrote the
	// call as template text instead. The client has no call to run, so the
	// turn ends as a plain stop that would otherwise read as a success.
	leakedToolCallCode = "leaked_tool_call"

	// leakedToolCallMarker is the tag the serving chat template writes around
	// a call parameter. A real tool_call clears it; without one it is the only
	// trace of the call the model never made.
	leakedToolCallMarker = "rfl:parameter"

	// leakedToolCallTail keeps enough reasoning to span a marker split across
	// two deltas, since where a chunk splits is the stream's choice, not the
	// model's.
	leakedToolCallTail = len(leakedToolCallMarker) - 1

	// The Qwen / MiMo family renders a call as text: a <tool_call> wrapper
	// with a <function=NAME>...</function> body closed by </parameter>.
	templateCallOpen       = "<tool_call>"
	templateCallClose      = "</tool_call>"
	templateFunctionOpen   = "<function="
	templateFunctionClose  = "</function>"
	templateParameterClose = "</parameter>"
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
	events := d.flushText()
	events = append(events, d.leakedToolCallEvents()...)
	events = append(events, d.toolCallEnds()...)
	terminal := inference.Event{
		Kind:     inference.EventTerminal,
		Terminal: &inference.TerminalInfo{Reason: d.terminalReason()},
	}
	return append(events, terminal), nil
}

// flushText releases text held for a possible template tag. A stream that
// ended inside an unclosed template span is not a call the client can run, so
// it is dropped and left to the leak guard to fail the turn over.
func (d *streamDecoder) flushText() []inference.Event {
	if d.inTemplate {
		d.templateBuf.Reset()
		d.inTemplate = false
		d.leakedToolCall = true
		return nil
	}
	if d.textTail == "" {
		return nil
	}
	tail := d.textTail
	d.textTail = ""
	return []inference.Event{{Kind: inference.EventTextDelta, Text: tail}}
}

// leakedToolCallEvents refuses a turn that formatted its tool call as text.
// Only a turn that called nothing is refused: the marker beside a real call is
// template noise, but alone it is a stop the client cannot act on. The answer
// has already reached the client by now, so this reports the fault instead of
// replaying a partial stream.
func (d *streamDecoder) leakedToolCallEvents() []inference.Event {
	if !d.leakedToolCall || d.sawToolCall || d.failure != nil {
		return nil
	}
	d.failure = &inference.ErrorInfo{
		Code:    leakedToolCallCode,
		Message: "the endpoint sent a tool call as text instead of calling it",
		Status:  http.StatusBadGateway,
	}
	return []inference.Event{{Kind: inference.EventError, Error: d.failure}}
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
		events = append(events, d.contentEvents(message.Content)...)
	}
	if reasoning := message.reasoningText(); reasoning != "" {
		d.observeReasoning(reasoning)
		events = append(events, inference.Event{Kind: inference.EventReasoningDelta, Text: reasoning})
	}
	return events
}

// contentEvents feeds assistant content through the template-call parser. A
// call the serving template wrote as text is recovered and suppressed rather
// than streamed as prose; everything around it becomes text as usual. A
// partial tag at the end of a delta is held back until the next delta
// completes or refutes it.
func (d *streamDecoder) contentEvents(delta string) []inference.Event {
	var events []inference.Event
	if d.textTail != "" {
		delta = d.textTail + delta
		d.textTail = ""
	}
	for {
		if d.inTemplate {
			var consumed bool
			events, delta, consumed = d.consumeTemplateBody(delta, events)
			if !consumed {
				return events
			}
			continue
		}
		openAt := strings.Index(delta, templateCallOpen)
		if openAt < 0 {
			return d.flushPlain(delta, events)
		}
		if before := delta[:openAt]; before != "" {
			events = append(events, inference.Event{Kind: inference.EventTextDelta, Text: before})
		}
		d.inTemplate = true
		d.templateBuf.Reset()
		delta = delta[openAt+len(templateCallOpen):]
	}
}

// consumeTemplateBody feeds one delta to an open template span. It reports
// whether the span closed, in which case the caller keeps parsing what
// followed.
func (d *streamDecoder) consumeTemplateBody(delta string, events []inference.Event) ([]inference.Event, string, bool) {
	closeAt := strings.Index(delta, templateCallClose)
	if closeAt < 0 {
		d.templateBuf.WriteString(delta)
		return events, delta, false
	}
	d.templateBuf.WriteString(delta[:closeAt])
	events = append(events, d.closeTemplate()...)
	return events, delta[closeAt+len(templateCallClose):], true
}

// flushPlain forwards the text before a possible open tag, holding back a
// trailing partial tag for the next delta.
func (d *streamDecoder) flushPlain(delta string, events []inference.Event) []inference.Event {
	hold := partialPrefix(delta, templateCallOpen)
	if flush := delta[:len(delta)-len(hold)]; flush != "" {
		events = append(events, inference.Event{Kind: inference.EventTextDelta, Text: flush})
	}
	d.textTail = delta[len(delta)-len(hold):]
	return events
}

// closeTemplate ends a suppressed template span. A span that parses as a call
// is recovered; anything else was prose and is forwarded unchanged.
func (d *streamDecoder) closeTemplate() []inference.Event {
	d.inTemplate = false
	body := d.templateBuf.String()
	d.templateBuf.Reset()
	call, ok := parseTemplateCall(body)
	if !ok {
		return []inference.Event{{Kind: inference.EventTextDelta, Text: templateCallOpen + body + templateCallClose}}
	}
	d.leakedCall = call
	d.leakedToolCall = true
	return nil
}

// parseTemplateCall reads the Qwen / MiMo family literal
// (<function=NAME>BODY</parameter>) into its call. It refuses a span without
// both a name and a body, so prose that merely mentions the tags is not
// mistaken for a call.
func parseTemplateCall(body string) (*templateCall, bool) {
	functionAt := strings.Index(body, templateFunctionOpen)
	if functionAt < 0 {
		return nil, false
	}
	rest := body[functionAt+len(templateFunctionOpen):]
	nameEnd := strings.IndexAny(rest, ">\n")
	if nameEnd <= 0 {
		return nil, false
	}
	name := strings.TrimSpace(rest[:nameEnd])
	afterName := strings.TrimPrefix(rest[nameEnd:], ">")
	bodyEnd := strings.Index(afterName, templateParameterClose)
	if bodyEnd < 0 {
		closeAt := strings.Index(afterName, templateFunctionClose)
		if closeAt < 0 {
			return nil, false
		}
		bodyEnd = closeAt
	}
	callBody := afterName[:bodyEnd]
	if name == "" || callBody == "" {
		return nil, false
	}
	return &templateCall{name: name, body: callBody}, true
}

// partialPrefix returns the longest suffix of text that is a prefix of
// marker, so a tag split across two deltas is never flushed early.
func partialPrefix(text, marker string) string {
	for size := min(len(text), len(marker)-1); size > 0; size-- {
		if strings.HasPrefix(marker, text[len(text)-size:]) {
			return text[len(text)-size:]
		}
	}
	return ""
}

// observeReasoning reads one reasoning delta for the leaked call marker,
// keeping only the tail that a later delta could complete.
func (d *streamDecoder) observeReasoning(delta string) {
	window := d.reasoningTail + delta
	if strings.Contains(window, leakedToolCallMarker) {
		d.leakedToolCall = true
	}
	if len(window) > leakedToolCallTail {
		d.reasoningTail = window[len(window)-leakedToolCallTail:]
	}
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
	d.sawToolCall = true
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
		call := snapshot(d.pending[index])
		d.repairArguments(call)
		events = append(events, inference.Event{Kind: inference.EventToolCallEnd, ToolCall: call})
	}
	d.pending = map[int]*inference.ToolCallDelta{}
	d.order = nil
	return events
}

// repairArguments replaces an empty structured call's arguments with the body
// the serving template wrote as text. Only a call whose name matches the
// recovered one is repaired, and only when its own arguments are empty, so a
// real call is never overwritten.
func (d *streamDecoder) repairArguments(call *inference.ToolCallDelta) {
	if d.leakedCall == nil || call.Name != d.leakedCall.name {
		return
	}
	if !emptyArguments(call.Arguments) {
		return
	}
	call.Arguments = customArguments(d.leakedCall.body)
}

// emptyArguments reports whether a call's accumulated arguments carry no
// body: an empty string, or an object with no fields.
func emptyArguments(arguments string) bool {
	trimmed := strings.TrimSpace(arguments)
	if trimmed == "" {
		return true
	}
	var object map[string]any
	if err := json.Unmarshal([]byte(trimmed), &object); err != nil {
		return false
	}
	return len(object) == 0
}

// customArguments wraps a recovered freeform body the way the Responses
// surface reads it back: the freeform input as a string field. The canonical
// call carries this shape, and the responses inbound unwraps it unchanged.
func customArguments(input string) string {
	encoded, err := json.Marshal(map[string]string{"input": input})
	if err != nil {
		return "{}"
	}
	return string(encoded)
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
