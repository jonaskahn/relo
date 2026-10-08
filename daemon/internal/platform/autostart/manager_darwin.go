//go:build darwin

package autostart

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	agentLabel    = "com.relo.daemon"
	agentFileName = "com.relo.daemon.plist"
	// trayAgentLabel and trayAgentFileName are the registration a desktop app
	// from before the tray moved into the daemon wrote. It names a command
	// this build no longer carries, so both Enable and Disable remove it.
	trayAgentLabel    = "com.relo.tray"
	trayAgentFileName = "com.relo.tray.plist"
)

type platformManager struct{}

// New returns the manager for this platform.
func New() Manager { return platformManager{} }

// Enable writes the LaunchAgent plist, which launchd picks up at the next
// login. The running instance is left alone.
func (m platformManager) Enable(exe string) error {
	if err := m.retireLegacy(); err != nil {
		return err
	}
	dir, err := launchAgentsDir()
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, agentFileName), []byte(agentPlist(exe)), fileMode); err != nil {
		return fmt.Errorf("write the LaunchAgent: %w", err)
	}
	return nil
}

// Disable unloads and removes the LaunchAgent. A label that was never loaded
// is not an error.
func (m platformManager) Disable() error {
	if err := m.retireLegacy(); err != nil {
		return err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve the home directory: %w", err)
	}
	_ = exec.Command("launchctl", "bootout", fmt.Sprintf("gui/%d/%s", os.Getuid(), agentLabel)).Run()
	if err := removeFile(filepath.Join(home, "Library", "LaunchAgents", agentFileName)); err != nil {
		return fmt.Errorf("remove the LaunchAgent: %w", err)
	}
	return nil
}

// IsEnabled reports whether the LaunchAgent exists.
func (platformManager) IsEnabled() bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	return fileExists(filepath.Join(home, "Library", "LaunchAgents", agentFileName))
}

// Executable reports the executable the LaunchAgent starts.
func (platformManager) Executable() (string, bool) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", false
	}
	raw, err := os.ReadFile(filepath.Join(home, "Library", "LaunchAgents", agentFileName))
	if err != nil {
		return "", false
	}
	var parsed struct {
		Dict struct {
			Arguments [][]string `xml:"array>string"`
		} `xml:"dict"`
	}
	if err := xml.Unmarshal(raw, &parsed); err != nil {
		return "", false
	}
	for _, args := range parsed.Dict.Arguments {
		if len(args) > 0 {
			return args[0], true
		}
	}
	return "", false
}

func (platformManager) retireLegacy() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve the home directory: %w", err)
	}
	_ = exec.Command("launchctl", "bootout", fmt.Sprintf("gui/%d/%s", os.Getuid(), trayAgentLabel)).Run()
	if err := removeFile(filepath.Join(home, "Library", "LaunchAgents", trayAgentFileName)); err != nil {
		return fmt.Errorf("remove the retired tray LaunchAgent: %w", err)
	}
	return nil
}

func launchAgentsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve the home directory: %w", err)
	}
	dir := filepath.Join(home, "Library", "LaunchAgents")
	if err := os.MkdirAll(dir, dirMode); err != nil {
		return "", fmt.Errorf("create the LaunchAgents directory: %w", err)
	}
	return dir, nil
}

func agentPlist(exe string) string {
	return launchAgentPlist(agentLabel, exe, "Interactive", daemonGroup, daemonRunCommand)
}

func launchAgentPlist(label, exe, processType string, args ...string) string {
	var arguments strings.Builder
	for _, arg := range args {
		arguments.WriteString("\t\t<string>" + xmlEscape(arg) + "</string>\n")
	}
	body := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>` + label + `</string>
	<key>ProgramArguments</key>
	<array>
		<string>` + xmlEscape(exe) + `</string>
` + arguments.String() + `	</array>
	<key>RunAtLoad</key>
	<true/>
`
	if processType != "" {
		body += "\t<key>ProcessType</key>\n\t<string>" + processType + "</string>\n"
	}
	return body + "</dict>\n</plist>\n"
}

func xmlEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}

func removeFile(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
