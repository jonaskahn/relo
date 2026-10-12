package cursor

import (
	"testing"

	adapterscursor "github.com/jonaskahn/relo/internal/adapters/cursor"
	"github.com/jonaskahn/relo/internal/adapters/wire"
	"github.com/jonaskahn/relo/internal/inference"
)

// Strict wire expectations for everything Relo emits in a turn's first
// second: the initial request, the context and tool-state answers, the blob
// answers, and the handoff acknowledgement. Each entry names a field's wire
// type and the message nested inside it, or nothing when the content is
// opaque bytes the server reads on its own terms. The numbers are the pinned
// Cursor schemas; our own parser accepts anything, so this table is what
// stops a misnested message from reaching the upstream.
const (
	strictVarint = 0
	strictBytes  = 2
)

type strictField struct {
	wire  int
	child string
}

var strictSchemas = map[string]map[int]strictField{
	"client": {
		1: {strictBytes, "run"}, 2: {strictBytes, "exec"}, 3: {strictBytes, "kv"},
	},
	"run": {
		1: {strictBytes, "state"}, 2: {strictBytes, "action"}, 3: {strictBytes, "details"},
		5: {strictBytes, ""}, 9: {strictBytes, "requested"},
	},
	"state": {1: {strictBytes, ""}, 8: {strictBytes, ""}},
	"action": {
		1: {strictBytes, "userAction"}, 2: {strictBytes, "resume"},
	},
	"userAction": {1: {strictBytes, "userMsg"}},
	"userMsg":    {1: {strictBytes, ""}, 2: {strictBytes, ""}},
	"resume":     {},
	"details": {
		1: {strictBytes, ""}, 3: {strictBytes, ""}, 4: {strictBytes, ""},
	},
	"requested": {
		1: {strictBytes, ""}, 2: {strictVarint, ""}, 3: {strictBytes, "pair"},
	},
	"pair": {1: {strictBytes, ""}, 2: {strictBytes, ""}},
	"exec": {
		1: {strictVarint, ""}, 15: {strictBytes, ""},
		10: {strictBytes, "reqCtxResult"}, 11: {strictBytes, "mcpResult"}, 36: {strictBytes, "mcpStateResult"},
		2: {strictBytes, ""}, 3: {strictBytes, ""}, 4: {strictBytes, ""}, 5: {strictBytes, ""},
		7: {strictBytes, ""}, 8: {strictBytes, ""}, 9: {strictBytes, ""},
	},
	"reqCtxResult":  {1: {strictBytes, "reqCtxSuccess"}},
	"reqCtxSuccess": {1: {strictBytes, "reqCtx"}},
	"reqCtx":        {2: {strictBytes, "rule"}, 7: {strictBytes, "tooldef"}},
	"rule": {
		1: {strictBytes, ""}, 2: {strictBytes, ""}, 3: {strictBytes, "ruletype"}, 4: {strictVarint, ""},
	},
	"ruletype": {1: {strictBytes, ""}},
	"tooldef": {
		1: {strictBytes, ""}, 2: {strictBytes, ""}, 3: {strictBytes, ""},
		4: {strictBytes, ""}, 5: {strictBytes, ""},
	},
	"mcpStateResult":  {1: {strictBytes, "mcpStateSuccess"}},
	"mcpStateSuccess": {1: {strictBytes, "mcpServer"}},
	"mcpServer":       {1: {strictBytes, ""}, 2: {strictBytes, ""}, 5: {strictBytes, "tooldef"}},
	"mcpResult":       {1: {strictBytes, "mcpSuccess"}},
	"mcpSuccess":      {1: {strictBytes, "mcpItem"}},
	"mcpItem":         {1: {strictBytes, "mcpText"}},
	"mcpText":         {1: {strictBytes, ""}},
	"kv": {
		1: {strictVarint, ""}, 2: {strictBytes, "kvget"}, 3: {strictBytes, "kvset"},
	},
	"kvget": {1: {strictBytes, ""}},
	"kvset": {},
}

// assertStrict walks one emitted message against the pinned table. An
// unknown field or a wrong wire type fails: that is the shape a strict
// upstream parser refuses.
func assertStrict(t *testing.T, name string, raw []byte) {
	t.Helper()
	fields, err := adapterscursor.Parse(raw)
	if err != nil {
		t.Fatalf("%s: parse: %v", name, err)
	}
	schema, known := strictSchemas[name]
	if !known {
		t.Fatalf("%s: no pinned schema", name)
	}
	for _, field := range fields {
		want, found := schema[field.Number]
		if !found {
			t.Fatalf("%s: field %d is not in the pinned schema", name, field.Number)
		}
		if field.Wire != want.wire {
			t.Fatalf("%s: field %d wire %d, want %d", name, field.Number, field.Wire, want.wire)
		}
		if want.child != "" {
			assertStrict(t, want.child, field.Raw)
		}
	}
}

func strictTools(t *testing.T) *ToolMap {
	t.Helper()
	tools, err := BuildToolMap([]inference.Tool{
		{Name: "exec", Description: "run code", Parameters: []byte(`{"type":"object"}`)},
		{Name: "clock", Description: "time", Parameters: []byte(`{"type":"object","properties":{"when":{"type":"string"}}}`)},
	})
	if err != nil {
		t.Fatalf("BuildToolMap() error = %v", err)
	}
	return tools
}

func strictSelection() *wire.CursorRequest {
	return &wire.CursorRequest{
		ConversationID: "11111111-2222-4333-8444-555555555555",
		ModelID:        "claude-haiku-5-5",
		DetailsID:      "claude-haiku-5-5-thinking-high",
		Params: []wire.CursorParam{
			{ID: "thinking", Value: "true"},
			{ID: "context", Value: "300k"},
			{ID: "reasoning_effort", Value: "high"},
		},
	}
}

// TestStrictRunRequest replays the refused turn's shape: system prompts,
// one history user message, a trailing user action, declared tools, and the
// stored tuple the account serves. Every level must match the pinned
// schema, especially the action's three-deep user message.
func TestStrictRunRequest(t *testing.T) {
	text := func(value string) []inference.ContentPart {
		return []inference.ContentPart{{Type: inference.ContentTypeText, Text: value}}
	}
	tools := strictTools(t)
	req := &inference.Request{Messages: []inference.Message{
		{Role: inference.RoleSystem, Content: text("You are a coding agent.")},
		{Role: inference.RoleUser, Content: text("environment context")},
		{Role: inference.RoleSystem, Content: text("permissions")},
		{Role: inference.RoleUser, Content: text("Generate a concise title.")},
	}}
	history, action, err := BuildHistory(req, tools)
	if err != nil {
		t.Fatalf("BuildHistory() error = %v", err)
	}
	// buildRunRequest already returns the whole client message; wrapping it
	// again would test a shape Relo never sends.
	assertStrict(t, "client", buildRunRequest(history.State, action, strictSelection()))
}

// TestStrictTurnReplies covers the answers a refused turn's first second
// may need: the request context, the tool state, and both blob answers.
func TestStrictTurnReplies(t *testing.T) {
	tools := strictTools(t)
	assertStrict(t, "client", buildClientExec(7, "exec-7", resultContext, buildRequestContext(nil, tools.Defs())))
	assertStrict(t, "client", buildClientExec(9, "exec-9", resultMCPState, buildMCPState(tools.Defs())))
	assertStrict(t, "client", buildClientExec(3, "exec-3", resultMCP, buildMCPHandoff()))
	assertStrict(t, "client", buildKVGet(4, []byte("blob-bytes")))
	assertStrict(t, "client", buildKVGet(5, nil))
	assertStrict(t, "client", buildKVSet(6))
}

// TestRunRequestAlwaysCarriesConversationState pins the field the server
// refused a turn for: a request with no history at all must still send the
// conversation state field, empty when there is nothing to put in it.
func TestRunRequestAlwaysCarriesConversationState(t *testing.T) {
	tools, err := BuildToolMap(nil)
	if err != nil {
		t.Fatalf("BuildToolMap() error = %v", err)
	}
	for name, req := range map[string]*inference.Request{
		"empty": {Model: "composer-2.5"},
		"one user message": {Model: "composer-2.5", Messages: []inference.Message{
			{Role: inference.RoleUser, Content: []inference.ContentPart{{Type: inference.ContentTypeText, Text: "hi"}}}}},
	} {
		history, action, err := BuildHistory(req, tools)
		if err != nil {
			t.Fatalf("%s: BuildHistory() error = %v", name, err)
		}
		client, err := adapterscursor.Parse(buildRunRequest(history.State, action, strictSelection()))
		if err != nil {
			t.Fatalf("%s: parse: %v", name, err)
		}
		run, err := adapterscursor.Nested(client, clientRunRequest)
		if err != nil || len(run) == 0 {
			t.Fatalf("%s: run message = %v %v", name, run, err)
		}
		found := false
		for _, field := range run[0] {
			if field.Number == runConversationState && field.Wire == 2 {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s: run request carries no conversation state", name)
		}
	}
}
