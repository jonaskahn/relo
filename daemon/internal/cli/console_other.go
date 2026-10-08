//go:build !windows

package cli

// EnsureConsole attaches the parent console on Windows. Everywhere else the
// process already has one, so there is nothing to do.
func EnsureConsole() {}
