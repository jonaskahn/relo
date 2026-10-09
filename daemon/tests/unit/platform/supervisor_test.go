package platform_test

import (
	"context"
	"net"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/config"
	"github.com/jonaskahn/relo/internal/platform"
	"github.com/jonaskahn/relo/tests/testkit"
)

// startSupervisor runs one supervised daemon on a free port and stops it when
// the test ends.
func startSupervisor(t *testing.T, home string) (*platform.Supervisor, int) {
	t.Helper()
	port := freeLoopbackPort(t)
	writeIsolatedPorts(t, home, port)
	logger, _ := testkit.TestLogger(t)
	supervisor := platform.NewSupervisor(platform.SupervisorOptions{Home: home, Port: port, Logger: logger})
	owned, err := supervisor.Start()
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if !owned {
		t.Fatal("Start() owned = false, want this process to serve the home")
	}
	t.Cleanup(func() { _ = supervisor.Stop() })
	return supervisor, port
}

func freeLoopbackPort(t *testing.T) int {
	t.Helper()
	listener := listenLocal(t)
	defer func() { _ = listener.Close() }()
	return listener.Addr().(*net.TCPAddr).Port
}

func TestSupervisorServesAndStops(t *testing.T) {
	home := testkit.TempHome(t)
	supervisor, port := startSupervisor(t, home)

	state, addr, err := supervisor.State()
	if state != platform.StateRunning || err != nil {
		t.Fatalf("State() = %v, %q, %v, want a running proxy", state, addr, err)
	}
	if addr == "" {
		t.Fatal("State() reported no address while running")
	}
	if _, found := platform.ReadRuntime(home); !found {
		t.Fatal("a running supervisor publishes no runtime file")
	}
	if published, running := platform.RunningInstanceOn(context.Background(), home, port); !running {
		t.Fatalf("RunningInstanceOn() = %+v, false, want the supervised run", published)
	}

	if err := supervisor.Stop(); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if state, _, _ := supervisor.State(); state != platform.StateStopped {
		t.Fatalf("State() = %v after Stop(), want stopped", state)
	}
	if _, found := platform.ReadRuntime(home); found {
		t.Fatal("a stopped supervisor left its runtime file behind")
	}
}

func TestSupervisorStopLeavesForeignPortsAlone(t *testing.T) {
	home := testkit.TempHome(t)
	supervisor, port := startSupervisor(t, home)
	_, held := spawnForeignHolder(t, "127.0.0.1:0")
	writeIsolatedPorts(t, home, port)
	appendDataPlanePort(t, home, held[0])
	if err := supervisor.Stop(); err != nil {
		t.Fatal(err)
	}
	if !portAccepts("127.0.0.1:" + strconv.Itoa(held[0])) {
		t.Fatal("Stop ended a foreign listener")
	}
	if owned, err := supervisor.Start(); owned || err != nil {
		t.Fatalf("terminal Stop allowed Start: %v, %v", owned, err)
	}
}

func appendDataPlanePort(t *testing.T, home string, port int) {
	t.Helper()
	path := config.ConfigPath(home)
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	updated := strings.Replace(string(body), "openai = 0", "openai = "+strconv.Itoa(port), 1)
	if err := os.WriteFile(path, []byte(updated), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

func TestSupervisorFollowsAnotherRun(t *testing.T) {
	home := testkit.TempHome(t)
	serving, _ := startSupervisor(t, home)

	logger, _ := testkit.TestLogger(t)
	follower := platform.NewSupervisor(platform.SupervisorOptions{Home: home, Logger: logger})
	owned, err := follower.Start()
	if err != nil {
		t.Fatalf("Start() error = %v, want the follower to bow out quietly", err)
	}
	if owned {
		t.Fatal("Start() owned = true, want the second supervisor to own nothing")
	}
	if state, _, _ := follower.State(); state != platform.StateStopped {
		t.Fatalf("State() = %v, want stopped for a follower", state)
	}
	if state, _, _ := serving.State(); state != platform.StateRunning {
		t.Fatalf("the serving supervisor = %v, want it left alone", state)
	}
}

func TestSupervisorReportsAFailedStart(t *testing.T) {
	home := testkit.TempHome(t)
	// A port outside the valid range fails validation before anything binds,
	// which is the shape of a start an operator has to fix.
	writeBrokenPortConfig(t, home)
	logger, _ := testkit.TestLogger(t)
	supervisor := platform.NewSupervisor(platform.SupervisorOptions{Home: home, Logger: logger})
	owned, err := supervisor.Start()
	if err == nil {
		t.Fatal("Start() error = nil, want the configuration failure")
	}
	if owned {
		t.Fatal("Start() owned = true for a run that never served")
	}
	state, _, lastErr := supervisor.State()
	if state != platform.StateFailed || lastErr == nil {
		t.Fatalf("State() = %v, %v, want the recorded failure", state, lastErr)
	}
}

func TestSupervisorRestarts(t *testing.T) {
	home := testkit.TempHome(t)
	supervisor, _ := startSupervisor(t, home)

	if err := supervisor.Restart(); err != nil {
		t.Fatalf("Restart() error = %v", err)
	}
	if state, addr, err := supervisor.State(); state != platform.StateRunning || err != nil || addr == "" {
		t.Fatalf("State() after Restart() = %v, %q, %v, want a running proxy", state, addr, err)
	}
	if err := supervisor.ForceRestart(); err != nil {
		t.Fatalf("ForceRestart() error = %v", err)
	}
	if state, _, err := supervisor.State(); state != platform.StateRunning || err != nil {
		t.Fatalf("State() after ForceRestart() = %v, %v, want a running proxy", state, err)
	}
}

func TestSupervisorNotifiesStateChanges(t *testing.T) {
	home := testkit.TempHome(t)
	port := freeLoopbackPort(t)
	writeIsolatedPorts(t, home, port)
	logger, _ := testkit.TestLogger(t)
	supervisor := platform.NewSupervisor(platform.SupervisorOptions{Home: home, Port: port, Logger: logger})

	changes := make(chan struct{}, 16)
	supervisor.OnStateChange(func() {
		select {
		case changes <- struct{}{}:
		default:
		}
	})
	if _, err := supervisor.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if err := supervisor.Stop(); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	select {
	case <-changes:
	case <-time.After(time.Second):
		t.Fatal("no state change was reported across a start and a stop")
	}
}

// TestTerminateSparesItsOwnProcess covers the guard a restarted run needs:
// the listeners this process holds are the ones it is about to replace, so
// freeing a port must never end the process itself.
func TestTerminateSparesItsOwnProcess(t *testing.T) {
	listener := listenLocal(t)
	port := listener.Addr().(*net.TCPAddr).Port
	t.Cleanup(func() { _ = listener.Close() })
	inspector := platform.NewPortInspector()
	owner, found := inspector.Owner(port)
	if !found || owner.PID != os.Getpid() {
		t.Skipf("the process table cannot name this process on port %d", port)
	}
	if err := inspector.Terminate(port); err != nil {
		t.Fatalf("Terminate() error = %v", err)
	}
	client, err := net.DialTimeout("tcp", listener.Addr().String(), time.Second)
	if err != nil {
		t.Fatalf("the listener this process holds was ended: %v", err)
	}
	_ = client.Close()
}

// writeBrokenPortConfig writes a startup file the validator refuses, which
// is the failure a start has to report.
func writeBrokenPortConfig(t *testing.T, home string) {
	t.Helper()
	body := "[server]\nbind = \"127.0.0.1\"\nport = 70000\n"
	if err := os.WriteFile(config.ConfigPath(home), []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

func TestQuitWinsAgainstRestart(t *testing.T) {
	for range 5 {
		supervisor, _ := startSupervisor(t, testkit.TempHome(t))
		restarted := make(chan error, 1)
		go func() { restarted <- supervisor.Restart() }()
		if err := supervisor.Stop(); err != nil {
			t.Fatal(err)
		}
		if err := <-restarted; err != nil {
			t.Fatal(err)
		}
		if state, _, _ := supervisor.State(); state != platform.StateStopped {
			t.Fatalf("restart survived quit: %v", state)
		}
	}
}
