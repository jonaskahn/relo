package cursor

import (
	"encoding/json"
	"fmt"
	"testing"

	adapterscursor "github.com/jonaskahn/relo/internal/adapters/cursor"
	"github.com/jonaskahn/relo/internal/inference"
)

func historyRequest() *inference.Request {
	return &inference.Request{
		Model: "sonnet",
		Messages: []inference.Message{
			{Role: inference.RoleSystem, Content: []inference.ContentPart{{Type: inference.ContentTypeText, Text: "be brief"}}},
			{Role: inference.RoleUser, Content: []inference.ContentPart{{Type: inference.ContentTypeText, Text: "read foo"}}},
			{Role: inference.RoleAssistant, Content: []inference.ContentPart{{Type: inference.ContentTypeText, Text: "reading"}},
				ToolCalls: []inference.ToolCall{{ID: "call-1", Name: "read", Arguments: `{"path":"foo"}`}}},
			{Role: inference.RoleTool, ToolCallID: "call-1", Name: "read", Content: []inference.ContentPart{{Type: inference.ContentTypeText, Text: "contents"}}},
			{Role: inference.RoleUser, Content: []inference.ContentPart{{Type: inference.ContentTypeText, Text: "thanks"}}},
		},
		Tools: []inference.Tool{{Name: "read", Description: "read", Parameters: []byte(`{"type":"object"}`)}},
	}
}

func TestBuildHistoryReplaysTools(t *testing.T) {
	tools, err := BuildToolMap(historyRequest().Tools)
	if err != nil {
		t.Fatalf("BuildToolMap() error = %v", err)
	}
	history, action, err := BuildHistory(historyRequest(), tools)
	if err != nil {
		t.Fatalf("BuildHistory() error = %v", err)
	}
	if len(history.Rules) != 1 {
		t.Fatalf("rules = %d, want the system prompt", len(history.Rules))
	}
	fields, err := adapterscursor.Parse(action)
	if err != nil {
		t.Fatalf("parse action: %v", err)
	}
	if len(fields) != 1 || fields[0].Number != actionUserMessage {
		t.Fatalf("action = %+v, want the trailing user message", fields)
	}
	state, err := adapterscursor.Parse(history.State)
	if err != nil {
		t.Fatalf("parse state: %v", err)
	}
	roots := 0
	for _, field := range state {
		if field.Number == stateRootPrompts && field.Wire == 2 {
			roots++
		}
	}
	if roots == 0 {
		t.Fatal("no root prompts in the state")
	}
	turns := 0
	for _, field := range state {
		if field.Number == stateTurns && field.Wire == 2 {
			turns++
		}
	}
	if turns != 1 {
		t.Fatalf("turns = %d, want the first user turn", turns)
	}
	if !historyHasToolResult(t, history, state) {
		t.Fatal("no tool-result entry in the root prompt")
	}
}

func historyHasToolResult(t *testing.T, history History, state []adapterscursor.Field) bool {
	t.Helper()
	for _, field := range state {
		if field.Number != stateRootPrompts || field.Wire != 2 {
			continue
		}
		blob, found := history.Store.Get(field.Raw)
		if !found {
			t.Fatalf("blob %x missing", field.Raw)
		}
		var entry map[string]any
		if err := json.Unmarshal(blob, &entry); err != nil {
			continue
		}
		if entry["role"] == "tool" {
			return true
		}
	}
	return false
}

func TestBuildHistoryServesBlobs(t *testing.T) {
	tools, err := BuildToolMap(historyRequest().Tools)
	if err != nil {
		t.Fatalf("BuildToolMap() error = %v", err)
	}
	history, _, err := BuildHistory(historyRequest(), tools)
	if err != nil {
		t.Fatalf("BuildHistory() error = %v", err)
	}
	state, err := adapterscursor.Parse(history.State)
	if err != nil {
		t.Fatalf("parse state: %v", err)
	}
	referenced := 0
	for _, field := range state {
		if (field.Number == stateRootPrompts || field.Number == stateTurns) && field.Wire == 2 {
			referenced++
		}
	}
	if referenced == 0 {
		t.Fatal("no history blobs referenced")
	}
	ids := collectBlobIDs(state)
	if len(ids) == 0 {
		t.Fatal("no blob ids in the state")
	}
	for _, id := range ids {
		blob, found := history.Store.Get(id)
		if !found || len(blob) == 0 {
			t.Fatalf("blob %x missing", id)
		}
	}
}

func collectBlobIDs(state []adapterscursor.Field) [][]byte {
	var ids [][]byte
	for _, number := range []int{stateRootPrompts, stateTurns} {
		for _, field := range state {
			if field.Number == number && field.Wire == 2 {
				ids = append(ids, field.Raw)
			}
		}
	}
	return ids
}

func TestBuildHistoryResume(t *testing.T) {
	request := historyRequest()
	request.Messages = request.Messages[:4]
	tools, err := BuildToolMap(request.Tools)
	if err != nil {
		t.Fatalf("BuildToolMap() error = %v", err)
	}
	_, action, err := BuildHistory(request, tools)
	if err != nil {
		t.Fatalf("BuildHistory() error = %v", err)
	}
	fields, err := adapterscursor.Parse(action)
	if err != nil {
		t.Fatalf("parse action: %v", err)
	}
	if len(fields) != 1 || fields[0].Number != actionResume {
		t.Fatalf("action = %+v, want a resume over the tool result", fields)
	}
}

func TestToolResultJSON(t *testing.T) {
	entry := toolResultEntry(inference.Message{
		ToolCallID: "call-1",
		Content:    []inference.ContentPart{{Type: inference.ContentTypeText, Text: "ok"}},
	}, "read")
	raw, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded["role"] != "tool" {
		t.Fatalf("entry = %v", decoded)
	}
}

// TestBuildHistoryHeadAlwaysPopulated covers the refused turn: a request
// with no history and no system prompt must still carry a system blob at
// the head of the state, which is what the server reads as conversation
// state. An empty head is the "Conversation state is required" refusal.
func TestBuildHistoryHeadAlwaysPopulated(t *testing.T) {
	tools, err := BuildToolMap(nil)
	if err != nil {
		t.Fatalf("BuildToolMap() error = %v", err)
	}
	plain := &inference.Request{Model: "composer-2.5", Messages: []inference.Message{
		{Role: inference.RoleUser, Content: []inference.ContentPart{{Type: inference.ContentTypeText, Text: "hello"}}},
	}}
	stated := &inference.Request{Model: "composer-2.5", Messages: []inference.Message{
		{Role: inference.RoleSystem, Content: []inference.ContentPart{{Type: inference.ContentTypeText, Text: "be brief"}}},
		{Role: inference.RoleUser, Content: []inference.ContentPart{{Type: inference.ContentTypeText, Text: "hello"}}},
	}}
	for name, request := range map[string]*inference.Request{"no system prompt": plain, "with system prompt": stated} {
		history, _, err := BuildHistory(request, tools)
		if err != nil {
			t.Fatalf("%s: BuildHistory() error = %v", name, err)
		}
		head, err := headPromptText(history)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if head == "" {
			t.Fatalf("%s: state head is empty", name)
		}
	}
}

// headPromptText reads the first root-prompt blob's system text.
func headPromptText(history History) (string, error) {
	state, err := adapterscursor.Parse(history.State)
	if err != nil {
		return "", err
	}
	for _, field := range state {
		if field.Number != stateRootPrompts || field.Wire != 2 {
			continue
		}
		blob, found := history.Store.Get(field.Raw)
		if !found {
			return "", fmt.Errorf("blob %x missing", field.Raw)
		}
		var entry struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		}
		if err := json.Unmarshal(blob, &entry); err != nil {
			return "", err
		}
		if entry.Role == "system" {
			return entry.Content, nil
		}
	}
	return "", nil
}
