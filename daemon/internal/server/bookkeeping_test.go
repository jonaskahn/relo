package server

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// TestBookkeepingDrainsInArrivalOrder covers what the relay leaves behind:
// the writer runs every accepted job in the order it arrived and finishes
// the ones already queued when the process stops.
func TestBookkeepingDrainsInArrivalOrder(t *testing.T) {
	worker := newBookkeeping()
	var sequence atomic.Int64
	var outOfOrder atomic.Bool
	for i := 0; i < 100; i++ {
		expected := int64(i + 1)
		worker.enqueue(func() {
			if sequence.Add(1) != expected {
				outOfOrder.Store(true)
			}
		})
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := worker.stop(ctx); err != nil {
		t.Fatalf("stop() error = %v", err)
	}
	if got := sequence.Load(); got != 100 {
		t.Fatalf("jobs finished = %d, want the queue drained", got)
	}
	if outOfOrder.Load() {
		t.Fatal("a job ran outside the order it arrived in")
	}
}

// TestBookkeepingTakesNoWorkAfterStop covers the handover at shutdown: a job
// that arrives once the writer stopped is dropped rather than run later.
func TestBookkeepingTakesNoWorkAfterStop(t *testing.T) {
	worker := newBookkeeping()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := worker.stop(ctx); err != nil {
		t.Fatalf("stop() error = %v", err)
	}
	ran := make(chan struct{}, 1)
	worker.enqueue(func() { ran <- struct{}{} })
	worker.enqueue(func() { ran <- struct{}{} })
	select {
	case <-ran:
		t.Fatal("a job ran after the writer stopped")
	case <-time.After(50 * time.Millisecond):
	}
	if err := worker.stop(ctx); err != nil {
		t.Fatalf("the second stop() error = %v", err)
	}
}

// TestBookkeepingDropsRatherThanBlockTheRelay covers the overflow rule: the
// queue is bounded, and a writer that falls behind costs bookkeeping rather
// than the responses still arriving.
func TestBookkeepingDropsRatherThanBlockTheRelay(t *testing.T) {
	worker := newBookkeeping()
	started := make(chan struct{})
	release := make(chan struct{})
	worker.enqueue(func() {
		close(started)
		<-release
	})
	<-started

	var ran atomic.Int64
	for i := 0; i < bookkeepingQueue+10; i++ {
		worker.enqueue(func() { ran.Add(1) })
	}
	close(release)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := worker.stop(ctx); err != nil {
		t.Fatalf("stop() error = %v", err)
	}
	if got := ran.Load(); got != bookkeepingQueue {
		t.Fatalf("jobs run = %d, want the queue capacity %d and the overflow dropped", got, bookkeepingQueue)
	}
}
