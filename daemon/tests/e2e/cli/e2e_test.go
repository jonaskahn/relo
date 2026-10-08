package e2e_test

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/adapters/sqlite"
)

func TestBinaryCommands(t *testing.T) {
	binary := buildBinary(t)
	home := newHome(t)
	env := commandEnv(home)

	t.Run("version prints the build identity", func(t *testing.T) {
		stdout, _, err := runBinary(t, binary, env, "version")
		if err != nil {
			t.Fatalf("version error = %v", err)
		}
		if !strings.HasPrefix(stdout, "relo ") || !strings.Contains(stdout, runtime.GOOS) {
			t.Fatalf("stdout = %q, want the version line", stdout)
		}
	})

	t.Run("help lists every command", func(t *testing.T) {
		stdout, _, err := runBinary(t, binary, env, "help")
		if err != nil {
			t.Fatalf("help error = %v", err)
		}
		names := []string{"autostart", "daemon", "version"}
		for _, name := range names {
			if !strings.Contains(stdout, name) {
				t.Fatalf("help = %q, want %s listed", stdout, name)
			}
		}
		if strings.Contains(stdout, "desktop") {
			t.Fatalf("help = %q, want no desktop command", stdout)
		}
	})

	t.Run("daemon run help still works", func(t *testing.T) {
		stdout, _, err := runBinary(t, binary, env, "daemon", "run", "--help")
		if err != nil {
			t.Fatalf("daemon run --help error = %v", err)
		}
		if !strings.Contains(stdout, "daemon") && !strings.Contains(stdout, "run") {
			t.Fatalf("help = %q, want the daemon run command", stdout)
		}
	})

	t.Run("status reports that nothing is running", func(t *testing.T) {
		stdout, _, err := runBinary(t, binary, env, "daemon", "status")
		if err == nil {
			t.Fatal("status error = nil, want a failing exit code")
		}
		if !strings.Contains(stdout, "Relo is not running") {
			t.Fatalf("stdout = %q, want the not-running message", stdout)
		}
	})

	t.Run("start serves in the background and stop ends it", func(t *testing.T) {
		lifecycleHome := newHome(t)
		writeServeConfig(t, lifecycleHome)
		lifecycleEnv := commandEnv(lifecycleHome)
		port := freePort(t)
		// Whatever the assertions do, the daemon this test starts does not
		// outlive it.
		t.Cleanup(func() { _, _, _ = runBinary(t, binary, lifecycleEnv, "daemon", "stop") })

		stdout, stderr, err := runBinary(t, binary, lifecycleEnv, "daemon", "start", "--port", fmt.Sprint(port))
		if err != nil {
			t.Fatalf("start error = %v (stdout %s, stderr %s)", err, stdout, stderr)
		}
		if want := fmt.Sprintf("Relo is running at 127.0.0.1:%d", port); !strings.Contains(stdout, want) {
			t.Fatalf("stdout = %q, want %q", stdout, want)
		}
		waitForHealthz(t, port)

		t.Run("a second start reports the running daemon", func(t *testing.T) {
			stdout, _, err := runBinary(t, binary, lifecycleEnv, "daemon", "start", "--port", fmt.Sprint(port))
			if err != nil {
				t.Fatalf("second start error = %v", err)
			}
			if !strings.Contains(stdout, "Relo is already running at") {
				t.Fatalf("stdout = %q, want the already-running report", stdout)
			}
		})

		t.Run("restart keeps the port the daemon held", func(t *testing.T) {
			stdout, stderr, err := runBinary(t, binary, lifecycleEnv, "daemon", "restart")
			if err != nil {
				t.Fatalf("restart error = %v (stdout %s, stderr %s)", err, stdout, stderr)
			}
			if want := fmt.Sprintf("127.0.0.1:%d", port); !strings.Contains(stdout, want) {
				t.Fatalf("stdout = %q, want the daemon back on %s", stdout, want)
			}
			waitForHealthz(t, port)
		})

		t.Run("stop ends the daemon and is safe to repeat", func(t *testing.T) {
			stdout, stderr, err := runBinary(t, binary, lifecycleEnv, "daemon", "stop")
			if err != nil {
				t.Fatalf("stop error = %v (stdout %s, stderr %s)", err, stdout, stderr)
			}
			if !strings.Contains(stdout, "Relo stopped") {
				t.Fatalf("stdout = %q, want the stop report", stdout)
			}
			waitForPortClosed(t, port)
			stdout, _, err = runBinary(t, binary, lifecycleEnv, "daemon", "stop")
			if err != nil {
				t.Fatalf("second stop error = %v", err)
			}
			if !strings.Contains(stdout, "Relo is not running") {
				t.Fatalf("stdout = %q, want the not-running report", stdout)
			}
		})
	})

	t.Run("serve starts, answers, and stops on interrupt", func(t *testing.T) {
		serveHome := newHome(t)
		writeServeConfig(t, serveHome)
		port := freePort(t)
		command := exec.Command(binary, "daemon", "run",
			"--home", serveHome,
			"--port", fmt.Sprint(port),
		)
		command.Env = commandEnv(serveHome)
		stderr := &strings.Builder{}
		command.Stderr = stderr
		if err := command.Start(); err != nil {
			t.Fatalf("start the daemon: %v", err)
		}
		stopped := make(chan error, 1)
		go func() { stopped <- command.Wait() }()
		t.Cleanup(func() {
			if command.Process != nil {
				_ = command.Process.Kill()
			}
		})

		waitForHealthz(t, port)
		status := statusReport(t, serveHome, port)
		if status["status"] != "running" {
			t.Fatalf("status = %v, want a running daemon", status)
		}
		if status["schema_version"].(float64) != float64(sqlite.LatestSchemaVersion()) {
			t.Fatalf("schema_version = %v, want a migrated database", status["schema_version"])
		}
		if err := command.Process.Signal(os.Interrupt); err != nil {
			t.Fatalf("interrupt the daemon: %v", err)
		}
		select {
		case err := <-stopped:
			if err != nil {
				t.Fatalf("the daemon exited with %v (stderr %s)", err, stderr)
			}
		case <-time.After(20 * time.Second):
			t.Fatal("the daemon did not stop after the interrupt")
		}
	})
}

var buildOnce struct {
	sync.Once
	path string
	err  error
}

// buildBinary compiles the real binary once per test run.
func buildBinary(t *testing.T) string {
	t.Helper()
	buildOnce.Do(func() {
		directory, err := os.MkdirTemp("", "relo-e2e")
		if err != nil {
			buildOnce.err = fmt.Errorf("create the build directory: %w", err)
			return
		}
		path := filepath.Join(directory, "relo")
		if runtime.GOOS == "windows" {
			path += ".exe"
		}
		command := exec.Command("go", "build", "-o", path, "./cmd/relo")
		command.Dir = moduleRoot(t)
		output, err := command.CombinedOutput()
		if err != nil {
			buildOnce.err = fmt.Errorf("build the binary: %v: %s", err, output)
			return
		}
		buildOnce.path = path
	})
	if buildOnce.err != nil {
		t.Fatal(buildOnce.err)
	}
	return buildOnce.path
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate the test file")
	}
	return filepath.Join(filepath.Dir(thisFile), "..", "..", "..")
}

// newHome creates a private state directory and returns its path.
func newHome(t *testing.T) string {
	t.Helper()
	home := filepath.Join(t.TempDir(), "relo-home")
	if err := os.Mkdir(home, 0o700); err != nil {
		t.Fatalf("create the state directory: %v", err)
	}
	return home
}

// commandEnv gives the binary a state directory of its own, so no test
// touches the developer's home or keychain. The vault key is written into
// that directory on the first run.
func commandEnv(home string) []string {
	return append(os.Environ(), "RELO_HOME="+home, "HOME="+home)
}

func runBinary(t *testing.T, binary string, env []string, args ...string) (string, string, error) {
	t.Helper()
	command := exec.Command(binary, args...)
	command.Env = env
	stdout := &strings.Builder{}
	stderr := &strings.Builder{}
	command.Stdout = stdout
	command.Stderr = stderr
	err := command.Run()
	return stdout.String(), stderr.String(), err
}

func statusReport(t *testing.T, home string, port int) map[string]any {
	t.Helper()
	token, err := os.ReadFile(filepath.Join(home, "admin-token"))
	if err != nil {
		t.Fatalf("read the admin token: %v", err)
	}
	request, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/api/v1/status", port), nil)
	if err != nil {
		t.Fatalf("build status request: %v", err)
	}
	request.Header.Set("Authorization", "Bearer "+strings.TrimSpace(string(token)))
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("status request failed: %v", err)
	}
	defer func() { _ = response.Body.Close() }()
	var report map[string]any
	if err := json.NewDecoder(response.Body).Decode(&report); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	return report
}

func freePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("find a free port: %v", err)
	}
	defer func() { _ = listener.Close() }()
	return listener.Addr().(*net.TCPAddr).Port
}

// writeServeConfig gives one serve run its own data plane ports. The machine
// default would be whatever daemon the machine already runs, and a run that
// cannot bind its listener stops instead of answering.
func writeServeConfig(t *testing.T, home string) {
	t.Helper()
	ports := fmt.Sprintf("[server.data_plane]\nopenai = %d\nanthropic = %d\ngemini = %d\n",
		freePort(t), freePort(t), freePort(t))
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte(ports), 0o600); err != nil {
		t.Fatalf("write the serve config: %v", err)
	}
}

func waitForHealthz(t *testing.T, port int) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		response, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/healthz", port))
		if err == nil {
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("the daemon never became healthy")
}

// waitForPortClosed waits until a listener stops accepting connections, which
// is how a stopped daemon is told from one that is draining.
func waitForPortClosed(t *testing.T, port int) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		connection, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 250*time.Millisecond)
		if err != nil {
			return
		}
		_ = connection.Close()
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("the daemon still accepts connections on port %d", port)
}
