package cli_test

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/cli"
	"github.com/jonaskahn/relo/internal/config"
	"github.com/jonaskahn/relo/internal/i18n"
	"github.com/jonaskahn/relo/tests/testkit"
)

// runLang executes the command line in one language, which is how a
// setting, a flag, or the system locale changes what a run prints.
func runLang(t *testing.T, language string, args ...string) (string, string, error) {
	t.Helper()
	root, err := cli.NewRootCommand(language)
	if err != nil {
		t.Fatalf("NewRootCommand() error = %v", err)
	}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.SetArgs(args)
	err = root.Execute()
	return stdout.String(), stderr.String(), err
}

func TestCommandLineSpeaksTheRunLanguage(t *testing.T) {
	home := testkit.TempHome(t)
	t.Setenv(config.ReloHomeEnv, home)
	t.Setenv("LC_ALL", "en_US.UTF-8")

	t.Run("the help is printed in the language of the run", func(t *testing.T) {
		stdout, _, err := runLang(t, "de", "--help")
		if err != nil {
			t.Fatalf("--help error = %v", err)
		}
		if !strings.Contains(stdout, "Lokaler KI-Proxy") {
			t.Fatalf("stdout = %q, want the German summary", stdout)
		}
	})

	t.Run("a command's own report follows too", func(t *testing.T) {
		stdout, _, err := runLang(t, "zh-Hans", "daemon", "status")
		if err == nil {
			t.Fatal("status error = nil, want the not-running exit code")
		}
		if !strings.Contains(stdout, "Relo 未在运行") {
			t.Fatalf("stdout = %q, want the localized not-running message", stdout)
		}
	})
}

func TestLanguageReloCannotSpeakFailsTheRun(t *testing.T) {
	t.Run("a flag naming an unknown language", func(t *testing.T) {
		if _, err := cli.NewRootCommand("sw"); !errors.Is(err, i18n.ErrUnsupportedLanguage) {
			t.Fatalf("NewRootCommand() error = %v, want ErrUnsupportedLanguage", err)
		}
	})

	t.Run("the configuration naming an unknown language", func(t *testing.T) {
		fresh := testkit.TempHome(t)
		body := `[ui]
language = "sw"
`
		if err := os.WriteFile(config.ConfigPath(fresh), []byte(body), 0o600); err != nil {
			t.Fatalf("write config: %v", err)
		}
		if err := withArgs(t, "--home", fresh, "version"); !errors.Is(err, i18n.ErrUnsupportedLanguage) {
			t.Fatalf("Execute() error = %v, want ErrUnsupportedLanguage", err)
		}
	})
}

func TestLanguageLayersAreReadHighestFirst(t *testing.T) {
	t.Run("the configuration decides when nothing else does", func(t *testing.T) {
		home := testkit.TempHome(t)
		body := `[ui]
language = "de"
`
		if err := os.WriteFile(config.ConfigPath(home), []byte(body), 0o600); err != nil {
			t.Fatalf("write config: %v", err)
		}
		t.Setenv(config.ReloHomeEnv, home)
		stdout, _, err := runLang(t, "", "daemon", "status")
		if err == nil {
			t.Fatal("status error = nil, want the not-running exit code")
		}
		if !strings.Contains(stdout, "Relo läuft nicht") {
			t.Fatalf("stdout = %q, want the configured language", stdout)
		}
	})

	t.Run("the flag wins over what is configured", func(t *testing.T) {
		home := testkit.TempHome(t)
		body := `[ui]
language = "de"
`
		if err := os.WriteFile(config.ConfigPath(home), []byte(body), 0o600); err != nil {
			t.Fatalf("write config: %v", err)
		}
		t.Setenv(config.ReloHomeEnv, home)
		stdout, _, err := runLang(t, "en", "daemon", "status")
		if err == nil {
			t.Fatal("status error = nil, want the not-running exit code")
		}
		if !strings.Contains(stdout, "Relo is not running") {
			t.Fatalf("stdout = %q, want the flag's language", stdout)
		}
	})

	t.Run("the preflight reads the flags a run was started with", func(t *testing.T) {
		home := testkit.TempHome(t)
		if err := withArgs(t, "--home", home, "--lang", "de", "version"); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
	})
}
