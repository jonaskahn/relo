// Idle bounds a provider call by silence: the response headers must arrive
// within the call wait, and a body that sends no byte for that long is a
// stall. A stream that keeps sending runs until the provider ends it.
package upstream

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"
)

// ErrUpstreamStall reports a provider body that sent no bytes for the whole
// call wait, so the attempt is a retryable timeout rather than an empty
// completion.
var ErrUpstreamStall = errors.New("the upstream sent no bytes within the call wait")

type headerClock struct {
	timer  *time.Timer
	done   chan struct{}
	fired  bool
	cancel context.CancelFunc
}

func startHeaderClock(parent context.Context, wait time.Duration) (*headerClock, context.Context) {
	ctx, cancel := context.WithCancel(parent)
	clock := &headerClock{done: make(chan struct{}), cancel: cancel}
	if wait <= 0 {
		close(clock.done)
		return clock, ctx
	}
	clock.timer = time.AfterFunc(wait, func() {
		clock.fired = true
		cancel()
		close(clock.done)
	})
	return clock, ctx
}

func (c *headerClock) defused() bool {
	if c.timer == nil {
		return false
	}
	if c.timer.Stop() {
		return false
	}
	<-c.done
	return c.fired
}

func (c *headerClock) release() {
	if c.timer != nil {
		c.timer.Stop()
	}
	c.cancel()
}

type idleBody struct {
	body      io.ReadCloser
	parent    context.Context
	wait      time.Duration
	release   context.CancelFunc
	mu        sync.Mutex
	timer     *time.Timer
	fired     bool
	stopped   chan struct{}
	stopOnce  sync.Once
	closeOnce sync.Once
}

func newIdleBody(parent context.Context, body io.ReadCloser, wait time.Duration, release context.CancelFunc) *idleBody {
	idle := &idleBody{body: body, parent: parent, wait: wait, release: release, stopped: make(chan struct{})}
	go idle.watchParent()
	return idle
}

func (b *idleBody) watchParent() {
	select {
	case <-b.parent.Done():
		_ = b.closeUnderlying()
	case <-b.stopped:
	}
}

// Read serves one upstream body while its request lives, reporting the
// client's cancellation as the read error once it arrives.
func (b *idleBody) Read(p []byte) (int, error) {
	if b.wait <= 0 {
		return b.body.Read(p)
	}
	if err := b.parent.Err(); err != nil {
		return 0, err
	}
	b.armStallTimer()
	n, err := b.body.Read(p)
	fired := b.disarmStallTimer()
	return b.stallResult(n, err, fired)
}

func (b *idleBody) armStallTimer() {
	b.mu.Lock()
	b.fired = false
	b.timer = time.AfterFunc(b.wait, b.expire)
	b.mu.Unlock()
}

func (b *idleBody) disarmStallTimer() bool {
	b.mu.Lock()
	fired := b.fired || !b.timer.Stop()
	b.fired = fired
	b.timer = nil
	b.mu.Unlock()
	return fired
}

func (b *idleBody) stallResult(n int, err error, fired bool) (int, error) {
	if !fired {
		if err != nil {
			if parentErr := b.parent.Err(); parentErr != nil {
				return n, parentErr
			}
		}
		return n, err
	}
	if parentErr := b.parent.Err(); parentErr != nil {
		return n, parentErr
	}
	return n, fmt.Errorf("%w: no bytes for %s", ErrUpstreamStall, b.wait)
}

func (b *idleBody) expire() {
	b.mu.Lock()
	b.fired = true
	b.mu.Unlock()
	_ = b.closeUnderlying()
}

// Close releases the underlying body, exactly once however often it is called.
func (b *idleBody) Close() error {
	b.stopOnce.Do(func() { close(b.stopped) })
	b.mu.Lock()
	if b.timer != nil {
		b.timer.Stop()
		b.timer = nil
	}
	b.mu.Unlock()
	if b.release != nil {
		b.release()
	}
	return b.closeUnderlying()
}

func (b *idleBody) closeUnderlying() error {
	var err error
	b.closeOnce.Do(func() { err = b.body.Close() })
	return err
}
