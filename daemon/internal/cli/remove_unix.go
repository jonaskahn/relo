//go:build !windows

package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

func detachAttributes() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}

func findSetupUninstaller() (string, bool) {
	return "", false
}

func (r Remover) removeCommand(executable string) string {
	return fmt.Sprintf("rm -f %s", shellQuote(executable))
}

func removeRunningProgram(executable string) error {
	err := os.Remove(executable)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func removeFromUserPath(dir string) error {
	return nil
}

// shellQuote single-quotes a path for a POSIX shell, closing and reopening
// the quote around each apostrophe.
func shellQuote(path string) string {
	return "'" + strings.ReplaceAll(filepath.Clean(path), "'", `'\''`) + "'"
}
