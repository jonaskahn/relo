package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestConversationOfReadsTheClientThread(t *testing.T) {
	t.Run("codex parent and child are one conversation", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodPost, "/", nil)
		request.Header.Set("x-codex-parent-thread-id", "parent")
		request.Header.Set("thread-id", "child")
		request.Header.Set("x-opencode-session", "oc")
		if got := conversationOf(request); got != "codex-thread:parent\x00child" {
			t.Fatalf("conversation = %q, want the parent and child pair", got)
		}
	})

	t.Run("a codex parent alone still names the conversation", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodPost, "/", nil)
		request.Header.Set("x-codex-parent-thread-id", "parent")
		if got := conversationOf(request); got != "codex-thread:parent" {
			t.Fatalf("conversation = %q, want the parent thread", got)
		}
	})

	t.Run("a root thread id does not replace the session header", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodPost, "/", nil)
		request.Header.Set("thread-id", "root")
		request.Header.Set("session-id", "sess")
		if got := conversationOf(request); got != "sess" {
			t.Fatalf("conversation = %q, want the session header", got)
		}
	})

	t.Run("opencode wins over a generic session id", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodPost, "/", nil)
		request.Header.Set("x-opencode-session", "oc-1")
		request.Header.Set("session_id", "other")
		if got := conversationOf(request); got != "oc-1" {
			t.Fatalf("conversation = %q, want the opencode session", got)
		}
	})

	t.Run("a request with no conversation keeps one opencode lane", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodPost, "/", nil)
		outcome := &requestOutcome{requestID: "req-1"}
		first := openCodeLane(outcome, request)
		again := openCodeLane(outcome, request)
		if first != "req-1" || again != first {
			t.Fatalf("lane = %q, again = %q", first, again)
		}
	})

	t.Run("a named conversation is the opencode lane", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodPost, "/", nil)
		request.Header.Set("session-id", "sess")
		outcome := &requestOutcome{requestID: "req-1"}
		if got := openCodeLane(outcome, request); got != "sess" {
			t.Fatalf("lane = %q, want the session", got)
		}
	})

	t.Run("a conversation id is still accepted", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodPost, "/", nil)
		request.Header.Set("conversation_id", "conv")
		if got := conversationOf(request); got != "conv" {
			t.Fatalf("conversation = %q, want the conversation id", got)
		}
	})
}
