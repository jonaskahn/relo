// Outcome bookkeeping leaves the request path: what a relayed response means
// for its credential, its model access, and its quota reaches the stores
// behind the response rather than in front of it.
package server

import (
	"context"
	"errors"
	"sync"
)

const bookkeepingQueue = 512

type bookkeeping struct {
	jobs chan func()
	quit chan struct{}
	done chan struct{}

	mu      sync.Mutex
	started bool
	stopped bool
	aborted bool
}

func newBookkeeping() *bookkeeping {
	return &bookkeeping{
		jobs: make(chan func(), bookkeepingQueue),
		quit: make(chan struct{}),
		done: make(chan struct{}),
	}
}

// enqueue hands one job to the worker and reports whether it was taken. A full
// queue drops the job rather than waiting, so bookkeeping never holds a
// response back.
func (b *bookkeeping) enqueue(job func()) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.stopped {
		return false
	}
	if !b.started {
		b.started = true
		go b.run()
	}
	select {
	case b.jobs <- job:
		return true
	default:
		return false
	}
}

// enqueueOrRun hands the job to the worker, and runs it here when the queue is
// full or the worker has stopped. A usage row must not be dropped, and the
// caller is past the response, so falling back to this goroutine trades a
// little latency under load for the row; a dedicated writer with backpressure
// is the upgrade path if that ever shows.
func (b *bookkeeping) enqueueOrRun(job func()) {
	if !b.enqueue(job) {
		job()
	}
}

// flush waits until every job queued so far has run, which is what a store
// assertion after a response waits for. A queue that cannot take the barrier
// reports it rather than waiting for a job that was never queued.
func (b *bookkeeping) flush(ctx context.Context) error {
	done := make(chan struct{})
	if !b.enqueue(func() { close(done) }) {
		if b.isStopped() {
			return nil
		}
		return errors.New("bookkeeping queue is full")
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (b *bookkeeping) run() {
	defer close(b.done)
	for {
		select {
		case job := <-b.jobs:
			if !b.isAborted() {
				job()
			}
		case <-b.quit:
			for {
				select {
				case job := <-b.jobs:
					if !b.isAborted() {
						job()
					}
				default:
					return
				}
			}
		}
	}
}

func (b *bookkeeping) stop(ctx context.Context) error {
	b.mu.Lock()
	if !b.stopped {
		b.stopped = true
		close(b.quit)
	}
	started := b.started
	b.mu.Unlock()
	if !started {
		return nil
	}
	select {
	case <-b.done:
		return nil
	case <-ctx.Done():
		b.mu.Lock()
		b.aborted = true
		b.mu.Unlock()
		return ctx.Err()
	}
}

func (b *bookkeeping) isAborted() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.aborted
}

func (b *bookkeeping) isStopped() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.stopped
}
