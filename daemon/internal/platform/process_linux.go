//go:build linux

package platform

import (
	"fmt"
	"os"
	"strconv"
)

func processExecutable(pid int) (string, error) {
	path, err := os.Readlink("/proc/" + strconv.Itoa(pid) + "/exe")
	if err != nil {
		return "", fmt.Errorf("read process %d: %w", pid, err)
	}
	return path, nil
}
