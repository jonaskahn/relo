package wire

import (
	"testing"
	"time"
)

// TestNativeIdentifierBodyMatchesTheClient pins the port against the
// TypeScript formula (packages/schema/src/identifier.ts in opencode): fixed
// clock, fixed counter, fixed tail bytes, values computed with node from the
// original code.
func TestNativeIdentifierBodyMatchesTheClient(t *testing.T) {
	tail := []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13}
	const ms = 1792078000000
	if got := "ses_" + nativeIdentifierBody(true, ms, 1, tail); got != "ses_ebfd3d07fffe0123456789ABCD" {
		t.Fatalf("session = %q, want the client value", got)
	}
	if got := "msg_" + nativeIdentifierBody(false, ms, 1, tail); got != "msg_1402c2f800010123456789ABCD" {
		t.Fatalf("request = %q, want the client value", got)
	}
	if got := "ses_" + nativeIdentifierBody(true, ms, 2, tail); got != "ses_ebfd3d07fffd0123456789ABCD" {
		t.Fatalf("session = %q, want the counter to advance the value", got)
	}
}

// TestProjectForgeRotatesOnARandomSchedule covers the 3-20 minute rotation:
// stable inside an epoch, fresh across one, every interval inside the bound,
// and the schedule actually varying instead of fixed.
func TestProjectForgeRotatesOnARandomSchedule(t *testing.T) {
	now := time.Now()
	forge := &ProjectForge{}
	forge.now = func() time.Time { return now }
	first := forge.Get()
	if got := forge.Get(); got != first {
		t.Fatalf("project changed from %q to %q inside its epoch", first, got)
	}
	epoch := forge.rotateAt.Sub(now)
	if epoch < projectForgeMin || epoch > projectForgeMin+projectForgeSpan {
		t.Fatalf("epoch = %v, want between 3 and 20 minutes", epoch)
	}
	now = forge.rotateAt.Add(time.Second)
	if got := forge.Get(); got == first {
		t.Fatal("project did not rotate past its epoch")
	}

	min, max := 25*time.Minute, time.Duration(0)
	distinct := map[time.Duration]bool{}
	for range 500 {
		now = forge.rotateAt.Add(time.Second)
		at := now
		forge.Get()
		span := forge.rotateAt.Sub(at)
		distinct[span] = true
		if span < projectForgeMin || span > projectForgeMin+projectForgeSpan {
			t.Fatalf("interval = %v, want between 3 and 20 minutes", span)
		}
		if span < min {
			min = span
		}
		if span > max {
			max = span
		}
	}
	if max-min < time.Minute {
		t.Fatalf("interval spread = %v, want real variance across 3-20 minutes", max-min)
	}
	if len(distinct) < 100 {
		t.Fatalf("only %d distinct intervals in 500 rotations, want the schedule to vary", len(distinct))
	}
}
