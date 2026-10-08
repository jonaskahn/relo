package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/account"
)

// The console's dashboard reads an account's rate-limit health from the
// credential breaker, so the JSON an account list reports has to carry the
// live state the pool just recorded.
func TestAccountReportsLimitState(t *testing.T) {
	harness := newHarness(t)
	entry := harness.addAccount("openai", "work", "sk-test-secret-value")
	ctx := context.Background()

	accounts, err := harness.accounts.Accounts(ctx, "openai")
	if err != nil {
		t.Fatalf("Accounts() error = %v", err)
	}
	if accounts[0].LimitState != "ready" || accounts[0].LimitedUntilMs != 0 {
		t.Fatalf("fresh account = %+v, want ready with no until time", accounts[0])
	}

	pool := harness.pools.GetPool("openai")
	for attempt := 0; attempt < 3; attempt++ {
		if verdict := pool.RecordFailure(entry.ID, 429, 0); verdict != account.VerdictRetry {
			t.Fatalf("RecordFailure() verdict = %v, want retry", verdict)
		}
	}

	accounts, err = harness.accounts.Accounts(ctx, "openai")
	if err != nil {
		t.Fatalf("Accounts() error = %v", err)
	}
	if accounts[0].LimitState != "limited" {
		t.Fatalf("LimitState = %q, want limited", accounts[0].LimitState)
	}
	if accounts[0].LimitedUntilMs <= time.Now().UnixMilli() {
		t.Fatalf("LimitedUntilMs = %d, want a future instant", accounts[0].LimitedUntilMs)
	}

	pool.RecordSuccess(entry.ID)
	accounts, _ = harness.accounts.Accounts(ctx, "openai")
	if accounts[0].LimitState != "ready" {
		t.Fatalf("LimitState after success = %q, want ready", accounts[0].LimitState)
	}
}
