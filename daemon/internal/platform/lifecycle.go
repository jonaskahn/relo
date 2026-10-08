// Daemon lifecycle: running instances, attach, and start.
package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jonaskahn/relo/internal/config"
	"github.com/jonaskahn/relo/internal/server"
)

const (
	startTimeout = 30 * time.Second
	// stopTimeout bounds how long a stop waits for a daemon to drain, which is
	// a little past the server's own shutdown budget.
	stopTimeout           = 35 * time.Second
	lifecyclePollInterval = 25 * time.Millisecond
	healthTimeout         = 2 * time.Second
)

var (
	// ErrListenerNotReady reports a spawned daemon that never answered.
	ErrListenerNotReady = errors.New("the listener never became healthy in time")
	// ErrDaemonStillUp reports a stop that ran out of time.
	ErrDaemonStillUp = errors.New("the daemon did not stop in time")
	// ErrNotReloProcess reports a published pid whose executable is not Relo.
	ErrNotReloProcess = errors.New("process is not a Relo daemon")
	// ErrPortInUse reports a configured port held by a process that is not Relo.
	ErrPortInUse = errors.New("address already in use")
	// ErrStopRefused reports a daemon that answered a stop or shutdown with a
	// status other than acceptance.
	ErrStopRefused = errors.New("the daemon refused to stop")
	// ErrAdminTokenMissing reports a home whose admin token cannot be read.
	ErrAdminTokenMissing = errors.New("read the admin token")
)

// RunningInstance reports the headless daemon a home currently serves, and
// clears a runtime file whose process is gone. A live pid that still holds
// its port keeps the file across a missed health probe.
func RunningInstance(ctx context.Context, home string) (Runtime, bool) {
	return RunningInstanceOn(ctx, home, 0)
}

// RunningInstanceOn reports the daemon for home. A non-zero port is the
// management port to recognise when the runtime file is missing, in place of
// the port in the startup configuration.
func RunningInstanceOn(ctx context.Context, home string, port int) (Runtime, bool) {
	if published, found := ReadRuntime(home); found {
		if pidHoldsPort(published) && HealthOf(ctx, published.Address) {
			return published, true
		}
		if pidHoldsPort(published) {
			return Runtime{}, false
		}
		RemoveRuntime(home)
	}
	return adoptDaemon(ctx, home, port)
}

// HoldingDaemon reports the published daemon whose pid still listens on its
// port. One missed health probe does not forget it.
func HoldingDaemon(home string) (Runtime, bool) {
	published, found := ReadRuntime(home)
	if !found || !pidHoldsPort(published) {
		return Runtime{}, false
	}
	return published, true
}

// StartOrAttach attaches to the daemon already listening for home, or starts
// one. The startup log path is empty when an existing daemon was attached.
// The other login agent is waited out: whichever process binds first keeps
// the service.
func StartOrAttach(ctx context.Context, home string, port int) (Runtime, string, error) {
	if published, running := RunningInstanceOn(ctx, home, port); running {
		return published, "", nil
	}
	if DaemonClaimHeld(home) || reloHoldsManagement(home, port) {
		if published, log, done := waitForClaimedDaemon(ctx, home, port); done {
			return published, log, nil
		}
		if reloHoldsManagement(home, port) {
			return Runtime{}, "", ErrListenerNotReady
		}
	}
	if err := ForeignListener(home, port); err != nil {
		return Runtime{}, "", err
	}
	return startAttachedDaemon(ctx, home, port)
}

func waitForClaimedDaemon(ctx context.Context, home string, port int) (Runtime, string, bool) {
	if published, err := WaitForRuntime(ctx, home); err == nil {
		return published, "", true
	}
	if published, running := RunningInstanceOn(context.Background(), home, port); running {
		return published, "", true
	}
	return Runtime{}, "", false
}

func startAttachedDaemon(ctx context.Context, home string, port int) (Runtime, string, error) {
	startupLog, err := SpawnDaemon(home, port)
	if err != nil {
		if published, running := RunningInstanceOn(context.Background(), home, port); running {
			return published, startupLog, nil
		}
		return Runtime{}, startupLog, err
	}
	published, err := WaitForRuntime(ctx, home)
	if err != nil {
		if published, running := RunningInstanceOn(context.Background(), home, port); running {
			return published, startupLog, nil
		}
		return Runtime{}, startupLog, err
	}
	return published, startupLog, nil
}

// ForceFreePorts ends whatever is listening on the management port and every
// data-plane port in the startup configuration, and waits until those ports
// stop accepting. The next start can bind them and publish a new runtime file.
func ForceFreePorts(home string, port int) error {
	ports := servicePorts(home, port)
	inspector := portInspector{}
	for _, candidate := range ports {
		err := inspector.Terminate(candidate)
		if err != nil && !errors.Is(err, ErrNoPortOwner) {
			return err
		}
	}
	return waitPortsReleased(ports)
}

// FreeOwnPorts ends the Relo processes listening on the management port and
// every data-plane port in the startup configuration, and waits until the
// ports it freed stop accepting. A port another program holds is left with
// that program, so an install that cannot bind it says so instead of ending a
// process that is not Relo.
func FreeOwnPorts(home string, port int) error {
	freed := make([]int, 0, 1)
	inspector := portInspector{}
	for _, candidate := range servicePorts(home, port) {
		owner, found := portOwnerPID(candidate)
		if !found || !reloProcess(owner) {
			continue
		}
		if err := inspector.Terminate(candidate); err != nil && !errors.Is(err, ErrNoPortOwner) {
			return err
		}
		freed = append(freed, candidate)
	}
	return waitPortsReleased(freed)
}

// ForeignListener reports the first configured port a process that is not
// Relo holds, naming the port and the program in the way so a caller that
// cannot bind it can say so instead of ending the program.
func ForeignListener(home string, port int) error {
	for _, candidate := range servicePorts(home, port) {
		owner, found := portOwnerPID(candidate)
		if !found || reloProcess(owner) {
			continue
		}
		return fmt.Errorf("port %d is held by %s: %w", candidate, processName(owner), ErrPortInUse)
	}
	return nil
}

// StopRunning asks one daemon to drain. A daemon recognised from its port,
// with no published instance, is shut down from this machine.
func StopRunning(ctx context.Context, home string, published Runtime) error {
	var err error
	if published.InstanceID == "" {
		err = requestShutdown(ctx, home, published.Address)
	} else {
		err = RequestStop(ctx, home, published)
	}
	if err != nil {
		return err
	}
	return WaitForShutdown(ctx, published)
}

// SpawnDaemon starts one detached headless daemon. The child's stderr is the
// timestamped startup transcript this call creates, and the child writes its
// ongoing JSONL to the daemon log itself. The returned path is that
// transcript. The child is left running: the caller waits for the runtime
// file it publishes rather than for the process.
func daemonCommand(home string, port int, executable, startupPath string, startupFile *os.File) *exec.Cmd {
	args := []string{"daemon", "run", "--home", home}
	if port > 0 {
		args = append(args, "--port", strconv.Itoa(port))
	}
	args = append(args, "--startup-log", startupPath)
	command := exec.Command(executable, args...)
	command.Stdout = startupFile
	command.Stderr = startupFile
	command.SysProcAttr = detachAttributes()
	return command
}

// SpawnDaemon starts the daemon detached from this process and returns the
// boot transcript it writes while starting.
func SpawnDaemon(home string, port int) (string, error) {
	if err := config.EnsureReloHome(home); err != nil {
		return "", err
	}
	executable, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve the relo executable: %w", err)
	}
	startupPath, startupFile, err := CreateStartupLog(home)
	if err != nil {
		return "", err
	}
	command := daemonCommand(home, port, executable, startupPath, startupFile)
	if err := command.Start(); err != nil {
		return startupPath, recordSpawnFailure(startupFile, err)
	}
	if err := startupFile.Close(); err != nil {
		return startupPath, fmt.Errorf("start the daemon: %w", err)
	}
	if err := command.Process.Release(); err != nil {
		return startupPath, fmt.Errorf("detach the daemon: %w", err)
	}
	return startupPath, nil
}

func recordSpawnFailure(startupFile *os.File, err error) error {
	if _, writeErr := fmt.Fprintln(startupFile, err.Error()); writeErr != nil {
		_ = startupFile.Close()
		return fmt.Errorf("start the daemon: %w", err)
	}
	if closeErr := startupFile.Close(); closeErr != nil {
		return fmt.Errorf("start the daemon: %w", err)
	}
	return fmt.Errorf("start the daemon: %w", err)
}

// SpawnRestart starts a detached helper that cycles this daemon after the
// caller has answered, so the HTTP response can leave first.
func SpawnRestart(home string, force bool) error {
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve the relo executable: %w", err)
	}
	args := []string{"daemon", "restart", "--home", home}
	if force {
		args = append(args, "--force")
	}
	command := exec.Command(executable, args...)
	command.SysProcAttr = detachAttributes()
	if err := command.Start(); err != nil {
		return fmt.Errorf("start the restart helper: %w", err)
	}
	if err := command.Process.Release(); err != nil {
		return fmt.Errorf("detach the restart helper: %w", err)
	}
	return nil
}

// WaitForRuntime waits for a spawned daemon to publish where it listens and
// to answer there, which is the moment the caller can report it started.
func WaitForRuntime(ctx context.Context, home string) (Runtime, error) {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, startTimeout)
		defer cancel()
	}
	for {
		if published, found := ReadRuntime(home); found && HealthOf(ctx, published.Address) {
			return published, nil
		}
		if !sleepContext(ctx, lifecyclePollInterval) {
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return Runtime{}, ErrListenerNotReady
			}
			return Runtime{}, ctx.Err()
		}
	}
}

// RequestStop asks one running daemon to end itself. The admin token and the
// instance it names are both required, so only the home that started this
// daemon can stop it.
func RequestStop(ctx context.Context, home string, published Runtime) error {
	token, err := readAdminToken(home)
	if err != nil {
		return err
	}
	body, err := json.Marshal(map[string]string{"instance_id": published.InstanceID})
	if err != nil {
		return err
	}
	target := "http://" + published.Address + "/api/v1/daemon/stop"
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build the stop request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+token)
	client := &http.Client{Timeout: stopTimeout}
	response, err := client.Do(request)
	if err != nil {
		// A daemon that died on its own is already stopped, so a connection
		// that failed is not a reason to report a failure.
		return nil
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusAccepted {
		return fmt.Errorf("%w (%s)", ErrStopRefused, response.Status)
	}
	return nil
}

func requestShutdown(ctx context.Context, home, address string) error {
	token, err := readAdminToken(home)
	if err != nil {
		return err
	}
	target := "http://" + address + "/api/v1/daemon/shutdown"
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, target, nil)
	if err != nil {
		return fmt.Errorf("build the shutdown request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	client := &http.Client{Timeout: stopTimeout}
	response, err := client.Do(request)
	if err != nil {
		return nil
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusAccepted {
		return fmt.Errorf("%w (%s)", ErrStopRefused, response.Status)
	}
	return nil
}

// WaitForShutdown waits until a daemon stops answering and every extra
// listener it published has released its port, which is how the caller
// knows the next process can bind them. A runtime file with no extra
// listeners waits only for the management health probe.
func WaitForShutdown(ctx context.Context, published Runtime) error {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, stopTimeout)
		defer cancel()
	}
	for {
		down := !HealthOf(ctx, published.Address) && listenersClosed(ctx, published)
		if err := ctx.Err(); err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				return ErrDaemonStillUp
			}
			return err
		}
		if down {
			return nil
		}
		if !sleepContext(ctx, lifecyclePollInterval) {
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return ErrDaemonStillUp
			}
			return ctx.Err()
		}
	}
}

func listenersClosed(ctx context.Context, published Runtime) bool {
	for _, address := range published.Listeners {
		if address == "" || address == published.Address {
			continue
		}
		if listenerAccepts(ctx, address) {
			return false
		}
	}
	return true
}

func listenerAccepts(ctx context.Context, address string) bool {
	dialer := net.Dialer{Timeout: 50 * time.Millisecond}
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// HealthOf reports whether a daemon answers the health probe at an address.
func HealthOf(ctx context.Context, address string) bool {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+address+"/healthz", nil)
	if err != nil {
		return false
	}
	client := &http.Client{Timeout: healthTimeout}
	response, err := client.Do(request)
	if err != nil {
		return false
	}
	defer func() { _ = response.Body.Close() }()
	return response.StatusCode == http.StatusOK
}

// KillDaemon ends the published process when it is still this Relo
// executable. A missing or already-dead process is not an error.
func KillDaemon(published Runtime) error {
	if published.PID <= 0 {
		return nil
	}
	path, err := processExecutable(published.PID)
	if err != nil {
		return nil
	}
	if !sameExecutable(path, currentReloExecutable()) {
		return fmt.Errorf("process %d: %w", published.PID, ErrNotReloProcess)
	}
	proc, err := os.FindProcess(published.PID)
	if err != nil {
		return nil
	}
	if err := proc.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return fmt.Errorf("kill the daemon: %w", err)
	}
	return nil
}

func readAdminToken(home string) (string, error) {
	content, err := os.ReadFile(server.TokenPath(home, server.AdminTokenFile))
	if err != nil {
		return "", fmt.Errorf("read the admin token of %s: %w", home, err)
	}
	token := strings.TrimSpace(string(content))
	if token == "" {
		return "", fmt.Errorf("%w of %s: missing", ErrAdminTokenMissing, home)
	}
	return token, nil
}

func currentReloExecutable() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		return resolved
	}
	return exe
}

func sameExecutable(left, right string) bool {
	if left == "" || right == "" {
		return false
	}
	if resolved, err := filepath.EvalSymlinks(left); err == nil {
		left = resolved
	}
	if resolved, err := filepath.EvalSymlinks(right); err == nil {
		right = resolved
	}
	if left == right {
		return true
	}
	leftInfo, leftErr := os.Stat(left)
	rightInfo, rightErr := os.Stat(right)
	if leftErr != nil || rightErr != nil {
		return false
	}
	return os.SameFile(leftInfo, rightInfo)
}

func adoptDaemon(ctx context.Context, home string, port int) (Runtime, bool) {
	address, port := managementAddress(home, port)
	if !healthyRelo(ctx, address, port) {
		return Runtime{}, false
	}
	owner, _ := portOwnerPID(port)
	return Runtime{Address: address, PID: owner}, true
}

func pidHoldsPort(published Runtime) bool {
	port := published.Port()
	if port <= 0 || published.PID <= 0 {
		return false
	}
	owner, found := portOwnerPID(port)
	return found && owner == published.PID && reloProcess(published.PID)
}

func reloHoldsManagement(home string, port int) bool {
	address, port := managementAddress(home, port)
	owner, found := portOwnerPID(port)
	if !found || address == "" {
		return false
	}
	return reloProcess(owner)
}

func reloProcess(pid int) bool {
	path, err := processExecutable(pid)
	if err != nil {
		return false
	}
	return sameExecutable(path, currentReloExecutable())
}

func servicePorts(home string, port int) []int {
	cfg, err := config.LoadOffline(home)
	if err != nil {
		if port > 0 {
			return []int{port}
		}
		return nil
	}
	if port > 0 {
		cfg.Server.Port = port
	}
	ports := make([]int, 0, 1+len(cfg.DataPlaneListeners()))
	if cfg.Server.Port > 0 {
		ports = append(ports, cfg.Server.Port)
	}
	for _, listener := range cfg.DataPlaneListeners() {
		if listener.Port > 0 {
			ports = append(ports, listener.Port)
		}
	}
	return ports
}

func managementAddress(home string, port int) (string, int) {
	cfg, err := config.LoadOffline(home)
	if err != nil {
		if port <= 0 {
			return "", 0
		}
		return net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), port
	}
	if port > 0 {
		cfg.Server.Port = port
	}
	if cfg.Server.Port <= 0 {
		return "", 0
	}
	return cfg.Server.Addr(), cfg.Server.Port
}

func waitPortsReleased(ports []int) error {
	ctx, cancel := context.WithTimeout(context.Background(), stopTimeout)
	defer cancel()
	return waitPortsClosed(ctx, ports)
}

func waitPortsClosed(ctx context.Context, ports []int) error {
	for {
		if portsClosed(ctx, ports) {
			return nil
		}
		if err := ctx.Err(); err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				return ErrDaemonStillUp
			}
			return err
		}
		if !sleepContext(ctx, lifecyclePollInterval) {
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return ErrDaemonStillUp
			}
			return ctx.Err()
		}
	}
}

func portsClosed(ctx context.Context, ports []int) bool {
	for _, port := range ports {
		if port <= 0 {
			continue
		}
		if listenerAccepts(ctx, net.JoinHostPort("127.0.0.1", strconv.Itoa(port))) {
			return false
		}
	}
	return true
}

func sleepContext(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
