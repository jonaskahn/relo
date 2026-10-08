// Autostart command: enabling and disabling launch at login.
package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/jonaskahn/relo/internal/platform/autostart"
)

func newAutostartCommand(localized ui) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "autostart",
		Short: localized.text("cli.autostart.short", nil),
		Args:  cobra.NoArgs,
	}
	cmd.AddCommand(
		&cobra.Command{
			Use:   "enable",
			Short: localized.text("cli.autostart.enable.short", nil),
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, _ []string) error {
				return runAutostartToggle(cmd, true, localized)
			},
		},
		&cobra.Command{
			Use:   "disable",
			Short: localized.text("cli.autostart.disable.short", nil),
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, _ []string) error {
				return runAutostartToggle(cmd, false, localized)
			},
		},
	)
	return cmd
}

func runAutostartToggle(cmd *cobra.Command, enable bool, localized ui) error {
	exe, err := autostart.Executable()
	if err != nil {
		return fmt.Errorf("resolve the relo executable: %w", err)
	}
	manager := autostart.New()
	if enable {
		if err := manager.Enable(exe); err != nil {
			return err
		}
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), localized.text("cli.autostart.added", nil))
		return nil
	}
	if err := manager.Disable(); err != nil {
		return err
	}
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), localized.text("cli.autostart.removed", nil))
	return nil
}
