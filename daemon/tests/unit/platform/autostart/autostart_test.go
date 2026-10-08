package autostart_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/platform/autostart"
)

func tempHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	return home
}

// registrationPath is the file one registration lives in, and legacyPath is
// the one a desktop app from before the tray moved into the daemon wrote.
func registrationPath(t *testing.T, home string) (string, string) {
	t.Helper()
	switch runtime.GOOS {
	case "darwin":
		dir := filepath.Join(home, "Library", "LaunchAgents")
		return filepath.Join(dir, "com.relo.daemon.plist"), filepath.Join(dir, "com.relo.tray.plist")
	case "linux":
		dir := filepath.Join(home, ".config", "autostart")
		return filepath.Join(dir, "relo-daemon.desktop"), filepath.Join(dir, "relo-tray.desktop")
	default:
		t.Skipf("start at login is not file-based on %s", runtime.GOOS)
		return "", ""
	}
}

func TestExecutablePointsAtAnAbsolutePath(t *testing.T) {
	exe, err := autostart.Executable()
	if err != nil {
		t.Fatalf("Executable() error = %v", err)
	}
	if !filepath.IsAbs(exe) {
		t.Fatalf("Executable() = %q, want an absolute path", exe)
	}
}

func TestEnable(t *testing.T) {
	t.Run("enable writes the registration that starts the app", func(t *testing.T) {
		home := tempHome(t)
		manager := autostart.New()
		if err := manager.Enable("/opt/relo/relo"); err != nil {
			t.Fatalf("Enable() error = %v", err)
		}
		if !manager.IsEnabled() {
			t.Fatal("IsEnabled() = false after Enable()")
		}
		path, _ := registrationPath(t, home)
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read the registration: %v", err)
		}
		if !strings.Contains(string(body), "/opt/relo/relo") {
			t.Fatalf("the registration does not run the executable: %s", body)
		}
	})

	t.Run("enable retires the registration a previous app wrote", func(t *testing.T) {
		home := tempHome(t)
		_, legacy := registrationPath(t, home)
		if err := os.MkdirAll(filepath.Dir(legacy), 0o755); err != nil {
			t.Fatalf("create the registration directory: %v", err)
		}
		if err := os.WriteFile(legacy, []byte("desktop run --start-hidden"), 0o644); err != nil {
			t.Fatalf("write the retired registration: %v", err)
		}
		if err := autostart.New().Enable("/opt/relo/relo"); err != nil {
			t.Fatalf("Enable() error = %v", err)
		}
		if _, err := os.Stat(legacy); !os.IsNotExist(err) {
			t.Fatalf("the retired registration still exists: %v", err)
		}
	})

	t.Run("a darwin registration is a launch agent", func(t *testing.T) {
		if runtime.GOOS != "darwin" {
			t.Skip("only darwin writes a launch agent")
		}
		home := tempHome(t)
		if err := autostart.New().Enable("/Applications/Relo.app/Contents/MacOS/Relo"); err != nil {
			t.Fatalf("Enable() error = %v", err)
		}
		path, _ := registrationPath(t, home)
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read the launch agent: %v", err)
		}
		for _, want := range []string{"<string>com.relo.daemon</string>", "daemon", "run", "RunAtLoad", "Interactive"} {
			if !strings.Contains(string(body), want) {
				t.Fatalf("the launch agent misses %q: %s", want, body)
			}
		}
		if strings.Contains(string(body), "--start-hidden") {
			t.Fatalf("the launch agent starts a windowless command: %s", body)
		}
	})

	t.Run("an executable with XML characters is escaped", func(t *testing.T) {
		if runtime.GOOS != "darwin" {
			t.Skip("only the launch agent is XML")
		}
		home := tempHome(t)
		if err := autostart.New().Enable("/tmp/a&b/relo"); err != nil {
			t.Fatalf("Enable() error = %v", err)
		}
		path, _ := registrationPath(t, home)
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read the launch agent: %v", err)
		}
		if !strings.Contains(string(body), "/tmp/a&amp;b/relo") {
			t.Fatalf("the executable was not escaped: %s", body)
		}
	})

	t.Run("a linux registration is a desktop entry", func(t *testing.T) {
		if runtime.GOOS != "linux" {
			t.Skip("only linux writes a desktop entry")
		}
		home := tempHome(t)
		if err := autostart.New().Enable("/usr/bin/relo"); err != nil {
			t.Fatalf("Enable() error = %v", err)
		}
		path, _ := registrationPath(t, home)
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read the desktop entry: %v", err)
		}
		for _, want := range []string{"[Desktop Entry]", `Exec="/usr/bin/relo" daemon run`, "X-GNOME-Autostart-enabled=true"} {
			if !strings.Contains(string(body), want) {
				t.Fatalf("the desktop entry misses %q: %s", want, body)
			}
		}
	})
}

func TestReconcile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the registry is not redirected in tests")
	}
	t.Run("reconcile is a no-op when the same executable is registered", func(t *testing.T) {
		home := tempHome(t)
		if err := autostart.New().Enable("/opt/relo/relo"); err != nil {
			t.Fatalf("Enable() error = %v", err)
		}
		if err := autostart.Reconcile("/opt/relo/relo"); err != nil {
			t.Fatalf("Reconcile() error = %v", err)
		}
		path, _ := registrationPath(t, home)
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read the registration: %v", err)
		}
		if !strings.Contains(string(body), "/opt/relo/relo") {
			t.Fatalf("Reconcile rewrote the registration: %s", body)
		}
	})

	t.Run("reconcile heals a registration naming another executable", func(t *testing.T) {
		home := tempHome(t)
		if err := autostart.New().Enable("/opt/relo/relo"); err != nil {
			t.Fatalf("Enable() error = %v", err)
		}
		if err := autostart.Reconcile("/opt/other/relo"); err != nil {
			t.Fatalf("Reconcile() error = %v", err)
		}
		path, _ := registrationPath(t, home)
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read the registration: %v", err)
		}
		if !strings.Contains(string(body), "/opt/other/relo") {
			t.Fatalf("Reconcile kept the stale registration: %s", body)
		}
		if exe, ok := autostart.New().Executable(); !ok || exe != "/opt/other/relo" {
			t.Fatalf("Executable() = %q, %v, want %q, true", exe, ok, "/opt/other/relo")
		}
	})
}

func TestDisable(t *testing.T) {
	t.Run("disable removes the registration", func(t *testing.T) {
		home := tempHome(t)
		manager := autostart.New()
		if err := manager.Enable("/opt/relo/relo"); err != nil {
			t.Fatalf("Enable() error = %v", err)
		}
		if err := manager.Disable(); err != nil {
			t.Fatalf("Disable() error = %v", err)
		}
		if manager.IsEnabled() {
			t.Fatal("IsEnabled() = true after Disable()")
		}
		path, _ := registrationPath(t, home)
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("the registration file still exists: %v", err)
		}
	})

	t.Run("disable retires the registration a previous app wrote", func(t *testing.T) {
		home := tempHome(t)
		_, legacy := registrationPath(t, home)
		if err := os.MkdirAll(filepath.Dir(legacy), 0o755); err != nil {
			t.Fatalf("create the registration directory: %v", err)
		}
		if err := os.WriteFile(legacy, []byte("desktop run --start-hidden"), 0o644); err != nil {
			t.Fatalf("write the retired registration: %v", err)
		}
		if err := autostart.New().Disable(); err != nil {
			t.Fatalf("Disable() error = %v", err)
		}
		if _, err := os.Stat(legacy); !os.IsNotExist(err) {
			t.Fatalf("the retired registration still exists: %v", err)
		}
	})

	t.Run("disable without a registration is not an error", func(t *testing.T) {
		tempHome(t)
		if err := autostart.New().Disable(); err != nil {
			t.Fatalf("Disable() error = %v", err)
		}
		if autostart.New().IsEnabled() {
			t.Fatal("IsEnabled() = true with nothing registered")
		}
	})
}

func TestFailures(t *testing.T) {
	fileBased := runtime.GOOS == "darwin" || runtime.GOOS == "linux"

	t.Run("enable reports a blocked location", func(t *testing.T) {
		if !fileBased {
			t.Skipf("start at login is not file-based on %s", runtime.GOOS)
		}
		home := tempHome(t)
		path, _ := registrationPath(t, home)
		blocker := filepath.Dir(filepath.Dir(path))
		if err := os.WriteFile(blocker, []byte("in the way"), 0o644); err != nil {
			t.Fatalf("write the blocker: %v", err)
		}
		if err := autostart.New().Enable("/opt/relo/relo"); err == nil {
			t.Fatal("Enable() error = nil, want a failure")
		}
	})

	t.Run("disable reports a registration it cannot remove", func(t *testing.T) {
		if !fileBased {
			t.Skipf("start at login is not file-based on %s", runtime.GOOS)
		}
		home := tempHome(t)
		path, _ := registrationPath(t, home)
		if err := os.MkdirAll(filepath.Join(path, "nested"), 0o755); err != nil {
			t.Fatalf("build the unremovable registration: %v", err)
		}
		if err := autostart.New().Disable(); err == nil {
			t.Fatal("Disable() error = nil, want a failure")
		}
	})

	t.Run("an unset home is reported", func(t *testing.T) {
		if !fileBased {
			t.Skipf("the home directory is not consulted on %s", runtime.GOOS)
		}
		t.Setenv("HOME", "")
		if autostart.New().IsEnabled() {
			t.Fatal("IsEnabled() = true without a home directory")
		}
		if err := autostart.New().Enable("/opt/relo/relo"); err == nil {
			t.Fatal("Enable() error = nil, want a failure")
		}
		if err := autostart.New().Disable(); err == nil {
			t.Fatal("Disable() error = nil, want a failure")
		}
	})
}
