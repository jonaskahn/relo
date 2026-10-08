package cli_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestAutostartCommand(t *testing.T) {
	t.Run("autostart help lists both actions", func(t *testing.T) {
		stdout, _, err := run(t, nil, "autostart", "--help")
		if err != nil {
			t.Fatalf("autostart --help error = %v", err)
		}
		for _, name := range []string{"enable", "disable"} {
			if !strings.Contains(stdout, name) {
				t.Fatalf("help = %q, want %s listed", stdout, name)
			}
		}
	})

	t.Run("autostart enable writes the login item and disable removes it", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("the registry is not redirected in tests")
		}
		home := t.TempDir()
		t.Setenv("HOME", home)
		registrationPath, _ := autostartRegistrationPaths(t, home)

		stdout, _, err := run(t, nil, "autostart", "enable")
		if err != nil {
			t.Fatalf("autostart enable error = %v", err)
		}
		if !strings.Contains(stdout, "will start at login") {
			t.Fatalf("stdout = %q, want the confirmation", stdout)
		}
		registration, err := os.ReadFile(registrationPath)
		if err != nil {
			t.Fatalf("read the registration: %v", err)
		}
		if !strings.Contains(string(registration), "daemon") {
			t.Fatalf("the registration does not run the app: %s", registration)
		}

		stdout, _, err = run(t, nil, "autostart", "disable")
		if err != nil {
			t.Fatalf("autostart disable error = %v", err)
		}
		if !strings.Contains(stdout, "no longer start at login") {
			t.Fatalf("stdout = %q, want the confirmation", stdout)
		}
		if _, err := os.Stat(registrationPath); !os.IsNotExist(err) {
			t.Fatalf("the registration still exists: %v", err)
		}
	})

	t.Run("a blocked home cannot register the login items", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("the registry is not redirected in tests")
		}
		home := t.TempDir()
		t.Setenv("HOME", home)
		registrationPath, _ := autostartRegistrationPaths(t, home)
		blocker := filepath.Dir(filepath.Dir(registrationPath))
		if err := os.WriteFile(blocker, []byte("in the way"), 0o644); err != nil {
			t.Fatalf("write the blocker: %v", err)
		}
		if _, _, err := run(t, nil, "autostart", "enable"); err == nil {
			t.Fatal("error = nil, want the registration failure")
		}
	})

	t.Run("an unremovable login item is reported", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("the registry is not redirected in tests")
		}
		home := t.TempDir()
		t.Setenv("HOME", home)
		registrationPath, _ := autostartRegistrationPaths(t, home)
		if err := os.MkdirAll(filepath.Join(registrationPath, "nested"), 0o755); err != nil {
			t.Fatalf("build the unremovable registration: %v", err)
		}
		if _, _, err := run(t, nil, "autostart", "disable"); err == nil {
			t.Fatal("error = nil, want the removal failure")
		}
	})
}

func autostartRegistrationPaths(t *testing.T, home string) (string, string) {
	t.Helper()
	if runtime.GOOS == "linux" {
		dir := filepath.Join(home, ".config", "autostart")
		return filepath.Join(dir, "relo-daemon.desktop"), filepath.Join(dir, "relo-tray.desktop")
	}
	dir := filepath.Join(home, "Library", "LaunchAgents")
	return filepath.Join(dir, "com.relo.daemon.plist"), filepath.Join(dir, "com.relo.tray.plist")
}
