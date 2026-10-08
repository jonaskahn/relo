//go:build windows

package autostart

import (
	"errors"
	"fmt"
	"syscall"

	"golang.org/x/sys/windows/registry"
)

const (
	runKeyPath = `Software\Microsoft\Windows\CurrentVersion\Run`
	// valueName is the Run value this build writes. The retired desktop app
	// wrote trayValueName, which named the desktop command this build no
	// longer carries, so both Enable and Disable remove it.
	valueName     = "ReloDaemon"
	trayValueName = "Relo"
)

type platformManager struct{}

// New returns the manager for this platform.
func New() Manager { return platformManager{} }

// Enable writes the Run value, which Windows reads at the next login. The
// running instance is left alone.
func (m platformManager) Enable(exe string) error {
	if err := m.retireLegacy(); err != nil {
		return err
	}
	key, _, err := registry.CreateKey(registry.CURRENT_USER, runKeyPath, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("open the Run registry key: %w", err)
	}
	defer func() { _ = key.Close() }()
	// %q would double the backslashes of the path, and Executable reads the
	// path back to notice a renamed binary.
	command := fmt.Sprintf(`"%s" %s %s`, exe, daemonGroup, daemonStartCommand)
	if err := key.SetStringValue(valueName, command); err != nil {
		return fmt.Errorf("write the Run registry value: %w", err)
	}
	return nil
}

// Disable removes the Run value.
func (m platformManager) Disable() error {
	if err := m.retireLegacy(); err != nil {
		return err
	}
	key, err := openRunKey(registry.SET_VALUE)
	if err != nil || key == 0 {
		return err
	}
	defer func() { _ = key.Close() }()
	if err := deleteValue(key, valueName); err != nil {
		return fmt.Errorf("remove the Run registry value: %w", err)
	}
	return nil
}

// IsEnabled reports whether the Run value exists.
func (platformManager) IsEnabled() bool {
	_, ok := platformManager{}.Executable()
	return ok
}

// Executable reports the executable the Run value starts.
func (platformManager) Executable() (string, bool) {
	key, err := openRunKey(registry.QUERY_VALUE)
	if err != nil || key == 0 {
		return "", false
	}
	defer func() { _ = key.Close() }()
	value, _, err := key.GetStringValue(valueName)
	if err != nil {
		return "", false
	}
	return splitQuotedExecutable(value)
}

func (platformManager) retireLegacy() error {
	key, err := openRunKey(registry.SET_VALUE)
	if err != nil || key == 0 {
		return err
	}
	defer func() { _ = key.Close() }()
	if err := deleteValue(key, trayValueName); err != nil {
		return fmt.Errorf("remove the retired Run registry value: %w", err)
	}
	return nil
}

func openRunKey(access uint32) (registry.Key, error) {
	key, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, access)
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			return 0, nil
		}
		return 0, fmt.Errorf("open the Run registry key: %w", err)
	}
	return key, nil
}

func deleteValue(key registry.Key, name string) error {
	err := key.DeleteValue(name)
	if err != nil && !errors.Is(err, registry.ErrNotExist) && !errors.Is(err, syscall.ERROR_FILE_NOT_FOUND) {
		return err
	}
	return nil
}
