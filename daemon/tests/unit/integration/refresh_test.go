package integration_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/application/integration"
)

// outdatedPi enables Pi, then moves the model list so the file Relo wrote is
// untouched but no longer what this build would write, which is what an
// upgrade leaves behind.
func outdatedPi(t *testing.T) (*harness, string, *[]integration.ModelRef) {
	t.Helper()
	h := newHarness(t, dataPlane)
	models := []integration.ModelRef{{ID: "relo-a-one", Name: "One", Connection: "A"}}
	withModels(h, &models)
	if _, err := h.manager.Enable(context.Background(), "pi", "rlo_ak_test"); err != nil {
		t.Fatalf("Enable(pi) error = %v", err)
	}
	path := filepath.Join(h.paths.Home, ".pi", "agent", "models.json")
	models = []integration.ModelRef{{ID: "relo-b-two", Name: "Two", Connection: "B"}}
	return h, path, &models
}

func piFile(t *testing.T, h *harness) integration.FileView {
	t.Helper()
	view, err := h.manager.Inspect(context.Background(), "pi")
	if err != nil {
		t.Fatalf("Inspect(pi) error = %v", err)
	}
	for _, file := range view.Files {
		if file.Kind == "pi-models" {
			return file
		}
	}
	t.Fatalf("Inspect(pi) files = %+v, want the pi catalog", view.Files)
	return integration.FileView{}
}

// TestOutdatedFileIsNotReportedAsChanged keeps an upgrade from raising Repair
// on a file nobody touched.
func TestOutdatedFileIsNotReportedAsChanged(t *testing.T) {
	h, _, _ := outdatedPi(t)
	file := piFile(t, h)
	if file.Drifted || !file.Outdated {
		t.Fatalf("file = %+v, want outdated and not drifted", file)
	}
	view, err := h.manager.Inspect(context.Background(), "pi")
	if err != nil {
		t.Fatalf("Inspect(pi) error = %v", err)
	}
	if reasonOfFound(view, integration.ReasonFileChanged) || view.State != integration.StateOn {
		t.Fatalf("view = %+v, want no file_changed and the state on", view)
	}
}

// TestRefreshHealsAnOutdatedFile is the silent half: the file follows this
// build without the operator doing anything.
func TestRefreshHealsAnOutdatedFile(t *testing.T) {
	h, path, _ := outdatedPi(t)
	if err := h.manager.RefreshOutdatedFiles(context.Background(), "pi", "rlo_ak_test"); err != nil {
		t.Fatalf("RefreshOutdatedFiles() error = %v", err)
	}
	if !strings.Contains(readFile(t, path), "relo-b-two") {
		t.Fatalf("catalog = %s, want the new model", readFile(t, path))
	}
	view, err := h.manager.Inspect(context.Background(), "pi")
	if err != nil {
		t.Fatalf("Inspect(pi) error = %v", err)
	}
	if file := piFile(t, h); file.Outdated || file.Drifted || view.ModelsStale {
		t.Fatalf("file = %+v stale = %v, want current", file, view.ModelsStale)
	}
	if len(h.store.ops) == 0 || h.store.ops[len(h.store.ops)-1].Action != "refresh" {
		t.Fatalf("ops = %+v, want one refresh", h.store.ops)
	}
}

// TestRefreshNeverOverwritesAnEdit keeps the operator's change: an edited file
// stays as it is and is reported until Repair.
func TestRefreshNeverOverwritesAnEdit(t *testing.T) {
	h, path, _ := outdatedPi(t)
	edited := strings.Replace(readFile(t, path), "relo-a-one", "relo-hand-edited", 1)
	if err := os.WriteFile(path, []byte(edited), 0o644); err != nil {
		t.Fatalf("edit the catalog: %v", err)
	}
	if file := piFile(t, h); !file.Drifted || file.Outdated {
		t.Fatalf("file = %+v, want drifted", file)
	}
	if err := h.manager.RefreshOutdatedFiles(context.Background(), "pi", "rlo_ak_test"); err != nil {
		t.Fatalf("RefreshOutdatedFiles() error = %v", err)
	}
	if readFile(t, path) != edited {
		t.Fatalf("catalog = %s, want the edit kept", readFile(t, path))
	}
	view, _ := h.manager.Inspect(context.Background(), "pi")
	if !reasonOfFound(view, integration.ReasonFileChanged) {
		t.Fatalf("reasons = %+v, want file_changed", view.Reasons)
	}
}

// TestRefreshLeavesAnAgentWithAnEditedFileAlone covers the blocking rule: one
// edited file keeps Relo from rewriting its neighbour.
func TestRefreshLeavesAnAgentWithAnEditedFileAlone(t *testing.T) {
	h := newHarness(t, dataPlane)
	models := []integration.ModelRef{{ID: "relo-a-one", Name: "One", Connection: "A"}}
	withModels(h, &models)
	ctx := context.Background()
	if _, err := h.manager.Enable(ctx, "cline", "rlo_ak_test"); err != nil {
		t.Fatalf("Enable(cline) error = %v", err)
	}
	root := filepath.Join(h.paths.Home, ".cline", "data", "settings")
	providers, catalog := filepath.Join(root, "providers.json"), filepath.Join(root, "models.json")
	models = []integration.ModelRef{{ID: "relo-b-two", Name: "Two", Connection: "B"}}
	edited := strings.Replace(readFile(t, catalog), "relo-a-one", "relo-hand-edited", 1)
	if err := os.WriteFile(catalog, []byte(edited), 0o644); err != nil {
		t.Fatalf("edit the models: %v", err)
	}
	before := readFile(t, providers)
	if err := h.manager.RefreshOutdatedFiles(ctx, "cline", "rlo_ak_test"); err != nil {
		t.Fatalf("RefreshOutdatedFiles() error = %v", err)
	}
	if readFile(t, providers) != before || readFile(t, catalog) != edited {
		t.Fatal("refresh wrote a file of an agent with an edited file")
	}
}

// TestRefreshIgnoresAnEditElsewhere covers a file changed outside Relo's block:
// nothing to report and nothing to rewrite.
func TestRefreshIgnoresAnEditElsewhere(t *testing.T) {
	h := newHarness(t, dataPlane)
	models := []integration.ModelRef{{ID: "relo-a-one", Name: "One", Connection: "A"}}
	withModels(h, &models)
	ctx := context.Background()
	if _, err := h.manager.Enable(ctx, "pi", "rlo_ak_test"); err != nil {
		t.Fatalf("Enable(pi) error = %v", err)
	}
	path := filepath.Join(h.paths.Home, ".pi", "agent", "models.json")
	edited := strings.Replace(readFile(t, path), "{", `{"note":"kept",`, 1)
	if err := os.WriteFile(path, []byte(edited), 0o644); err != nil {
		t.Fatalf("edit the catalog: %v", err)
	}
	file := piFile(t, h)
	view, _ := h.manager.Inspect(ctx, "pi")
	if file.Drifted || file.Outdated || len(view.Reasons) != 0 {
		t.Fatalf("file = %+v reasons = %+v, want nothing to report", file, view.Reasons)
	}
	if err := h.manager.RefreshOutdatedFiles(ctx, "pi", "rlo_ak_test"); err != nil {
		t.Fatalf("RefreshOutdatedFiles() error = %v", err)
	}
	if readFile(t, path) != edited {
		t.Fatalf("catalog = %s, want it untouched", readFile(t, path))
	}
}
