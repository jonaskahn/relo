package codingclients_test

import (
	"path/filepath"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/codingclients"
)

// TestCodexCatalogDiffCountsTheChange names how a catalog moved: the models
// Relo publishes now that the file lacks, and the ones the file still holds.
func TestCodexCatalogDiffCountsTheChange(t *testing.T) {
	written := []codingclients.ModelRef{{ID: "alpha", Name: "Alpha"}, {ID: "beta", Name: "Beta"}}
	path := codingclients.CodexCatalogPath(filepath.Join(t.TempDir(), "config.toml"))
	if err := codingclients.WriteCodexCatalog(path, written); err != nil {
		t.Fatalf("WriteCodexCatalog() error = %v", err)
	}
	if diff := codingclients.CodexCatalogDiff(path, written); !diff.Empty() {
		t.Fatalf("CodexCatalogDiff() = %+v, want no change", diff)
	}
	grown := append([]codingclients.ModelRef{{ID: "gamma", Name: "Gamma"}}, written...)
	if diff := codingclients.CodexCatalogDiff(path, grown); diff.Added != 1 || diff.Removed != 0 {
		t.Fatalf("CodexCatalogDiff() = %+v, want one added", diff)
	}
	if diff := codingclients.CodexCatalogDiff(path, written[:1]); diff.Added != 0 || diff.Removed != 1 {
		t.Fatalf("CodexCatalogDiff() = %+v, want one removed", diff)
	}
	if diff := codingclients.CodexCatalogDiff(filepath.Join(t.TempDir(), "missing.json"), written); diff.Added != 2 {
		t.Fatalf("CodexCatalogDiff() for a missing file = %+v, want every model added", diff)
	}
}

// TestGatewayCacheDiffCountsTheChange covers the picker cache, whose models
// carry the Claude spellings rather than the published ids.
func TestGatewayCacheDiffCountsTheChange(t *testing.T) {
	const base = "http://127.0.0.1:10202"
	written := []codingclients.ModelRef{{ID: "relo-openai-gpt-4o", Name: "GPT-4o"}}
	dir := t.TempDir()
	if err := codingclients.WriteGatewayCache(dir, base, written); err != nil {
		t.Fatalf("WriteGatewayCache() error = %v", err)
	}
	if diff := codingclients.GatewayCacheDiff(dir, base, written); !diff.Empty() {
		t.Fatalf("GatewayCacheDiff() = %+v, want no change", diff)
	}
	grown := append([]codingclients.ModelRef{{ID: "relo-openai-gpt-5", Name: "GPT-5"}}, written...)
	if diff := codingclients.GatewayCacheDiff(dir, base, grown); diff.Added != 1 || diff.Removed != 0 {
		t.Fatalf("GatewayCacheDiff() = %+v, want one added", diff)
	}
	if diff := codingclients.GatewayCacheDiff(dir, "https://other.example", written); diff.Added != 1 {
		t.Fatalf("GatewayCacheDiff() for another gateway = %+v, want every model added", diff)
	}
}
