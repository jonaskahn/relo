//go:build windows

package platform

import "syscall"

const (
	createNewProcessGroup = 0x00000200
	detachedProcess       = 0x00000008
	createNoWindow        = 0x08000000
)

// detachAttributes leaves the child without a console. The child must not
// attach its parent's console again, or closing that terminal ends it.
func detachAttributes() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		CreationFlags: createNewProcessGroup | detachedProcess | createNoWindow,
	}
}
