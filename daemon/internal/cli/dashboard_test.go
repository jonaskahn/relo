package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

func TestOpenDashboardUsesLiveAddress(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("uses a Linux browser launcher stub")
	}
	dir := t.TempDir()
	output := filepath.Join(dir, "opened")
	t.Setenv("PATH", dir)
	t.Setenv("RELO_TEST_BROWSER_OUTPUT", output)
	if err := os.WriteFile(filepath.Join(dir, "xdg-open"), []byte("#!/bin/sh\nprintf '%s' \"$1\" > \"$RELO_TEST_BROWSER_OUTPUT\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ address, url string }{
		{"127.0.0.1:12345", "http://127.0.0.1:12345"},
		{"0.0.0.0:12346", "http://127.0.0.1:12346"},
		{"[::]:12347", "http://127.0.0.1:12347"},
		{"[::1]:12348", "http://[::1]:12348"},
	} {
		t.Run(tc.address, func(t *testing.T) {
			openDashboard(tc.address, func(err error) { t.Fatal(err) })
			deadline := time.Now().Add(time.Second)
			for {
				data, _ := os.ReadFile(output)
				if string(data) == tc.url {
					break
				}
				if time.Now().After(deadline) {
					t.Fatalf("opened %q, want %q", data, tc.url)
				}
				time.Sleep(time.Millisecond)
			}
		})
	}
}

func TestOpenCommandDashboardWarnsWithoutFailing(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	var stderr bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetErr(&stderr)
	openCommandDashboard(cmd, "127.0.0.1:12345")
	if !strings.Contains(stderr.String(), "could not open the dashboard") {
		t.Fatalf("stderr = %q, want browser warning", stderr.String())
	}
}

func TestOpenDashboardRejectsInvalidAddresses(t *testing.T) {
	for _, address := range []string{"invalid", "localhost:invalid"} {
		t.Run(address, func(t *testing.T) {
			warnings := 0
			openDashboard(address, func(err error) {
				if err == nil {
					t.Error("warning has no error")
				}
				warnings++
			})
			if warnings != 1 {
				t.Fatalf("warnings = %d, want 1", warnings)
			}
		})
	}
}
