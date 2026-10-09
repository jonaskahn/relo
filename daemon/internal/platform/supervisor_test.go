package platform

import (
	"testing"
	"time"
)

func TestStopCancelsBeforeLifecycleActionFinishes(t *testing.T) {
	supervisor := NewSupervisor(SupervisorOptions{})
	if !supervisor.begin() {
		t.Fatal("could not begin lifecycle action")
	}
	if supervisor.begin() {
		t.Fatal("a second lifecycle action was accepted")
	}
	cancelled := make(chan struct{})
	supervisor.setRun(func() {
		close(cancelled)
		supervisor.clearRun()
	})
	stopped := make(chan error, 1)
	go func() { stopped <- supervisor.Stop() }()
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("Stop waited for the action before cancelling")
	}
	select {
	case err := <-stopped:
		t.Errorf("Stop returned during a lifecycle action: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	supervisor.end()
	select {
	case err := <-stopped:
		if err != nil {
			t.Fatalf("Stop failed: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Stop did not finish after the lifecycle action")
	}
	select {
	case <-cancelled:
	default:
		t.Fatal("Stop did not cancel the daemon")
	}
}
