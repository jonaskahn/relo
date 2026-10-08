//go:build windows

package platform

import (
	"fmt"

	"golang.org/x/sys/windows"
)

func processExecutable(pid int) (string, error) {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return "", fmt.Errorf("open process %d: %w", pid, err)
	}
	defer func() { _ = windows.CloseHandle(handle) }()
	var buf [windows.MAX_PATH]uint16
	size := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(handle, 0, &buf[0], &size); err != nil {
		return "", fmt.Errorf("read process %d: %w", pid, err)
	}
	return windows.UTF16ToString(buf[:size]), nil
}
