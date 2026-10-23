// Program removal is the last step of an uninstall: Relo removes itself
// through the channel that installed it, because only that channel knows every
// file it owns. The channel comes from the same detector the update notice
// uses, so updating and removing can never disagree.
package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/jonaskahn/relo/internal/updates"
)

// Text renders one message, which is what the removal needs to report itself
// without carrying the whole language of this run.
type Text func(id string, data map[string]any) string

const (
	packageName    = "relo"
	npmPackageName = "@jonaskahn/relo"
	npmLauncher    = "relo.mjs"
	dataHandled    = "/DATA=handled"
)

// bundleMacOS is the directory a macOS application bundle keeps its executable
// in, and bundleExtension is what ends the name of the bundle holding it.
const (
	bundleMacOS     = "MacOS"
	bundleExtension = ".app"
)

const trayBundleID = "com.relo.tray"

var homebrewLinks = []string{"/opt/homebrew/bin/" + packageName, "/usr/local/bin/" + packageName}

// Remover removes the program through the channel that installed it. Its
// fields are the effects an uninstall reaches past this process, held on the
// struct so a test can drive every channel without touching a real package
// manager, app bundle, or npm installation.
type Remover struct {
	// GOOS is the operating system whose installation layout applies.
	GOOS string
	// LookPath reports whether a command this host provides is there.
	LookPath func(string) (string, error)
	// Exists reports whether a path is still present.
	Exists func(string) bool
	// RemoveFile deletes one file, treating an already absent file as success.
	RemoveFile func(string) error
	// RemoveTree deletes a macOS application bundle.
	RemoveTree func(string) error
	// SetupUninstaller returns the command the Windows setup registered.
	SetupUninstaller func() (string, bool)
	// RemoveRunningProgram deletes the executable itself, using the delayed
	// deletion Windows needs while the image is still mapped.
	RemoveRunningProgram func(string) error
	// RemoveFromUserPath drops a script install directory from the user PATH.
	RemoveFromUserPath func(string) error
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
	r := Remover{
		GOOS:                 runtime.GOOS,
		LookPath:             exec.LookPath,
		Exists:               exists,
		RemoveFile:           removeFile,
		RemoveTree:           os.RemoveAll,
		SetupUninstaller:     findSetupUninstaller,
		RemoveRunningProgram: removeRunningProgram,
		RemoveFromUserPath:   removeFromUserPath,
		AskTrayToQuit:        askTrayToQuit,
		SpawnDetached:        spawnDetached,
		RunRemoval:           runRemoval,
	}
	return r
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func removeFile(path string) error {
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func askTrayToQuit() {
	_ = exec.Command("osascript", "-e", `tell application id "`+trayBundleID+`" to quit`).Run()
}

func spawnDetached(name string, args ...string) error {
	command := exec.Command(name, args...)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	command.SysProcAttr = detachAttributes()
	if err := command.Start(); err != nil {
		return err
	}
	return command.Process.Release()
}

func runRemoval(out io.Writer, name string, args ...string) error {
	command := exec.Command(name, args...)
	command.Stdout = out
	command.Stderr = out
	return command.Run()
}

func detachProgram(out io.Writer, report Text) error {
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve the relo executable: %w", err)
	}
	return NewRemover().RemoveInstalledProgram(out, executable, report)
}

// RemoveInstalledProgram removes the build that installed executable and
// verifies what it can. A system package is reported rather than removed:
// its package manager needs an explicit, privileged command from the operator.
func (r Remover) RemoveInstalledProgram(out io.Writer, executable string, report Text) error {
	channel := r.installerFor(executable)
	_, _ = fmt.Fprintln(out, report("cli.uninstall.channel", map[string]any{"Channel": channel}))

	switch channel {
	case updates.InstallerSparkle:
		return r.removeBundle(out, executable, report)
	case updates.InstallerNPM:
		return r.removeNpmPackage(out, executable, report)
	case updates.InstallerBrew:
		return r.removeBrewFormula(out, executable, report)
	case updates.InstallerShell, updates.InstallerPowerShell:
		return r.removeScriptInstall(out, executable, report)
	case updates.InstallerSetup:
		return r.removeSetup(out, r.SetupUninstaller, executable, report)
	default:
		return r.removeManual(out, executable, report)
	}
}

func (r Remover) installerFor(executable string) string {
	goos := r.goos()
	channel := updates.InstallerFor(executable, goos)
	if channel == updates.InstallerUnknown && goos == "windows" {
		// Windows paths are case-insensitive, and the setup lives under the
		// path the installer chose. The detector compares slash paths, so ask
		// the registry as a second witness before calling it manual.
		if r.SetupUninstaller != nil {
			if _, found := r.SetupUninstaller(); found {
				return updates.InstallerSetup
			}
		}
	}
	return channel
}

func (r Remover) goos() string {
	if r.GOOS != "" {
		return r.GOOS
	}
	return runtime.GOOS
}

func (r Remover) removeBundle(out io.Writer, executable string, report Text) error {
	bundle, inside := macBundle(executable)
	if !inside {
		return r.removeManual(out, executable, report)
	}
	// The tray holds no port, so it is asked to quit rather than required:
	// the deletion below works whether or not it is still listening.
	r.AskTrayToQuit()
	if err := removeHomebrewLinks(bundle); err != nil {
		return err
	}
	if err := r.RemoveTree(bundle); err != nil {
		return fmt.Errorf("%s: %w", report("cli.uninstall.trash", map[string]any{"Bundle": bundle}), err)
	}
	return r.verifyGone(out, report, bundle)
}

func (r Remover) removeScriptInstall(out io.Writer, executable string, report Text) error {
	receipt := updates.ScriptReceiptPath(executable)
	command := r.removeCommand(executable)
	_, _ = fmt.Fprintln(out, report("cli.uninstall.command", map[string]any{"Command": command}))
	if r.goos() == "windows" {
		if r.RemoveFromUserPath == nil {
			return fmt.Errorf("no Windows PATH editor is configured")
		}
		if err := r.RemoveFromUserPath(filepath.Dir(executable)); err != nil {
			return err
		}
	}
	if err := r.RemoveFile(receipt); err != nil {
		return fmt.Errorf("remove %s: %w", receipt, err)
	}
	if r.goos() == "windows" {
		if r.RemoveRunningProgram == nil {
			return fmt.Errorf("no Windows delayed deleter is configured")
		}
		if err := r.RemoveRunningProgram(executable); err != nil {
			return err
		}
		_, _ = fmt.Fprintln(out, report("cli.uninstall.self_delete", map[string]any{"Path": executable}))
		return nil
	}
	if err := r.RemoveFile(executable); err != nil {
		r.printInstruction(out, report, command)
		return err
	}
	return r.verifyGone(out, report, executable, receipt)
}

func (r Remover) removeNpmPackage(out io.Writer, executable string, report Text) error {
	root, found := npmPackageRoot(executable)
	command := []string{"npm", "uninstall", "-g", npmPackageName}
	if _, err := r.LookPath("npm"); err != nil {
		r.printInstruction(out, report, strings.Join(command, " "))
		return fmt.Errorf("npm is not on PATH: %w", err)
	}
	if r.goos() == "windows" {
		if err := r.SpawnDetached(command[0], command[1:]...); err != nil {
			r.printInstruction(out, report, strings.Join(command, " "))
			return err
		}
		_, _ = fmt.Fprintln(out, report("cli.uninstall.self_delete", map[string]any{"Path": executable}))
		return nil
	}
	if err := r.RunRemoval(out, command[0], command[1:]...); err != nil {
		r.printInstruction(out, report, strings.Join(command, " "))
		return err
	}
	if found {
		return r.verifyGone(out, report, root)
	}
	return nil
}

func (r Remover) removeBrewFormula(out io.Writer, executable string, report Text) error {
	formula := brewFormula(executable)
	if _, err := r.LookPath("brew"); err != nil {
		r.printInstruction(out, report, "brew uninstall "+formula)
		return fmt.Errorf("brew is not on PATH: %w", err)
	}
	if err := r.RunRemoval(out, "brew", "uninstall", formula); err != nil {
		r.printInstruction(out, report, "brew uninstall "+formula)
		return err
	}
	if r.RunRemoval(io.Discard, "brew", "list", "--formula", formula) == nil {
		return fmt.Errorf("%s", report("cli.uninstall.failed", map[string]any{"Path": formula}))
	}
	_, _ = fmt.Fprintln(out, report("cli.uninstall.verified", map[string]any{"Path": formula}))
	return nil
}

// removeSetup removes a packaged install. The Windows setup owns an
// uninstaller and is handed to it. A Linux package manager needs privilege and
// an explicit operator command, so that channel is printed, not run.
func (r Remover) removeSetup(out io.Writer, setup func() (string, bool), executable string, report Text) error {
	if r.goos() == "windows" {
		uninstaller, found := "", false
		if setup != nil {
			uninstaller, found = setup()
		}
		if !found {
			r.printInstruction(out, report, "Remove Relo from Settings > Apps")
			return fmt.Errorf("no Relo uninstaller is registered")
		}
		if err := r.SpawnDetached(uninstaller, dataHandled); err != nil {
			r.printInstruction(out, report, uninstaller+" "+dataHandled)
			return err
		}
		_, _ = fmt.Fprintln(out, report("cli.uninstall.self_delete", map[string]any{"Path": executable}))
		return nil
	}
	command := "sudo apt remove " + packageName
	if r.installedBy("rpm", "-q") {
		command = "sudo dnf remove " + packageName
	}
	r.printInstruction(out, report, command)
	return nil
}

func (r Remover) installedBy(manager, question string) bool {
	if _, err := r.LookPath(manager); err != nil {
		return false
	}
	return r.RunRemoval(io.Discard, manager, question, packageName) == nil
}

func (r Remover) removeManual(out io.Writer, executable string, report Text) error {
	command := r.removeCommand(executable)
	_, _ = fmt.Fprintln(out, report("cli.uninstall.command", map[string]any{"Command": command}))
	if r.goos() == "windows" {
		if r.RemoveRunningProgram == nil {
			return fmt.Errorf("no Windows delayed deleter is configured")
		}
		if err := r.RemoveRunningProgram(executable); err != nil {
			return err
		}
		_, _ = fmt.Fprintln(out, report("cli.uninstall.self_delete", map[string]any{"Path": executable}))
		return nil
	}
	if err := r.RemoveFile(executable); err != nil {
		r.printInstruction(out, report, command)
		return err
	}
	return r.verifyGone(out, report, executable)
}

func (r Remover) verifyGone(out io.Writer, report Text, paths ...string) error {
	for _, path := range paths {
		if r.Exists(path) {
			return fmt.Errorf("%s", report("cli.uninstall.failed", map[string]any{"Path": path}))
		}
		_, _ = fmt.Fprintln(out, report("cli.uninstall.verified", map[string]any{"Path": path}))
	}
	return nil
}

func (r Remover) printInstruction(out io.Writer, report Text, command string) {
	_, _ = fmt.Fprintln(out, report("cli.uninstall.run", map[string]any{"Command": command}))
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

func brewFormula(executable string) string {
	resolved := executable
	if target, err := filepath.EvalSymlinks(executable); err == nil {
		resolved = target
	}
	if strings.Contains(filepath.ToSlash(resolved), "/Cellar/relo-beta/") {
		return "relo-beta"
	}
	return packageName
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
