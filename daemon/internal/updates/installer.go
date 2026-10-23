// Install channel detection names the program that installed this build, so
// the console can teach the command that upgrades it.
package updates

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// The channels DetectInstaller reports.
const (
	InstallerSparkle    = "sparkle"
	InstallerBrew       = "brew"
	InstallerNPM        = "npm"
	InstallerShell      = "shell"
	InstallerPowerShell = "powershell"
	InstallerSetup      = "installer"
	InstallerUnknown    = "unknown"
)

const (
	scriptReceipt    = ".relo-install"
	receiptChannel   = "channel=script"
	npmPackagePath   = "/node_modules/@jonaskahn/relo/"
	brewStablePath   = "/Cellar/relo/"
	brewBetaPath     = "/Cellar/relo-beta/"
	windowsSetupPath = "/programs/relo/"
	linuxPackagePath = "/usr/bin/relo"
)

// DetectInstaller reports the channel that installed the running build, or
// InstallerUnknown when the executable carries no recognizable marker.
func DetectInstaller() string {
	executable, err := os.Executable()
	if err != nil {
		return InstallerUnknown
	}
	return InstallerFor(executable, runtime.GOOS)
}

// InstallerFor reports the channel that installed executable on goos. The
// uninstaller and the update notice share this one answer, so a build cannot
// be removed through a different channel than the one that updates it.
func InstallerFor(executable, goos string) string {
	slashPath := filepath.ToSlash(executable)
	if inAppBundle(slashPath) {
		return InstallerSparkle
	}
	if strings.Contains(slashPath, npmPackagePath) {
		return InstallerNPM
	}
	resolved := executable
	if target, err := filepath.EvalSymlinks(executable); err == nil {
		resolved = target
	}
	if scriptReceiptInstalled(resolved) {
		if goos == "windows" {
			return InstallerPowerShell
		}
		return InstallerShell
	}
	resolvedPath := filepath.ToSlash(resolved)
	if strings.Contains(resolvedPath, brewBetaPath) || strings.Contains(resolvedPath, brewStablePath) {
		return InstallerBrew
	}
	if goos == "windows" && strings.Contains(strings.ToLower(slashPath), windowsSetupPath) {
		return InstallerSetup
	}
	if goos == "linux" && resolvedPath == linuxPackagePath {
		return InstallerSetup
	}
	return InstallerUnknown
}

func scriptReceiptInstalled(executable string) bool {
	body, err := os.ReadFile(ScriptReceiptPath(executable))
	return err == nil && strings.Contains(string(body), receiptChannel)
}

// ScriptReceiptPath names the marker a shell or PowerShell install writes
// beside its binary. The uninstaller deletes it with the program it proves.
func ScriptReceiptPath(executable string) string {
	return filepath.Join(filepath.Dir(executable), scriptReceipt)
}

func installerOf(c *Checker) string {
	if c != nil && strings.TrimSpace(c.Installer) != "" {
		return c.Installer
	}
	return InstallerUnknown
}
