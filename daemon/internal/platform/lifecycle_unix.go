//go:build darwin || linux

package platform

import "syscall"

func detachAttributes() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}
