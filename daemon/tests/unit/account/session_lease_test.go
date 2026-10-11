package pool_test

import (
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/account"
	"github.com/jonaskahn/relo/tests/testkit"
)

func TestSessionLeases(t *testing.T) {
	t.Run("one client key keeps one session across requests", func(t *testing.T) {
		leases := account.NewSessionLeases(0, 0, testkit.NewFakeClock(time.Now()))
		key := account.SessionKey{ClientKind: "claude-code", ClientKey: "key-1"}
		first := leases.Lease(key)
		again := leases.Lease(key)
		if first == "" || first != again {
			t.Fatalf("Lease() = %q then %q, want one stable id", first, again)
		}
	})

	t.Run("separate client keys get separate sessions", func(t *testing.T) {
		leases := account.NewSessionLeases(0, 0, testkit.NewFakeClock(time.Now()))
		one := leases.Lease(account.SessionKey{ClientKind: "claude-code", ClientKey: "key-1"})
		other := leases.Lease(account.SessionKey{ClientKind: "claude-code", ClientKey: "key-2"})
		if one == other {
			t.Fatalf("Lease() returned %q for two client keys", one)
		}
	})

	t.Run("separate client kinds get separate sessions", func(t *testing.T) {
		leases := account.NewSessionLeases(0, 0, testkit.NewFakeClock(time.Now()))
		claude := leases.Lease(account.SessionKey{ClientKind: "claude-code", ClientKey: "key-1"})
		codex := leases.Lease(account.SessionKey{ClientKind: "codex", ClientKey: "key-1"})
		if claude == codex {
			t.Fatalf("Lease() returned %q for two client kinds", claude)
		}
	})

	t.Run("an idle lease expires and is replaced", func(t *testing.T) {
		clock := testkit.NewFakeClock(time.Now())
		leases := account.NewSessionLeases(0, time.Minute, clock)
		key := account.SessionKey{ClientKind: "opencode", ClientKey: "key-1"}
		first := leases.Lease(key)
		clock.Add(time.Minute + time.Second)
		if second := leases.Lease(key); second == first {
			t.Fatalf("Lease() = %q after the ttl, want a fresh id", second)
		}
	})

	t.Run("a lease in use never expires", func(t *testing.T) {
		clock := testkit.NewFakeClock(time.Now())
		leases := account.NewSessionLeases(0, time.Minute, clock)
		key := account.SessionKey{ClientKind: "opencode", ClientKey: "key-1"}
		first := leases.Lease(key)
		for range 5 {
			clock.Add(30 * time.Second)
			if again := leases.Lease(key); again != first {
				t.Fatalf("Lease() = %q, want %q while in use", again, first)
			}
		}
	})

	t.Run("an evicted lease gets a fresh session", func(t *testing.T) {
		leases := account.NewSessionLeases(0, 0, testkit.NewFakeClock(time.Now()))
		key := account.SessionKey{ClientKind: "codex", ClientKey: "key-1"}
		before := leases.Lease(key)
		leases.Evict(key)
		if after := leases.Lease(key); after == before {
			t.Fatalf("Lease() = %q after Evict, want a fresh id", after)
		}
	})

	t.Run("the store stays bounded", func(t *testing.T) {
		leases := account.NewSessionLeases(3, 0, testkit.NewFakeClock(time.Now()))
		for _, id := range []string{"a", "b", "c", "d", "e"} {
			leases.Lease(account.SessionKey{ClientKind: "claude-code", ClientKey: id})
		}
		if got := leases.Len(); got > 3 {
			t.Fatalf("Len() = %d, want at most 3", got)
		}
	})

	t.Run("a nil store still answers with a session", func(t *testing.T) {
		var leases *account.SessionLeases
		if got := leases.Lease(account.SessionKey{ClientKind: "codex"}); got == "" {
			t.Fatal("Lease() on a nil store = \"\", want a minted session")
		}
	})
}

func TestLeasedSessionIDShape(t *testing.T) {
	const shape = "xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx"
	leases := account.NewSessionLeases(0, 0, testkit.NewFakeClock(time.Now()))
	for range 20 {
		id := leases.Lease(account.SessionKey{ClientKind: "codex", ClientKey: "key-1"})
		leases.Evict(account.SessionKey{ClientKind: "codex", ClientKey: "key-1"})
		if len(id) != len(shape) {
			t.Fatalf("Lease() = %q, want the UUID shape", id)
		}
		for index, char := range id {
			if shape[index] == 'x' || shape[index] == 'y' {
				continue
			}
			if rune(shape[index]) != char {
				t.Fatalf("Lease() = %q, want %q at position %d", id, shape[index], index)
			}
		}
	}
}
