//go:build darwin

package desktop

import (
	"os"
	"syscall"
)

func requestSparkleCheck() error {
	return syscall.Kill(os.Getppid(), syscall.SIGUSR1)
}
