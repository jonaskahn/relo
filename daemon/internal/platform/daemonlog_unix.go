//go:build unix

package platform

import (
	"fmt"
	"os"
	"syscall"
)

func lockDaemon(file *os.File) error {
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("lock the daemon log: %w", err)
	}
	return nil
}

func unlockDaemon(file *os.File) error {
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_UN); err != nil {
		return fmt.Errorf("unlock the daemon log: %w", err)
	}
	return nil
}
