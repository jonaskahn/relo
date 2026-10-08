// Launcher records and the key-helper scripts agents invoke.
package codingclients

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	// legacyLauncherMarker identifies the shim earlier Relo versions installed
	// at the Codex launcher path, so it is recognised and never mistaken for
	// the operator's own client.
	legacyLauncherMarker = "relo codex launcher shim"
	helperMarker         = "relo key helper"
	launcherBackupSuffix = ".relo-backup"
	// markerScanBytes bounds how much of a launcher is read to look for the
	// marker, because the launcher on PATH may be a native binary.
	markerScanBytes = 4096
)

// LauncherRecord is the state file an earlier Relo version kept for a wrapped
// Codex launcher: where it sat and where the real binary was moved.
type LauncherRecord struct {
	Path          string `json:"path"`
	RealPath      string `json:"real_path"`
	Digest        string `json:"digest"`
	Relo          string `json:"relo"`
	Home          string `json:"home"`
	KeyFile       string `json:"key_file"`
	InstalledAtMs int64  `json:"installed_at_ms"`
}

// KeyHelperScript renders the helper a coding client runs for its key. The
// secret stays in Relo's own directory rather than in the client's file, and
// the helper starts the daemon on the way, which is the only hook these
// clients offer without wrapping their binary.
func KeyHelperScript(paths Paths, agentID string) string {
	if runtime.GOOS == "windows" {
		return windowsKeyHelper(paths, agentID)
	}
	return strings.Join([]string{
		"#!/bin/sh",
		"# " + helperMarker,
		"if [ -n \"" + paths.Relo + "\" ] && [ -x \"" + paths.Relo + "\" ]; then",
		"  \"" + paths.Relo + "\" --home \"" + paths.StateHome + "\" daemon start >/dev/null 2>&1 || true",
		"fi",
		"cat \"" + paths.KeyFile(agentID) + "\"",
		"",
	}, "\n")
}

func windowsKeyHelper(paths Paths, agentID string) string {
	return strings.Join([]string{
		"@echo off",
		"rem " + helperMarker,
		"\"" + paths.Relo + "\" --home \"" + paths.StateHome + "\" daemon start >nul 2>nul",
		"type \"" + paths.KeyFile(agentID) + "\"",
		"",
	}, "\r\n")
}

// LauncherState reads the record of a launcher an earlier version wrapped,
// and reports whether one exists at all.
func LauncherState(path string) (LauncherRecord, bool, error) {
	content, err := ReadFileOrEmpty(path)
	if err != nil {
		return LauncherRecord{}, false, err
	}
	if strings.TrimSpace(content) == "" {
		return LauncherRecord{}, false, nil
	}
	record := LauncherRecord{}
	if err := json.Unmarshal([]byte(content), &record); err != nil {
		return LauncherRecord{}, false, fmt.Errorf("read the launcher state %s: %w", path, err)
	}
	return record, true, nil
}

// RetireLauncher puts the operator's own Codex launcher back wherever an
// earlier Relo version wrapped it, whether or not its state file survived.
// It never touches a launcher Relo did not write.
func RetireLauncher(statePath string) error {
	if _, err := RemoveLauncher(statePath); err != nil {
		return err
	}
	return removeStrayShim()
}

// RemoveLauncher undoes the wrapping recorded in a state file. A launcher that
// was replaced by a reinstall is left alone, and only the stale copy Relo kept
// goes. A launcher that vanished gets its real binary back rather than losing
// it.
func RemoveLauncher(statePath string) (bool, error) {
	record, found, err := LauncherState(statePath)
	if err != nil {
		return false, err
	}
	if !found {
		return false, nil
	}
	if err := unwrap(record); err != nil {
		return false, err
	}
	if err := os.Remove(statePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("remove the launcher state %s: %w", statePath, err)
	}
	return true, nil
}

func unwrap(record LauncherRecord) error {
	switch {
	case isLegacyShim(record.Path):
		return replaceShimWithBackup(record.Path, record.RealPath)
	case !exists(record.Path):
		return restoreBackup(record.Path, record.RealPath)
	default:
		return discardBackup(record.RealPath)
	}
}

func removeStrayShim() error {
	resolved, err := exec.LookPath("codex")
	if err != nil {
		return nil
	}
	path, err := filepath.Abs(resolved)
	if err != nil || !isLegacyShim(path) {
		return nil
	}
	return replaceShimWithBackup(path, path+launcherBackupSuffix)
}

func replaceShimWithBackup(shim, backup string) error {
	if err := os.Remove(shim); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove the launcher shim %s: %w", shim, err)
	}
	return restoreBackup(shim, backup)
}

func restoreBackup(path, backup string) error {
	if !exists(backup) {
		return nil
	}
	if err := os.Rename(backup, path); err != nil {
		return fmt.Errorf("restore %s: %w", path, err)
	}
	return nil
}

func discardBackup(backup string) error {
	if err := os.Remove(backup); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove the stale launcher copy %s: %w", backup, err)
	}
	return nil
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func isLegacyShim(path string) bool {
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer func() { _ = file.Close() }()
	head, err := io.ReadAll(io.LimitReader(file, markerScanBytes))
	if err != nil {
		return false
	}
	return strings.Contains(string(head), legacyLauncherMarker)
}
