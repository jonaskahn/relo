package codingclients_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/codingclients"
)

func TestRestartOpenCodeServiceStoresTheKeyThenRestarts(t *testing.T) {
	binDir := t.TempDir()
	marker := filepath.Join(t.TempDir(), "ran")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> " + marker + "\n"
	if err := os.WriteFile(filepath.Join(binDir, "opencode"), []byte(script), 0o755); err != nil {
		t.Fatalf("write the stub launcher: %v", err)
	}
	t.Setenv("PATH", binDir)
	if err := codingclients.RestartOpenCodeService(context.Background(), t.TempDir(), "test-key\n"); err != nil {
		t.Fatalf("RestartOpenCodeService() error = %v", err)
	}
	got, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("read the stub marker: %v", err)
	}
	want := "service set env RELO_OPENCODE_API_KEY test-key\nservice restart\n"
	if string(got) != want {
		t.Fatalf("launcher args = %q, want %q", got, want)
	}
}

func TestRestartOpenCodeServiceRunsANodeShimWithNodeFromAnotherBin(t *testing.T) {
	root := t.TempDir()
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
	if err := os.WriteFile(filepath.Join(shimDir, "opencode"), []byte("#!/usr/bin/env node\n"), 0o755); err != nil {
		t.Fatalf("write the node shim: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "ran")
	node := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> " + marker + "\n"
	if err := os.WriteFile(filepath.Join(nodeDir, "node"), []byte(node), 0o755); err != nil {
		t.Fatalf("write the node stub: %v", err)
	}
	if err := codingclients.RestartOpenCodeService(context.Background(), root, "test-key\n"); err != nil {
		t.Fatalf("RestartOpenCodeService() error = %v", err)
	}
	got, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("read the node marker: %v", err)
	}
	text := string(got)
	if !strings.Contains(text, "service set env RELO_OPENCODE_API_KEY test-key") || !strings.Contains(text, "service restart") {
		t.Fatalf("node shim args = %q, want the service set and restart", got)
	}
}

func TestRestartOpenCodeServiceRefusesAnEmptyKey(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	t.Setenv("SHELL", "/bin/false")
	err := codingclients.RestartOpenCodeService(context.Background(), t.TempDir(), "  ")
	if !errors.Is(err, codingclients.ErrOpenCodeKeyMissing) {
		t.Fatalf("RestartOpenCodeService() error = %v, want ErrOpenCodeKeyMissing", err)
	}
}

func TestRestartOpenCodeServiceReportsAMissingLauncher(t *testing.T) {
	// A home and shell this machine keeps nothing in, so the resolver cannot
	// reach the real launcher the operator has installed.
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SHELL", "/bin/false")
	t.Setenv("PATH", t.TempDir())
	if err := codingclients.RestartOpenCodeService(context.Background(), home, "test-key"); !errors.Is(err, codingclients.ErrOpenCodeLauncherMissing) {
		t.Fatalf("RestartOpenCodeService() error = %v, want ErrOpenCodeLauncherMissing", err)
	}
}
