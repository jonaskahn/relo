package integration_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/codingclients"
	"github.com/jonaskahn/relo/internal/application/integration"
)

// A DeepSeek Harness integration wired before the file moved keeps a record
// of the settings file it wrote then. Enabling it again retires that record
// and takes Relo's block out of the file nothing reads any more, while the
// Cordis patch the harness actually reads is written in its place.
func TestEnableRetiresTheMovedDSHFile(t *testing.T) {
	h := newHarness(t, func(string) string { return "http://127.0.0.1:10201" })
	models := []integration.ModelRef{{ID: "relo-openai-gpt-4o", Name: "GPT-4o"}}
	withModels(h, &models)
	home := h.paths.Home
	t.Setenv("DSH_HOME", filepath.Join(home, ".dsh"))

	stale := filepath.Join(home, ".dsh", "settings.yaml")
	staleBlock := "providers:\n  relo:\n    baseUrl: http://127.0.0.1:10201\n"
	if err := os.MkdirAll(filepath.Dir(stale), 0o755); err != nil {
		t.Fatalf("create the old settings directory: %v", err)
	}
	if err := os.WriteFile(stale, []byte(staleBlock), 0o644); err != nil {
		t.Fatalf("write the old settings file: %v", err)
	}
	h.store.files["dsh"] = []integration.FileRecord{{
		IntegrationID: "dsh", Kind: codingclients.FileDSH, Path: stale,
	}}

	if _, err := h.manager.Enable(t.Context(), "dsh", "rlo_ak_test"); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}

	patch := filepath.Join(home, ".dsh", "profiles", "web", "cordis.patch.yml")
	written, err := os.ReadFile(patch)
	if err != nil {
		t.Fatalf("read the Cordis patch: %v", err)
	}
	if !strings.Contains(string(written), "apiKeyEnv: RELO_DSH_API_KEY") {
		t.Fatalf("patch = %s, want the env helper", written)
	}

	headlessPatch := filepath.Join(home, ".dsh", "profiles", "headless", "cordis.patch.yml")
	headless, err := os.ReadFile(headlessPatch)
	if err != nil {
		t.Fatalf("read the headless Cordis patch: %v", err)
	}
	for _, want := range []string{"apiKeyEnv: RELO_DSH_API_KEY", "- id: agent-default-model", "provider: relo"} {
		if !strings.Contains(string(headless), want) {
			t.Fatalf("headless patch = %s, want %q", headless, want)
		}
	}
	if strings.Contains(string(written), "agent-default-model") {
		t.Fatalf("web patch = %s, want no default model: the browser picks its own", written)
	}

	recorded := map[string]string{}
	for _, file := range h.store.files["dsh"] {
		recorded[file.Kind] = file.Path
	}
	for kind, want := range map[string]string{
		codingclients.FileDSH:         filepath.Join(home, ".dsh", "profiles", "web", "cordis.patch.yml"),
		codingclients.FileDSHHeadless: filepath.Join(home, ".dsh", "profiles", "headless", "cordis.patch.yml"),
	} {
		if got, found := recorded[kind]; !found || got != want {
			t.Fatalf("file rows = %+v, want kind %q at %q", h.store.files["dsh"], kind, want)
		}
	}

	restored, err := os.ReadFile(stale)
	if err != nil {
		t.Fatalf("read the old settings file: %v", err)
	}
	if strings.Contains(string(restored), "relo") {
		t.Fatalf("old settings file still names Relo:\n%s", restored)
	}

	for _, file := range h.store.files["dsh"] {
		if file.Path == stale {
			t.Fatalf("the moved file is still recorded: %+v", file)
		}
	}
}
