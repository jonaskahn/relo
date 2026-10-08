//go:build darwin

package codingclients

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const envAgentPrefix = "com.relo.env."

func applyLiveEnv(paths Paths, agent Agent) error {
	if err := writeEnvAgent(paths, agent); err != nil {
		return err
	}
	if !liveHome(paths.Home) {
		return nil
	}
	value, err := keyValue(paths, agent.ID)
	if err != nil {
		return nil
	}
	if err := exec.Command("launchctl", "setenv", agent.EnvKey, value).Run(); err != nil {
		return fmt.Errorf("set %s: %w", agent.EnvKey, err)
	}
	loadEnvAgent(paths, agent)
	return nil
}

func clearLiveEnv(paths Paths, agent Agent) error {
	if err := removeEnvAgent(paths, agent); err != nil {
		return err
	}
	if !liveHome(paths.Home) {
		return nil
	}
	if err := exec.Command("launchctl", "unsetenv", agent.EnvKey).Run(); err != nil {
		return fmt.Errorf("unset %s: %w", agent.EnvKey, err)
	}
	return nil
}

func writeEnvAgent(paths Paths, agent Agent) error {
	path := envAgentPath(paths, agent.ID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create the LaunchAgents directory: %w", err)
	}
	return WriteFileAtomic(path, []byte(envAgentPlist(agent, paths.KeyFile(agent.ID))), 0o644)
}

func envPublished(paths Paths, agent Agent) bool {
	if !shellPublished(paths, agent.ID) {
		return false
	}
	_, err := os.Stat(envAgentPath(paths, agent.ID))
	return err == nil
}

func removeEnvAgent(paths Paths, agent Agent) error {
	if liveHome(paths.Home) {
		_ = exec.Command("launchctl", "bootout", launchDomain()+"/"+envAgentLabel(agent.ID)).Run()
	}
	if err := os.Remove(envAgentPath(paths, agent.ID)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func loadEnvAgent(paths Paths, agent Agent) {
	domain := launchDomain()
	label := envAgentLabel(agent.ID)
	_ = exec.Command("launchctl", "bootout", domain+"/"+label).Run()
	_ = exec.Command("launchctl", "bootstrap", domain, envAgentPath(paths, agent.ID)).Run()
}

func envAgentPath(paths Paths, id string) string {
	return filepath.Join(paths.Home, "Library", "LaunchAgents", envAgentLabel(id)+".plist")
}

func envAgentLabel(id string) string {
	return envAgentPrefix + id
}

func launchDomain() string {
	return fmt.Sprintf("gui/%d", os.Getuid())
}

func envAgentPlist(agent Agent, keyFile string) string {
	command := "launchctl setenv " + agent.EnvKey + " \"$(tr -d '\\n' < " + shellSingleQuote(keyFile) + ")\""
	return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>` + envAgentLabel(agent.ID) + `</string>
	<key>ProgramArguments</key>
	<array>
		<string>/bin/sh</string>
		<string>-c</string>
		<string>` + envXMLEscape(command) + `</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
</dict>
</plist>
`
}

func envXMLEscape(value string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(value)
}
