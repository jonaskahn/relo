//go:build darwin || windows

package codingclients

import (
	"os"
	"path/filepath"
	"strings"
)

func liveHome(home string) bool {
	real, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	return filepath.Clean(home) == filepath.Clean(real)
}

func keyValue(paths Paths, id string) (string, error) {
	content, err := os.ReadFile(paths.KeyFile(id))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(content)), nil
}
