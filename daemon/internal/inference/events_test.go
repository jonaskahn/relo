package inference

import "testing"

// TestEventMessageStartString pins the canonical name a client-facing log
// reads for the upstream identity event.
func TestEventMessageStartString(t *testing.T) {
	if got := EventMessageStart.String(); got != "message_start" {
		t.Fatalf("EventMessageStart.String() = %q, want message_start", got)
	}
}
