package codingclients_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/codingclients"
)

func TestGatewayCacheKeepsOnlyClaudeSpellings(t *testing.T) {
	dir := t.TempDir()
	window := int64(1_000_000)
	err := codingclients.WriteGatewayCache(dir, "http://127.0.0.1:10202/", []codingclients.ModelRef{
		{ID: "relo-openai-gpt-4o", Name: "OpenAI | GPT-4o", ContextWindow: &window},
		{ID: "claude-relo-google-antigravity--claude-sonnet-4-6", Name: "Claude Sonnet 4.6"},
		{ID: "reloc-combo", Name: "Relo | Combo"},
	})
	if err != nil {
		t.Fatalf("WriteGatewayCache() error = %v", err)
	}
	info, err := os.Stat(codingclients.GatewayCachePath(dir))
	if err != nil {
		t.Fatalf("stat the cache: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("cache mode = %v, want 0600", info.Mode().Perm())
	}
	content, err := os.ReadFile(codingclients.GatewayCachePath(dir))
	if err != nil {
		t.Fatalf("read the cache: %v", err)
	}
	text := string(content)
	for _, want := range []string{
		`"baseUrl":"http://127.0.0.1:10202"`, "claude-relo-openai-gpt-4o[1m]", "claude-reloc-combo",
		"claude-relo-google-antigravity--claude-sonnet-4-6",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("cache = %s, want %s", text, want)
		}
	}
	if strings.Contains(text, "claude-relo-google-antigravity-claude-sonnet-4-6") {
		t.Fatalf("cache = %s, want the model id left after the separator", text)
	}
	other := filepath.Join(t.TempDir(), "cache")
	if err := os.MkdirAll(other, 0o700); err != nil {
		t.Fatalf("create another cache dir: %v", err)
	}
	foreign := codingclients.GatewayCachePath(filepath.Dir(other))
	if err := os.WriteFile(foreign, []byte(`{"baseUrl":"https://other.example","fetchedAt":1,"models":[]}`), 0o600); err != nil {
		t.Fatalf("write a foreign cache: %v", err)
	}
	if err := codingclients.RemoveGatewayCache(filepath.Dir(other), "http://127.0.0.1:10202"); err != nil {
		t.Fatalf("RemoveGatewayCache() error = %v", err)
	}
	if _, err := os.Stat(foreign); err != nil {
		t.Fatalf("foreign cache was removed: %v", err)
	}
	if err := codingclients.RemoveGatewayCache(dir, "http://127.0.0.1:10202"); err != nil {
		t.Fatalf("RemoveGatewayCache() error = %v", err)
	}
	if _, err := os.Stat(codingclients.GatewayCachePath(dir)); !os.IsNotExist(err) {
		t.Fatalf("Relo cache after remove: %v, want it gone", err)
	}
}
