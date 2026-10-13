package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jonaskahn/relo/internal/account"
	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/internal/routing"
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

func TestConversationAnchorLeasesASession(t *testing.T) {
	t.Run("a named conversation always wins", func(t *testing.T) {
		server := &Server{opts: Options{SessionLeases: account.NewSessionLeases(0, 0, nil)}}
		request := httptest.NewRequest(http.MethodPost, "/", nil)
		request.Header.Set("session-id", "sess")
		outcome := &requestOutcome{requestID: "req-1"}
		if got := server.conversationAnchor(outcome, request); got != "sess" {
			t.Fatalf("anchor = %q, want the named conversation", got)
		}
	})

	t.Run("one unlabelled client keeps one session across requests", func(t *testing.T) {
		server := &Server{opts: Options{SessionLeases: account.NewSessionLeases(0, 0, nil)}}
		first := &requestOutcome{requestID: "req-1"}
		request := httptest.NewRequest(http.MethodPost, "/", nil)
		anchor := server.conversationAnchor(first, request)
		if anchor == "" {
			t.Fatal("anchor is empty, want a leased session")
		}
		if again := server.conversationAnchor(first, request); again != anchor {
			t.Fatalf("anchor = %q, want the leased session %q", again, anchor)
		}
		second := &requestOutcome{requestID: "req-2"}
		if again := server.conversationAnchor(second, request); again != anchor {
			t.Fatalf("anchor = %q on the next request, want the leased session", again)
		}
	})

	t.Run("one retry attempt stays on the request session", func(t *testing.T) {
		server := &Server{opts: Options{SessionLeases: account.NewSessionLeases(0, 0, nil)}}
		request := httptest.NewRequest(http.MethodPost, "/", nil)
		outcome := &requestOutcome{requestID: "req-1"}
		anchor := server.conversationAnchor(outcome, request)
		if again := server.conversationAnchor(outcome, request); again != anchor {
			t.Fatalf("anchor = %q on the retry, want the leased session", again)
		}
	})

	t.Run("a server without leases leaves the anchor empty", func(t *testing.T) {
		server := &Server{}
		request := httptest.NewRequest(http.MethodPost, "/", nil)
		outcome := &requestOutcome{requestID: "req-1"}
		if got := server.conversationAnchor(outcome, request); got != "" {
			t.Fatalf("anchor = %q, want no session without a store", got)
		}
	})
}

// TestNativeBodyFollowsTheWireFamily pins that the client's own Messages body
// is reused whenever the attempt speaks the Messages wire, whatever spelling
// named the target: an alias or a route member must not cost the client its
// native controls. A candidate on another wire family rebuilds.
func TestNativeBodyFollowsTheWireFamily(t *testing.T) {
	server := &Server{}
	native := []byte(`{"model":"claude-sonnet-5-5"}`)
	messages := routing.Candidate{Model: catalog.Model{APIFormat: catalog.FormatAnthropic, UpstreamID: "claude-sonnet-5-5"}}
	if got := server.nativeBody(&requestOutcome{requested: "claude-relo-claude--claude-sonnet-5-5", native: native}, messages); string(got) != string(native) {
		t.Fatalf("native body = %q, want the original request", got)
	}

	t.Run("another wire family rebuilds from the canonical request", func(t *testing.T) {
		chat := routing.Candidate{Model: catalog.Model{APIFormat: catalog.FormatOpenAIChat, UpstreamID: "gpt-5"}}
		if got := server.nativeBody(&requestOutcome{requested: "claude-sonnet-5-5", native: native}, chat); got != nil {
			t.Fatalf("native body = %q, want no reuse across wire families", got)
		}
	})

	t.Run("a request without a native body rebuilds", func(t *testing.T) {
		if got := server.nativeBody(&requestOutcome{requested: "claude-sonnet-5-5"}, messages); got != nil {
			t.Fatalf("native body = %q, want no reuse without one", got)
		}
	})
}
