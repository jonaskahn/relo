//go:build windows

package sqlite

import (
	"fmt"

	"golang.org/x/sys/windows"
)

// IsNetworkFS reports whether path lives on a mapped or UNC drive.
func IsNetworkFS(path string) (bool, error) {
	pointer, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return false, fmt.Errorf("encode path %s: %w", path, err)
	}
	return windows.GetDriveType(pointer) == windows.DRIVE_REMOTE, nil
}
