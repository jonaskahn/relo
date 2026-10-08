//go:build !windows

package cli

import (
	"fmt"
	"io"
	"os"
	"strings"
	"syscall"
)

func detachAttributes() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}

// RemoveInstalledProgram removes this build through the macOS bundle it runs
// from, the Linux package that owns the binary, or the npm package that
// installed it. With none of them, the operator is told what removes Relo by
// hand.
func (r Remover) RemoveInstalledProgram(out io.Writer, executable string, report Text) error {
	if bundle, inside := macBundle(executable); inside {
		return r.removeBundle(bundle, report)
	}
	if removal, installed := r.linuxPackageRemoval(report); installed {
		return removal(out)
	}
	if _, inside := npmPackageRoot(executable); inside {
		return r.removeNpmPackage(out, report)
	}
	return manualRemoval(out, report, "")
}

func (r Remover) removeBundle(bundle string, report Text) error {
	// The tray holds no port, so it is asked to quit rather than required:
	// the deletion below works whether or not it is still listening.
	r.AskTrayToQuit()
	if err := removeHomebrewLinks(bundle); err != nil {
		return err
	}
	if err := r.RemoveTree(bundle); err != nil {
		return fmt.Errorf("%s: %w", report("cli.uninstall.trash", map[string]any{"Bundle": bundle}), err)
	}
	return nil
}

func (r Remover) linuxPackageRemoval(report Text) (func(io.Writer) error, bool) {
	if r.installedBy("dpkg", "-s") {
		return func(out io.Writer) error { return r.removePackage(out, report, "apt-get", "remove", "-y") }, true
	}
	if r.installedBy("rpm", "-q") {
		return func(out io.Writer) error { return r.removePackage(out, report, "dnf", "remove", "-y") }, true
	}
	return nil, false
}

func (r Remover) installedBy(manager, question string) bool {
	if _, err := r.LookPath(manager); err != nil {
		return false
	}
	return r.RunRemoval(io.Discard, manager, question, packageName) == nil
}

func (r Remover) removePackage(out io.Writer, report Text, manager string, args ...string) error {
	command := elevated(manager, append(args, packageName)...)
	if err := r.RunRemoval(out, command[0], command[1:]...); err == nil {
		return nil
	}
	return manualRemoval(out, report, strings.Join(command, " "))
}

func elevated(name string, args ...string) []string {
	if os.Geteuid() == 0 {
		return append([]string{name}, args...)
	}
	return append([]string{"sudo", name}, args...)
}

func (r Remover) removeNpmPackage(out io.Writer, report Text) error {
	command := []string{"npm", "uninstall", "-g", packageName}
	if _, err := r.LookPath("npm"); err != nil {
		return manualRemoval(out, report, strings.Join(command, " "))
	}
	if err := r.SpawnDetached(command[0], command[1:]...); err != nil {
		return manualRemoval(out, report, strings.Join(command, " "))
	}
	return nil
}
