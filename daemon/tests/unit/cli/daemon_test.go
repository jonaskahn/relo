package cli_test

import (
	"encoding/json"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/platform"
	"github.com/jonaskahn/relo/internal/server"
	"github.com/jonaskahn/relo/tests/testkit"
)

// The lifecycle commands spawn whatever executable the process is, which
// under `go test` is this test binary. Two variables let a test stand in for
// a running daemon without one: TestMain intercepts the mode before the test
// flags are parsed, and the mode reads the state directory and port from the
// environment it inherited.
const (
	daemonHomeEnv  = "RELO_TEST_DAEMON_HOME"
	daemonPortEnv  = "RELO_TEST_DAEMON_PORT"
	daemonInstance = "instance-under-test"
	// holdPortEnv turns this binary into a process that listens on a port and
	// nothing else, which is how a test stands a program that is not Relo on
	// one of Relo's ports.
	holdPortEnv = "RELO_TEST_HOLD_PORT"
)

func TestMain(m *testing.M) {
	if home := os.Getenv(daemonHomeEnv); home != "" {
		runHelperDaemon(home, os.Getenv(daemonPortEnv))
		return
	}
	if port := os.Getenv(holdPortEnv); port != "" {
		runPortHolder(port)
		return
	}
	os.Exit(m.Run())
}

// runPortHolder listens on the port it was given until it is killed, so the
// process table has an owner on it that is not a Relo daemon.
func runPortHolder(port string) {
	listener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", port))
	if err != nil {
		os.Exit(3)
	}
	for {
		conn, err := listener.Accept()
		if err != nil {
			os.Exit(0)
		}
		_ = conn.Close()
	}
}

// runHelperDaemon is the daemon a start spawns in these tests: it publishes
// the runtime file the way a real daemon does, answers the health probe, and
// ends when the stop route is called.
func runHelperDaemon(home, port string) {
	address := net.JoinHostPort("127.0.0.1", port)
	published, err := json.Marshal(map[string]any{
		"address": address, "instance_id": daemonInstance, "pid": os.Getpid(),
	})
	if err != nil {
		os.Exit(3)
	}
	if err := os.WriteFile(platform.RuntimePath(home), published, 0o600); err != nil {
		os.Exit(3)
	}
	stopped := make(chan struct{})
	route := http.NewServeMux()
	route.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	route.HandleFunc("POST /api/v1/daemon/stop", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		close(stopped)
	})
	listener, err := net.Listen("tcp", address)
	if err != nil {
		os.Exit(3)
	}
	server := &http.Server{Handler: route}
	go func() { _ = server.Serve(listener) }()
	<-stopped
	_ = server.Close()
	os.Exit(0)
}

// startedHome prepares a state directory whose lifecycle commands spawn the
// stand-in daemon, on a port this machine can bind.
func startedHome(t *testing.T) (string, int) {
	t.Helper()
	home := testkit.TempHome(t)
	useFreePorts(t, home)
	port := freePort(t)
	// A daemon that has served once has published the admin token the stop
	// request authenticates with.
	if _, err := server.EnsureToken(server.TokenPath(home, server.AdminTokenFile)); err != nil {
		t.Fatalf("EnsureToken() error = %v", err)
	}
	t.Setenv(daemonHomeEnv, home)
	t.Setenv(daemonPortEnv, strconv.Itoa(port))
	t.Cleanup(func() {
		// A failed assertion must not leave the stand-in running.
		if published, found := platform.ReadRuntime(home); found {
			if process, err := os.FindProcess(published.PID); err == nil {
				_ = process.Kill()
			}
		}
	})
	return home, port
}

// TestLifecycleCommands drives start, restart, and stop against a stand-in
// daemon, which is what the commands do to a real one: publish an address,
// answer the health probe, and end when asked.
func TestLifecycleCommands(t *testing.T) {
	home, port := startedHome(t)
	running := "Relo is running at 127.0.0.1:" + strconv.Itoa(port)

	t.Run("start runs the daemon in the background", func(t *testing.T) {
		stdout, stderr, err := run(t, nil, "--home", home, "daemon", "start", "--port", strconv.Itoa(port))
		if err != nil {
			t.Fatalf("start error = %v (stdout %s, stderr %s)", err, stdout, stderr)
		}
		if !strings.Contains(stdout, running) {
			t.Fatalf("stdout = %q, want %q", stdout, running)
		}
		published, found := platform.ReadRuntime(home)
		if !found || published.Address != "127.0.0.1:"+strconv.Itoa(port) {
			t.Fatalf("runtime = %+v (found %v), want the bound address", published, found)
		}
	})

	t.Run("a second start reports the daemon already running", func(t *testing.T) {
		stdout, _, err := run(t, nil, "--home", home, "daemon", "start", "--port", strconv.Itoa(port))
		if err != nil {
			t.Fatalf("start error = %v", err)
		}
		if !strings.Contains(stdout, "Relo is already running at") {
			t.Fatalf("stdout = %q, want the already-running report", stdout)
		}
	})

	t.Run("restart returns the daemon on the port it held", func(t *testing.T) {
		stdout, stderr, err := run(t, nil, "--home", home, "daemon", "restart")
		if err != nil {
			t.Fatalf("restart error = %v (stdout %s, stderr %s)", err, stdout, stderr)
		}
		if !strings.Contains(stdout, running) {
			t.Fatalf("stdout = %q, want the daemon back on %d", stdout, port)
		}
	})

	t.Run("stop ends the daemon and clears its address", func(t *testing.T) {
		stdout, stderr, err := run(t, nil, "--home", home, "daemon", "stop")
		if err != nil {
			t.Fatalf("stop error = %v (stdout %s, stderr %s)", err, stdout, stderr)
		}
		if !strings.Contains(stdout, "Relo stopped") {
			t.Fatalf("stdout = %q, want the stop report", stdout)
		}
		if _, found := platform.ReadRuntime(home); found {
			t.Fatal("the runtime file outlived the daemon")
		}
	})

	t.Run("stop with nothing running is not a failure", func(t *testing.T) {
		stdout, _, err := run(t, nil, "--home", home, "daemon", "stop")
		if err != nil {
			t.Fatalf("stop error = %v", err)
		}
		if !strings.Contains(stdout, "Relo is not running") {
			t.Fatalf("stdout = %q, want the not-running report", stdout)
		}
	})
}
