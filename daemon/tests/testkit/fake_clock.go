package testkit

import (
	"sync"
	"time"

	"github.com/jonaskahn/relo/internal/clock"
)

// FakeClock is a clock.Clock that only moves when a test moves it.
type FakeClock struct {
	mu      sync.Mutex
	now     time.Time
	waiters []*waiter
	tickers []*fakeTicker
}

type waiter struct {
	deadline time.Time
	ch       chan time.Time
}

type fakeTicker struct {
	owner    *FakeClock
	interval time.Duration
	next     time.Time
	ch       chan time.Time
	stopped  bool
}

// NewFakeClock returns a clock frozen at now.
func NewFakeClock(now time.Time) *FakeClock {
	return &FakeClock{now: now}
}

// Set moves the clock to t and fires everything that became due.
func (f *FakeClock) Set(t time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = t
	f.fireDue()
}

// Add advances the clock by d and fires everything that became due.
func (f *FakeClock) Add(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = f.now.Add(d)
	f.fireDue()
}

// Now returns the current fake time.
func (f *FakeClock) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

// After returns a channel that receives one value once d has elapsed.
func (f *FakeClock) After(d time.Duration) <-chan time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	ch := make(chan time.Time, 1)
	f.waiters = append(f.waiters, &waiter{deadline: f.now.Add(d), ch: ch})
	return ch
}

// NewTicker returns a ticker driven by the fake clock instead of wall time.
func (f *FakeClock) NewTicker(d time.Duration) clock.Ticker {
	f.mu.Lock()
	defer f.mu.Unlock()
	ticker := &fakeTicker{owner: f, interval: d, next: f.now.Add(d), ch: make(chan time.Time, 1)}
	f.tickers = append(f.tickers, ticker)
	return ticker
}

func (f *FakeClock) fireDue() {
	pending := f.waiters[:0]
	for _, w := range f.waiters {
		if w.deadline.After(f.now) {
			pending = append(pending, w)
			continue
		}
		sendLatest(w.ch, f.now)
	}
	f.waiters = pending
	for _, ticker := range f.tickers {
		ticker.advance(f.now)
	}
}

func (f *FakeClock) stopTicker(target *fakeTicker) {
	f.mu.Lock()
	defer f.mu.Unlock()
	target.stopped = true
}

func (t *fakeTicker) C() <-chan time.Time {
	return t.ch
}

func (t *fakeTicker) Stop() {
	t.owner.stopTicker(t)
}

func (t *fakeTicker) advance(now time.Time) {
	if t.stopped || t.next.After(now) {
		return
	}
	sendLatest(t.ch, now)
	for !t.next.After(now) {
		t.next = t.next.Add(t.interval)
	}
}

func sendLatest(ch chan time.Time, t time.Time) {
	select {
	case ch <- t:
	default:
	}
}
