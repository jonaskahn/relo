// Version command: build version and commit.
package cli

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

// Version returns the build identity printed by the version command.
func Version() string {
	return fmt.Sprintf("relo %s (%s) %s/%s", version, commit, runtime.GOOS, runtime.GOARCH)
}

func newVersionCommand(localized ui) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: localized.text("cli.version.short", nil),
		RunE:  runVersion,
	}
}

func runVersion(cmd *cobra.Command, _ []string) error {
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), Version())
	return nil
}
