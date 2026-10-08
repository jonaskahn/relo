// Uninstall takes Relo off a machine: the running service is stopped, the
// login items are unregistered, the data goes or stays as the operator asked,
// and the program itself is handed to whatever installed it.
package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jonaskahn/relo/internal/platform"
	"github.com/jonaskahn/relo/internal/platform/autostart"
)

// The steps of an uninstall that reach past this process, held as variables so
// a test can drive the whole flow without deleting a real state directory,
// waiting on a terminal, or asking a package manager to uninstall Relo.
var (
	// WipeState removes a state directory and the credentials the operating
	// system keychain holds for it.
	WipeState = platform.WipeState
	// AskWipeData reads the answer to the data question.
	AskWipeData = PromptWipeData
	// AskOnTerminal reports where to ask that question, and whether anyone is
	// there to answer it.
	AskOnTerminal = findTerminal
	// RemoveProgram hands the program to whatever installed this build. It
	// reports what an operator has to do when nothing on this machine removes
	// Relo, so it renders its own messages.
	RemoveProgram = func(out io.Writer, text func(string, map[string]any) string) error {
		return detachProgram(out, text)
	}
)

// ErrBothDataFlags reports the two data flags an operator cannot mean at once.
var ErrBothDataFlags = errors.New("ask for one of --keep-data or --wipe")

func newUninstallCommand(localized ui) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: localized.text("cli.uninstall.short", nil),
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUninstall(cmd, args, localized)
		},
	}
	cmd.Flags().Bool("keep-data", false, localized.text("cli.uninstall.keep_data", nil))
	cmd.Flags().Bool("wipe", false, localized.text("cli.uninstall.wipe", nil))
	cmd.Flags().Bool("prepare", false, localized.text("cli.uninstall.prepare", nil))
	_ = cmd.Flags().MarkHidden("prepare")
	return cmd
}

func stopUninstallServices(cmd *cobra.Command, home string) error {
	if err := removeService(cmd.Context(), home); err != nil {
		return err
	}
	return autostart.New().Disable()
}

func runUninstall(cmd *cobra.Command, _ []string, localized ui) error {
	home, err := homeFromFlags(cmd)
	if err != nil {
		return err
	}
	keep, wipe := flagBool(cmd, "keep-data"), flagBool(cmd, "wipe")
	if keep && wipe {
		return ErrBothDataFlags
	}
	// A native remover runs this before it deletes the program, so the program
	// stays where it is: it runs again for the operator.
	prepare := flagBool(cmd, "prepare")
	out := cmd.OutOrStdout()

	if err := stopUninstallServices(cmd, home); err != nil {
		return err
	}
	data, err := decideData(cmd, localized, keep, wipe, prepare)
	if err != nil {
		return err
	}
	if err := settleData(out, home, data, localized); err != nil {
		return err
	}
	if prepare {
		return nil
	}
	_, _ = fmt.Fprintln(out, localized.text("cli.uninstall.removing", nil))
	return RemoveProgram(out, localized.text)
}

func removeService(ctx context.Context, home string) error {
	if err := quiesce(ctx, home, 0, true); err != nil {
		return err
	}
	return platform.ForceFreePorts(home, 0)
}

type dataOutcome int

const (
	wipeState dataOutcome = iota
	keepState
)

func settleData(out io.Writer, home string, outcome dataOutcome, localized ui) error {
	if outcome == keepState {
		_, _ = fmt.Fprintln(out, localized.text("cli.uninstall.kept", map[string]any{"Home": home}))
		return nil
	}
	if err := WipeState(home); err != nil {
		return err
	}
	_, _ = fmt.Fprintln(out, localized.text("cli.uninstall.wiped", nil))
	return nil
}

func decideData(cmd *cobra.Command, localized ui, keep, wipe, prepare bool) (dataOutcome, error) {
	switch {
	case wipe:
		return wipeState, nil
	case keep || prepare:
		return keepState, nil
	}
	in, answerable := AskOnTerminal()
	if !answerable {
		return keepState, nil
	}
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), localized.text("cli.uninstall.ask_data", nil))
	answer, err := AskWipeData(in)
	if err != nil {
		return keepState, err
	}
	if answer {
		return wipeState, nil
	}
	return keepState, nil
}

// PromptWipeData reads the answer to the data question. Anything but an
// explicit yes keeps the data: a mistyped answer must never delete a state
// directory.
func PromptWipeData(in io.Reader) (bool, error) {
	line, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, err
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes", nil
}
