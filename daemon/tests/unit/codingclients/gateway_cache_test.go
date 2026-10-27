package codingclients_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func TestGatewayCacheListsOnlyWhatReloWouldWrite(t *testing.T) {
	const base = "http://127.0.0.1:10202"
	models := []codingclients.ModelRef{
		{ID: "relo-openai-gpt-4o", Name: "OpenAI | GPT-4o"},
		{ID: "  "},
	}
	dir := t.TempDir()
	if codingclients.GatewayCacheLists(dir, base, models) {
		t.Fatal("GatewayCacheLists() = true for a missing cache")
	}
	if err := codingclients.WriteGatewayCache(dir, base+"/", models); err != nil {
		t.Fatalf("WriteGatewayCache() error = %v", err)
	}
	if !codingclients.GatewayCacheLists(dir, base, models) {
		t.Fatal("GatewayCacheLists() = false for the cache just written")
	}
	for name, change := range map[string]struct {
		base   string
		models []codingclients.ModelRef
	}{
		"another gateway":    {"https://other.example", models},
		"an extra model":     {base, append([]codingclients.ModelRef{{ID: "relo-openai-gpt-5", Name: "GPT-5"}}, models...)},
		"a renamed model":    {base, []codingclients.ModelRef{{ID: "relo-openai-gpt-4o", Name: "Renamed"}}},
		"a swapped model id": {base, []codingclients.ModelRef{{ID: "relo-openai-gpt-5", Name: "OpenAI | GPT-4o"}}},
	} {
		if codingclients.GatewayCacheLists(dir, change.base, change.models) {
			t.Fatalf("GatewayCacheLists() = true for %s", name)
		}
	}
	if err := os.WriteFile(codingclients.GatewayCachePath(dir), []byte("not json"), 0o600); err != nil {
		t.Fatalf("corrupt the cache: %v", err)
	}
	if codingclients.GatewayCacheLists(dir, base, models) {
		t.Fatal("GatewayCacheLists() = true for an unreadable cache")
	}
}

func TestWriteGatewayCacheLeavesAnUpToDateCacheAlone(t *testing.T) {
	const base = "http://127.0.0.1:10202"
	models := []codingclients.ModelRef{{ID: "relo-openai-gpt-4o", Name: "GPT-4o"}}
	dir := t.TempDir()
	if err := codingclients.WriteGatewayCache(dir, base, models); err != nil {
		t.Fatalf("WriteGatewayCache() error = %v", err)
	}
	path := codingclients.GatewayCachePath(dir)
	past := time.Now().Add(-time.Hour)
	if err := os.Chtimes(path, past, past); err != nil {
		t.Fatalf("age the cache: %v", err)
	}
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read the cache: %v", err)
	}
	// A second write would stamp a new fetch time, so equal bytes mean no write.
	time.Sleep(5 * time.Millisecond)
	if err := codingclients.WriteGatewayCache(dir, base, models); err != nil {
		t.Fatalf("WriteGatewayCache() error = %v", err)
	}
	second, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read the cache again: %v", err)
	}
	if string(first) != string(second) {
		t.Fatalf("cache rewritten: %s then %s", first, second)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat the cache: %v", err)
	}
	if info.ModTime().After(past.Add(time.Second)) {
		t.Fatal("an unchanged gateway cache was rewritten")
	}
	if err := codingclients.WriteGatewayCache(t.TempDir(), "  ", models); err != nil {
		t.Fatalf("WriteGatewayCache(blank base) error = %v", err)
	}
}
