package cli_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/platform"
	"github.com/jonaskahn/relo/internal/server"
	"github.com/jonaskahn/relo/tests/testkit"
)

// publishedDaemon stands in for a running headless daemon: it answers the
// health probe until its stop route is called, then stops answering.
type publishedDaemon struct {
	server    *httptest.Server
	token     string
	instance  string
	stopped   atomic.Bool
	stopCalls chan struct{}
}

// start serves the stand-in and publishes the address it listens on, the way
// a headless daemon does once it is bound.
func (d *publishedDaemon) start(t *testing.T, home string) platform.Runtime {
	t.Helper()
	d.instance = "instance-under-test"
	d.stopCalls = make(chan struct{}, 1)
	route := http.NewServeMux()
	route.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		if d.stopped.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	route.HandleFunc("POST /api/v1/daemon/stop", func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer "+d.token {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		d.stopped.Store(true)
		select {
		case d.stopCalls <- struct{}{}:
		default:
		}
		w.WriteHeader(http.StatusAccepted)
		w.(http.Flusher).Flush()
		go d.server.Close()
	})
	d.server = httptest.NewServer(route)
	t.Cleanup(d.server.Close)
	published := platform.Runtime{
		Address:    strings.TrimPrefix(d.server.URL, "http://"),
		InstanceID: d.instance,
		PID:        os.Getpid(),
	}
	writeRuntimeFile(t, home, published)
	return published
}

// writeRuntimeFile stores what a daemon published, the way the daemon does.
func writeRuntimeFile(t *testing.T, home string, published platform.Runtime) {
	t.Helper()
	body := fmt.Sprintf(`{"address":%q,"instance_id":%q,"pid":%d}`,
		published.Address, published.InstanceID, published.PID)
	if err := os.WriteFile(platform.RuntimePath(home), []byte(body), 0o600); err != nil {
		t.Fatalf("write the runtime file: %v", err)
	}
}

func TestStopCommand(t *testing.T) {
	t.Run("nothing running is not a failure", func(t *testing.T) {
		home := testkit.TempHome(t)
		stdout, _, err := run(t, nil, "--home", home, "daemon", "stop")
		if err != nil {
			t.Fatalf("stop error = %v", err)
		}
		if !strings.Contains(stdout, "Relo is not running") {
			t.Fatalf("stdout = %q, want the not-running report", stdout)
		}
	})

	t.Run("a stale runtime file is cleared", func(t *testing.T) {
		home := testkit.TempHome(t)
		writeRuntimeFile(t, home, platform.Runtime{
			Address: "127.0.0.1:1", InstanceID: "instance-under-test", PID: 1,
		})
		stdout, _, err := run(t, nil, "--home", home, "daemon", "stop")
		if err != nil {
			t.Fatalf("stop error = %v", err)
		}
		if !strings.Contains(stdout, "Relo is not running") {
			t.Fatalf("stdout = %q, want the not-running report", stdout)
		}
		if _, found := platform.ReadRuntime(home); found {
			t.Fatal("the stale runtime file survived the stop")
		}
	})

	t.Run("--force frees the ports a running daemon was serving", func(t *testing.T) {
		home, port := startedHome(t)
		if stdout, _, err := run(t, nil, "--home", home, "daemon", "start", "--port", strconv.Itoa(port)); err != nil {
			t.Fatalf("daemon start error = %v (stdout %s)", err, stdout)
		}
		stdout, _, err := run(t, nil, "--home", home, "daemon", "stop", "--force")
		if err != nil {
			t.Fatalf("stop --force error = %v", err)
		}
		if !strings.Contains(stdout, "Relo stopped") {
			t.Fatalf("stdout = %q, want the stop report", stdout)
		}
		if stillListening(t, port) {
			t.Fatalf("port %d is still held after the forced stop", port)
		}
	})

	t.Run("--force leaves a port another program holds and says so", func(t *testing.T) {
		home := testkit.TempHome(t)
		port := freePort(t)
		holdPortWithForeignProcess(t, port)
		writePort(t, home, strconv.Itoa(port))
		_, _, err := run(t, nil, "--home", home, "daemon", "stop", "--force")
		if !errors.Is(err, platform.ErrPortInUse) {
			t.Fatalf("stop --force error = %v, want the port-in-use refusal", err)
		}
		if !stillListening(t, port) {
			t.Fatalf("the forced stop killed a process on port %d that is not Relo", port)
		}
	})

	t.Run("a published daemon is stopped", func(t *testing.T) {
		home := testkit.TempHome(t)
		token, err := server.EnsureToken(server.TokenPath(home, server.AdminTokenFile))
		if err != nil {
			t.Fatalf("EnsureToken() error = %v", err)
		}
		daemon := &publishedDaemon{token: token}
		published := daemon.start(t, home)

		stdout, _, err := run(t, nil, "--home", home, "daemon", "stop")
		if err != nil {
			t.Fatalf("stop error = %v", err)
		}
		if !strings.Contains(stdout, "Relo stopped") {
			t.Fatalf("stdout = %q, want the stop report", stdout)
		}
		select {
		case <-daemon.stopCalls:
		case <-time.After(5 * time.Second):
			t.Fatal("the daemon was reported stopped but never asked to stop")
		}
		if _, found := platform.ReadRuntime(home); found {
			t.Fatalf("the runtime file of %s survived the stop", published.Address)
		}
	})
}

func TestStartRefusedBeforeSpawning(t *testing.T) {
	home := testkit.TempHome(t)
	port := freePort(t)
	holdPortWithForeignProcess(t, port)
	writePort(t, home, strconv.Itoa(port))

	_, _, err := run(t, nil, "--home", home, "daemon", "start")
	if !errors.Is(err, platform.ErrPortInUse) {
		t.Fatalf("start error = %v, want the port-in-use refusal", err)
	}

	logs := filepath.Dir(platform.DaemonLogPath(home))
	transcripts, err := filepath.Glob(filepath.Join(logs, "startup-*.log"))
	if err != nil || len(transcripts) != 1 {
		t.Fatalf("transcripts = %v (%v), want one boot transcript for the refused start", transcripts, err)
	}
	body, err := os.ReadFile(transcripts[0])
	if err != nil {
		t.Fatalf("read the transcript: %v", err)
	}
	if !strings.Contains(string(body), "address already in use") {
		t.Fatalf("transcript = %q, want the refusal that stopped the start", body)
	}
}

func TestReadRuntime(t *testing.T) {
	t.Run("a published address names its port", func(t *testing.T) {
		published := platform.Runtime{Address: "127.0.0.1:12345", InstanceID: "one"}
		if got := published.Port(); got != 12345 {
			t.Fatalf("Port() = %d, want the published port", got)
		}
	})

	t.Run("an address without a port names none", func(t *testing.T) {
		for _, address := range []string{"127.0.0.1", "127.0.0.1:not-a-port", ":"} {
			published := platform.Runtime{Address: address, InstanceID: "one"}
			if got := published.Port(); got != 0 {
				t.Fatalf("Port() of %q = %d, want none", address, got)
			}
		}
	})

	t.Run("a file nothing usable is in is not a running daemon", func(t *testing.T) {
		home := testkit.TempHome(t)
		for _, body := range []string{"not json", `{"address":""}`, ""} {
			if err := os.WriteFile(platform.RuntimePath(home), []byte(body), 0o600); err != nil {
				t.Fatalf("write the runtime file: %v", err)
			}
			if _, found := platform.ReadRuntime(home); found {
				t.Fatalf("ReadRuntime() found a daemon in %q", body)
			}
		}
	})
}

// TestHeadlessServeWithoutAPublication covers what a daemon does when it
// cannot write the address the lifecycle commands read: it serves anyway and
// says so, so an operator reads a warning rather than a daemon that refused
// to start.
func TestHeadlessServeWithoutAPublication(t *testing.T) {
	home := testkit.TempHome(t)
	useFreePorts(t, home)
	if err := os.Mkdir(platform.RuntimePath(home), 0o700); err != nil {
		t.Fatalf("put a directory in place of the runtime file: %v", err)
	}
	port := freePort(t)
	logger, buffer := testkit.TestLogger(t)
	ctx, cancel := context.WithCancel(context.Background())
	serving := make(chan error, 1)
	go func() {
		serving <- platform.Serve(ctx, platform.Options{
			Home: home, Port: port, Logger: logger, Headless: true,
		})
	}()
	waitForHealthz(t, port)
	cancel()
	select {
	case err := <-serving:
		if err != nil {
			t.Fatalf("Serve() error = %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("the daemon did not stop")
	}
	if !strings.Contains(buffer.String(), "could not publish the daemon runtime file") {
		t.Fatalf("log = %s, want the publication warning", buffer.String())
	}
}
