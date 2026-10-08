package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	"github.com/jonaskahn/relo/internal/cli"
	"github.com/jonaskahn/relo/internal/config"
	"github.com/jonaskahn/relo/internal/server"
	"github.com/jonaskahn/relo/tests/testkit"
)

func TestVersionCommand(t *testing.T) {
	t.Run("version prints expected format", func(t *testing.T) {
		stdout, stderr, err := run(t, nil, "version")
		if err != nil {
			t.Fatalf("version error = %v (stderr %s)", err, stderr)
		}
		if got := cli.Version() + "\n"; stdout != got {
			t.Fatalf("stdout = %q, want %q", stdout, got)
		}
	})
}

func TestRootCommand(t *testing.T) {
	t.Run("root command help includes the commands that remain", func(t *testing.T) {
		stdout, _, err := run(t, nil, "--help")
		if err != nil {
			t.Fatalf("help error = %v", err)
		}
		for _, name := range []string{"autostart", "daemon", "uninstall", "version"} {
			if !strings.Contains(stdout, name) {
				t.Fatalf("help = %q, want %s listed", stdout, name)
			}
		}
	})

	t.Run("the management commands are gone", func(t *testing.T) {
		root, err := cli.NewRootCommand("en")
		if err != nil {
			t.Fatalf("NewRootCommand() error = %v", err)
		}
		// The console owns provider and account management now, so the tree
		// holds the commands that run the app and nothing else.
		want := []string{"autostart", "daemon", "uninstall", "version"}
		got := make([]string, 0, len(want))
		for _, command := range root.Commands() {
			// help and completion are cobra's own, created ahead of Execute
			// so Relo can word them in the language of the run.
			if command.Name() == "help" || command.Name() == "completion" {
				continue
			}
			got = append(got, command.Name())
		}
		sort.Strings(got)
		sort.Strings(want)
		if strings.Join(got, " ") != strings.Join(want, " ") {
			t.Fatalf("commands = %v, want %v", got, want)
		}
	})

	t.Run("help speaks the language of the run, including cobra's own text", func(t *testing.T) {
		root, err := cli.NewRootCommand("vi")
		if err != nil {
			t.Fatalf("NewRootCommand() error = %v", err)
		}
		stdout := &bytes.Buffer{}
		root.SetOut(stdout)
		root.SetArgs([]string{"--help"})
		if err := root.Execute(); err != nil {
			t.Fatalf("help error = %v", err)
		}
		for _, want := range []string{
			"Cách dùng:", "Lệnh khả dụng:", "Cờ:",
			"Trợ giúp về mọi lệnh", "Tạo tập lệnh tự động hoàn thành cho shell được chỉ định",
			"trợ giúp cho relo", `Dùng "relo [command] --help"`,
		} {
			if !strings.Contains(stdout.String(), want) {
				t.Errorf("help = %q, want %q", stdout.String(), want)
			}
		}
		for _, english := range []string{"Usage:", "Available Commands:", "Flags:", "Help about any command", "help for"} {
			if strings.Contains(stdout.String(), english) {
				t.Errorf("help = %q, still holds English %q", stdout.String(), english)
			}
		}
	})

	t.Run("an unknown command fails", func(t *testing.T) {
		if _, _, err := run(t, nil, "nonsense"); err == nil {
			t.Fatal("unknown command error = nil, want a failure")
		}
	})

	t.Run("exit codes follow the failure kind", func(t *testing.T) {
		if got := cli.ExitCode(nil); got != 0 {
			t.Fatalf("ExitCode(nil) = %d, want 0", got)
		}
		if got := cli.ExitCode(errors.New("plain")); got != 1 {
			t.Fatalf("ExitCode(plain) = %d, want 1", got)
		}
	})
}

func TestStatusCommand(t *testing.T) {
	t.Run("status when not running returns error message", func(t *testing.T) {
		home := testkit.TempHome(t)
		t.Setenv(config.ReloHomeEnv, home)
		stdout, _, err := run(t, nil, "daemon", "status")
		if err == nil {
			t.Fatal("status error = nil, want a failure")
		}
		if !errors.Is(err, cli.ErrNotRunning) {
			t.Fatalf("status error = %v, want %v", err, cli.ErrNotRunning)
		}
		if !strings.Contains(stdout, "Relo is not running") {
			t.Fatalf("stdout = %q, want the not-running message", stdout)
		}
		// A daemon that is not running is its own outcome, so a script can
		// tell it apart from a check that failed.
		if got := cli.ExitCode(err); got != 3 {
			t.Fatalf("exit code = %d, want 3", got)
		}
	})

	t.Run("status prints a running report", func(t *testing.T) {
		home := testkit.TempHome(t)
		management := fakeManagement(t, home, statusPayload())
		t.Setenv(config.ReloHomeEnv, home)
		writePort(t, home, portOf(t, management.URL))
		stdout, _, err := run(t, nil, "daemon", "status")
		if err != nil {
			t.Fatalf("status error = %v", err)
		}
		fragments := []string{"Relo is running", "test-version", "1h 23m", "127.0.0.1:10101", "keychain",
			"Schema version:", fmt.Sprintf("%d", sqlite.LatestSchemaVersion())}
		for _, fragment := range fragments {
			if !strings.Contains(stdout, fragment) {
				t.Fatalf("stdout = %q, want %s", stdout, fragment)
			}
		}
	})

	t.Run("status json prints the report", func(t *testing.T) {
		home := testkit.TempHome(t)
		management := fakeManagement(t, home, statusPayload())
		t.Setenv(config.ReloHomeEnv, home)
		writePort(t, home, portOf(t, management.URL))
		stdout, _, err := run(t, nil, "daemon", "status", "--json")
		if err != nil {
			t.Fatalf("status error = %v", err)
		}
		var report cli.StatusReport
		if err := json.Unmarshal([]byte(stdout), &report); err != nil {
			t.Fatalf("decode report: %v", err)
		}
		if report.Version != "test-version" || report.SchemaVersion != sqlite.LatestSchemaVersion() || report.Status != "running" {
			t.Fatalf("report = %+v, want the daemon state", report)
		}
	})

	t.Run("status reports an unreachable daemon", func(t *testing.T) {
		home := testkit.TempHome(t)
		if _, err := server.EnsureToken(server.TokenPath(home, server.AdminTokenFile)); err != nil {
			t.Fatalf("EnsureToken() error = %v", err)
		}
		t.Setenv(config.ReloHomeEnv, home)
		writePort(t, home, "1")
		if _, err := cli.Status(context.Background(), home); !errors.Is(err, cli.ErrNotRunning) {
			t.Fatalf("Status() error = %v, want %v", err, cli.ErrNotRunning)
		}
	})

	t.Run("status reports a rejected report", func(t *testing.T) {
		home := testkit.TempHome(t)
		management := fakeManagementStatus(t, home, http.StatusInternalServerError, "{")
		t.Setenv(config.ReloHomeEnv, home)
		writePort(t, home, portOf(t, management.URL))
		if _, err := cli.Status(context.Background(), home); err == nil {
			t.Fatal("Status() error = nil, want a failure for a rejected report")
		}
	})

	t.Run("status reports a broken configuration", func(t *testing.T) {
		home := testkit.TempHome(t)
		path := config.ConfigPath(home)
		if err := os.WriteFile(path, []byte("not = = toml"), 0o600); err != nil {
			t.Fatalf("write the broken config: %v", err)
		}
		t.Setenv(config.ReloHomeEnv, home)
		_, _, err := run(t, nil, "daemon", "status")
		if err == nil {
			t.Fatal("status error = nil, want a parse failure")
		}
		if errors.Is(err, cli.ErrNotRunning) || !strings.Contains(err.Error(), "parse config") {
			t.Fatalf("status error = %v, want the parse failure", err)
		}
	})
}

// run executes the command line with the given arguments and returns what
// the user would see.
// stillListening reports whether something accepts on a port, which is how a
// test tells a freed port from one a process kept.
func stillListening(t *testing.T, port int) bool {
	t.Helper()
	conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), time.Second)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// waitForPort blocks until a port accepts a connection, which is how a test
// knows a process it just started is serving.
func waitForPort(t *testing.T, port int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !stillListening(t, port) {
		if time.Now().After(deadline) {
			t.Fatalf("nothing came up on port %d", port)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func run(t *testing.T, stdin *strings.Reader, args ...string) (string, string, error) {
	t.Helper()
	root, err := cli.NewRootCommand("en")
	if err != nil {
		t.Fatalf("NewRootCommand() error = %v", err)
	}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.SetArgs(args)
	if stdin != nil {
		root.SetIn(stdin)
	}
	err = root.Execute()
	return stdout.String(), stderr.String(), err
}
