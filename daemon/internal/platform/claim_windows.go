//go:build windows

package platform

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

func lockClaim(file *os.File) error {
	return lockClaimFlags(file, windows.LOCKFILE_EXCLUSIVE_LOCK)
}

func unlockClaim(file *os.File) error {
	// LockFileEx dereferences the overlapped offset, so it must never be nil.
	overlapped := new(windows.Overlapped)
	if err := windows.UnlockFileEx(windows.Handle(file.Fd()), 0, 1, 0, overlapped); err != nil {
		return fmt.Errorf("unlock the daemon claim: %w", err)
	}
	return nil
}

func tryLockClaim(file *os.File) (bool, error) {
	err := lockClaimFlags(file, windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
		return false, nil
	}
	return false, err
}

func lockClaimFlags(file *os.File, flags uint32) error {
	// LockFileEx dereferences the overlapped offset, so it must never be nil.
	overlapped := new(windows.Overlapped)
	if err := windows.LockFileEx(windows.Handle(file.Fd()), flags, 0, 1, 0, overlapped); err != nil {
		return err
	}
	return nil
}
