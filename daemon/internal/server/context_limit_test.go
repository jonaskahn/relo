package server

import (
	"errors"
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/wire"
	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/internal/inference"
	"github.com/jonaskahn/relo/internal/routing"
)

func TestAttemptRejectsExplicitlyUnsupportedCapabilities(t *testing.T) {
	no := false
	server := New(Options{})
	spec := attemptSpec{candidate: routing.Candidate{Model: catalog.Model{ID: "m", SupportsTools: &no}}}
	request := &inference.Request{Tools: []inference.Tool{{Name: "exec"}}}
	if err := server.validateOutboundRequest(spec, request); !errors.Is(err, wire.ErrUnsupportedFeature) {
		t.Fatalf("validateOutboundRequest() = %v, want unsupported tools", err)
	}
}

func TestAttemptReducesOutputToFitProviderContext(t *testing.T) {
	server := New(Options{})
	maxInput := int64(1_000)
	contextWindow := int64(256)
	maxOutput := int64(128)
	spec := attemptSpec{candidate: routing.Candidate{Model: catalog.Model{
		ID: "m", MaxInput: &maxInput, ContextWindow: &contextWindow, MaxOutput: &maxOutput,
	}}}
	request := &inference.Request{
		Messages: []inference.Message{{Role: inference.RoleUser, Content: []inference.ContentPart{{
			Type: inference.ContentTypeText, Text: strings.Repeat("token ", 40),
		}}}}, MaxTokens: 1000,
	}
	if err := server.validateOutboundRequest(spec, request); err != nil {
		t.Fatalf("validateOutboundRequest() = %v, want a reduced but valid request", err)
	}
	if request.MaxTokens <= 0 || request.MaxTokens >= 1000 {
		t.Fatalf("MaxTokens = %d, want the request reduced to the available context", request.MaxTokens)
	}
}

func TestAttemptRejectsInputThatCannotFit(t *testing.T) {
	server := New(Options{})
	maxInput := int64(8)
	contextWindow := int64(8)
	spec := attemptSpec{candidate: routing.Candidate{Model: catalog.Model{
		ID: "m", MaxInput: &maxInput, ContextWindow: &contextWindow,
	}}}
	request := &inference.Request{
		Messages: []inference.Message{{Role: inference.RoleUser, Content: []inference.ContentPart{{
			Type: inference.ContentTypeText, Text: strings.Repeat("token ", 100),
		}}}}, MaxTokens: 64,
	}
	if err := server.validateOutboundRequest(spec, request); !errors.Is(err, errContextLimitExceeded) {
		t.Fatalf("validateOutboundRequest() = %v, want an input refusal", err)
	}
}
