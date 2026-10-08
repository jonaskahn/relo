// Responses inbound codec: client requests into canonical form.
package openairesponses

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/jonaskahn/relo/internal/inference"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jonaskahn/relo/internal/adapters/wire"
)

const (
	maxRequestBytes  = 32 << 20
	responseObject   = "response"
	textPartType     = "output_text"
	roleDeveloper    = "developer"
	statusInProgress = "in_progress"
	responseIDBytes  = 16
	streamErrorCode  = "stream_error"
)

// Inbound translates the Responses surface Codex speaks into canonical form
// and back. One instance serves one request, which is what lets every frame
// share one response identity.
type Inbound struct {
	suffix      string
	created     int64
	model       string
	seq         int
	started     bool
	outputIndex int
	textIndex   int
	textOpen    bool
	textDone    bool
	text        strings.Builder
	items       map[int]map[string]any
	calls       map[int]*callState
	custom      map[string]bool
	reasonID    string
	reasonIndex int
	reasonOpen  bool
	reasonDone  bool
	reasonText  strings.Builder
	usage       *inference.UsageReport
	failure     *inference.ErrorInfo
	reason      string
}

type callState struct {
	index int
	id    string
	name  string
	args  strings.Builder
	// custom marks a call to a freeform tool, rendered as a custom_tool_call.
	custom bool
}

var _ wire.InboundCodec = (*Inbound)(nil)

// NewInbound returns an inbound codec bound to a fresh response identity.
func NewInbound() *Inbound {
	return &Inbound{
		suffix:  newResponseSuffix(),
		created: time.Now().Unix(),
		items:   map[int]map[string]any{},
		calls:   map[int]*callState{},
	}
}

// DecodeRequest parses a client Responses request into canonical form.
func (i *Inbound) DecodeRequest(r *http.Request) (*inference.Request, error) {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxRequestBytes))
	if err != nil {
		return nil, fmt.Errorf("read responses request: %w", err)
	}
	var request inboundRequest
	if err := json.Unmarshal(body, &request); err != nil {
		return nil, fmt.Errorf("%w: %v", wire.ErrInvalidRequest, err)
	}
	canonical, custom, err := request.toCanonical()
	if err != nil {
		return nil, err
	}
	i.custom = custom
	i.model = canonical.Model
	return canonical, nil
}

// EncodeResponseEvent renders one canonical event as Responses SSE frames.
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

// EncodeResponse renders a completed response as one response object, the
// shape a client that asked for no stream expects.
func (i *Inbound) EncodeResponse(events []inference.Event) ([]byte, error) {
	for _, event := range events {
		if _, err := i.EncodeResponseEvent(event); err != nil {
			return nil, err
		}
	}
	return json.Marshal(i.finalObject())
}

// EncodeError renders a relay failure as the frames that close a stream.
func (i *Inbound) EncodeError(err error) []wire.SSEEvent {
	failure := &inference.ErrorInfo{Code: streamErrorCode, Message: err.Error(), Status: http.StatusBadGateway}
	return append(i.failureFrames(failure), i.failedFrame(failure)...)
}

func (i *Inbound) textFrames(text string) []wire.SSEEvent {
	i.text.WriteString(text)
	frames := append(i.startedFrames(), i.openTextItem()...)
	delta := i.frame(eventOutputTextDelta, map[string]any{
		"item_id":       i.messageItemID(),
		"output_index":  i.textIndex,
		"content_index": 0,
		"delta":         text,
	})
	return append(frames, delta...)
}

func (i *Inbound) openTextItem() []wire.SSEEvent {
	if i.textOpen {
		return nil
	}
	i.textOpen = true
	i.textIndex = i.nextOutputIndex()
	i.items[i.textIndex] = i.messageItem(statusInProgress)
	frames := i.frame(eventOutputItemAdded, map[string]any{
		"output_index": i.textIndex,
		"item":         i.items[i.textIndex],
	})
	part := i.frame("response.content_part.added", map[string]any{
		"item_id":       i.messageItemID(),
		"output_index":  i.textIndex,
		"content_index": 0,
		"part":          i.textPart(),
	})
	return append(frames, part...)
}

func (i *Inbound) closeTextItem() []wire.SSEEvent {
	if !i.textOpen || i.textDone {
		return nil
	}
	i.textDone = true
	done := i.frame("response.output_text.done", map[string]any{
		"item_id":       i.messageItemID(),
		"output_index":  i.textIndex,
		"content_index": 0,
		"text":          i.text.String(),
	})
	part := i.frame("response.content_part.done", map[string]any{
		"item_id":       i.messageItemID(),
		"output_index":  i.textIndex,
		"content_index": 0,
		"part":          i.textPart(),
	})
	i.items[i.textIndex] = i.messageItem(statusCompleted)
	item := i.frame(eventOutputItemDone, map[string]any{"output_index": i.textIndex, "item": i.items[i.textIndex]})
	return append(append(done, part...), item...)
}

func (i *Inbound) reasoningFrames(event inference.Event) []wire.SSEEvent {
	frames := i.startedFrames()
	if event.Reasoning != nil && event.Reasoning.ID != "" {
		i.reasonID = event.Reasoning.ID
	}
	if event.Reasoning != nil && len(event.Reasoning.EncryptedContent) > 0 {
		return append(frames, i.reasoningItemFrames(event.Reasoning)...)
	}
	if event.Text == "" {
		return frames
	}
	frames = append(frames, i.openReasoningSummary()...)
	i.reasonText.WriteString(event.Text)
	i.items[i.reasonIndex] = i.reasoningSummaryItem(statusInProgress)
	delta := i.frame(eventReasoningDelta, map[string]any{
		"item_id":       i.reasoningItemID(),
		"output_index":  i.reasonIndex,
		"summary_index": 0,
		"delta":         event.Text,
	})
	return append(frames, delta...)
}

func (i *Inbound) openReasoningSummary() []wire.SSEEvent {
	if i.reasonOpen {
		return nil
	}
	i.reasonOpen = true
	i.reasonIndex = i.nextOutputIndex()
	i.items[i.reasonIndex] = i.reasoningSummaryItem(statusInProgress)
	added := i.frame(eventOutputItemAdded, map[string]any{
		"output_index": i.reasonIndex,
		"item":         i.items[i.reasonIndex],
	})
	part := i.frame(eventReasoningPartAdded, map[string]any{
		"item_id":       i.reasoningItemID(),
		"output_index":  i.reasonIndex,
		"summary_index": 0,
		"part":          map[string]any{"type": summaryText, "text": ""},
	})
	return append(added, part...)
}

func (i *Inbound) closeReasoningSummary() []wire.SSEEvent {
	if !i.reasonOpen || i.reasonDone {
		return nil
	}
	i.reasonDone = true
	text := i.reasonText.String()
	done := i.frame(eventReasoningTextDone, map[string]any{
		"item_id":       i.reasoningItemID(),
		"output_index":  i.reasonIndex,
		"summary_index": 0,
		"text":          text,
	})
	part := i.frame(eventReasoningPartDone, map[string]any{
		"item_id":       i.reasoningItemID(),
		"output_index":  i.reasonIndex,
		"summary_index": 0,
		"part":          map[string]any{"type": summaryText, "text": text},
	})
	i.items[i.reasonIndex] = i.reasoningSummaryItem(statusCompleted)
	item := i.frame(eventOutputItemDone, map[string]any{
		"output_index": i.reasonIndex,
		"item":         i.items[i.reasonIndex],
	})
	return append(append(done, part...), item...)
}

func (i *Inbound) reasoningSummaryItem(status string) map[string]any {
	return map[string]any{
		"type":    itemReasoning,
		"id":      i.reasoningItemID(),
		"status":  status,
		"summary": i.reasoningSummary(),
	}
}

func (i *Inbound) reasoningSummary() []any {
	text := i.reasonText.String()
	if text == "" {
		return []any{}
	}
	return []any{map[string]any{"type": summaryText, "text": text}}
}

func (i *Inbound) reasoningItemFrames(reasoning *inference.ReasoningConfig) []wire.SSEEvent {
	if i.items[i.reasonIndex] == nil {
		i.reasonIndex = i.nextOutputIndex()
	}
	item := map[string]any{
		"type":              itemReasoning,
		"id":                reasoning.ID,
		"summary":           []any{},
		"encrypted_content": string(reasoning.EncryptedContent),
	}
	i.items[i.reasonIndex] = item
	return i.frame(eventOutputItemDone, map[string]any{"output_index": i.reasonIndex, "item": item})
}

func (i *Inbound) toolStartFrames(call *inference.ToolCallDelta) []wire.SSEEvent {
	if call == nil {
		return nil
	}
	started := i.call(call)
	item := i.callItem(started, statusInProgress)
	i.items[started.index] = item
	return append(i.startedFrames(), i.frame(eventOutputItemAdded, map[string]any{
		"output_index": started.index,
		"item":         item,
	})...)
}

func (i *Inbound) toolDeltaFrames(delta *inference.ToolCallDelta) []wire.SSEEvent {
	started, found := i.calls[indexOf(delta)]
	if !found || delta.Arguments == "" {
		return nil
	}
	started.args.WriteString(delta.Arguments)
	if started.custom {
		return i.startedFrames()
	}
	return append(i.startedFrames(), i.frame(eventArgumentsDelta, map[string]any{
		"item_id":      started.id,
		"output_index": started.index,
		"delta":        delta.Arguments,
	})...)
}

func (i *Inbound) toolEndFrames(delta *inference.ToolCallDelta) []wire.SSEEvent {
	started, found := i.calls[indexOf(delta)]
	if !found {
		return nil
	}
	if delta.Arguments != "" {
		started.absorbArguments(delta.Arguments)
	}
	if started.custom {
		return i.customEndFrames(started)
	}
	item := i.functionCallItem(started, statusCompleted)
	i.items[started.index] = item
	frames := append(i.startedFrames(), i.frame(eventArgumentsDone, map[string]any{
		"item_id":      started.id,
		"output_index": started.index,
		"arguments":    started.args.String(),
	})...)
	return append(frames, i.frame(eventOutputItemDone, map[string]any{
		"output_index": started.index,
		"item":         item,
	})...)
}

func (i *Inbound) terminalFrames(info *inference.TerminalInfo) []wire.SSEEvent {
	if info != nil {
		i.reason = info.Reason
	}
	frames := append(i.startedFrames(), i.closeReasoningSummary()...)
	frames = append(frames, i.closeTextItem()...)
	if i.failure != nil {
		return append(frames, i.failedFrame(i.failure)...)
	}
	return append(frames, i.frame(eventCompleted, map[string]any{"response": i.finalObject()})...)
}

func (i *Inbound) failureFrames(failure *inference.ErrorInfo) []wire.SSEEvent {
	if failure == nil {
		return i.startedFrames()
	}
	payload := map[string]any{"code": failure.Code, "message": failure.Message, "param": nil}
	return append(i.startedFrames(), i.frame(eventError, payload)...)
}

func (i *Inbound) failedFrame(failure *inference.ErrorInfo) []wire.SSEEvent {
	i.failure = failure
	return i.frame(eventFailed, map[string]any{"response": i.finalObject()})
}

func (i *Inbound) startedFrames() []wire.SSEEvent {
	if i.started {
		return nil
	}
	i.started = true
	created := i.frame(eventCreated, map[string]any{"response": i.baseObject(statusInProgress)})
	progress := i.frame(eventInProgress, map[string]any{"response": i.baseObject(statusInProgress)})
	return append(created, progress...)
}

func (i *Inbound) frame(name string, payload map[string]any) []wire.SSEEvent {
	i.seq++
	payload["type"] = name
	payload["sequence_number"] = i.seq
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil
	}
	return []wire.SSEEvent{{Name: name, Data: string(encoded)}}
}

func (i *Inbound) call(delta *inference.ToolCallDelta) *callState {
	index := indexOf(delta)
	if existing, found := i.calls[index]; found {
		return existing
	}
	started := &callState{index: i.nextOutputIndex(), id: delta.ID, name: delta.Name, custom: i.custom[delta.Name]}
	i.calls[index] = started
	return started
}

func (c *callState) absorbArguments(arguments string) {
	c.args.Reset()
	c.args.WriteString(arguments)
}

func (i *Inbound) callItem(call *callState, status string) map[string]any {
	if call.custom {
		return i.customCallItem(call, status)
	}
	return i.functionCallItem(call, status)
}

func (i *Inbound) functionCallItem(call *callState, status string) map[string]any {
	return map[string]any{
		"type":      itemFunctionCall,
		"id":        call.id,
		"call_id":   call.id,
		"name":      call.name,
		"arguments": call.args.String(),
		"status":    status,
	}
}

func (i *Inbound) finalObject() map[string]any {
	status := i.status()
	object := i.baseObject(status)
	object["output"] = i.outputItems()
	if i.usage != nil {
		object["usage"] = i.usageObject()
	}
	if i.failure != nil {
		object["error"] = map[string]any{"code": i.failure.Code, "message": i.failure.Message}
	}
	if status == statusIncomplete {
		object["incomplete_details"] = map[string]any{"reason": reasonMaxOutput}
	}
	return object
}

func (i *Inbound) baseObject(status string) map[string]any {
	return map[string]any{
		"id":         i.responseID(),
		"object":     responseObject,
		"created_at": i.created,
		"status":     status,
		"model":      i.model,
		"output":     []any{},
	}
}

func (i *Inbound) usageObject() map[string]any {
	return map[string]any{
		"input_tokens":          i.usage.InputTokens,
		"output_tokens":         i.usage.OutputTokens,
		"total_tokens":          i.usage.InputTokens + i.usage.OutputTokens,
		"input_tokens_details":  map[string]any{"cached_tokens": i.usage.CacheReadTokens},
		"output_tokens_details": map[string]any{"reasoning_tokens": 0},
	}
}

func (i *Inbound) messageItem(status string) map[string]any {
	return map[string]any{
		"type":    itemMessage,
		"id":      i.messageItemID(),
		"status":  status,
		"role":    inference.RoleAssistant,
		"content": []any{i.textPart()},
	}
}

func (i *Inbound) textPart() map[string]any {
	return map[string]any{"type": textPartType, "text": i.text.String(), "annotations": []any{}}
}

func (i *Inbound) outputItems() []any {
	items := make([]any, 0, len(i.items))
	for index := 0; index < i.outputIndex; index++ {
		if item, found := i.items[index]; found {
			items = append(items, item)
		}
	}
	return items
}

func (i *Inbound) nextOutputIndex() int {
	index := i.outputIndex
	i.outputIndex++
	return index
}

func (i *Inbound) status() string {
	switch {
	case i.failure != nil:
		return statusFailed
	case i.reason == inference.EventReasonLength:
		return statusIncomplete
	default:
		return statusCompleted
	}
}

func (i *Inbound) responseID() string    { return "resp_" + i.suffix }
func (i *Inbound) messageItemID() string { return "msg_" + i.suffix }
func (i *Inbound) reasoningItemID() string {
	if i.reasonID != "" {
		return i.reasonID
	}
	return "rs_" + i.suffix
}

func indexOf(delta *inference.ToolCallDelta) int {
	if delta == nil {
		return 0
	}
	return delta.Index
}

func newResponseSuffix() string {
	raw := make([]byte, responseIDBytes)
	if _, err := rand.Read(raw); err != nil {
		return "0000000000000000"
	}
	return hex.EncodeToString(raw)
}

type inboundRequest struct {
	Model           string          `json:"model"`
	Instructions    string          `json:"instructions"`
	Input           json.RawMessage `json:"input"`
	Tools           []toolDef       `json:"tools"`
	Stream          bool            `json:"stream"`
	MaxOutputTokens int             `json:"max_output_tokens"`
	Temperature     *float64        `json:"temperature"`
	Reasoning       *reasoningDef   `json:"reasoning"`
}

func (r inboundRequest) toCanonical() (*inference.Request, map[string]bool, error) {
	if r.Model == "" {
		return nil, nil, fmt.Errorf("%w: model is required", wire.ErrInvalidRequest)
	}
	items, err := r.items()
	if err != nil {
		return nil, nil, err
	}
	collector := newCollector()
	if r.Instructions != "" {
		collector.addSystem(r.Instructions)
	}
	for _, item := range items {
		collector.add(item)
	}
	tools, custom := decodeTools(r.Tools)
	return &inference.Request{
		Model:       r.Model,
		Messages:    collector.messages,
		Tools:       tools,
		Stream:      r.Stream,
		MaxTokens:   r.MaxOutputTokens,
		Temperature: r.Temperature,
		Reasoning:   collector.reasoning(r.Reasoning),
	}, custom, nil
}

func (r inboundRequest) items() ([]inputItem, error) {
	if len(r.Input) == 0 {
		return nil, nil
	}
	var text string
	if err := json.Unmarshal(r.Input, &text); err == nil {
		return []inputItem{{Role: inference.RoleUser, Content: []contentItem{{Type: contentInputText, Text: text}}}}, nil
	}
	var items []inputItem
	if err := json.Unmarshal(r.Input, &items); err != nil {
		return nil, fmt.Errorf("%w: input must be a string or an item list", wire.ErrInvalidRequest)
	}
	return items, nil
}

type collector struct {
	messages        []inference.Message
	encrypted       *inference.ReasoningConfig
	pendingThinking string
}

func newCollector() *collector {
	return &collector{}
}

func (c *collector) add(item inputItem) {
	switch item.Type {
	case itemFunctionCall:
		c.addToolCall(item)
	case itemCustomToolCall:
		item.Arguments = customArguments(item.Input)
		c.addToolCall(item)
	case itemFunctionCallOutput, itemCustomCallOutput:
		c.addToolResult(toolMessage(item))
	case itemReasoning:
		c.rememberReasoning(item)
	default:
		c.addMessage(item)
	}
}

func (c *collector) addSystem(text string) {
	c.messages = append(c.messages, inference.Message{
		Role:    inference.RoleSystem,
		Content: []inference.ContentPart{{Type: inference.ContentTypeText, Text: text}},
	})
}

func (c *collector) addMessage(item inputItem) {
	role := item.Role
	if role == roleDeveloper {
		role = inference.RoleSystem
	}
	if role == "" {
		role = inference.RoleUser
	}
	content := decodeContent(item.Content)
	if role == inference.RoleAssistant {
		content = append(c.takeThinking(), content...)
		// Codex records a turn's calls before its reasoning and text. The
		// message still belongs to the call it follows.
		if count := len(c.messages); count > 0 && len(c.messages[count-1].ToolCalls) > 0 {
			c.messages[count-1].Content = append(c.messages[count-1].Content, content...)
			return
		}
	}
	c.messages = append(c.messages, inference.Message{Role: role, Content: content})
}

func (c *collector) addToolCall(item inputItem) {
	call := inference.ToolCall{ID: callID(&item), Name: item.Name, Arguments: item.Arguments}
	count := len(c.messages)
	if count > 0 && c.messages[count-1].Role == inference.RoleAssistant {
		message := &c.messages[count-1]
		message.ToolCalls = append(message.ToolCalls, call)
		message.Content = append(c.takeThinking(), message.Content...)
		return
	}
	c.messages = append(c.messages, inference.Message{
		Role:      inference.RoleAssistant,
		Content:   c.takeThinking(),
		ToolCalls: []inference.ToolCall{call},
	})
}

func (c *collector) addToolResult(result inference.Message) {
	position := len(c.messages)
	for index, message := range c.messages {
		if message.Role == inference.RoleAssistant && hasCall(message, result.ToolCallID) {
			c.messages[index].Content = append(c.takeThinking(), c.messages[index].Content...)
			position = index + 1
			for position < len(c.messages) && c.messages[position].Role == inference.RoleTool {
				position++
			}
			break
		}
	}
	c.messages = append(c.messages, inference.Message{})
	copy(c.messages[position+1:], c.messages[position:])
	c.messages[position] = result
}

func hasCall(message inference.Message, id string) bool {
	for _, call := range message.ToolCalls {
		if call.ID == id {
			return true
		}
	}
	return false
}

func (c *collector) rememberReasoning(item inputItem) {
	if item.EncryptedContent != "" {
		c.encrypted = &inference.ReasoningConfig{ID: item.ID, EncryptedContent: []byte(item.EncryptedContent)}
	}
	if text := joinedSummary(item.Summary); text != "" {
		c.pendingThinking += text
	}
}

func (c *collector) takeThinking() []inference.ContentPart {
	if c.pendingThinking == "" {
		return nil
	}
	text := c.pendingThinking
	c.pendingThinking = ""
	return []inference.ContentPart{{Type: inference.ContentTypeThinking, Text: text}}
}

func joinedSummary(parts []summaryPart) string {
	var builder strings.Builder
	for _, part := range parts {
		builder.WriteString(part.Text)
	}
	return builder.String()
}

func (c *collector) reasoning(request *reasoningDef) *inference.ReasoningConfig {
	effort := ""
	if request != nil {
		effort = request.Effort
	}
	if c.encrypted == nil {
		return effortConfig(effort)
	}
	config := *c.encrypted
	config.Effort = effort
	return &config
}

func effortConfig(effort string) *inference.ReasoningConfig {
	if effort == "" {
		return nil
	}
	return &inference.ReasoningConfig{Effort: effort}
}

func toolMessage(item inputItem) inference.Message {
	return inference.Message{
		Role:       inference.RoleTool,
		ToolCallID: item.CallID,
		Content:    []inference.ContentPart{{Type: inference.ContentTypeText, Text: string(item.Output)}},
	}
}

func decodeContent(parts []contentItem) []inference.ContentPart {
	decoded := make([]inference.ContentPart, 0, len(parts))
	for _, part := range parts {
		if part.Type == contentInputImage {
			decoded = append(decoded, inference.ContentPart{Type: inference.ContentTypeImage, ImageURL: part.ImageURL})
			continue
		}
		decoded = append(decoded, inference.ContentPart{Type: inference.ContentTypeText, Text: part.Text})
	}
	return decoded
}
