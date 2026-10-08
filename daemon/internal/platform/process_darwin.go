//go:build darwin

package platform

import (
	"errors"
	"fmt"

	"golang.org/x/sys/unix"
)

// Process errors name why the executable behind a pid cannot be read: a
// sysctl that failed and an argument block too short to hold one.
var (
	ErrProcessArgsShort = errors.New("the process arguments were too short")
)

func processExecutable(pid int) (string, error) {
	data, err := unix.SysctlRaw("kern.procargs2", pid)
	if err != nil {
		return "", fmt.Errorf("read process %d: %w", pid, err)
	}
	if len(data) < 5 {
		return "", ErrProcessArgsShort
	}
	path := data[4:]
	for i, b := range path {
		if b == 0 {
			return string(path[:i]), nil
		}
	}
	return string(path), nil
}
