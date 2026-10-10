// Gemini response decoding, including streams.
package google

import (
	"encoding/json"
	"fmt"
	"github.com/jonaskahn/relo/internal/inference"
	"net/http"
	"strconv"
	"strings"

	"github.com/jonaskahn/relo/internal/adapters/wire"
)

// The finish reasons that mean the model stopped before it answered, and
// the shapes a GenerateContent response arrives in.
const (
	finishStop      = "STOP"
	finishMaxTokens = "MAX_TOKENS"

	safetyErrorCode = "safety"
	callIDPrefix    = "call_"
)

var blockedReasons = map[string]bool{
	"SAFETY":             true,
	"RECITATION":         true,
	"BLOCKLIST":          true,
	"PROHIBITED_CONTENT": true,
	"SPII":               true,
	"IMAGE_SAFETY":       true,
}

type streamDecoder struct {
	// mode names the endpoint shape that produced the frames, so a Cloud Code
	// Assist answer is read through the envelope its internal endpoint wraps
	// every payload in.
	mode         Mode
	replay       replayScope
	carried      string
	finishReason string
	calls        int
	sawCall      bool
	usage        *inference.UsageReport
	usageSent    bool
	responseID   string
	failure      *inference.ErrorInfo
	terminated   bool
}

// NewStreamDecoder returns a decoder for one upstream response.
func (c *Codec) NewStreamDecoder() wire.StreamDecoder {
	return &streamDecoder{mode: c.cfg.Mode, replay: c.replay}
}

type ccaEnvelope struct {
	Response *streamChunk `json:"response"`
}

type streamChunk struct {
	Candidates    []candidate    `json:"candidates"`
	UsageMetadata *usageMetadata `json:"usageMetadata"`
	Error         *errorBody     `json:"error"`
	ResponseID    string         `json:"responseId"`
}

type candidate struct {
	Content       *content       `json:"content"`
	FinishReason  string         `json:"finishReason"`
	SafetyRatings []safetyRating `json:"safetyRatings"`
}

type safetyRating struct {
	Category    string `json:"category"`
	Probability string `json:"probability"`
	Blocked     bool   `json:"blocked"`
}

type usageMetadata struct {
	PromptTokenCount        int `json:"promptTokenCount"`
	CandidatesTokenCount    int `json:"candidatesTokenCount"`
	ThoughtsTokenCount      int `json:"thoughtsTokenCount"`
	CachedContentTokenCount int `json:"cachedContentTokenCount"`
}

type errorEnvelope struct {
	Error *errorBody `json:"error"`
}

type errorBody struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Status  string `json:"status"`
}

// Push translates one upstream frame into canonical events.
func (d *streamDecoder) Push(event wire.SSEEvent) ([]inference.Event, error) {
	if strings.TrimSpace(event.Data) == "" {
		return nil, nil
	}
	var chunk streamChunk
	if err := json.Unmarshal([]byte(event.Data), &chunk); err != nil {
		return nil, fmt.Errorf("%w: %v", wire.ErrMalformedEvent, err)
	}
	chunk = unwrapCloudCode(d.mode, event.Data, chunk)
	return d.absorb(&chunk), nil
}

func unwrapCloudCode(mode Mode, raw string, chunk streamChunk) streamChunk {
	if mode != ModeCloudCodeAssist {
		return chunk
	}
	var envelope ccaEnvelope
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil || envelope.Response == nil {
		return chunk
	}
	wrapped := *envelope.Response
	if chunk.Error != nil && wrapped.Error == nil {
		wrapped.Error = chunk.Error
	}
	return wrapped
}

// Finish closes the stream. It always emits exactly one terminal event,
// synthesizing one when the upstream never reported a finish reason.
func (d *streamDecoder) Finish() ([]inference.Event, error) {
	if d.terminated {
		return nil, nil
	}
	d.terminated = true
	d.rememberExecution()
	events := d.usageEvents()
	terminal := inference.Event{Kind: inference.EventTerminal, Terminal: &inference.TerminalInfo{Reason: d.terminalReason()}}
	return append(events, terminal), nil
}

func (d *streamDecoder) absorb(chunk *streamChunk) []inference.Event {
	var events []inference.Event
	for index := range chunk.Candidates {
		events = append(events, d.candidateEvents(&chunk.Candidates[index])...)
	}
	d.rememberUsage(chunk.UsageMetadata)
	if chunk.ResponseID != "" {
		d.responseID = chunk.ResponseID
	}
	return append(events, d.errorEvents(chunk.Error)...)
}

func (d *streamDecoder) candidateEvents(source *candidate) []inference.Event {
	var events []inference.Event
	if source.Content != nil {
		d.carried = rememberSignatures(d.replay, source.Content.Parts, d.carried)
		events = d.partEvents(source.Content.Parts)
	}
	d.finishReason = source.FinishReason
	return append(events, d.finishEvents(source)...)
}

func (d *streamDecoder) partEvents(parts []part) []inference.Event {
	var events []inference.Event
	for index := range parts {
		signature := usableSignature(parts[index].ThoughtSignature)
		switch {
		case parts[index].FunctionCall != nil:
			events = append(events, d.callEvents(parts[index].FunctionCall)...)
			events = append(events, reasoningEvent("", signature)...)
		case parts[index].Thought:
			events = append(events, reasoningEvent(parts[index].Text, signature)...)
		default:
			events = append(events, textEvent(parts[index].Text)...)
			events = append(events, reasoningEvent("", signature)...)
		}
	}
	return events
}

func (d *streamDecoder) callEvents(call *functionCall) []inference.Event {
	index := d.calls
	d.calls++
	d.sawCall = true
	start := &inference.ToolCallDelta{Index: index, ID: toolCallID(index), Name: call.Name, Arguments: string(call.Args)}
	end := &inference.ToolCallDelta{Index: index, ID: start.ID, Name: start.Name, Arguments: start.Arguments}
	return []inference.Event{
		{Kind: inference.EventToolCallStart, ToolCall: start},
		{Kind: inference.EventToolCallEnd, ToolCall: end},
	}
}

func (d *streamDecoder) finishEvents(source *candidate) []inference.Event {
	if !blockedReasons[source.FinishReason] {
		return nil
	}
	d.failure = &inference.ErrorInfo{
		Code:    safetyErrorCode,
		Message: safetyDetail(source),
		Status:  http.StatusBadGateway,
	}
	return []inference.Event{{Kind: inference.EventError, Error: d.failure}}
}

func safetyDetail(source *candidate) string {
	for _, rating := range source.SafetyRatings {
		if rating.Blocked {
			return "google blocked the response on " + rating.Category + " (" + rating.Probability + ")"
		}
	}
	return "google blocked the response with finish reason " + source.FinishReason
}

func (d *streamDecoder) rememberUsage(usage *usageMetadata) {
	if usage != nil {
		d.usage = usage.report()
	}
}

func (d *streamDecoder) usageEvents() []inference.Event {
	if d.usage == nil || d.usageSent {
		return nil
	}
	d.usageSent = true
	return []inference.Event{{Kind: inference.EventUsage, Usage: d.usage}}
}

func (u *usageMetadata) report() *inference.UsageReport {
	return &inference.UsageReport{
		InputTokens:     u.PromptTokenCount,
		OutputTokens:    u.CandidatesTokenCount + u.ThoughtsTokenCount,
		CacheReadTokens: u.CachedContentTokenCount,
	}
}

func (d *streamDecoder) errorEvents(failure *errorBody) []inference.Event {
	if failure == nil {
		return nil
	}
	d.failure = failure.info(http.StatusBadGateway)
	return []inference.Event{{Kind: inference.EventError, Error: d.failure}}
}

func (d *streamDecoder) terminalReason() string {
	switch {
	case d.failure != nil:
		return inference.EventReasonError
	case d.finishReason == finishMaxTokens:
		return inference.EventReasonLength
	case d.sawCall:
		return inference.EventReasonToolUse
	default:
		return inference.EventReasonStop
	}
}

func textEvent(text string) []inference.Event {
	if text == "" {
		return nil
	}
	return []inference.Event{{Kind: inference.EventTextDelta, Text: text}}
}

func reasoningEvent(text, signature string) []inference.Event {
	if text == "" && signature == "" {
		return nil
	}
	event := inference.Event{Kind: inference.EventReasoningDelta, Text: text}
	if signature != "" {
		event.Reasoning = &inference.ReasoningConfig{Signature: signature}
	}
	return []inference.Event{event}
}

func toolCallID(index int) string {
	return callIDPrefix + strconv.Itoa(index)
}

func (e *errorBody) info(status int) *inference.ErrorInfo {
	if status == 0 {
		status = e.Code
	}
	code := e.Status
	if code == "" {
		code = http.StatusText(status)
	}
	return &inference.ErrorInfo{Code: code, Message: e.Message, Status: status}
}
