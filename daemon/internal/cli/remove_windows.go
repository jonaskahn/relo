//go:build windows

package cli

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"syscall"

	"golang.org/x/sys/windows/registry"
)

const (
	createNewProcessGroup = 0x00000200
	detachedProcess       = 0x00000008
	uninstallKeyPath      = `Software\Microsoft\Windows\CurrentVersion\Uninstall\Relo`
	uninstallValueName    = "UninstallString"
)

func detachAttributes() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		CreationFlags: createNewProcessGroup | detachedProcess,
	}
}

func findSetupUninstaller() (string, bool) {
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

func (r Remover) removeCommand(executable string) string {
	return fmt.Sprintf("del /f \"%s\"", executable)
}

// removeRunningProgram asks cmd.exe to wait for this process to exit, then
// delete its executable. Windows keeps a running image locked, so the delete
// cannot happen in this process.
func removeRunningProgram(executable string) error {
	command := fmt.Sprintf(`ping 127.0.0.1 -n 3 >nul & del /f /q "%s"`, executable)
	return exec.Command("cmd.exe", "/c", command).Start()
}

// removeFromUserPath drops only the directory this script install added.
func removeFromUserPath(dir string) error {
	key, err := registry.OpenKey(registry.CURRENT_USER, `Environment`, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer func() { _ = key.Close() }()
	value, _, err := key.GetStringValue("Path")
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			return nil
		}
		return err
	}
	entries := strings.Split(value, ";")
	kept := entries[:0]
	for _, entry := range entries {
		if !strings.EqualFold(strings.TrimSpace(entry), dir) {
			kept = append(kept, entry)
		}
	}
	if len(kept) == len(entries) {
		return nil
	}
	if len(kept) == 0 {
		return key.DeleteValue("Path")
	}
	return key.SetExpandStringValue("Path", strings.Join(kept, ";"))
}
