package activity_test

import (
	"testing"

	"github.com/jonaskahn/relo/internal/activity"
)

// TestFormatMicros covers the one place the daemon renders a cost. Micros are
// the unit every price is stored in, so the rendering is the only thing that
// keeps a spend from reading as a whole number of dollars it is not.
func TestFormatMicros(t *testing.T) {
	for _, testCase := range []struct {
		micros int64
		want   string
	}{
		{0, "$0.000000"},
		{1, "$0.000001"},
		{100_000, "$0.100000"},
		{999_999, "$0.999999"},
		{1_000_000, "$1.000000"},
		{4_250_000, "$4.250000"},
		// A negative spend is a credit, and it reads with its own sign rather
		// than as an amount the wrong way round.
		{-1, "-$0.000001"},
		{-2_000_000, "-$2.000000"},
	} {
		if got := activity.FormatMicros(testCase.micros); got != testCase.want {
			t.Errorf("FormatMicros(%d) = %q, want %q", testCase.micros, got, testCase.want)
		}
	}
}

// TestFormatMicrosPtr covers the rendering a cost that may be unknown goes
// through, which is what an unpriced model leaves behind.
func TestFormatMicrosPtr(t *testing.T) {
	if got := activity.FormatMicrosPtr(nil); got != "unknown" {
		t.Errorf("FormatMicrosPtr(nil) = %q, want the unpriced reading", got)
	}

	micros := int64(1_500_000)
	if got := activity.FormatMicrosPtr(&micros); got != "$1.500000" {
		t.Errorf("FormatMicrosPtr(%d) = %q, want the cost rendered", micros, got)
	}
	credit := int64(-500_000)
	if got := activity.FormatMicrosPtr(&credit); got != "-$0.500000" {
		t.Errorf("FormatMicrosPtr(%d) = %q, want the credit rendered", credit, got)
	}
}
