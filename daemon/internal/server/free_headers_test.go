package server

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/wire"
	wireformats "github.com/jonaskahn/relo/internal/adapters/wire/formats"
	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/internal/inference"
	"github.com/jonaskahn/relo/internal/routing"
)

var freeProjectShape = regexp.MustCompile(`^[0-9a-f]{40}$`)

// freeHeadersServer builds the smallest server that resolves attempt
// headers: the free lane needs formats for the endpoint mode only.
func freeHeadersServer() *Server {
	return &Server{opts: Options{Formats: wireformats.New()}}
}

// freeHeadersSpec builds an attempt on the free template with the given
// inbound request and connection headers.
func freeHeadersSpec(r *http.Request, headers map[string]string) (attemptSpec, catalog.Template) {
	return attemptSpec{
		request:   r,
		candidate: routing.Candidate{Model: catalog.Model{ID: "m", APIFormat: catalog.FormatOpenAIChat}},
		outcome:   &requestOutcome{request: &inference.Request{Model: "m"}, requestID: "req-9"},
		host:      catalog.Provider{Headers: headers},
	}, catalog.Template{ID: catalog.OpenCodeFreeTemplate}
}

// TestAttemptHeadersRelaysTheGenuineIdentity covers a real client turn: its
// own agent, affinity, project, and parent travel upstream, so nothing is
// forged and the lane cache stays empty.
func TestAttemptHeadersRelaysTheGenuineIdentity(t *testing.T) {
	server := freeHeadersServer()
	affinity := wire.NativeSessionID()
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	r.Header.Set("User-Agent", "opencode/latest/2.0.29/cli")
	r.Header.Set("X-Session-Id", affinity)
	r.Header.Set("x-opencode-project", "3f786850e387550fdab836ed7e6dc881de23001b")
	r.Header.Set("x-opencode-parent-session-id", affinity)
	spec, template := freeHeadersSpec(r, nil)
	headers, err := server.attemptHeaders(spec, template, true)
	if err != nil {
		t.Fatalf("attemptHeaders() error = %v", err)
	}
	if headers["User-Agent"] != "opencode/latest/2.0.29/cli" {
		t.Fatalf("agent = %q, want the inbound client", headers["User-Agent"])
	}
	for _, name := range []string{"x-opencode-session", "x-opencode-session-id", "x-session-affinity", "x-session-id"} {
		if headers[name] != affinity {
			t.Fatalf("%s = %q, want the genuine affinity %q", name, headers[name], affinity)
		}
	}
	if headers["x-opencode-project"] != "3f786850e387550fdab836ed7e6dc881de23001b" {
		t.Fatalf("project = %q, want the inbound project", headers["x-opencode-project"])
	}
	if headers["x-opencode-parent-session-id"] != affinity || headers["x-parent-session-id"] != affinity {
		t.Fatalf("headers = %#v, want both parent spellings", headers)
	}
	if len(server.freeSessions) != 0 {
		t.Fatalf("lane cache holds %d sessions, want none forged", len(server.freeSessions))
	}
}

// TestAttemptHeadersForgesWhatTheClientNeverNamed covers an anonymous
// caller: the session is forged once per lane, the project rotates, the
// agent falls back to the pinned fallback, and the client key never travels.
func TestAttemptHeadersForgesWhatTheClientNeverNamed(t *testing.T) {
	server := freeHeadersServer()
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	r.Header.Set("Authorization", "Bearer client-key")
	spec, template := freeHeadersSpec(r, nil)
	first, err := server.attemptHeaders(spec, template, true)
	if err != nil {
		t.Fatalf("attemptHeaders() error = %v", err)
	}
	session := first["x-opencode-session"]
	if !wire.ValidSessionID(session) {
		t.Fatalf("session = %q, want a forged native shape", session)
	}
	if first["Authorization"] != "Bearer public" {
		t.Fatalf("authorization = %q, want the lane sentinel, never the client key", first["Authorization"])
	}
	if !freeProjectShape.MatchString(first["x-opencode-project"]) {
		t.Fatalf("project = %q, want a forged 40-hex id", first["x-opencode-project"])
	}
	if !strings.Contains(first["User-Agent"], "opencode/") {
		t.Fatalf("agent = %q, want the fallback client marker", first["User-Agent"])
	}
	if _, ok := first["x-opencode-parent-session-id"]; ok {
		t.Fatalf("headers = %#v, want no parent for an unnamed turn", first)
	}
	again, err := server.attemptHeaders(spec, template, true)
	if err != nil {
		t.Fatalf("attemptHeaders() error = %v", err)
	}
	if again["x-opencode-session"] != session {
		t.Fatalf("session changed from %q to %q", session, again["x-opencode-session"])
	}
	if again["x-opencode-request"] == first["x-opencode-request"] {
		t.Fatal("message id was reused across attempts")
	}
}

// TestAttemptHeadersPrefersTheOperatorProject covers a connection that sets
// its own project: it fills the gap the client left, ahead of the forge.
func TestAttemptHeadersPrefersTheOperatorProject(t *testing.T) {
	server := freeHeadersServer()
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	spec, template := freeHeadersSpec(r, map[string]string{"x-opencode-project": "op-1"})
	headers, err := server.attemptHeaders(spec, template, true)
	if err != nil {
		t.Fatalf("attemptHeaders() error = %v", err)
	}
	if headers["x-opencode-project"] != "op-1" {
		t.Fatalf("project = %q, want the operator override", headers["x-opencode-project"])
	}
}

// TestAttemptHeadersReadsEveryAffinitySpelling covers clients that name the
// conversation under an older header: the first non-blank one wins.
func TestAttemptHeadersReadsEveryAffinitySpelling(t *testing.T) {
	server := freeHeadersServer()
	affinity := wire.NativeSessionID()
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	r.Header.Set("X-Session-Id", affinity)
	spec, template := freeHeadersSpec(r, nil)
	headers, err := server.attemptHeaders(spec, template, true)
	if err != nil {
		t.Fatalf("attemptHeaders() error = %v", err)
	}
	if headers["x-opencode-session"] != affinity {
		t.Fatalf("session = %q, want the affinity %q", headers["x-opencode-session"], affinity)
	}
}

// TestAttemptHeadersWithoutARequest covers retries that outlive their
// inbound request: the lane still forges from nothing.
func TestAttemptHeadersWithoutARequest(t *testing.T) {
	server := freeHeadersServer()
	spec, template := freeHeadersSpec(nil, nil)
	spec.request = nil
	headers, err := server.attemptHeaders(spec, template, true)
	if err != nil {
		t.Fatalf("attemptHeaders() error = %v", err)
	}
	if !wire.ValidSessionID(headers["x-opencode-session"]) {
		t.Fatalf("session = %q, want a forged session", headers["x-opencode-session"])
	}
}
