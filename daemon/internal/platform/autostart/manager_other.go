//go:build !darwin && !linux && !windows

package autostart

import "errors"

// ErrAutostartUnsupported reports a platform without start-at-login.
var ErrAutostartUnsupported = errors.New("start at login is not supported on this platform")

type platformManager struct{}

// New returns the manager for this platform.
func New() Manager { return platformManager{} }

// Enable reports that start at login is unsupported here.
func (platformManager) Enable(string) error {
	return ErrAutostartUnsupported
}

// Disable has nothing to remove.
func (platformManager) Disable() error { return nil }

// IsEnabled is always false here.
func (platformManager) IsEnabled() bool { return false }

// Executable is always unknown here.
func (platformManager) Executable() (string, bool) { return "", false }
