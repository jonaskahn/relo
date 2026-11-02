package pool_test

import (
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/account"
)

func TestAccountSessionID(t *testing.T) {
	t.Run("one conversation on one account always maps to one session", func(t *testing.T) {
		first := account.AccountSessionID("claude", "a", "ses_ed417ca14ffe1y0NFcyusdkUpM")
		again := account.AccountSessionID("claude", "a", "ses_ed417ca14ffe1y0NFcyusdkUpM")
		if first != again {
			t.Fatalf("sessions = %q and %q, want one stable id", first, again)
		}
		if len(first) != 36 || strings.Count(first, "-") != 4 {
			t.Fatalf("session = %q, want uuid shape", first)
		}
	})

	t.Run("a conversation keeps a separate id on each account", func(t *testing.T) {
		onA := account.AccountSessionID("claude", "a", "ses_one")
		onB := account.AccountSessionID("claude", "b", "ses_one")
		if onA == onB {
			t.Fatalf("accounts share session %q", onA)
		}
		if back := account.AccountSessionID("claude", "a", "ses_one"); back != onA {
			t.Fatalf("session = %q after switching back, want the id %q the account knew", back, onA)
		}
	})

	t.Run("separate anchors and connections stay separate", func(t *testing.T) {
		if got := account.AccountSessionID("claude", "a", "ses_one"); got == account.AccountSessionID("claude", "a", "ses_two") {
			t.Fatalf("sessions collide at %q", got)
		}
		if got := account.AccountSessionID("claude", "a", "ses_one"); got == account.AccountSessionID("other", "a", "ses_one") {
			t.Fatalf("connections collide at %q", got)
		}
	})

	t.Run("no conversation yields no session", func(t *testing.T) {
		if got := account.AccountSessionID("claude", "a", " "); got != "" {
			t.Fatalf("session = %q, want none", got)
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
