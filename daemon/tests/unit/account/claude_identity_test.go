package pool_test

import (
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/account"
)

func TestClaudeSessionID(t *testing.T) {
	t.Run("a native uuid passes through", func(t *testing.T) {
		id := "fcb4e0be-b97b-42f9-ac38-522c1444bd98"
		if got := account.ClaudeSessionID("claude", id); got != id {
			t.Fatalf("session = %q, want the native id", got)
		}
	})

	t.Run("one anchor always maps to one session", func(t *testing.T) {
		first := account.ClaudeSessionID("claude", "ses_ed417ca14ffe1y0NFcyusdkUpM")
		again := account.ClaudeSessionID("claude", "ses_ed417ca14ffe1y0NFcyusdkUpM")
		if first != again {
			t.Fatalf("sessions = %q and %q, want one stable id", first, again)
		}
		if len(first) != 36 || strings.Count(first, "-") != 4 {
			t.Fatalf("session = %q, want uuid shape", first)
		}
	})

	t.Run("separate anchors and namespaces stay separate", func(t *testing.T) {
		if got := account.ClaudeSessionID("claude", "ses_one"); got == account.ClaudeSessionID("claude", "ses_two") {
			t.Fatalf("sessions collide at %q", got)
		}
		if got := account.ClaudeSessionID("claude", "ses_one"); got == account.ClaudeSessionID("other", "ses_one") {
			t.Fatalf("namespaces collide at %q", got)
		}
	})
}

func TestClaudeDeviceID(t *testing.T) {
	t.Run("one seed and account always derive one device", func(t *testing.T) {
		first := account.ClaudeDeviceID("seed-1", "acct-1")
		if again := account.ClaudeDeviceID("seed-1", "acct-1"); first != again || len(first) != 64 {
			t.Fatalf("devices = %q and %q, want one 64-char id", first, again)
		}
		if other := account.ClaudeDeviceID("seed-1", "acct-2"); other == first {
			t.Fatalf("accounts collide at %q", other)
		}
		if other := account.ClaudeDeviceID("seed-2", "acct-1"); other == first {
			t.Fatalf("seeds collide at %q", other)
		}
	})
}
