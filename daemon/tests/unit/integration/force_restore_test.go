package integration_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/application/integration"
	"github.com/jonaskahn/relo/internal/platform"
)

// enabledOpenCode is an OpenCode integration wired over a config the operator
// wrote themselves, which is the machine where the snapshot is the only copy of
// their file Relo has.
func enabledOpenCode(t *testing.T) (*harness, string, string) {
	t.Helper()
	h := newHarness(t, dataPlane)
	config := filepath.Join(h.paths.Home, ".config", "opencode", "opencode.json")
	if err := os.MkdirAll(filepath.Dir(config), 0o755); err != nil {
		t.Fatalf("create the opencode directory: %v", err)
	}
	original := "{\"theme\":\"dark\"}\n"
	if err := os.WriteFile(config, []byte(original), 0o644); err != nil {
		t.Fatalf("seed the opencode config: %v", err)
	}
	window := int64(1000)
	h.manager = integration.New(integration.Options{
		Paths: platform.NewPathResolver(h.paths), Agents: platform.NewAgentRegistry(),
		Files: h.files, Store: h.store, Now: func() time.Time { return time.Unix(1_700_000_000, 0) },
		Models: func() []integration.ModelRef {
			return []integration.ModelRef{{ID: "relo-openai-gpt-4o", Name: "GPT-4o", ContextWindow: &window}}
		},
	})
	if _, err := h.manager.Enable(context.Background(), "opencode", "rlo_ak_secret"); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	return h, config, original
}

// TestRestoreWithoutItsSnapshotAsksToForce is a machine whose snapshot
// directory is gone: nothing Relo saw is there to put back, so the removal
// stops and asks rather than writing over a file it cannot restore.
func TestRestoreWithoutItsSnapshotAsksToForce(t *testing.T) {
	h, config, _ := enabledOpenCode(t)
	if err := os.RemoveAll(h.paths.SnapshotDir()); err != nil {
		t.Fatalf("remove the snapshots: %v", err)
	}

	if _, err := h.manager.Restore(context.Background(), "opencode", false); !errors.Is(err, integration.ErrRestoreBlocked) {
		t.Fatalf("Restore() error = %v, want ErrRestoreBlocked", err)
	}
	if got := readFile(t, config); !strings.Contains(got, "relo") {
		t.Fatalf("config = %s, want the provider left in place after a blocked restore", got)
	}
}

// TestForceRestoreStripsWhatIsOnDisk is the removal an operator confirms when
// the snapshot is gone: Relo's provider goes, their own settings stay, and the
// integration is off.
func TestForceRestoreStripsWhatIsOnDisk(t *testing.T) {
	h, config, _ := enabledOpenCode(t)
	if err := os.RemoveAll(h.paths.SnapshotDir()); err != nil {
		t.Fatalf("remove the snapshots: %v", err)
	}

	view, err := h.manager.Restore(context.Background(), "opencode", true)
	if err != nil {
		t.Fatalf("Restore(force) error = %v", err)
	}
	if view.Enabled || view.State != integration.StateOff {
		t.Fatalf("state = %q enabled = %v, want off", view.State, view.Enabled)
	}
	if got := readFile(t, config); strings.Contains(got, "relo") || !strings.Contains(got, "\"theme\":\"dark\"") {
		t.Fatalf("config = %s, want Relo gone and the theme kept", got)
	}
	if len(h.store.files["opencode"]) != 0 {
		t.Fatalf("file rows = %v, want none after a forced restore", h.store.files["opencode"])
	}
}

// TestForceRestoreStripsWhenASnapshotCannotBeRead covers a snapshot that is not
// gone but unreadable: the forced removal completes on that too, because the
// operator confirmed losing the copy either way.
func TestForceRestoreStripsWhenASnapshotCannotBeRead(t *testing.T) {
	h, config, _ := enabledOpenCode(t)
	for _, file := range h.store.files["opencode"] {
		if file.SnapshotPath == "" {
			continue
		}
		if err := os.RemoveAll(file.SnapshotPath); err != nil {
			t.Fatalf("remove the snapshot: %v", err)
		}
		if err := os.MkdirAll(file.SnapshotPath, 0o755); err != nil {
			t.Fatalf("put a directory where the snapshot was: %v", err)
		}
	}

	if _, err := h.manager.Restore(context.Background(), "opencode", true); err != nil {
		t.Fatalf("Restore(force) error = %v", err)
	}
	if got := readFile(t, config); strings.Contains(got, "relo") || !strings.Contains(got, "\"theme\":\"dark\"") {
		t.Fatalf("config = %s, want Relo gone and the theme kept", got)
	}
}

// TestRestoreUnrecognisedAsksThenForceDeletes is the rewrite-first,
// confirm-delete flow: a catalog file Relo cannot parse blocks a normal
// removal with a confirmable error, and a forced removal deletes that file
// and retires the integration.
func TestRestoreUnrecognisedAsksThenForceDeletes(t *testing.T) {
	h := newHarness(t, dataPlane)
	path := filepath.Join(h.paths.Home, ".pi", "agent", "models.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create the pi directory: %v", err)
	}
	if err := os.WriteFile(path, []byte("{\"providers\":{}}\n"), 0o644); err != nil {
		t.Fatalf("seed the catalog: %v", err)
	}
	if _, err := h.manager.Enable(context.Background(), "pi", "rlo_ak_secret"); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	if err := os.WriteFile(path, []byte("- just-a-list\n"), 0o644); err != nil {
		t.Fatalf("corrupt the catalog: %v", err)
	}

	if _, err := h.manager.Restore(context.Background(), "pi", false); !errors.Is(err, integration.ErrRestoreBlocked) {
		t.Fatalf("Restore() error = %v, want ErrRestoreBlocked", err)
	}
	if got := readFile(t, path); got != "- just-a-list\n" {
		t.Fatalf("catalog = %q, want the file untouched after a blocked restore", got)
	}
	if len(h.store.files["pi"]) == 0 {
		t.Fatal("file rows are gone, want them kept after a blocked restore")
	}

	view, err := h.manager.Restore(context.Background(), "pi", true)
	if err != nil {
		t.Fatalf("Restore(force) error = %v", err)
	}
	if view.Enabled || view.State != integration.StateOff {
		t.Fatalf("state = %q enabled = %v, want off", view.State, view.Enabled)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("catalog stat = %v, want the unrewritable file deleted", err)
	}
	if len(h.store.files["pi"]) != 0 {
		t.Fatalf("file rows = %v, want none after a forced restore", h.store.files["pi"])
	}
}
