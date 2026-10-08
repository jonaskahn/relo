package integration_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/codingclients"
	"github.com/jonaskahn/relo/internal/application/integration"
)

func TestEnablePiCreatesTheCatalogAndRestoreKeepsTheFile(t *testing.T) {
	h := newHarness(t, dataPlane)
	models := []integration.ModelRef{{ID: "relo-openai-gpt-4o", Name: "GPT-4o"}}
	withModels(h, &models)
	path := filepath.Join(h.paths.Home, ".pi", "agent", "models.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create the pi directory: %v", err)
	}
	original := "{\"providers\":{\"openai\":{\"api\":\"openai\"}},\"theme\":\"dark\"}\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatalf("seed the catalog: %v", err)
	}

	if _, err := h.manager.Enable(context.Background(), "pi", "rlo_ak_secret"); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	merged := readFile(t, path)
	if strings.Contains(merged, "rlo_ak_secret") || !strings.Contains(merged, "$RELO_PI_API_KEY") {
		t.Fatalf("catalog = %s, want the env reference and not the raw token", merged)
	}
	if !strings.Contains(merged, "openai") || !strings.Contains(merged, "dark") {
		t.Fatalf("catalog = %s, want the operator's providers kept", merged)
	}
	if !strings.Contains(merged, "http://127.0.0.1:10201/v1") {
		t.Fatalf("catalog = %s, want the data plane /v1 address", merged)
	}
	var snapshot string
	for _, file := range h.store.files["pi"] {
		if file.Kind == codingclients.FilePi {
			snapshot = file.SnapshotPath
		}
	}
	if snapshot == "" {
		t.Fatal("snapshot path is empty, want a backup of the original catalog")
	}

	var document map[string]any
	if err := json.Unmarshal([]byte(merged), &document); err != nil {
		t.Fatalf("read the catalog: %v", err)
	}
	document["note"] = "kept"
	edited, err := json.Marshal(document)
	if err != nil {
		t.Fatalf("edit the catalog: %v", err)
	}
	if err := os.WriteFile(path, append(edited, '\n'), 0o644); err != nil {
		t.Fatalf("write the catalog: %v", err)
	}
	if _, err := h.manager.Restore(context.Background(), "pi"); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("catalog after restore: %v, want the file kept", err)
	}
	restored := readFile(t, path)
	if strings.Contains(restored, "\"relo\"") {
		t.Fatalf("restored = %s, want the relo block removed", restored)
	}
	if !strings.Contains(restored, "openai") || !strings.Contains(restored, "kept") {
		t.Fatalf("restored = %s, want the operator's later edit kept", restored)
	}
}

func TestEnablePiCreatesAMissingCatalog(t *testing.T) {
	h := newHarness(t, dataPlane)
	path := filepath.Join(h.paths.Home, ".pi", "agent", "models.json")
	if _, err := h.manager.Enable(context.Background(), "pi", "rlo_ak_secret"); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("catalog: %v, want the file created", err)
	}
	if _, err := h.manager.Disable(context.Background(), "pi"); err != nil {
		t.Fatalf("Disable() error = %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("catalog after disable: %v, want the file kept", err)
	}
	if strings.Contains(readFile(t, path), "\"relo\"") {
		t.Fatal("disable left the relo block in the catalog")
	}
}

func TestEnableAsideRefusesAnAmbiguousAccount(t *testing.T) {
	h := newHarness(t, dataPlane)
	if _, err := h.manager.Enable(context.Background(), "aside", "rlo_ak_secret"); !errors.Is(err, codingclients.ErrAsideAccount) {
		t.Fatalf("Enable() error = %v, want %v", err, codingclients.ErrAsideAccount)
	}
	if err := os.MkdirAll(filepath.Join(h.paths.Home, ".aside", "u", "work"), 0o755); err != nil {
		t.Fatalf("create an account: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(h.paths.Home, ".aside", "u", "play"), 0o755); err != nil {
		t.Fatalf("create a second account: %v", err)
	}
	if _, err := h.manager.Enable(context.Background(), "aside", "rlo_ak_secret"); !errors.Is(err, codingclients.ErrAsideAccount) {
		t.Fatalf("Enable() error = %v, want %v", err, codingclients.ErrAsideAccount)
	}
}

func TestEnableClineWritesBothFiles(t *testing.T) {
	h := newHarness(t, dataPlane)
	models := []integration.ModelRef{{ID: "relo-openai-gpt-4o", Name: "GPT-4o"}}
	withModels(h, &models)
	if _, err := h.manager.Enable(context.Background(), "cline", "rlo_ak_secret"); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	root := filepath.Join(h.paths.Home, ".cline", "data", "settings")
	providers := readFile(t, filepath.Join(root, "providers.json"))
	catalog := readFile(t, filepath.Join(root, "models.json"))
	if !strings.Contains(providers, "openai-responses") || !strings.Contains(providers, "$RELO_CLINE_API_KEY") {
		t.Fatalf("providers = %s, want the settings block", providers)
	}
	if !strings.Contains(catalog, "relo-openai-gpt-4o") || !strings.Contains(catalog, "\"version\":1") {
		t.Fatalf("models = %s, want the catalog entry", catalog)
	}
	applied, message := h.manager.ConfigApplied(context.Background(), "cline")
	if !applied {
		t.Fatalf("ConfigApplied() = %s, want the blocks in place", message)
	}
}
