// Anthropic inbound codec: client requests into canonical form.
package anthropic

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/jonaskahn/relo/internal/inference"
	"io"
	"net/http"
	"strings"

	"github.com/jonaskahn/relo/internal/adapters/wire"
)

const (
	maxRequestBytes  = 32 << 20
	messageObject    = "message"
	redactedThinking = "redacted_thinking"
	dataURLPrefix    = "data:"
	dataURLBase64    = ";base64,"
	messageIDBytes   = 16
	streamErrorCode  = "stream_error"
)

// Inbound translates the Messages surface Claude Code speaks into canonical
// form and back. One instance serves one request.
type Inbound struct {
	suffix     string
	model      string
	seq        int
	started    bool
	blockIndex int
	block      *openBlock
	blocks     []map[string]any
	usage      *inference.UsageReport
	failure    *inference.ErrorInfo
	reason     string
}

type openBlock struct {
	index     int
	kind      string
	id        string
	name      string
	text      strings.Builder
	signature string
	args      strings.Builder
}

var _ wire.InboundCodec = (*Inbound)(nil)

// NewInbound returns an inbound codec bound to a fresh message identity.
func NewInbound() *Inbound {
	return &Inbound{suffix: newMessageSuffix(), blocks: []map[string]any{}}
}

// DecodeRequest parses a client Messages request into canonical form.
func (i *Inbound) DecodeRequest(r *http.Request) (*inference.Request, error) {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxRequestBytes))
	if err != nil {
		return nil, fmt.Errorf("read messages request: %w", err)
	}
	var request inboundRequest
	if err := json.Unmarshal(body, &request); err != nil {
		return nil, fmt.Errorf("%w: %v", wire.ErrInvalidRequest, err)
	}
	canonical, err := request.toCanonical()
	if err != nil {
		return nil, err
	}
	i.model = canonical.Model
	return canonical, nil
}

// EncodeResponseEvent renders one canonical event as Messages SSE frames.
func (i *Inbound) EncodeResponseEvent(event inference.Event) ([]wire.SSEEvent, error) {
	switch event.Kind {
	case inference.EventTextDelta:
		return i.textFrames(event.Text), nil
	case inference.EventReasoningDelta:
		return i.reasoningFrames(event), nil
	case inference.EventToolCallStart:
		return i.toolStartFrames(event.ToolCall), nil
	case inference.EventToolCallDelta:
		return i.toolDeltaFrames(event.ToolCall), nil
	case inference.EventToolCallEnd:
		return i.toolEndFrames(event.ToolCall), nil
	case inference.EventUsage:
		i.usage = event.Usage
		return nil, nil
	case inference.EventTerminal:
		return i.terminalFrames(event.Terminal), nil
	case inference.EventError:
		i.failure = event.Error
		return i.failureFrames(event.Error), nil
	default:
		return nil, fmt.Errorf("%w: %s", wire.ErrUnknownEvent, event.Kind)
	}
}

// EncodeResponse renders a completed message, the shape a client that asked
// for no stream expects.
func (i *Inbound) EncodeResponse(events []inference.Event) ([]byte, error) {
	for _, event := range events {
		if _, err := i.EncodeResponseEvent(event); err != nil {
			return nil, err
		}
	}
	return json.Marshal(i.assembledMessage())
}

// FailureDocument is the JSON body an Anthropic client reads for a refusal
// that happens before any stream starts. A rate limit and an overload use
// the types Claude Code branches on; every other status is an API error.
func FailureDocument(status int, message string) []byte {
	if status == 0 {
		status = http.StatusBadGateway
	}
	payload, err := json.Marshal(map[string]any{
		"type": "error",
		"error": map[string]any{
			"type":    failureType(status),
			"message": message,
		},
	})
	if err != nil {
		return []byte(`{"type":"error","error":{"type":"api_error","message":"encoding the error failed"}}`)
	}
	return payload
}

func failureType(status int) string {
	switch status {
	case http.StatusTooManyRequests:
		return "rate_limit_error"
	case http.StatusServiceUnavailable, http.StatusBadGateway, 529:
		return "overloaded_error"
	default:
		return "api_error"
	}
}

// EncodeError renders a relay failure as the frames that close a stream.
func (i *Inbound) EncodeError(err error) []wire.SSEEvent {
	failure := &inference.ErrorInfo{Code: streamErrorCode, Message: err.Error(), Status: http.StatusBadGateway}
	i.failure = failure
	return append(i.failureFrames(failure), i.frame(eventMessageStop, map[string]any{})...)
}

func (i *Inbound) textFrames(text string) []wire.SSEEvent {
	frames := append(i.startedFrames(), i.openBlock(blockText, "", "")...)
	i.block.text.WriteString(text)
	return append(frames, i.deltaFrame(map[string]any{"type": deltaText, "text": text})...)
}

func (i *Inbound) reasoningFrames(event inference.Event) []wire.SSEEvent {
	frames := i.startedFrames()
	if event.Text != "" {
		frames = append(frames, i.openBlock(blockThinking, "", "")...)
		i.block.text.WriteString(event.Text)
		frames = append(frames, i.deltaFrame(map[string]any{"type": deltaThinking, "thinking": event.Text})...)
	}
	if event.Reasoning == nil || event.Reasoning.Signature == "" {
		return frames
	}
	frames = append(frames, i.openBlock(blockThinking, "", "")...)
	i.block.signature = event.Reasoning.Signature
	return append(frames, i.deltaFrame(map[string]any{"type": deltaSignature, "signature": event.Reasoning.Signature})...)
}

func (i *Inbound) toolStartFrames(call *inference.ToolCallDelta) []wire.SSEEvent {
	if call == nil {
		return nil
	}
	frames := append(i.startedFrames(), i.openBlock(blockToolUse, call.ID, call.Name)...)
	if call.Arguments == "" {
		return frames
	}
	i.block.args.WriteString(call.Arguments)
	return append(frames, i.deltaFrame(map[string]any{"type": deltaJSON, "partial_json": call.Arguments})...)
}

func (i *Inbound) toolDeltaFrames(call *inference.ToolCallDelta) []wire.SSEEvent {
	if call == nil || call.Arguments == "" || !i.toolOpen() {
		return nil
	}
	i.block.args.WriteString(call.Arguments)
	return i.deltaFrame(map[string]any{"type": deltaJSON, "partial_json": call.Arguments})
}

func (i *Inbound) toolEndFrames(call *inference.ToolCallDelta) []wire.SSEEvent {
	if !i.toolOpen() {
		return nil
	}
	if call != nil && call.Arguments != "" {
		i.block.args.Reset()
		i.block.args.WriteString(call.Arguments)
	}
	return i.closeBlock()
}

func (i *Inbound) terminalFrames(info *inference.TerminalInfo) []wire.SSEEvent {
	if info != nil {
		i.reason = info.Reason
	}
	frames := append(i.startedFrames(), i.closeBlock()...)
	if i.failure != nil {
		return append(frames, i.frame(eventMessageStop, map[string]any{})...)
	}
	delta := map[string]any{"stop_reason": stopReasonOf(i.reason), "stop_sequence": nil}
	usage := i.frame(eventMessageDelta, map[string]any{"delta": delta, "usage": i.usageObject()})
	return append(append(frames, usage...), i.frame(eventMessageStop, map[string]any{})...)
}

func (i *Inbound) failureFrames(failure *inference.ErrorInfo) []wire.SSEEvent {
	if failure == nil {
		return i.startedFrames()
	}
	payload := map[string]any{"error": map[string]any{"type": failure.Code, "message": failure.Message}}
	return append(i.startedFrames(), i.frame(eventError, payload)...)
}

func (i *Inbound) startedFrames() []wire.SSEEvent {
	if i.started {
		return nil
	}
	i.started = true
	message := map[string]any{
		"id":            i.messageID(),
		"type":          messageObject,
		"role":          inference.RoleAssistant,
		"model":         i.model,
		"content":       []any{},
		"stop_reason":   nil,
		"stop_sequence": nil,
		"usage":         i.startUsage(),
	}
	return i.frame(eventMessageStart, map[string]any{"message": message})
}

func (i *Inbound) frame(name string, payload map[string]any) []wire.SSEEvent {
	i.seq++
	payload["type"] = name
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil
	}
	return []wire.SSEEvent{{Name: name, Data: string(encoded)}}
}

func (i *Inbound) openBlock(kind, id, name string) []wire.SSEEvent {
	if i.block != nil && i.block.kind == kind && i.block.id == id {
		return nil
	}
	frames := i.closeBlock()
	i.block = &openBlock{index: i.blockIndex, kind: kind, id: id, name: name}
	i.blockIndex++
	start := i.frame(eventContentBlockStart, map[string]any{"index": i.block.index, "content_block": i.block.startObject()})
	return append(frames, start...)
}

func (i *Inbound) closeBlock() []wire.SSEEvent {
	if i.block == nil {
		return nil
	}
	block := i.block
	i.block = nil
	i.blocks = append(i.blocks, block.object())
	return i.frame(eventContentBlockStop, map[string]any{"index": block.index})
}

func (i *Inbound) deltaFrame(delta map[string]any) []wire.SSEEvent {
	return i.frame(eventContentBlockDelta, map[string]any{"index": i.block.index, "delta": delta})
}

func (i *Inbound) toolOpen() bool {
	return i.block != nil && i.block.kind == blockToolUse
}

func (b *openBlock) startObject() map[string]any {
	switch b.kind {
	case blockText:
		return map[string]any{"type": blockText, "text": ""}
	case blockThinking:
		return map[string]any{"type": blockThinking, "thinking": ""}
	default:
		return map[string]any{"type": blockToolUse, "id": b.id, "name": b.name, "input": map[string]any{}}
	}
}

func (b *openBlock) object() map[string]any {
	switch b.kind {
	case blockText:
		return map[string]any{"type": blockText, "text": b.text.String()}
	case blockThinking:
		return map[string]any{"type": blockThinking, "thinking": b.text.String(), "signature": b.signature}
	default:
		return map[string]any{"type": blockToolUse, "id": b.id, "name": b.name, "input": json.RawMessage(toolInput(b.args.String()))}
	}
}

func (i *Inbound) assembledMessage() map[string]any {
	message := map[string]any{
		"id":            i.messageID(),
		"type":          messageObject,
		"role":          inference.RoleAssistant,
		"model":         i.model,
		"content":       i.blocks,
		"stop_reason":   stopReasonOf(i.reason),
		"stop_sequence": nil,
		"usage":         i.usageObject(),
	}
	if i.failure != nil {
		message["error"] = map[string]any{"type": i.failure.Code, "message": i.failure.Message}
	}
	return message
}

func (i *Inbound) startUsage() map[string]any {
	usage := i.usageObject()
	usage["output_tokens"] = 0
	return usage
}

func (i *Inbound) usageObject() map[string]any {
	if i.usage == nil {
		return map[string]any{
			"input_tokens":                0,
			"output_tokens":               0,
			"cache_creation_input_tokens": 0,
			"cache_read_input_tokens":     0,
		}
	}
	return map[string]any{
		"input_tokens":                i.usage.InputTokens,
		"output_tokens":               i.usage.OutputTokens,
		"cache_creation_input_tokens": i.usage.CacheWriteTokens,
		"cache_read_input_tokens":     i.usage.CacheReadTokens,
	}
}

func (i *Inbound) messageID() string {
	return "msg_" + i.suffix
}

func stopReasonOf(reason string) string {
	switch reason {
	case inference.EventReasonToolUse:
		return stopToolUse
	case inference.EventReasonLength:
		return stopMaxTokens
	default:
		return stopEndTurn
	}
}

func newMessageSuffix() string {
	raw := make([]byte, messageIDBytes)
	if _, err := rand.Read(raw); err != nil {
		return "0000000000000000"
	}
	return hex.EncodeToString(raw)
}

type inboundRequest struct {
	Model        string           `json:"model"`
	MaxTokens    int              `json:"max_tokens"`
	System       json.RawMessage  `json:"system"`
	Messages     []inboundMessage `json:"messages"`
	Tools        []wireTool       `json:"tools"`
	ToolChoice   json.RawMessage  `json:"tool_choice"`
	Stream       bool             `json:"stream"`
	Temperature  *float64         `json:"temperature"`
	Thinking     *thinkingDef     `json:"thinking"`
	OutputConfig *outputConfig    `json:"output_config"`
}

type inboundMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type inboundBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	Thinking  string          `json:"thinking"`
	Signature string          `json:"signature"`
	Data      string          `json:"data"`
	Source    *imageSource    `json:"source"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
	Content   json.RawMessage `json:"content"`
}

func (r inboundRequest) toCanonical() (*inference.Request, error) {
	if r.Model == "" {
		return nil, fmt.Errorf("%w: model is required", wire.ErrInvalidRequest)
	}
	system, err := r.systemPrompt()
	if err != nil {
		return nil, err
	}
	messages, err := r.messages()
	if err != nil {
		return nil, err
	}
	if system != "" {
		messages = append([]inference.Message{systemMessage(system)}, messages...)
	}
	return &inference.Request{
		Model:       r.Model,
		Messages:    messages,
		Tools:       decodeTools(r.Tools),
		ToolChoice:  decodeToolChoice(r.ToolChoice),
		Stream:      r.Stream,
		MaxTokens:   tokenLimit(r.MaxTokens),
		Temperature: r.Temperature,
		Reasoning:   decodeThinking(r.Thinking, r.OutputConfig),
	}, nil
}

// The tool-choice modes the Messages wire names, which are its own names rather
// than the Chat wire's.
const (
	choiceAuto     = "auto"
	choiceNone     = "none"
	choiceAny      = "any"
	choiceToolType = "tool"
)

// decodeToolChoice reads the Messages wire's tool choice. A shape this wire
// does not define leaves the choice unset rather than guessing at what the
// client meant.
func decodeToolChoice(raw json.RawMessage) *inference.ToolChoice {
	if len(raw) == 0 {
		return nil
	}
	var selector struct {
		Type string `json:"type"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(raw, &selector); err != nil {
		return nil
	}
	switch selector.Type {
	case choiceAuto:
		return &inference.ToolChoice{Mode: inference.ToolChoiceAuto}
	case choiceNone:
		return &inference.ToolChoice{Mode: inference.ToolChoiceNone}
	case choiceAny:
		return &inference.ToolChoice{Mode: inference.ToolChoiceRequired}
	case choiceToolType:
		return &inference.ToolChoice{Mode: inference.ToolChoiceTool, Name: selector.Name}
	default:
		return nil
	}
}

func (r inboundRequest) systemPrompt() (string, error) {
	if len(r.System) == 0 {
		return "", nil
	}
	var text string
	if err := json.Unmarshal(r.System, &text); err == nil {
		return text, nil
	}
	var blocks []inboundBlock
	if err := json.Unmarshal(r.System, &blocks); err != nil {
		return "", fmt.Errorf("%w: system must be a string or a block list", wire.ErrInvalidRequest)
	}
	return textOfBlocks(blocks), nil
}

func (r inboundRequest) messages() ([]inference.Message, error) {
	decoded := make([]inference.Message, 0, len(r.Messages))
	for _, message := range r.Messages {
		turns, err := decodeMessage(message)
		if err != nil {
			return nil, err
		}
		decoded = append(decoded, turns...)
	}
	return decoded, nil
}

func decodeMessage(message inboundMessage) ([]inference.Message, error) {
	parts, calls, results, err := decodeBlocks(message.Content)
	if err != nil {
		return nil, err
	}
	if len(parts) == 0 && len(calls) == 0 {
		return results, nil
	}
	turn := inference.Message{Role: message.Role, Content: parts, ToolCalls: calls}
	return append(results, turn), nil
}

func decodeBlocks(raw json.RawMessage) ([]inference.ContentPart, []inference.ToolCall, []inference.Message, error) {
	if len(raw) == 0 {
		return nil, nil, nil, nil
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return []inference.ContentPart{{Type: inference.ContentTypeText, Text: text}}, nil, nil, nil
	}
	var blocks []inboundBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return nil, nil, nil, fmt.Errorf("%w: content must be a string or a block list", wire.ErrInvalidRequest)
	}
	var parts []inference.ContentPart
	var calls []inference.ToolCall
	var results []inference.Message
	for _, block := range blocks {
		parts, calls, results = decodeBlock(block, parts, calls, results)
	}
	return parts, calls, results, nil
}

func decodeBlock(block inboundBlock, parts []inference.ContentPart, calls []inference.ToolCall, results []inference.Message) ([]inference.ContentPart, []inference.ToolCall, []inference.Message) {
	switch block.Type {
	case blockText:
		parts = append(parts, inference.ContentPart{Type: inference.ContentTypeText, Text: block.Text})
	case blockThinking:
		parts = append(parts, thinkingPart(block.Thinking, block.Signature))
	case redactedThinking:
		parts = append(parts, thinkingPart("", block.Data))
	case blockImage:
		parts = append(parts, inference.ContentPart{Type: inference.ContentTypeImage, ImageURL: imageURL(block.Source)})
	case blockToolUse:
		calls = append(calls, inference.ToolCall{ID: block.ID, Name: block.Name, Arguments: string(block.Input)})
	case blockToolResult:
		results = append(results, toolResultMessage(block))
	}
	return parts, calls, results
}

func thinkingPart(text, signature string) inference.ContentPart {
	return inference.ContentPart{Type: inference.ContentTypeThinking, Text: text, Signature: signature}
}

func toolResultMessage(block inboundBlock) inference.Message {
	return inference.Message{
		Role:       inference.RoleTool,
		ToolCallID: block.ToolUseID,
		Content:    resultParts(block.Content),
	}
}

func resultParts(raw json.RawMessage) []inference.ContentPart {
	if len(raw) == 0 {
		return nil
	}
	parts, _, _, err := decodeBlocks(raw)
	if err != nil {
		return []inference.ContentPart{{Type: inference.ContentTypeText, Text: strings.TrimSpace(string(raw))}}
	}
	return parts
}

func imageURL(source *imageSource) string {
	if source == nil {
		return ""
	}
	if source.Type == sourceBase64 {
		return dataURLPrefix + source.MediaType + dataURLBase64 + source.Data
	}
	return source.URL
}

func textOfBlocks(blocks []inboundBlock) string {
	var builder strings.Builder
	for _, block := range blocks {
		if block.Type == blockText {
			builder.WriteString(block.Text)
		}
	}
	return builder.String()
}

func systemMessage(text string) inference.Message {
	return inference.Message{Role: inference.RoleSystem, Content: []inference.ContentPart{{Type: inference.ContentTypeText, Text: text}}}
}

func decodeTools(tools []wireTool) []inference.Tool {
	decoded := make([]inference.Tool, 0, len(tools))
	for _, tool := range tools {
		decoded = append(decoded, inference.Tool{Name: tool.Name, Description: tool.Description, Parameters: tool.InputSchema})
	}
	return decoded
}

func decodeThinking(thinking *thinkingDef, output *outputConfig) *inference.ReasoningConfig {
	if output != nil && strings.TrimSpace(output.Effort) != "" {
		return &inference.ReasoningConfig{Effort: output.Effort}
	}
	if thinking == nil {
		return nil
	}
	switch thinking.Type {
	case "enabled":
		return &inference.ReasoningConfig{Effort: effortFor(thinking.BudgetTokens)}
	case "disabled":
		return &inference.ReasoningConfig{Effort: "none"}
	default:
		return nil
	}
}

func effortFor(budget int) string {
	switch {
	case budget >= thinkingBudgets["max"]:
		return "max"
	case budget >= thinkingBudgets["xhigh"]:
		return "xhigh"
	case budget >= thinkingBudgets["high"]:
		return "high"
	case budget >= thinkingBudgets["medium"]:
		return "medium"
	case budget >= thinkingBudgets["low"]:
		return "low"
	default:
		return "low"
	}
}
