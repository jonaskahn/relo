// Quota worker: refreshing windows and marking reauth.
package activity

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/jonaskahn/relo/internal/clock"
)

const (
	workerInterval = 10 * time.Minute
	workerJitter   = time.Minute
	// probeConcurrency bounds how many accounts are probed at once, so one
	// slow provider costs its own slot instead of the whole sweep's pace.
	probeConcurrency = 4
)

// CredentialSource lists the credentials a worker probes.
type CredentialSource interface {
	List(ctx context.Context) ([]Credential, error)
}

// ReauthMarker records that a credential must be logged in again.
type ReauthMarker interface {
	MarkNeedsReauth(ctx context.Context, id string) error
}

// TokenRefresher renews an access token a quota probe just refused.
type TokenRefresher interface {
	RefreshRejected(ctx context.Context, providerID, credentialID, rejectedToken string) error
}

// ErrReauthRequired reports that a refused token cannot be renewed and the
// account has to be signed in again.
var ErrReauthRequired = errors.New("the account must be logged in again")

// Worker refreshes quota snapshots in the background. One probe per
// credential runs at a time, a failed probe keeps serving the last good
// snapshot, and a probe the provider refuses marks the credential for a
// new login.
type Worker struct {
	cache     *Cache
	probers   map[string]QuotaProber
	resolver  func(Credential) (QuotaProber, bool)
	source    CredentialSource
	marker    ReauthMarker
	refresher TokenRefresher
	clock     clock.Clock
	logger    *slog.Logger
	interval  time.Duration
	jitter    time.Duration
	mu        sync.Mutex
	inflight  map[string]bool
	stale     map[string]bool
}

// WorkerOptions tunes a worker.
type WorkerOptions struct {
	Probers map[string]QuotaProber
	// Resolver builds a prober for a credential the Probers map does not name,
	// so a connection added after the worker started is probed like any other
	// rather than skipped for the life of the process.
	Resolver  func(Credential) (QuotaProber, bool)
	Source    CredentialSource
	Marker    ReauthMarker
	Refresher TokenRefresher
	Clock     clock.Clock
	Logger    *slog.Logger
	Interval  time.Duration
	Jitter    time.Duration
}

// NewWorker returns a quota worker over the given cache and probers.
func NewWorker(cache *Cache, options WorkerOptions) *Worker {
	settings := options
	if settings.Clock == nil {
		settings.Clock = clock.New()
	}
	if settings.Logger == nil {
		settings.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	if settings.Interval <= 0 {
		settings.Interval = workerInterval
	}
	if settings.Jitter <= 0 {
		settings.Jitter = workerJitter
	}
	return &Worker{
		cache:     cache,
		probers:   settings.Probers,
		resolver:  settings.Resolver,
		source:    settings.Source,
		marker:    settings.Marker,
		refresher: settings.Refresher,
		clock:     settings.Clock,
		logger:    settings.Logger,
		interval:  settings.Interval,
		jitter:    settings.Jitter,
		inflight:  map[string]bool{},
		stale:     map[string]bool{},
	}
}

// Refresh probes every credential once, a few accounts at a time.
func (w *Worker) Refresh(ctx context.Context) error {
	credentials, err := w.source.List(ctx)
	if err != nil {
		return fmt.Errorf("list credentials to probe: %w", err)
	}
	slots := make(chan struct{}, probeConcurrency)
	var wait sync.WaitGroup
	for _, credential := range credentials {
		wait.Add(1)
		go func(credential Credential) {
			defer wait.Done()
			slots <- struct{}{}
			defer func() { <-slots }()
			if _, err := w.Probe(ctx, credential); err != nil {
				w.logger.Warn("quota probe failed", "credential", credential.ID, "provider", credential.ProviderID, "error", err)
			}
		}(credential)
	}
	wait.Wait()
	return nil
}

// Probe refreshes one credential and returns the windows the cache now
// holds. A probe that fails serves the last good snapshot.
func (w *Worker) Probe(ctx context.Context, credential Credential) ([]Snapshot, error) {
	prober, found := w.probers[credential.ProviderID]
	if !found && w.resolver != nil {
		prober, found = w.resolver(credential)
	}
	if !found {
		return nil, fmt.Errorf("%s: %w", credential.ProviderID, ErrNoProber)
	}
	if !w.begin(credential.ID) {
		return w.cache.Windows(ctx, credential.ID)
	}
	defer w.end(credential.ID)
	samples, err := prober.Probe(ctx, credential)
	if err != nil && w.refresher != nil && errors.Is(err, ErrProbeRejected) {
		samples, err = w.retryRefused(ctx, prober, credential, err)
	}
	if err != nil {
		w.serveStale(ctx, credential, err)
		return w.cache.Windows(ctx, credential.ID)
	}
	w.markFresh(credential.ID)
	if err := w.cache.Record(ctx, credential.ID, samples); err != nil {
		return nil, err
	}
	return w.cache.Windows(ctx, credential.ID)
}

// Stale reports whether the last probe of a credential failed and nothing
// newer has replaced that observation.
func (w *Worker) Stale(credentialID string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.stale[credentialID]
}

// NoteFresh records that a live reading replaced the failed probe, so the
// windows on screen are no longer an older observation.
func (w *Worker) NoteFresh(credentialID string) {
	if credentialID == "" {
		return
	}
	w.markFresh(credentialID)
}

// ProbeCredential reads one account's quota again. The account is the one
// the worker's source lists under that id.
func (w *Worker) ProbeCredential(ctx context.Context, credentialID string) ([]Snapshot, error) {
	credential, err := w.credential(ctx, credentialID)
	if err != nil {
		return nil, err
	}
	return w.Probe(ctx, credential)
}

// ProbeProvider reads quota again for every account of one connection.
func (w *Worker) ProbeProvider(ctx context.Context, providerID string) ([]Snapshot, error) {
	if w.source == nil {
		return nil, fmt.Errorf("list credentials to probe: %w", ErrNoCredential)
	}
	credentials, err := w.source.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list credentials to probe: %w", err)
	}
	var windows []Snapshot
	for _, credential := range credentials {
		if credential.ProviderID != providerID {
			continue
		}
		probed, err := w.Probe(ctx, credential)
		if err != nil {
			w.logger.Warn("quota probe failed", "credential", credential.ID, "provider", credential.ProviderID, "error", err)
			continue
		}
		windows = append(windows, probed...)
	}
	return windows, nil
}

func (w *Worker) credential(ctx context.Context, credentialID string) (Credential, error) {
	if w.source == nil {
		return Credential{}, fmt.Errorf("list credentials to probe: %w", ErrNoCredential)
	}
	credentials, err := w.source.List(ctx)
	if err != nil {
		return Credential{}, fmt.Errorf("list credentials to probe: %w", err)
	}
	for _, credential := range credentials {
		if credential.ID == credentialID {
			return credential, nil
		}
	}
	return Credential{}, fmt.Errorf("probe quota for %s: %w", credentialID, ErrNoCredential)
}

// Run probes every credential on a jittered interval until the context
// ends.
func (w *Worker) Run(ctx context.Context) {
	for {
		w.probeAll(ctx)
		delay := w.interval + time.Duration(rand.Int64N(int64(w.jitter)))
		select {
		case <-ctx.Done():
			return
		case <-w.clock.After(delay):
		}
	}
}

func (w *Worker) probeAll(ctx context.Context) {
	if err := w.Refresh(ctx); err != nil {
		w.logger.Warn("quota sweep failed", "error", err)
	}
}

func (w *Worker) retryRefused(ctx context.Context, prober QuotaProber, credential Credential, probeErr error) ([]WindowSample, error) {
	refreshErr := w.refresher.RefreshRejected(ctx, credential.ProviderID, credential.ID, credential.AccessToken)
	if errors.Is(refreshErr, ErrReauthRequired) {
		return nil, probeErr
	}
	if refreshErr != nil {
		return nil, refreshErr
	}
	fresh, err := w.credential(ctx, credential.ID)
	if err != nil {
		return nil, err
	}
	return prober.Probe(ctx, fresh)
}

func (w *Worker) serveStale(ctx context.Context, credential Credential, probeErr error) {
	w.markStale(credential.ID)
	w.logger.Warn("serving stale quota", "credential", credential.ID, "error", probeErr)
	if !errors.Is(probeErr, ErrProbeRejected) || w.marker == nil {
		return
	}
	if err := w.marker.MarkNeedsReauth(ctx, credential.ID); err != nil {
		w.logger.Warn("mark credential for a new login", "credential", credential.ID, "error", err)
	}
}

func (w *Worker) begin(credentialID string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.inflight[credentialID] {
		return false
	}
	w.inflight[credentialID] = true
	return true
}

func (w *Worker) end(credentialID string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	delete(w.inflight, credentialID)
}

func (w *Worker) markStale(credentialID string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.stale[credentialID] = true
}

func (w *Worker) markFresh(credentialID string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	delete(w.stale, credentialID)
}
