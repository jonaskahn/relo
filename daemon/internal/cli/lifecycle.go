// Lifecycle commands: start, stop, and restart.
package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/jonaskahn/relo/internal/platform"
)

func newStartCommand(localized ui) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "start",
		Short: localized.text("cli.daemon.start.short", nil),
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStart(cmd, args, localized)
		},
	}
	cmd.Flags().Int("port", 0, localized.text("cli.daemon.start.port", nil))
	return cmd
}

func newStopCommand(localized ui) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stop",
		Short: localized.text("cli.daemon.stop.short", nil),
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStop(cmd, args, localized)
		},
	}
	cmd.Flags().Bool("force", false, localized.text("cli.daemon.stop.force", nil))
	return cmd
}

func newRestartCommand(localized ui) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "restart",
		Short: localized.text("cli.daemon.restart.short", nil),
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRestart(cmd, args, localized)
		},
	}
	cmd.Flags().Int("port", 0, localized.text("cli.daemon.restart.port", nil))
	cmd.Flags().Bool("force", false, localized.text("cli.daemon.restart.force", nil))
	return cmd
}

func runStart(cmd *cobra.Command, _ []string, localized ui) error {
	home, err := homeFromFlags(cmd)
	if err != nil {
		return err
	}
	ctx := cmd.Context()
	out := cmd.OutOrStdout()
	port := flagPort(cmd)
	if published, running := platform.RunningInstanceOn(ctx, home, port); running {
		_, _ = fmt.Fprintln(out, localized.text("cli.daemon.start.already", map[string]any{"Addr": published.Address}))
		return nil
	}
	published, startupLog, err := platform.StartOrAttach(ctx, home, port)
	if err != nil {
		recordStartFailure(home, startupLog, err)
		return startFailure(localized, home, err)
	}
	if startupLog == "" {
		_, _ = fmt.Fprintln(out, localized.text("cli.daemon.start.already", map[string]any{"Addr": published.Address}))
		return nil
	}
	_, _ = fmt.Fprintln(out, localized.text("cli.daemon.start.started", map[string]any{"Addr": published.Address}))
	return nil
}

func runStop(cmd *cobra.Command, _ []string, localized ui) error {
	home, err := homeFromFlags(cmd)
	if err != nil {
		return err
	}
	ctx := cmd.Context()
	out := cmd.OutOrStdout()
	force := flagBool(cmd, "force")
	previous, found := platform.ReadRuntime(home)
	if _, running := platform.RunningInstanceOn(ctx, home, 0); !running && !force {
		if found {
			if err := platform.WaitForShutdown(ctx, previous); err != nil {
				return err
			}
			platform.RemoveInstanceRuntime(home, previous)
		}
		_, _ = fmt.Fprintln(out, localized.text("cli.daemon.stop.not_running", nil))
		return nil
	}
	if err := quiesce(ctx, home, 0, force); err != nil {
		return err
	}
	if force {
		// A forced stop is what an install runs before it replaces the
		// binaries, so a port another program holds is reported rather than
		// ended: an automatic update must never kill a process that is not
		// Relo.
		if err := platform.ForeignListener(home, 0); err != nil {
			return err
		}
	}
	_, _ = fmt.Fprintln(out, localized.text("cli.daemon.stop.stopped", nil))
	return nil
}

func runRestart(cmd *cobra.Command, _ []string, localized ui) error {
	home, err := homeFromFlags(cmd)
	if err != nil {
		return err
	}
	ctx := cmd.Context()
	out := cmd.OutOrStdout()
	port := flagPort(cmd)
	force := flagBool(cmd, "force")
	published, running := platform.RunningInstanceOn(ctx, home, port)
	if running && port == 0 {
		port = published.Port()
	}
	if running || force {
		if err := quiesce(ctx, home, port, force); err != nil {
			return err
		}
	}
	restarted, startupLog, err := platform.StartOrAttach(ctx, home, port)
	if err != nil {
		recordStartFailure(home, startupLog, err)
		return startFailure(localized, home, err)
	}
	_, _ = fmt.Fprintln(out, localized.text("cli.daemon.restart.restarted", map[string]any{"Addr": restarted.Address}))
	return nil
}

func quiesce(ctx context.Context, home string, port int, force bool) error {
	published, running := platform.RunningInstanceOn(ctx, home, port)
	if running {
		if port == 0 {
			port = published.Port()
		}
		if err := platform.StopRunning(ctx, home, published); err != nil && !force {
			return err
		}
	}
	if force {
		if err := platform.FreeOwnPorts(home, port); err != nil {
			return err
		}
	}
	if published.InstanceID != "" {
		platform.RemoveInstanceRuntime(home, published)
	}
	return nil
}

func flagPort(cmd *cobra.Command) int {
	port, _ := cmd.Flags().GetInt("port")
	return port
}

func flagBool(cmd *cobra.Command, name string) bool {
	flag, _ := cmd.Flags().GetBool(name)
	return flag
}

func recordStartFailure(home, startupLog string, cause error) {
	// A start refused before it spawned anything, such as a port another
	// program holds, has no transcript yet. The Windows login item and the
	// shortcut have no console to print the error to, so without a file the
	// failure would leave no trace.
	if startupLog == "" {
		created, file, err := platform.CreateStartupLog(home)
		if err != nil {
			return
		}
		_ = file.Close()
		startupLog = created
	}
	_ = platform.AppendStartupLog(startupLog, cause.Error())
}

func startFailure(localized ui, home string, cause error) error {
	return fmt.Errorf("%s: %w (%s)",
		localized.text("cli.daemon.start.failed", map[string]any{"Detail": cause.Error()}),
		cause,
		localized.text("cli.daemon.start.log", map[string]any{"Path": platform.DaemonLogPath(home)}))
}
