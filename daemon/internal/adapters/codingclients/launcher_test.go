// Key helper scripts: the daemon start a client's key request triggers.
package codingclients

import (
	"strings"
	"testing"
)

// TestKeyHelpersStartTheDaemonSilently keeps a client's key request from
// opening the dashboard: Codex re-runs its helper every few minutes.
func TestKeyHelpersStartTheDaemonSilently(t *testing.T) {
	for name, script := range map[string]string{
		"unix":    KeyHelperScript(Paths{Relo: "/bin/relo", StateHome: "/home/.relo"}, "codex"),
		"windows": windowsKeyHelper(Paths{Relo: "relo.exe", StateHome: "C:\\relo"}, "codex"),
	} {
		if !strings.Contains(script, "daemon start --silent") {
			t.Errorf("%s helper must start the daemon with --silent, got:\n%s", name, script)
		}
	}
}
