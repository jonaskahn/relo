// Cursor conversation history: rebuilding state from canonical messages.
package cursor

import (
	"crypto/sha256"
	"encoding/json"
	"strings"

	"github.com/jonaskahn/relo/internal/adapters/cursor"
	"github.com/jonaskahn/relo/internal/inference"
)

// History field numbers inside the conversation state.
const (
	stateRootPrompts = 1
	stateTurns       = 8

	turnUserMessage = 1
	turnSteps       = 2

	stepAssistant = 1
	stepToolCall  = 2

	assistantText = 1

	toolCallIDField = 57
	toolCallMCP     = 15
	mcpArgsField    = 1
	mcpResultField  = 2
	mcpArgName      = 1
	mcpArgValues    = 2
	mcpArgCallID    = 3
	mcpArgProvider  = 4
	mcpArgToolName  = 5
	mapEntryKey     = 1
	mapEntryValue   = 2
	mcpTextCase     = 1
	mcpContentText  = 1
)

// Store is an attempt-owned blob store. Blob ids are the SHA-256 of their
// content, matching the reference client, so a resumed id always names the
// same bytes.
type Store struct {
	blobs map[string][]byte
}

// NewStore returns an empty blob store.
func NewStore() *Store {
	return &Store{blobs: map[string][]byte{}}
}

// Put stores data and returns its blob id.
func (s *Store) Put(data []byte) []byte {
	sum := sha256.Sum256(data)
	id := sum[:]
	s.blobs[string(id)] = data
	return id
}

// PutID stores data under a server-provided id.
func (s *Store) PutID(id, data []byte) {
	if len(id) == 0 {
		return
	}
	s.blobs[string(id)] = data
}

// Get returns the blob one id names.
func (s *Store) Get(id []byte) ([]byte, bool) {
	blob, found := s.blobs[string(id)]
	return blob, found
}

// History is one rebuilt conversation state with the store its blob ids
// point into.
type History struct {
	State []byte
	Rules [][]byte
	Store *Store
}

// BuildHistory rebuilds the conversation state from canonical messages.
// The trailing user message rides in the action instead; everything else,
// including trailing tool results, is replayed from history.
func BuildHistory(req *inference.Request, tools *ToolMap) (History, []byte, error) {
	store := NewStore()
	rules := buildSystemRules(req)
	active, cutoff := activeUserMessage(req)
	root, err := buildRootPrompt(req, tools, store, cutoff)
	if err != nil {
		return History{}, nil, err
	}
	turns, err := buildTurns(req, tools, store, cutoff)
	if err != nil {
		return History{}, nil, err
	}
	var state cursor.Writer
	for _, id := range root {
		state.Bytes(stateRootPrompts, id)
	}
	for _, id := range turns {
		state.Bytes(stateTurns, id)
	}
	var action []byte
	if active != nil {
		action = buildUserAction(messageText(*active))
	} else {
		action = buildResumeAction()
	}
	return History{State: state.BytesOut(), Rules: rules, Store: store}, action, nil
}

func buildSystemRules(req *inference.Request) [][]byte {
	var rules [][]byte
	index := 0
	for _, message := range req.Messages {
		if message.Role != inference.RoleSystem {
			continue
		}
		if text := messageText(message); strings.TrimSpace(text) != "" {
			rules = append(rules, buildRule(systemRulePath(index), text))
			index++
		}
	}
	return rules
}

func systemRulePath(index int) string {
	return "/relo/system-prompt/" + itoa(index) + ".mdc"
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	var digits []byte
	for value > 0 {
		digits = append([]byte{byte('0' + value%10)}, digits...)
		value /= 10
	}
	return string(digits)
}

// activeUserMessage finds the trailing user message the action carries. It
// reports the message and the history cutoff: messages before the last user
// message. A trailing tool result means a resume over the full history.
func activeUserMessage(req *inference.Request) (*inference.Message, int) {
	lastUser := -1
	for index, message := range req.Messages {
		if message.Role == inference.RoleUser {
			lastUser = index
		}
	}
	if lastUser < 0 {
		return nil, len(req.Messages)
	}
	if lastUser != len(req.Messages)-1 {
		return nil, len(req.Messages)
	}
	candidate := req.Messages[lastUser]
	if messageText(candidate) == "" && !messageHasImages(candidate) {
		return nil, len(req.Messages)
	}
	return &candidate, lastUser
}

func messageText(message inference.Message) string {
	var parts []string
	for _, part := range message.Content {
		if part.Type == inference.ContentTypeText && strings.TrimSpace(part.Text) != "" {
			parts = append(parts, strings.TrimSpace(part.Text))
		}
	}
	return strings.Join(parts, "\n")
}

func messageHasImages(message inference.Message) bool {
	for _, part := range message.Content {
		if part.Type == inference.ContentTypeImage {
			return true
		}
	}
	return false
}

func buildRootPrompt(req *inference.Request, tools *ToolMap, store *Store, cutoff int) ([][]byte, error) {
	var ids [][]byte
	calls := map[string]string{}
	emitted := map[string]bool{}
	push := func(value any) []byte {
		raw, _ := json.Marshal(value)
		return store.Put(raw)
	}
	for index := 0; index < cutoff && index < len(req.Messages); index++ {
		message := req.Messages[index]
		switch message.Role {
		case inference.RoleUser:
			if text := messageText(message); text != "" {
				ids = append(ids, push(map[string]any{"role": "user", "content": []any{map[string]any{"type": "text", "text": text}}}))
			}
		case inference.RoleAssistant:
			rounds := assistantRounds(message)
			for round, step := range rounds {
				ids = append(ids, push(map[string]any{"role": "assistant", "content": step}))
				if round == len(rounds)-1 {
					continue
				}
				for _, result := range roundResults(req, index, cutoff, step, calls) {
					if emitted[result.id] {
						continue
					}
					emitted[result.id] = true
					ids = append(ids, push(result.entry))
				}
			}
			for _, call := range assistantCalls(message) {
				calls[normalizeToolCallID(call.ID)] = call.Name
			}
		case inference.RoleTool:
			if emitted[message.ToolCallID] {
				continue
			}
			name, paired := calls[normalizeToolCallID(message.ToolCallID)]
			if !paired {
				if text := toolResultText(message); text != "" {
					ids = append(ids, push(map[string]any{"role": "assistant", "content": []any{map[string]any{"type": "text", "text": text}}}))
				}
				continue
			}
			emitted[message.ToolCallID] = true
			ids = append(ids, push(toolResultEntry(message, name)))
		}
	}
	return ids, nil
}

type roundResult struct {
	id    string
	entry map[string]any
}

func roundResults(req *inference.Request, assistant, cutoff int, step []any, calls map[string]string) []roundResult {
	callIDs := map[string]bool{}
	for _, part := range step {
		item, ok := part.(map[string]any)
		if !ok || item["type"] != "tool-call" {
			continue
		}
		if id, ok := item["toolCallId"].(string); ok {
			callIDs[id] = true
		}
	}
	var results []roundResult
	for index := assistant + 1; index < cutoff && index < len(req.Messages); index++ {
		message := req.Messages[index]
		if message.Role != inference.RoleTool || !callIDs[normalizeToolCallID(message.ToolCallID)] {
			continue
		}
		name, paired := calls[normalizeToolCallID(message.ToolCallID)]
		if !paired {
			name = ""
		}
		results = append(results, roundResult{id: message.ToolCallID, entry: toolResultEntry(message, name)})
	}
	return results
}

func toolResultEntry(message inference.Message, name string) map[string]any {
	return map[string]any{
		"role": "tool",
		"id":   normalizeToolCallID(message.ToolCallID),
		"content": []any{map[string]any{
			"type": "tool-result", "toolName": name,
			"toolCallId": normalizeToolCallID(message.ToolCallID),
			"result":     toolResultText(message),
		}},
	}
}

func toolResultText(message inference.Message) string {
	return messageText(inference.Message{Content: message.Content})
}

func toolResultIsError(message inference.Message) bool {
	return strings.HasPrefix(strings.TrimSpace(message.Name), "error")
}

func assistantRounds(message inference.Message) [][]any {
	var rounds [][]any
	step := []any{}
	flush := func() {
		if len(step) > 0 {
			rounds = append(rounds, step)
			step = []any{}
		}
	}
	items := assistantItems(message)
	for _, item := range items {
		if kind, _ := item["kind"].(string); kind != "tool-call" && hasRoundCalls(step) {
			flush()
		}
		step = append(step, item["entry"])
	}
	flush()
	return rounds
}

func hasRoundCalls(step []any) bool {
	for _, part := range step {
		if item, ok := part.(map[string]any); ok && item["type"] == "tool-call" {
			return true
		}
	}
	return false
}

func assistantItems(message inference.Message) []map[string]any {
	var items []map[string]any
	for _, part := range message.Content {
		if part.Type == inference.ContentTypeText && strings.TrimSpace(part.Text) != "" {
			items = append(items, map[string]any{"kind": "text", "entry": map[string]any{"type": "text", "text": strings.TrimSpace(part.Text)}})
		}
	}
	for _, call := range message.ToolCalls {
		items = append(items, map[string]any{"kind": "tool-call", "entry": map[string]any{
			"type": "tool-call", "toolCallId": normalizeToolCallID(call.ID),
			"toolName": call.Name, "args": toolArgsOf(call.Arguments),
		}})
	}
	return items
}

func assistantCalls(message inference.Message) []inference.ToolCall {
	return message.ToolCalls
}

func toolArgsOf(raw string) map[string]any {
	args := map[string]any{}
	if raw == "" {
		return args
	}
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return map[string]any{}
	}
	return args
}

func buildTurns(req *inference.Request, tools *ToolMap, store *Store, cutoff int) ([][]byte, error) {
	var ids [][]byte
	index := 0
	for index < cutoff && index < len(req.Messages) {
		message := req.Messages[index]
		if message.Role != inference.RoleUser {
			index++
			continue
		}
		if text := messageText(message); text == "" && !messageHasImages(message) {
			index++
			continue
		}
		var user cursor.Writer
		user.String(userMessageText, messageText(message))
		user.String(2, turnMessageID())
		userBlob := store.Put(user.BytesOut())
		var steps [][]byte
		index++
		calls := map[string]inference.ToolCall{}
		var orphans []inference.Message
		for index < cutoff && index < len(req.Messages) && req.Messages[index].Role != inference.RoleUser {
			step := req.Messages[index]
			switch step.Role {
			case inference.RoleAssistant:
				for _, part := range step.Content {
					if part.Type == inference.ContentTypeText && strings.TrimSpace(part.Text) != "" {
						steps = append(steps, assistantStep(store, strings.TrimSpace(part.Text)))
					}
				}
				for _, call := range step.ToolCalls {
					calls[normalizeToolCallID(call.ID)] = call
				}
			case inference.RoleTool:
				if _, paired := calls[normalizeToolCallID(step.ToolCallID)]; !paired {
					orphans = append(orphans, step)
				}
			}
			index++
		}
		for _, call := range turnCalls(req, tools, calls) {
			steps = append(steps, call)
		}
		for _, orphan := range orphans {
			if text := toolResultText(orphan); text != "" {
				steps = append(steps, assistantStep(store, text))
			}
		}
		var turn cursor.Writer
		turn.Bytes(turnUserMessage, userBlob)
		for _, step := range steps {
			turn.Bytes(turnSteps, step)
		}
		var wrapped cursor.Writer
		wrapped.Message(1, turn.BytesOut())
		ids = append(ids, store.Put(wrapped.BytesOut()))
	}
	return ids, nil
}

func assistantStep(store *Store, text string) []byte {
	var assistant cursor.Writer
	assistant.String(assistantText, text)
	var step cursor.Writer
	step.Message(stepAssistant, assistant.BytesOut())
	return store.Put(step.BytesOut())
}

func turnCalls(req *inference.Request, tools *ToolMap, calls map[string]inference.ToolCall) [][]byte {
	_ = req
	var steps [][]byte
	names := make([]string, 0, len(calls))
	for id := range calls {
		names = append(names, id)
	}
	sortStrings(names)
	for _, id := range names {
		call := calls[id]
		result, found := pairResult(req, call.ID)
		steps = append(steps, toolCallStep(call, result, found, tools))
	}
	return steps
}

func pairResult(req *inference.Request, callID string) (inference.Message, bool) {
	for _, message := range req.Messages {
		if message.Role == inference.RoleTool && normalizeToolCallID(message.ToolCallID) == normalizeToolCallID(callID) {
			return message, true
		}
	}
	return inference.Message{}, false
}

func sortStrings(values []string) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j] < values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}

// toolCallStep encodes one history tool-call step with its paired result.
func toolCallStep(call inference.ToolCall, result inference.Message, paired bool, tools *ToolMap) []byte {
	args := buildMCPArgs(call, tools)
	if paired {
		var completed cursor.Writer
		completed.Message(mcpArgsField, args)
		completed.Message(mcpResultField, buildHistoryResult(result))
		var mcp cursor.Writer
		mcp.Message(toolCallMCP, completed.BytesOut())
		var tool cursor.Writer
		tool.String(toolCallIDField, normalizeToolCallID(call.ID))
		tool.Message(toolCallMCP, mcp.BytesOut())
		var step cursor.Writer
		step.Message(stepToolCall, tool.BytesOut())
		return step.BytesOut()
	}
	var mcp cursor.Writer
	mcp.Message(mcpArgsField, args)
	var tool cursor.Writer
	tool.String(toolCallIDField, normalizeToolCallID(call.ID))
	tool.Message(toolCallMCP, mcp.BytesOut())
	var step cursor.Writer
	step.Message(stepToolCall, tool.BytesOut())
	return step.BytesOut()
}

func buildHistoryResult(result inference.Message) []byte {
	var text cursor.Writer
	text.String(mcpContentText, toolResultText(result))
	var item cursor.Writer
	item.Message(mcpTextCase, text.BytesOut())
	var success cursor.Writer
	success.Message(1, item.BytesOut())
	var outcome cursor.Writer
	outcome.Message(1, success.BytesOut())
	return outcome.BytesOut()
}
