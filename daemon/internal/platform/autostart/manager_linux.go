//go:build linux

package autostart

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const (
	xdgFileName = "relo-daemon.desktop"
	// trayXDGFileName is the entry a desktop app from before the tray moved
	// into the daemon wrote. It names a command this build no longer carries,
	// so both Enable and Disable remove it.
	trayXDGFileName = "relo-tray.desktop"
)

type platformManager struct{}

// New returns the manager for this platform.
func New() Manager { return platformManager{} }

// Enable writes the autostart entry, which the desktop session reads at the
// next login. The running instance is left alone.
func (m platformManager) Enable(exe string) error {
	if err := m.retireLegacy(); err != nil {
		return err
	}
	dir, err := xdgAutostartDir()
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, xdgFileName), []byte(xdgEntry(exe)), fileMode); err != nil {
		return fmt.Errorf("write the autostart entry: %w", err)
	}
	return nil
}

// Disable removes the autostart entry.
func (m platformManager) Disable() error {
	if err := m.retireLegacy(); err != nil {
		return err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve the home directory: %w", err)
	}
	if err := removeFile(filepath.Join(home, ".config", "autostart", xdgFileName)); err != nil {
		return fmt.Errorf("remove the autostart entry: %w", err)
	}
	return nil
}

// IsEnabled reports whether the autostart entry exists.
func (platformManager) IsEnabled() bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	return fileExists(filepath.Join(home, ".config", "autostart", xdgFileName))
}

// Executable reports the executable the autostart entry starts.
func (platformManager) Executable() (string, bool) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", false
	}
	raw, err := os.ReadFile(filepath.Join(home, ".config", "autostart", xdgFileName))
	if err != nil {
		return "", false
	}
	for _, line := range strings.Split(string(raw), "\n") {
		if command, ok := strings.CutPrefix(line, "Exec="); ok {
			return splitQuotedExecutable(strings.TrimSpace(command))
		}
	}
	return "", false
}

func (platformManager) retireLegacy() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve the home directory: %w", err)
	}
	if err := removeFile(filepath.Join(home, ".config", "autostart", trayXDGFileName)); err != nil {
		return fmt.Errorf("remove the retired tray autostart entry: %w", err)
	}
	return nil
}

func xdgAutostartDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve the home directory: %w", err)
	}
	dir := filepath.Join(home, ".config", "autostart")
	if err := os.MkdirAll(dir, dirMode); err != nil {
		return "", fmt.Errorf("create the autostart directory: %w", err)
	}
	return dir, nil
}

func xdgEntry(exe string) string {
	return `[Desktop Entry]
Type=Application
Name=Relo
Comment=Local AI proxy with multi-account credential pooling
Exec="` + exe + `" ` + daemonGroup + ` ` + daemonRunCommand + `
Terminal=false
X-GNOME-Autostart-enabled=true
`
}

func removeFile(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
