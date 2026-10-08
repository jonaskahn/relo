package service_test

import (
	"context"
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/access"
	"github.com/jonaskahn/relo/internal/adapters/codingclients"
)

// TestEveryIntegrationClientHasASetupPath is the contract the console reads:
// a client Relo does not write files for must still hand an operator the exact
// steps that point it at this machine, and a client Relo does configure must
// not claim to have hand-written steps.
func TestEveryIntegrationClientHasASetupPath(t *testing.T) {
	_, built := integrationService(t)
	views, err := built.Integrations(context.Background())
	if err != nil {
		t.Fatalf("Integrations() error = %v", err)
	}
	if len(views) != len(codingclients.Agents()) {
		t.Fatalf("Integrations() = %d entries, want %d", len(views), len(codingclients.Agents()))
	}
	for _, view := range views {
		t.Run(view.ID, func(t *testing.T) {
			if view.ManagesFiles {
				if len(view.Steps) != 0 {
					t.Fatalf("steps = %v, want none for a client Relo writes files for", view.Steps)
				}
				return
			}
			if len(view.Steps) == 0 {
				t.Fatal("steps are empty, want what an operator does by hand")
			}
			joined := strings.Join(view.Steps, "\n")
			if view.BaseURL != "" && !strings.Contains(joined, view.BaseURL) {
				t.Fatalf("steps = %q, want this machine's address %q", joined, view.BaseURL)
			}
			if strings.Contains(joined, access.BaseURLPlaceholder) {
				t.Fatalf("steps = %q, want the address filled in", joined)
			}
		})
	}
}

// TestASetupStepFillsEveryPlaceholder keeps the steps an operator copies
// ready to paste: the address, the key it owns, and a model are all filled in
// rather than left as the template markers they started as.
func TestASetupStepFillsEveryPlaceholder(t *testing.T) {
	_, built := integrationService(t)
	view, err := built.Integration(context.Background(), "cursor")
	if err != nil {
		t.Fatalf("Integration() error = %v", err)
	}
	joined := strings.Join(view.Steps, "\n")
	for _, marker := range []string{access.ModelPlaceholder, access.BaseURLPlaceholder} {
		if strings.Contains(joined, marker) {
			t.Fatalf("steps = %q, want %q filled in", joined, marker)
		}
	}
	if !strings.Contains(joined, view.BaseURL) {
		t.Fatalf("steps = %q, want this machine's address %q", joined, view.BaseURL)
	}
}
