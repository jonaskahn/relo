// Dashboard opening after successful lifecycle actions.
package cli

import (
	"fmt"
	"net"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/jonaskahn/relo/internal/desktop"
)

func openCommandDashboard(cmd *cobra.Command, address string) {
	openDashboard(address, func(err error) {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "could not open the dashboard: %v\n", err)
	})
}

func openDashboard(address string, warn func(error)) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		warn(err)
		return
	}
	number, err := strconv.Atoi(port)
	if err != nil {
		warn(err)
		return
	}
	if err := desktop.Open(desktop.DashboardURL(host, number)); err != nil {
		warn(err)
	}
}
