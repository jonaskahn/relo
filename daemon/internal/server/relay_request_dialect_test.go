package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/wire/formats"
	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/internal/routing"
)

// TestAttemptCodecOptionsCarriesTheModelDialect pins that a gateway without
// a connection-level dialect still hands the decoder the one its model family
// uses, and that a connection-level dialect keeps precedence.
func TestAttemptCodecOptionsCarriesTheModelDialect(t *testing.T) {
	server := New(Options{Formats: formats.New()})
	candidate := routing.Candidate{Model: catalog.Model{
		ID: "mimo-v2.6-pro", ToolCallDialect: catalog.ToolCallDialectText,
	}}

	spec := attemptSpec{
		candidate: candidate, outcome: &requestOutcome{},
		request: httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil),
	}
	opts := server.attemptCodecOptions(spec, attemptCodecInputs{})
	if opts.ToolCallDialect != catalog.ToolCallDialectText {
		t.Fatalf("dialect = %q, want the model's %q", opts.ToolCallDialect, catalog.ToolCallDialectText)
	}

	spec.host = catalog.Provider{ToolCallDialect: "host-dialect"}
	opts = server.attemptCodecOptions(spec, attemptCodecInputs{})
	if opts.ToolCallDialect != "host-dialect" {
		t.Fatalf("dialect = %q, want the connection's own dialect to win", opts.ToolCallDialect)
	}
}
