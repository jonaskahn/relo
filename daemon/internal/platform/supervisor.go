// Daemon supervisor: state machine for start and stop.
package platform

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/jonaskahn/relo/internal/server"
)

const restartDelay = 250 * time.Millisecond

// DaemonState is where one supervised run stands.
type DaemonState int

const (
	// StateRunning means the proxy is serving and published its listeners.
	StateRunning DaemonState = iota
	// StateStopped means nothing is serving: a run was asked to stop, or
	// another process already owns the state directory.
	StateStopped
	// StateFailed means the last run ended with an error.
	StateFailed
)

// SupervisorOptions configures one supervised daemon.
type SupervisorOptions struct {
	// Home is the state directory the run serves.
	Home string
	// Port overrides the configured listen port when non-zero.
	Port int
	// Language is the language the run was asked for, when a command named
	// one. The rest of the resolution happens inside the run.
	Language string
	// Version is the build version the run reports to a status request.
	Version string
	// StartupLog is the boot transcript a spawned run tees its boot lines
	// into, which the spawning command line passes through.
	StartupLog string
	// Logger receives the run's log lines. A nil logger opens the daemon log.
	Logger *slog.Logger
}

// Supervisor owns the proxy run of one process. The tray drives it, so a
// desktop run serves and shows its icon from the same process, and a stop
// request from anywhere reaches that one run.
type Supervisor struct {
	opts SupervisorOptions

	mu      sync.Mutex
	busy    bool
	running bool
	addr    string
	lastErr error
	onState func()
	// cancel ends the run in flight, and idle is closed once Serve has
	// returned. Both are nil and closed while nothing runs.
	cancel context.CancelFunc
	idle   chan struct{}
}

// NewSupervisor returns a supervisor for one state directory. Nothing runs
// until Start is called.
func NewSupervisor(opts SupervisorOptions) *Supervisor {
	idle := make(chan struct{})
	close(idle)
	return &Supervisor{opts: opts, idle: idle}
}

// OnStateChange registers a callback invoked after every lifecycle change,
// so a surface redraws from what State reports.
func (s *Supervisor) OnStateChange(fn func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onState = fn
}

// Start runs the proxy and reports whether this process owns the state
// directory. A healthy daemon another process already serves reports false
// with no error, which is how a second command exits without a second app.
// A start that never became healthy reports the failure.
func (s *Supervisor) Start() (bool, error) {
	if !s.begin() {
		return false, nil
	}
	defer s.end()
	return s.start()
}

func (s *Supervisor) start() (bool, error) {
	runCtx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	ready := make(chan server.Addrs, 1)
	followed := make(chan struct{}, 1)
	started := make(chan startResult, 1)
	options := serveOptions(s.opts, ready, followed)
	s.setRun(cancel)
	go func() { done <- Serve(runCtx, options) }()
	// One watcher owns the run's channel: it reports the start outcome to
	// the caller and the end outcome to the state, and it is what releases
	// Stop and a restart.
	go s.watchStart(runCtx, done, ready, followed, started, cancel)

	result := <-started
	if result.owned {
		s.recordRunning(result.addr)
	}
	return result.owned, result.err
}

func serveOptions(opts SupervisorOptions, ready chan server.Addrs, followed chan struct{}) Options {
	return Options{
		Home: opts.Home, Port: opts.Port, Language: opts.Language,
		Version: opts.Version, StartupLog: opts.StartupLog,
		Logger: opts.Logger, Headless: true,
		OnReady: func(addrs server.Addrs) {
			select {
			case ready <- addrs:
			default:
			}
		},
		OnFollowed: func() {
			select {
			case followed <- struct{}{}:
			default:
			}
		},
	}
}

func (s *Supervisor) watchStart(runCtx context.Context, done chan error, ready chan server.Addrs, followed chan struct{}, started chan startResult, cancel context.CancelFunc) {
	timer := time.NewTimer(startTimeout)
	defer timer.Stop()
	select {
	case addrs := <-ready:
		started <- startResult{owned: true, addr: addrs.Management}
		s.finish(runCtx, <-done)
	case <-followed:
		started <- startResult{}
		<-done
		s.finish(runCtx, nil)
	case err := <-done:
		started <- startResult{err: err}
		s.finish(runCtx, err)
	case <-timer.C:
		started <- startResult{err: ErrListenerNotReady}
		cancel()
		<-done
		s.finish(runCtx, ErrListenerNotReady)
	}
}

type startResult struct {
	owned bool
	addr  string
	err   error
}

func (s *Supervisor) finish(runCtx context.Context, err error) {
	// A run this supervisor cancelled is a stop it asked for, whatever the
	// run reports: only a run that ended on its own is a failure.
	if errors.Is(runCtx.Err(), context.Canceled) {
		err = nil
	}
	s.clearRun()
	s.record(err)
}

// Stop ends the run and returns once Serve has drained. Stopping a
// supervisor that runs nothing is a no-op.
func (s *Supervisor) Stop() error {
	if !s.begin() {
		return nil
	}
	defer s.end()
	return s.stop()
}

func (s *Supervisor) stop() error {
	s.mu.Lock()
	cancel, idle := s.cancel, s.idle
	s.mu.Unlock()
	if cancel == nil {
		s.record(nil)
		return nil
	}
	cancel()
	select {
	case <-idle:
		return nil
	case <-time.After(stopTimeout):
		s.record(ErrDaemonStillUp)
		return ErrDaemonStillUp
	}
}

// Restart cycles the run, the action an operator reaches for after changing
// credentials.
func (s *Supervisor) Restart() error {
	if !s.begin() {
		return nil
	}
	defer s.end()
	return s.restart(false)
}

// ForceRestart cycles the run after freeing the configured ports, the escape
// hatch for a port another process took over. The running proxy is stopped
// first, so the release never reaches this process's own listeners.
func (s *Supervisor) ForceRestart() error {
	if !s.begin() {
		return nil
	}
	defer s.end()
	return s.restart(true)
}

func (s *Supervisor) restart(force bool) error {
	if err := s.stop(); err != nil {
		return err
	}
	if force {
		if err := ForceFreePorts(s.opts.Home, s.opts.Port); err != nil && !errors.Is(err, ErrNoPortOwner) {
			s.record(err)
			return err
		}
		RemoveRuntime(s.opts.Home)
	}
	time.Sleep(restartDelay)
	_, err := s.start()
	return err
}

// State reports where the run stands, its address while running, and the
// error of the last failed run.
func (s *Supervisor) State() (DaemonState, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		return StateRunning, s.addr, nil
	}
	if s.lastErr != nil {
		return StateFailed, "", s.lastErr
	}
	return StateStopped, "", nil
}

// Busy reports whether a lifecycle action is already running.
func (s *Supervisor) Busy() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.busy
}

func (s *Supervisor) setRun(cancel context.CancelFunc) {
	s.mu.Lock()
	s.cancel = cancel
	s.idle = make(chan struct{})
	s.mu.Unlock()
}

func (s *Supervisor) clearRun() {
	s.mu.Lock()
	if s.cancel != nil {
		s.cancel = nil
		close(s.idle)
	}
	s.mu.Unlock()
}

func (s *Supervisor) begin() bool {
	s.mu.Lock()
	if s.busy {
		s.mu.Unlock()
		return false
	}
	s.busy = true
	fn := s.onState
	s.mu.Unlock()
	if fn != nil {
		fn()
	}
	return true
}

func (s *Supervisor) end() {
	s.mu.Lock()
	s.busy = false
	fn := s.onState
	s.mu.Unlock()
	if fn != nil {
		fn()
	}
}

func (s *Supervisor) recordRunning(addr string) {
	s.mu.Lock()
	already := s.running && s.addr == addr && s.lastErr == nil
	s.running, s.addr, s.lastErr = true, addr, nil
	fn := s.onState
	s.mu.Unlock()
	if already || fn == nil {
		return
	}
	fn()
}

func (s *Supervisor) record(err error) {
	s.mu.Lock()
	already := !s.running && s.lastErr == nil && err == nil
	s.running, s.addr, s.lastErr = false, "", err
	fn := s.onState
	s.mu.Unlock()
	if already || fn == nil {
		return
	}
	fn()
}
