package activity_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/activity"
	"github.com/jonaskahn/relo/tests/testkit"
)

func TestCache(t *testing.T) {
	t.Run("snapshots round-trip through the repository", func(t *testing.T) {
		repo := &fakeRepo{}
		cache := activity.NewCache(repo, activity.CacheOptions{Clock: testkit.NewFakeClock(time.Now()), Source: "probe"})
		samples := []activity.WindowSample{{Window: "Gem", UsedPercent: 20, ResetAt: 100}, {Window: "Cla", UsedPercent: 75}}
		if err := cache.Record(context.Background(), "credential-1", samples); err != nil {
			t.Fatalf("Record() error = %v", err)
		}
		if len(repo.snapshots) != 2 {
			t.Fatalf("snapshots = %+v, want both windows stored", repo.snapshots)
		}
		if repo.snapshots[0].Source != "probe" || repo.snapshots[0].CredentialID != "credential-1" {
			t.Fatalf("snapshot = %+v, want the credential and source recorded", repo.snapshots[0])
		}
		windows, err := cache.Windows(context.Background(), "credential-1")
		if err != nil {
			t.Fatalf("Windows() error = %v", err)
		}
		if len(windows) != 2 || windows[0].Window != "Cla" {
			t.Fatalf("windows = %+v, want the most constrained window first", windows)
		}
	})

	t.Run("a record without windows stores nothing", func(t *testing.T) {
		repo := &fakeRepo{}
		cache := activity.NewCache(repo, activity.CacheOptions{})
		if err := cache.Record(context.Background(), "credential-1", nil); err != nil {
			t.Fatalf("Record() error = %v", err)
		}
		if err := cache.Record(context.Background(), "credential-1", []activity.WindowSample{{UsedPercent: 10}}); err != nil {
			t.Fatalf("Record() error = %v", err)
		}
		if len(repo.snapshots) != 0 {
			t.Fatalf("snapshots = %+v, want a window without a name to be skipped", repo.snapshots)
		}
	})

	t.Run("headroom reads one window and the bottleneck", func(t *testing.T) {
		repo := &fakeRepo{}
		cache := activity.NewCache(repo, activity.CacheOptions{})
		if err := cache.Record(context.Background(), "credential-1", []activity.WindowSample{
			{Window: "Gem", UsedPercent: 20}, {Window: "Cla", UsedPercent: 90},
		}); err != nil {
			t.Fatalf("Record() error = %v", err)
		}
		headroom, ok := cache.Headroom(context.Background(), "credential-1", "Gem")
		if !ok || headroom != 80 {
			t.Fatalf("Headroom(Gem) = %v, %v, want 80", headroom, ok)
		}
		if _, ok := cache.Headroom(context.Background(), "credential-1", "absent"); ok {
			t.Fatal("Headroom() reported a window that was never recorded")
		}
		bottleneck, ok := cache.MostConstrained(context.Background(), "credential-1")
		if !ok || bottleneck != 10 {
			t.Fatalf("MostConstrained() = %v, %v, want the bottleneck window", bottleneck, ok)
		}
		if _, ok := cache.MostConstrained(context.Background(), "absent"); ok {
			t.Fatal("MostConstrained() reported a credential with no snapshots")
		}
		lookup := cache.Lookup(context.Background(), "credential-1")
		if percent, ok := lookup(); !ok || percent != 10 {
			t.Fatalf("Lookup() = %v, %v, want the bottleneck headroom", percent, ok)
		}
	})

	t.Run("money readings do not affect headroom", func(t *testing.T) {
		repo := &fakeRepo{}
		cache := activity.NewCache(repo, activity.CacheOptions{})
		if err := cache.Record(context.Background(), "credential-1", []activity.WindowSample{
			{Window: "balance", Amount: new(42.5), Currency: "USD"},
			{Window: "weekly", UsedPercent: 80},
		}); err != nil {
			t.Fatalf("Record() error = %v", err)
		}
		windows, err := cache.Windows(context.Background(), "credential-1")
		if err != nil || len(windows) != 2 || windows[1].Amount == nil {
			t.Fatalf("Windows() = %+v, %v, want the percent and money readings", windows, err)
		}
		if headroom, ok := cache.MostConstrained(context.Background(), "credential-1"); !ok || headroom != 20 {
			t.Fatalf("MostConstrained() = %v, %v, want only the percent window", headroom, ok)
		}
		if _, ok := cache.Headroom(context.Background(), "credential-1", "balance"); ok {
			t.Fatal("Headroom() treated a money reading as a percent window")
		}
	})

	t.Run("repository failures are reported", func(t *testing.T) {
		repo := &fakeRepo{upsertErr: errors.New("disk is full"), listErr: errors.New("disk is gone")}
		cache := activity.NewCache(repo, activity.CacheOptions{})
		if err := cache.Record(context.Background(), "credential-1", []activity.WindowSample{{Window: "Gem"}}); err == nil {
			t.Fatal("Record() error = nil, want the store failure")
		}
		if _, err := cache.Windows(context.Background(), "credential-1"); err == nil {
			t.Fatal("Windows() error = nil, want the read failure")
		}
		if _, ok := cache.Headroom(context.Background(), "credential-1", "Gem"); ok {
			t.Fatal("Headroom() reported a window although the read failed")
		}
		if _, ok := cache.MostConstrained(context.Background(), "credential-1"); ok {
			t.Fatal("MostConstrained() reported a window although the read failed")
		}
	})
}

func TestHistory(t *testing.T) {
	t.Run("append trims with the fixed bounds", func(t *testing.T) {
		repo := &fakeRepo{}
		history := activity.NewHistory(repo, activity.HistoryOptions{})
		if err := history.Append(context.Background(), []activity.Snapshot{{CredentialID: "c", Window: "Gem"}}); err != nil {
			t.Fatalf("Append() error = %v", err)
		}
		if len(repo.history) != 1 {
			t.Fatalf("history = %+v, want the observation stored", repo.history)
		}
		if len(repo.trims) != 1 || repo.trims[0] != activity.DefaultHistoryBounds() {
			t.Fatalf("trims = %+v, want the documented bounds", repo.trims)
		}
		if err := history.Append(context.Background(), nil); err != nil {
			t.Fatalf("Append() error = %v", err)
		}
		if len(repo.trims) != 1 {
			t.Fatal("an empty append triggered a trim")
		}
	})

	t.Run("trim failures are logged and returned", func(t *testing.T) {
		logger, buffer := testkit.TestLogger(t)
		repo := &fakeRepo{trimErr: errors.New("database is gone")}
		history := activity.NewHistory(repo, activity.HistoryOptions{Logger: logger, Bounds: activity.HistoryBounds{PerCredential: 2}})
		if err := history.Trim(context.Background()); err == nil {
			t.Fatal("Trim() error = nil, want the trim failure")
		}
		if !strings.Contains(buffer.String(), "trim failed") {
			t.Fatalf("log = %q, want the trim failure logged", buffer.String())
		}
	})

	t.Run("append reports a store failure", func(t *testing.T) {
		repo := &fakeRepo{appendErr: errors.New("disk is full")}
		history := activity.NewHistory(repo, activity.HistoryOptions{})
		if err := history.Append(context.Background(), []activity.Snapshot{{CredentialID: "c"}}); err == nil {
			t.Fatal("Append() error = nil, want the store failure")
		}
	})

	t.Run("append skips money readings", func(t *testing.T) {
		repo := &fakeRepo{}
		history := activity.NewHistory(repo, activity.HistoryOptions{})
		if err := history.Append(context.Background(), []activity.Snapshot{
			{CredentialID: "c", Window: "balance", Amount: new(10.0), Currency: "USD"},
			{CredentialID: "c", Window: "5h", UsedPercent: 20},
		}); err != nil {
			t.Fatalf("Append() error = %v", err)
		}
		if len(repo.history) != 1 || repo.history[0].Window != "5h" {
			t.Fatalf("history = %+v, want only the percent reading", repo.history)
		}
	})

	t.Run("the bounds match the reference", func(t *testing.T) {
		bounds := activity.DefaultHistoryBounds()
		if bounds.PerCredential != 200 || bounds.Credentials != 64 || bounds.Total != 4096 || bounds.MaxAge != 30*24*time.Hour {
			t.Fatalf("bounds = %+v, want 200/64/4096/30d", bounds)
		}
	})
}

func TestWorker(t *testing.T) {
	t.Run("a probe records the windows", func(t *testing.T) {
		repo := &fakeRepo{}
		cache := activity.NewCache(repo, activity.CacheOptions{Clock: testkit.NewFakeClock(time.Now())})
		prober := &fakeProber{samples: []activity.WindowSample{{Window: "Gem", UsedPercent: 30}}}
		worker := activity.NewWorker(cache, activity.WorkerOptions{Probers: map[string]activity.QuotaProber{"google-antigravity": prober}})
		windows, err := worker.Probe(context.Background(), activity.Credential{ID: "credential-1", ProviderID: "google-antigravity"})
		if err != nil {
			t.Fatalf("Probe() error = %v", err)
		}
		if len(windows) != 1 || windows[0].Headroom() != 70 {
			t.Fatalf("windows = %+v, want the recorded window", windows)
		}
		if worker.Stale("credential-1") {
			t.Fatal("a successful probe was reported as stale")
		}
	})

	t.Run("a failed probe serves the last good snapshot", func(t *testing.T) {
		repo := &fakeRepo{}
		cache := activity.NewCache(repo, activity.CacheOptions{})
		if err := cache.Record(context.Background(), "credential-1", []activity.WindowSample{{Window: "Gem", UsedPercent: 30}}); err != nil {
			t.Fatalf("Record() error = %v", err)
		}
		prober := &fakeProber{err: errors.New("upstream is down")}
		worker := activity.NewWorker(cache, activity.WorkerOptions{Probers: map[string]activity.QuotaProber{"google-antigravity": prober}})
		windows, err := worker.Probe(context.Background(), activity.Credential{ID: "credential-1", ProviderID: "google-antigravity"})
		if err != nil {
			t.Fatalf("Probe() error = %v, want the stale snapshot", err)
		}
		if len(windows) != 1 || windows[0].Headroom() != 70 {
			t.Fatalf("windows = %+v, want the stale snapshot", windows)
		}
		if !worker.Stale("credential-1") {
			t.Fatal("a failed probe did not mark the credential stale")
		}
		worker.NoteFresh("credential-1")
		if worker.Stale("credential-1") {
			t.Fatal("a live reading left the failed probe marked stale")
		}
	})

	t.Run("a connection refresh probes only that connection", func(t *testing.T) {
		repo := &fakeRepo{}
		cache := activity.NewCache(repo, activity.CacheOptions{})
		prober := &fakeProber{samples: []activity.WindowSample{{Window: "Gem", UsedPercent: 10}}}
		worker := activity.NewWorker(cache, activity.WorkerOptions{
			Probers: map[string]activity.QuotaProber{"google-antigravity": prober},
			Source: &fakeSource{credentials: []activity.Credential{
				{ID: "credential-1", ProviderID: "google-antigravity"},
				{ID: "credential-2", ProviderID: "other"},
			}},
		})
		windows, err := worker.ProbeProvider(context.Background(), "google-antigravity")
		if err != nil {
			t.Fatalf("ProbeProvider() error = %v", err)
		}
		if prober.callCount() != 1 || len(windows) != 1 || windows[0].CredentialID != "credential-1" {
			t.Fatalf("windows = %+v calls = %d, want the one connection", windows, prober.callCount())
		}
	})

	t.Run("a refused probe marks the credential for a new login", func(t *testing.T) {
		repo := &fakeRepo{}
		cache := activity.NewCache(repo, activity.CacheOptions{})
		marker := &fakeMarker{}
		worker := activity.NewWorker(cache, activity.WorkerOptions{
			Probers: map[string]activity.QuotaProber{"google-antigravity": &fakeProber{err: activity.ErrProbeRejected}},
			Marker:  marker,
		})
		if _, err := worker.Probe(context.Background(), activity.Credential{ID: "credential-1", ProviderID: "google-antigravity"}); err != nil {
			t.Fatalf("Probe() error = %v", err)
		}
		if len(marker.ids) != 1 || marker.ids[0] != "credential-1" {
			t.Fatalf("marked = %v, want the refused credential", marker.ids)
		}
	})

	t.Run("a probe runs one credential at a time", func(t *testing.T) {
		repo := &fakeRepo{}
		cache := activity.NewCache(repo, activity.CacheOptions{})
		prober := &fakeProber{samples: []activity.WindowSample{{Window: "Gem"}}, block: make(chan struct{})}
		worker := activity.NewWorker(cache, activity.WorkerOptions{Probers: map[string]activity.QuotaProber{"google-antigravity": prober}})
		credential := activity.Credential{ID: "credential-1", ProviderID: "google-antigravity"}
		done := make(chan struct{})
		go func() {
			defer close(done)
			if _, err := worker.Probe(context.Background(), credential); err != nil {
				t.Errorf("Probe() error = %v", err)
			}
		}()
		waitForCalls(t, prober, 1)
		if _, err := worker.Probe(context.Background(), credential); err != nil {
			t.Fatalf("Probe() error = %v, want the in-flight probe to answer from the cache", err)
		}
		if calls := prober.callCount(); calls != 1 {
			t.Fatalf("prober calls = %d, want one probe while the first is in flight", calls)
		}
		close(prober.block)
		<-done
	})

	t.Run("refresh probes every credential and skips unknown providers", func(t *testing.T) {
		repo := &fakeRepo{}
		cache := activity.NewCache(repo, activity.CacheOptions{})
		prober := &fakeProber{samples: []activity.WindowSample{{Window: "Gem"}}}
		logger, buffer := testkit.TestLogger(t)
		worker := activity.NewWorker(cache, activity.WorkerOptions{
			Probers: map[string]activity.QuotaProber{"google-antigravity": prober},
			Source: &fakeSource{credentials: []activity.Credential{
				{ID: "credential-1", ProviderID: "google-antigravity"},
				{ID: "credential-2", ProviderID: "absent"},
			}},
			Logger: logger,
		})
		if err := worker.Refresh(context.Background()); err != nil {
			t.Fatalf("Refresh() error = %v", err)
		}
		if prober.callCount() != 1 {
			t.Fatalf("prober calls = %d, want only the known provider probed", prober.callCount())
		}
		if !strings.Contains(buffer.String(), "no quota prober") {
			t.Fatalf("log = %q, want the missing prober reported", buffer.String())
		}
	})

	t.Run("a slow account does not hold the rest of the sweep", func(t *testing.T) {
		cache := activity.NewCache(&fakeRepo{}, activity.CacheOptions{})
		slow := &fakeProber{samples: []activity.WindowSample{{Window: "Gem"}}, block: make(chan struct{})}
		fast := &fakeProber{samples: []activity.WindowSample{{Window: "Gem"}}}
		worker := activity.NewWorker(cache, activity.WorkerOptions{
			Probers: map[string]activity.QuotaProber{"slow": slow, "fast": fast},
			Source: &fakeSource{credentials: []activity.Credential{
				{ID: "credential-1", ProviderID: "slow"},
				{ID: "credential-2", ProviderID: "fast"},
			}},
		})
		done := make(chan error, 1)
		go func() { done <- worker.Refresh(context.Background()) }()
		// The fast account answers while the slow one still holds its probe,
		// which a sweep that probes in turn could not do.
		deadline := time.Now().Add(2 * time.Second)
		for fast.callCount() == 0 && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
		if fast.callCount() != 1 {
			t.Fatal("the fast probe waited behind the slow account")
		}
		close(slow.block)
		if err := <-done; err != nil {
			t.Fatalf("Refresh() error = %v", err)
		}
	})

	t.Run("refresh reports a source failure", func(t *testing.T) {
		cache := activity.NewCache(&fakeRepo{}, activity.CacheOptions{})
		worker := activity.NewWorker(cache, activity.WorkerOptions{Source: &fakeSource{err: errors.New("database is gone")}})
		if err := worker.Refresh(context.Background()); err == nil {
			t.Fatal("Refresh() error = nil, want the source failure")
		}
	})

	t.Run("a probe without a prober is refused", func(t *testing.T) {
		cache := activity.NewCache(&fakeRepo{}, activity.CacheOptions{})
		worker := activity.NewWorker(cache, activity.WorkerOptions{})
		if _, err := worker.Probe(context.Background(), activity.Credential{ProviderID: "absent"}); !errors.Is(err, activity.ErrNoProber) {
			t.Fatalf("Probe() error = %v, want %v", err, activity.ErrNoProber)
		}
	})

	t.Run("a store failure after a probe is reported", func(t *testing.T) {
		cache := activity.NewCache(&fakeRepo{upsertErr: errors.New("disk is full")}, activity.CacheOptions{})
		worker := activity.NewWorker(cache, activity.WorkerOptions{
			Probers: map[string]activity.QuotaProber{"google-antigravity": &fakeProber{samples: []activity.WindowSample{{Window: "Gem"}}}},
		})
		_, err := worker.Probe(context.Background(), activity.Credential{ID: "credential-1", ProviderID: "google-antigravity"})
		if err == nil {
			t.Fatal("Probe() error = nil, want the store failure")
		}
	})

	t.Run("run probes on a jittered interval", func(t *testing.T) {
		clock := testkit.NewFakeClock(time.Now())
		cache := activity.NewCache(&fakeRepo{}, activity.CacheOptions{Clock: clock})
		prober := &fakeProber{samples: []activity.WindowSample{{Window: "Gem"}}}
		worker := activity.NewWorker(cache, activity.WorkerOptions{
			Probers:  map[string]activity.QuotaProber{"google-antigravity": prober},
			Source:   &fakeSource{credentials: []activity.Credential{{ID: "credential-1", ProviderID: "google-antigravity"}}},
			Clock:    clock,
			Interval: time.Minute,
			Jitter:   time.Minute,
		})
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() {
			worker.Run(ctx)
			close(done)
		}()
		deadline := time.Now().Add(2 * time.Second)
		for prober.callCount() == 0 && time.Now().Before(deadline) {
			clock.Add(30 * time.Second)
			time.Sleep(5 * time.Millisecond)
		}
		if prober.callCount() == 0 {
			t.Fatal("Run() did not probe the credential")
		}
		before := prober.callCount()
		deadline = time.Now().Add(2 * time.Second)
		for prober.callCount() == before && time.Now().Before(deadline) {
			clock.Add(time.Minute)
			time.Sleep(5 * time.Millisecond)
		}
		if prober.callCount() == before {
			t.Fatal("Run() did not probe again on the next interval")
		}
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("Run() did not stop when its context ended")
		}
	})
}

type fakeRepo struct {
	mu        sync.Mutex
	snapshots []activity.Snapshot
	history   []activity.Snapshot
	trims     []activity.HistoryBounds
	upsertErr error
	listErr   error
	appendErr error
	trimErr   error
}

func (r *fakeRepo) UpsertSnapshots(_ context.Context, snapshots []activity.Snapshot) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.upsertErr != nil {
		return r.upsertErr
	}
	for _, snapshot := range snapshots {
		replaced := false
		for index := range r.snapshots {
			if r.snapshots[index].CredentialID == snapshot.CredentialID && r.snapshots[index].Window == snapshot.Window {
				r.snapshots[index] = snapshot
				replaced = true
			}
		}
		if !replaced {
			r.snapshots = append(r.snapshots, snapshot)
		}
	}
	return nil
}

func (r *fakeRepo) ListSnapshots(_ context.Context, credentialID string) ([]activity.Snapshot, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.listErr != nil {
		return nil, r.listErr
	}
	found := make([]activity.Snapshot, 0, len(r.snapshots))
	for _, snapshot := range r.snapshots {
		if snapshot.CredentialID == credentialID {
			found = append(found, snapshot)
		}
	}
	return found, nil
}

func (r *fakeRepo) AppendHistory(_ context.Context, snapshots []activity.Snapshot) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.appendErr != nil {
		return r.appendErr
	}
	r.history = append(r.history, snapshots...)
	return nil
}

func (r *fakeRepo) TrimHistory(_ context.Context, bounds activity.HistoryBounds) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.trimErr != nil {
		return r.trimErr
	}
	r.trims = append(r.trims, bounds)
	return nil
}

type fakeProber struct {
	mu      sync.Mutex
	samples []activity.WindowSample
	err     error
	calls   int
	block   chan struct{}
}

func (p *fakeProber) Probe(context.Context, activity.Credential) ([]activity.WindowSample, error) {
	p.mu.Lock()
	p.calls++
	block := p.block
	samples, err := p.samples, p.err
	p.mu.Unlock()
	if block != nil {
		<-block
	}
	if err != nil {
		return nil, err
	}
	return samples, nil
}

func (p *fakeProber) callCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

type fakeSource struct {
	credentials []activity.Credential
	err         error
}

func (s *fakeSource) List(context.Context) ([]activity.Credential, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.credentials, nil
}

type fakeMarker struct {
	ids []string
}

func (m *fakeMarker) MarkNeedsReauth(_ context.Context, id string) error {
	m.ids = append(m.ids, id)
	return nil
}

func TestClaudeProbeRefreshesOnce(t *testing.T) {
	repo := &fakeRepo{}
	cache := activity.NewCache(repo, activity.CacheOptions{})
	prober := &tokenProber{reject: "stale", samples: []activity.WindowSample{{Window: "5h", UsedPercent: 10}}}
	source := &fakeSource{credentials: []activity.Credential{{ID: "credential-1", ProviderID: "claude", AccessToken: "stale"}}}
	marker := &fakeMarker{}
	worker := activity.NewWorker(cache, activity.WorkerOptions{
		Probers: map[string]activity.QuotaProber{"claude": prober},
		Source:  source,
		Marker:  marker,
		Refresher: refreshFunc(func(context.Context, string, string, string) error {
			source.credentials[0].AccessToken = "fresh"
			return nil
		}),
	})
	windows, err := worker.Probe(context.Background(), source.credentials[0])
	if err != nil {
		t.Fatalf("Probe() error = %v", err)
	}
	if prober.calls != 2 || len(windows) != 1 || len(marker.ids) != 0 || worker.Stale("credential-1") {
		t.Fatalf("calls = %d windows = %+v marked = %v, want one retry", prober.calls, windows, marker.ids)
	}
}

func TestClaudeProbeReauthAndTransient(t *testing.T) {
	t.Run("a rejected refresh marks a new login", func(t *testing.T) {
		cache := activity.NewCache(&fakeRepo{}, activity.CacheOptions{})
		marker := &fakeMarker{}
		worker := activity.NewWorker(cache, activity.WorkerOptions{
			Probers: map[string]activity.QuotaProber{"claude": &fakeProber{err: activity.ErrProbeRejected}},
			Marker:  marker,
			Refresher: refreshFunc(func(context.Context, string, string, string) error {
				return activity.ErrReauthRequired
			}),
		})
		if _, err := worker.Probe(context.Background(), activity.Credential{ID: "credential-1", ProviderID: "claude", AccessToken: "stale"}); err != nil {
			t.Fatalf("Probe() error = %v", err)
		}
		if len(marker.ids) != 1 {
			t.Fatalf("marked = %v, want the refused credential", marker.ids)
		}
	})

	t.Run("a transport failure stays active", func(t *testing.T) {
		cache := activity.NewCache(&fakeRepo{}, activity.CacheOptions{})
		marker := &fakeMarker{}
		worker := activity.NewWorker(cache, activity.WorkerOptions{
			Probers: map[string]activity.QuotaProber{"claude": &fakeProber{err: activity.ErrProbeRejected}},
			Marker:  marker,
			Refresher: refreshFunc(func(context.Context, string, string, string) error {
				return errors.New("token endpoint is down")
			}),
		})
		if _, err := worker.Probe(context.Background(), activity.Credential{ID: "credential-1", ProviderID: "claude", AccessToken: "stale"}); err != nil {
			t.Fatalf("Probe() error = %v", err)
		}
		if len(marker.ids) != 0 || !worker.Stale("credential-1") {
			t.Fatalf("marked = %v stale = %v, want a stale account that stays signed in", marker.ids, worker.Stale("credential-1"))
		}
	})
}

func TestCodexProbeRefreshesOnce(t *testing.T) {
	repo := &fakeRepo{}
	cache := activity.NewCache(repo, activity.CacheOptions{})
	prober := &tokenProber{reject: "stale", samples: []activity.WindowSample{{Window: "5h", UsedPercent: 10}}}
	source := &fakeSource{credentials: []activity.Credential{{ID: "credential-9", ProviderID: "openai-codex", AccessToken: "stale"}}}
	marker := &fakeMarker{}
	worker := activity.NewWorker(cache, activity.WorkerOptions{
		Probers: map[string]activity.QuotaProber{"openai-codex": prober},
		Source:  source,
		Marker:  marker,
		Refresher: refreshFunc(func(_ context.Context, providerID, _, _ string) error {
			if providerID != "openai-codex" {
				return activity.ErrNoCredential
			}
			source.credentials[0].AccessToken = "fresh"
			return nil
		}),
	})
	windows, err := worker.Probe(context.Background(), source.credentials[0])
	if err != nil {
		t.Fatalf("Probe() error = %v", err)
	}
	if prober.calls != 2 || len(windows) != 1 || len(marker.ids) != 0 || worker.Stale("credential-9") {
		t.Fatalf("calls = %d windows = %+v marked = %v, want one retry", prober.calls, windows, marker.ids)
	}
}

type refreshFunc func(context.Context, string, string, string) error

func (f refreshFunc) RefreshRejected(ctx context.Context, providerID, credentialID, rejectedToken string) error {
	return f(ctx, providerID, credentialID, rejectedToken)
}

type tokenProber struct {
	reject  string
	samples []activity.WindowSample
	calls   int
}

func (p *tokenProber) Probe(_ context.Context, credential activity.Credential) ([]activity.WindowSample, error) {
	p.calls++
	if credential.AccessToken == p.reject {
		return nil, activity.ErrProbeRejected
	}
	return p.samples, nil
}

func waitForCalls(t *testing.T, prober *fakeProber, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for prober.callCount() < want {
		if time.Now().After(deadline) {
			t.Fatalf("prober calls = %d, want %d", prober.callCount(), want)
		}
		time.Sleep(5 * time.Millisecond)
	}
}
