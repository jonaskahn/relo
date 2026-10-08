//go:build darwin

package sqlite

import (
	"fmt"
	"strings"
	"syscall"
)

var darwinNetworkFilesystems = []string{"nfs", "smbfs", "cifs", "afpfs", "webdav", "fuse.sshfs"}

// IsNetworkFS reports whether path lives on a remote filesystem, where
// SQLite's write-ahead log cannot stay consistent.
func IsNetworkFS(path string) (bool, error) {
	var stats syscall.Statfs_t
	if err := syscall.Statfs(path, &stats); err != nil {
		return false, fmt.Errorf("statfs %s: %w", path, err)
	}
	kind := strings.ToLower(int8SliceToString(stats.Fstypename[:]))
	for _, remote := range darwinNetworkFilesystems {
		if strings.Contains(kind, remote) {
			return true, nil
		}
	}
	return false, nil
}

func int8SliceToString(raw []int8) string {
	buffer := make([]byte, 0, len(raw))
	for _, value := range raw {
		if value == 0 {
			break
		}
		buffer = append(buffer, byte(value))
	}
	return string(buffer)
}
