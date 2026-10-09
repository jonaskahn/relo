// Package cli implements the relo command line. It is the composition
// root: every concrete implementation is built and injected here.
//
// Commands read their own flags with the error discarded: cobra has
// already typed every value it parsed, so reading one back cannot fail.
// Everything else returns its error to the caller.
package cli

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/jonaskahn/relo/internal/config"
)

// version and commit are injected through -ldflags at build time.
var (
	version = "0.1.0"
	commit  = "none"
)

const homeFlag = "home"

type fail struct {
	code int
	err  error
}

// Error returns the failure message.
func (f fail) Error() string {
	return f.err.Error()
}

// Unwrap exposes the underlying cause.
func (f fail) Unwrap() error {
	return f.err
}

// ExitCode returns the process exit code for this failure.
func (f fail) ExitCode() int {
	return f.code
}

// NewRootCommand builds the relo command tree in the given language. An
// empty language is resolved from the environment, the state directory,
// and the system locale.
func NewRootCommand(language string) (*cobra.Command, error) {
	home, err := config.ReloHome()
	if err != nil {
		return nil, err
	}
	localized, err := loadUI(home, language)
	if err != nil {
		return nil, err
	}
	return newRootCommand(localized), nil
}

func newRootCommand(localized ui) *cobra.Command {
	root := &cobra.Command{
		Use:           "relo",
		Short:         localized.text("cli.root.short", nil),
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().String(homeFlag, "", localized.text("cli.root.home", nil))
	root.PersistentFlags().String(langFlag, "", localized.text("cli.root.lang", nil))
	root.AddCommand(
		newAutostartCommand(localized),
		newDaemonCommand(localized),
		newUninstallCommand(localized),
		newVersionCommand(localized),
	)
	localizeBuiltins(root, localized)
	return root
}

// Execute runs the command line. The language is resolved before the tree
// is built, because every command renders its help in it, and a language
// Relo cannot speak is reported rather than quietly dropped. Commands that
// print their own report return a silent failure, so nothing is written
// twice.
func Execute() error { return ExecuteWithShutdown(nil) }

// ExecuteWithShutdown installs the executable's final shutdown deadline hook.
func ExecuteWithShutdown(onStopping func()) error {
	// A Windows launch from Explorer, a shortcut, or the login item owns no
	// console; attaching the parent one first keeps terminal output visible.
	EnsureConsole()
	// Explorer starts this binary from the shortcut and the login item, and
	// cobra's guard would answer that launch by printing a hint to a console
	// that does not exist and exiting after five seconds.
	cobra.MousetrapHelpText = ""
	flaggedHome, language := preflight(os.Args[1:])
	home, err := preflightHome(flaggedHome)
	if err != nil {
		return err
	}
	localized, err := loadUI(home, language)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "relo:", err)
		return err
	}
	root := newRootCommand(localized)
	root.SetContext(context.WithValue(context.Background(), shutdownHookKey{}, onStopping))
	err = root.Execute()
	if err == nil {
		return nil
	}
	var reported fail
	if !errors.As(err, &reported) {
		_, _ = fmt.Fprintln(root.ErrOrStderr(), "relo:", err)
	}
	return err
}

// ExitCode maps a command failure to the process exit code to use.
func ExitCode(err error) int {
	var reported fail
	if errors.As(err, &reported) {
		return reported.ExitCode()
	}
	if err != nil {
		return 1
	}
	return 0
}

func homeFromFlags(cmd *cobra.Command) (string, error) {
	flagged, _ := cmd.Flags().GetString(homeFlag)
	if flagged != "" {
		return flagged, nil
	}
	return config.ReloHome()
}

func languageFromFlags(cmd *cobra.Command) (string, error) {
	return cmd.Flags().GetString(langFlag)
}

type shutdownHookKey struct{}
