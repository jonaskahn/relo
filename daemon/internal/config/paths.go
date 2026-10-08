// State directory paths: home, config, and database locations.
package config

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	configFileName = "config.toml"
	databaseName   = "state.sqlite"
	homeMode       = 0o700
	configMode     = 0o600
)

// ReloHome returns the state directory, honouring RELO_HOME.
func ReloHome() (string, error) {
	if home := os.Getenv(ReloHomeEnv); home != "" {
		return home, nil
	}
	userHome, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home: %w", err)
	}
	return filepath.Join(userHome, DefaultReloHome), nil
}

// EnsureReloHome creates the state directory when missing and verifies
// that an existing one is private to the user.
func EnsureReloHome(home string) error {
	if err := os.MkdirAll(home, homeMode); err != nil {
		return fmt.Errorf("create RELO_HOME: %w", err)
	}
	return checkHomePerm(home)
}

// ConfigPath returns the startup config path for a state directory.
func ConfigPath(home string) string {
	return filepath.Join(home, configFileName)
}

// DatabasePath returns the SQLite path for a state directory.
func DatabasePath(home string) string {
	return filepath.Join(home, databaseName)
}
