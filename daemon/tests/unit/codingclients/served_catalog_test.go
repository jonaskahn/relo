package codingclients_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/codingclients"
)

func TestDropServedCatalogRemovesOnlyTheCLICatalogs(t *testing.T) {
	dir := t.TempDir()
	cache := filepath.Join(dir, "cache", "model-catalog")
	if err := os.MkdirAll(cache, 0o700); err != nil {
		t.Fatalf("make the cache: %v", err)
	}
	for _, name := range []string{"a-cc.json", "b-cc.json", "a-ccd.json", "note.txt"} {
		if err := os.WriteFile(filepath.Join(cache, name), []byte("{}"), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	if removed := codingclients.DropServedCatalog(dir); removed != 2 {
		t.Fatalf("DropServedCatalog() = %d, want 2", removed)
	}
	for name, want := range map[string]bool{"a-cc.json": false, "b-cc.json": false, "a-ccd.json": true, "note.txt": true} {
		if _, err := os.Stat(filepath.Join(cache, name)); (err == nil) != want {
			t.Fatalf("%s present = %v, want %v", name, err == nil, want)
		}
	}
}

func TestDropServedCatalogWithoutACacheDoesNothing(t *testing.T) {
	if removed := codingclients.DropServedCatalog(t.TempDir()); removed != 0 {
		t.Fatalf("DropServedCatalog() = %d, want 0", removed)
	}
}

func TestPickerIDKeepsAClaudeSpellingAndSpellsTheRest(t *testing.T) {
	if got := codingclients.PickerID(codingclients.ModelRef{ID: "claude-relo-openai--alpha"}); got != "claude-relo-openai--alpha" {
		t.Fatalf("PickerID(claude id) = %q, want it kept", got)
	}
	if got := codingclients.PickerID(codingclients.ModelRef{ID: "gpt-4o"}); got == "gpt-4o" || got == "" {
		t.Fatalf("PickerID(gpt-4o) = %q, want a Claude-shaped id", got)
	}
}
