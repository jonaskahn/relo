// Program removal is the last step of an uninstall: Relo hands itself to
// whatever installed it, because only that can delete the files it owns while
// this process is still running. The flavor is read from the running
// executable, so the same command works from a package, from npm, and from
// inside an app bundle.
package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ErrManualRemoval reports a build nothing on this machine removes on its own,
// so the operator is told what to do by hand.
var ErrManualRemoval = errors.New("nothing on this machine removes relo")

// Text renders one message, which is what the removal needs to report itself
// without carrying the whole language of this run.
type Text func(id string, data map[string]any) string

const packageName = "relo"

const npmLauncher = "relo.mjs"

// bundleMacOS is the directory a macOS application bundle keeps its executable
// in, and bundleExtension is what ends the name of the bundle holding it.
const (
	bundleMacOS     = "MacOS"
	bundleExtension = ".app"
)

const trayBundleID = "com.relo.tray"

var homebrewLinks = []string{"/opt/homebrew/bin/" + packageName, "/usr/local/bin/" + packageName}

// Remover deletes the installed program through whatever installed it: the
// macOS bundle, the Linux package manager, the npm package, or the Windows
// uninstaller. Its fields are the processes an uninstall spawns, held on the
// struct so a test can drive the removal flavors without a package manager,
// an npm, or an app bundle. NewRemover fills in the real implementations.
type Remover struct {
	// LookPath reports whether a command this host provides is there.
	LookPath func(string) (string, error)
	// RemoveTree deletes a macOS application bundle.
	RemoveTree func(string) error
	// AskTrayToQuit asks the tray app to close before its bundle is deleted.
	AskTrayToQuit func()
	// SpawnDetached starts a command that may delete the running executable,
	// giving it a process group of its own and letting go of it rather than
	// waiting on a program that outlives this one.
	SpawnDetached func(name string, args ...string) error
	// RunRemoval runs a command that has to answer before this process exits.
	RunRemoval func(out io.Writer, name string, args ...string) error
}

// NewRemover builds a Remover that drives the real package managers and
// launchers of this host.
func NewRemover() Remover {
	return Remover{
		LookPath:   exec.LookPath,
		RemoveTree: os.RemoveAll,
		AskTrayToQuit: func() {
			_ = exec.Command("osascript", "-e", `tell application id "`+trayBundleID+`" to quit`).Run()
		},
		SpawnDetached: func(name string, args ...string) error {
			command := exec.Command(name, args...)
			command.Stdout = os.Stdout
			command.Stderr = os.Stderr
			command.SysProcAttr = detachAttributes()
			if err := command.Start(); err != nil {
				return err
			}
			return command.Process.Release()
		},
		RunRemoval: func(out io.Writer, name string, args ...string) error {
			command := exec.Command(name, args...)
			command.Stdout = out
			command.Stderr = out
			return command.Run()
		},
	}
}

func detachProgram(out io.Writer, report Text) error {
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve the relo executable: %w", err)
	}
	return NewRemover().RemoveInstalledProgram(out, executable, report)
}

func manualRemoval(out io.Writer, report Text, command string) error {
	if command != "" {
		_, _ = fmt.Fprintln(out, report("cli.uninstall.run", map[string]any{"Command": command}))
	} else {
		_, _ = fmt.Fprintln(out, report("cli.uninstall.manual", nil))
	}
	return ErrManualRemoval
}

func npmPackageRoot(executable string) (string, bool) {
	for dir := filepath.Dir(executable); ; dir = filepath.Dir(dir) {
		if isNpmPackageRoot(dir) {
			return dir, true
		}
		if parent := filepath.Dir(dir); parent == dir {
			return "", false
		}
	}
}

func isNpmPackageRoot(dir string) bool {
	body, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil || !strings.Contains(string(body), `"`+packageName+`"`) {
		return false
	}
	_, err = os.Stat(filepath.Join(dir, "bin", npmLauncher))
	return err == nil
}

func macBundle(executable string) (string, bool) {
	if filepath.Base(filepath.Dir(executable)) != bundleMacOS {
		return "", false
	}
	bundle := filepath.Dir(filepath.Dir(filepath.Dir(executable)))
	if !strings.HasSuffix(bundle, bundleExtension) {
		return "", false
	}
	return bundle, true
}

func removeHomebrewLinks(bundle string) error {
	for _, link := range homebrewLinks {
		err := os.Remove(link)
		if err == nil || errors.Is(err, os.ErrNotExist) {
			continue
		}
		if !pointsInside(link, bundle) {
			continue
		}
		return fmt.Errorf("remove the Homebrew link %s: %w", link, err)
	}
	return nil
}

func pointsInside(link, bundle string) bool {
	resolved, err := filepath.EvalSymlinks(link)
	if err != nil {
		return false
	}
	inside, err := filepath.Rel(bundle, resolved)
	if err != nil {
		return false
	}
	return inside != ".." && !strings.HasPrefix(inside, ".."+string(filepath.Separator))
}
