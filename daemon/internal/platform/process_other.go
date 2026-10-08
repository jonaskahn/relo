//go:build !darwin && !linux && !windows

package platform

import "errors"

// ErrProcessUnsupported reports a platform Relo cannot inspect processes on.
var ErrProcessUnsupported = errors.New("process inspection is not supported on this platform")

func processExecutable(int) (string, error) {
	return "", ErrProcessUnsupported
}
