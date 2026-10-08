//go:build windows

package cli

import (
	"io"
	"strings"
	"syscall"

	"golang.org/x/sys/windows/registry"
)

const (
	createNewProcessGroup = 0x00000200
	detachedProcess       = 0x00000008
	uninstallKeyPath      = `Software\Microsoft\Windows\CurrentVersion\Uninstall\Relo`
	uninstallValueName    = "UninstallString"
	// dataHandled tells the uninstaller that `relo uninstall` already decided
	// about the data, so it removes the program without asking again.
	dataHandled = "/DATA=handled"
)

func detachAttributes() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		CreationFlags: createNewProcessGroup | detachedProcess,
	}
}

// RemoveInstalledProgram hands this build to the uninstaller the installer
// registered. That uninstaller deletes the executable this process is running,
// which a running program cannot do itself, so it is spawned detached.
func (r Remover) RemoveInstalledProgram(out io.Writer, executable string, report Text) error {
	uninstaller, found := windowsUninstaller()
	if !found {
		return manualRemoval(out, report, "")
	}
	if err := r.SpawnDetached(uninstaller, dataHandled); err != nil {
		return manualRemoval(out, report, uninstaller+" "+dataHandled)
	}
	return nil
}

func windowsUninstaller() (string, bool) {
	key, err := registry.OpenKey(registry.CURRENT_USER, uninstallKeyPath, registry.QUERY_VALUE)
	if err != nil {
		return "", false
	}
	defer func() { _ = key.Close() }()
	command, _, err := key.GetStringValue(uninstallValueName)
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(command), command != ""
}
