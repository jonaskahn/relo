package activity_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/activity"
	"github.com/jonaskahn/relo/tests/testkit"
)

// TestProbeCredential covers the entry point that reads one account's quota
// again without the caller naming a provider: the worker finds the credential
// the source lists under that id and probes it as it would any other.
func TestProbeCredential(t *testing.T) {
	repo := &fakeRepo{}
	cache := activity.NewCache(repo, activity.CacheOptions{Clock: testkit.NewFakeClock(time.Now()), Source: "probe"})
	prober := &fakeProber{samples: []activity.WindowSample{{Window: "Gem", UsedPercent: 40, ResetAt: 900}}}
	source := &fakeSource{credentials: []activity.Credential{
		{ID: "credential-1", ProviderID: "google-antigravity"},
		{ID: "credential-2", ProviderID: "google-antigravity"},
	}}
	worker := activity.NewWorker(cache, activity.WorkerOptions{
		Probers: map[string]activity.QuotaProber{"google-antigravity": prober},
		Source:  source,
	})

	windows, err := worker.ProbeCredential(context.Background(), "credential-2")
	if err != nil {
		t.Fatalf("ProbeCredential() error = %v", err)
	}
	if len(windows) != 1 || windows[0].Headroom() != 60 {
		t.Fatalf("windows = %+v, want the window the prober read", windows)
	}

	t.Run("names the credential it could not find", func(t *testing.T) {
		if _, err := worker.ProbeCredential(context.Background(), "missing"); !errors.Is(err, activity.ErrNoCredential) {
			t.Fatalf("ProbeCredential() error = %v, want ErrNoCredential", err)
		}
	})

	t.Run("reports a worker with no source at all", func(t *testing.T) {
		sourceless := activity.NewWorker(cache, activity.WorkerOptions{
			Probers: map[string]activity.QuotaProber{"google-antigravity": prober},
		})
		if _, err := sourceless.ProbeCredential(context.Background(), "credential-1"); !errors.Is(err, activity.ErrNoCredential) {
			t.Fatalf("ProbeCredential() error = %v, want ErrNoCredential", err)
		}
	})
}
