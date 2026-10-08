//go:build darwin || linux

package config

import (
	"fmt"
	"os"
)

func checkHomePerm(home string) error {
	info, err := os.Stat(home)
	if err != nil {
		return fmt.Errorf("stat RELO_HOME: %w", err)
	}
	if info.Mode().Perm() != homeMode {
		return fmt.Errorf("RELO_HOME %s: %w", home, ErrHomePerm)
	}
	return nil
}
