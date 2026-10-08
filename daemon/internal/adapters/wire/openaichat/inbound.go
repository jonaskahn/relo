// Chat completions inbound codec.
package openaichat

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/jonaskahn/relo/internal/adapters/wire"
	"github.com/jonaskahn/relo/internal/inference"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	maxRequestBytes    = 32 << 20
	chatChunkIDPrefix  = "chatcmpl-"
	chatChunkIDBytes   = 16
	chatObjectChunk    = "chat.completion.chunk"
	chatObjectComplete = "chat.completion"
	doneSentinel       = "[DONE]"
)

// ChatCompletionsCodec translates the OpenAI Chat Completions surface that
// Grok Build and file-toggle clients speak. One instance serves one request,
// which is what lets every chunk share one response identifier.
type ChatCompletionsCodec struct {
	id      string
	created int64
	model   string
	failed  bool
}

var (
	_ wire.InboundCodec = (*ChatCompletionsCodec)(nil)
)

// NewChatCompletionsCodec returns a codec bound to a fresh response identity.
func NewChatCompletionsCodec() *ChatCompletionsCodec {
	return &ChatCompletionsCodec{id: newChunkID(), created: time.Now().Unix()}
}

// DecodeRequest parses a client chat completions request into canonical form.
func (c *ChatCompletionsCodec) DecodeRequest(r *http.Request) (*inference.Request, error) {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxRequestBytes))
	if err != nil {
		return nil, fmt.Errorf("read chat completions request: %w", err)
	}
	var payload chatWireRequest
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("%w: %v", wire.ErrInvalidRequest, err)
	}
	canonical, err := payload.toCanonical()
	if err != nil {
		return nil, err
	}
	c.model = canonical.Model
	return canonical, nil
}

// EncodeResponseEvent renders one canonical event as client SSE frames.
func (c *ChatCompletionsCodec) EncodeResponseEvent(event inference.Event) ([]wire.SSEEvent, error) {
	switch event.Kind {
	case inference.EventTextDelta:
		return c.deltaFrames(map[string]any{"content": event.Text})
	case inference.EventReasoningDelta:
		return c.deltaFrames(map[string]any{"reasoning_content": event.Text})
	case inference.EventToolCallStart:
		return c.toolCallStartFrames(event.ToolCall)
	case inference.EventToolCallDelta:
		return c.toolCallDeltaFrames(event.ToolCall)
	case inference.EventToolCallEnd:
		return nil, nil
	case inference.EventUsage:
		return c.usageFrames(event.Usage), nil
	case inference.EventTerminal:
		return c.terminalFrames(event.Terminal), nil
	case inference.EventError:
		return c.errorFrames(event.Error), nil
	default:
		return nil, fmt.Errorf("%w: %s", wire.ErrUnknownEvent, event.Kind)
	}
}

// EncodeResponse renders a completed response as one chat completion object.
func (c *ChatCompletionsCodec) EncodeResponse(events []inference.Event) ([]byte, error) {
	assembled := newChatAssembler()
	for _, event := range events {
		if err := assembled.observe(event); err != nil {
			return nil, err
		}
	}
	if assembled.failure != nil {
		return json.Marshal(errorObject(assembled.failure))
	}
	return json.Marshal(c.completionObject(assembled))
}

// EncodeError renders a relay failure as client SSE frames.
func (c *ChatCompletionsCodec) EncodeError(err error) []wire.SSEEvent {
	return c.errorFrames(&inference.ErrorInfo{
		Code:    "stream_error",
		Message: err.Error(),
		Status:  http.StatusBadGateway,
	})
}

func (c *ChatCompletionsCodec) deltaFrames(delta map[string]any) ([]wire.SSEEvent, error) {
	return c.marshalFrame(c.chunk(map[string]any{"index": 0, "delta": delta, "finish_reason": nil}))
}

func (c *ChatCompletionsCodec) toolCallStartFrames(delta *inference.ToolCallDelta) ([]wire.SSEEvent, error) {
	if delta == nil {
		return nil, nil
	}
	call := map[string]any{
		"index": delta.Index,
		"id":    delta.ID,
		"type":  "function",
		"function": map[string]any{
			"name":      delta.Name,
			"arguments": delta.Arguments,
		},
	}
	return c.deltaFrames(map[string]any{"tool_calls": []any{call}})
}

func (c *ChatCompletionsCodec) toolCallDeltaFrames(delta *inference.ToolCallDelta) ([]wire.SSEEvent, error) {
	if delta == nil {
		return nil, nil
	}
	call := map[string]any{
		"index":    delta.Index,
		"function": map[string]any{"arguments": delta.Arguments},
	}
	return c.deltaFrames(map[string]any{"tool_calls": []any{call}})
}

func (c *ChatCompletionsCodec) usageFrames(usage *inference.UsageReport) []wire.SSEEvent {
	if usage == nil {
		return nil
	}
	chunk := map[string]any{
		"id":      c.id,
		"object":  chatObjectChunk,
		"created": c.created,
		"model":   c.model,
		"choices": []any{},
		"usage":   chatUsage(usage),
	}
	frames, err := c.marshalFrame(chunk)
	if err != nil {
		return nil
	}
	return frames
}

func (c *ChatCompletionsCodec) terminalFrames(info *inference.TerminalInfo) []wire.SSEEvent {
	if c.failed {
		return nil
	}
	reason := inference.EventReasonStop
	if info != nil && info.Reason != "" {
		reason = chatFinishReason(info.Reason)
	}
	choice := map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": reason}
	frames, err := c.marshalFrame(c.chunk(choice))
	if err != nil {
		return []wire.SSEEvent{{Data: doneSentinel}}
	}
	return append(frames, wire.SSEEvent{Data: doneSentinel})
}

func (c *ChatCompletionsCodec) errorFrames(info *inference.ErrorInfo) []wire.SSEEvent {
	c.failed = true
	if info == nil {
		info = &inference.ErrorInfo{Code: "internal_error", Message: "relay failed", Status: http.StatusBadGateway}
	}
	payload, err := json.Marshal(errorObject(info))
	if err != nil {
		return []wire.SSEEvent{{Data: doneSentinel}}
	}
	return []wire.SSEEvent{{Data: string(payload)}, {Data: doneSentinel}}
}

func errorObject(info *inference.ErrorInfo) map[string]any {
	return map[string]any{"error": map[string]any{
		"type":    info.Code,
		"message": info.Message,
		"code":    info.Status,
	}}
}

func (c *ChatCompletionsCodec) chunk(choice map[string]any) map[string]any {
	return map[string]any{
		"id":      c.id,
		"object":  chatObjectChunk,
		"created": c.created,
		"model":   c.model,
		"choices": []any{choice},
	}
}

func (c *ChatCompletionsCodec) completionObject(assembled *chatAssembler) map[string]any {
	return map[string]any{
		"id":      c.id,
		"object":  chatObjectComplete,
		"created": c.created,
		"model":   c.model,
		"choices": []any{map[string]any{
			"index":         0,
			"message":       assembled.message(),
			"finish_reason": assembled.finishReason(),
		}},
		"usage": assembled.usageObject(),
	}
}

func (c *ChatCompletionsCodec) marshalFrame(chunk map[string]any) ([]wire.SSEEvent, error) {
	payload, err := json.Marshal(chunk)
	if err != nil {
		return nil, fmt.Errorf("encode chat completions frame: %w", err)
	}
	return []wire.SSEEvent{{Data: string(payload)}}, nil
}

func chatUsage(usage *inference.UsageReport) map[string]any {
	return map[string]any{
		"prompt_tokens":     usage.InputTokens,
		"completion_tokens": usage.OutputTokens,
		"total_tokens":      usage.InputTokens + usage.OutputTokens,
		"prompt_tokens_details": map[string]any{
			"cached_tokens": usage.CacheReadTokens,
		},
	}
}

func chatFinishReason(reason string) string {
	if reason == inference.EventReasonToolUse {
		return "tool_calls"
	}
	if reason == inference.EventReasonLength {
		return "length"
	}
	return inference.EventReasonStop
}

func newChunkID() string {
	suffix := make([]byte, chatChunkIDBytes)
	if _, err := rand.Read(suffix); err != nil {
		return chatChunkIDPrefix + "0"
	}
	return chatChunkIDPrefix + hex.EncodeToString(suffix)
}

type chatAssembler struct {
	content   strings.Builder
	reasoning strings.Builder
	toolCalls []*streamedToolCall
	usage     *inference.UsageReport
	terminal  string
	failure   *inference.ErrorInfo
}

type streamedToolCall struct {
	id        string
	name      string
	arguments strings.Builder
}

func newChatAssembler() *chatAssembler {
	return &chatAssembler{}
}

func (a *chatAssembler) observe(event inference.Event) error {
	switch event.Kind {
	case inference.EventTextDelta:
		a.content.WriteString(event.Text)
	case inference.EventReasoningDelta:
		a.reasoning.WriteString(event.Text)
	case inference.EventToolCallStart:
		a.startToolCall(event.ToolCall)
	case inference.EventToolCallDelta:
		a.appendArguments(event.ToolCall)
	case inference.EventToolCallEnd:
		return nil
	case inference.EventUsage:
		a.usage = event.Usage
	case inference.EventTerminal:
		a.terminal = event.Terminal.Reason
	case inference.EventError:
		a.failure = event.Error
	default:
		return fmt.Errorf("%w: %s", wire.ErrUnknownEvent, event.Kind)
	}
	return nil
}

func (a *chatAssembler) startToolCall(delta *inference.ToolCallDelta) {
	if delta == nil {
		return
	}
	call := &streamedToolCall{id: delta.ID, name: delta.Name}
	call.arguments.WriteString(delta.Arguments)
	a.toolCalls = append(a.toolCalls, call)
}

func (a *chatAssembler) appendArguments(delta *inference.ToolCallDelta) {
	if delta == nil || len(a.toolCalls) == 0 {
		return
	}
	a.toolCalls[len(a.toolCalls)-1].arguments.WriteString(delta.Arguments)
}

func (a *chatAssembler) message() map[string]any {
	if a.failure != nil {
		return map[string]any{"role": inference.RoleAssistant, "content": ""}
	}
	message := map[string]any{"role": inference.RoleAssistant, "content": a.content.String()}
	if text := a.reasoning.String(); text != "" {
		message["reasoning_content"] = text
	}
	if len(a.toolCalls) > 0 {
		message["tool_calls"] = a.toolCallObjects()
	}
	return message
}

func (a *chatAssembler) toolCallObjects() []any {
	calls := make([]any, 0, len(a.toolCalls))
	for _, call := range a.toolCalls {
		calls = append(calls, map[string]any{
			"id":   call.id,
			"type": "function",
			"function": map[string]any{
				"name":      call.name,
				"arguments": call.arguments.String(),
			},
		})
	}
	return calls
}

func (a *chatAssembler) finishReason() string {
	if a.failure != nil {
		return inference.EventReasonError
	}
	if a.terminal == "" {
		return inference.EventReasonStop
	}
	return chatFinishReason(a.terminal)
}

func (a *chatAssembler) usageObject() any {
	if a.usage == nil {
		return map[string]any{"prompt_tokens": 0, "completion_tokens": 0, "total_tokens": 0}
	}
	return chatUsage(a.usage)
}
