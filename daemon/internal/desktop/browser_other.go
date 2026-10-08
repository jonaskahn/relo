//go:build !darwin && !linux && !windows

package desktop

import (
	"errors"
	"os/exec"
)

// ErrBrowserUnsupported reports a platform Relo cannot open a browser on.
var ErrBrowserUnsupported = errors.New("opening a browser is not supported on this platform")

func openCommand(string) (*exec.Cmd, error) {
	return nil, ErrBrowserUnsupported
}
