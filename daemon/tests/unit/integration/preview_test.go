package integration_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/application/integration"
)

// TestPreviewNamesWhatASetUpWouldWrite is the promise that nothing touches the
// machine until the operator presses the button: what would be written is
// readable first.
func TestPreviewNamesWhatASetUpWouldWrite(t *testing.T) {
	h := newHarness(t, dataPlane)
	planned, err := h.manager.Preview(context.Background(), "codex")
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}
	kinds := map[string]integration.PlannedFile{}
	for _, plan := range planned {
		kinds[plan.Kind] = plan
	}
	config, found := kinds[integration.KindCodexConfig]
	if !found {
		t.Fatalf("preview = %+v, want the Codex configuration named", planned)
	}
	if config.Path != h.paths.CodexConfig() || !strings.Contains(config.Fragment, "model_providers.relo") {
		t.Fatalf("preview config = %+v, want the block that would be written", config)
	}
	if _, found := kinds[integration.KindKey]; !found {
		t.Fatalf("preview = %+v, want the key file named", planned)
	}

	// A file Relo cannot merge is refused in the preview rather than at the
	// moment the operator presses Set up.
	configPath := h.paths.CodexConfig()
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("create the codex directory: %v", err)
	}
	if err := os.WriteFile(configPath, []byte("[broken\nkey = \"unterminated"), 0o644); err != nil {
		t.Fatalf("write the broken config: %v", err)
	}
	planned, err = h.manager.Preview(context.Background(), "codex")
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}
	for _, plan := range planned {
		if plan.Kind == integration.KindCodexConfig {
			if !plan.Refused || plan.Reason == "" {
				t.Fatalf("preview config = %+v, want the refusal named", plan)
			}
		}
	}
	if _, err := h.manager.Preview(context.Background(), "nope"); err == nil {
		t.Fatal("Preview() on an unknown agent error = nil, want a refusal")
	}
	if _, err := h.manager.Enable(context.Background(), "codex", "rlo_ak_test"); err == nil {
		t.Fatal("Enable() error = nil for a config Relo cannot merge, want a refusal")
	}
}
