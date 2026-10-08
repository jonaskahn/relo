//go:build windows

package platform

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

func lockDaemon(file *os.File) error {
	// LockFileEx dereferences the overlapped offset, so it must never be nil.
	overlapped := new(windows.Overlapped)
	if err := windows.LockFileEx(
		windows.Handle(file.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK, 0, 1, 0, overlapped,
	); err != nil {
		return fmt.Errorf("lock the daemon log: %w", err)
	}
	return nil
}

func unlockDaemon(file *os.File) error {
	// LockFileEx dereferences the overlapped offset, so it must never be nil.
	overlapped := new(windows.Overlapped)
	if err := windows.UnlockFileEx(windows.Handle(file.Fd()), 0, 1, 0, overlapped); err != nil {
		return fmt.Errorf("unlock the daemon log: %w", err)
	}
	return nil
}
