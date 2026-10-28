// models.dev directory: network-first reads with a local build seed and cache.
package modelsdev

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/jonaskahn/relo/internal/catalog"
)

// ErrFetchStatus names why the index has nothing fresh to report: a dataset
// source that answered with failure.
var ErrFetchStatus = errors.New("status")

// Mode controls fetching behavior.
type Mode = catalog.ModelsDevFetchMode

// Fetch modes name how the index answers: network first, saved copy only,
// and the source either reads from.
const (
	ModeOnlineFirst = catalog.FetchOnlineFirst
	ModeForce       = catalog.FetchForce
	DefaultURL      = catalog.ModelsDevDefaultURL
)

const (
	fetchBackoff    = 5 * time.Minute
	fetchRevalidate = time.Minute
)

// State reports the current status of models.dev data.
type State = catalog.ModelsDevState

// Directory manages fetching, caching, and serving models.dev catalog data.
type Directory struct {
	mu         sync.RWMutex
	cacheFile  string
	sourceURL  string
	httpClient *http.Client
	seed       Seed

	// seedIndex is decompressed on the first read that needs a local copy.
	seedIndex  *Index
	seedLoaded bool

	index        *Index
	etag         string
	lastModified string
	lastFetchErr error
	lastSuccess  time.Time
	// failedAt starts the backoff after a live fetch fails, and cached holds
	// the saved copy parsed for readers that never fetch.
	failedAt      time.Time
	cached        *Index
	cachedModTime time.Time
	// now is the clock tests pin; production uses time.Now.
	now func() time.Time
}

// NewDirectory creates a seedless models.dev Directory. A caller that wants
// the embedded build seed passes it to NewDirectoryWithSeed.
func NewDirectory(cacheDir, sourceURL string, client *http.Client) *Directory {
	return NewDirectoryWithSeed(cacheDir, sourceURL, client, Seed{})
}

// NewDirectoryWithSeed creates a Directory that falls back to seed. A seed is
// used only for the official source; a custom catalog URL never receives the
// embedded copy.
func NewDirectoryWithSeed(cacheDir, sourceURL string, client *http.Client, seed Seed) *Directory {
	if sourceURL == "" {
		sourceURL = DefaultURL
	}
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	dir := &Directory{
		cacheFile:  filepath.Join(cacheDir, "modelsdev.json"),
		sourceURL:  sourceURL,
		httpClient: client,
		now:        time.Now,
	}
	if sourceURL == DefaultURL && len(seed.Gzip) > 0 {
		dir.seed = seed
	}
	return dir
}

// Get returns the loaded index, fetching online before using a local copy.
func (d *Directory) Get(ctx context.Context, mode Mode) (*Index, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if cached, err, ok := d.serveMemoryLocked(mode); ok {
		return cached, err
	}
	now := d.nowTime()
	nowMs := now.UnixMilli()

	result, err := d.fetchDatasetLocked(ctx, mode)
	if err != nil {
		return d.recordFetchFailureLocked(err, now, nowMs)
	}
	if result.notModified {
		return d.acceptNotModifiedLocked(now, nowMs)
	}
	return d.acceptDatasetLocked(result, now, nowMs)
}

func (d *Directory) nowTime() time.Time {
	if d.now != nil {
		return d.now()
	}
	return time.Now()
}

func (d *Directory) serveMemoryLocked(mode Mode) (*Index, error, bool) {
	if mode == ModeForce {
		return nil, nil, false
	}
	// A live fetch that just failed is left alone until the backoff passes, so
	// a network that is down costs one attempt every few minutes and the copy
	// already held answers in the meantime.
	if !d.failedAt.IsZero() && d.nowTime().Sub(d.failedAt) < fetchBackoff {
		if d.index != nil {
			if d.index.State.Available {
				return d.index, nil, true
			}
			return d.index, d.lastFetchErr, true
		}
	}
	// A successful read is asked again at most once a minute; until then the
	// copy this process holds answers without touching the network.
	if d.index != nil && d.index.State.Available && !d.index.State.Stale &&
		!d.lastSuccess.IsZero() && d.nowTime().Sub(d.lastSuccess) < fetchRevalidate {
		return d.index, nil, true
	}
	return nil, nil, false
}

type fetchResult struct {
	body         []byte
	notModified  bool
	etag         string
	lastModified string
}

func (d *Directory) fetchDatasetLocked(ctx context.Context, mode Mode) (fetchResult, error) {
	req, err := d.newDatasetRequest(ctx, mode)
	if err != nil {
		return fetchResult{}, err
	}
	resp, err := d.httpClient.Do(req)
	if err != nil {
		return fetchResult{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	return receiveDataset(resp)
}

func (d *Directory) newDatasetRequest(ctx context.Context, mode Mode) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, d.sourceURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "relo")
	if d.etag != "" && mode != ModeForce {
		req.Header.Set("If-None-Match", d.etag)
	}
	if d.lastModified != "" && mode != ModeForce {
		req.Header.Set("If-Modified-Since", d.lastModified)
	}
	return req, nil
}

func receiveDataset(resp *http.Response) (fetchResult, error) {
	if resp.StatusCode == http.StatusNotModified {
		return fetchResult{notModified: true}, nil
	}
	if resp.StatusCode != http.StatusOK {
		return fetchResult{}, fmt.Errorf("%w %d", ErrFetchStatus, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxDatasetBytes+1))
	if err != nil {
		return fetchResult{}, err
	}
	if len(body) > maxDatasetBytes {
		return fetchResult{}, fmt.Errorf("models.dev dataset exceeds %d bytes", maxDatasetBytes)
	}
	return fetchResult{
		body: body, etag: resp.Header.Get("ETag"),
		lastModified: resp.Header.Get("Last-Modified"),
	}, nil
}

func (d *Directory) acceptNotModifiedLocked(now time.Time, nowMs int64) (*Index, error) {
	if d.index == nil || !d.index.State.Available {
		return d.recordFetchFailureLocked(
			fmt.Errorf("models.dev answered %d without a local copy", http.StatusNotModified), now, nowMs)
	}
	d.index.State.LastAttemptAtMs = nowMs
	d.index.State.LastError = ""
	d.lastFetchErr = nil
	d.lastSuccess = now
	d.failedAt = time.Time{}
	return d.index, nil
}

func (d *Directory) acceptDatasetLocked(result fetchResult, now time.Time, nowMs int64) (*Index, error) {
	idx, err := parseDataset(result.body)
	if err != nil {
		return d.recordFetchFailureLocked(fmt.Errorf("parse models.dev data: %w", err), now, nowMs)
	}
	d.etag = result.etag
	d.lastModified = result.lastModified
	d.failedAt = time.Time{}
	d.lastFetchErr = nil
	d.lastSuccess = now
	d.forgetFileLocked()
	idx.State = d.networkState(idx, nowMs)
	d.index = idx
	_ = d.saveCache(result.body)
	return idx, nil
}

func (d *Directory) recordFetchFailureLocked(fetchErr error, now time.Time, nowMs int64) (*Index, error) {
	d.failedAt = now
	d.lastFetchErr = fetchErr
	errMsg := fetchErr.Error()

	local, _ := d.chooseLocalLocked()
	if local != nil {
		local.idx.State = d.localState(local, nowMs, errMsg)
		d.index = local.idx
		return d.index, fetchErr
	}
	if d.index != nil && d.index.State.Available {
		// The copy this process fetched could not be saved, so it is still
		// the best local answer even though no file holds it.
		d.index.State.LastAttemptAtMs = nowMs
		d.index.State.LastError = errMsg
		d.index.State.Stale = d.index.State.Origin != catalog.ModelsDevOriginSeed
		return d.index, fetchErr
	}
	empty := d.emptyDatasetLocked(nowMs, errMsg)
	d.index = empty
	return empty, fetchErr
}

func (d *Directory) emptyDatasetLocked(nowMs int64, errMsg string) *Index {
	return &Index{
		Providers: make(map[string]Provider),
		State: State{
			SourceURL:       d.sourceURL,
			Available:       false,
			Stale:           false,
			LastAttemptAtMs: nowMs,
			LastError:       errMsg,
		},
	}
}

func (d *Directory) networkState(idx *Index, nowMs int64) State {
	return State{
		SourceURL:       d.sourceURL,
		FetchedAtMs:     nowMs,
		Available:       true,
		ProvidersCount:  len(idx.Providers),
		ModelsCount:     idx.TotalModels(),
		LastAttemptAtMs: nowMs,
		Origin:          catalog.ModelsDevOriginNetwork,
		ETag:            d.etag,
		LastModified:    d.lastModified,
	}
}

func (d *Directory) localState(copy *localCopy, nowMs int64, errMsg string) State {
	lastAttempt := nowMs
	if errMsg == "" {
		lastAttempt = copy.at.UnixMilli()
	}
	return State{
		SourceURL:       d.sourceURL,
		FetchedAtMs:     copy.at.UnixMilli(),
		Stale:           copy.origin == catalog.ModelsDevOriginCache && errMsg != "",
		Available:       true,
		ProvidersCount:  len(copy.idx.Providers),
		ModelsCount:     copy.idx.TotalModels(),
		LastAttemptAtMs: lastAttempt,
		LastError:       errMsg,
		Origin:          copy.origin,
	}
}

// CurrentState reports the copy this process holds, loading the local copy
// when it has not been read yet. It never asks the network.
func (d *Directory) CurrentState() State {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.index != nil {
		return d.index.State
	}
	local, err := d.chooseLocalLocked()
	if local != nil {
		local.idx.State = d.localState(local, 0, "")
		d.index = local.idx
		return d.index.State
	}
	state := State{SourceURL: d.sourceURL, Available: false}
	if err != nil {
		state.LastError = err.Error()
	}
	return state
}

func (d *Directory) saveCache(data []byte) error {
	dir := filepath.Dir(d.cacheFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	tmpFile := d.cacheFile + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmpFile, d.cacheFile)
}

// Cached returns the copy this process already holds or the local copy on
// disk, and never asks the network for it.
func (d *Directory) Cached() (*Index, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.index != nil && d.index.State.Available {
		return d.index, nil
	}
	local, err := d.chooseLocalLocked()
	if local == nil {
		return nil, err
	}
	local.idx.State = d.localState(local, 0, "")
	d.index = local.idx
	return d.index, nil
}

type localCopy struct {
	idx    *Index
	origin catalog.ModelsDevOrigin
	at     time.Time
}

// chooseLocalLocked applies one precedence rule: a valid cache at least as new
// as the seed wins, otherwise the seed answers, and a corrupt cache falls back
// to the seed.
func (d *Directory) chooseLocalLocked() (*localCopy, error) {
	cache, cacheErr := d.loadCacheLocked()
	seed := d.seedCopyLocked()
	if cache != nil && (seed == nil || !cache.at.Before(seed.at)) {
		return cache, nil
	}
	if seed != nil {
		return seed, nil
	}
	return nil, cacheErr
}

func (d *Directory) loadCacheLocked() (*localCopy, error) {
	info, err := os.Stat(d.cacheFile)
	if err != nil {
		return nil, nil
	}
	if d.cached != nil && info.ModTime().Equal(d.cachedModTime) {
		return &localCopy{idx: d.cached, origin: catalog.ModelsDevOriginCache, at: info.ModTime()}, nil
	}
	data, err := os.ReadFile(d.cacheFile)
	if err != nil || len(data) == 0 {
		return nil, nil
	}
	idx, err := parseDataset(data)
	if err != nil {
		return nil, fmt.Errorf("parse models.dev data: %w", err)
	}
	d.cached = idx
	d.cachedModTime = info.ModTime()
	return &localCopy{idx: idx, origin: catalog.ModelsDevOriginCache, at: info.ModTime()}, nil
}

func (d *Directory) seedCopyLocked() *localCopy {
	if d.seedLoaded {
		if d.seedIndex == nil {
			return nil
		}
		return &localCopy{idx: d.seedIndex, origin: catalog.ModelsDevOriginSeed, at: d.seed.FetchedAt}
	}
	d.seedLoaded = true
	data, err := decompressSeed(d.seed)
	if err != nil {
		return nil
	}
	idx, err := parseDataset(data)
	if err != nil {
		return nil
	}
	d.seedIndex = idx
	return &localCopy{idx: idx, origin: catalog.ModelsDevOriginSeed, at: d.seed.FetchedAt}
}

func (d *Directory) forgetFileLocked() {
	d.cached = nil
	d.cachedModTime = time.Time{}
}
