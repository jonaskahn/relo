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
	mcpImageCase    = 2
	mcpContentText  = 1
	// The image case's payload: McpImageContent carries the raw bytes and
	// the MIME type, each numbered from one inside that message.
	mcpImageData = 1
	mcpImageMime = 2
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
	system := systemPromptBlobs(req, store)
	root, err := buildRootPrompt(req, tools, store, cutoff)
	if err != nil {
		return History{}, nil, err
	}
	turns, err := buildTurns(req, tools, store, cutoff)
	if err != nil {
		return History{}, nil, err
	}
	var state cursor.Writer
	for _, id := range system {
		state.Bytes(stateRootPrompts, id)
	}
	for _, id := range root {
		state.Bytes(stateRootPrompts, id)
	}
	for _, id := range turns {
		state.Bytes(stateTurns, id)
	}
	action, err := buildAction(active)
	if err != nil {
		return History{}, nil, err
	}
	return History{State: state.BytesOut(), Rules: rules, Store: store}, action, nil
}

// buildAction encodes the trailing action: the active user message when the
// request ends with one, a resume otherwise.
func buildAction(active *inference.Message) ([]byte, error) {
	if active == nil {
		return buildResumeAction(), nil
	}
	return buildUserAction(*active)
}

// systemPromptBlobs stores one JSON blob per system prompt, so the state
// head always carries the system prompt the way the reference client does.
// Without a blob the head is empty and the server refuses the turn for
// missing conversation state.
func systemPromptBlobs(req *inference.Request, store *Store) [][]byte {
	var blobs [][]byte
	for _, message := range req.Messages {
		if message.Role != inference.RoleSystem {
			continue
		}
		if text := messageText(message); text != "" {
			blobs = append(blobs, store.Put(systemPromptEntry(text)))
		}
	}
	if len(blobs) == 0 {
		blobs = append(blobs, store.Put(systemPromptEntry("You are a helpful assistant.")))
	}
	return blobs
}

func systemPromptEntry(content string) []byte {
	raw, _ := json.Marshal(struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}{Role: "system", Content: content})
	return raw
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
	rules = append(rules, buildRule(systemRulePath(index), externalToolRule))
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

// rootPromptBuilder replays canonical messages into root-prompt JSON
// blobs, pairing tool results with the assistant calls that issued them.
type rootPromptBuilder struct {
	messages []inference.Message
	cutoff   int
	store    *Store
	calls    map[string]string
	emitted  map[string]bool
	ids      [][]byte
}

func buildRootPrompt(req *inference.Request, tools *ToolMap, store *Store, cutoff int) ([][]byte, error) {
	_ = tools
	builder := &rootPromptBuilder{messages: req.Messages, cutoff: cutoff, store: store,
		calls: map[string]string{}, emitted: map[string]bool{}}
	for index := 0; index < cutoff && index < len(builder.messages); index++ {
		builder.replay(builder.messages[index], index)
	}
	return builder.ids, nil
}

func (b *rootPromptBuilder) push(value any) []byte {
	raw, _ := json.Marshal(value)
	return b.store.Put(raw)
}

func (b *rootPromptBuilder) replay(message inference.Message, index int) {
	switch message.Role {
	case inference.RoleUser:
		b.replayUser(message)
	case inference.RoleAssistant:
		b.replayAssistant(message, index)
	case inference.RoleTool:
		b.replayResult(message)
	}
}

func (b *rootPromptBuilder) replayUser(message inference.Message) {
	content := []any{}
	if text := messageText(message); text != "" {
		content = append(content, map[string]any{"type": "text", "text": text})
	}
	for _, part := range message.Content {
		if part.Type != inference.ContentTypeImage {
			continue
		}
		mimeType, _, ok := splitDataURL(part.ImageURL)
		if !ok {
			continue
		}
		content = append(content, map[string]any{
			"type": "image", "image": part.ImageURL, "mediaType": mimeType,
		})
	}
	if len(content) == 0 {
		return
	}
	b.ids = append(b.ids, b.push(map[string]any{"role": "user", "content": content}))
}

func (b *rootPromptBuilder) replayAssistant(message inference.Message, index int) {
	rounds := assistantRounds(message)
	for round, step := range rounds {
		b.ids = append(b.ids, b.push(map[string]any{"role": "assistant", "content": step}))
		if round == len(rounds)-1 {
			continue
		}
		b.replayRound(index, step)
	}
	for _, call := range assistantCalls(message) {
		b.calls[normalizeToolCallID(call.ID)] = call.Name
	}
}

func (b *rootPromptBuilder) replayRound(assistant int, step []any) {
	for _, result := range b.roundResults(assistant, step) {
		if b.emitted[result.id] {
			continue
		}
		b.emitted[result.id] = true
		b.ids = append(b.ids, b.push(result.entry))
	}
}

func (b *rootPromptBuilder) replayResult(message inference.Message) {
	if b.emitted[message.ToolCallID] {
		return
	}
	name, paired := b.calls[normalizeToolCallID(message.ToolCallID)]
	if !paired {
		if text := toolResultText(message); text != "" {
			b.ids = append(b.ids, b.push(map[string]any{"role": "assistant", "content": []any{map[string]any{"type": "text", "text": text}}}))
		}
		return
	}
	b.emitted[message.ToolCallID] = true
	b.ids = append(b.ids, b.push(toolResultEntry(message, name)))
}

type roundResult struct {
	id    string
	entry map[string]any
}

func (b *rootPromptBuilder) roundResults(assistant int, step []any) []roundResult {
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
	for index := assistant + 1; index < b.cutoff && index < len(b.messages); index++ {
		message := b.messages[index]
		if message.Role != inference.RoleTool || !callIDs[normalizeToolCallID(message.ToolCallID)] {
			continue
		}
		name, paired := b.calls[normalizeToolCallID(message.ToolCallID)]
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
	builder := &turnBuilder{messages: req.Messages, tools: tools, store: store, cutoff: cutoff}
	for builder.index < cutoff && builder.index < len(builder.messages) {
		if err := builder.replayTurn(); err != nil {
			return nil, err
		}
	}
	return builder.ids, nil
}

// turnBuilder groups canonical messages into conversation turns: one user
// message followed by its assistant steps and paired tool results.
type turnBuilder struct {
	messages []inference.Message
	tools    *ToolMap
	store    *Store
	cutoff   int
	index    int
	ids      [][]byte
}

func (b *turnBuilder) replayTurn() error {
	message := b.messages[b.index]
	if message.Role != inference.RoleUser || b.skippable(message) {
		b.index++
		return nil
	}
	userBlob, err := b.userBlob(message)
	if err != nil {
		return err
	}
	b.index++
	steps, calls, orphans := b.collectSteps()
	steps = append(steps, turnCalls(b.messages, b.tools, calls)...)
	for _, orphan := range orphans {
		if text := toolResultText(orphan); text != "" {
			steps = append(steps, assistantStep(b.store, text))
		}
	}
	var turn cursor.Writer
	turn.Bytes(turnUserMessage, userBlob)
	for _, step := range steps {
		turn.Bytes(turnSteps, step)
	}
	var wrapped cursor.Writer
	wrapped.Message(1, turn.BytesOut())
	b.ids = append(b.ids, b.store.Put(wrapped.BytesOut()))
	return nil
}

func (b *turnBuilder) skippable(message inference.Message) bool {
	return messageText(message) == "" && !messageHasImages(message)
}

// userBlob stores the turn's user message: its text and any inline images.
func (b *turnBuilder) userBlob(message inference.Message) ([]byte, error) {
	user, err := buildUserMessage(message)
	if err != nil {
		return nil, err
	}
	return b.store.Put(user), nil
}

func (b *turnBuilder) collectSteps() ([][]byte, map[string]inference.ToolCall, []inference.Message) {
	var steps [][]byte
	calls := map[string]inference.ToolCall{}
	var orphans []inference.Message
	for b.index < b.cutoff && b.index < len(b.messages) && b.messages[b.index].Role != inference.RoleUser {
		b.collectStep(b.messages[b.index], &steps, calls, &orphans)
		b.index++
	}
	return steps, calls, orphans
}

func (b *turnBuilder) collectStep(step inference.Message, steps *[][]byte, calls map[string]inference.ToolCall, orphans *[]inference.Message) {
	switch step.Role {
	case inference.RoleAssistant:
		for _, part := range step.Content {
			if part.Type == inference.ContentTypeText && strings.TrimSpace(part.Text) != "" {
				*steps = append(*steps, assistantStep(b.store, strings.TrimSpace(part.Text)))
			}
		}
		for _, call := range step.ToolCalls {
			calls[normalizeToolCallID(call.ID)] = call
		}
	case inference.RoleTool:
		if _, paired := calls[normalizeToolCallID(step.ToolCallID)]; !paired {
			*orphans = append(*orphans, step)
		}
	}
}

func assistantStep(store *Store, text string) []byte {
	var assistant cursor.Writer
	assistant.String(assistantText, text)
	var step cursor.Writer
	step.Message(stepAssistant, assistant.BytesOut())
	return store.Put(step.BytesOut())
}

func turnCalls(messages []inference.Message, tools *ToolMap, calls map[string]inference.ToolCall) [][]byte {
	var steps [][]byte
	names := make([]string, 0, len(calls))
	for id := range calls {
		names = append(names, id)
	}
	sortStrings(names)
	for _, id := range names {
		call := calls[id]
		result, found := pairResult(messages, call.ID)
		steps = append(steps, toolCallStep(call, result, found, tools))
	}
	return steps
}

func pairResult(messages []inference.Message, callID string) (inference.Message, bool) {
	for _, message := range messages {
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
		var tool cursor.Writer
		tool.String(toolCallIDField, normalizeToolCallID(call.ID))
		tool.Message(toolCallMCP, completed.BytesOut())
		var step cursor.Writer
		step.Message(stepToolCall, tool.BytesOut())
		return step.BytesOut()
	}
	var pending cursor.Writer
	pending.Message(mcpArgsField, args)
	var tool cursor.Writer
	tool.String(toolCallIDField, normalizeToolCallID(call.ID))
	tool.Message(toolCallMCP, pending.BytesOut())
	var step cursor.Writer
	step.Message(stepToolCall, tool.BytesOut())
	return step.BytesOut()
}

func buildHistoryResult(result inference.Message) []byte {
	var success cursor.Writer
	if text := toolResultText(result); text != "" {
		var content cursor.Writer
		content.String(mcpContentText, text)
		var item cursor.Writer
		item.Message(mcpTextCase, content.BytesOut())
		success.Message(1, item.BytesOut())
	}
	for _, part := range result.Content {
		if part.Type != inference.ContentTypeImage {
			continue
		}
		mimeType, data, ok := splitDataURL(part.ImageURL)
		if !ok {
			continue
		}
		var content cursor.Writer
		content.Bytes(mcpImageData, data)
		content.String(mcpImageMime, mimeType)
		var item cursor.Writer
		item.Message(mcpImageCase, content.BytesOut())
		success.Message(1, item.BytesOut())
	}
	var outcome cursor.Writer
	outcome.Message(1, success.BytesOut())
	return outcome.BytesOut()
}
