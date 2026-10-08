package pool_test

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/account"
	"github.com/jonaskahn/relo/tests/testkit"
)

func TestBreaker(t *testing.T) {
	t.Run("1 failure opens the breaker", func(t *testing.T) {
		breaker := account.NewBreakerWithBackoff(testkit.NewFakeClock(time.Now()), fixedWait)
		if verdict := breaker.RecordFailure(500, 0); verdict != account.VerdictRetry {
			t.Fatalf("verdict = %v, want retry", verdict)
		}
		if breaker.IsAvailable() {
			t.Fatal("breaker stayed closed after the first failure")
		}
		if got := breaker.State(); got != account.BreakerOpen {
			t.Fatalf("State() = %q, want %q", got, account.BreakerOpen)
		}
	})

	t.Run("after the backoff one trial gets through", func(t *testing.T) {
		clock := testkit.NewFakeClock(time.Now())
		breaker := openBreaker(t, clock)
		clock.Add(15*time.Minute - time.Second)
		if breaker.IsAvailable() {
			t.Fatal("the breaker opened before the 15m cooldown elapsed")
		}
		clock.Add(2 * time.Second)
		if !breaker.IsAvailable() {
			t.Fatal("the breaker stayed open after the backoff")
		}
		if got := breaker.State(); got != account.BreakerHalfOpen {
			t.Fatalf("State() = %q, want %q", got, account.BreakerHalfOpen)
		}
		if breaker.IsAvailable() {
			t.Fatal("a second request got through while the trial was out")
		}
	})

	t.Run("a successful trial closes the breaker", func(t *testing.T) {
		clock := testkit.NewFakeClock(time.Now())
		breaker := openBreaker(t, clock)
		clock.Add(16 * time.Minute)
		if !breaker.IsAvailable() {
			t.Fatal("the trial was not offered after the backoff")
		}
		breaker.RecordSuccess()
		if got := breaker.State(); got != account.BreakerClosed {
			t.Fatalf("State() = %q, want %q", got, account.BreakerClosed)
		}
		breaker.RecordFailure(503, 0)
		if breaker.IsAvailable() {
			t.Fatal("a fresh failure left the breaker closed")
		}
	})

	t.Run("a failed trial keeps the cooldown", func(t *testing.T) {
		clock := testkit.NewFakeClock(time.Now())
		breaker := openBreaker(t, clock)
		clock.Add(16 * time.Minute)
		breaker.IsAvailable()
		breaker.RecordFailure(500, 0)
		clock.Add(15*time.Minute - time.Second)
		if breaker.IsAvailable() {
			t.Fatal("the cooldown elapsed too early")
		}
		clock.Add(2 * time.Second)
		if !breaker.IsAvailable() {
			t.Fatal("the cooldown did not expire")
		}
	})

	t.Run("429 with Retry-After sets that backoff", func(t *testing.T) {
		clock := testkit.NewFakeClock(time.Now())
		breaker := account.NewBreaker(clock)
		breaker.RecordFailure(429, 90*time.Second)
		clock.Add(89 * time.Second)
		if breaker.IsAvailable() {
			t.Fatal("the breaker ignored the Retry-After delay")
		}
		clock.Add(2 * time.Second)
		if !breaker.IsAvailable() {
			t.Fatal("the breaker stayed open past the Retry-After delay")
		}
	})

	t.Run("the cooldown caps at five hours", func(t *testing.T) {
		clock := testkit.NewFakeClock(time.Now())
		breaker := account.NewBreaker(clock)
		breaker.RecordFailure(429, 10*time.Hour)
		clock.Add(5*time.Hour - time.Second)
		if breaker.IsAvailable() {
			t.Fatal("the Retry-After wait exceeded the documented ceiling")
		}
		clock.Add(2 * time.Second)
		if !breaker.IsAvailable() {
			t.Fatal("the capped wait did not expire")
		}
	})

	t.Run("the shipped ladder starts with no wait and grows", func(t *testing.T) {
		clock := testkit.NewFakeClock(time.Now())
		breaker := account.NewBreaker(clock)
		breaker.RecordFailure(429, 0)
		if !breaker.IsAvailable() {
			t.Fatal("the first failure kept the credential out of rotation")
		}
		if got := breaker.Snapshot(); got.Failures != 1 || got.State != account.BreakerClosed {
			t.Fatalf("snapshot = %+v, want one counted failure and a closed breaker", got)
		}
		breaker.RecordFailure(429, 0)
		clock.Add(3*time.Minute - time.Second)
		if breaker.IsAvailable() {
			t.Fatal("the second failure did not wait its three-minute step")
		}
		clock.Add(2 * time.Second)
		if !breaker.IsAvailable() {
			t.Fatal("the three-minute step did not expire")
		}
		breaker.RecordFailure(429, 0)
		clock.Add(5*time.Minute - time.Second)
		if breaker.IsAvailable() {
			t.Fatal("the third failure did not wait its five-minute step")
		}
	})

	t.Run("a success resets the ladder", func(t *testing.T) {
		clock := testkit.NewFakeClock(time.Now())
		breaker := account.NewBreaker(clock)
		breaker.RecordFailure(429, 0)
		breaker.RecordFailure(429, 0)
		breaker.RecordSuccess()
		breaker.RecordFailure(429, 0)
		if !breaker.IsAvailable() {
			t.Fatal("a fresh failure after a success did not start at the first step")
		}
	})

	t.Run("authorization failures are not breaker events", func(t *testing.T) {
		breaker := account.NewBreaker(testkit.NewFakeClock(time.Now()))
		for attempt := 0; attempt < 5; attempt++ {
			if verdict := breaker.RecordFailure(401, 0); verdict != account.VerdictNeedsReauth {
				t.Fatalf("verdict = %v, want needs_reauth", verdict)
			}
			breaker.RecordFailure(403, 0)
		}
		if !breaker.IsAvailable() || breaker.State() != account.BreakerClosed {
			t.Fatalf("state = %q, want an authorization failure to leave the breaker closed", breaker.State())
		}
	})

	t.Run("client errors are not breaker events", func(t *testing.T) {
		breaker := account.NewBreaker(testkit.NewFakeClock(time.Now()))
		for _, status := range []int{400, 404, 422} {
			if verdict := breaker.RecordFailure(status, 0); verdict != account.VerdictHealthy {
				t.Fatalf("status %d verdict = %v, want healthy", status, verdict)
			}
		}
		if !breaker.IsAvailable() {
			t.Fatal("a client error opened the breaker")
		}
	})

	t.Run("status classification covers the gateway statuses", func(t *testing.T) {
		tests := []struct {
			status int
			want   account.CredentialVerdict
		}{
			{200, account.VerdictHealthy},
			{401, account.VerdictNeedsReauth},
			{403, account.VerdictNeedsReauth},
			{429, account.VerdictRetry},
			{500, account.VerdictRetry},
			{502, account.VerdictRetry},
		}
		for _, tt := range tests {
			if got := account.ClassifyStatus(tt.status); got != tt.want {
				t.Fatalf("ClassifyStatus(%d) = %v, want %v", tt.status, got, tt.want)
			}
		}
	})
}

func TestPoolBreakers(t *testing.T) {
	t.Run("an open breaker takes a credential out of rotation", func(t *testing.T) {
		clock := testkit.NewFakeClock(time.Now())
		pooled := account.NewPoolWithOptions("openai", account.Options{Clock: clock, FailoverBackoff: fixedWait})
		pooled.Add(entry("a", account.StatusActive))
		pooled.Add(entry("b", account.StatusActive))
		for attempt := 0; attempt < 3; attempt++ {
			pooled.RecordFailure("a", 500, 0)
		}
		for attempt := 0; attempt < 4; attempt++ {
			selected, err := pooled.Select(account.Selection{})
			if err != nil {
				t.Fatalf("Select() error = %v", err)
			}
			if selected.ID != "b" {
				t.Fatalf("Select() = %q, want the healthy credential", selected.ID)
			}
		}
		pooled.RecordSuccess("a")
		seen := map[string]bool{}
		for attempt := 0; attempt < 4; attempt++ {
			selected, _ := pooled.Select(account.Selection{})
			seen[selected.ID] = true
		}
		if !seen["a"] {
			t.Fatal("a recovered credential never returned to rotation")
		}
	})
}

func TestLeastLoadedStrategy(t *testing.T) {
	t.Run("the highest headroom wins", func(t *testing.T) {
		headroom := map[string]float64{"a": 20, "b": 80}
		strategy := &account.LeastLoadedStrategy{Headroom: mapHeadroom(headroom)}
		selected := selectOnce(t, strategy, entry("a", account.StatusActive), entry("b", account.StatusActive))
		if selected != "b" {
			t.Fatalf("Select() = %q, want the credential with 80%% headroom", selected)
		}
	})

	t.Run("a missing quota counts as half a tank", func(t *testing.T) {
		headroom := map[string]float64{"a": 40}
		strategy := &account.LeastLoadedStrategy{Headroom: mapHeadroom(headroom)}
		selected := selectOnce(t, strategy, entry("a", account.StatusActive), entry("b", account.StatusActive))
		if selected != "b" {
			t.Fatalf("Select() = %q, want the credential with no data to rank at 50%%", selected)
		}
	})

	t.Run("equal headroom lets priority decide", func(t *testing.T) {
		headroom := map[string]float64{"a": 50, "b": 50}
		strategy := &account.LeastLoadedStrategy{Headroom: mapHeadroom(headroom)}
		low, high := entry("a", account.StatusActive), entry("b", account.StatusActive)
		high.Priority = 10
		selected := selectOnce(t, strategy, low, high)
		if selected != "b" {
			t.Fatalf("Select() = %q, want the higher priority credential", selected)
		}
	})

	t.Run("equal everything rotates", func(t *testing.T) {
		strategy := &account.LeastLoadedStrategy{}
		order := make([]string, 0, 4)
		for attempt := 0; attempt < 4; attempt++ {
			order = append(order, selectOnce(t, strategy, entry("a", account.StatusActive), entry("b", account.StatusActive)))
		}
		if strings.Join(order, "") != "abab" {
			t.Fatalf("selection order = %v, want the tied credentials to alternate", order)
		}
	})

	t.Run("a quota update changes the next selection", func(t *testing.T) {
		headroom := map[string]float64{"a": 80, "b": 20}
		strategy := &account.LeastLoadedStrategy{Headroom: mapHeadroom(headroom)}
		if selected := selectOnce(t, strategy, entry("a", account.StatusActive), entry("b", account.StatusActive)); selected != "a" {
			t.Fatalf("Select() = %q, want a first", selected)
		}
		headroom["a"] = 5
		if selected := selectOnce(t, strategy, entry("a", account.StatusActive), entry("b", account.StatusActive)); selected != "b" {
			t.Fatalf("Select() = %q, want the refreshed quota to move the load", selected)
		}
	})

	t.Run("an empty pool is reported", func(t *testing.T) {
		if _, err := (&account.LeastLoadedStrategy{}).Select(nil, account.Selection{}); !errors.Is(err, account.ErrNoCredentials) {
			t.Fatalf("Select() error = %v, want %v", err, account.ErrNoCredentials)
		}
	})

	t.Run("headroom is read through the seam", func(t *testing.T) {
		lookups := 0
		strategy := &account.LeastLoadedStrategy{Headroom: func(string) (float64, bool) {
			lookups++
			return 10, true
		}}
		selectOnce(t, strategy, entry("a", account.StatusActive), entry("b", account.StatusActive))
		if lookups != 2 {
			t.Fatalf("headroom lookups = %d, want one per candidate", lookups)
		}
	})
}

func TestRandomStrategy(t *testing.T) {
	t.Run("every candidate is reachable", func(t *testing.T) {
		strategy := &account.RandomStrategy{}
		candidates := []account.PoolEntry{entry("a", account.StatusActive), entry("b", account.StatusActive), entry("c", account.StatusActive)}
		seen := map[string]int{}
		for attempt := 0; attempt < 300; attempt++ {
			selected, err := strategy.Select(candidates, account.Selection{})
			if err != nil {
				t.Fatalf("Select() error = %v", err)
			}
			seen[selected.ID]++
		}
		if len(seen) != 3 {
			t.Fatalf("selections = %v, want every credential to be reachable", seen)
		}
	})

	t.Run("a broken random source is reported", func(t *testing.T) {
		strategy := &account.RandomStrategy{Source: failingReader{}}
		_, err := strategy.Select([]account.PoolEntry{entry("a", account.StatusActive), entry("b", account.StatusActive)}, account.Selection{})
		if !errors.Is(err, account.ErrNoRandomness) {
			t.Fatalf("Select() error = %v, want %v", err, account.ErrNoRandomness)
		}
	})

	t.Run("an empty pool is reported", func(t *testing.T) {
		if _, err := (&account.RandomStrategy{}).Select(nil, account.Selection{}); !errors.Is(err, account.ErrNoCredentials) {
			t.Fatalf("Select() error = %v, want %v", err, account.ErrNoCredentials)
		}
	})
}

func TestStrategySwitching(t *testing.T) {
	t.Run("a pool can switch strategy at runtime", func(t *testing.T) {
		pooled := account.NewPoolWithStrategy("openai", &account.RoundRobinStrategy{})
		pooled.Add(entry("a", account.StatusActive))
		pooled.Add(entry("b", account.StatusActive))
		first, _ := pooled.Select(account.Selection{})
		pooled.SetStrategy(&account.LeastLoadedStrategy{Headroom: func(credentialID string) (float64, bool) {
			return map[string]float64{"a": 10, "b": 90}[credentialID], true
		}})
		second, err := pooled.Select(account.Selection{})
		if err != nil {
			t.Fatalf("Select() error = %v", err)
		}
		if first.ID != "a" || second.ID != "b" {
			t.Fatalf("selections = %q then %q, want the strategy to take effect", first.ID, second.ID)
		}
		pooled.SetStrategy(nil)
		if _, err := pooled.Select(account.Selection{}); err != nil {
			t.Fatalf("Select() after a nil strategy error = %v", err)
		}
	})
}

func TestPinCache(t *testing.T) {
	t.Run("a pinned conversation keeps its credential", func(t *testing.T) {
		clock := testkit.NewFakeClock(time.Now())
		pins := account.NewPinCache(0, 0, clock)
		pins.Pin("conversation-1", "a")
		if credentialID, ok := pins.Get("conversation-1"); !ok || credentialID != "a" {
			t.Fatalf("Get() = %q, %v, want the pinned credential", credentialID, ok)
		}
		pins.Pin("conversation-1", "b")
		if credentialID, _ := pins.Get("conversation-1"); credentialID != "b" {
			t.Fatalf("Get() = %q, want the updated pin", credentialID)
		}
		if _, ok := pins.Get("absent"); ok {
			t.Fatal("Get() reported a pin that was never created")
		}
		if _, ok := pins.Get(""); ok {
			t.Fatal("Get() reported a pin for an empty conversation")
		}
	})

	t.Run("a pin expires after the ttl", func(t *testing.T) {
		clock := testkit.NewFakeClock(time.Now())
		pins := account.NewPinCache(0, time.Minute, clock)
		pins.Pin("conversation-1", "a")
		clock.Add(59 * time.Second)
		if _, ok := pins.Get("conversation-1"); !ok {
			t.Fatal("the pin expired before the ttl elapsed")
		}
		clock.Add(2 * time.Second)
		if _, ok := pins.Get("conversation-1"); ok {
			t.Fatal("the pin outlived its ttl")
		}
		if got := pins.Len(); got != 0 {
			t.Fatalf("Len() = %d, want the expired pin to be dropped", got)
		}
	})

	t.Run("the oldest pin is evicted at capacity", func(t *testing.T) {
		pins := account.NewPinCache(2, time.Hour, testkit.NewFakeClock(time.Now()))
		pins.Pin("one", "a")
		pins.Pin("two", "a")
		pins.Pin("three", "a")
		if got := pins.Len(); got != 2 {
			t.Fatalf("Len() = %d, want the cache bounded to 2", got)
		}
		if _, ok := pins.Get("one"); ok {
			t.Fatal("the least recently used pin survived the eviction")
		}
		if _, ok := pins.Get("three"); !ok {
			t.Fatal("the newest pin was evicted")
		}
	})

	t.Run("eviction by credential clears every conversation", func(t *testing.T) {
		pins := account.NewPinCache(0, 0, testkit.NewFakeClock(time.Now()))
		pins.Pin("one", "a")
		pins.Pin("two", "a")
		pins.Pin("three", "b")
		pins.EvictByCredential("a")
		if _, ok := pins.Get("one"); ok {
			t.Fatal("a pin of the evicted credential survived")
		}
		if _, ok := pins.Get("three"); !ok {
			t.Fatal("a pin of another credential was evicted")
		}
		pins.Evict("three")
		if got := pins.Len(); got != 0 {
			t.Fatalf("Len() = %d, want every pin gone", got)
		}
		pins.Evict("absent")
	})

	t.Run("empty identifiers are ignored", func(t *testing.T) {
		pins := account.NewPinCache(0, 0, testkit.NewFakeClock(time.Now()))
		pins.Pin("", "a")
		pins.Pin("one", "")
		if got := pins.Len(); got != 0 {
			t.Fatalf("Len() = %d, want no pin for an empty identifier", got)
		}
	})

	t.Run("concurrent access is safe", func(t *testing.T) {
		pins := account.NewPinCache(0, time.Hour, testkit.NewFakeClock(time.Now()))
		var workers sync.WaitGroup
		for worker := 0; worker < 20; worker++ {
			workers.Add(1)
			go func(index int) {
				defer workers.Done()
				conversation := "conversation-" + string(rune('a'+index%5))
				pins.Pin(conversation, "a")
				pins.Get(conversation)
				pins.EvictByCredential("b")
			}(worker)
		}
		workers.Wait()
	})
}

func TestPoolPinning(t *testing.T) {
	t.Run("a conversation keeps its credential", func(t *testing.T) {
		pooled := account.NewPoolWithStrategy("openai", &account.RoundRobinStrategy{})
		pooled.Add(entry("a", account.StatusActive))
		pooled.Add(entry("b", account.StatusActive))
		routed := account.Selection{ConversationID: "conversation-1"}
		first, err := pooled.Select(routed)
		if err != nil {
			t.Fatalf("Select() error = %v", err)
		}
		for attempt := 0; attempt < 3; attempt++ {
			selected, err := pooled.Select(routed)
			if err != nil {
				t.Fatalf("Select() error = %v", err)
			}
			if selected.ID != first.ID {
				t.Fatalf("Select() = %q, want the pinned %q", selected.ID, first.ID)
			}
		}
		unpinned, _ := pooled.Select(account.Selection{})
		if unpinned.ID == first.ID {
			t.Fatal("an unbound conversation did not rotate to the other credential")
		}
	})

	t.Run("an unhealthy pinned credential is replaced", func(t *testing.T) {
		clock := testkit.NewFakeClock(time.Now())
		pooled := account.NewPoolWithOptions("openai", account.Options{Clock: clock, Strategy: &account.RoundRobinStrategy{}, FailoverBackoff: fixedWait})
		pooled.Add(entry("a", account.StatusActive))
		pooled.Add(entry("b", account.StatusActive))
		routed := account.Selection{ConversationID: "conversation-1"}
		first, _ := pooled.Select(routed)
		for attempt := 0; attempt < 3; attempt++ {
			pooled.RecordFailure(first.ID, 500, 0)
		}
		replaced, err := pooled.Select(routed)
		if err != nil {
			t.Fatalf("Select() error = %v", err)
		}
		if replaced.ID == first.ID {
			t.Fatalf("Select() = %q, want a healthy credential", replaced.ID)
		}
		if _, ok := pooled.Pins().Get("conversation-1"); !ok {
			t.Fatal("the conversation was left unpinned after the replacement")
		}
	})

	t.Run("pausing a credential unpins its conversations", func(t *testing.T) {
		pooled := account.NewPool("openai")
		pooled.Add(entry("a", account.StatusActive))
		if _, err := pooled.Select(account.Selection{ConversationID: "conversation-1"}); err != nil {
			t.Fatalf("Select() error = %v", err)
		}
		if err := pooled.Pause("a"); err != nil {
			t.Fatalf("Pause() error = %v", err)
		}
		if _, ok := pooled.Pins().Get("conversation-1"); ok {
			t.Fatal("a paused credential kept its conversations pinned")
		}
	})

	t.Run("removing a credential unpins its conversations", func(t *testing.T) {
		pooled := account.NewPool("openai")
		pooled.Add(entry("a", account.StatusActive))
		if _, err := pooled.Select(account.Selection{ConversationID: "conversation-1"}); err != nil {
			t.Fatalf("Select() error = %v", err)
		}
		pooled.Remove("a")
		if _, ok := pooled.Pins().Get("conversation-1"); ok {
			t.Fatal("a removed credential kept its conversations pinned")
		}
	})
}

func TestDisabledFailoverBackoffKeepsEveryCredential(t *testing.T) {
	t.Run("the shipped ladder keeps the first failure in rotation", func(t *testing.T) {
		pooled := account.NewPool("openai")
		pooled.Add(entry("a", account.StatusActive))
		pooled.RecordFailure("a", 429, 0)
		if !pooled.Breaker("a").IsAvailable() {
			t.Fatal("the first failure took the credential out of rotation")
		}
		pooled.RecordFailure("a", 429, 0)
		if pooled.Breaker("a").IsAvailable() {
			t.Fatal("the second failure did not start a wait")
		}
	})

	t.Run("a refusal without a denylist never takes the credential out", func(t *testing.T) {
		clock := testkit.NewFakeClock(time.Now())
		pooled := account.NewPoolWithOptions("openai", account.Options{Clock: clock})
		pooled.Add(entry("a", account.StatusActive))
		pooled.Add(entry("b", account.StatusActive))
		for attempt := 0; attempt < 3; attempt++ {
			if verdict := pooled.RecordFailure("a", 500, 0); verdict != account.VerdictRetry {
				t.Fatalf("verdict = %v, want retry", verdict)
			}
			if !pooled.Breaker("a").IsAvailable() {
				t.Fatal("a refusal without a denylist took the credential out of rotation")
			}
		}
		if got := pooled.Breaker("a").Snapshot(); got.State != account.BreakerClosed || got.Failures != 0 {
			t.Fatalf("snapshot = %+v, want a breaker that never opened", got)
		}
	})

	t.Run("a Retry-After the provider named is still waited out", func(t *testing.T) {
		clock := testkit.NewFakeClock(time.Now())
		pooled := account.NewPoolWithOptions("openai", account.Options{Clock: clock})
		pooled.Add(entry("a", account.StatusActive))
		pooled.RecordFailure("a", 429, time.Minute)
		if pooled.Breaker("a").IsAvailable() {
			t.Fatal("the breaker ignored the Retry-After delay")
		}
		clock.Add(time.Minute)
		if !pooled.Breaker("a").IsAvailable() {
			t.Fatal("the breaker stayed open past the Retry-After delay")
		}
	})

	t.Run("disabling the wait closes open breakers at once", func(t *testing.T) {
		clock := testkit.NewFakeClock(time.Now())
		pooled := account.NewPoolWithOptions("openai", account.Options{Clock: clock, FailoverBackoff: fixedWait})
		pooled.Add(entry("a", account.StatusActive))
		pooled.RecordFailure("a", 500, 0)
		if pooled.Breaker("a").IsAvailable() {
			t.Fatal("the breaker stayed closed after a refusal")
		}
		pooled.SetFailoverBackoff(nil)
		if !pooled.Breaker("a").IsAvailable() {
			t.Fatal("disabling the denylist left the credential out of rotation")
		}
	})
}

func TestManagerSharing(t *testing.T) {
	t.Run("pools share the pin cache and the clock", func(t *testing.T) {
		clock := testkit.NewFakeClock(time.Now())
		pins := account.NewPinCache(4, time.Minute, clock)
		manager := account.NewManagerWithOptions(newFakeRepo(), fakeSecrets{}, account.ManagerOptions{Clock: clock, Pins: pins})
		pooled := manager.GetPool("openai")
		pooled.Add(entry("a", account.StatusActive))
		if _, err := pooled.Select(account.Selection{ConversationID: "conversation-1"}); err != nil {
			t.Fatalf("Select() error = %v", err)
		}
		if _, ok := pins.Get("conversation-1"); !ok {
			t.Fatal("the manager did not wire the shared pin cache")
		}
		if err := manager.Pause(context.Background(), "openai", "a"); err != nil {
			t.Fatalf("Pause() error = %v", err)
		}
		if _, ok := pins.Get("conversation-1"); ok {
			t.Fatal("pausing through the manager did not unpin the conversation")
		}
		if !pooled.Breaker("a").IsAvailable() {
			t.Fatal("the pool breaker should start closed")
		}
	})
}

// fixedWait is a one-step ladder: every failure waits the same fifteen
// minutes, which is what the single-wait tests assert.
var fixedWait = []time.Duration{15 * time.Minute}

func openBreaker(t *testing.T, clk *testkit.FakeClock) *account.Breaker {
	t.Helper()
	breaker := account.NewBreakerWithBackoff(clk, fixedWait)
	breaker.RecordFailure(500, 0)
	if breaker.IsAvailable() {
		t.Fatal("the breaker did not open after one failure")
	}
	return breaker
}

func TestBreakerSnapshot(t *testing.T) {
	t.Run("an open breaker reports its state, until time, and failures", func(t *testing.T) {
		now := time.Now()
		clock := testkit.NewFakeClock(now)
		breaker := openBreaker(t, clock)
		snapshot := breaker.Snapshot()
		if snapshot.State != account.BreakerOpen {
			t.Fatalf("State = %q, want open", snapshot.State)
		}
		if snapshot.Failures != 1 {
			t.Fatalf("Failures = %d, want 1", snapshot.Failures)
		}
		if !snapshot.Until.After(now.Add(14 * time.Minute)) {
			t.Fatalf("Until = %v, want about fifteen minutes after now", snapshot.Until)
		}
	})

	t.Run("the snapshot settles a breaker whose backoff has passed", func(t *testing.T) {
		clock := testkit.NewFakeClock(time.Now())
		breaker := openBreaker(t, clock)
		clock.Add(16 * time.Minute)
		if got := breaker.Snapshot().State; got != account.BreakerHalfOpen {
			t.Fatalf("State = %q, want half-open", got)
		}
	})

	t.Run("a closed breaker reports no failures", func(t *testing.T) {
		breaker := account.NewBreaker(testkit.NewFakeClock(time.Now()))
		snapshot := breaker.Snapshot()
		if snapshot.State != account.BreakerClosed || snapshot.Failures != 0 || !snapshot.Until.IsZero() {
			t.Fatalf("snapshot = %+v, want a fresh closed breaker", snapshot)
		}
	})
}

func TestCooldownUntil(t *testing.T) {
	t.Run("a healthy pool has no cooldown", func(t *testing.T) {
		pooled := account.NewPool("openai")
		pooled.Add(entry("a", account.StatusActive))
		if !pooled.CooldownUntil().IsZero() {
			t.Fatal("a healthy pool reported a cooldown")
		}
	})

	t.Run("an open breaker reports when it retries", func(t *testing.T) {
		clock := testkit.NewFakeClock(time.Now())
		pooled := account.NewPoolWithOptions("openai", account.Options{Clock: clock, FailoverBackoff: fixedWait})
		pooled.Add(entry("a", account.StatusActive))
		pooled.RecordFailure("a", 429, 0)
		until := pooled.CooldownUntil()
		if until.IsZero() || until.Before(clock.Now().Add(14*time.Minute)) {
			t.Fatalf("until = %v, want about fifteen minutes after now", until)
		}
	})

	t.Run("a coverage check does not spend the half-open trial", func(t *testing.T) {
		clock := testkit.NewFakeClock(time.Now())
		manager := account.NewManagerWithOptions(newFakeRepo(), fakeSecrets{}, account.ManagerOptions{Clock: clock, FailoverBackoff: fixedWait})
		pooled := manager.GetPool("openai")
		pooled.Add(entry("a", account.StatusActive))
		pooled.RecordFailure("a", 429, 0)
		clock.Add(16 * time.Minute)
		if !manager.Available("openai") {
			t.Fatal("Available() stayed false after the backoff")
		}
		if !manager.Available("openai") {
			t.Fatal("Available() spent the half-open trial")
		}
		if manager.CooldownUntil("openai").IsZero() == false {
			t.Fatal("a half-open account still reported a cooldown")
		}
	})
}

func mapHeadroom(values map[string]float64) account.Headroom {
	return func(credentialID string) (float64, bool) {
		percent, found := values[credentialID]
		return percent, found
	}
}

func selectOnce(t *testing.T, strategy account.PoolStrategy, candidates ...account.PoolEntry) string {
	t.Helper()
	selected, err := strategy.Select(candidates, account.Selection{})
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	return selected.ID
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) {
	return 0, io.ErrUnexpectedEOF
}
