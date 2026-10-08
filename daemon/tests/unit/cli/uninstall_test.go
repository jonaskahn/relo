package cli_test

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/cli"
	"github.com/jonaskahn/relo/internal/config"
	"github.com/jonaskahn/relo/internal/platform"
	"github.com/jonaskahn/relo/tests/testkit"
)

// stateFiles are what a state directory holds besides its configuration, so a
// test can tell a wipe from a keep.
var stateFiles = []string{"state.sqlite", "secret.key", "secrets.enc"}

func installedHome(t *testing.T) string {
	t.Helper()
	home := testkit.TempHome(t)
	if _, err := config.LoadConfig(config.ConfigPath(home), nil); err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	for _, name := range stateFiles {
		if err := os.WriteFile(filepath.Join(home, name), []byte("content"), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return home
}

func stateExists(t *testing.T, home, name string) bool {
	t.Helper()
	_, err := os.Stat(filepath.Join(home, name))
	return err == nil
}

// answering scripts the terminal the uninstall would ask on, so a test can
// drive its one question.
func answering(t *testing.T, answer string) {
	t.Helper()
	original := cli.AskOnTerminal
	cli.AskOnTerminal = func() (io.Reader, bool) { return strings.NewReader(answer), true }
	t.Cleanup(func() { cli.AskOnTerminal = original })
}

// notAnswering is an uninstall with nobody at the terminal, which is what a
// piped install and a test runner both are.
func notAnswering(t *testing.T) {
	t.Helper()
	original := cli.AskOnTerminal
	cli.AskOnTerminal = func() (io.Reader, bool) { return nil, false }
	t.Cleanup(func() { cli.AskOnTerminal = original })
}

// keepingTheProgram keeps the last step of an uninstall away from the machine,
// so a test never asks a package manager to remove Relo.
func keepingTheProgram(t *testing.T) *bool {
	t.Helper()
	original := cli.RemoveProgram
	removed := false
	cli.RemoveProgram = func(io.Writer, func(string, map[string]any) string) error {
		removed = true
		return nil
	}
	t.Cleanup(func() { cli.RemoveProgram = original })
	return &removed
}

func TestUninstallFlags(t *testing.T) {
	t.Run("asking for both data answers at once is refused", func(t *testing.T) {
		home := testkit.TempHome(t)
		_, _, err := run(t, nil, "--home", home, "uninstall", "--keep-data", "--wipe")
		if !errors.Is(err, cli.ErrBothDataFlags) {
			t.Fatalf("uninstall error = %v, want the data-flag refusal", err)
		}
	})
}

func TestUninstallData(t *testing.T) {
	t.Run("--wipe removes the state directory", func(t *testing.T) {
		notAnswering(t)
		keepingTheProgram(t)
		home := installedHome(t)
		stdout, _, err := run(t, nil, "--home", home, "uninstall", "--wipe")
		if err != nil {
			t.Fatalf("uninstall --wipe error = %v", err)
		}
		if _, err := os.Stat(home); !os.IsNotExist(err) {
			t.Fatalf("the state directory survived the wipe: %v", err)
		}
		if !strings.Contains(stdout, "Relo data was removed") {
			t.Fatalf("stdout = %q, want the wipe report", stdout)
		}
	})

	t.Run("--keep-data leaves the state directory", func(t *testing.T) {
		keepingTheProgram(t)
		home := installedHome(t)
		stdout, _, err := run(t, nil, "--home", home, "uninstall", "--keep-data")
		if err != nil {
			t.Fatalf("uninstall --keep-data error = %v", err)
		}
		if !stateExists(t, home, "config.toml") {
			t.Fatal("the state directory was removed although the data was to be kept")
		}
		if !strings.Contains(stdout, home) {
			t.Fatalf("stdout = %q, want the kept home named", stdout)
		}
	})

	// The go test binary inherits the terminal of whoever ran it, so a test that
	// wants the unattended path has to say so.
	t.Run("a run with nobody to ask keeps the data", func(t *testing.T) {
		notAnswering(t)
		keepingTheProgram(t)
		home := installedHome(t)
		stdout, _, err := run(t, nil, "--home", home, "uninstall")
		if err != nil {
			t.Fatalf("uninstall error = %v", err)
		}
		if !stateExists(t, home, "config.toml") {
			t.Fatal("the state directory was removed although nobody was asked")
		}
		if strings.Contains(stdout, "Remove your Relo data") {
			t.Fatalf("stdout = %q, want no question asked without a terminal", stdout)
		}
	})
}

func TestUninstallPrompt(t *testing.T) {
	t.Run("an explicit yes wipes the data", func(t *testing.T) {
		keepingTheProgram(t)
		home := installedHome(t)
		answering(t, "y\n")
		stdout, _, err := run(t, nil, "--home", home, "uninstall")
		if err != nil {
			t.Fatalf("uninstall error = %v", err)
		}
		if !strings.Contains(stdout, "Remove your Relo data") {
			t.Fatalf("stdout = %q, want the data question", stdout)
		}
		if _, err := os.Stat(home); !os.IsNotExist(err) {
			t.Fatalf("an explicit yes did not wipe the state directory: %v", err)
		}
	})

	t.Run("an empty answer keeps the data", func(t *testing.T) {
		keepingTheProgram(t)
		home := installedHome(t)
		answering(t, "\n")
		stdout, _, err := run(t, nil, "--home", home, "uninstall")
		if err != nil {
			t.Fatalf("uninstall error = %v", err)
		}
		if !strings.Contains(stdout, "Remove your Relo data") {
			t.Fatalf("stdout = %q, want the data question", stdout)
		}
		if !stateExists(t, home, "config.toml") {
			t.Fatal("an empty answer removed the data")
		}
	})
}

func TestUninstallPromptAnswers(t *testing.T) {
	for _, tc := range []struct {
		answer string
		wipe   bool
	}{
		{"y\n", true},
		{"Y\n", true},
		{"yes\n", true},
		{"YES\n", true},
		{" y \n", true},
		{"\n", false},
		{"n\n", false},
		{"no\n", false},
		{"", false},
		{"yesterday\n", false},
	} {
		wipe, err := cli.PromptWipeData(strings.NewReader(tc.answer))
		if err != nil {
			t.Fatalf("PromptWipeData(%q) error = %v", tc.answer, err)
		}
		if wipe != tc.wipe {
			t.Fatalf("PromptWipeData(%q) = %v, want %v", tc.answer, wipe, tc.wipe)
		}
	}
}

func TestUninstallPrepare(t *testing.T) {
	t.Run("--prepare keeps the data and leaves the program in place", func(t *testing.T) {
		removed := keepingTheProgram(t)
		notAnswering(t)
		home := installedHome(t)
		stdout, _, err := run(t, nil, "--home", home, "uninstall", "--prepare")
		if err != nil {
			t.Fatalf("uninstall --prepare error = %v", err)
		}
		if !stateExists(t, home, "config.toml") {
			t.Fatal("--prepare removed the data a native remover was to ask about")
		}
		if *removed {
			t.Fatal("--prepare asked for the program to be removed")
		}
		if strings.Contains(stdout, "Removing Relo") {
			t.Fatalf("stdout = %q, want no program removal from a prepare", stdout)
		}
	})

	t.Run("--prepare --wipe is the purge a native remover asks for", func(t *testing.T) {
		removed := keepingTheProgram(t)
		home := installedHome(t)
		stdout, _, err := run(t, nil, "--home", home, "uninstall", "--prepare", "--wipe")
		if err != nil {
			t.Fatalf("uninstall --prepare --wipe error = %v", err)
		}
		if _, err := os.Stat(home); !os.IsNotExist(err) {
			t.Fatalf("the state directory survived the purge: %v", err)
		}
		if *removed {
			t.Fatal("--prepare asked for the program to be removed")
		}
		if strings.Contains(stdout, "Removing Relo") {
			t.Fatalf("stdout = %q, want no program removal from a prepare", stdout)
		}
	})

	t.Run("--prepare clears the runtime file a daemon published", func(t *testing.T) {
		home := installedHome(t)
		writeRuntimeFile(t, home, platform.Runtime{
			Address: "127.0.0.1:1", InstanceID: "instance-under-test", PID: 1,
		})
		if _, _, err := run(t, nil, "--home", home, "uninstall", "--prepare"); err != nil {
			t.Fatalf("uninstall --prepare error = %v", err)
		}
		if _, found := platform.ReadRuntime(home); found {
			t.Fatal("the runtime file survived the prepare")
		}
	})
}

func TestUninstallQuiesce(t *testing.T) {
	t.Run("a prepare frees the port a running daemon was serving", func(t *testing.T) {
		home, port := startedHome(t)
		if stdout, _, err := run(t, nil, "--home", home, "daemon", "start", "--port", strconv.Itoa(port)); err != nil {
			t.Fatalf("daemon start error = %v (stdout %s)", err, stdout)
		}
		if !stillListening(t, port) {
			t.Fatalf("the stand-in daemon never served %d", port)
		}
		if _, _, err := run(t, nil, "--home", home, "uninstall", "--prepare"); err != nil {
			t.Fatalf("uninstall --prepare error = %v", err)
		}
		if stillListening(t, port) {
			t.Fatalf("port %d is still held after the prepare", port)
		}
		if _, found := platform.ReadRuntime(home); found {
			t.Fatal("the runtime file survived the prepare")
		}
	})

	t.Run("a prepare empties a port another program holds, because Relo is leaving", func(t *testing.T) {
		home := testkit.TempHome(t)
		port := freePort(t)
		holdPortWithForeignProcess(t, port)
		writePort(t, home, strconv.Itoa(port))
		if _, _, err := run(t, nil, "--home", home, "uninstall", "--prepare"); err != nil {
			t.Fatalf("uninstall --prepare error = %v", err)
		}
		if stillListening(t, port) {
			t.Fatalf("port %d is still held after the prepare", port)
		}
	})
}

// holdPortWithForeignProcess binds a port in a process that is not a Relo
// daemon, which is what the port-inspection code has to tell apart. The binary
// is copied out of the way first: a copy is not the executable a running Relo
// process would name, which is the only thing that tells the two apart.
func holdPortWithForeignProcess(t *testing.T, port int) {
	t.Helper()
	holder := filepath.Join(t.TempDir(), "port-holder")
	body, err := os.ReadFile(os.Args[0])
	if err != nil {
		t.Fatalf("read this test binary: %v", err)
	}
	if err := os.WriteFile(holder, body, 0o755); err != nil {
		t.Fatalf("copy this test binary: %v", err)
	}
	command := exec.Command(holder)
	command.Env = append(os.Environ(), holdPortEnv+"="+strconv.Itoa(port))
	if err := command.Start(); err != nil {
		t.Fatalf("start the port holder: %v", err)
	}
	t.Cleanup(func() { _ = command.Process.Kill() })
	waitForPort(t, port)
}

func TestUninstallHelp(t *testing.T) {
	t.Run("the data flags are offered", func(t *testing.T) {
		stdout, _, err := run(t, nil, "uninstall", "--help")
		if err != nil {
			t.Fatalf("uninstall --help error = %v", err)
		}
		for _, flag := range []string{"--keep-data", "--wipe"} {
			if !strings.Contains(stdout, flag) {
				t.Fatalf("help = %q, want %s listed", stdout, flag)
			}
		}
	})

	t.Run("--prepare is hidden from an operator who did not ask for it", func(t *testing.T) {
		stdout, _, err := run(t, nil, "uninstall", "--help")
		if err != nil {
			t.Fatalf("uninstall --help error = %v", err)
		}
		if strings.Contains(stdout, "--prepare") {
			t.Fatalf("help = %q, want the prepare flag hidden", stdout)
		}
	})
}
