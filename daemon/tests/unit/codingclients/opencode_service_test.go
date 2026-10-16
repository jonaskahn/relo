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

// stubOpenCode installs a launcher that records its arguments and prints the
// given text, and puts it alone on PATH.
func stubOpenCode(t *testing.T, script string) string {
	t.Helper()
	binDir := t.TempDir()
	marker := filepath.Join(t.TempDir(), "ran")
	body := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> " + marker + "\n" + script
	if err := os.WriteFile(filepath.Join(binDir, "opencode"), []byte(body), 0o755); err != nil {
		t.Fatalf("write the stub launcher: %v", err)
	}
	t.Setenv("PATH", binDir)
	return marker
}

// bareHome keeps the launcher resolver from finding an OpenCode the operator
// installed on this machine.
func bareHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SHELL", "/bin/false")
	t.Setenv("PATH", t.TempDir())
	return home
}

func TestReloadOpenCodeServiceAsksTheServerToReload(t *testing.T) {
	marker := stubOpenCode(t, "")
	if err := codingclients.ReloadOpenCodeService(nil, t.TempDir()); err != nil { //nolint:staticcheck // a nil context is the documented default
		t.Fatalf("ReloadOpenCodeService() error = %v", err)
	}
	got, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("read the stub marker: %v", err)
	}
	if string(got) != "reload\n" {
		t.Fatalf("launcher args = %q, want a reload", got)
	}
}

func TestReloadOpenCodeServiceReportsWhyItFailed(t *testing.T) {
	t.Run("a missing launcher", func(t *testing.T) {
		home := bareHome(t)
		if err := codingclients.ReloadOpenCodeService(context.Background(), home); !errors.Is(err, codingclients.ErrOpenCodeLauncherMissing) {
			t.Fatalf("ReloadOpenCodeService() error = %v, want ErrOpenCodeLauncherMissing", err)
		}
	})
	t.Run("a launcher that refuses", func(t *testing.T) {
		stubOpenCode(t, "echo no server >&2\nexit 1\n")
		err := codingclients.ReloadOpenCodeService(context.Background(), t.TempDir())
		if err == nil || !strings.Contains(err.Error(), "reload the OpenCode service") || !strings.Contains(err.Error(), "no server") {
			t.Fatalf("ReloadOpenCodeService() error = %v, want the launcher's own words", err)
		}
	})
}

func TestOpenCodeServiceKeyReadsTheStoredKey(t *testing.T) {
	marker := stubOpenCode(t, "printf '  stored-key \\n'\n")
	key, err := codingclients.OpenCodeServiceKey(nil, t.TempDir(), "RELO_OPENCODE_API_KEY") //nolint:staticcheck // a nil context is the documented default
	if err != nil {
		t.Fatalf("OpenCodeServiceKey() error = %v", err)
	}
	if key != "stored-key" {
		t.Fatalf("key = %q, want the trimmed value", key)
	}
	got, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("read the stub marker: %v", err)
	}
	if string(got) != "service get env RELO_OPENCODE_API_KEY\n" {
		t.Fatalf("launcher args = %q, want the key read", got)
	}
}

func TestOpenCodeServiceKeyReportsWhyItFailed(t *testing.T) {
	t.Run("a missing launcher", func(t *testing.T) {
		home := bareHome(t)
		if _, err := codingclients.OpenCodeServiceKey(context.Background(), home, "KEY"); !errors.Is(err, codingclients.ErrOpenCodeLauncherMissing) {
			t.Fatalf("OpenCodeServiceKey() error = %v, want ErrOpenCodeLauncherMissing", err)
		}
	})
	t.Run("a launcher that refuses", func(t *testing.T) {
		stubOpenCode(t, "exit 1\n")
		if _, err := codingclients.OpenCodeServiceKey(context.Background(), t.TempDir(), "KEY"); err == nil {
			t.Fatal("OpenCodeServiceKey() error = nil, want the refusal")
		}
	})
}

func TestRestartOpenCodeServiceReportsAStepThatFails(t *testing.T) {
	t.Run("storing the key", func(t *testing.T) {
		stubOpenCode(t, "exit 1\n")
		err := codingclients.RestartOpenCodeService(context.Background(), t.TempDir(), "test-key")
		if err == nil || !strings.Contains(err.Error(), "store the OpenCode service key") {
			t.Fatalf("RestartOpenCodeService() error = %v, want the store step named", err)
		}
	})
	t.Run("the restart", func(t *testing.T) {
		stubOpenCode(t, "[ \"$2\" = restart ] && exit 1\nexit 0\n")
		err := codingclients.RestartOpenCodeService(context.Background(), t.TempDir(), "test-key")
		if err == nil || !strings.Contains(err.Error(), "restart the OpenCode service") {
			t.Fatalf("RestartOpenCodeService() error = %v, want the restart step named", err)
		}
	})
}
