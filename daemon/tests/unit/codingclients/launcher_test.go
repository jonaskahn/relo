package codingclients_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/codingclients"
)

const oldShim = "#!/bin/sh\n# relo codex launcher shim\nexec codex.relo-backup\n"

func writeLauncherState(t *testing.T, dir string, record codingclients.LauncherRecord) string {
	t.Helper()
	encoded, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("encode the launcher state: %v", err)
	}
	path := filepath.Join(dir, "launcher.json")
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		t.Fatalf("write the launcher state: %v", err)
	}
	return path
}

// TestRemoveLauncherBringsBackAVanishedClient is the case that used to delete
// the operator's binary: the launcher path is empty and only the moved copy
// is left, so that copy has to go back rather than be discarded.
func TestRemoveLauncherBringsBackAVanishedClient(t *testing.T) {
	dir := t.TempDir()
	launcher := filepath.Join(dir, "codex")
	backup := launcher + ".relo-backup"
	if err := os.WriteFile(backup, []byte("real"), 0o755); err != nil {
		t.Fatalf("write the moved client: %v", err)
	}
	state := writeLauncherState(t, dir, codingclients.LauncherRecord{Path: launcher, RealPath: backup})

	removed, err := codingclients.RemoveLauncher(state)
	if err != nil || !removed {
		t.Fatalf("RemoveLauncher() = %v, %v, want removed", removed, err)
	}
	if got, _ := os.ReadFile(launcher); string(got) != "real" {
		t.Fatalf("launcher = %q, want the moved client back", got)
	}
}

// TestRemoveLauncherLeavesAReinstalledClient keeps a client the operator
// reinstalled over the shim, and drops only the stale copy.
func TestRemoveLauncherLeavesAReinstalledClient(t *testing.T) {
	dir := t.TempDir()
	launcher := filepath.Join(dir, "codex")
	backup := launcher + ".relo-backup"
	if err := os.WriteFile(launcher, []byte("new"), 0o755); err != nil {
		t.Fatalf("write the reinstalled client: %v", err)
	}
	if err := os.WriteFile(backup, []byte("old"), 0o755); err != nil {
		t.Fatalf("write the stale copy: %v", err)
	}
	state := writeLauncherState(t, dir, codingclients.LauncherRecord{Path: launcher, RealPath: backup})

	if _, err := codingclients.RemoveLauncher(state); err != nil {
		t.Fatalf("RemoveLauncher() error = %v", err)
	}
	if got, _ := os.ReadFile(launcher); string(got) != "new" {
		t.Fatalf("launcher = %q, want the reinstalled client kept", got)
	}
	if _, err := os.Stat(backup); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the stale copy survived: %v", err)
	}
}

// TestRetireLauncherFindsAShimOnPathWithoutState covers the machine whose
// state file is gone: the shim on PATH is recognised by its marker.
func TestRetireLauncherFindsAShimOnPathWithoutState(t *testing.T) {
	dir := t.TempDir()
	launcher := filepath.Join(dir, "codex")
	if err := os.WriteFile(launcher, []byte(oldShim), 0o755); err != nil {
		t.Fatalf("write the shim: %v", err)
	}
	if err := os.WriteFile(launcher+".relo-backup", []byte("real"), 0o755); err != nil {
		t.Fatalf("write the moved client: %v", err)
	}
	t.Setenv("PATH", dir)

	if err := codingclients.RetireLauncher(filepath.Join(dir, "missing.json")); err != nil {
		t.Fatalf("RetireLauncher() error = %v", err)
	}
	if got, _ := os.ReadFile(launcher); string(got) != "real" {
		t.Fatalf("launcher = %q, want the moved client back", got)
	}
}

// TestRetireLauncherLeavesAnotherToolsWrapperAlone never touches a launcher
// Relo did not write.
func TestRetireLauncherLeavesAnotherToolsWrapperAlone(t *testing.T) {
	dir := t.TempDir()
	launcher := filepath.Join(dir, "codex")
	foreign := "#!/bin/sh\n# opencodex shim\nexec codex.opencodex-real\n"
	if err := os.WriteFile(launcher, []byte(foreign), 0o755); err != nil {
		t.Fatalf("write the foreign shim: %v", err)
	}
	t.Setenv("PATH", dir)

	if err := codingclients.RetireLauncher(filepath.Join(dir, "missing.json")); err != nil {
		t.Fatalf("RetireLauncher() error = %v", err)
	}
	if got, _ := os.ReadFile(launcher); string(got) != foreign {
		t.Fatalf("launcher = %q, want it untouched", got)
	}
}
