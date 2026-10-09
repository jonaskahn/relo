// Outcome bookkeeping leaves the request path: what a relayed response means
// for its credential, its model access, and its quota reaches the stores
// behind the response rather than in front of it.
package server

import (
	"context"
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

func (b *bookkeeping) enqueue(job func()) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.stopped {
		return
	}
	if !b.started {
		b.started = true
		go b.run()
	}
	select {
	case b.jobs <- job:
	default:
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
