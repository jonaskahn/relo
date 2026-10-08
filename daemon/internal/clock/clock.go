// Package clock isolates Relo from wall time so tests can drive it.
package clock

import "time"

// Clock is the time source every long-lived component receives.
type Clock interface {
	Now() time.Time
	After(d time.Duration) <-chan time.Time
	NewTicker(d time.Duration) Ticker
}

// Ticker is the subset of time.Ticker that Relo uses.
type Ticker interface {
	C() <-chan time.Time
	Stop()
}

// Real is the production clock, backed by the standard library.
type Real struct{}

// New returns the production clock.
func New() Real {
	return Real{}
}

// Now returns the current wall time.
func (Real) Now() time.Time {
	return time.Now()
}

// After waits for d to elapse on the wall clock.
func (Real) After(d time.Duration) <-chan time.Time {
	return time.After(d)
}

// NewTicker starts a wall-clock ticker.
func (Real) NewTicker(d time.Duration) Ticker {
	return realTicker{time.NewTicker(d)}
}

type realTicker struct {
	ticker *time.Ticker
}

// C is the channel a tick arrives on.
func (t realTicker) C() <-chan time.Time {
	return t.ticker.C
}

// Stop releases the underlying ticker.
func (t realTicker) Stop() {
	t.ticker.Stop()
}
