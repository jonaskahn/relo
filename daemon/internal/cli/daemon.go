// Daemon command: the lifecycle subcommands.
package cli

import (
	"github.com/spf13/cobra"
)

func newDaemonCommand(localized ui) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "daemon",
		Short: localized.text("cli.daemon.short", nil),
		Args:  cobra.NoArgs,
	}
	cmd.AddCommand(
		newRunCommand(localized),
		newStartCommand(localized),
		newStopCommand(localized),
		newRestartCommand(localized),
		newStatusCommand(localized),
	)
	return cmd
}
