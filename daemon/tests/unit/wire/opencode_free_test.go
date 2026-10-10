package codec

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/wire"
	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/internal/inference"
)

// The signed-out Zen gateway is the one OpenCode serves without an account.
// It keeps no subscription, so the two paths have to stay distinguishable.
func TestOpenCodeFreeIsNotOpenCodeGo(t *testing.T) {
	cases := []struct {
		baseURL string
		free    bool
		goPath  bool
	}{
		{baseURL: "https://opencode.ai/zen/v1", free: true},
		{baseURL: "https://opencode.ai/zen", free: true},
		{baseURL: "https://OPENCODE.AI/zen/v1", free: true},
		{baseURL: "https://opencode.ai/zen/go/v1", goPath: true},
		{baseURL: "https://opencode.ai/v1"},
		{baseURL: "https://api.opencode.ai/zen/v1"},
		{baseURL: "https://openrouter.ai/api/v1"},
		{baseURL: ""},
		{baseURL: "://nonsense"},
	}
	for _, entry := range cases {
		if got := wire.OpenCodeFree(entry.baseURL); got != entry.free {
			t.Errorf("OpenCodeFree(%q) = %t, want %t", entry.baseURL, got, entry.free)
		}
		if got := wire.OpenCodeGo(entry.baseURL); got != entry.goPath {
			t.Errorf("OpenCodeGo(%q) = %t, want %t", entry.baseURL, got, entry.goPath)
		}
	}
}

// A plain chat turn carries no tools, which the free lane refuses, so the
// adaptation declares the inspected names and forbids tool calls on a copy.
func TestOpenCodeFreeRequestDeclaresToolsForAToolFreeChatTurn(t *testing.T) {
	original := &inference.Request{Model: "mimo-v2.6-flash-free"}
	got, _ := wire.OpenCodeFreeRequest(original, catalog.FormatOpenAIChat)
	if len(original.Tools) != 0 {
		t.Fatalf("original tools = %v, want the caller's request untouched", original.Tools)
	}
	names := map[string]bool{}
	for _, tool := range got.Tools {
		names[tool.Name] = true
	}
	for _, want := range []string{"bash", "glob", "grep", "read"} {
		if !names[want] {
			t.Fatalf("tools = %v, want the declaration the lane inspects", got.Tools)
		}
	}
	if got.ToolChoice == nil || got.ToolChoice.Mode != inference.ToolChoiceNone {
		t.Fatalf("tool choice = %+v, want tool calls forbidden", got.ToolChoice)
	}
	if got.Model != original.Model {
		t.Fatalf("model = %q, want the routed model kept", got.Model)
	}
}

// The Responses wire answers the same declarations without a tool choice,
// which is the shape the working client sends there.
func TestOpenCodeFreeRequestDeclaresToolsWithoutChoiceOnResponses(t *testing.T) {
	got, _ := wire.OpenCodeFreeRequest(&inference.Request{}, catalog.FormatOpenAIResp)
	if len(got.Tools) != 4 {
		t.Fatalf("tools = %v, want the four declarations the lane inspects", got.Tools)
	}
	if got.ToolChoice != nil {
		t.Fatalf("tool choice = %+v, want the Responses shape untouched", got.ToolChoice)
	}
}

// A coding client that already declares the lane's tools keeps its own
// schemas and choice, and a partial set only gains what is missing.
func TestOpenCodeFreeRequestPreservesRealTools(t *testing.T) {
	real := inference.Tool{Name: "read", Description: "repo read"}
	choice := &inference.ToolChoice{Mode: inference.ToolChoiceAuto}
	full := &inference.Request{Tools: []inference.Tool{
		{Name: "bash"}, {Name: "glob"}, {Name: "grep"}, real,
	}, ToolChoice: choice}
	if got, _ := wire.OpenCodeFreeRequest(full, catalog.FormatOpenAIChat); len(got.Tools) != 4 ||
		got.Tools[3].Description != "repo read" || got.ToolChoice != choice {
		t.Fatalf("request = %+v, want the client's own declarations kept", got)
	}

	partial, _ := wire.OpenCodeFreeRequest(&inference.Request{
		Tools: []inference.Tool{real},
	}, catalog.FormatOpenAIChat)
	if len(partial.Tools) != 4 || partial.Tools[0].Description != "repo read" {
		t.Fatalf("tools = %v, want the real tool first and only the missing appended", partial.Tools)
	}
	if partial.ToolChoice != nil {
		t.Fatalf("tool choice = %+v, want a mixed tool set left alone", partial.ToolChoice)
	}
	if got, _ := wire.OpenCodeFreeRequest(nil, catalog.FormatOpenAIChat); got != nil {
		t.Fatal("nil request should stay nil")
	}
}

// Differently-cased client tools do not satisfy the lane: Claude Code sends
// Bash and friends, so the exact lowercase declarations are still appended
// beside the originals the client can actually run.
func TestOpenCodeFreeRequestAppendsLowercaseBesideCapitalizedTools(t *testing.T) {
	original := &inference.Request{Tools: []inference.Tool{
		{Name: "Bash"}, {Name: "Glob"}, {Name: "Grep"}, {Name: "Read"},
	}}
	got, _ := wire.OpenCodeFreeRequest(original, catalog.FormatOpenAIChat)
	if len(original.Tools) != 4 {
		t.Fatalf("original tools = %v, want the caller's request untouched", original.Tools)
	}
	names := map[string]bool{}
	for _, tool := range got.Tools {
		names[tool.Name] = true
	}
	for _, want := range []string{"Bash", "Glob", "Grep", "Read", "bash", "glob", "grep", "read"} {
		if !names[want] {
			t.Fatalf("tools = %v, want originals kept and lowercase appended", got.Tools)
		}
	}
}

// The gateway answers "Invalid JSON schema ... not valid under any of the
// schemas listed in the 'anyOf' keyword" naming the fragment it refused, so the
// rejected keywords are removed from every tool schema on a copy. The keywords
// that shape a valid call survive, because a narrower model of a tool is a tool
// the model will call wrongly.
func TestOpenCodeFreeRequestDropsRejectedSchemaKeywords(t *testing.T) {
	original := &inference.Request{Tools: []inference.Tool{{
		Name: "read",
		Parameters: json.RawMessage(`{"type":"object","properties":{"file_path":` +
			`{"type":"string","minLength":1,"maxLength":1024,"pattern":"^[^\\0]*$"},` +
			`"limit":{"$schema":"https://json-schema.org/draft/2020-12/schema",` +
			`"oneOf":[{"type":"integer"},{"type":"string"}],` +
			`"additionalProperties":false,"type":"integer"}}}`),
	}}}
	got, _ := wire.OpenCodeFreeRequest(original, catalog.FormatOpenAIResp)
	if !strings.Contains(string(original.Tools[0].Parameters), "pattern") {
		t.Fatalf("original schema = %s, want the caller's request untouched", original.Tools[0].Parameters)
	}
	var cleaned map[string]any
	if err := json.Unmarshal(got.Tools[0].Parameters, &cleaned); err != nil {
		t.Fatalf("decode cleaned schema: %v", err)
	}
	properties, _ := cleaned["properties"].(map[string]any)
	filePath, _ := properties["file_path"].(map[string]any)
	limit, _ := properties["limit"].(map[string]any)
	if len(filePath) == 0 || len(limit) == 0 {
		t.Fatalf("properties = %v, want both declarations kept", properties)
	}
	for _, key := range []string{"pattern", "minLength", "maxLength"} {
		if _, found := filePath[key]; found {
			t.Errorf("file_path kept %q, want the keyword the gateway refuses removed", key)
		}
	}
	if _, found := limit["$schema"]; found {
		t.Error("limit kept \"$schema\", want the dialect declaration removed")
	}
	for _, key := range []string{"oneOf", "additionalProperties"} {
		if _, found := limit[key]; !found {
			t.Errorf("limit dropped %q, want the shape of a valid call kept", key)
		}
	}
	if kind, _ := filePath["type"].(string); kind != "string" {
		t.Errorf("file_path type = %q, want the declaration's meaning kept", kind)
	}
	if kind, _ := limit["type"].(string); kind != "integer" {
		t.Errorf("limit type = %q, want the declaration's meaning kept", kind)
	}
}

// A schema Relo cannot read is one it must not alter, and one already within
// the dialect is left exactly as the client wrote it, because re-marshalling
// every schema of a hundred-tool turn to fix a rare refusal costs more than
// the refusal does.
func TestOpenCodeFreeRequestLeavesUnusableSchemasAlone(t *testing.T) {
	clean := json.RawMessage(`{"type":"object","properties":{"a":{"type":"string"}}}`)
	broken := json.RawMessage(`{"type":`)
	for _, schema := range []json.RawMessage{clean, broken, nil} {
		original := &inference.Request{Tools: []inference.Tool{{Name: "read", Parameters: schema}}}
		got, _ := wire.OpenCodeFreeRequest(original, catalog.FormatOpenAIResp)
		for _, tool := range got.Tools {
			if tool.Name == "read" && string(tool.Parameters) != string(schema) {
				t.Errorf("schema = %s, want %s returned unchanged", tool.Parameters, schema)
			}
		}
	}
}

// A client that asks for a smaller ceiling than the gateway accepts is refused,
// so a stated ceiling is raised to the floor and an unset one is left unset.
func TestOpenCodeFreeRequestFloorsTheOutputCeiling(t *testing.T) {
	cases := []struct{ asked, want int }{
		{asked: 1, want: 16},
		{asked: 15, want: 16},
		{asked: 16, want: 16},
		{asked: 32000, want: 32000},
		{asked: 0, want: 0},
	}
	for _, entry := range cases {
		original := &inference.Request{MaxTokens: entry.asked}
		got, _ := wire.OpenCodeFreeRequest(original, catalog.FormatOpenAIResp)
		if got.MaxTokens != entry.want {
			t.Errorf("MaxTokens(%d) = %d, want %d", entry.asked, got.MaxTokens, entry.want)
		}
		if original.MaxTokens != entry.asked {
			t.Errorf("MaxTokens(%d) left the caller's request at %d",
				entry.asked, original.MaxTokens)
		}
	}
}

// mcpToolName is a tool a coding session actually sends once it loads an MCP
// server, and it is longer than the gateway accepts.
const mcpToolName = "mcp__claude_ai_Cloudflare_Developer_Platform__complete_authentication"

// A session that loads an MCP server names a tool past the gateway's limit, and
// the gateway refuses the whole turn for it. The name goes out under an alias
// every part of the request agrees on, and the answer names it again.
func TestOpenCodeFreeRequestShortensToolNamesPastTheGatewayLimit(t *testing.T) {
	if len(mcpToolName) <= 64 {
		t.Fatalf("the sample name is %d characters, want one past the gateway limit",
			len(mcpToolName))
	}
	original := &inference.Request{
		Tools:      []inference.Tool{{Name: mcpToolName}, {Name: "read"}},
		ToolChoice: &inference.ToolChoice{Mode: inference.ToolChoiceTool, Name: mcpToolName},
		Messages: []inference.Message{{
			Role:      inference.RoleAssistant,
			ToolCalls: []inference.ToolCall{{ID: "call_1", Name: mcpToolName}},
		}},
	}
	got, originals := wire.OpenCodeFreeRequest(original, catalog.FormatOpenAIResp)

	alias := got.Tools[0].Name
	if alias == mcpToolName {
		t.Fatalf("tool name = %q, want the name the gateway accepts", alias)
	}
	if len(alias) > 64 {
		t.Fatalf("tool name = %q (%d characters), want at most 64", alias, len(alias))
	}
	if originals[alias] != mcpToolName {
		t.Fatalf("originals = %v, want the alias read back as the client's name", originals)
	}
	if got.ToolChoice == nil || got.ToolChoice.Mode != inference.ToolChoiceTool ||
		got.ToolChoice.Name != alias {
		t.Fatalf("tool choice = %+v, want the demand under the same alias", got.ToolChoice)
	}
	if got.Messages[0].ToolCalls[0].Name != alias {
		t.Fatalf("replayed call = %q, want the alias its declaration went out under",
			got.Messages[0].ToolCalls[0].Name)
	}
	if got.Tools[1].Name != "read" {
		t.Fatalf("short name = %q, want it left alone", got.Tools[1].Name)
	}
	again, _ := wire.OpenCodeFreeRequest(original, catalog.FormatOpenAIResp)
	if again.Tools[0].Name != alias {
		t.Fatalf("alias = %q then %q, want the same name for the same tool",
			alias, again.Tools[0].Name)
	}
	if original.Tools[0].Name != mcpToolName ||
		original.ToolChoice.Name != mcpToolName ||
		original.Messages[0].ToolCalls[0].Name != mcpToolName {
		t.Fatal("the caller's request was rewritten, want every name it arrived with")
	}
}

// Two tools whose names share a head are distinct tools, so their aliases have
// to stay apart or the answer would call the wrong one.
func TestOpenCodeFreeRequestKeepsLongNamesWithTheSameHeadApart(t *testing.T) {
	head := strings.Repeat("m", 60)
	first, second := head+"-alpha", head+"-beta"
	got, originals := wire.OpenCodeFreeRequest(&inference.Request{
		Tools: []inference.Tool{{Name: first}, {Name: second}},
	}, catalog.FormatOpenAIResp)

	if got.Tools[0].Name == got.Tools[1].Name {
		t.Fatalf("aliases = %q, want names that share a head kept apart", got.Tools[0].Name)
	}
	if originals[got.Tools[0].Name] != first || originals[got.Tools[1].Name] != second {
		t.Fatalf("originals = %v, want each alias read back as its own tool", originals)
	}
}

// A turn whose names all fit is ordinary, so it carries no alias and no map to
// read an answer back through.
func TestOpenCodeFreeRequestLeavesNamesWithinTheLimitAlone(t *testing.T) {
	original := &inference.Request{Tools: []inference.Tool{{Name: "read"}}}
	got, originals := wire.OpenCodeFreeRequest(original, catalog.FormatOpenAIResp)
	if got.Tools[0].Name != "read" {
		t.Fatalf("tool name = %q, want it left alone", got.Tools[0].Name)
	}
	if len(originals) != 0 {
		t.Fatalf("originals = %v, want no map for a turn that renamed nothing", originals)
	}
}
