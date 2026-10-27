package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jonaskahn/relo/internal/account"
	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/internal/inference"
	"github.com/jonaskahn/relo/internal/routing"
)

type attemptStubRelay struct {
	accounts []catalog.Authorization
	// requests are the credential contracts the stub was asked to authorize,
	// so a test can read what one attempt carried.
	requests []catalog.AuthRequest
	calls    int
}

func (r *attemptStubRelay) Authorize(_ context.Context, request catalog.AuthRequest) (catalog.Authorization, error) {
	r.calls++
	r.requests = append(r.requests, request)
	for _, account := range r.accounts {
		if account.CredentialID == "" {
			continue
		}
		excluded := false
		for _, id := range request.Selection.Exclude {
			if id == account.CredentialID {
				excluded = true
			}
		}
		if !excluded {
			return account, nil
		}
	}
	return catalog.Authorization{}, account.ErrNoCredentials
}

func (r *attemptStubRelay) RefreshRejected(_ context.Context, _, _, _ string) (catalog.Authorization, error) {
	return catalog.Authorization{}, nil
}

func (r *attemptStubRelay) Report(_ context.Context, _ Outcome) error { return nil }

func (r *attemptStubRelay) Execute(_ context.Context, _ Exchange, _ http.ResponseWriter) (Result, error) {
	return Result{}, nil
}

type attemptStubConfigs struct {
	configs map[string][]catalog.ModelRequestConfig
}

func (s attemptStubConfigs) ModelRequestConfigs(_ context.Context, _ string, credentialID string) ([]catalog.ModelRequestConfig, error) {
	return s.configs[credentialID], nil
}

func attemptTestServer(relay *attemptStubRelay, configs attemptStubConfigs) *Server {
	return &Server{opts: Options{Relay: relay, RequestConfigs: configs}}
}

func cursorTestSpec(authorized catalog.Authorization, effort string) *attemptSpec {
	outcome := &requestOutcome{
		requestID: "req-1",
		request:   &inference.Request{Model: "sonnet"},
	}
	if effort != "" {
		outcome.request.Reasoning = &inference.ReasoningConfig{Effort: effort}
	}
	return &attemptSpec{
		request:    httptest.NewRequest(http.MethodPost, "/", nil),
		candidate:  routing.Candidate{ProviderID: "cursor", ModelID: "sonnet", Model: catalog.Model{ID: "sonnet"}},
		outcome:    outcome,
		host:       catalog.Provider{ID: "cursor", Auth: catalog.AuthOAuth},
		authorized: authorized,
	}
}

func cursorVariant(group, selection, effort, details string) catalog.ModelRequestConfig {
	return catalog.EncodeCursorConfig(catalog.CursorVariant{
		GroupID: group, Selection: selection, Effort: effort,
		ModelID: "sonnet", DetailsID: details,
	})
}

func TestSelectCursorAttempt(t *testing.T) {
	first := catalog.Authorization{CredentialID: "cred-a", Label: "a", Token: "a"}
	second := catalog.Authorization{CredentialID: "cred-b", Label: "b", Token: "b"}
	configs := attemptStubConfigs{configs: map[string][]catalog.ModelRequestConfig{
		"cred-a": {
			cursorVariant("sonnet", "low", "low", "sonnet-low"),
			cursorVariant("sonnet", "high", "high", "sonnet-high"),
		},
		"cred-b": {
			cursorVariant("sonnet", "medium", "medium", "sonnet-medium"),
		},
	}}
	configs.configs["cred-a"][0].Default = true

	t.Run("an eligible account keeps the attempt", func(t *testing.T) {
		relay := &attemptStubRelay{accounts: []catalog.Authorization{first, second}}
		server := attemptTestServer(relay, configs)
		tried := map[string][]string{}
		selection, err := server.selectCursorAttempt(context.Background(), cursorTestSpec(first, "high"), tried)
		if err != nil {
			t.Fatalf("selectCursorAttempt() error = %v", err)
		}
		if selection.DetailsID != "sonnet-high" {
			t.Fatalf("selection = %+v, want the high tuple", selection)
		}
		if relay.calls != 0 {
			t.Fatalf("authorize calls = %d, want no re-authorization", relay.calls)
		}
	})

	t.Run("an ineligible account yields to the next one", func(t *testing.T) {
		relay := &attemptStubRelay{accounts: []catalog.Authorization{first, second}}
		server := attemptTestServer(relay, configs)
		tried := map[string][]string{}
		spec := cursorTestSpec(first, "medium")
		selection, err := server.selectCursorAttempt(context.Background(), spec, tried)
		if err != nil {
			t.Fatalf("selectCursorAttempt() error = %v", err)
		}
		if selection.DetailsID != "sonnet-medium" {
			t.Fatalf("selection = %+v, want the medium tuple", selection)
		}
		if spec.authorized.CredentialID != "cred-b" {
			t.Fatalf("authorized = %q, want the serving account", spec.authorized.CredentialID)
		}
		if len(tried["cursor"]) != 1 || tried["cursor"][0] != "cred-a" {
			t.Fatalf("tried = %v, want cred-a excluded", tried)
		}
	})

	t.Run("no serving account fails the selection", func(t *testing.T) {
		relay := &attemptStubRelay{accounts: []catalog.Authorization{first}}
		server := attemptTestServer(relay, configs)
		spec := cursorTestSpec(first, "medium")
		if _, err := server.selectCursorAttempt(context.Background(), spec, map[string][]string{}); err == nil {
			t.Fatal("selectCursorAttempt() succeeded without a serving account")
		}
	})

	t.Run("an unknown effort is refused", func(t *testing.T) {
		relay := &attemptStubRelay{accounts: []catalog.Authorization{first}}
		server := attemptTestServer(relay, configs)
		if _, err := server.selectCursorAttempt(context.Background(), cursorTestSpec(first, "nonsense"), map[string][]string{}); err == nil {
			t.Fatal("selectCursorAttempt() accepted an unknown effort")
		}
	})

	t.Run("an empty effort takes the default", func(t *testing.T) {
		relay := &attemptStubRelay{accounts: []catalog.Authorization{first}}
		server := attemptTestServer(relay, configs)
		selection, err := server.selectCursorAttempt(context.Background(), cursorTestSpec(first, ""), map[string][]string{})
		if err != nil {
			t.Fatalf("selectCursorAttempt() error = %v", err)
		}
		if selection.DetailsID != "sonnet-low" {
			t.Fatalf("selection = %+v, want the default tuple", selection)
		}
	})
}

func TestCursorConversationID(t *testing.T) {
	first := cursorConversationID("cursor", "cred-a", "session-1")
	if first == "" {
		t.Fatal("conversation id is empty")
	}
	if again := cursorConversationID("cursor", "cred-a", "session-1"); again != first {
		t.Fatalf("conversation = %q, want stability across turns", again)
	}
	if other := cursorConversationID("cursor", "cred-b", "session-1"); other == first {
		t.Fatal("accounts share a conversation id")
	}
	if other := cursorConversationID("cursor", "cred-a", "session-2"); other == first {
		t.Fatal("conversations share an id")
	}
	if len(first) != 36 || first[14] != '4' {
		t.Fatalf("conversation = %q, want a UUID", first)
	}
}

// TestCursorWireIDRotatesAfterHandoff pins the resume path: the client keeps
// one anchor, but each native handoff moves the wire conversation on, because
// the old one still records an execution awaiting an in-stream result.
func TestCursorWireIDRotatesAfterHandoff(t *testing.T) {
	server := &Server{}
	first := server.cursorWireID("cursor", "cred-a", "session-1")
	if again := server.cursorWireID("cursor", "cred-a", "session-1"); again != first {
		t.Fatalf("wire id = %q, want stability before a handoff", again)
	}
	server.cursorWires.rotate("cursor", "cred-a", "session-1")
	second := server.cursorWireID("cursor", "cred-a", "session-1")
	if second == first {
		t.Fatal("wire id did not rotate after the handoff")
	}
	if len(second) != 36 || second[14] != '4' {
		t.Fatalf("wire id = %q, want a UUID", second)
	}
	if other := server.cursorWireID("cursor", "cred-a", "session-2"); other == second || other == first {
		t.Fatal("conversations share a wire id")
	}
}

// TestCursorRotateWireMovesTheNextRequest covers Cursor's failed_precondition
// on a reused conversation: the request is sound, so the next request for the
// same client conversation belongs on a fresh wire id.
func TestCursorRotateWireMovesTheNextRequest(t *testing.T) {
	server := &Server{}
	request := httptest.NewRequest(http.MethodPost, "/", nil)
	request.Header.Set("session-id", "session-1")
	outcome := &requestOutcome{requestID: "req-1"}
	driver := &sendDriver{s: server, r: request, outcome: outcome}
	failed := failedAttempt{
		candidate: routing.Candidate{ProviderID: "cursor"},
		result:    Result{CredentialID: "cred-a", RotateWire: true},
	}
	before := server.cursorWireID("cursor", "cred-a", "session-1")
	driver.rotateWire(failed)
	if after := server.cursorWireID("cursor", "cred-a", "session-1"); after == before {
		t.Fatal("the retry stayed on the refused wire conversation")
	}
	unchanged := server.cursorWireID("cursor", "cred-a", "session-1")
	driver.rotateWire(failedAttempt{result: Result{CredentialID: "cred-a"}})
	if again := server.cursorWireID("cursor", "cred-a", "session-1"); again != unchanged {
		t.Fatal("an ordinary refusal rotated the wire conversation")
	}
}
