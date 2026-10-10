package pool_test

import (
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/account"
	"github.com/jonaskahn/relo/tests/testkit"
)

// newConnectionPool builds a pool of two accounts over a clock the test moves.
func newConnectionPool(t *testing.T, clock *testkit.FakeClock) *account.Pool {
	t.Helper()
	pool := account.NewPoolWithOptions("openai", account.Options{
		Strategy: &account.LeastLoadedStrategy{}, Clock: clock,
		FailoverBackoff: []time.Duration{0, 3 * time.Minute, 5 * time.Minute},
	})
	pool.Add(account.PoolEntry{ID: "one", ProviderID: "openai", Status: account.StatusActive})
	pool.Add(account.PoolEntry{ID: "two", ProviderID: "openai", Status: account.StatusActive})
	return pool
}

// TestConnectionStartsOnTheAccountThatServed pins the connection's own memory:
// the account that answered is asked first again, instead of the strategy
// moving the next request to another one.
func TestConnectionStartsOnTheAccountThatServed(t *testing.T) {
	clock := testkit.NewFakeClock(time.Now())
	pool := newConnectionPool(t, clock)

	pool.MarkServed("two")
	for request := 0; request < 3; request++ {
		selected, err := pool.Select(account.Selection{})
		if err != nil {
			t.Fatalf("Select() error = %v", err)
		}
		if selected.ID != "two" {
			t.Fatalf("Select() = %q, want the account that served", selected.ID)
		}
	}
}

// TestConnectionForgetsAServedAccountOnceEveryAccountFailed pins the release:
// a connection whose every account failed no longer remembers one, and a
// success starts remembering again.
func TestConnectionForgetsAServedAccountOnceEveryAccountFailed(t *testing.T) {
	clock := testkit.NewFakeClock(time.Now())
	pool := newConnectionPool(t, clock)

	pool.MarkServed("two")
	pool.MarkExhausted(0)
	if selected, err := pool.Select(account.Selection{}); err != nil || selected.ID == "" {
		t.Fatalf("Select() = %v, %v, want an account to pick from", selected.ID, err)
	}
	pool.MarkServed("one")
	selected, err := pool.Select(account.Selection{})
	if err != nil || selected.ID != "one" {
		t.Fatalf("Select() = %q, %v, want the account that served after the failure", selected.ID, err)
	}
}

// TestConnectionCooldownEscalatesAndExpires pins the ladder a connection walks
// once every account failed: the first step of zero keeps it usable, the
// second parks it, and the wait expires on its own.
func TestConnectionCooldownEscalatesAndExpires(t *testing.T) {
	clock := testkit.NewFakeClock(time.Now())
	pool := newConnectionPool(t, clock)

	if until := pool.ConnectionCooldownUntil(); !until.IsZero() {
		t.Fatalf("ConnectionCooldownUntil() = %v, want no cooldown before a failure", until)
	}
	pool.MarkExhausted(0)
	if until := pool.ConnectionCooldownUntil(); !until.IsZero() {
		t.Fatalf("ConnectionCooldownUntil() = %v, want the zero step to keep the connection usable", until)
	}
	pool.MarkExhausted(0)
	if until := pool.ConnectionCooldownUntil(); until.IsZero() {
		t.Fatal("ConnectionCooldownUntil() = zero, want the second failure to park the connection")
	}
	clock.Add(2*time.Minute + 59*time.Second)
	if until := pool.ConnectionCooldownUntil(); until.IsZero() {
		t.Fatal("ConnectionCooldownUntil() = zero, want the three-minute step to still run")
	}
	clock.Add(2 * time.Second)
	if until := pool.ConnectionCooldownUntil(); !until.IsZero() {
		t.Fatalf("ConnectionCooldownUntil() = %v, want the step to expire", until)
	}
}

// TestConnectionCooldownHonoursARetryAfter pins a provider that named a longer
// wait than the ladder step the connection had reached.
func TestConnectionCooldownHonoursARetryAfter(t *testing.T) {
	clock := testkit.NewFakeClock(time.Now())
	pool := newConnectionPool(t, clock)

	pool.MarkExhausted(0)
	pool.MarkExhausted(10 * time.Minute)
	clock.Add(6 * time.Minute)
	if until := pool.ConnectionCooldownUntil(); until.IsZero() {
		t.Fatal("ConnectionCooldownUntil() = zero, want the provider's own wait to win")
	}
}

// TestConnectionCooldownReportsThePooledWait covers the resolution the catalog
// and the router read: a connection on its cooldown names that moment, and a
// success clears it.
func TestConnectionCooldownReportsThePooledWait(t *testing.T) {
	clock := testkit.NewFakeClock(time.Now())
	pool := newConnectionPool(t, clock)

	pool.MarkExhausted(0)
	pool.MarkExhausted(0)
	if until := pool.CooldownUntil(); until.IsZero() {
		t.Fatal("CooldownUntil() = zero, want the connection's own cooldown")
	}
	pool.MarkServed("one")
	if until := pool.CooldownUntil(); !until.IsZero() {
		t.Fatalf("CooldownUntil() = %v, want a success to clear the connection cooldown", until)
	}
}
