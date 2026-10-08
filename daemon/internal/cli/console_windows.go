//go:build windows

package cli

import (
	"os"

	"golang.org/x/sys/windows"
)

const attachParentProcess = ^uint32(0)

var (
	kernel32             = windows.NewLazySystemDLL("kernel32.dll")
	procAttachConsole    = kernel32.NewProc("AttachConsole")
	procGetConsoleWindow = kernel32.NewProc("GetConsoleWindow")
)

// EnsureConsole attaches the parent console to this GUI-subsystem binary, so
// a command someone runs from a terminal prints into it. A launch from
// Explorer, a shortcut, or the login item has no parent console and stays
// silent. Execute calls this before the command tree runs.
func EnsureConsole() {
	attachParentConsole()
}

func attachParentConsole() {
	window, _, _ := procGetConsoleWindow.Call()
	if window != 0 {
		return
	}
	if attached, _, _ := procAttachConsole.Call(uintptr(attachParentProcess)); attached == 0 {
		return
	}
	output, err := os.OpenFile("CONOUT$", os.O_WRONLY, 0)
	if err != nil {
		return
	}
	os.Stdout = output
	os.Stderr = output
}
