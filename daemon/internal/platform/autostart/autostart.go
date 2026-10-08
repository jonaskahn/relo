// Package autostart registers the app to start at login, using the mechanism
// the platform provides: a LaunchAgent on macOS, a Run registry value on
// Windows, and an XDG autostart entry on Linux.
package autostart

import (
	"os"
	"path/filepath"
)

// The names and modes the platform managers share.
const (
	// daemonGroup and its two commands are what the registration starts: the
	// daemon, which carries the tray icon. Windows uses start so the login
	// launcher detaches without a window; macOS and Linux keep the run under
	// the session manager that started it.
	daemonGroup        = "daemon"
	daemonRunCommand   = "run"
	daemonStartCommand = "start"

	dirMode  = 0o755
	fileMode = 0o644
)

// Manager toggles the start-at-login registration.
type Manager interface {
	// Enable registers exe as the app to start at login.
	Enable(exe string) error
	// Disable removes the registration. A missing registration is not an
	// error.
	Disable() error
	// IsEnabled reports whether a registration exists.
	IsEnabled() bool
	// Executable reports the executable the registration starts, so a
	// renamed binary is noticed instead of trusted.
	Executable() (string, bool)
}

// Reconcile registers the login item when it is missing or names another
// executable. A renamed binary heals its own registration on the next start
// instead of rotting at login.
func Reconcile(exe string) error {
	manager := New()
	if current, ok := manager.Executable(); ok && current == exe {
		return nil
	}
	return manager.Enable(exe)
}

// Executable resolves the running binary, following symlinks so a login item
// points at a stable path.
func Executable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return exe, nil
}
