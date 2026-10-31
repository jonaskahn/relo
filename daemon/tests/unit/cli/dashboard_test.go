package cli_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestStartOpensOnlyWhenAttaching(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("uses a Linux browser launcher stub")
	}
	home, port := startedHome(t)
	dir := t.TempDir()
	output := filepath.Join(dir, "opened")
	t.Setenv("RELO_TEST_BROWSER_OUTPUT", output)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	if err := os.WriteFile(filepath.Join(dir, "xdg-open"), []byte("#!/bin/sh\nprintf '%s\\n' \"$1\" >> \"$RELO_TEST_BROWSER_OUTPUT\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	args := []string{"--home", home, "daemon", "start", "--port", strconv.Itoa(port)}
	if _, _, err := run(t, nil, args...); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("spawning parent opened dashboard: %v", err)
	}
	if _, _, err := run(t, nil, args...); err != nil {
		t.Fatal(err)
	}
	expected := "http://127.0.0.1:" + strconv.Itoa(port) + "\n"
	deadline := time.Now().Add(time.Second)
	for {
		data, _ := os.ReadFile(output)
		if string(data) == expected {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("opened %q, want %q", data, expected)
		}
		time.Sleep(time.Millisecond)
	}
	t.Setenv("PATH", t.TempDir())
	stdout, stderr, err := run(t, nil, args...)
	if err != nil || !strings.Contains(stdout, "already running") || !strings.Contains(stderr, "could not open the dashboard") {
		t.Fatalf("browser failure changed lifecycle result: stdout %q, stderr %q, error %v", stdout, stderr, err)
	}
}
