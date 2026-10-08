package codingclients_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/adapters/codingclients"
)

func TestCodexDaemonStartedAtIgnoresAMissingOrGarbageFile(t *testing.T) {
	home := t.TempDir()
	if _, found := codingclients.CodexDaemonStartedAt(home); found {
		t.Fatal("a missing pid file reported a daemon")
	}
	writeDaemonPid(t, home, 1, 1, `{not json`)
	if _, found := codingclients.CodexDaemonStartedAt(home); found {
		t.Fatal("a garbage pid file reported a daemon")
	}
}

func TestCodexNeedsRestartComparesTheDaemonToTheCatalog(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a Windows Signal(0) cannot prove a pid is alive")
	}
	home := t.TempDir()
	catalog := filepath.Join(home, "relo-model-catalog.json")
	if err := os.WriteFile(catalog, []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("write the catalog: %v", err)
	}
	if codingclients.CodexNeedsRestart(home, catalog) {
		t.Fatal("no daemon still needed a restart")
	}
	writeDaemonPid(t, home, os.Getpid(), time.Now().Add(-time.Hour).Unix(), "")
	if !codingclients.CodexNeedsRestart(home, catalog) {
		t.Fatal("a daemon older than the catalog did not need a restart")
	}
	writeDaemonPid(t, home, os.Getpid(), time.Now().Add(time.Hour).Unix(), "")
	if codingclients.CodexNeedsRestart(home, catalog) {
		t.Fatal("a daemon newer than the catalog still needed a restart")
	}
	if codingclients.CodexNeedsRestart(home, filepath.Join(home, "missing.json")) {
		t.Fatal("a missing catalog needed a restart")
	}
}

func TestRestartCodexDaemonDoesNothingWithoutADaemon(t *testing.T) {
	if err := codingclients.RestartCodexDaemon(context.Background(), t.TempDir(), t.TempDir()); err != nil {
		t.Fatalf("RestartCodexDaemon() error = %v", err)
	}
}

func TestRestartCodexDaemonRunsTheLauncher(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a Windows Signal(0) cannot prove a pid is alive")
	}
	home := t.TempDir()
	writeDaemonPid(t, home, os.Getpid(), time.Now().Unix(), "")
	binDir := t.TempDir()
	marker := filepath.Join(t.TempDir(), "ran")
	script := "#!/bin/sh\nprintf '%s' \"$1 $2 $3\" > " + marker + "\n"
	if err := os.WriteFile(filepath.Join(binDir, "codex"), []byte(script), 0o755); err != nil {
		t.Fatalf("write the stub launcher: %v", err)
	}
	t.Setenv("PATH", binDir)
	if err := codingclients.RestartCodexDaemon(context.Background(), home, t.TempDir()); err != nil {
		t.Fatalf("RestartCodexDaemon() error = %v", err)
	}
	got, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("read the stub marker: %v", err)
	}
	if string(got) != "app-server daemon restart" {
		t.Fatalf("launcher args = %q, want app-server daemon restart", got)
	}
}

func TestRestartCodexDaemonUsesTheBundledLauncher(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a Windows Signal(0) cannot prove a pid is alive")
	}
	home := t.TempDir()
	writeDaemonPid(t, home, os.Getpid(), time.Now().Unix(), "")
	t.Setenv("PATH", t.TempDir())
	t.Setenv("SHELL", "/bin/false")
	marker := filepath.Join(t.TempDir(), "ran")
	bin := filepath.Join(home, "packages", "app-server-daemon", "releases", "0.1.0", "bin")
	if err := os.MkdirAll(bin, 0o700); err != nil {
		t.Fatalf("create the bundled launcher directory: %v", err)
	}
	script := "#!/bin/sh\nprintf restarted > " + marker + "\n"
	if err := os.WriteFile(filepath.Join(bin, "codex"), []byte(script), 0o755); err != nil {
		t.Fatalf("write the bundled launcher: %v", err)
	}
	if err := codingclients.RestartCodexDaemon(context.Background(), home, t.TempDir()); err != nil {
		t.Fatalf("RestartCodexDaemon() error = %v", err)
	}
	got, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("read the bundled marker: %v", err)
	}
	if string(got) != "restarted" {
		t.Fatalf("bundled launcher = %q, want restarted", got)
	}
}

func TestRestartCodexDaemonRunsANodeShimWithItsOwnNode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a Windows Signal(0) cannot prove a pid is alive")
	}
	root := t.TempDir()
	writeDaemonPid(t, root, os.Getpid(), time.Now().Unix(), "")
	t.Setenv("PATH", t.TempDir())
	t.Setenv("SHELL", "/bin/false")
	bin := filepath.Join(root, ".nvm", "versions", "node", "v24.0.0", "bin")
	if err := os.MkdirAll(bin, 0o700); err != nil {
		t.Fatalf("create the nvm bin: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "ran")
	node := "#!/bin/sh\nprintf '%s' \"$*\" > " + marker + "\n"
	if err := os.WriteFile(filepath.Join(bin, "node"), []byte(node), 0o755); err != nil {
		t.Fatalf("write the node stub: %v", err)
	}
	if err := os.WriteFile(filepath.Join(bin, "codex"), []byte("#!/usr/bin/env node\n"), 0o755); err != nil {
		t.Fatalf("write the node shim: %v", err)
	}
	if err := codingclients.RestartCodexDaemon(context.Background(), root, root); err != nil {
		t.Fatalf("RestartCodexDaemon() error = %v", err)
	}
	got, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("read the node marker: %v", err)
	}
	if !strings.Contains(string(got), "app-server daemon restart") {
		t.Fatalf("node shim args = %q, want app-server daemon restart", got)
	}
}

func TestRestartCodexDaemonRunsANodeShimWithNodeFromAnotherBin(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a Windows Signal(0) cannot prove a pid is alive")
	}
	root := t.TempDir()
	writeDaemonPid(t, root, os.Getpid(), time.Now().Unix(), "")
	t.Setenv("PATH", t.TempDir())
	t.Setenv("SHELL", "/bin/false")
	shimDir := filepath.Join(root, ".local", "bin")
	nodeDir := filepath.Join(root, ".nvm", "versions", "node", "v24.0.0", "bin")
	if err := os.MkdirAll(shimDir, 0o700); err != nil {
		t.Fatalf("create the shim directory: %v", err)
	}
	if err := os.MkdirAll(nodeDir, 0o700); err != nil {
		t.Fatalf("create the nvm bin: %v", err)
	}
	if err := os.WriteFile(filepath.Join(shimDir, "codex"), []byte("#!/usr/bin/env node\n"), 0o755); err != nil {
		t.Fatalf("write the node shim: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "ran")
	node := "#!/bin/sh\nprintf '%s' \"$*\" > " + marker + "\n"
	if err := os.WriteFile(filepath.Join(nodeDir, "node"), []byte(node), 0o755); err != nil {
		t.Fatalf("write the node stub: %v", err)
	}
	if err := codingclients.RestartCodexDaemon(context.Background(), root, root); err != nil {
		t.Fatalf("RestartCodexDaemon() error = %v", err)
	}
	got, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("read the node marker: %v", err)
	}
	if !strings.Contains(string(got), "app-server daemon restart") {
		t.Fatalf("node shim args = %q, want app-server daemon restart", got)
	}
}

func TestRestartCodexDaemonReportsAMissingLauncher(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a Windows Signal(0) cannot prove a pid is alive")
	}
	// A home and shell this machine keeps nothing in, so the resolver cannot
	// reach the real launcher the operator has installed.
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SHELL", "/bin/false")
	writeDaemonPid(t, home, os.Getpid(), time.Now().Unix(), "")
	t.Setenv("PATH", t.TempDir())
	if err := codingclients.RestartCodexDaemon(context.Background(), home, home); err == nil {
		t.Fatal("RestartCodexDaemon() error = nil, want a missing launcher")
	}
}

func writeDaemonPid(t *testing.T, home string, pid int, startSeconds int64, raw string) {
	t.Helper()
	dir := filepath.Join(home, "app-server-daemon")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("create the daemon directory: %v", err)
	}
	content := raw
	if content == "" {
		encoded, err := json.Marshal(map[string]any{
			"pid": pid, "processIdentity": map[string]any{"startSeconds": startSeconds},
		})
		if err != nil {
			t.Fatalf("encode the pid file: %v", err)
		}
		content = string(encoded)
	}
	if err := os.WriteFile(filepath.Join(dir, "daemon.pid"), []byte(content), 0o600); err != nil {
		t.Fatalf("write the pid file: %v", err)
	}
}
