// Integration chat tester: one turn across every client shape Relo accepts.
package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jonaskahn/relo/internal/adapters/wire"
	"github.com/jonaskahn/relo/internal/inference"
)

// Chat-tester refusals name the malformed turns the console maps to its own
// wording: a target that names nothing and messages without usable content.
var (
	ErrChatTargetRequired     = errors.New("a route, or a provider and model, is required")
	ErrChatModelRequired      = errors.New("a provider and model are required")
	ErrChatMessageRequired    = errors.New("at least one message is required")
	ErrChatRoleRequired       = errors.New("messages must be a user or assistant turn, with the system prompt kept separate")
	ErrChatContentRequired    = errors.New("every message needs content")
	ErrChatCanonicalOnly      = errors.New("the chat tester builds its request in canonical form")
	errChatSurfaceUnavailable = errors.New("the requested client surface is unavailable")
)

// The modes the Integrations chat tester runs in: one call straight to a
// chosen connection and model, or one call through a client format Relo
// accepts, so an operator can prove both the credentials and the translation
// a real client would use.
const (
	chatModeDirect = "direct"
	chatModeFormat = "format"
)

// The client shapes a format test can send through. Each names one inbound
// surface Relo already answers on a data plane port.
const (
	chatFormatOpenAIChat = "openai-chat"
	chatFormatOpenAIResp = "openai-responses"
	chatFormatAnthropic  = "anthropic"
	// defaultChatMaxTokens keeps a test that names no ceiling inside what
	// every provider accepts, without leaving the answer truncated.
	defaultChatMaxTokens = 1024
)

type chatTesterRequest struct {
	Mode string `json:"mode"`
	// ProviderID and ModelID name one connection's model, which a direct test
	// and a format test may both target.
	ProviderID string `json:"provider_id"`
	ModelID    string `json:"model_id"`
	// RouteID names a published route instead of one model, which a format
	// test follows the way a client would.
	RouteID string `json:"route_id"`
	// Format is the client shape a format test sends. A direct test ignores
	// it and uses the connection's own codec.
	Format string `json:"format"`
	// System is an optional system prompt the tester prepends.
	System    string `json:"system"`
	MaxTokens int    `json:"max_tokens"`
	// Stream asks the tester to answer frame by frame: the request goes out
	// as a stream, and each text delta is forwarded as it arrives.
	Stream   bool                `json:"stream"`
	Messages []chatTesterMessage `json:"messages"`
}

type chatTesterMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatTesterAnswer struct {
	RequestID    string `json:"request_id"`
	OK           bool   `json:"ok"`
	Text         string `json:"text"`
	Provider     string `json:"provider,omitempty"`
	Model        string `json:"model,omitempty"`
	RouteReason  string `json:"route_reason,omitempty"`
	Status       int    `json:"status"`
	InputTokens  int    `json:"input_tokens"`
	OutputTokens int    `json:"output_tokens"`
	DurationMs   int64  `json:"duration_ms"`
	Error        string `json:"error,omitempty"`
	ErrorCode    string `json:"error_code,omitempty"`
}

func (s *Server) testCanonical(w http.ResponseWriter, r *http.Request, request chatTesterRequest, surface inferenceSurface, target string) (*inference.Request, error) {
	inbound, found := s.inboundFor(surface)
	if request.Mode == chatModeFormat && !found {
		s.handleNotImplemented(w, r)
		return nil, errChatSurfaceUnavailable
	}
	canonical, err := request.canonical(r.Context(), surface, inbound, target)
	if err != nil {
		s.fail(w, r, refusal{Status: http.StatusBadRequest, Code: "bad_request", Detail: err})
		return nil, err
	}
	return canonical, nil
}

func (s *Server) handleIntegrationChat(w http.ResponseWriter, r *http.Request) {
	if !s.relayReady() {
		s.handleNotImplemented(w, r)
		return
	}
	var request chatTesterRequest
	if !s.decodeJSON(w, r, &request) {
		return
	}
	target, err := request.resolveTarget()
	if err != nil {
		s.fail(w, r, refusal{Status: http.StatusBadRequest, Code: "bad_request", Detail: err})
		return
	}
	surface := request.surface()
	canonical, err := s.testCanonical(w, r, request, surface, target)
	if err != nil {
		return
	}
	s.runChatTurn(w, r, request, surface, canonical)
}

func (s *Server) runChatTurn(w http.ResponseWriter, r *http.Request, request chatTesterRequest, surface inferenceSurface, canonical *inference.Request) {
	// The tester and the data plane share one relay, so a test exercises the
	// same eligibility, account selection, failover, and usage recording a
	// client request would.
	capture := &chatCapture{}
	// A turn that streams opens its frames before the relay starts, so the
	// console stops waiting the moment the request is under way. Anything
	// refused before this point is a plain refusal, because no stream began.
	stream := request.openStream(w)
	if stream != nil {
		capture.onEvent = stream.delta
	}
	sink := newChatSink()
	outcome := s.relay(relaySpec{writer: sink, request: r, surface: surface, inbound: capture, canonical: canonical, origin: inference.OriginInternal})
	answer := chatAnswer(outcome, capture, sink)
	if stream == nil {
		writeJSON(w, http.StatusOK, answer)
		return
	}
	stream.finish(answer)
}

func (request chatTesterRequest) resolveTarget() (string, error) {
	route := strings.TrimSpace(request.RouteID)
	provider := strings.TrimSpace(request.ProviderID)
	model := strings.TrimSpace(request.ModelID)
	if request.Mode == chatModeFormat {
		if route != "" {
			return route, nil
		}
		if provider != "" && model != "" {
			return provider + "/" + model, nil
		}
		return "", ErrChatTargetRequired
	}
	if provider == "" || model == "" {
		return "", ErrChatModelRequired
	}
	return provider + "/" + model, nil
}

func (request chatTesterRequest) surface() inferenceSurface {
	switch request.Format {
	case chatFormatOpenAIResp:
		return responsesSurface()
	case chatFormatAnthropic:
		return messagesSurface()
	default:
		return chatCompletionsSurface()
	}
}

func (request chatTesterRequest) openStream(w http.ResponseWriter) *chatStream {
	if !request.Stream {
		return nil
	}
	return newChatStream(w)
}

func (request chatTesterRequest) canonical(ctx context.Context, surface inferenceSurface, inbound wire.InboundCodec, target string) (*inference.Request, error) {
	if len(request.Messages) == 0 {
		return nil, ErrChatMessageRequired
	}
	for _, message := range request.Messages {
		if message.Role != inference.RoleUser && message.Role != inference.RoleAssistant {
			return nil, ErrChatRoleRequired
		}
		if strings.TrimSpace(message.Content) == "" {
			return nil, ErrChatContentRequired
		}
	}
	if request.Mode == chatModeFormat {
		return request.decodeThroughSurface(ctx, surface, inbound, target)
	}
	return request.directRequest(target), nil
}

func (request chatTesterRequest) directRequest(target string) *inference.Request {
	canonical := &inference.Request{
		Model: target, MaxTokens: request.tokenLimit(), Stream: request.Stream,
	}
	if system := strings.TrimSpace(request.System); system != "" {
		canonical.Messages = append(canonical.Messages, chatTextMessage(inference.RoleSystem, system))
	}
	for _, message := range request.Messages {
		canonical.Messages = append(canonical.Messages, chatTextMessage(message.Role, message.Content))
	}
	return canonical
}

func (request chatTesterRequest) decodeThroughSurface(ctx context.Context, surface inferenceSurface, inbound wire.InboundCodec, target string) (*inference.Request, error) {
	body, err := request.clientBody(surface.name, target)
	if err != nil {
		return nil, err
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, "/v1", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	canonical, err := inbound.DecodeRequest(httpRequest)
	if err != nil {
		return nil, err
	}
	// The body carries only the turns; the tester's choice of route or
	// provider model is what routing resolves.
	canonical.Model = target
	canonical.Stream = request.Stream
	return canonical, nil
}

func (request chatTesterRequest) clientBody(surface, target string) ([]byte, error) {
	switch surface {
	case inference.SurfaceResponses:
		return json.Marshal(request.responsesBody(target))
	case inference.SurfaceMessages:
		return json.Marshal(request.messagesBody(target))
	default:
		return json.Marshal(request.chatBody(target))
	}
}

func (request chatTesterRequest) chatBody(target string) map[string]any {
	messages := make([]map[string]any, 0, len(request.Messages)+1)
	if system := strings.TrimSpace(request.System); system != "" {
		messages = append(messages, map[string]any{"role": inference.RoleSystem, "content": system})
	}
	for _, message := range request.Messages {
		messages = append(messages, map[string]any{"role": message.Role, "content": message.Content})
	}
	return map[string]any{
		"model": target, "messages": messages, "max_tokens": request.tokenLimit(), "stream": request.Stream,
	}
}

func (request chatTesterRequest) responsesBody(target string) map[string]any {
	items := make([]map[string]any, 0, len(request.Messages))
	for _, message := range request.Messages {
		items = append(items, map[string]any{
			"type": "message", "role": message.Role,
			"content": []map[string]any{{"type": "input_text", "text": message.Content}},
		})
	}
	body := map[string]any{
		"model": target, "input": items, "max_output_tokens": request.tokenLimit(), "stream": request.Stream,
	}
	if system := strings.TrimSpace(request.System); system != "" {
		body["instructions"] = system
	}
	return body
}

func (request chatTesterRequest) messagesBody(target string) map[string]any {
	messages := make([]map[string]any, 0, len(request.Messages))
	for _, message := range request.Messages {
		messages = append(messages, map[string]any{"role": message.Role, "content": message.Content})
	}
	body := map[string]any{
		"model": target, "max_tokens": request.tokenLimit(), "messages": messages, "stream": request.Stream,
	}
	if system := strings.TrimSpace(request.System); system != "" {
		body["system"] = system
	}
	return body
}

func (request chatTesterRequest) tokenLimit() int {
	if request.MaxTokens > 0 {
		return request.MaxTokens
	}
	return defaultChatMaxTokens
}

func chatTextMessage(role, text string) inference.Message {
	return inference.Message{
		Role:    role,
		Content: []inference.ContentPart{{Type: inference.ContentTypeText, Text: text}},
	}
}

type chatCapture struct {
	events   []inference.Event
	answered bool
	// onEvent is where the events of a streamed turn go as they arrive, which
	// is how the tester forwards text before the answer is complete. It is set
	// only for a turn that asked to stream.
	onEvent func(inference.Event)
}

var _ wire.InboundCodec = (*chatCapture)(nil)

// DecodeRequest is unreachable: the tester builds its request in canonical
// form, so nothing ever decodes a client body into this codec.
func (c *chatCapture) DecodeRequest(*http.Request) (*inference.Request, error) {
	return nil, ErrChatCanonicalOnly
}

// EncodeResponseEvent keeps one event a streaming attempt delivered and
// forwards it to the console, which renders the reply as it is written. The
// codec renders no client body at all: what the console reads is the tester's
// own frame, not one client format's.
func (c *chatCapture) EncodeResponseEvent(event inference.Event) ([]wire.SSEEvent, error) {
	c.events = append(c.events, event)
	if c.onEvent != nil {
		c.onEvent(event)
	}
	return nil, nil
}

// EncodeResponse keeps the events of a completed response and marks the
// attempt as answered, which is how the tester tells a reply from a refusal.
func (c *chatCapture) EncodeResponse(events []inference.Event) ([]byte, error) {
	c.answered = true
	c.events = append(c.events, events...)
	return []byte("{}"), nil
}

type chatStreamFrame struct {
	Type   string            `json:"type"`
	Text   string            `json:"text,omitempty"`
	Answer *chatTesterAnswer `json:"answer,omitempty"`
}

type chatStream struct {
	writer  http.ResponseWriter
	flusher http.Flusher
}

func newChatStream(w http.ResponseWriter) *chatStream {
	stream := &chatStream{writer: w}
	if flusher, ok := w.(http.Flusher); ok {
		stream.flusher = flusher
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	stream.flush()
	return stream
}

func (s *chatStream) delta(event inference.Event) {
	if event.Kind != inference.EventTextDelta || event.Text == "" {
		return
	}
	s.frame(chatStreamFrame{Type: "delta", Text: event.Text})
}

func (s *chatStream) finish(answer chatTesterAnswer) {
	s.frame(chatStreamFrame{Type: "answer", Answer: &answer})
}

func (s *chatStream) frame(frame chatStreamFrame) {
	body, err := json.Marshal(frame)
	if err != nil {
		return
	}
	payload := make([]byte, 0, len(body)+8)
	payload = append(payload, "data: "...)
	payload = append(payload, body...)
	payload = append(payload, '\n', '\n')
	_, _ = s.writer.Write(payload)
	s.flush()
}

func (s *chatStream) flush() {
	if s.flusher != nil {
		s.flusher.Flush()
	}
}

type chatSink struct {
	header http.Header
	status int
	body   bytes.Buffer
}

var _ http.ResponseWriter = (*chatSink)(nil)

func newChatSink() *chatSink {
	return &chatSink{header: http.Header{}}
}

// Header returns the headers the attempt will carry once it is answered.
func (s *chatSink) Header() http.Header { return s.header }

// WriteHeader records the attempt's status, keeping the first one a relay writes.
func (s *chatSink) WriteHeader(status int) {
	if s.status == 0 {
		s.status = status
	}
}

// Write buffers one attempt body for the tester to decode, defaulting the
// status a relay never set to success.
func (s *chatSink) Write(data []byte) (int, error) {
	if s.status == 0 {
		s.status = http.StatusOK
	}
	return s.body.Write(data)
}

// Flush is a no-op: the frames of a streamed attempt are decoded into the
// tester's own answer, so a codec that expects a flusher only needs one to
// exist.
func (s *chatSink) Flush() {}

func (s *chatSink) streamed() bool {
	return strings.Contains(s.header.Get("Content-Type"), "text/event-stream")
}

func chatAnswer(outcome *requestOutcome, capture *chatCapture, sink *chatSink) chatTesterAnswer {
	// An attempt reports its answer either as one encoded body or as the frames
	// of the stream it opened, so both are what tells a reply from a refusal.
	answered := capture.answered || delivered(outcome, sink)
	answer := chatTesterAnswer{
		RequestID:   outcome.requestID,
		OK:          answered,
		Provider:    outcome.providerID,
		Model:       outcome.modelID,
		RouteReason: string(outcome.kind),
		DurationMs:  time.Since(outcome.started).Milliseconds(),
	}
	if answered {
		answer.Status = sink.status
		if answer.Status == 0 {
			answer.Status = http.StatusOK
		}
		answer.Text = capturedText(capture.events)
		answer.InputTokens, answer.OutputTokens = capturedUsage(capture.events)
		return answer
	}
	answer.Status = sink.status
	if answer.Status == 0 {
		answer.Status = http.StatusBadGateway
	}
	answer.ErrorCode, answer.Error = sinkFailure(sink)
	return answer
}

func delivered(outcome *requestOutcome, sink *chatSink) bool {
	if !sink.streamed() || len(outcome.attempts) == 0 {
		return false
	}
	return outcome.attempts[len(outcome.attempts)-1].code == ""
}

func capturedText(events []inference.Event) string {
	var builder strings.Builder
	for _, event := range events {
		if event.Kind == inference.EventTextDelta {
			builder.WriteString(event.Text)
		}
	}
	return builder.String()
}

func capturedUsage(events []inference.Event) (int, int) {
	for _, event := range events {
		if event.Kind == inference.EventUsage && event.Usage != nil {
			return event.Usage.InputTokens, event.Usage.OutputTokens
		}
	}
	return 0, 0
}

func sinkFailure(sink *chatSink) (string, string) {
	// A refusal the management layer writes names its code as a string, and a
	// relayed upstream failure names the status as a number and the code in
	// type, so both shapes are read here.
	var body struct {
		Error struct {
			Code    json.RawMessage `json:"code"`
			Type    string          `json:"type"`
			Message string          `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(sink.body.Bytes(), &body); err == nil && body.Error.Message != "" {
		return chatFailureCode(body.Error.Code, body.Error.Type), body.Error.Message
	}
	if text := strings.TrimSpace(sink.body.String()); text != "" {
		return "error", text
	}
	return "error", "the request did not reach a provider"
}

func chatFailureCode(raw json.RawMessage, fallback string) string {
	var text string
	if err := json.Unmarshal(raw, &text); err == nil && text != "" {
		return text
	}
	if fallback != "" {
		return fallback
	}
	return "error"
}
