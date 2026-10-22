package server

import (
	"testing"

	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/internal/inference"
	"github.com/jonaskahn/relo/internal/routing"
)

// TestModelRequestOutputDefault pins the wire-side ceiling for a client that
// names no max tokens: when no fact layer described the model either, the
// request carries the catalog default instead of letting the upstream gateway
// quietly apply its own (often 32K). A stated client ceiling and a model a
// layer described both keep today's pass-through.
func TestModelRequestOutputDefault(t *testing.T) {
	silent := &attemptSpec{outcome: &requestOutcome{request: &inference.Request{}}}
	ceiling := catalog.DefaultMaxOutput
	model := routing.Candidate{
		Model: catalog.Model{
			UpstreamID: "beam-free",
			MaxOutput:  &ceiling,
		},
	}
	stated := &attemptSpec{
		outcome:   &requestOutcome{request: &inference.Request{MaxTokens: 1234}},
		candidate: model,
	}
	described := &attemptSpec{outcome: silent.outcome}
	described.candidate.Model.FactSources.MaxOutput = "modelsdev"

	t.Run("a silent client carries the catalog default when no layer stated a ceiling", func(t *testing.T) {
		sent, originals := (&Server{}).modelRequest(*silent, catalog.Template{}, false)
		if originals != nil {
			t.Fatalf("originals = %v, want none on a plain lane", originals)
		}
		if sent.MaxTokens != int(catalog.DefaultMaxOutput) {
			t.Fatalf("MaxTokens = %d, want the %d default", sent.MaxTokens, catalog.DefaultMaxOutput)
		}
	})

	t.Run("a stated client ceiling stays untouched", func(t *testing.T) {
		if sent, _ := (&Server{}).modelRequest(*stated, catalog.Template{}, false); sent.MaxTokens != 1234 {
			t.Fatalf("MaxTokens = %d, want the client's 1234", sent.MaxTokens)
		}
	})

	t.Run("a model a layer described keeps pass-through", func(t *testing.T) {
		if sent, _ := (&Server{}).modelRequest(*described, catalog.Template{}, false); sent.MaxTokens != 0 {
			t.Fatalf("MaxTokens = %d, want no ceiling sent", sent.MaxTokens)
		}
	})
}
