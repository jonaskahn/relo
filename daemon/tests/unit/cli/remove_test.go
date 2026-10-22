package cli_test

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/cli"
	"github.com/jonaskahn/relo/internal/updates"
)

// removal records what a removal would have done, which is the only way to
// drive every install channel without a package manager, npm, or an app bundle
// on the test machine.
type removal struct {
	spawned []string
	removed []string
	files   []string
	run     []string
	// removalsFail makes the removal itself fail while the package manager
	// still answers that it knows the package.
	removalsFail  bool
	knownTools    map[string]bool
	quitAsked     bool
	present       map[string]bool
	goos          string
	removeFile    func(string) error
	setup         string
	brewInstalled bool
	npmRoot       string
	running       []string
}

// report renders a message as its id followed by its data, so a test can see
// which message was printed and what it was given.
func (r *removal) report(id string, data map[string]any) string {
	rendered := id
	for _, value := range data {
		rendered += " " + strings.TrimSpace(toText(value))
	}
	return rendered
}

func toText(value any) string {
	text, _ := value.(string)
	return text
}

// install points a remover's effects at this recorder.
func (r *removal) install() cli.Remover {
	if r.present == nil {
		r.present = map[string]bool{}
	}
	return cli.Remover{
		GOOS: r.goos,
		LookPath: func(name string) (string, error) {
			if r.knownTools[name] {
				return "/usr/bin/" + name, nil
			}
			return "", errors.New("not found: " + name)
		},
		Exists: func(path string) bool { return r.present[path] },
		SetupUninstaller: func() (string, bool) {
			return r.setup, r.setup != ""
		},
		RemoveRunningProgram: func(executable string) error {
			r.running = append(r.running, executable)
			return nil
		},
		RemoveFromUserPath: func(string) error { return nil },
		RemoveFile: func(path string) error {
			if r.removeFile != nil {
				return r.removeFile(path)
			}
			r.files = append(r.files, path)
			delete(r.present, path)
			return nil
		},
		SpawnDetached: func(name string, args ...string) error {
			r.spawned = append(r.spawned, strings.Join(append([]string{name}, args...), " "))
			return nil
		},
		RunRemoval: func(_ io.Writer, name string, args ...string) error {
			command := strings.Join(append([]string{name}, args...), " ")
			r.run = append(r.run, command)
			if name == "npm" && len(args) > 0 && args[0] == "uninstall" {
				delete(r.present, r.npmRoot)
			}
			if name == "brew" && len(args) > 0 {
				if args[0] == "uninstall" {
					r.brewInstalled = false
				}
				if args[0] == "list" && !r.brewInstalled {
					return errors.New("not installed")
				}
			}
			if r.removalsFail && !strings.Contains(command, "-s ") && !strings.Contains(command, "-q ") && !strings.Contains(command, "list ") {
				return errors.New("refused")
			}
			return nil
		},
		RemoveTree: func(path string) error {
			r.removed = append(r.removed, path)
			delete(r.present, path)
			return nil
		},
		AskTrayToQuit: func() { r.quitAsked = true },
	}
}

// npmPackage writes the two files that mark a directory as an npm install of
// relo, which is how the installed layout is reproduced.
func npmPackage(t *testing.T) (root string, executable string) {
	t.Helper()
	root = filepath.Join(t.TempDir(), "node_modules", "@jonaskahn", "relo")
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

func scriptInstall(t *testing.T, goos string) (dir, executable string) {
	t.Helper()
	dir = t.TempDir()
	executable = filepath.Join(dir, "relo")
	if goos == "windows" {
		executable += ".exe"
	}
	if err := os.WriteFile(executable, []byte("binary"), 0o755); err != nil {
		t.Fatalf("write the executable: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".relo-install"), []byte("channel=script\n"), 0o600); err != nil {
		t.Fatalf("write the receipt: %v", err)
	}
	return dir, executable
}

func TestRemoveInstalledProgram(t *testing.T) {
	t.Run("a shell script install deletes its binary and receipt", func(t *testing.T) {
		_, executable := scriptInstall(t, "linux")
		receipt := updates.ScriptReceiptPath(executable)
		recorded := &removal{}
		recorded.removeFile = func(path string) error {
			recorded.files = append(recorded.files, path)
			return os.Remove(path)
		}
		out := &strings.Builder{}
		if err := recorded.install().RemoveInstalledProgram(out, executable, recorded.report); err != nil {
			t.Fatalf("RemoveInstalledProgram() error = %v", err)
		}
		for _, path := range []string{executable, receipt} {
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatalf("%s survived the script removal: %v", path, err)
			}
		}
		if !strings.Contains(out.String(), "cli.uninstall.verified") {
			t.Fatalf("out = %q, want the completed removal", out.String())
		}
	})

	t.Run("an npm install is uninstalled by its scoped package name", func(t *testing.T) {
		recorded := &removal{knownTools: map[string]bool{"npm": true}}
		_, executable := npmPackage(t)
		recorded.npmRoot = filepath.Dir(filepath.Dir(executable))
		recorded.present = map[string]bool{recorded.npmRoot: true}
		if err := recorded.install().RemoveInstalledProgram(io.Discard, executable, recorded.report); err != nil {
			t.Fatalf("RemoveInstalledProgram() error = %v", err)
		}
		if len(recorded.run) != 1 || recorded.run[0] != "npm uninstall -g @jonaskahn/relo" {
			t.Fatalf("run = %v, want the scoped npm removal", recorded.run)
		}
	})

	t.Run("a Debian package prints the privileged command", func(t *testing.T) {
		recorded := &removal{goos: "linux", knownTools: map[string]bool{"dpkg": true}}
		out := &strings.Builder{}
		if err := recorded.install().RemoveInstalledProgram(out, "/usr/bin/relo", recorded.report); err != nil {
			t.Fatalf("RemoveInstalledProgram() error = %v", err)
		}
		if !strings.Contains(out.String(), "sudo apt remove relo") {
			t.Fatalf("out = %q, want the apt command", out.String())
		}
		if len(recorded.files) != 0 || len(recorded.run) != 0 {
			t.Fatalf("files = %v run = %v, want the package left for the operator", recorded.files, recorded.run)
		}
	})

	t.Run("an rpm package prints the privileged command", func(t *testing.T) {
		recorded := &removal{goos: "linux", knownTools: map[string]bool{"rpm": true}}
		out := &strings.Builder{}
		if err := recorded.install().RemoveInstalledProgram(out, "/usr/bin/relo", recorded.report); err != nil {
			t.Fatalf("RemoveInstalledProgram() error = %v", err)
		}
		if !strings.Contains(out.String(), "sudo dnf remove relo") {
			t.Fatalf("out = %q, want the dnf command", out.String())
		}
	})

	t.Run("a macOS bundle deletes itself, after the tray is asked to quit", func(t *testing.T) {
		bundle, executable := macBundle(t)
		recorded := &removal{present: map[string]bool{bundle: true}}
		if err := recorded.install().RemoveInstalledProgram(io.Discard, executable, recorded.report); err != nil {
			t.Fatalf("RemoveInstalledProgram() error = %v", err)
		}
		if !recorded.quitAsked {
			t.Fatal("the tray was not asked to quit")
		}
		if len(recorded.removed) != 1 || recorded.removed[0] != bundle {
			t.Fatalf("removed = %v, want the bundle", recorded.removed)
		}
	})

	t.Run("a Homebrew stable install runs brew uninstall relo", func(t *testing.T) {
		recorded := &removal{knownTools: map[string]bool{"brew": true}, brewInstalled: true}
		executable := filepath.Join(t.TempDir(), "Cellar", "relo", "0.1.3", "bin", "relo")
		if err := recorded.install().RemoveInstalledProgram(io.Discard, executable, recorded.report); err != nil {
			t.Fatalf("RemoveInstalledProgram() error = %v", err)
		}
		if len(recorded.run) != 2 || recorded.run[0] != "brew uninstall relo" {
			t.Fatalf("run = %v, want the Homebrew removal and its verification", recorded.run)
		}
	})

	t.Run("a Homebrew beta install runs brew uninstall relo-beta", func(t *testing.T) {
		recorded := &removal{knownTools: map[string]bool{"brew": true}, brewInstalled: true}
		executable := filepath.Join(t.TempDir(), "Cellar", "relo-beta", "0.1.4", "bin", "relo")
		if err := recorded.install().RemoveInstalledProgram(io.Discard, executable, recorded.report); err != nil {
			t.Fatalf("RemoveInstalledProgram() error = %v", err)
		}
		if len(recorded.run) != 2 || recorded.run[0] != "brew uninstall relo-beta" {
			t.Fatalf("run = %v, want the beta removal", recorded.run)
		}
	})

	t.Run("a Windows setup is handed to its uninstaller", func(t *testing.T) {
		recorded := &removal{goos: "windows", setup: `C:/Users/me/AppData/Local/Programs/Relo/uninstall.exe`}
		executable := `C:/Users/me/AppData/Local/Programs/Relo/relo.exe`
		if err := recorded.install().RemoveInstalledProgram(io.Discard, executable, recorded.report); err != nil {
			t.Fatalf("RemoveInstalledProgram() error = %v", err)
		}
		if len(recorded.spawned) != 1 || !strings.Contains(recorded.spawned[0], "/DATA=handled") {
			t.Fatalf("spawned = %v, want the setup uninstaller", recorded.spawned)
		}
	})

	t.Run("a Windows script install schedules its own deletion", func(t *testing.T) {
		dir, executable := scriptInstall(t, "windows")
		recorded := &removal{goos: "windows", present: map[string]bool{executable: true}}
		if err := recorded.install().RemoveInstalledProgram(io.Discard, executable, recorded.report); err != nil {
			t.Fatalf("RemoveInstalledProgram() error = %v", err)
		}
		if len(recorded.running) != 1 || !strings.Contains(recorded.running[0], executable) {
			t.Fatalf("running = %v, want the delayed self removal", recorded.running)
		}
		_ = dir
	})

	t.Run("an unknown install deletes its own resolved binary", func(t *testing.T) {
		dir := t.TempDir()
		executable := filepath.Join(dir, "relo")
		if err := os.WriteFile(executable, []byte("binary"), 0o755); err != nil {
			t.Fatalf("write the executable: %v", err)
		}
		recorded := &removal{}
		recorded.removeFile = func(path string) error {
			recorded.files = append(recorded.files, path)
			return os.Remove(path)
		}
		if err := recorded.install().RemoveInstalledProgram(io.Discard, executable, recorded.report); err != nil {
			t.Fatalf("RemoveInstalledProgram() error = %v", err)
		}
		if _, err := os.Stat(executable); !os.IsNotExist(err) {
			t.Fatalf("the unknown install survived: %v", err)
		}
	})

	t.Run("a failed removal reports the command and the failure", func(t *testing.T) {
		dir, executable := scriptInstall(t, "linux")
		recorded := &removal{present: map[string]bool{executable: true}}
		recorded.removeFile = func(path string) error {
			if path == executable {
				return errors.New("busy")
			}
			delete(recorded.present, path)
			return nil
		}
		out := &strings.Builder{}
		err := recorded.install().RemoveInstalledProgram(out, executable, recorded.report)
		if err == nil {
			t.Fatal("RemoveInstalledProgram() error = nil, want the removal failure")
		}
		if !strings.Contains(out.String(), "cli.uninstall.run") {
			t.Fatalf("out = %q, want the finishing command", out.String())
		}
		_ = dir
	})

	t.Run("a path that survives verification fails the removal", func(t *testing.T) {
		dir := t.TempDir()
		executable := filepath.Join(dir, "relo")
		recorded := &removal{present: map[string]bool{executable: true}}
		recorded.removeFile = func(path string) error { return nil }
		err := recorded.install().RemoveInstalledProgram(io.Discard, executable, recorded.report)
		if err == nil || !strings.Contains(err.Error(), "cli.uninstall.failed") {
			t.Fatalf("RemoveInstalledProgram() error = %v, want the verification failure", err)
		}
	})
}
