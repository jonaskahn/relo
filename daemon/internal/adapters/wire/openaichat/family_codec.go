// Package openai_chat translates between the canonical format and the
// OpenAI Chat Completions wire, in both directions.
package openaichat

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/jonaskahn/relo/internal/inference"
	"net/http"
	"strings"

	"github.com/jonaskahn/relo/internal/adapters/wire"
)

const (
	// ID names the wire family in configuration and logs.
	ID = "openai-chat"

	// DefaultBaseURL is the endpoint the OpenAI API answers on. A stored
	// base URL carries the version segment, so a codec only appends the
	// resource the request names.
	DefaultBaseURL = "https://api.openai.com/v1"

	chatCompletionsPath = "/chat/completions"
	functionType        = "function"
	imageURLPart        = "image_url"

	// DeepSeek rejects a tool-using thinking request that omits reasoning_content,
	// and rejects an empty string, so a missing chain is replayed as one space.
	deepSeekMissingReasoning = " "
)

// Codec speaks the Chat Completions wire. One codec serves every request;
// each response gets its own stream decoder.
type Codec struct {
	baseURL string
}

var _ wire.CodecModule = (*Codec)(nil)

// NewCodec returns a codec for the given base URL. A request may override
// the base URL through CodecOpts.
func NewCodec(baseURL string) *Codec {
	return &Codec{baseURL: baseURL}
}

// Module returns the codec a caller without a base URL gets.
func Module() wire.CodecModule {
	return NewCodec(DefaultBaseURL)
}

// EncodeRequest builds the upstream chat completions request. The body
// carries no credential and no routing decision: the codec applies
// whichever authentication the attempt resolved.
func (c *Codec) EncodeRequest(req *inference.Request, opts wire.CodecOpts) (*http.Request, error) {
	if opts.CredentialRef == "" && opts.AuthMethod != wire.AuthNone {
		return nil, wire.ErrMissingCredential
	}
	payload, err := json.Marshal(newPayload(req, opts, c.upstreamBase(opts)))
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequest(http.MethodPost, c.endpoint(opts), bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("build chat completions request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	wire.ApplyCredential(request, opts)
	for name, value := range opts.ExtraHeaders {
		request.Header.Set(name, value)
	}
	return request, nil
}

// EncodePayload renders a canonical request as a Chat Completions body, for
// a transport that speaks the same shape over a different endpoint.
func EncodePayload(req *inference.Request) ([]byte, error) {
	payload, err := json.Marshal(newPayload(req, wire.CodecOpts{}, ""))
	if err != nil {
		return nil, fmt.Errorf("encode chat completions request: %w", err)
	}
	return payload, nil
}

// DecodeResponseEvent translates one frame in isolation. A whole stream
// needs NewStreamDecoder, because tool calls span several frames.
func (c *Codec) DecodeResponseEvent(event wire.SSEEvent) ([]inference.Event, error) {
	return c.NewStreamDecoder().Push(event)
}

// DecodeResponse translates a complete chat completion into canonical events.
func (c *Codec) DecodeResponse(body []byte) ([]inference.Event, error) {
	var envelope chatEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("decode chat completion: %w", err)
	}
	if envelope.Error != nil {
		return []inference.Event{failureEvent(envelope.Error, http.StatusBadGateway)}, nil
	}
	decoder := c.NewStreamDecoder().(*streamDecoder)
	events := decoder.absorb(&envelope)
	closing, err := decoder.Finish()
	if err != nil {
		return nil, err
	}
	return append(events, closing...), nil
}

// DecodeError translates an upstream error body into a canonical error.
func (c *Codec) DecodeError(status int, body []byte) *inference.ErrorInfo {
	var envelope chatEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil || envelope.Error == nil {
		return &inference.ErrorInfo{
			Code:    http.StatusText(status),
			Message: strings.TrimSpace(string(body)),
			Status:  status,
		}
	}
	return envelope.Error.info(status)
}

func (c *Codec) upstreamBase(opts wire.CodecOpts) string {
	if opts.BaseURL != "" {
		return opts.BaseURL
	}
	return c.baseURL
}

func (c *Codec) endpoint(opts wire.CodecOpts) string {
	return strings.TrimSuffix(c.upstreamBase(opts), "/") + chatCompletionsPath
}

type chatRequestPayload struct {
	Model           string          `json:"model"`
	Messages        []chatMessage   `json:"messages"`
	Tools           []chatTool      `json:"tools,omitempty"`
	ToolChoice      json.RawMessage `json:"tool_choice,omitempty"`
	Stream          bool            `json:"stream,omitempty"`
	StreamOptions   *streamOptions  `json:"stream_options,omitempty"`
	MaxTokens       int             `json:"max_tokens,omitempty"`
	Temperature     *float64        `json:"temperature,omitempty"`
	ReasoningEffort string          `json:"reasoning_effort,omitempty"`
	Thinking        *chatThinking   `json:"thinking,omitempty"`
	EnableThinking  *bool           `json:"enable_thinking,omitempty"`
	ThinkingBudget  *int            `json:"thinking_budget,omitempty"`
}

type streamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type chatMessage struct {
	Role             string         `json:"role"`
	Content          any            `json:"content"`
	ToolCalls        []chatToolCall `json:"tool_calls,omitempty"`
	ToolCallID       string         `json:"tool_call_id,omitempty"`
	Name             string         `json:"name,omitempty"`
	ReasoningContent string         `json:"reasoning_content,omitempty"`
}

type chatToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type chatThinking struct {
	Type string `json:"type"`
}

type chatTool struct {
	Type     string `json:"type"`
	Function struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Parameters  json.RawMessage `json:"parameters"`
	} `json:"function"`
}

type chatPart struct {
	Type     string        `json:"type"`
	Text     string        `json:"text,omitempty"`
	ImageURL *chatImageURL `json:"image_url,omitempty"`
}

type chatImageURL struct {
	URL string `json:"url"`
}

func newPayload(req *inference.Request, opts wire.CodecOpts, baseURL string) chatRequestPayload {
	payload := chatRequestPayload{
		Model:       req.Model,
		Messages:    encodeMessages(req.Messages, replayDeepSeekReasoning(baseURL, req, opts)),
		Tools:       encodeTools(req.Tools, baseURL),
		ToolChoice:  encodeToolChoice(req.ToolChoice),
		Stream:      req.Stream,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
	}
	if req.Stream {
		payload.StreamOptions = &streamOptions{IncludeUsage: true}
	}
	applyChatThinking(&payload, req, opts)
	return payload
}

func applyChatThinking(payload *chatRequestPayload, req *inference.Request, opts wire.CodecOpts) {
	if req.Reasoning == nil {
		return
	}
	level, off := inference.ChooseThinking(req.Reasoning.Effort, opts.ReasoningEfforts, opts.ReasoningToggle)
	if qwenThinking(opts.TemplateID) {
		applyQwenThinking(payload, req.Reasoning.Effort, level, off, opts)
		return
	}
	if off {
		payload.Thinking = &chatThinking{Type: "disabled"}
		return
	}
	if level == "" && budgetOnly(opts) {
		if name, ok := inference.NormalizeEffort(req.Reasoning.Effort); ok && name != "none" {
			level = name
		}
	}
	if level != "" {
		payload.ReasoningEffort = level
	}
	if opts.ReasoningToggle && level != "none" && !off {
		payload.Thinking = &chatThinking{Type: "enabled"}
	}
}

func applyQwenThinking(payload *chatRequestPayload, effort, level string, off bool, opts wire.CodecOpts) {
	name, recognized := inference.NormalizeEffort(effort)
	if off || (recognized && name == "none" && level == "") {
		disabled := false
		payload.EnableThinking = &disabled
		return
	}
	if level != "" {
		payload.ReasoningEffort = level
	}
	enabled := true
	payload.EnableThinking = &enabled
	if !opts.ReasoningBudget {
		return
	}
	budgetName := level
	if budgetName == "" {
		budgetName = name
	}
	budget, ok := inference.EffortBudget(budgetName)
	if !ok {
		return
	}
	budget = inference.ClampBudget(budget, opts.ReasoningBudgetMin, opts.ReasoningBudgetMax)
	payload.ThinkingBudget = &budget
}

func budgetOnly(opts wire.CodecOpts) bool {
	return opts.ReasoningBudget && opts.ReasoningEfforts != nil && len(opts.ReasoningEfforts) == 0
}

func qwenThinking(templateID string) bool {
	id := strings.ToLower(strings.TrimSpace(templateID))
	return id == "qwen" || strings.HasPrefix(id, "alibaba")
}

func replayDeepSeekReasoning(baseURL string, req *inference.Request, opts wire.CodecOpts) bool {
	if !deepSeekAPI(baseURL) || len(req.Tools) == 0 {
		return false
	}
	if req.Reasoning == nil {
		return true
	}
	_, off := inference.ChooseThinking(req.Reasoning.Effort, opts.ReasoningEfforts, opts.ReasoningToggle)
	return !off
}

func encodeMessages(messages []inference.Message, replayReasoning bool) []chatMessage {
	encoded := make([]chatMessage, 0, len(messages))
	for _, message := range messages {
		encoded = append(encoded, encodeMessage(message, replayReasoning))
	}
	return encoded
}

func encodeMessage(message inference.Message, replayReasoning bool) chatMessage {
	parts := message.Content
	encoded := chatMessage{
		Role:       message.Role,
		ToolCalls:  encodeToolCalls(message.ToolCalls),
		ToolCallID: message.ToolCallID,
		Name:       message.Name,
	}
	if replayReasoning && message.Role == inference.RoleAssistant {
		encoded.Content = encodeContent(withoutThinking(parts))
		encoded.ReasoningContent = thinkingText(parts)
		if encoded.ReasoningContent == "" {
			encoded.ReasoningContent = deepSeekMissingReasoning
		}
		return encoded
	}
	encoded.Content = encodeContent(parts)
	return encoded
}

func withoutThinking(parts []inference.ContentPart) []inference.ContentPart {
	visible := make([]inference.ContentPart, 0, len(parts))
	for _, part := range parts {
		if part.Type == inference.ContentTypeThinking {
			continue
		}
		visible = append(visible, part)
	}
	return visible
}

func thinkingText(parts []inference.ContentPart) string {
	var builder strings.Builder
	for _, part := range parts {
		if part.Type == inference.ContentTypeThinking {
			builder.WriteString(part.Text)
		}
	}
	return builder.String()
}

func encodeContent(parts []inference.ContentPart) any {
	encoded := make([]chatPart, 0, len(parts))
	textOnly := true
	var builder strings.Builder
	for _, part := range parts {
		switch part.Type {
		case inference.ContentTypeImage:
			textOnly = false
			encoded = append(encoded, chatPart{Type: imageURLPart, ImageURL: &chatImageURL{URL: part.ImageURL}})
		case inference.ContentTypeText:
			spoken := inference.SpokenText([]inference.ContentPart{part})
			if len(spoken) == 0 {
				continue
			}
			builder.WriteString(spoken[0].Text)
			encoded = append(encoded, chatPart{Type: inference.ContentTypeText, Text: spoken[0].Text})
		}
	}
	if len(encoded) == 0 {
		return nil
	}
	if textOnly {
		return builder.String()
	}
	return encoded
}

func encodeToolCalls(calls []inference.ToolCall) []chatToolCall {
	encoded := make([]chatToolCall, 0, len(calls))
	for _, call := range calls {
		entry := chatToolCall{ID: call.ID, Type: functionType}
		entry.Function.Name = call.Name
		entry.Function.Arguments = call.Arguments
		encoded = append(encoded, entry)
	}
	return encoded
}

// encodeToolChoice renders the canonical choice the way the Chat wire names it.
// A client that said nothing about it gets no field, which is what every
// provider on this wire already treats as its own default.
func encodeToolChoice(choice *inference.ToolChoice) json.RawMessage {
	if choice == nil {
		return nil
	}
	if choice.Mode == inference.ToolChoiceTool {
		if choice.Name == "" {
			return nil
		}
		encoded, err := json.Marshal(map[string]any{"type": functionType,
			"function": map[string]string{"name": choice.Name}})
		if err != nil {
			return nil
		}
		return encoded
	}
	encoded, err := json.Marshal(choice.Mode)
	if err != nil {
		return nil
	}
	return encoded
}

func encodeTools(tools []inference.Tool, baseURL string) []chatTool {
	encoded := make([]chatTool, 0, len(tools))
	for _, tool := range tools {
		entry := chatTool{Type: functionType}
		entry.Function.Name = tool.Name
		entry.Function.Description = tool.Description
		entry.Function.Parameters = tool.Parameters
		if deepSeekAPI(baseURL) {
			entry.Function.Parameters = dropNullBytePatterns(tool.Parameters)
		}
		encoded = append(encoded, entry)
	}
	return encoded
}
