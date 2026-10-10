// Gemini assist replay: Claude-shaped requests on Antigravity.
package google

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jonaskahn/relo/internal/adapters/antigravity"
	"github.com/jonaskahn/relo/internal/inference"
)

const (
	assistMaxTokens        = 65536
	continueText           = "(continue)"
	thoughtSignatureBypass = "skip_thought_signature_validator"
	assistToolMode         = "VALIDATED"
	replayCap              = 1024
	minSignatureLen        = 16
)

var geminiWirePattern = regexp.MustCompile(`(?i)^gemini[-.\d]`)

var foreignSignature = regexp.MustCompile(`(?i)^(fc|ctc|tsc|call|msg|rs|resp|reasoning|item|ws|toolu|tool|func|function)[-_]`)

type toolConfig struct {
	FunctionCallingConfig *functionCallingConfig `json:"functionCallingConfig,omitempty"`
}

type functionCallingConfig struct {
	Mode string `json:"mode,omitempty"`
}

type replayScope struct {
	model   string
	session string
	record  bool
}

// replayCache is the bounded assist-signature memory: recent thought
// signatures keyed by model, session, call, and arguments, oldest first.
type replayCache struct {
	mu      sync.Mutex
	entries map[string]string
	order   []string
	max     int
}

// newReplayCache opens an empty signature memory holding at most max entries.
func newReplayCache(max int) *replayCache {
	return &replayCache{entries: make(map[string]string), max: max}
}

// defaultReplayCache is the process-wide signature memory the assist codec
// reads. All access goes through its methods, so eviction lives in one place.
var defaultReplayCache = newReplayCache(replayCap)

// The endpoint names the execution that produced the last answer and expects
// the next turn to name it back, which is what keeps a multi-turn
// conversation attached to the reasoning it already did. A refused turn
// leaves the pointer where it was, so a retried one still refers to the
// execution that actually answered.
var defaultExecutions = newReplayCache(replayCap)

func storeExecution(session, responseID string) {
	defaultExecutions.store("execution\x00"+session, responseID, session)
}

func executionFor(session string) string {
	return defaultExecutions.lookup("execution\x00" + session)
}

// rememberExecution files the answer's execution under its session once the
// stream closes successfully.
func (d *streamDecoder) rememberExecution() {
	if d.responseID == "" || d.replay.session == "" || d.failure != nil {
		return
	}
	storeExecution(d.replay.session, d.responseID)
}

func claudeOnAntigravity(model string) bool {
	return strings.Contains(strings.ToLower(model), "claude")
}

func geminiWire(model string) bool {
	afterTransport := model[strings.LastIndex(model, ":")+1:]
	wireModel := afterTransport[strings.LastIndex(afterTransport, "/")+1:]
	return geminiWirePattern.MatchString(wireModel)
}

func usableSignature(signature string) string {
	if len(signature) < minSignatureLen || signature == thoughtSignatureBypass || foreignSignature.MatchString(signature) {
		return ""
	}
	for _, r := range signature {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '+' || r == '/' || r == '_' || r == '-' || r == '=':
		default:
			return ""
		}
	}
	return signature
}

func sessionPreimage(anchor string, req *inference.Request) string {
	if anchor != "" {
		return anchor
	}
	return firstUserText(req.Messages)
}

func firstUserText(messages []inference.Message) string {
	for _, message := range messages {
		if message.Role != inference.RoleUser {
			continue
		}
		for _, block := range message.Content {
			if block.Type == inference.ContentTypeText && block.Text != "" {
				return block.Text
			}
		}
	}
	return ""
}

func sessionID(preimage string) string {
	if preimage == "" {
		return randomSessionID()
	}
	sum := sha256.Sum256([]byte(preimage))
	value := binary.BigEndian.Uint64(sum[:8]) & 0x7fffffffffffffff
	return "-" + strconv.FormatUint(value, 10)
}

func randomSessionID() string {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		binary.BigEndian.PutUint64(buf[:], uint64(time.Now().UnixNano()))
	}
	value := binary.BigEndian.Uint64(buf[:]) & 0x7fffffffffffffff
	return "-" + strconv.FormatUint(value, 10)
}

// The envelope names two identities per conversation, and the endpoint gates
// its models on both staying the same across a session. Both are derived from
// the preimage rather than stored, so a conversation keeps them without the
// codec holding any session memory; a client that sends no preimage falls back
// to fresh ids per request, which the endpoint reads as a new session.
const (
	agentDomain      = "agent"
	trajectoryDomain = "trajectory"
)

func sessionUUID(preimage, domain string) string {
	if preimage == "" {
		return newID()
	}
	sum := sha256.Sum256([]byte(domain + "\x00" + preimage))
	return fmt.Sprintf("%x-%x-%x-%x-%x", sum[0:4], sum[4:6], sum[6:8], sum[8:10], sum[10:16])
}

// firstStep is where the endpoint expects a conversation's step count to
// start, so the first turn reports itself as the second.
const firstStep = 2

// userTurns counts the user turns a request carries. It reads the request
// rather than the encoded contents, which may have gained the synthetic
// continue turn and would then report a step the conversation never took.
func userTurns(req *inference.Request) int {
	turns := 0
	for _, message := range req.Messages {
		if message.Role == inference.RoleUser {
			turns++
		}
	}
	if turns < 1 {
		return firstStep
	}
	return turns + 1
}

func replayKey(model, session, name, args string) string {
	return model + "\x00" + session + "\x00" + name + "\x00" + args
}

func storeSignature(model, session, name, args, signature string) {
	defaultReplayCache.store(replayKey(model, session, name, args), signature, session)
}

// store remembers one signature under its key, evicting the oldest once the
// memory is full. An empty signature or session stores nothing.
func (c *replayCache) store(key, signature, session string) {
	if signature == "" || session == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, found := c.entries[key]; !found {
		c.order = append(c.order, key)
	}
	c.entries[key] = signature
	for len(c.order) > c.max {
		oldest := c.order[0]
		c.order = c.order[1:]
		delete(c.entries, oldest)
	}
}

func lookupSignature(model, session, name, args string) string {
	return defaultReplayCache.lookup(replayKey(model, session, name, args))
}

// lookup returns the signature remembered under one key, if any.
func (c *replayCache) lookup(key string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.entries[key]
}

func rememberSignatures(scope replayScope, parts []part, carried string) string {
	if !scope.record {
		return carried
	}
	for _, item := range parts {
		signature := usableSignature(item.ThoughtSignature)
		if item.FunctionCall == nil {
			if signature != "" && item.Thought {
				carried = signature
			}
			continue
		}
		if signature == "" {
			signature = usableSignature(carried)
		}
		carried = ""
		if signature == "" {
			continue
		}
		storeSignature(scope.model, scope.session, item.FunctionCall.Name, string(item.FunctionCall.Args), signature)
	}
	return carried
}

func encodeAssist(req *inference.Request, anchor string) ([]content, error) {
	session := sessionID(sessionPreimage(anchor, req))
	names := map[string]string{}
	encoded := make([]content, 0, len(req.Messages))
	for index := 0; index < len(req.Messages); index++ {
		next, err := encodeAssistMessage(req, index, session, names, &encoded)
		if err != nil {
			return nil, err
		}
		index = next
	}
	if len(encoded) == 0 || encoded[len(encoded)-1].Role == roleModel {
		encoded = append(encoded, content{Role: roleUser, Parts: []part{{Text: continueText}}})
	}
	return encoded, nil
}

func encodeAssistMessage(req *inference.Request, index int, session string, names map[string]string, encoded *[]content) (int, error) {
	message := req.Messages[index]
	if message.Role == inference.RoleSystem {
		return index, nil
	}
	rememberCalls(message, names)
	if message.Role == inference.RoleTool {
		*encoded = append(*encoded, orphanTool(message))
		return index, nil
	}
	parts, err := assistParts(message, req.Model, session)
	if err != nil {
		return index, err
	}
	if len(parts) == 0 {
		return index, nil
	}
	*encoded = append(*encoded, content{Role: contentRole(message.Role), Parts: parts})
	if message.Role != inference.RoleAssistant || len(message.ToolCalls) == 0 {
		return index, nil
	}
	batch, next := toolBatch(req.Messages, index, names, claudeOnAntigravity(req.Model))
	*encoded = append(*encoded, content{Role: roleUser, Parts: batch})
	return next, nil
}

func assistParts(message inference.Message, model, session string) ([]part, error) {
	claude := claudeOnAntigravity(model)
	parts := make([]part, 0, len(message.Content)+len(message.ToolCalls))
	var pending string
	for _, block := range message.Content {
		if block.Type == inference.ContentTypeThinking {
			parts, pending = appendThinkingPart(parts, pending, block, claude)
			continue
		}
		encoded, err := encodePart(block)
		if err != nil {
			return nil, err
		}
		if encoded == nil {
			continue
		}
		parts, pending = appendContentPart(parts, pending, message, block, encoded)
	}
	return append(parts, assistCalls(message.ToolCalls, model, session, pending)...), nil
}

func appendThinkingPart(parts []part, pending string, block inference.ContentPart, claude bool) ([]part, string) {
	signature := usableSignature(block.Signature)
	if claude && signature != "" {
		return append(parts, part{Text: block.Text, Thought: true, ThoughtSignature: signature}), pending
	}
	if signature != "" {
		pending = signature
	}
	return parts, pending
}

func appendContentPart(parts []part, pending string, message inference.Message, block inference.ContentPart, encoded *part) ([]part, string) {
	if message.Role == inference.RoleAssistant && encoded.Text != "" {
		if signature := usableSignature(block.Signature); signature != "" {
			encoded.ThoughtSignature = signature
		} else if pending != "" && len(message.ToolCalls) == 0 {
			encoded.ThoughtSignature = pending
			pending = ""
		}
	}
	return append(parts, *encoded), pending
}

func assistCalls(calls []inference.ToolCall, model, session, pending string) []part {
	if len(calls) == 0 {
		return nil
	}
	claude := claudeOnAntigravity(model)
	parts := make([]part, 0, len(calls))
	for index, call := range calls {
		encoded := part{FunctionCall: &functionCall{Name: call.Name, Args: toolArguments(call.Arguments)}}
		if claude {
			encoded.FunctionCall.ID = claudeCallID(call.ID, call.Name)
		}
		if index == 0 && pending != "" {
			encoded.ThoughtSignature = pending
		}
		if encoded.ThoughtSignature == "" && geminiWire(model) {
			if signature := lookupSignature(model, session, call.Name, string(encoded.FunctionCall.Args)); signature != "" {
				encoded.ThoughtSignature = signature
			}
		}
		parts = append(parts, encoded)
	}
	if parts[0].ThoughtSignature == "" && geminiWire(model) {
		parts[0].ThoughtSignature = thoughtSignatureBypass
	}
	return parts
}

func followingToolResults(messages []inference.Message, callIndex int) ([]inference.Message, int) {
	var following []inference.Message
	next := callIndex + 1
	for next < len(messages) && messages[next].Role == inference.RoleTool {
		following = append(following, messages[next])
		next++
	}
	return following, next
}

func indexToolResults(following []inference.Message) map[string]inference.Message {
	byID := map[string]inference.Message{}
	for _, result := range following {
		if _, seen := byID[result.ToolCallID]; !seen {
			byID[result.ToolCallID] = result
		}
	}
	return byID
}

func toolBatch(messages []inference.Message, callIndex int, names map[string]string, claude bool) ([]part, int) {
	call := messages[callIndex]
	following, next := followingToolResults(messages, callIndex)
	byID := indexToolResults(following)
	used := map[string]bool{}
	parts := make([]part, 0, len(call.ToolCalls)+len(following))
	for _, toolCall := range call.ToolCalls {
		if result, found := byID[toolCall.ID]; found {
			parts = append(parts, assistResponse(result, names, claude))
			used[toolCall.ID] = true
			continue
		}
		parts = append(parts, missingResponse(toolCall, claude))
	}
	for _, result := range following {
		if used[result.ToolCallID] {
			continue
		}
		text := textOf(result.Content)
		if text == "" {
			text = "(tool result)"
		}
		parts = append(parts, part{Text: text})
	}
	return parts, next - 1
}

func assistResponse(message inference.Message, names map[string]string, claude bool) part {
	response := &functionResponse{Name: toolName(message, names), Response: functionResult(textOf(message.Content))}
	if claude {
		response.ID = claudeCallID(message.ToolCallID, response.Name)
	}
	return part{FunctionResponse: response}
}

func missingResponse(call inference.ToolCall, claude bool) part {
	response := &functionResponse{Name: call.Name, Response: map[string]any{"result": ""}}
	if claude {
		response.ID = claudeCallID(call.ID, call.Name)
	}
	return part{FunctionResponse: response}
}

func orphanTool(message inference.Message) content {
	text := textOf(message.Content)
	if text == "" {
		text = "(tool result)"
	}
	return content{Role: roleUser, Parts: []part{{Text: text}}}
}

func claudeCallID(raw, name string) string {
	if raw == "" {
		raw = name
	}
	var builder strings.Builder
	rewritten := false
	for _, r := range raw {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
			builder.WriteRune(r)
		default:
			rewritten = true
			builder.WriteByte('_')
		}
	}
	if !rewritten {
		return builder.String()
	}
	sum := sha256.Sum256([]byte(raw))
	return builder.String() + "_" + hex.EncodeToString(sum[:4])
}

// assistTurn is the per-request context the envelope is finished against: the
// conversation it belongs to and the wire SKU its thinking effort collapses
// onto.
type assistTurn struct {
	anchor  string
	session string
	variant antigravity.Variant
}

// sessionIdentity is the pair of conversation-scoped ids the envelope names,
// and the turn this request falls at within it.
type sessionIdentity struct {
	agent      string
	trajectory string
	step       int
}

func finishAssist(payload *requestPayload, req *inference.Request, turn assistTurn) sessionIdentity {
	if payload.SystemInstruction != nil {
		payload.SystemInstruction.Role = roleUser
	}
	if payload.GenerationConfig == nil {
		payload.GenerationConfig = &generationConfig{}
	}
	capOutput := outputCeiling(req, turn.variant)
	switch {
	case payload.GenerationConfig.MaxOutputTokens == 0 || payload.GenerationConfig.MaxOutputTokens > capOutput:
		payload.GenerationConfig.MaxOutputTokens = capOutput
	}
	applyAssistThinking(payload, req)
	preimage := sessionPreimage(turn.anchor, req)
	identity := sessionIdentity{
		agent:      sessionUUID(preimage, agentDomain),
		trajectory: sessionUUID(preimage, trajectoryDomain),
		step:       userTurns(req),
	}
	payload.SessionID = sessionID(preimage)
	payload.Labels = assistLabels(req.Model, identity.trajectory, identity.step, turn)
	return identity
}

// applyAssistThinking gives each model family the thinking shape it accepts.
// A Claude turn carries none, because the vendor bills reasoning through its
// own beta; a Gemini turn with no stated effort is left open-ended.
func applyAssistThinking(payload *requestPayload, req *inference.Request) {
	switch {
	case claudeOnAntigravity(req.Model):
		payload.GenerationConfig.ThinkingConfig = nil
		if len(payload.Tools) > 0 {
			payload.ToolConfig = &toolConfig{FunctionCallingConfig: &functionCallingConfig{Mode: assistToolMode}}
		}
	case geminiWire(req.Model):
		if payload.GenerationConfig.ThinkingConfig == nil {
			open := -1
			payload.GenerationConfig.ThinkingConfig = &thinkingConfig{ThinkingBudget: &open, IncludeThoughts: true}
		}
	}
}

// outputCeiling reports the largest completion the vendor serves on this
// request. A caller's own ask and the routed model's catalog ceiling both
// narrow it, never widen it, because the endpoint refuses a larger one with a
// 400 the client cannot recover from.
func outputCeiling(req *inference.Request, variant antigravity.Variant) int {
	ceiling := assistMaxTokens
	if variant.MaxOutput > 0 && variant.MaxOutput < ceiling {
		ceiling = variant.MaxOutput
	}
	if req.MaxTokens > 0 && req.MaxTokens < ceiling {
		ceiling = req.MaxTokens
	}
	return ceiling
}

func assistLabels(model, trajectory string, step int, turn assistTurn) map[string]string {
	if step < 1 {
		step = 1
	}
	labels := map[string]string{
		"last_step_index":          strconv.Itoa(step - 1),
		"trajectory_id":            trajectory,
		"used_claude":              boolLabel(claudeOnAntigravity(model)),
		"used_claude_conservative": "false",
	}
	// The first turn has no execution to name back, so the label is left off
	// rather than filled with an id the endpoint never issued.
	if execution := executionFor(turn.session); execution != "" {
		labels["last_execution_id"] = execution
	}
	if turn.variant.ModelEnum != "" {
		labels["model_enum"] = turn.variant.ModelEnum
	}
	return labels
}

func boolLabel(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

// assistRequestID names the request the way the first-party client does: the
// agent that is speaking, when, the conversation it belongs to, and how far
// into that conversation this turn is.
func assistRequestID(agent, trajectory string, step int) string {
	return fmt.Sprintf("agent/%s/%d/%s/%d", agent, time.Now().UnixMilli(), trajectory, step)
}

func newID() string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		binary.BigEndian.PutUint64(raw[:8], uint64(time.Now().UnixNano()))
	}
	raw[6] = (raw[6] & 0x0f) | 0x40
	raw[8] = (raw[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", raw[0:4], raw[4:6], raw[6:8], raw[8:10], raw[10:])
}
