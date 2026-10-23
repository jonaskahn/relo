//go:build !windows

package cli

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestDetachAttributesSetsid(t *testing.T) {
	attr := detachAttributes()
	if attr == nil || !attr.Setsid {
		t.Fatalf("detachAttributes() = %+v, want Setsid", attr)
	}
}

func TestFindSetupUninstallerNone(t *testing.T) {
	if path, ok := findSetupUninstaller(); ok || path != "" {
		t.Fatalf("findSetupUninstaller() = %q, %v, want \"\", false", path, ok)
	}
}

func TestRemoveCommand(t *testing.T) {
	r := Remover{}
	got := r.removeCommand("/tmp/relo")
	if got != "rm -f '/tmp/relo'" {
		t.Fatalf("removeCommand = %q, want %q", got, "rm -f '/tmp/relo'")
	}
}

func TestRemoveRunningProgramMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nope")
	if err := removeRunningProgram(path); err != nil {
		t.Fatalf("removeRunningProgram(missing) = %v, want nil", err)
	}
}

func TestRemoveRunningProgramRemoves(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bin")
	if err := os.WriteFile(path, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := removeRunningProgram(path); err != nil {
		t.Fatalf("removeRunningProgram = %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("file still exists: %v", err)
	}
}

func TestRemoveFromUserPathNoop(t *testing.T) {
	if err := removeFromUserPath("/tmp"); err != nil {
		t.Fatalf("removeFromUserPath = %v, want nil", err)
	}
}

func TestShellQuote(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"/tmp/a", "'/tmp/a'"},
		{"/tmp/it's", "'/tmp/it'\\''s'"},
	}
	for _, c := range cases {
		if got := shellQuote(c.in); got != c.want {
			t.Errorf("shellQuote(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

var _ = syscall.SysProcAttr{}
