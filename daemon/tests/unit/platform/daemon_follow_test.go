package platform_test

import (
	"bufio"
	"context"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/config"
	"github.com/jonaskahn/relo/internal/platform"
	"github.com/jonaskahn/relo/tests/testkit"
)

func TestRunningInstanceKeepsALivePidWhenHealthFails(t *testing.T) {
	home := testkit.TempHome(t)
	listener := listenLocal(t)
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	})}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })
	writeRuntime(t, home, platform.Runtime{
		Address: listener.Addr().String(), InstanceID: "live", PID: os.Getpid(),
	})

	if _, running := platform.RunningInstance(context.Background(), home); running {
		t.Fatal("RunningInstance() reported a daemon that failed the health probe")
	}
	if _, found := platform.ReadRuntime(home); !found {
		t.Fatal("the runtime file was deleted while its pid still held the port")
	}
}

func TestRunningInstanceDropsADeadPid(t *testing.T) {
	home := testkit.TempHome(t)
	writeRuntime(t, home, platform.Runtime{
		Address: "127.0.0.1:1", InstanceID: "gone", PID: 1 << 30,
	})
	if _, running := platform.RunningInstance(context.Background(), home); running {
		t.Fatal("RunningInstance() reported a dead pid")
	}
	if _, found := platform.ReadRuntime(home); found {
		t.Fatal("the runtime file of a dead pid was kept")
	}
}

func TestServeFollowsALiveDaemon(t *testing.T) {
	home := testkit.TempHome(t)
	listener := listenLocal(t)
	port := listener.Addr().(*net.TCPAddr).Port
	writeIsolatedPorts(t, home, port)
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })
	writeRuntime(t, home, platform.Runtime{
		Address: listener.Addr().String(), InstanceID: "winner", PID: os.Getpid(),
	})

	err := platform.Serve(context.Background(), platform.Options{
		Home: home, Port: port, Headless: true,
	})
	if err != nil {
		t.Fatalf("Serve() error = %v, want to follow the daemon already listening", err)
	}
	published, found := platform.ReadRuntime(home)
	if !found || published.InstanceID != "winner" || published.PID != os.Getpid() {
		t.Fatalf("runtime = %+v (found %v), want the original daemon", published, found)
	}
}

func TestServeBindFailureLeavesAnotherRuntime(t *testing.T) {
	home := testkit.TempHome(t)
	winner := listenLocal(t)
	writeRuntime(t, home, platform.Runtime{
		Address: winner.Addr().String(), InstanceID: "winner", PID: os.Getpid(),
	})
	busy := listenLocal(t)
	port := busy.Addr().(*net.TCPAddr).Port
	writeIsolatedPorts(t, home, port)
	t.Cleanup(func() { _ = busy.Close(); _ = winner.Close() })

	err := platform.Serve(context.Background(), platform.Options{
		Home: home, Port: port, Headless: true,
	})
	if err == nil {
		t.Fatal("Serve() error = nil, want a bind failure")
	}
	published, found := platform.ReadRuntime(home)
	if !found || published.InstanceID != "winner" {
		t.Fatalf("runtime = %+v (found %v), want the other daemon's file", published, found)
	}
}

func TestStartOrAttachWaitsForTheOtherProcess(t *testing.T) {
	home := testkit.TempHome(t)
	probe := listenLocal(t)
	port := probe.Addr().(*net.TCPAddr).Port
	if err := probe.Close(); err != nil {
		t.Fatalf("close the probe listener: %v", err)
	}
	writeIsolatedPorts(t, home, port)
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(),
		"RELO_TEST_HOLD_CLAIM=1",
		"RELO_TEST_CLAIM_HOME="+home,
		"RELO_TEST_CLAIM_PORT="+strconv.Itoa(port),
	)
	output, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start the claim holder: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	})
	if _, err := bufio.NewReader(output).ReadString('\n'); err != nil {
		t.Fatalf("read the claim holder: %v", err)
	}
	if !platform.DaemonClaimHeld(home) {
		t.Fatal("DaemonClaimHeld() = false while the other process holds the claim")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	published, startupLog, err := platform.StartOrAttach(ctx, home, port)
	if err != nil {
		t.Fatalf("StartOrAttach() error = %v", err)
	}
	if startupLog != "" {
		t.Fatalf("startup log = %q, want none when another process holds the claim", startupLog)
	}
	want := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	if published.Address != want || published.PID != cmd.Process.Pid {
		t.Fatalf("runtime = %+v, want %s pid %d", published, want, cmd.Process.Pid)
	}
}

func listenLocal(t *testing.T) net.Listener {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	return listener
}

func writeRuntime(t *testing.T, home string, published platform.Runtime) {
	t.Helper()
	body := []byte(`{"address":"` + published.Address + `","instance_id":"` + published.InstanceID + `","pid":` + strconv.Itoa(published.PID) + `}`)
	if err := os.WriteFile(platform.RuntimePath(home), body, 0o600); err != nil {
		t.Fatalf("write the runtime file: %v", err)
	}
}

func writeIsolatedPorts(t *testing.T, home string, port int) {
	t.Helper()
	body := "[server]\nbind = \"127.0.0.1\"\nport = " + strconv.Itoa(port) +
		"\n\n[server.data_plane]\nopenai = 0\nanthropic = 0\ngemini = 0\n"
	if err := os.WriteFile(config.ConfigPath(home), []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
}
