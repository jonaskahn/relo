package platform_test

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

const holdPortsEnv = "RELO_TEST_HOLD_PORTS"

// holdPorts listens on the comma-separated specs a test names and blocks
// until killed, so the sweep has a foreign listener to end. A spec is a
// listen address, with :0 picking a free port. The chosen ports are printed
// first, so the parent learns them without guessing.
func holdPorts(specs string) {
	var listeners []net.Listener
	var ports []string
	for _, spec := range strings.Split(specs, ",") {
		spec = strings.TrimSpace(spec)
		listener, err := net.Listen("tcp", spec)
		if err != nil {
			fmt.Fprintln(os.Stderr, "bind "+spec+": "+err.Error())
			os.Exit(2)
		}
		listeners = append(listeners, listener)
	}
	for _, listener := range listeners {
		ports = append(ports, strconv.Itoa(listener.Addr().(*net.TCPAddr).Port))
	}
	fmt.Println("ready " + strings.Join(ports, ","))
	// A sleep loop rather than select{}: with no other goroutine the
	// detector would read an empty select as a deadlock and exit.
	for {
		time.Sleep(time.Hour)
	}
}

// spawnForeignHolder starts a port holder from a copy of the test binary, so
// the process table does not mistake it for this run the way it does a
// re-executed test binary: ownership checks treat the copy as a foreign
// program.
func spawnForeignHolder(t *testing.T, specs ...string) (*exec.Cmd, []int) {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve the test binary: %v", err)
	}
	data, err := os.ReadFile(self)
	if err != nil {
		t.Fatalf("read the test binary: %v", err)
	}
	copy := filepath.Join(t.TempDir(), "foreign-holder")
	if err := os.WriteFile(copy, data, 0o700); err != nil {
		t.Fatalf("copy the test binary: %v", err)
	}
	return spawnHolder(t, copy, specs...)
}

func spawnHolder(t *testing.T, binary string, specs ...string) (*exec.Cmd, []int) {
	t.Helper()
	cmd := exec.Command(binary, "-test.run=^$")
	cmd.Env = append(os.Environ(), holdPortsEnv+"="+strings.Join(specs, ","))
	output, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start the port holder: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	})
	ready := make(chan string, 1)
	go func() {
		line, err := bufio.NewReader(output).ReadString('\n')
		if err != nil {
			ready <- ""
			return
		}
		ready <- line
	}()
	select {
	case line := <-ready:
		ports := parseHeldPorts(t, line)
		if len(ports) != len(specs) {
			t.Fatalf("holder bound %v for specs %q", ports, specs)
		}
		return cmd, ports
	case <-time.After(10 * time.Second):
		t.Fatal("the port holder never reported its listeners")
		return nil, nil
	}
}

func parseHeldPorts(t *testing.T, line string) []int {
	t.Helper()
	ports := strings.TrimPrefix(strings.TrimSpace(line), "ready ")
	if ports == "" || line == "" {
		t.Fatalf("holder reported %q, want its bound ports", line)
	}
	var out []int
	for _, raw := range strings.Split(ports, ",") {
		port, err := strconv.Atoi(strings.TrimSpace(raw))
		if err != nil || port <= 0 {
			t.Fatalf("holder reported %q, want its bound ports", line)
		}
		out = append(out, port)
	}
	return out
}

func portAccepts(address string) bool {
	conn, err := net.DialTimeout("tcp", address, 200*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}
