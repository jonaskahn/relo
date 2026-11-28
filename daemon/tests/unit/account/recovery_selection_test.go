package pool_test

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/account"
	"github.com/jonaskahn/relo/tests/testkit"
)

const recoveryWait = 15 * time.Minute

func recoveryPool(t *testing.T, strategy account.PoolStrategy) (*account.Pool, *testkit.FakeClock) {
	t.Helper()
	clock := testkit.NewFakeClock(time.Unix(1_700_000_000, 0))
	pooled := account.NewPoolWithOptions("provider-a", account.Options{
		Clock: clock, Strategy: strategy, FailoverBackoff: []time.Duration{0, recoveryWait},
	})
	pooled.Add(entry("acct-primary", account.StatusActive))
	pooled.Add(entry("acct-backup", account.StatusActive))
	return pooled, clock
}

func cooledBackup(t *testing.T, pooled *account.Pool, clock *testkit.FakeClock) {
	t.Helper()
	if verdict := pooled.RecordFailure("acct-backup", 429, recoveryWait); verdict != account.VerdictRetry {
		t.Fatalf("RecordFailure() = %v, want retry", verdict)
	}
	clock.Add(recoveryWait)
	if state := pooled.Health("acct-backup").State; state != account.BreakerHalfOpen {
		t.Fatalf("backup state = %q, want half_open", state)
	}
}

func assertTrial(t *testing.T, pooled *account.Pool, id string, want bool) {
	t.Helper()
	breaker := pooled.Breaker(id)
	if got := breaker.Allows(); got != want {
		t.Fatalf("Allows(%q) = %v, want %v", id, got, want)
	}
}

// TestSelectionDoesNotSpendAnUnusedRecoveryTrial pins the reported sequence:
// choosing a healthy account leaves a backup's expired trial alone, so a long
// limit on the primary can fail over to it within the same request.
func TestSelectionDoesNotSpendAnUnusedRecoveryTrial(t *testing.T) {
	pooled, clock := recoveryPool(t, rotatingStrategy{})
	pooled.MarkServed("acct-primary")
	cooledBackup(t, pooled, clock)

	first, err := pooled.Select(account.Selection{})
	if err != nil || first.ID != "acct-primary" {
		t.Fatalf("Select() = %q, %v, want the healthy primary", first.ID, err)
	}
	assertTrial(t, pooled, "acct-backup", true)

	if verdict := pooled.RecordFailure("acct-primary", 429, 4*time.Hour); verdict != account.VerdictRetry {
		t.Fatalf("RecordFailure() = %v, want retry", verdict)
	}
	next, err := pooled.Select(account.Selection{Exclude: []string{"acct-primary"}})
	if err != nil || next.ID != "acct-backup" {
		t.Fatalf("Select() = %q, %v, want the recovered backup", next.ID, err)
	}
	assertTrial(t, pooled, "acct-backup", false)
}

// TestPreferredAndStickySelectionOnlySpendTheChosenTrial pins the two
// precedence paths that pick a named account before the strategy runs.
func TestPreferredAndStickySelectionOnlySpendTheChosenTrial(t *testing.T) {
	t.Run("preferred", func(t *testing.T) {
		pooled, clock := recoveryPool(t, rotatingStrategy{})
		cooledBackup(t, pooled, clock)
		selected, err := pooled.Select(account.Selection{Prefer: "acct-primary"})
		if err != nil || selected.ID != "acct-primary" {
			t.Fatalf("Select() = %q, %v, want the preferred account", selected.ID, err)
		}
		assertTrial(t, pooled, "acct-backup", true)
	})

	t.Run("conversation pin", func(t *testing.T) {
		pooled, clock := recoveryPool(t, rotatingStrategy{})
		cooledBackup(t, pooled, clock)
		selection := account.Selection{ConversationID: "conv-a"}
		first, err := pooled.Select(selection)
		if err != nil || first.ID != "acct-primary" {
			t.Fatalf("first Select() = %q, %v, want the primary", first.ID, err)
		}
		if verdict := pooled.RecordFailure("acct-primary", 429, recoveryWait); verdict != account.VerdictRetry {
			t.Fatalf("RecordFailure() = %v, want retry", verdict)
		}
		clock.Add(recoveryWait)
		selected, err := pooled.Select(selection)
		if err != nil || selected.ID != first.ID {
			t.Fatalf("Select() = %q, %v, want the pinned account", selected.ID, err)
		}
		assertTrial(t, pooled, "acct-backup", true)
		if _, ok := pooled.Pins().Get("conv-a"); !ok {
			t.Fatal("Select() did not renew the pin it used")
		}
	})

	t.Run("sticky connection", func(t *testing.T) {
		pooled, clock := recoveryPool(t, &account.LeastLoadedStrategy{})
		cooledBackup(t, pooled, clock)
		pooled.MarkServed("acct-primary")
		selected, err := pooled.Select(account.Selection{})
		if err != nil || selected.ID != "acct-primary" {
			t.Fatalf("Select() = %q, %v, want the sticky account", selected.ID, err)
		}
		assertTrial(t, pooled, "acct-backup", true)
	})

	t.Run("strategy", func(t *testing.T) {
		pooled, clock := recoveryPool(t, lastStrategy{})
		cooledBackup(t, pooled, clock)
		selected, err := pooled.Select(account.Selection{})
		if err != nil || selected.ID != "acct-backup" {
			t.Fatalf("Select() = %q, %v, want the strategy's account", selected.ID, err)
		}
		assertTrial(t, pooled, "acct-primary", true)
		assertTrial(t, pooled, "acct-backup", false)
	})
}

// TestFilteringNeverSpendsAnIneligibleTrial keeps a recovery account reserved
// while a model roster or this request's exclusion keeps it out of the walk.
func TestFilteringNeverSpendsAnIneligibleTrial(t *testing.T) {
	t.Run("excluded", func(t *testing.T) {
		pooled, clock := recoveryPool(t, lastStrategy{})
		cooledBackup(t, pooled, clock)
		selected, err := pooled.Select(account.Selection{Exclude: []string{"acct-backup"}})
		if err != nil || selected.ID != "acct-primary" {
			t.Fatalf("Select() = %q, %v, want the only nonexcluded account", selected.ID, err)
		}
		assertTrial(t, pooled, "acct-backup", true)
	})

	t.Run("model ineligible", func(t *testing.T) {
		index := account.NewModelIndex(nil)
		index.SetRoster("acct-primary", []string{"model-a"}, account.ModelSourceListing)
		index.SetRoster("acct-backup", []string{"model-b"}, account.ModelSourceListing)
		clock := testkit.NewFakeClock(time.Unix(1_700_000_000, 0))
		pooled := account.NewPoolWithOptions("provider-a", account.Options{
			Clock: clock, Strategy: lastStrategy{}, Models: index,
			FailoverBackoff: []time.Duration{0, recoveryWait},
		})
		pooled.Add(entry("acct-primary", account.StatusActive))
		pooled.Add(entry("acct-backup", account.StatusActive))
		cooledBackup(t, pooled, clock)
		selected, err := pooled.Select(account.Selection{Models: []string{"model-a"}})
		if err != nil || selected.ID != "acct-primary" {
			t.Fatalf("Select() = %q, %v, want the account that serves the model", selected.ID, err)
		}
		assertTrial(t, pooled, "acct-backup", true)
		paused := account.PoolEntry{ID: "acct-primary", ProviderID: "provider-a", Status: account.StatusPaused}
		pooled.Add(paused)
		if _, err := pooled.Select(account.Selection{Models: []string{"model-a"}}); !errors.Is(err, account.ErrNoModelAccount) {
			t.Fatalf("Select() error = %v, want %v", err, account.ErrNoModelAccount)
		}
		assertTrial(t, pooled, "acct-backup", true)
	})
}

type errorStrategy struct{ err error }

// rotatingStrategy takes the first candidate each time, which lets a test
// see the eligible candidate list this request built.
type rotatingStrategy struct{}

func (rotatingStrategy) Select(candidates []account.PoolEntry, _ account.Selection) (account.PoolEntry, error) {
	if len(candidates) == 0 {
		return account.PoolEntry{}, account.ErrNoCredentials
	}
	return candidates[0], nil
}

func (s errorStrategy) Select([]account.PoolEntry, account.Selection) (account.PoolEntry, error) {
	return account.PoolEntry{}, s.err
}

// otherRequestStrategy simulates another request taking one account's trial
// between this request's eligibility check and its own acquisition.
type otherRequestStrategy struct {
	pool   *account.Pool
	winner string
	calls  int
}

func (s *otherRequestStrategy) Select(candidates []account.PoolEntry, _ account.Selection) (account.PoolEntry, error) {
	s.calls++
	if s.calls > 1 {
		return candidates[0], nil
	}
	if !s.pool.Breaker(s.winner).IsAvailable() {
		return account.PoolEntry{}, errors.New("the simulated request failed to take its trial")
	}
	for _, candidate := range candidates {
		if candidate.ID == s.winner {
			return candidate, nil
		}
	}
	return account.PoolEntry{}, account.ErrNoCredentials
}

// TestStrategyFailureReservesNoTrial pins that a strategy refusal is returned
// before the pool spends a half-open trial on any candidate.
func TestStrategyFailureReservesNoTrial(t *testing.T) {
	want := errors.New("strategy unavailable")
	pooled, clock := recoveryPool(t, errorStrategy{err: want})
	cooledBackup(t, pooled, clock)
	if _, err := pooled.Select(account.Selection{}); !errors.Is(err, want) {
		t.Fatalf("Select() error = %v, want %v", err, want)
	}
	assertTrial(t, pooled, "acct-primary", true)
	assertTrial(t, pooled, "acct-backup", true)
}

// TestOneSelectionReservesOneOfSeveralHalfOpenTrials pins the single-trial
// budget when every account has recovered: one call cannot hand out two.
func TestOneSelectionReservesOneOfSeveralHalfOpenTrials(t *testing.T) {
	pooled, clock := recoveryPool(t, rotatingStrategy{})
	for _, id := range []string{"acct-primary", "acct-backup"} {
		if verdict := pooled.RecordFailure(id, 429, recoveryWait); verdict != account.VerdictRetry {
			t.Fatalf("RecordFailure(%q) = %v, want retry", id, verdict)
		}
	}
	clock.Add(recoveryWait)
	if _, err := pooled.Select(account.Selection{}); err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	primary, backup := pooled.Breaker("acct-primary").Allows(), pooled.Breaker("acct-backup").Allows()
	if primary == backup {
		t.Fatalf("trials = %v/%v, want exactly one spent", primary, backup)
	}
}

// TestConcurrentSelectionSpendsEachTrialOnce pins the loser's fallback with
// barriers: only one request reaches each half-open account, and the other
// completes on the remaining candidate without waiting for a clock or sleep.
func TestConcurrentSelectionSpendsEachTrialOnce(t *testing.T) {
	pooled, clock := recoveryPool(t, rotatingStrategy{})
	for _, id := range []string{"acct-primary", "acct-backup"} {
		if verdict := pooled.RecordFailure(id, 429, recoveryWait); verdict != account.VerdictRetry {
			t.Fatalf("RecordFailure(%q) = %v, want retry", id, verdict)
		}
	}
	clock.Add(recoveryWait)

	const workers = 2
	var ready, release, done sync.WaitGroup
	ready.Add(workers)
	release.Add(1)
	done.Add(workers)
	results := make([]account.PoolEntry, workers)
	errs := make([]error, workers)
	for worker := range workers {
		go func(index int) {
			defer done.Done()
			ready.Done()
			release.Wait()
			results[index], errs[index] = pooled.Select(account.Selection{})
		}(worker)
	}
	ready.Wait()
	release.Done()
	done.Wait()
	for worker := range workers {
		if errs[worker] != nil {
			t.Fatalf("Select(%d) error = %v", worker, errs[worker])
		}
	}
	if results[0].ID == results[1].ID {
		t.Fatalf("selections = %q/%q, want different accounts", results[0].ID, results[1].ID)
	}
	primary, backup := pooled.Breaker("acct-primary").Allows(), pooled.Breaker("acct-backup").Allows()
	if primary || backup {
		t.Fatalf("trials = %v/%v, want both spent once", primary, backup)
	}
}

// TestRecoveryReportsStillCloseOrRecoolBreakers pins the breaker behavior a
// trial carries after it is selected: a success closes it and a failure walks
// the next wait, while a genuinely exhausted pool keeps its local error.
func TestRecoveryReportsStillCloseOrRecoolBreakers(t *testing.T) {
	pooled, clock := recoveryPool(t, &account.LeastLoadedStrategy{})
	cooledBackup(t, pooled, clock)
	if _, err := pooled.Select(account.Selection{Prefer: "acct-backup"}); err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	pooled.RecordSuccess("acct-backup")
	if state := pooled.Health("acct-backup").State; state != account.BreakerClosed {
		t.Fatalf("state after success = %q, want closed", state)
	}

	cooledBackup(t, pooled, clock)
	if _, err := pooled.Select(account.Selection{Prefer: "acct-backup"}); err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	pooled.RecordFailure("acct-backup", 429, 0)
	if state := pooled.Health("acct-backup").State; state != account.BreakerOpen {
		t.Fatalf("state after failure = %q, want open", state)
	}
	if err := pooled.Pause("acct-primary"); err != nil {
		t.Fatalf("Pause() error = %v", err)
	}
	if _, err := pooled.Select(account.Selection{Exclude: []string{"acct-primary"}}); !errors.Is(err, account.ErrNoCredentials) {
		t.Fatalf("Select() error = %v, want %v", err, account.ErrNoCredentials)
	}
}

// TestContentionUsesAFreshCandidate pins the retry inside one Select: after
// another request takes the first choice, this call tries the next candidate
// and never reconsiders the rejected one.
func TestContentionUsesAFreshCandidate(t *testing.T) {
	strategy := &otherRequestStrategy{winner: "acct-primary"}
	pooled, clock := recoveryPool(t, strategy)
	strategy.pool = pooled
	for _, id := range []string{"acct-primary", "acct-backup"} {
		if verdict := pooled.RecordFailure(id, 429, recoveryWait); verdict != account.VerdictRetry {
			t.Fatalf("RecordFailure(%q) = %v, want retry", id, verdict)
		}
	}
	clock.Add(recoveryWait)

	selected, err := pooled.Select(account.Selection{})
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	if selected.ID != "acct-backup" {
		t.Fatalf("Select() = %q, want the unclaimed candidate", selected.ID)
	}
	if strategy.calls != 2 {
		t.Fatalf("strategy calls = %d, want the rejected account tried once and never reconsidered", strategy.calls)
	}
	assertTrial(t, pooled, "acct-primary", false)
	assertTrial(t, pooled, "acct-backup", false)
}
