package pool_test

import (
	"net/http"
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

// TestModelCooldownParksOnlyThatModel pins the scope of the wait: a model that
// failed every account of a connection sits out, while the other models of the
// same connection keep being asked.
func TestModelCooldownParksOnlyThatModel(t *testing.T) {
	clock := testkit.NewFakeClock(time.Now())
	pool := newConnectionPool(t, clock)

	pool.MarkModelUnavailable("alpha", 0)
	pool.MarkModelUnavailable("alpha", 0)
	if until := pool.ModelCooldownUntil("alpha"); until.IsZero() {
		t.Fatal("ModelCooldownUntil(alpha) = zero, want the model parked")
	}
	if until := pool.ModelCooldownUntil("beta"); !until.IsZero() {
		t.Fatalf("ModelCooldownUntil(beta) = %v, want the other model usable", until)
	}
	if selected, err := pool.Select(account.Selection{}); err != nil || selected.ID == "" {
		t.Fatalf("Select() = %v, %v, want an account to pick from", selected.ID, err)
	}
}

// TestModelCooldownEscalatesAndExpires pins the ladder one model walks once
// every account of the connection failed it: the first step of zero keeps it
// usable, the second parks it, and the wait expires on its own.
func TestModelCooldownEscalatesAndExpires(t *testing.T) {
	clock := testkit.NewFakeClock(time.Now())
	pool := newConnectionPool(t, clock)

	if until := pool.ModelCooldownUntil("alpha"); !until.IsZero() {
		t.Fatalf("ModelCooldownUntil() = %v, want no cooldown before a failure", until)
	}
	pool.MarkModelUnavailable("alpha", 0)
	if until := pool.ModelCooldownUntil("alpha"); !until.IsZero() {
		t.Fatalf("ModelCooldownUntil() = %v, want the zero step to keep the model usable", until)
	}
	pool.MarkModelUnavailable("alpha", 0)
	if until := pool.ModelCooldownUntil("alpha"); until.IsZero() {
		t.Fatal("ModelCooldownUntil() = zero, want the second failure to park the model")
	}
	clock.Add(2*time.Minute + 59*time.Second)
	if until := pool.ModelCooldownUntil("alpha"); until.IsZero() {
		t.Fatal("ModelCooldownUntil() = zero, want the three-minute step to still run")
	}
	clock.Add(2 * time.Second)
	if until := pool.ModelCooldownUntil("alpha"); !until.IsZero() {
		t.Fatalf("ModelCooldownUntil() = %v, want the step to expire", until)
	}
}

// TestModelCooldownWalksItsOwnLadder pins that the failures of one model say
// nothing about another: a second model starts at the first step of its own.
func TestModelCooldownWalksItsOwnLadder(t *testing.T) {
	clock := testkit.NewFakeClock(time.Now())
	pool := newConnectionPool(t, clock)

	pool.MarkModelUnavailable("alpha", 0)
	pool.MarkModelUnavailable("alpha", 0)
	pool.MarkModelUnavailable("beta", 0)
	if until := pool.ModelCooldownUntil("beta"); !until.IsZero() {
		t.Fatalf("ModelCooldownUntil(beta) = %v, want the first step to keep it usable", until)
	}
	pool.MarkModelUnavailable("beta", 0)
	if until := pool.ModelCooldownUntil("beta"); until.IsZero() {
		t.Fatal("ModelCooldownUntil(beta) = zero, want the second failure to park it")
	}
}

// TestModelCooldownHonoursARetryAfter pins a provider that named a longer wait
// than the ladder step the model had reached.
func TestModelCooldownHonoursARetryAfter(t *testing.T) {
	clock := testkit.NewFakeClock(time.Now())
	pool := newConnectionPool(t, clock)

	pool.MarkModelUnavailable("alpha", 0)
	pool.MarkModelUnavailable("alpha", 10*time.Minute)
	clock.Add(6 * time.Minute)
	if until := pool.ModelCooldownUntil("alpha"); until.IsZero() {
		t.Fatal("ModelCooldownUntil() = zero, want the provider's own wait to win")
	}
}

// TestModelServedClearsOnlyThatModel pins that a model which answered ends its
// own wait and its own ladder, and leaves the other models alone.
func TestModelServedClearsOnlyThatModel(t *testing.T) {
	clock := testkit.NewFakeClock(time.Now())
	pool := newConnectionPool(t, clock)

	pool.MarkModelUnavailable("alpha", 0)
	pool.MarkModelUnavailable("alpha", 0)
	pool.MarkModelUnavailable("beta", 0)
	pool.MarkModelUnavailable("beta", 0)

	pool.MarkModelServed("alpha")
	if until := pool.ModelCooldownUntil("alpha"); !until.IsZero() {
		t.Fatalf("ModelCooldownUntil(alpha) = %v, want the served model usable", until)
	}
	if until := pool.ModelCooldownUntil("beta"); until.IsZero() {
		t.Fatal("ModelCooldownUntil(beta) = zero, want the other model still parked")
	}
	pool.MarkModelServed("alpha")
	if until := pool.ModelCooldownUntil("beta"); until.IsZero() {
		t.Fatal("ModelCooldownUntil(beta) = zero, want a served model to clear only its own wait")
	}
}

// TestLaneSpentIsItsOwnKindOfCooldown pins the marker a spent shared lane
// leaves: the connection cools for the named wait, a model that failed does
// not mark the lane, a success clears it, and the wait expires on its own.
func TestLaneSpentIsItsOwnKindOfCooldown(t *testing.T) {
	clock := testkit.NewFakeClock(time.Now())
	pool := newConnectionPool(t, clock)

	pool.MarkModelUnavailable("alpha", 0)
	if until := pool.LaneSpentUntil(); !until.IsZero() {
		t.Fatalf("LaneSpentUntil() = %v, want a failed model not to mark the lane", until)
	}
	pool.MarkLaneSpent(10 * time.Minute)
	until := pool.LaneSpentUntil()
	if until.IsZero() {
		t.Fatal("LaneSpentUntil() = zero, want the lane marked spent")
	}
	if got := pool.CooldownUntil(); !got.Equal(until) {
		t.Fatalf("CooldownUntil() = %v, want the spent lane's own wait", got)
	}
	pool.MarkServed("one")
	if until := pool.LaneSpentUntil(); !until.IsZero() {
		t.Fatalf("LaneSpentUntil() = %v, want a success to clear the marker", until)
	}

	pool.MarkLaneSpent(time.Minute)
	clock.Add(time.Minute)
	if until := pool.LaneSpentUntil(); !until.IsZero() {
		t.Fatalf("LaneSpentUntil() = %v, want an expired wait to read zero", until)
	}
}

// TestSpentLaneWaitOutlivesAShorterModelPark pins the shared allowance's own
// clock: the lane named a long wait, and one model that failed afterwards must
// not shorten it. The pool's failover step is 5 minutes at most, so a spent
// lane that cooled 20 hours has to still read its own wait after a model park
// lands on the same connection.
func TestSpentLaneWaitOutlivesAShorterModelPark(t *testing.T) {
	clock := testkit.NewFakeClock(time.Now())
	pool := newConnectionPool(t, clock)

	pool.MarkLaneSpent(20 * time.Hour)
	spent := pool.LaneSpentUntil()
	if spent.IsZero() {
		t.Fatal("LaneSpentUntil() = zero, want the lane marked spent")
	}

	// A rate limit that named a one-minute wait, the kind a single account
	// answers. It describes the model, not the shared allowance.
	pool.MarkModelUnavailable("alpha", time.Minute)
	pool.MarkModelUnavailable("alpha", 0)

	if got := pool.CooldownUntil(); got.Before(spent) {
		t.Fatalf("CooldownUntil() = %v, want the spent lane's %v to stand", got, spent)
	}
	if until := pool.LaneSpentUntil(); until.IsZero() {
		t.Fatal("LaneSpentUntil() = zero, want the lane still spent")
	}
}

// TestConnectionCooldownReportsThePooledWait covers the resolution the catalog
// and the router read: a connection whose credentials are all waiting names
// that moment, and a success clears it.
func TestConnectionCooldownReportsThePooledWait(t *testing.T) {
	clock := testkit.NewFakeClock(time.Now())
	pool := newConnectionPool(t, clock)

	pool.RecordFailure("one", http.StatusTooManyRequests, 0)
	pool.RecordFailure("one", http.StatusTooManyRequests, 0)
	pool.RecordFailure("two", http.StatusTooManyRequests, 0)
	pool.RecordFailure("two", http.StatusTooManyRequests, 0)
	if until := pool.CooldownUntil(); until.IsZero() {
		t.Fatal("CooldownUntil() = zero, want the connection's own cooldown")
	}
	pool.RecordSuccess("one")
	pool.RecordSuccess("two")
	if until := pool.CooldownUntil(); !until.IsZero() {
		t.Fatalf("CooldownUntil() = %v, want a success to clear the connection cooldown", until)
	}
}
