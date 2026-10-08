//go:build windows

package cli

import (
	"os"

	"golang.org/x/sys/windows"
)

func isTerminal(file *os.File) bool {
	var mode uint32
	handle := windows.Handle(file.Fd())
	if err := windows.GetConsoleMode(handle, &mode); err != nil {
		return false
	}
	return true
}
