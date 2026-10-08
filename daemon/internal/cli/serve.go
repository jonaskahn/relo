// Run command: serving the daemon in the foreground.
package cli

import (
	"context"
	"log/slog"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/jonaskahn/relo/internal/config"
	"github.com/jonaskahn/relo/internal/desktop"
	"github.com/jonaskahn/relo/internal/platform"
)

func newRunCommand(localized ui) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run",
		Short: localized.text("cli.daemon.run.short", nil),
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServe(cmd, args, localized)
		},
	}
	cmd.Flags().Int("port", 0, localized.text("cli.daemon.run.port", nil))
	cmd.Flags().String("startup-log", "", "boot transcript this run tees its boot lines into")
	_ = cmd.Flags().MarkHidden("startup-log")
	return cmd
}

func runServe(cmd *cobra.Command, _ []string, localized ui) error {
	home, err := homeFromFlags(cmd)
	if err != nil {
		return err
	}
	port, _ := cmd.Flags().GetInt("port")
	startupLog, _ := cmd.Flags().GetString("startup-log")
	language, err := languageFromFlags(cmd)
	if err != nil {
		return err
	}
	resolution, err := platform.ResolveLanguage(home, language)
	if err != nil {
		return err
	}
	serving, err := openServeSession(home, port, startupLog)
	if err != nil {
		return err
	}

	return serveTray(cmd, home, serving, localized, startupLog, resolution.Language)
}

func serveTray(cmd *cobra.Command, home string, serving *serveSession, localized ui, startupLog, language string) error {
	ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	supervisor := platform.NewSupervisor(platform.SupervisorOptions{
		Home: home, Port: serving.port, Language: language,
		Version: version, StartupLog: startupLog, Logger: serving.logger,
	})
	owned, err := supervisor.Start()
	if err != nil {
		reportStartupError(err)
		return err
	}
	if !owned {
		// Another process already serves this home; a second icon would only
		// fight it for the menu bar.
		return nil
	}
	return serving.runDesktop(ctx, home, supervisor, localized, startupLog, language)
}

func (s *serveSession) runDesktop(ctx context.Context, home string, supervisor *platform.Supervisor, localized ui, startupLog, language string) error {
	return desktop.Run(ctx, s.desktopOptions(home, supervisor, localized, startupLog, language))
}

type serveSession struct {
	port   int
	logger *slog.Logger
	token  string
	config config.Config
}

func openServeSession(home string, port int, startupLog string) (*serveSession, error) {
	// The tray needs the same picture of the configuration the run serves
	// with: the logger it logs through and the address the dashboard link
	// points at. A configuration that does not parse is the run's to report;
	// the app starts with defaults and shows the failure.
	cfg := config.DefaultConfig()
	if loaded, err := config.LoadConfig(config.ConfigPath(home), nil); err == nil {
		cfg = *loaded
	}
	if port == 0 {
		port = cfg.Server.Port
	}
	token, err := platform.EnsureAdminToken(home)
	if err != nil {
		return nil, err
	}
	logger, err := platform.NewLogger(home, cfg.System.Logging.SlogLevel(), startupLog)
	if err != nil {
		return nil, err
	}
	return &serveSession{port: port, logger: logger, token: token, config: cfg}, nil
}

func (s *serveSession) desktopOptions(home string, supervisor *platform.Supervisor, localized ui, startupLog, language string) desktop.Options {
	return desktop.Options{
		Home:        home,
		Control:     supervisor,
		Version:     version,
		URL:         desktop.DashboardURL(s.config.Server.Bind, s.port),
		AdminToken:  s.token,
		Autostart:   s.config.System.Autostart,
		Language:    language,
		Catalogs:    localized.catalogs,
		SetLanguage: func(tag string) error { return storeLanguage(home, tag) },
		LogPath:     platform.DaemonLogPath(home),
		Logger:      s.logger,
		Source:      NewDesktopActivitySource(home, s.logger),
	}
}
