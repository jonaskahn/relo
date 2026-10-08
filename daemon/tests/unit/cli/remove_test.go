package cli_test

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/cli"
)

// removal records what a removal would have done, which is the only way to
// drive these flavors without a package manager, npm, or an app bundle on the
// test machine.
type removal struct {
	spawned []string
	removed []string
	run     []string
	// removalsFail makes the removal itself fail while the package manager
	// still answers that it knows the package.
	removalsFail bool
	knownTools   map[string]bool
	quitAsked    bool
}

// report renders a message as its id followed by its data, so a test can see
// which message was printed and what it was given.
func (r *removal) report(id string, data map[string]any) string {
	rendered := id
	for _, value := range data {
		rendered += " " + strings.TrimSpace(strings.Join([]string{toText(value)}, ""))
	}
	return rendered
}

func toText(value any) string {
	text, _ := value.(string)
	return text
}

// install points a remover's process seams at this recorder.
func (r *removal) install() cli.Remover {
	return cli.Remover{
		LookPath: func(name string) (string, error) {
			if r.knownTools[name] {
				return "/usr/bin/" + name, nil
			}
			return "", errors.New("not found: " + name)
		},
		SpawnDetached: func(name string, args ...string) error {
			r.spawned = append(r.spawned, strings.Join(append([]string{name}, args...), " "))
			return nil
		},
		RunRemoval: func(_ io.Writer, name string, args ...string) error {
			command := strings.Join(append([]string{name}, args...), " ")
			r.run = append(r.run, command)
			if r.removalsFail && !strings.Contains(command, "-s ") && !strings.Contains(command, "-q ") {
				return errors.New("refused")
			}
			return nil
		},
		RemoveTree: func(path string) error {
			r.removed = append(r.removed, path)
			return nil
		},
		AskTrayToQuit: func() { r.quitAsked = true },
	}
}

// npmPackage writes the two files that mark a directory as an npm install of
// relo, which is how the installed layout is reproduced.
func npmPackage(t *testing.T) (root string, executable string) {
	t.Helper()
	root = t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "bin"), 0o755); err != nil {
		t.Fatalf("create the bin directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "package.json"),
		[]byte(`{"name":"relo","version":"0.1.0","bin":{"relo":"bin/relo.mjs"}}`), 0o600); err != nil {
		t.Fatalf("write package.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "bin", "relo.mjs"), []byte("//"), 0o600); err != nil {
		t.Fatalf("write the launcher: %v", err)
	}
	return root, filepath.Join(root, "bin", "relo")
}

func macBundle(t *testing.T) (bundle string, executable string) {
	t.Helper()
	bundle = filepath.Join(t.TempDir(), "Relo.app")
	executable = filepath.Join(bundle, "Contents", "MacOS", "relo")
	if err := os.MkdirAll(filepath.Dir(executable), 0o755); err != nil {
		t.Fatalf("create the MacOS directory: %v", err)
	}
	if err := os.WriteFile(executable, []byte("binary"), 0o755); err != nil {
		t.Fatalf("write the executable: %v", err)
	}
	return bundle, executable
}

func TestRemoveInstalledProgram(t *testing.T) {
	t.Run("an npm install is uninstalled by npm", func(t *testing.T) {
		recorded := &removal{knownTools: map[string]bool{"npm": true}}
		remover := recorded.install()
		_, executable := npmPackage(t)
		if err := remover.RemoveInstalledProgram(io.Discard, executable, recorded.report); err != nil {
			t.Fatalf("RemoveInstalledProgram() error = %v", err)
		}
		if len(recorded.spawned) != 1 || recorded.spawned[0] != "npm uninstall -g relo" {
			t.Fatalf("spawned = %v, want the npm uninstall", recorded.spawned)
		}
	})

	t.Run("a Debian package is removed by apt", func(t *testing.T) {
		recorded := &removal{knownTools: map[string]bool{"dpkg": true, "apt-get": true}}
		remover := recorded.install()
		if err := remover.RemoveInstalledProgram(io.Discard, "/usr/bin/relo", recorded.report); err != nil {
			t.Fatalf("RemoveInstalledProgram() error = %v", err)
		}
		if len(recorded.run) != 2 || !strings.HasSuffix(recorded.run[1], "remove -y relo") {
			t.Fatalf("run = %v, want the dpkg question and the apt removal", recorded.run)
		}
	})

	t.Run("an rpm package is removed by dnf", func(t *testing.T) {
		recorded := &removal{knownTools: map[string]bool{"rpm": true, "dnf": true}}
		remover := recorded.install()
		if err := remover.RemoveInstalledProgram(io.Discard, "/usr/bin/relo", recorded.report); err != nil {
			t.Fatalf("RemoveInstalledProgram() error = %v", err)
		}
		if !strings.HasSuffix(recorded.run[len(recorded.run)-1], "remove -y relo") {
			t.Fatalf("run = %v, want the dnf removal", recorded.run)
		}
	})

	t.Run("a failed package removal names the command to run", func(t *testing.T) {
		recorded := &removal{knownTools: map[string]bool{"dpkg": true, "apt-get": true}, removalsFail: true}
		remover := recorded.install()
		out := &strings.Builder{}
		err := remover.RemoveInstalledProgram(out, "/usr/bin/relo", recorded.report)
		if !errors.Is(err, cli.ErrManualRemoval) {
			t.Fatalf("RemoveInstalledProgram() error = %v, want the manual refusal", err)
		}
		if !strings.Contains(out.String(), "remove -y relo") {
			t.Fatalf("out = %q, want the command to run", out.String())
		}
	})

	t.Run("a macOS bundle deletes itself, after the tray is asked to quit", func(t *testing.T) {
		recorded := &removal{}
		remover := recorded.install()
		bundle, executable := macBundle(t)
		if err := remover.RemoveInstalledProgram(io.Discard, executable, recorded.report); err != nil {
			t.Fatalf("RemoveInstalledProgram() error = %v", err)
		}
		if !recorded.quitAsked {
			t.Fatal("the tray was not asked to quit")
		}
		if len(recorded.removed) != 1 || recorded.removed[0] != bundle {
			t.Fatalf("removed = %v, want the bundle", recorded.removed)
		}
	})

	t.Run("a bundle that cannot be deleted is reported as a move to the Trash", func(t *testing.T) {
		recorded := &removal{}
		remover := recorded.install()
		_, executable := macBundle(t)
		remover.RemoveTree = func(string) error { return errors.New("busy") }
		err := remover.RemoveInstalledProgram(io.Discard, executable, recorded.report)
		if err == nil {
			t.Fatal("RemoveInstalledProgram() error = nil, want the removal failure")
		}
		if !strings.Contains(err.Error(), "cli.uninstall.trash") {
			t.Fatalf("error = %v, want the move-to-the-Trash report", err)
		}
	})

	t.Run("an install nothing knows about is reported by hand", func(t *testing.T) {
		recorded := &removal{}
		remover := recorded.install()
		out := &strings.Builder{}
		err := remover.RemoveInstalledProgram(out, "/usr/local/bin/relo", recorded.report)
		if !errors.Is(err, cli.ErrManualRemoval) {
			t.Fatalf("RemoveInstalledProgram() error = %v, want the manual refusal", err)
		}
		if !strings.Contains(out.String(), "cli.uninstall.manual") {
			t.Fatalf("out = %q, want the manual report", out.String())
		}
	})

	t.Run("an npm that cannot be found is reported with the command", func(t *testing.T) {
		recorded := &removal{}
		remover := recorded.install()
		_, executable := npmPackage(t)
		out := &strings.Builder{}
		err := remover.RemoveInstalledProgram(out, executable, recorded.report)
		if !errors.Is(err, cli.ErrManualRemoval) {
			t.Fatalf("RemoveInstalledProgram() error = %v, want the manual refusal", err)
		}
		if !strings.Contains(out.String(), "npm uninstall -g relo") {
			t.Fatalf("out = %q, want the npm command", out.String())
		}
	})
}
