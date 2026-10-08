//go:build windows

package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/windows"
)

func reportStartupError(err error) {
	logPath := appendStartupLog(err)
	if hasConsole() {
		return
	}
	showErrorDialog(err, logPath)
}

func hasConsole() bool {
	window, _, _ := procGetConsoleWindow.Call()
	return window != 0
}

func appendStartupLog(err error) string {
	exe, exeErr := os.Executable()
	if exeErr != nil {
		return ""
	}
	path := filepath.Join(filepath.Dir(exe), "startup.log")
	entry := fmt.Sprintf("%s relo: %v\n", time.Now().Format(time.RFC3339), err)
	file, openErr := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if openErr != nil {
		return ""
	}
	defer func() { _ = file.Close() }()
	if _, writeErr := file.WriteString(entry); writeErr != nil {
		return ""
	}
	return path
}

func showErrorDialog(err error, logPath string) {
	text := "Relo could not start:\n\n" + err.Error()
	if logPath != "" {
		text += "\n\nDetails were written to:\n" + logPath
	}
	message, messageErr := windows.UTF16PtrFromString(text)
	caption, captionErr := windows.UTF16PtrFromString("Relo")
	if messageErr != nil || captionErr != nil {
		return
	}
	_, _ = windows.MessageBox(0, message, caption, windows.MB_OK|windows.MB_ICONERROR|windows.MB_TOPMOST)
}
