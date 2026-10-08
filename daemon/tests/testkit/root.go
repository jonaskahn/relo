// Package testkit provides the shared helpers every Relo test suite uses.
package testkit

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// FixturePath returns the absolute path to a file under tests/fixtures/.
func FixturePath(parts ...string) string {
	_, thisFile, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(thisFile), "..", "fixtures")
	return filepath.Join(append([]string{root}, parts...)...)
}

// TempHome creates a temporary RELO_HOME directory with mode 0700.
func TempHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	if err := os.Chmod(home, 0o700); err != nil {
		t.Fatalf("chmod temp home: %v", err)
	}
	return home
}
