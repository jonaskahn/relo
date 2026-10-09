//go:build darwin || linux

package e2e_test

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestShutdownDeadlineSurvivesBlockedLogger(t *testing.T) {
	binary := buildBinary(t)
	home := newHome(t)
	writeServeConfig(t, home)
	port := freePort(t)
	command := exec.Command(binary, "daemon", "run", "--home", home, "--port", fmt.Sprint(port))
	command.Env = commandEnv(home)
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = command.Process.Kill() })
	waitForHealthz(t, port)
	lock, err := os.OpenFile(filepath.Join(home, "logs", "daemon.log.lock"), os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lock.Close() }()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = syscall.Flock(int(lock.Fd()), syscall.LOCK_UN) }()
	began := time.Now()
	if err := command.Process.Signal(os.Interrupt); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("blocked cleanup unexpectedly completed")
		}
		if elapsed := time.Since(began); elapsed > 1500*time.Millisecond {
			t.Fatalf("exit deadline: %v", elapsed)
		}
	case <-time.After(1500 * time.Millisecond):
		t.Fatal("blocked logger kept the daemon alive")
	}
	if reply, err := (&http.Client{Timeout: 100 * time.Millisecond}).Get(fmt.Sprintf("http://127.0.0.1:%d/healthz", port)); err == nil {
		_ = reply.Body.Close()
		t.Fatal("daemon still serves after exit")
	}
}
