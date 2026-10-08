package integration_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/codingclients"
	"github.com/jonaskahn/relo/internal/application/integration"
)

// TestRotateRewritesTheFilesThatCarryTheKey is what makes a rotation useful:
// the agent reads the new secret on its next launch without a re-setup.
func TestRotateRewritesTheFilesThatCarryTheKey(t *testing.T) {
	h := newHarness(t, dataPlane)
	if _, err := h.manager.Enable(context.Background(), "claude-code", "rlo_ak_first"); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	if _, err := h.manager.Rotate(context.Background(), "claude-code", "rlo_ak_second"); err != nil {
		t.Fatalf("Rotate() error = %v", err)
	}
	keyPath := h.paths.KeyFile("claude-code")
	if got := strings.TrimSpace(readFile(t, keyPath)); got != "rlo_ak_second" {
		t.Fatalf("key file = %q, want the rotated secret", got)
	}
	if runtime.GOOS != "windows" {
		env := readFile(t, filepath.Join(h.paths.Home, ".zshenv"))
		if !strings.Contains(env, "RELO_CLAUDE_CODE_API_KEY") || !strings.Contains(env, keyPath) {
			t.Fatalf("zshenv = %s, want the export still pointing at the key file", env)
		}
		if strings.Contains(env, "rlo_ak_second") {
			t.Fatalf("zshenv carries a secret:\n%s", env)
		}
	}
	if _, err := h.manager.Rotate(context.Background(), "nope", "rlo_ak_second"); !errors.Is(err, integration.ErrUnknownAgent) {
		t.Fatalf("Rotate() on an unknown agent error = %v, want %v", err, integration.ErrUnknownAgent)
	}
}

// TestAgentsListsWhatThisBuildKnows keeps the catalogue and the manager in
// step: the page can only offer what the manager can wire.
func TestAgentsListsWhatThisBuildKnows(t *testing.T) {
	h := newHarness(t, dataPlane)
	agents := h.manager.Agents()
	if len(agents) == 0 {
		t.Fatal("Agents() = none, want the agents this build wires")
	}
	if _, err := h.manager.Inspect(context.Background(), "nope"); !errors.Is(err, integration.ErrUnknownAgent) {
		t.Fatalf("Inspect() on an unknown agent error = %v, want %v", err, integration.ErrUnknownAgent)
	}
	if _, err := h.manager.Enable(context.Background(), "nope", "rlo_ak_test"); !errors.Is(err, integration.ErrUnknownAgent) {
		t.Fatalf("Enable() on an unknown agent error = %v, want %v", err, integration.ErrUnknownAgent)
	}
	if _, err := h.manager.Disable(context.Background(), "nope"); !errors.Is(err, integration.ErrUnknownAgent) {
		t.Fatalf("Disable() on an unknown agent error = %v, want %v", err, integration.ErrUnknownAgent)
	}
	if _, err := h.manager.Restore(context.Background(), "nope"); !errors.Is(err, integration.ErrUnknownAgent) {
		t.Fatalf("Restore() on an unknown agent error = %v, want %v", err, integration.ErrUnknownAgent)
	}
}

// TestRestoreRemovesAFileReloCreated is the other half of the undo: a client
// file Relo created where the operator had none is removed, not emptied.
func TestRestoreRemovesAFileReloCreated(t *testing.T) {
	h := newHarness(t, dataPlane)
	if _, err := h.manager.Enable(context.Background(), "claude-code", "rlo_ak_test"); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	settingsPath := h.paths.ClaudeSettings()
	if _, err := os.Stat(settingsPath); err != nil {
		t.Fatalf("the settings file Relo created is missing: %v", err)
	}
	if _, err := h.manager.Restore(context.Background(), "claude-code"); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}
	if _, err := os.Stat(settingsPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("settings after restore: %v, want the file Relo created to be gone", err)
	}
	if _, err := os.Stat(h.paths.KeyFile("claude-code")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("key file after restore: %v, want it gone", err)
	}
}

// TestPathsHonourTheClientsOwnEnvironment keeps Relo writing where the client
// actually lives.
func TestPathsHonourTheClientsOwnEnvironment(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", filepath.Join(home, "codex-home"))
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(home, "claude-home"))
	paths := codingclients.Paths{Home: home, StateHome: filepath.Join(home, "state")}
	if got := paths.CodexConfig(); got != filepath.Join(home, "codex-home", "config.toml") {
		t.Fatalf("CodexConfig() = %q, want the directory CODEX_HOME names", got)
	}
	if got := paths.ClaudeSettings(); got != filepath.Join(home, "claude-home", "settings.json") {
		t.Fatalf("ClaudeSettings() = %q, want the directory CLAUDE_CONFIG_DIR names", got)
	}
	if got := paths.KeyFile("codex"); got != filepath.Join(home, "state", "integrations", "codex", "key") {
		t.Fatalf("KeyFile() = %q, want the state directory", got)
	}
	if got := paths.BaseURL("openai"); got != "" {
		t.Fatalf("BaseURL() = %q, want an empty address without a data plane", got)
	}
	if got := paths.BaseURL("nope"); got != "" {
		t.Fatalf("BaseURL() = %q, want an empty address for a protocol nothing serves", got)
	}
}
