// Package bedrock encodes AWS Bedrock requests and decodes its responses,
// including the streaming frames a relay turns back into SSE.
package bedrock

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/jonaskahn/relo/internal/adapters/wire"
	"github.com/jonaskahn/relo/internal/inference"
	"net/http"
	"strings"
)

// Bedrock wire identity is what the Converse endpoint accepts: the family
// name and the regional address.
const (
	ID             = "bedrock-converse"
	DefaultBaseURL = "https://bedrock-runtime.us-east-1.amazonaws.com"
	conversePath   = "/converse"
	streamPath     = "/converse-stream"
	continueText   = "(continue)"
)

// Codec speaks Amazon Bedrock Converse API.
type Codec struct {
	baseURL string
}

var _ wire.CodecModule = (*Codec)(nil)

// NewCodec creates a new Bedrock converse Codec.
func NewCodec(baseURL string) *Codec {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	return &Codec{baseURL: baseURL}
}

// Module returns the default Bedrock converse codec.
func Module() wire.CodecModule {
	return NewCodec(DefaultBaseURL)
}

// EncodeRequest encodes a canonical request to Bedrock Converse request.
func (c *Codec) EncodeRequest(req *inference.Request, opts wire.CodecOpts) (*http.Request, error) {
	payload, err := encodePayload(req, opts)
	if err != nil {
		return nil, fmt.Errorf("encode bedrock converse request: %w", err)
	}

	endpoint := c.endpoint(opts, req)
	httpReq, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("build bedrock request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if req.Stream {
		httpReq.Header.Set("Accept", "application/vnd.amazon.eventstream")
	} else {
		httpReq.Header.Set("Accept", "application/json")
	}

	applyBedrockCredential(httpReq, req, opts)

	for name, value := range opts.ExtraHeaders {
		httpReq.Header.Set(name, value)
	}

	return httpReq, nil
}

func applyBedrockCredential(httpReq *http.Request, req *inference.Request, opts wire.CodecOpts) {
	if opts.CredentialRef == "" {
		return
	}
	if opts.KeyHeader != "" && opts.KeyHeader != "none" {
		wire.ApplyCredential(httpReq, opts)
		return
	}
	httpReq.Header.Set("Authorization", "Bearer "+opts.CredentialRef)
}

func (c *Codec) endpoint(opts wire.CodecOpts, req *inference.Request) string {
	base := c.baseURL
	if opts.BaseURL != "" {
		base = opts.BaseURL
	}
	base = strings.TrimSuffix(base, "/")
	action := conversePath
	if req != nil && req.Stream {
		action = streamPath
	}
	return base + "/model/" + req.Model + action
}

func encodePayload(req *inference.Request, opts wire.CodecOpts) ([]byte, error) {
	system, messages := encodeBedrockMessages(req)
	messages = continueTurn(messages)

	body := map[string]any{
		"messages": messages,
	}
	if len(system) > 0 {
		body["system"] = system
	}
	applyBedrockInferenceConfig(body, req)
	applyBedrockThinking(body, req, opts)
	applyBedrockTools(body, req)

	return json.Marshal(body)
}

func encodeBedrockMessages(req *inference.Request) ([]map[string]string, []map[string]any) {
	var system []map[string]string
	var messages []map[string]any
	for _, msg := range req.Messages {
		if msg.Role == "system" {
			for _, part := range inference.SpokenText(msg.Content) {
				system = append(system, map[string]string{"text": part.Text})
			}
			continue
		}
		if content := encodeBedrockContent(msg); len(content) > 0 {
			messages = append(messages, map[string]any{
				"role":    bedrockRoleOf(msg),
				"content": content,
			})
		}
	}
	return system, messages
}

func bedrockRoleOf(msg inference.Message) string {
	if msg.Role == "user" || msg.Role == "assistant" {
		return msg.Role
	}
	return "user"
}

func encodeBedrockContent(msg inference.Message) []map[string]any {
	var content []map[string]any
	for _, part := range inference.SpokenText(msg.Content) {
		content = append(content, map[string]any{"text": part.Text})
	}
	for _, tc := range msg.ToolCalls {
		var args any
		if err := json.Unmarshal([]byte(tc.Arguments), &args); err != nil {
			args = map[string]any{}
		}
		content = append(content, map[string]any{
			"toolUse": map[string]any{
				"toolUseId": tc.ID,
				"name":      tc.Name,
				"input":     args,
			},
		})
	}
	return appendBedrockToolResult(content, msg)
}

func appendBedrockToolResult(content []map[string]any, msg inference.Message) []map[string]any {
	if msg.ToolCallID == "" {
		return content
	}
	var text string
	for _, p := range msg.Content {
		text += p.Text
	}
	return append(content, map[string]any{
		"toolResult": map[string]any{
			"toolUseId": msg.ToolCallID,
			"content":   []map[string]any{{"text": text}},
			"status":    "success",
		},
	})
}

func applyBedrockInferenceConfig(body map[string]any, req *inference.Request) {
	inferenceConfig := make(map[string]any)
	if req.MaxTokens > 0 {
		inferenceConfig["maxTokens"] = req.MaxTokens
	}
	if req.Temperature != nil {
		inferenceConfig["temperature"] = *req.Temperature
	}
	if len(inferenceConfig) > 0 {
		body["inferenceConfig"] = inferenceConfig
	}
}

func applyBedrockTools(body map[string]any, req *inference.Request) {
	if len(req.Tools) == 0 {
		return
	}
	var tools []map[string]any
	for _, t := range req.Tools {
		var schema any
		_ = json.Unmarshal(t.Parameters, &schema)
		tools = append(tools, map[string]any{
			"toolSpec": map[string]any{
				"name":        t.Name,
				"description": t.Description,
				"inputSchema": map[string]any{"json": schema},
			},
		})
	}
	body["toolConfig"] = map[string]any{"tools": tools}
}

func applyBedrockThinking(body map[string]any, req *inference.Request, opts wire.CodecOpts) {
	if req.Reasoning == nil {
		return
	}
	level, off := inference.ChooseThinking(req.Reasoning.Effort, opts.ReasoningEfforts, opts.ReasoningToggle)
	fields := map[string]any{}
	if claudeBedrock(req.Model) {
		switch {
		case off || (level == "none" && len(opts.ReasoningEfforts) == 0):
			fields["thinking"] = map[string]any{"type": "disabled"}
		case level != "" && len(opts.ReasoningEfforts) > 0:
			fields["output_config"] = map[string]any{"effort": level}
		case level != "":
			budget, _ := inference.EffortBudget(level)
			fields["thinking"] = map[string]any{"type": "enabled", "budget_tokens": budget}
		}
	} else {
		switch {
		case off:
			fields["thinking"] = map[string]any{"type": "disabled"}
		case level != "":
			fields["reasoning_effort"] = level
		case opts.ReasoningToggle:
			fields["thinking"] = map[string]any{"type": "enabled"}
		}
	}
	if len(fields) > 0 {
		body["additionalModelRequestFields"] = fields
	}
}

func claudeBedrock(model string) bool {
	return strings.Contains(strings.ToLower(model), "claude")
}

func continueTurn(messages []map[string]any) []map[string]any {
	if len(messages) == 0 {
		return messages
	}
	if last, ok := messages[len(messages)-1]["role"].(string); !ok || last != inference.RoleAssistant {
		return messages
	}
	return append(messages, map[string]any{
		"role":    inference.RoleUser,
		"content": []map[string]any{{"text": continueText}},
	})
}
