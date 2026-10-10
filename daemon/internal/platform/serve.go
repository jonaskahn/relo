// Package platform is Relo's composition root. Every concrete implementation is
// built here, once and explicitly, and injected into the surfaces that run:
// the daemon, the background workers, and the offline management session a
// command or the tray runs against.
package platform

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/access"
	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	"github.com/jonaskahn/relo/internal/config"
	// Login items are stored next to the tray code; the daemon uses them
	// without importing the desktop surface.
	"github.com/jonaskahn/relo/internal/platform/autostart"
	"github.com/jonaskahn/relo/internal/server"
)

const (
	// upstreamTimeout bounds a discovery or probe call, which answers with
	// one body. Inference uses NewInferenceClient: a stream that keeps
	// sending must run until the provider is done, so its reads enforce the
	// call wait instead of a total client timeout.
	upstreamTimeout = 180 * time.Second
	instanceIDBytes = 16
	daemonLogName   = "daemon.log"
)

// Options configures one daemon run.
type Options struct {
	Home   string
	Port   int
	Logger *slog.Logger
	// Language is the language this run was asked for, when the command
	// line named one. The rest of the resolution happens here, because
	// only the daemon reads the state directory the setting lives in.
	Language string
	// Version is the build version this run reports to a status request.
	Version string
	// StartupLog is the boot transcript this run tees its boot lines into
	// until it finishes starting. SpawnDaemon sets it through the
	// --startup-log flag; a foreground run leaves it empty.
	StartupLog string
	// OnReady, when set, is called once with every address the process
	// bound, before the daemon answers its first request. The tray app
	// learns the live address of a port-0 listener this way.
	OnReady func(addrs server.Addrs)
	// OnFollowed, when set, is called when another process already serves
	// the state directory and this run leaves it alone. A supervisor uses it
	// to tell a run that owns nothing from one that just failed.
	OnFollowed func()
	// Headless reports that this run is the daemon the lifecycle commands
	// own. Such a run publishes where it listens and accepts
	// `relo daemon stop`.
	Headless bool
	// Stop, when set, routes API shutdown through the process supervisor.
	Stop func()
	// Restart cycles a supervised run without spawning a detached helper.
	Restart func(force bool)

	// instanceID and shutdown are filled in by Serve for a headless run.
	instanceID string
	shutdown   func()
}

// Serve runs the daemon until ctx is cancelled. It owns the state
// directory, the database, the credential pools, and the listener.
func Serve(ctx context.Context, options Options) error {
	cfg, logger, err := prepareServe(options)
	if err != nil {
		return err
	}
	defer FinishStartupLog()
	reconcileAutostart(options, cfg, logger)
	// A headless run publishes where it listens and how to end it, so a
	// daemon no terminal owns can still be found and stopped. The claim
	// is held until that publication, so the other login agent waits and
	// follows instead of binding the same ports.
	ctx, stop := context.WithCancel(ctx)
	defer stop()
	cause := observeSignals()
	defer cause.stop()
	return executeServe(ctx, options, cfg, logger, cause, stop)
}

func prepareServe(options Options) (*config.Config, *slog.Logger, error) {
	if err := config.EnsureReloHome(options.Home); err != nil {
		return nil, nil, err
	}
	cfg, err := runtimeConfig(options)
	if err != nil {
		return nil, nil, err
	}
	logger, err := serveLogger(options, cfg)
	if err != nil {
		return nil, nil, err
	}
	return cfg, logger, nil
}

func executeServe(ctx context.Context, options Options, cfg *config.Config, logger *slog.Logger, cause *signalCause, stop context.CancelFunc) error {
	release, err := claimHeadless(ctx, &options, cfg, logger, stop)
	if err != nil {
		return err
	}
	if release == nil {
		return nil
	}
	defer release()
	resolution, err := ResolveLanguage(options.Home, options.Language)
	if err != nil {
		return err
	}
	options.Language = resolution.Language
	db, err := openServeDB(options.Home, logger)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	wired, err := buildDeps(ctx, cfg, options, db, logger)
	if err != nil {
		return err
	}
	defer func() { _ = wired.deps.usage.Close() }()
	return serveWired(ctx, wired, cfg, logger, cause)
}

func serveLogger(options Options, cfg *config.Config) (*slog.Logger, error) {
	if options.Logger != nil {
		return options.Logger, nil
	}
	return NewLogger(options.Home, cfg.System.Logging.SlogLevel(), options.StartupLog)
}

func reconcileAutostart(options Options, cfg *config.Config, logger *slog.Logger) {
	if !options.Headless || !cfg.System.Autostart || testing.Testing() {
		return
	}
	if exe, resolveErr := autostart.Executable(); resolveErr != nil {
		logger.Warn("could not resolve the executable for start at login", "error", resolveErr)
	} else if err := autostart.Reconcile(exe); err != nil {
		logger.Warn("could not register the app to start at login", "error", err)
	}
}

func claimHeadless(ctx context.Context, options *Options, cfg *config.Config, logger *slog.Logger, stop context.CancelFunc) (func(), error) {
	if !options.Headless {
		return func() {}, nil
	}
	claim, followed, err := claimDaemon(ctx, options.Home, cfg)
	if err != nil {
		return nil, err
	}
	if followed {
		if options.OnFollowed != nil {
			options.OnFollowed()
		}
		return nil, nil
	}
	options.instanceID = newInstanceID()
	options.shutdown = stop
	if options.Stop != nil {
		options.shutdown = options.Stop
	}
	ready := options.OnReady
	options.OnReady = publishOnReady(*options, logger, claim, ready)
	return func() {
		claim.release()
		removeOwnRuntime(options.Home, options.instanceID)
	}, nil
}

func publishOnReady(options Options, logger *slog.Logger, claim *daemonClaim, ready func(addrs server.Addrs)) func(addrs server.Addrs) {
	return func(addrs server.Addrs) {
		// The runtime file goes out first: a caller that hears from OnReady
		// (the supervisor behind the tray) can then read the home as owned
		// and serving, and a competing process finds this run instead of
		// starting a second one.
		published := Runtime{
			Address:    addrs.Management,
			InstanceID: options.instanceID,
			PID:        os.Getpid(),
			Listeners:  boundListeners(addrs),
		}
		if err := publishRuntime(options.Home, published); err != nil {
			logger.Warn("could not publish the daemon runtime file", "error", err)
		}
		claim.release()
		if ready != nil {
			ready(addrs)
		}
	}
}

func openServeDB(home string, logger *slog.Logger) (*sqlite.DB, error) {
	db, err := sqlite.OpenDB(config.DatabasePath(home), logger)
	if err != nil {
		return nil, err
	}
	if err := sqlite.Migrate(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func startServeWorkers(ctx context.Context, wired wired) {
	startMaintenance(ctx, wired.server, wired.deps)
	startRefreshing(ctx, wired.deps)
	startDiscovery(ctx, wired.deps)
	startAntigravityVersion(ctx, wired.deps)
	startQuota(ctx, wired.deps)
	warnOnMissingClientKeys(ctx, wired.deps)
}

func serveWired(ctx context.Context, wired wired, cfg *config.Config, logger *slog.Logger, cause *signalCause) error {
	startServeWorkers(ctx, wired)
	logger.Info("relo serving", serveFields(cfg, wired)...)
	FinishStartupLog()
	if err := wired.server.Start(ctx); err != nil {
		logger.Info("daemon stopped", "signal", cause.text(), "error", err)
		return err
	}
	logger.Info("daemon stopped", "signal", cause.text())
	return nil
}

func serveFields(cfg *config.Config, wired wired) []any {
	fields := []any{
		"addr", cfg.Server.Addr(),
		"secret_mode", string(wired.secrets.Mode()),
	}
	for _, listener := range cfg.DataPlaneListeners() {
		fields = append(fields, listener.Protocol, cfg.Server.DataPlaneAddr(listener.Protocol))
	}
	return fields
}

func observeSignals() *signalCause {
	cause := &signalCause{done: make(chan struct{}), signals: make(chan os.Signal, 1)}
	signal.Notify(cause.signals, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		select {
		case sig := <-cause.signals:
			cause.name.Store(sig.String())
		case <-cause.done:
		}
	}()
	return cause
}

type signalCause struct {
	done    chan struct{}
	signals chan os.Signal
	name    atomic.Value
}

func (c *signalCause) stop() {
	signal.Stop(c.signals)
	close(c.done)
}

func (c *signalCause) text() string {
	name, _ := c.name.Load().(string)
	return name
}

func boundListeners(addrs server.Addrs) []string {
	listeners := make([]string, 0, 1+len(addrs.DataPlane))
	if addrs.Management != "" {
		listeners = append(listeners, addrs.Management)
	}
	names := make([]string, 0, len(addrs.DataPlane))
	for name := range addrs.DataPlane {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		address := addrs.DataPlane[name]
		if address == "" || address == addrs.Management {
			continue
		}
		listeners = append(listeners, address)
	}
	return listeners
}

func runtimeConfig(options Options) (*config.Config, error) {
	cfg, err := config.LoadConfig(config.ConfigPath(options.Home), options.Logger)
	if err != nil {
		return nil, err
	}
	if options.Port > 0 {
		cfg.Server.Port = options.Port
	}
	if err := config.ValidateConfig(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// NewLogger returns the logger every Relo process writes through. Lines are
// JSONL in the state directory's logs folder. A foreground run also copies
// them to stderr; a spawned daemon copies its boot lines to the startup
// transcript startupPath names until FinishStartupLog. An empty startupPath
// copies nothing extra.
func NewLogger(home string, level slog.Level, startupPath string) (*slog.Logger, error) {
	writer, err := openDaemonLog(home)
	if err != nil {
		return nil, err
	}
	output := &startupTee{main: writer}
	if startupPath != "" {
		file, openErr := os.OpenFile(startupPath, os.O_APPEND|os.O_WRONLY, 0)
		if openErr == nil {
			output.extra = file
			registerStartupClose(output.dropExtra)
		}
	} else if !testing.Testing() {
		output.mirror = os.Stderr
	}
	return slog.New(newLogrusHandler(output, level)), nil
}

// NewHTTPClient returns the bounded client the daemon uses for calls that
// answer with one body, such as discovery.
func NewHTTPClient() *http.Client {
	return &http.Client{Timeout: upstreamTimeout, Transport: config.OutboundTransport()}
}

// NewInferenceClient returns the client the relay reaches providers with. It
// carries no overall timeout, because that timer would include reading the
// body and abort a live stream; the call wait bounds silence on each read.
func NewInferenceClient() *http.Client {
	return &http.Client{Transport: config.OutboundTransport()}
}

// EnsureAdminToken reads or creates the one management credential.
func EnsureAdminToken(home string) (string, error) {
	return server.EnsureToken(server.TokenPath(home, server.AdminTokenFile))
}

func newInstanceID() string {
	buffer := make([]byte, instanceIDBytes)
	if _, err := rand.Read(buffer); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return hex.EncodeToString(buffer)
}

// DaemonLogPath is where a started daemon writes what it logs.
func DaemonLogPath(home string) string {
	return filepath.Join(logDir(home), daemonLogName)
}

func logDir(home string) string {
	return filepath.Join(home, logDirName)
}

func warnOnMissingClientKeys(ctx context.Context, built deps) {
	keys, err := built.keys.AccessKeys(ctx)
	if err != nil {
		built.logger.Warn("could not read the client keys", "error", err)
		return
	}
	active := 0
	for _, key := range keys {
		if key.Status == access.StatusActive {
			active++
		}
	}
	if active == 0 {
		built.logger.Warn("no active client key: the data plane refuses inference until one is created",
			"create", "the Keys page of the console", "dashboard", "/keys")
		return
	}
	if _, err := os.Stat(filepath.Join(built.home, access.LegacyTokenFile)); err == nil {
		built.logger.Warn("the legacy data-plane token file is ignored; delete it once every client uses a client key",
			"file", access.LegacyTokenFile)
	}
}
