package wire

import (
	"errors"
	"strings"
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

// failingReader stands in for exhausted randomness, the only way the
// fallback sources below are exercised.
type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("no randomness") }

// TestIdentifierMintSurvivesRandomnessFailure covers the fallback that keeps
// minting a shape-correct identifier when crypto randomness fails.
func TestIdentifierMintSurvivesRandomnessFailure(t *testing.T) {
	previous := randSource
	randSource = failingReader{}
	defer func() { randSource = previous }()
	for _, id := range []string{NativeSessionID(), NativeRequestID()} {
		if !ValidSessionID(id) && !(strings.HasPrefix(id, "msg_") && len(id) == 30) {
			t.Fatalf("id = %q, want a shape-correct fallback", id)
		}
	}
	if got := randInt63(1021); got != 1021/2 {
		t.Fatalf("randInt63 = %d, want the middle of the range", got)
	}
}

// TestProjectForgeKeepsTheOldIDWithoutRandomness covers a rotation attempt
// with no randomness to mint from: the current id stays instead of blanking.
func TestProjectForgeKeepsTheOldIDWithoutRandomness(t *testing.T) {
	previous := randSource
	now := time.Now()
	forge := &ProjectForge{}
	forge.now = func() time.Time { return now }
	first := forge.Get()
	if first == "" {
		t.Fatal("want an initial id from live randomness")
	}
	randSource = failingReader{}
	defer func() { randSource = previous }()
	now = forge.rotateAt.Add(time.Second)
	if got := forge.Get(); got != first {
		t.Fatalf("project = %q, want the previous %q kept", got, first)
	}
}

// stable inside an epoch, fresh across one, every interval inside the bound,
// and the schedule actually varying instead of fixed.
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
