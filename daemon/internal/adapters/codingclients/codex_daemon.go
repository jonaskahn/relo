// Codex daemon supervision: liveness and restart.
package codingclients

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

const (
	codexDaemonDir      = "app-server-daemon"
	codexDaemonPidFile  = "daemon.pid"
	codexRestartTimeout = 30 * time.Second
)

// ErrCodexLauncherNotFound reports a Codex install the daemon cannot drive.
var ErrCodexLauncherNotFound = errors.New("codex: the Codex launcher was not found; install Codex or put it on PATH")

// CodexDaemonStartedAt reports when the managed Codex app-server last started.
// A missing, unreadable, or dead pid file reports no daemon.
func CodexDaemonStartedAt(codexHome string) (time.Time, bool) {
	content, err := os.ReadFile(filepath.Join(codexHome, codexDaemonDir, codexDaemonPidFile))
	if err != nil {
		return time.Time{}, false
	}
	var file struct {
		PID             int `json:"pid"`
		ProcessIdentity struct {
			StartSeconds int64 `json:"startSeconds"`
		} `json:"processIdentity"`
	}
	if err := json.Unmarshal(content, &file); err != nil {
		return time.Time{}, false
	}
	if file.PID <= 0 || file.ProcessIdentity.StartSeconds <= 0 || !processAlive(file.PID) {
		return time.Time{}, false
	}
	return time.Unix(file.ProcessIdentity.StartSeconds, 0), true
}

// CodexNeedsRestart reports a managed Codex app-server that started before
// Relo last wrote the catalog, so the picker still holds the old list.
func CodexNeedsRestart(codexHome, catalogPath string) bool {
	started, found := CodexDaemonStartedAt(codexHome)
	if !found {
		return false
	}
	info, err := os.Stat(catalogPath)
	if err != nil {
		return false
	}
	return started.Before(info.ModTime())
}

// RestartCodexDaemon asks Codex to restart its managed app-server so the
// picker rereads Relo's catalog. A machine with no running daemon is left
// alone. Home is the operator's home, which the launcher resolver scans
// when the daemon PATH holds no Codex. An npm launcher is a Node script, so
// when that PATH also holds no node, Relo uses the node beside the launcher
// or the one the same lookup finds.
func RestartCodexDaemon(ctx context.Context, codexHome, home string) error {
	if _, found := CodexDaemonStartedAt(codexHome); !found {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	binary, err := findCodexBinary(ctx, codexHome, home)
	if err != nil {
		return err
	}
	binary, env := codexLauncherForDaemon(ctx, binary, os.Environ(), codexHome, home)
	ctx, cancel := context.WithTimeout(ctx, codexRestartTimeout)
	defer cancel()
	command := exec.CommandContext(ctx, binary, "app-server", "daemon", "restart")
	command.Env = append(env, "CODEX_HOME="+codexHome)
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("restart the Codex app-server: %w (%s)", err, output)
	}
	return nil
}

func codexLauncherForDaemon(ctx context.Context, binary string, env []string, codexHome, home string) (string, []string) {
	updated := envWithLauncherNode(ctx, home, binary, env)
	if scriptNeedsEnvNode(binary) && !pathHasExecutable(updated, "node") {
		if bundled := newestBundledCodex(codexHome); bundled != "" {
			return bundled, env
		}
	}
	return binary, updated
}

func processAlive(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return process.Signal(syscall.Signal(0)) == nil
}

func findCodexBinary(ctx context.Context, codexHome, home string) (string, error) {
	if resolved, err := findClientBinary(ctx, home, "codex"); err == nil {
		return resolved, nil
	}
	if bundled := newestBundledCodex(codexHome); bundled != "" {
		return bundled, nil
	}
	return "", ErrCodexLauncherNotFound
}

func newestBundledCodex(codexHome string) string {
	root := filepath.Join(codexHome, "packages", codexDaemonDir, "releases")
	entries, err := os.ReadDir(root)
	if err != nil {
		return ""
	}
	var newest string
	var newestTime time.Time
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		for _, name := range []string{"codex", "codex.exe"} {
			path := filepath.Join(root, entry.Name(), "bin", name)
			info, err := os.Stat(path)
			if err != nil {
				continue
			}
			if newest == "" || info.ModTime().After(newestTime) {
				newest = path
				newestTime = info.ModTime()
			}
		}
	}
	return newest
}
