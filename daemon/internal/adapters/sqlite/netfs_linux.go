//go:build linux

package sqlite

import (
	"fmt"
	"syscall"
)

const (
	nfsSuperMagic  = 0x6969
	smbSuperMagic  = 0x517B
	smb2SuperMagic = 0xFE534D42
	cifsSuperMagic = 0xFF534D42
)

var linuxNetworkFilesystems = map[int64]struct{}{
	nfsSuperMagic:  {},
	smbSuperMagic:  {},
	smb2SuperMagic: {},
	cifsSuperMagic: {},
}

// IsNetworkFS reports whether path lives on a remote filesystem, where
// SQLite's write-ahead log cannot stay consistent.
func IsNetworkFS(path string) (bool, error) {
	var stats syscall.Statfs_t
	if err := syscall.Statfs(path, &stats); err != nil {
		return false, fmt.Errorf("statfs %s: %w", path, err)
	}
	_, remote := linuxNetworkFilesystems[stats.Type]
	return remote, nil
}
