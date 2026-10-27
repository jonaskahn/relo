package cursor

import (
	"strings"
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

// openCodeTools are the tools OpenCode declares in the failing Composer turn.
// The request context and the MCP state both have to carry every one of them,
// with the native collisions renamed and their schemas intact.
var openCodeTools = []struct {
	name   string
	schema string
}{
	{"codegraph_codegraph_explore", `{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}`},
	{"edit", `{"type":"object","properties":{"path":{"type":"string"},"oldString":{"type":"string"},"newString":{"type":"string"}},"required":["path","oldString","newString"]}`},
	{"glob", `{"type":"object","properties":{"pattern":{"type":"string"}},"required":["pattern"]}`},
	{"grep", `{"type":"object","properties":{"pattern":{"type":"string"},"path":{"type":"string"}},"required":["pattern"]}`},
	{"question", `{"type":"object","properties":{"questions":{"type":"array","items":{"type":"object"}}}}`},
	{"read", `{"type":"object","properties":{"path":{"type":"string"},"offset":{"type":"integer"},"limit":{"type":"integer"}},"required":["path"]}`},
	{"shell", `{"type":"object","properties":{"command":{"type":"string"},"workdir":{"type":"string"},"timeout":{"type":"integer"}},"required":["command"]}`},
	{"skill", `{"type":"object","properties":{"id":{"type":"string"}},"required":["id"]}`},
	{"subagent", `{"type":"object","properties":{"agent":{"type":"string"},"description":{"type":"string"}}}`},
	{"webfetch", `{"type":"object","properties":{"url":{"type":"string"},"format":{"type":"string"}},"required":["url"]}`},
	{"websearch", `{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}`},
	{"write", `{"type":"object","properties":{"path":{"type":"string"},"content":{"type":"string"}},"required":["path","content"]}`},
	{"execute", `{"type":"object","properties":{"code":{"type":"string"}},"required":["code"]}`},
}

// TestRequestContextCarriesEveryDeclaredTool decodes the context answer the
// server actually reads, not the builder's input: every declared tool, its
// wire name, its canonical label, and its schema must survive the encoding.
func TestRequestContextCarriesEveryDeclaredTool(t *testing.T) {
	declared := make([]inference.Tool, 0, len(openCodeTools))
	for _, tool := range openCodeTools {
		declared = append(declared, inference.Tool{
			Name: tool.name, Description: tool.name + " tool", Parameters: []byte(tool.schema),
		})
	}
	tools, err := BuildToolMap(declared)
	if err != nil {
		t.Fatalf("BuildToolMap() error = %v", err)
	}
	for name, decoded := range map[string][][]adapterscursor.Field{
		"request context": decodedContext(t, buildRequestContext(nil, tools.Defs())),
		"mcp state":       decodedMCPState(t, buildMCPState(tools.Defs())),
	} {
		if len(decoded) != len(openCodeTools) {
			t.Fatalf("%s carries %d tools, want %d", name, len(decoded), len(openCodeTools))
		}
	}
	defs := decodedContext(t, buildRequestContext(nil, tools.Defs()))
	for index, def := range defs {
		want := openCodeTools[index]
		wireName := adapterscursor.FirstString(def, defName)
		if wireName != WireName(want.name) {
			t.Fatalf("tool %d wire name = %q, want %q", index, wireName, WireName(want.name))
		}
		description := adapterscursor.FirstString(def, defDescription)
		if !strings.Contains(description, "External client tool `"+want.name+"`") {
			t.Fatalf("tool %q description = %q, want the canonical name", wireName, description)
		}
		schema := adapterscursor.FirstString(def, defSchema)
		object, ok := decodeJSONValue([]byte(schema))
		if !ok {
			t.Fatalf("tool %q schema is not a proto Value", wireName)
		}
		fields, ok := object.(map[string]any)
		if !ok || fields["type"] != "object" {
			t.Fatalf("tool %q schema = %v, want an object schema", wireName, object)
		}
	}
}

func decodedContext(t *testing.T, payload []byte) [][]adapterscursor.Field {
	t.Helper()
	result := mustParseForTest(t, payload)
	success, err := adapterscursor.Nested(result, 1)
	if err != nil || len(success) != 1 {
		t.Fatalf("context result = %v %v", success, err)
	}
	contexts, err := adapterscursor.Nested(success[0], 1)
	if err != nil || len(contexts) != 1 {
		t.Fatalf("context success = %v %v", contexts, err)
	}
	tools, err := adapterscursor.Nested(contexts[0], contextTools)
	if err != nil {
		t.Fatalf("context tools: %v", err)
	}
	return tools
}

func decodedMCPState(t *testing.T, payload []byte) [][]adapterscursor.Field {
	t.Helper()
	result := mustParseForTest(t, payload)
	success, err := adapterscursor.Nested(result, mcpStateSuccess)
	if err != nil || len(success) != 1 {
		t.Fatalf("mcp state result = %v %v", success, err)
	}
	servers, err := adapterscursor.Nested(success[0], mcpStateServer)
	if err != nil || len(servers) != 1 {
		t.Fatalf("mcp state success = %v %v", servers, err)
	}
	tools, err := adapterscursor.Nested(servers[0], stateTools)
	if err != nil {
		t.Fatalf("mcp state tools: %v", err)
	}
	return tools
}

// TestRequestContextSteersAwayFromNativeTools pins the rule that makes the
// model prefer the declared external tools over Cursor's native channel.
func TestRequestContextSteersAwayFromNativeTools(t *testing.T) {
	rules := buildSystemRules(&inference.Request{Messages: []inference.Message{{
		Role: inference.RoleSystem, Content: []inference.ContentPart{{Type: inference.ContentTypeText, Text: "be brief"}},
	}}})
	found := ""
	for _, rule := range rules {
		fields := mustParseForTest(t, rule)
		if content := adapterscursor.FirstString(fields, ruleContent); strings.Contains(content, "relo_") {
			found = content
		}
	}
	if !strings.Contains(found, "Native tool execution is unavailable") {
		t.Fatalf("rules = %d, want the native-tool steering rule", len(rules))
	}
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
	"userMsg":    {1: {strictBytes, ""}, 2: {strictBytes, ""}, 3: {strictBytes, "userCtx"}},
	"userCtx":    {1: {strictBytes, "selectedImage"}},
	"selectedImage": {
		2: {strictBytes, ""}, 7: {strictBytes, ""}, 8: {strictBytes, ""},
	},
	"resume": {},
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
	"mcpItem":         {1: {strictBytes, "mcpText"}, 2: {strictBytes, "mcpImage"}},
	"mcpText":         {1: {strictBytes, ""}},
	"mcpImage":        {1: {strictBytes, ""}, 2: {strictBytes, ""}},
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

// TestBuildToolMapServesARealToolSet pins the ceiling a client's own tool set
// has to fit under. Claude Code declares 90-145 tools once its MCP servers are
// attached, and every one of them was refused as unrepresentable.
func TestBuildToolMapServesARealToolSet(t *testing.T) {
	declared := make([]inference.Tool, 0, 200)
	for index := 0; index < 200; index++ {
		declared = append(declared, inference.Tool{Name: "tool_" + itoa(index), Parameters: []byte(`{"type":"object"}`)})
	}
	mapped, err := BuildToolMap(declared)
	if err != nil {
		t.Fatalf("BuildToolMap() error = %v", err)
	}
	if len(mapped.Names()) != len(declared) {
		t.Fatalf("Declared() = %d tools, want %d", len(mapped.Names()), len(declared))
	}
}
