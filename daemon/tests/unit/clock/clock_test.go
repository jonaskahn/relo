package clock_test

import (
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/clock"
	"github.com/jonaskahn/relo/tests/testkit"
)

var _ clock.Clock = clock.New()
var _ clock.Clock = testkit.NewFakeClock(time.Unix(0, 0))

func TestRealClockNowReturnsCurrentTime(t *testing.T) {
	real := clock.New()
	before := time.Now()
	now := real.Now()
	after := time.Now()
	if now.Before(before) || now.After(after) {
		t.Fatalf("Now() = %v, want between %v and %v", now, before, after)
	}
}

func TestRealClockAfterFires(t *testing.T) {
	select {
	case fired := <-clock.New().After(time.Millisecond):
		if fired.IsZero() {
			t.Fatal("After() delivered a zero time")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("After() never fired")
	}
}

func TestRealClockTickerTicksAndStops(t *testing.T) {
	ticker := clock.New().NewTicker(time.Millisecond)
	select {
	case <-ticker.C():
	case <-time.After(5 * time.Second):
		t.Fatal("ticker never ticked")
	}
	ticker.Stop()
}

func TestFakeClockSetControlsNow(t *testing.T) {
	fake := testkit.NewFakeClock(time.Unix(1000, 0))
	fake.Set(time.Unix(2000, 0))
	if got := fake.Now(); !got.Equal(time.Unix(2000, 0)) {
		t.Fatalf("Now() = %v, want %v", got, time.Unix(2000, 0))
	}
}

func TestFakeClockAddAdvancesTime(t *testing.T) {
	fake := testkit.NewFakeClock(time.Unix(1000, 0))
	fake.Add(90 * time.Second)
	if got := fake.Now(); !got.Equal(time.Unix(1090, 0)) {
		t.Fatalf("Now() = %v, want %v", got, time.Unix(1090, 0))
	}
}

func TestFakeClockAfterFiresOnlyWhenDue(t *testing.T) {
	fake := testkit.NewFakeClock(time.Unix(1000, 0))
	fired := fake.After(time.Minute)
	assertQuiet(t, fired)
	fake.Add(30 * time.Second)
	assertQuiet(t, fired)
	fake.Add(30 * time.Second)
	assertDelivered(t, fired, time.Unix(1060, 0))
	assertQuiet(t, fired)
}

func TestFakeClockTickerTicksOnAdvance(t *testing.T) {
	fake := testkit.NewFakeClock(time.Unix(1000, 0))
	ticker := fake.NewTicker(time.Second)
	assertQuiet(t, ticker.C())
	fake.Add(3 * time.Second)
	assertDelivered(t, ticker.C(), time.Unix(1003, 0))
	ticker.Stop()
	fake.Add(time.Minute)
	assertQuiet(t, ticker.C())
}

func assertQuiet(t *testing.T, ch <-chan time.Time) {
	t.Helper()
	select {
	case value := <-ch:
		t.Fatalf("channel delivered %v while nothing was due", value)
	default:
	}
}

func assertDelivered(t *testing.T, ch <-chan time.Time, want time.Time) {
	t.Helper()
	select {
	case value := <-ch:
		if !value.Equal(want) {
			t.Fatalf("channel delivered %v, want %v", value, want)
		}
	case <-time.After(time.Second):
		t.Fatalf("channel never delivered %v", want)
	}
}
