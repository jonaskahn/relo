package codec

import (
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
	got := wire.OpenCodeFreeRequest(original, catalog.FormatOpenAIChat)
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
	got := wire.OpenCodeFreeRequest(&inference.Request{}, catalog.FormatOpenAIResp)
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
	if got := wire.OpenCodeFreeRequest(full, catalog.FormatOpenAIChat); len(got.Tools) != 4 ||
		got.Tools[3].Description != "repo read" || got.ToolChoice != choice {
		t.Fatalf("request = %+v, want the client's own declarations kept", got)
	}

	partial := wire.OpenCodeFreeRequest(&inference.Request{
		Tools: []inference.Tool{real},
	}, catalog.FormatOpenAIChat)
	if len(partial.Tools) != 4 || partial.Tools[0].Description != "repo read" {
		t.Fatalf("tools = %v, want the real tool first and only the missing appended", partial.Tools)
	}
	if partial.ToolChoice != nil {
		t.Fatalf("tool choice = %+v, want a mixed tool set left alone", partial.ToolChoice)
	}
	if wire.OpenCodeFreeRequest(nil, catalog.FormatOpenAIChat) != nil {
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
	got := wire.OpenCodeFreeRequest(original, catalog.FormatOpenAIChat)
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
