package codec

import (
	"encoding/json"
	"fmt"
	"slices"
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
		// The gateway's current Go path is the same lane as the older one.
		{baseURL: "https://opencode.ai/inference/go/v1", goPath: true},
		// The paid Console path is neither the Go lane nor the signed-out one.
		{baseURL: "https://opencode.ai/inference/v1"},
		{baseURL: "https://opencode.ai/v1"},
		{baseURL: "https://api.opencode.ai/zen/v1"},
		{baseURL: "https://openrouter.ai/api/v1"},
		{baseURL: "https://opencode.ai.evil.test/zen/v1"},
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

// A tool-free turn gains the lane declarations: the gateway answers a bare
// turn with 403 FreeTierError (observed live on title generation, 24 of 24
// refused). The appended placeholders carry no tool choice of their own.
func TestOpenCodeFreeRequestDeclaresToolsForAToolFreeChatTurn(t *testing.T) {
	original := &inference.Request{Model: "mimo-v2.6-flash-free"}
	got, _ := wire.OpenCodeFreeRequest(original, catalog.FormatOpenAIChat)
	if len(original.Tools) != 0 {
		t.Fatalf("original tools = %v, want the caller's request untouched", original.Tools)
	}
	assertFreeLaneTools(t, got.Tools)
	if got.ToolChoice != nil {
		t.Fatalf("tool choice = %+v, want no choice added to a tool-free turn", got.ToolChoice)
	}
	if got.Model != original.Model {
		t.Fatalf("model = %q, want the routed model kept", got.Model)
	}
}

// The Responses wire answers a tool-free turn the same way: the lane
// declarations travel, and no tool choice is invented for it.
func TestOpenCodeFreeRequestDeclaresToolsWithoutChoiceOnResponses(t *testing.T) {
	got, _ := wire.OpenCodeFreeRequest(&inference.Request{}, catalog.FormatOpenAIResp)
	assertFreeLaneTools(t, got.Tools)
	if got.ToolChoice != nil {
		t.Fatalf("tool choice = %+v, want the Responses shape untouched", got.ToolChoice)
	}
}

// The turn shape that produced every observed 403: a title generation, two
// messages, no tools, a full output ceiling and a stated effort. It must
// reach the gateway carrying the lane declarations.
func TestOpenCodeFreeRequestDeclaresToolsForATitleGenerationTurn(t *testing.T) {
	got, _ := wire.OpenCodeFreeRequest(&inference.Request{
		Messages:  []inference.Message{{Role: "system"}, {Role: "user"}},
		MaxTokens: 32000,
		Reasoning: &inference.ReasoningConfig{Effort: "high"},
	}, catalog.FormatOpenAIChat)
	assertFreeLaneTools(t, got.Tools)
}

// assertFreeLaneTools checks the declarations the gateway inspects, in the
// order it reads them.
func assertFreeLaneTools(t *testing.T, got []inference.Tool) {
	t.Helper()
	want := []string{"bash", "glob", "grep", "read"}
	if len(got) != len(want) {
		t.Fatalf("tools = %d declarations, want the %d lane tools", len(got), len(want))
	}
	for index, name := range want {
		if got[index].Name != name {
			t.Fatalf("tool %d = %q, want %q", index, got[index].Name, name)
		}
	}
}

func TestOpenCodeFreeRequestCutsToolsAtTheGatewayLimit(t *testing.T) {
	original := &inference.Request{Tools: freeToolCrowd(36, 1, 114)}
	got, _ := wire.OpenCodeFreeRequestWithToolCutoff(original, catalog.FormatOpenAIChat, "conversation-one")
	if len(got.Tools) != 128 {
		t.Fatalf("tools = %d, want the gateway limit", len(got.Tools))
	}
	if len(original.Tools) != 151 {
		t.Fatalf("original tools = %d, want the caller's request untouched", len(original.Tools))
	}
	names := freeToolNames(got.Tools)
	for _, name := range append(append([]string{}, freeLaneNames...), "mcp__codegraph__tool0") {
		if !names[name] {
			t.Fatalf("tools = %v, want %q kept", names, name)
		}
	}
	for index := range 36 {
		name := fmt.Sprintf("Native%02d", index)
		if !names[name] {
			t.Fatalf("tools = %v, want native tool %q kept", names, name)
		}
	}
}

func TestOpenCodeFreeRequestKeepsTheChosenTool(t *testing.T) {
	tools := freeToolCrowd(0, 0, 140)
	tools = append(tools, inference.Tool{Name: "mcp__other__chosen"})
	original := &inference.Request{
		Tools:      tools,
		ToolChoice: &inference.ToolChoice{Mode: inference.ToolChoiceTool, Name: "mcp__other__chosen"},
	}
	got, _ := wire.OpenCodeFreeRequestWithToolCutoff(original, catalog.FormatOpenAIChat, "conversation-two")
	if len(got.Tools) != 128 {
		t.Fatalf("tools = %d, want the gateway limit", len(got.Tools))
	}
	if !freeToolNames(got.Tools)["mcp__other__chosen"] {
		t.Fatal("the chosen tool was cut, want every tool choice kept")
	}
}

func TestOpenCodeFreeRequestCutoffIsStableForAConversation(t *testing.T) {
	original := &inference.Request{Tools: freeToolCrowd(10, 1, 130)}
	first, _ := wire.OpenCodeFreeRequestWithToolCutoff(original, catalog.FormatOpenAIChat, "same-conversation")
	second, _ := wire.OpenCodeFreeRequestWithToolCutoff(original, catalog.FormatOpenAIChat, "same-conversation")
	firstNames := make([]string, 0, len(first.Tools))
	for _, tool := range first.Tools {
		firstNames = append(firstNames, tool.Name)
	}
	secondNames := make([]string, 0, len(second.Tools))
	for _, tool := range second.Tools {
		secondNames = append(secondNames, tool.Name)
	}
	if !slices.Equal(firstNames, secondNames) {
		t.Fatalf("tool lists differ:\n%v\n%v", firstNames, secondNames)
	}
}

func TestOpenCodeFreeRequestLeavesAtTheLimitAlone(t *testing.T) {
	original := &inference.Request{Tools: freeToolCrowd(124, 0, 0)}
	got, _ := wire.OpenCodeFreeRequestWithToolCutoff(original, catalog.FormatOpenAIChat, "at-the-limit")
	if len(got.Tools) != 128 {
		t.Fatalf("tools = %d, want the four lane declarations beside the client's own", len(got.Tools))
	}
	if !slices.Equal(freeToolNamesList(got.Tools), freeToolNamesList(append(original.Tools, freeLaneToolDeclarations()...))) {
		t.Fatal("a request at the limit was reordered or trimmed")
	}
}

func freeToolCrowd(native, code, other int) []inference.Tool {
	tools := make([]inference.Tool, 0, native+code+other)
	for index := range native {
		tools = append(tools, inference.Tool{Name: fmt.Sprintf("Native%02d", index)})
	}
	for index := range code {
		tools = append(tools, inference.Tool{Name: fmt.Sprintf("mcp__codegraph__tool%d", index)})
	}
	for index := range other {
		tools = append(tools, inference.Tool{Name: fmt.Sprintf("mcp__other__tool%03d", index)})
	}
	return tools
}

func freeToolNames(tools []inference.Tool) map[string]bool {
	names := make(map[string]bool, len(tools))
	for _, tool := range tools {
		names[tool.Name] = true
	}
	return names
}

func freeToolNamesList(tools []inference.Tool) []string {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Name)
	}
	return names
}

func freeLaneToolDeclarations() []inference.Tool {
	tools := make([]inference.Tool, 0, len(freeLaneNames))
	for _, name := range freeLaneNames {
		tools = append(tools, inference.Tool{Name: name})
	}
	return tools
}

var freeLaneNames = []string{"bash", "glob", "grep", "read"}

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

// An argument named like a rejected keyword is not a keyword. The gateway
// refused the schema dialect, so only keywords inside a schema object are
// stripped; names inside a properties map name what a valid call carries.
func TestOpenCodeFreeRequestKeepsArgumentsNamedLikeRejectedKeywords(t *testing.T) {
	original := &inference.Request{Tools: []inference.Tool{{
		Name: "grep",
		Parameters: json.RawMessage(`{"type":"object","properties":{"pattern":` +
			`{"type":"string","description":"regex to search for","minLength":1},` +
			`"path":{"type":"string"}},"required":["pattern"],` +
			`"additionalProperties":false}`),
	}}}
	got, _ := wire.OpenCodeFreeRequest(original, catalog.FormatOpenAIResp)
	var cleaned map[string]any
	if err := json.Unmarshal(got.Tools[0].Parameters, &cleaned); err != nil {
		t.Fatalf("decode cleaned schema: %v", err)
	}
	properties, _ := cleaned["properties"].(map[string]any)
	pattern, _ := properties["pattern"].(map[string]any)
	if len(pattern) == 0 {
		t.Fatalf("properties = %v, want the pattern argument kept", properties)
	}
	if kind, _ := pattern["type"].(string); kind != "string" {
		t.Errorf("pattern type = %q, want the declaration's meaning kept", kind)
	}
	if _, found := pattern["minLength"]; found {
		t.Error("pattern kept \"minLength\", want the keyword inside its schema removed")
	}
	required, _ := cleaned["required"].([]any)
	if len(required) != 1 || required[0] != "pattern" {
		t.Errorf("required = %v, want the required argument still named", required)
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

// The paid gateways share the signed-out lane's tool-name limit, and a paid
// lane must keep the client's request otherwise whole: it declares no tool of
// its own, refuses no schema keyword, and dictates no ceiling.
func TestOpenCodeGatewayRequestShortensOnlyTheNames(t *testing.T) {
	original := &inference.Request{
		MaxTokens:  4,
		Tools:      []inference.Tool{{Name: mcpToolName, Parameters: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","pattern":"^[^\\0]*$"}}}`)}},
		ToolChoice: &inference.ToolChoice{Mode: inference.ToolChoiceTool, Name: mcpToolName},
		Messages: []inference.Message{{
			Role:      inference.RoleAssistant,
			ToolCalls: []inference.ToolCall{{ID: "call_1", Name: mcpToolName}},
		}},
	}
	got, originals := wire.OpenCodeGatewayRequest(original)

	alias := got.Tools[0].Name
	if alias == mcpToolName || len(alias) > 64 {
		t.Fatalf("tool name = %q, want a name of at most 64 characters", alias)
	}
	if originals[alias] != mcpToolName {
		t.Fatalf("originals = %v, want the alias read back as the client's name", originals)
	}
	if got.ToolChoice == nil || got.ToolChoice.Name != alias {
		t.Fatalf("tool choice = %+v, want the demand under the same alias", got.ToolChoice)
	}
	if got.Messages[0].ToolCalls[0].Name != alias {
		t.Fatalf("replayed call = %q, want the alias its declaration went out under",
			got.Messages[0].ToolCalls[0].Name)
	}
	if len(got.Tools) != 1 {
		t.Fatalf("tools = %v, want the client's own declarations and no lane tool", got.Tools)
	}
	if got.MaxTokens != 4 {
		t.Fatalf("max tokens = %d, want the client's own ceiling", got.MaxTokens)
	}
	if !strings.Contains(string(got.Tools[0].Parameters), "pattern") {
		t.Fatalf("parameters = %s, want the paid lane to keep the client's schema",
			got.Tools[0].Parameters)
	}
	if original.Tools[0].Name != mcpToolName || original.ToolChoice.Name != mcpToolName ||
		original.Messages[0].ToolCalls[0].Name != mcpToolName {
		t.Fatal("the caller's request was rewritten, want every name it arrived with")
	}
}

// A turn whose names all fit stays exactly as it is, so the paid lane pays
// nothing for a session that names nothing past the limit.
func TestOpenCodeGatewayRequestLeavesNamesWithinTheLimitAlone(t *testing.T) {
	got, originals := wire.OpenCodeGatewayRequest(&inference.Request{Tools: []inference.Tool{{Name: "read"}}})
	if got.Tools[0].Name != "read" {
		t.Fatalf("tool name = %q, want it left alone", got.Tools[0].Name)
	}
	if len(originals) != 0 {
		t.Fatalf("originals = %v, want no map for a turn that renamed nothing", originals)
	}
}

// A turn that was never built has nothing to shorten, and the caller is told
// so rather than handed a request it has to test for.
func TestOpenCodeGatewayRequestWithoutARequest(t *testing.T) {
	got, originals := wire.OpenCodeGatewayRequest(nil)
	if got != nil || originals != nil {
		t.Fatalf("request = %v originals = %v, want neither", got, originals)
	}
}

// OpenCodeGateway covers both lanes and both generations of the gateway's
// paths, and nothing else.
func TestOpenCodeGatewayRecognizesOnlyTheGatewayLanes(t *testing.T) {
	for _, baseURL := range []string{
		"https://opencode.ai/zen/v1",
		"https://opencode.ai/zen/go/v1",
		"https://opencode.ai/inference/v1",
		"https://opencode.ai/inference/go/v1",
	} {
		if !wire.OpenCodeGateway(baseURL) {
			t.Errorf("OpenCodeGateway(%q) = false, want the gateway lane", baseURL)
		}
	}
	for _, baseURL := range []string{
		"https://api.openai.com/v1",
		"https://opencode.ai/v1",
		"https://example.test/zen/v1",
		"",
	} {
		if wire.OpenCodeGateway(baseURL) {
			t.Errorf("OpenCodeGateway(%q) = true, want another connection left alone", baseURL)
		}
	}
}
