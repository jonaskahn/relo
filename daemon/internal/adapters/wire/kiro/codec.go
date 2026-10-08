// Package kiro speaks the Kiro wire protocol: its request encoding and the
// response shapes the relay translates into the canonical format.
package kiro

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/jonaskahn/relo/internal/adapters/wire"
	"github.com/jonaskahn/relo/internal/inference"
	"net/http"
	"strings"
)

// Kiro wire identity is what its endpoint accepts: the family name and
// the regional address.
const (
	ID             = "kiro"
	DefaultBaseURL = "https://codewhisperer.us-east-1.amazonaws.com"
	path           = "/generateAssistantResponse"
)

// Codec speaks Kiro / CodeWhisperer assistant API.
type Codec struct {
	baseURL string
}

var _ wire.CodecModule = (*Codec)(nil)

// NewCodec creates a new Kiro Codec.
func NewCodec(baseURL string) *Codec {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	return &Codec{baseURL: baseURL}
}

// Module returns the default Kiro codec.
func Module() wire.CodecModule {
	return NewCodec(DefaultBaseURL)
}

// EncodeRequest encodes a canonical request to Kiro assistant request.
func (c *Codec) EncodeRequest(req *inference.Request, opts wire.CodecOpts) (*http.Request, error) {
	payload, err := json.Marshal(kiroRequestBody(req, opts))
	if err != nil {
		return nil, fmt.Errorf("encode kiro request: %w", err)
	}

	endpoint := kiroEndpoint(c.baseURL, opts.BaseURL)

	httpReq, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("build kiro request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/vnd.amazon.eventstream")

	if opts.CredentialRef != "" {
		httpReq.Header.Set("Authorization", "Bearer "+opts.CredentialRef)
	}

	for name, value := range opts.ExtraHeaders {
		httpReq.Header.Set(name, value)
	}

	return httpReq, nil
}

func kiroRequestBody(req *inference.Request, opts wire.CodecOpts) map[string]any {
	body := map[string]any{
		"conversationState": map[string]any{
			"currentMessage": map[string]any{
				"userInputMessage": map[string]any{
					"content": extractPrompt(req),
				},
			},
			"chatTriggerType": "MANUAL",
		},
	}
	if opts.Project != "" {
		body["profileArn"] = opts.Project
	}
	return body
}

func kiroEndpoint(baseURL, override string) string {
	endpoint := strings.TrimSuffix(baseURL, "/") + path
	if override != "" {
		endpoint = strings.TrimSuffix(override, "/") + path
	}
	return endpoint
}

func extractPrompt(req *inference.Request) string {
	var parts []string
	for _, msg := range req.Messages {
		var msgText []string
		for _, p := range msg.Content {
			if p.Type == inference.ContentTypeText && p.Text != "" {
				msgText = append(msgText, p.Text)
			}
		}
		if len(msgText) > 0 {
			parts = append(parts, msg.Role+": "+strings.Join(msgText, " "))
		}
	}
	return strings.Join(parts, "\n\n")
}

// DecodeResponse decodes a complete Kiro response body.
func (c *Codec) DecodeResponse(body []byte) ([]inference.Event, error) {
	var resp struct {
		Message string `json:"message,omitempty"`
		Content string `json:"content,omitempty"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode kiro response: %w", err)
	}

	if resp.Message != "" {
		return kiroFailureEvents(resp.Message), nil
	}

	events := []inference.Event{
		{Kind: inference.EventTextDelta, Text: resp.Content},
		{Kind: inference.EventTerminal, Terminal: &inference.TerminalInfo{Reason: inference.EventReasonStop}},
	}
	return events, nil
}

func kiroFailureEvents(message string) []inference.Event {
	return []inference.Event{
		{
			Kind: inference.EventError,
			Error: &inference.ErrorInfo{
				Code:    "upstream_error",
				Message: message,
				Status:  http.StatusBadGateway,
			},
		},
		{
			Kind: inference.EventTerminal,
			Terminal: &inference.TerminalInfo{
				Reason: inference.EventReasonError,
			},
		},
	}
}

// DecodeResponseEvent decodes a single event.
func (c *Codec) DecodeResponseEvent(event wire.SSEEvent) ([]inference.Event, error) {
	return c.NewStreamDecoder().Push(event)
}

// DecodeError translates an upstream error body into a canonical error.
func (c *Codec) DecodeError(status int, body []byte) *inference.ErrorInfo {
	var resp struct {
		Message string `json:"message"`
	}
	_ = json.Unmarshal(body, &resp)
	msg := resp.Message
	if msg == "" {
		msg = strings.TrimSpace(string(body))
	}
	return &inference.ErrorInfo{
		Code:    http.StatusText(status),
		Message: msg,
		Status:  status,
	}
}

// NewStreamDecoder opens a Kiro stream session, holding the usage the
// vendor reports until the stream ends.
func (c *Codec) NewStreamDecoder() wire.StreamDecoder {
	return &streamDecoder{}
}

type streamDecoder struct {
	finished bool
}

// Push translates one Kiro stream frame into canonical events, skipping
// heartbeats and undecodable frames the relay must not fail on.
func (s *streamDecoder) Push(event wire.SSEEvent) ([]inference.Event, error) {
	if event.Data == "" || event.Data == "[DONE]" {
		return nil, nil
	}

	var chunk struct {
		AssistantResponseEvent *struct {
			Content string `json:"content"`
		} `json:"assistantResponseEvent,omitempty"`
	}
	if err := json.Unmarshal([]byte(event.Data), &chunk); err != nil {
		return nil, nil
	}

	if chunk.AssistantResponseEvent != nil && chunk.AssistantResponseEvent.Content != "" {
		return []inference.Event{
			{
				Kind: inference.EventTextDelta,
				Text: chunk.AssistantResponseEvent.Content,
			},
		}, nil
	}

	return nil, nil
}

// Finish closes the Kiro stream, emitting nothing further once it has ended.
func (s *streamDecoder) Finish() ([]inference.Event, error) {
	if s.finished {
		return nil, nil
	}
	s.finished = true
	return []inference.Event{
		{
			Kind:     inference.EventTerminal,
			Terminal: &inference.TerminalInfo{Reason: inference.EventReasonStop},
		},
	}, nil
}
