// Claude auth resolution: keychain presence and config state.
package codingclients

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	// ClaudeAuthLogin is a machine whose Claude Code has a claude.ai login.
	// Relo points it at the Anthropic surface and does not install a key.
	ClaudeAuthLogin ClaudeAuth = "login"
	// ClaudeAuthProxy is a machine with no claude.ai login. Relo installs the
	// key helper, which is the credential Claude Code then uses.
	ClaudeAuthProxy ClaudeAuth = "proxy"

	claudeKeychainService = "Claude Code-credentials"
	keychainMissing       = 44
	keychainProbe         = 1500 * time.Millisecond
)

// ClaudeAuth is how Claude Code authenticates on one machine.
type ClaudeAuth string

// ClaudeAuth reports whether Claude Code has a claude.ai login. An unreadable
// source counts as a login, so a subscriber is never handed a proxy key
// because a file or the keychain could not be read.
func (p Paths) ClaudeAuth() ClaudeAuth {
	return ResolveClaudeAuth(p.ClaudeConfigDir(), readClaudeKeychain)
}

// KeychainProbe reports the macOS credential item. present means the item
// exists. unreadable means the probe failed and must not be treated as absence.
type KeychainProbe func() (present, unreadable bool)

// ResolveClaudeAuth decides login or proxy from Claude Code's own files and
// the keychain probe the caller supplies. Any present source, and any source
// that could not be read, selects login. Proxy is only every readable source
// being absent.
func ResolveClaudeAuth(configDir string, keychain KeychainProbe) ClaudeAuth {
	sources := []KeychainProbe{
		func() (bool, bool) { return claudeJSONAccount(configDir) },
		func() (bool, bool) { return claudeCredentials(configDir) },
	}
	if keychain != nil {
		sources = append(sources, keychain)
	}
	unreadable := false
	for _, source := range sources {
		present, failed := source()
		if present {
			return ClaudeAuthLogin
		}
		if failed {
			unreadable = true
		}
	}
	if unreadable {
		return ClaudeAuthLogin
	}
	return ClaudeAuthProxy
}

func claudeJSONAccount(configDir string) (present, unreadable bool) {
	path := filepath.Join(filepath.Dir(configDir), ".claude.json")
	content, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, false
	}
	if err != nil {
		return false, true
	}
	var document struct {
		Account struct {
			Email string `json:"emailAddress"`
		} `json:"oauthAccount"`
	}
	if err := json.Unmarshal(content, &document); err != nil {
		return false, true
	}
	return strings.TrimSpace(document.Account.Email) != "", false
}

func claudeCredentials(configDir string) (present, unreadable bool) {
	_, err := os.Stat(filepath.Join(configDir, ".credentials.json"))
	if err == nil {
		return true, false
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, false
	}
	return false, true
}

func readClaudeKeychain() (present, unreadable bool) {
	if runtime.GOOS != "darwin" {
		return false, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), keychainProbe)
	defer cancel()
	cmd := exec.CommandContext(ctx, "security", "find-generic-password", "-s", claudeKeychainService)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	err := cmd.Run()
	if err == nil {
		return true, false
	}
	if ctx.Err() != nil {
		return false, true
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == keychainMissing {
		return false, false
	}
	return false, true
}
