// ClientBinary locates a coding client's launcher the way the operator's
// terminal finds it: their PATH, their login shell, then the well-known
// install directories a daemon PATH misses.
package codingclients

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const shellProbeTimeout = 5 * time.Second

// ErrLauncherNotFound names why a coding client cannot be driven: its binary is
// nowhere the daemon looks.
var ErrLauncherNotFound = errors.New("the launcher was not found; install it or put it on PATH")

func findClientBinary(ctx context.Context, home, name string) (string, error) {
	if resolved, err := exec.LookPath(name); err == nil {
		return resolved, nil
	}
	if resolved := binaryFromShell(ctx, name); resolved != "" {
		return resolved, nil
	}
	for _, dir := range knownBinDirs(home) {
		if dir == "" {
			continue
		}
		if resolved := findExecutableInDir(dir, name); resolved != "" {
			return resolved, nil
		}
	}
	return "", fmt.Errorf("%s: %w", name, ErrLauncherNotFound)
}

func envWithLauncherNode(ctx context.Context, home, binary string, env []string) []string {
	if !scriptNeedsEnvNode(binary) || pathHasExecutable(env, "node") {
		return env
	}
	if dir := nodeBesideLauncher(binary); dir != "" {
		return prependDirToPath(env, dir)
	}
	if nodeBin, err := findClientBinary(ctx, home, "node"); err == nil {
		return prependDirToPath(env, filepath.Dir(nodeBin))
	}
	return env
}

func binaryFromShell(ctx context.Context, name string) string {
	shell := os.Getenv("SHELL")
	if shell == "" {
		switch runtime.GOOS {
		case "darwin":
			shell = "/bin/zsh"
		default:
			shell = "/bin/sh"
		}
	}
	shellCtx, cancel := context.WithTimeout(ctx, shellProbeTimeout)
	defer cancel()
	command := exec.CommandContext(shellCtx, shell, "-lic", "command -v "+name)
	command.Env = append(os.Environ(), "HOME="+os.Getenv("HOME"))
	// A login shell inherits no binary directory from this daemon beyond
	// what its rc files set up: the command answers with an absolute path,
	// and LookPath was already consulted for what the daemon PATH holds.
	output, err := command.Output()
	if err != nil {
		return ""
	}
	return existingExecutableInLines(string(output))
}

func existingExecutableInLines(output string) string {
	lines := strings.Split(output, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		candidate := strings.TrimSpace(lines[i])
		if !isExecutableFile(candidate) {
			continue
		}
		return candidate
	}
	return ""
}

func isExecutableFile(path string) bool {
	if path == "" {
		return false
	}
	if !filepath.IsAbs(path) {
		return false
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	if runtime.GOOS == "windows" {
		return true
	}
	return info.Mode().Perm()&0o111 != 0
}

func findExecutableInDir(dir, name string) string {
	suffixes := []string{""}
	if runtime.GOOS == "windows" {
		suffixes = []string{".exe", ".cmd", ".bat", ""}
	}
	for _, suffix := range suffixes {
		candidate := filepath.Join(dir, name+suffix)
		if isExecutableFile(candidate) {
			return candidate
		}
	}
	return ""
}

func knownBinDirs(home string) []string {
	dirs := []string{
		filepath.Join(home, ".opencode", "bin"),
		filepath.Join(home, ".local", "bin"),
		filepath.Join(home, "bin"),
		filepath.Join(home, ".bun", "bin"),
	}
	switch runtime.GOOS {
	case "windows":
		return append(dirs, windowsBinDirs(home)...)
	case "darwin":
		return append(dirs, darwinBinDirs(home)...)
	default:
		return append(dirs, linuxBinDirs(home)...)
	}
}

func windowsBinDirs(home string) []string {
	return []string{
		filepath.Join(envPath("APPDATA"), "npm"),
		filepath.Join(envPath("LOCALAPPDATA"), "pnpm"),
		filepath.Join(home, "scoop", "shims"),
		filepath.Join(envPath("LOCALAPPDATA"), "Microsoft", "WinGet", "Links"),
		filepath.Join(envPath("PROGRAMDATA"), "chocolatey", "bin"),
	}
}

func darwinBinDirs(home string) []string {
	return []string{
		filepath.Join(home, ".npm-global", "bin"),
		filepath.Join(home, ".npm", "bin"),
		nvmBinDir(home),
		filepath.Join(home, "Library", "pnpm"),
		"/opt/homebrew/bin",
		"/usr/local/bin",
	}
}

func linuxBinDirs(home string) []string {
	return []string{
		filepath.Join(home, ".npm-global", "bin"),
		filepath.Join(home, ".npm", "bin"),
		nvmBinDir(home),
		filepath.Join(home, ".local", "share", "pnpm"),
		"/home/linuxbrew/.linuxbrew/bin",
		"/usr/local/bin",
	}
}

func envPath(name string) string { return os.Getenv(name) }

func nvmBinDir(home string) string {
	root := filepath.Join(home, ".nvm", "versions", "node")
	entries, err := os.ReadDir(root)
	if err != nil {
		return ""
	}
	newest, newestSegments := "", []int(nil)
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "v") {
			continue
		}
		segments, ok := versionSegments(entry.Name()[1:])
		if !ok {
			continue
		}
		if newest == "" || compareSegments(segments, newestSegments) > 0 {
			newest, newestSegments = entry.Name(), segments
		}
	}
	if newest == "" {
		return ""
	}
	return filepath.Join(root, newest, "bin")
}

func versionSegments(version string) ([]int, bool) {
	major := version
	if cut := strings.IndexAny(version, "-+"); cut >= 0 {
		major = version[:cut]
	}
	fields := strings.Split(major, ".")
	segments := make([]int, 0, len(fields))
	for _, field := range fields {
		number, err := strconv.Atoi(field)
		if err != nil {
			break
		}
		segments = append(segments, number)
	}
	return segments, len(segments) > 0
}

func compareSegments(a, b []int) int {
	for i, left := range a {
		if i >= len(b) {
			return 1
		}
		switch {
		case left < b[i]:
			return -1
		case left > b[i]:
			return 1
		}
	}
	if len(b) > len(a) {
		return -1
	}
	return 0
}

func scriptNeedsEnvNode(path string) bool {
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer func() { _ = file.Close() }()
	buf := make([]byte, 256)
	n, err := file.Read(buf)
	if n == 0 || (err != nil && !errors.Is(err, io.EOF)) {
		return false
	}
	line := string(buf[:n])
	if cut := strings.IndexByte(line, '\n'); cut >= 0 {
		line = line[:cut]
	}
	if !strings.HasPrefix(line, "#!") {
		return false
	}
	fields := strings.Fields(line)
	for _, field := range fields[1:] {
		base := filepath.Base(field)
		if base == "node" || base == "node.exe" {
			return true
		}
	}
	return false
}

func nodeBesideLauncher(binary string) string {
	name := "node"
	if runtime.GOOS == "windows" {
		name = "node.exe"
	}
	dir := filepath.Dir(binary)
	if isExecutableFile(filepath.Join(dir, name)) {
		return dir
	}
	return ""
}

func pathHasExecutable(env []string, name string) bool {
	for _, entry := range env {
		key, value, ok := strings.Cut(entry, "=")
		if !ok || key != "PATH" {
			continue
		}
		for _, dir := range filepath.SplitList(value) {
			if findExecutableInDir(dir, name) != "" {
				return true
			}
		}
		return false
	}
	_, err := exec.LookPath(name)
	return err == nil
}

func prependDirToPath(env []string, dir string) []string {
	updated := make([]string, len(env), len(env)+1)
	copy(updated, env)
	for i, entry := range updated {
		key, value, ok := strings.Cut(entry, "=")
		if !ok || key != "PATH" {
			continue
		}
		updated[i] = "PATH=" + dir + string(os.PathListSeparator) + value
		return updated
	}
	return append(updated, "PATH="+dir)
}
