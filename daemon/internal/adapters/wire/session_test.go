package wire

import (
	"net/http/httptest"
	"testing"
)

func TestSessionAnchorNamesEveryClientConversation(t *testing.T) {
	cases := []struct {
		name   string
		header map[string]string
		want   string
	}{
		{"a claude code session header", map[string]string{"x-claude-code-session-id": "cc-1"}, "cc-1"},
		{"a session affinity header", map[string]string{"x-session-affinity": "aff-1"}, "aff-1"},
		{"an opencode session id header", map[string]string{"x-opencode-session-id": "oc-9"}, "oc-9"},
		{
			"a codex turn envelope", map[string]string{
				"x-codex-turn-metadata": `{"session_id":"turn-7","turn":2}`,
			}, "codex-turn:turn-7",
		},
		{
			"a malformed codex turn envelope falls back", map[string]string{
				"x-codex-turn-metadata": `not json`, "session_id": "plain",
			}, "plain",
		},
		{
			"a generic session id keeps its existing precedence", map[string]string{
				"session_id": "other", "x-claude-code-session-id": "cc-1",
			}, "other",
		},
		{
			"an opencode session beats a claude code one", map[string]string{
				"x-opencode-session": "oc-1", "x-claude-code-session-id": "cc-1",
			}, "oc-1",
		},
		{
			"a codex parent and child stay one anchor", map[string]string{
				"x-codex-parent-thread-id": "parent", "thread-id": "child",
				"x-claude-code-session-id": "cc-1",
			}, "codex-thread:parent\x00child",
		},
		{"a request with no session header", map[string]string{"user-agent": "claude-cli/1"}, ""},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			request := httptest.NewRequest("POST", "/", nil)
			for name, value := range testCase.header {
				request.Header.Set(name, value)
			}
			if got := SessionAnchor(request.Header); got != testCase.want {
				t.Fatalf("SessionAnchor = %q, want %q", got, testCase.want)
			}
		})
	}
}

func TestClientKindNamesTheRequestingFamily(t *testing.T) {
	cases := []struct {
		name   string
		header map[string]string
		want   string
	}{
		{"opencode session headers", map[string]string{"x-opencode-session": "oc-1"}, "opencode"},
		{"the opencode client header", map[string]string{"x-opencode-client": "cli"}, "opencode"},
		{"the claude cli agent", map[string]string{"user-agent": "claude-cli/2.1.0"}, "claude-code"},
		{"a codex originator", map[string]string{"originator": "codex_cli"}, "codex"},
		{"a codex thread id", map[string]string{"thread-id": "child"}, "codex"},
		{"a client relo does not name", map[string]string{"user-agent": "curl/8"}, "other"},
		{"a request with no identity", map[string]string{}, ""},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			request := httptest.NewRequest("POST", "/", nil)
			for name, value := range testCase.header {
				request.Header.Set(name, value)
			}
			if got := ClientKind(request.Header); got != testCase.want {
				t.Fatalf("ClientKind = %q, want %q", got, testCase.want)
			}
		})
	}
}

func TestSessionAnchorWithoutAHeader(t *testing.T) {
	if got := SessionAnchor(nil); got != "" {
		t.Fatalf("SessionAnchor(nil) = %q, want an empty anchor", got)
	}
}
