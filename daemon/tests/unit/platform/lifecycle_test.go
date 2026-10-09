package platform_test

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/platform"
)

const sleepEnv = "RELO_TEST_SLEEP"

func TestMain(m *testing.M) {
	if os.Getenv(sleepEnv) == "1" {
		select {}
	}
	if os.Getenv("RELO_TEST_HOLD_CLAIM") == "1" {
		holdClaim()
		return
	}
	if os.Getenv("RELO_TEST_HOLD_PORTS") != "" {
		holdPorts(os.Getenv("RELO_TEST_HOLD_PORTS"))
		return
	}
	os.Exit(m.Run())
}

func TestKillDaemon(t *testing.T) {
	t.Run("kills the published Relo pid", func(t *testing.T) {
		cmd := exec.Command(os.Args[0])
		cmd.Env = append(os.Environ(), sleepEnv+"=1")
		if err := cmd.Start(); err != nil {
			t.Fatalf("start the stand-in: %v", err)
		}
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		deadline := time.Now().Add(2 * time.Second)
		for {
			err := platform.KillDaemon(platform.Runtime{PID: cmd.Process.Pid, Address: "127.0.0.1:1", InstanceID: "one"})
			if err != nil {
				t.Fatalf("KillDaemon() error = %v", err)
			}
			select {
			case <-done:
				return
			default:
			}
			if time.Now().After(deadline) {
				t.Fatal("the Relo pid was not killed")
			}
			time.Sleep(20 * time.Millisecond)
		}
	})

	t.Run("refuses a pid whose executable is not Relo", func(t *testing.T) {
		cmd := startDecoy(t)
		err := platform.KillDaemon(platform.Runtime{PID: cmd.Process.Pid, Address: "127.0.0.1:1", InstanceID: "one"})
		if !errors.Is(err, platform.ErrNotReloProcess) {
			t.Fatalf("KillDaemon() error = %v, want %v", err, platform.ErrNotReloProcess)
		}
		if cmd.ProcessState != nil && cmd.ProcessState.Exited() {
			t.Fatal("the decoy was killed")
		}
	})

	t.Run("a missing pid is not an error", func(t *testing.T) {
		if err := platform.KillDaemon(platform.Runtime{}); err != nil {
			t.Fatalf("KillDaemon() error = %v", err)
		}
	})
}

func TestWaitForShutdownHoldsUntilEveryListenerCloses(t *testing.T) {
	management := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	})}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() { _ = management.Serve(listener) }()
	t.Cleanup(func() { _ = management.Close() })

	extra, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	published := platform.Runtime{
		Address: listener.Addr().String(), InstanceID: "one",
		Listeners: []string{listener.Addr().String(), extra.Addr().String()},
	}

	held, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	err = platform.WaitForShutdown(held, published)
	if !errors.Is(err, platform.ErrDaemonStillUp) {
		t.Fatalf("WaitForShutdown() error = %v, want %v", err, platform.ErrDaemonStillUp)
	}

	if err := extra.Close(); err != nil {
		t.Fatalf("close the extra listener: %v", err)
	}
	if err := management.Close(); err != nil {
		t.Fatal(err)
	}
	freed, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()
	if err := platform.WaitForShutdown(freed, published); err != nil {
		t.Fatalf("WaitForShutdown() error = %v", err)
	}
}

func startDecoy(t *testing.T) *exec.Cmd {
	t.Helper()
	name, args := "sleep", []string{"30"}
	if runtime.GOOS == "windows" {
		name, args = "ping", []string{"-n", "30", "127.0.0.1"}
	}
	cmd := exec.Command(name, args...)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start the decoy: %v", err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill() })
	return cmd
}
