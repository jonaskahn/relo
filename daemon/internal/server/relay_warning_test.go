package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/internal/inference"
	"github.com/jonaskahn/relo/internal/routing"
)

// warningStubTemplates serves one curated template: the connection under
// test refuses an output ceiling or it does not.
type warningStubTemplates struct {
	template catalog.Template
}

func (s warningStubTemplates) Curated(id string) (catalog.Template, bool) {
	if id == "" {
		return catalog.Template{}, false
	}
	return s.template, true
}

func (s warningStubTemplates) ByFlow(string) (catalog.Template, bool) {
	return catalog.Template{}, false
}

func warningTestSpec(maxTokens int, warnings ...string) *attemptSpec {
	return &attemptSpec{
		request:   httptest.NewRequest(http.MethodPost, "/", nil),
		candidate: routing.Candidate{ProviderID: "cursor", ModelID: "sonnet", Warnings: warnings},
		outcome: &requestOutcome{
			requestID: "req-1",
			request:   &inference.Request{Model: "sonnet", MaxTokens: maxTokens},
		},
		host:       catalog.Provider{ID: "cursor", TemplateID: "cursor"},
		authorized: catalog.Authorization{Label: "a"},
	}
}

func warningTestServer(refuses bool) *Server {
	return &Server{opts: Options{Templates: warningStubTemplates{
		template: catalog.Template{ID: "cursor", RefusesMaxOutputTokens: refuses},
	}}}
}

// TestDroppedCeilingWarning covers the honesty side of leaving a ceiling
// out: a request that states one against a dialect without the parameter
// warns on the answer, and nothing else changes.
func TestDroppedCeilingWarning(t *testing.T) {
	t.Run("a stated ceiling on a refusing dialect warns", func(t *testing.T) {
		server := warningTestServer(true)
		spec := warningTestSpec(1024, "the model may not call tools")
		recorder := httptest.NewRecorder()
		server.announceAttempt(recorder, *spec)
		header := recorder.Header().Get(warningHeader)
		if !strings.Contains(header, "the model may not call tools") ||
			!strings.Contains(header, "the model ignores the max-output ceiling") {
			t.Fatalf("warning = %q, want both warnings", header)
		}
		if len(spec.outcome.warnings) != 2 {
			t.Fatalf("warnings = %v, want the pair recorded", spec.outcome.warnings)
		}
		if len(spec.candidate.Warnings) != 1 {
			t.Fatalf("candidate warnings = %v, want them unmutated", spec.candidate.Warnings)
		}
	})

	t.Run("an unstated ceiling warns nothing", func(t *testing.T) {
		server := warningTestServer(true)
		spec := warningTestSpec(0)
		recorder := httptest.NewRecorder()
		server.announceAttempt(recorder, *spec)
		if header := recorder.Header().Get(warningHeader); header != "" {
			t.Fatalf("warning = %q, want none", header)
		}
	})

	t.Run("a dialect with a ceiling warns nothing", func(t *testing.T) {
		server := warningTestServer(false)
		spec := warningTestSpec(1024)
		recorder := httptest.NewRecorder()
		server.announceAttempt(recorder, *spec)
		if header := recorder.Header().Get(warningHeader); header != "" {
			t.Fatalf("warning = %q, want none", header)
		}
	})
}
