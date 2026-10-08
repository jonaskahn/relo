//go:build !windows

package codingclients

import (
	"os"
	"path/filepath"
	"strings"
)

func writeShellEnv(paths Paths, agent Agent) error {
	for _, path := range shellFiles(paths) {
		if err := mergeShellFile(path, agent.ID, paths.KeyFile(agent.ID)); err != nil {
			return err
		}
	}
	return mergeFishFile(fishFile(paths), agent.ID, paths.KeyFile(agent.ID))
}

func stripShellEnv(paths Paths, agent Agent) error {
	for _, path := range shellFiles(paths) {
		if err := stripShellFile(path, agent.ID); err != nil {
			return err
		}
	}
	return stripShellFile(fishFile(paths), agent.ID)
}

func shellFiles(paths Paths) []string {
	return []string{
		filepath.Join(paths.Home, ".zshenv"),
		filepath.Join(paths.Home, ".zprofile"),
		filepath.Join(paths.Home, ".bashrc"),
		filepath.Join(paths.Home, ".bash_profile"),
		filepath.Join(paths.Home, ".profile"),
	}
}

func fishFile(paths Paths) string {
	return filepath.Join(paths.Home, ".config", "fish", "conf.d", "relo.fish")
}

func shellPublished(paths Paths, id string) bool {
	fence := envFenceStart(id)
	for _, path := range shellFiles(paths) {
		existing, err := ReadFileOrEmpty(path)
		if err != nil || !strings.Contains(existing, fence) {
			return false
		}
	}
	existing, err := ReadFileOrEmpty(fishFile(paths))
	return err == nil && strings.Contains(existing, fence)
}

func mergeShellFile(path, id, keyFile string) error {
	existing, err := ReadFileOrEmpty(path)
	if err != nil {
		return err
	}
	return WriteFileAtomic(path, []byte(MergeEnvFile(existing, id, keyFile)), 0o644)
}

func mergeFishFile(path, id, keyFile string) error {
	existing, err := ReadFileOrEmpty(path)
	if err != nil {
		return err
	}
	return WriteFileAtomic(path, []byte(MergeEnvBlock(existing, id, EnvBlockFish(id, keyFile))), 0o644)
}

func stripShellFile(path, id string) error {
	existing, err := ReadFileOrEmpty(path)
	if err != nil {
		return err
	}
	if strings.TrimSpace(existing) == "" {
		return nil
	}
	stripped := StripEnvFile(existing, id)
	if strings.TrimSpace(stripped) == "" {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	return WriteFileAtomic(path, []byte(stripped), 0o644)
}
