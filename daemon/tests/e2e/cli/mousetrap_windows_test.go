//go:build windows

package e2e_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// TestStartedFromExplorer runs the binary the way the Start Menu shortcut and
// the login item do: as a child of explorer.exe. cobra answers that launch
// with a hint and a five second exit unless the guard is switched off, so a
// successful command proves the guard is off.
func TestStartedFromExplorer(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "relo.exe")
	build := exec.Command("go", "build", "-ldflags=-H windowsgui", "-o", binary, "./cmd/relo")
	build.Dir = moduleRoot(t)
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build the GUI-subsystem binary: %v: %s", err, output)
	}
	explorer := startFakeExplorer(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, binary, "version")
	command.Env = commandEnv(newHome(t))
	command.SysProcAttr = &syscall.SysProcAttr{ParentProcess: explorer}
	stdout := &strings.Builder{}
	command.Stdout = stdout

	if err := command.Run(); err != nil {
		t.Fatalf("version error = %v (stdout %q)", err, stdout.String())
	}
	if !strings.HasPrefix(stdout.String(), "relo ") {
		t.Fatalf("stdout = %q, want the version line", stdout.String())
	}
}

// startFakeExplorer starts a long-lived process named explorer.exe and
// returns a handle that can be given to a child as its parent.
func startFakeExplorer(t *testing.T) syscall.Handle {
	t.Helper()
	system, err := windows.GetSystemDirectory()
	if err != nil {
		t.Fatalf("find the system directory: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(system, "ping.exe"))
	if err != nil {
		t.Fatalf("read ping.exe: %v", err)
	}
	fake := filepath.Join(t.TempDir(), "explorer.exe")
	if err := os.WriteFile(fake, body, 0o755); err != nil {
		t.Fatalf("copy ping.exe: %v", err)
	}
	process := exec.Command(fake, "-n", "60", "127.0.0.1")
	// Explorer has no console; a console here would redirect the child's
	// output away from the pipe used to verify the version command.
	process.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.DETACHED_PROCESS}
	if err := process.Start(); err != nil {
		t.Fatalf("start the stand-in for explorer.exe: %v", err)
	}
	t.Cleanup(func() {
		_ = process.Process.Kill()
		_ = process.Wait()
	})

	handle, err := windows.OpenProcess(windows.PROCESS_CREATE_PROCESS|windows.PROCESS_DUP_HANDLE, false, uint32(process.Process.Pid))
	if err != nil {
		t.Fatalf("open the stand-in for explorer.exe: %v", err)
	}
	t.Cleanup(func() { _ = windows.CloseHandle(handle) })
	return syscall.Handle(handle)
}
