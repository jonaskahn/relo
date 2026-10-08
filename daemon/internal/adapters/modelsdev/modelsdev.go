// models.dev directory: cached downloads of the dataset.
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

const fetchBackoff = 5 * time.Minute

// State reports the current status of models.dev data.
type State = catalog.ModelsDevState

// Directory manages fetching, caching, and serving models.dev catalog data.
type Directory struct {
	mu           sync.RWMutex
	cacheFile    string
	sourceURL    string
	httpClient   *http.Client
	index        *Index
	etag         string
	lastModified string
	fetchedAtMs  int64
	lastError    string
	// failedAt is when the last live fetch failed, which starts the backoff,
	// and cached holds the saved copy parsed for readers that never fetch.
	failedAt      time.Time
	cached        *Index
	cachedModTime time.Time
}

// NewDirectory creates a new models.dev Directory.
func NewDirectory(cacheDir, sourceURL string, client *http.Client) *Directory {
	if sourceURL == "" {
		sourceURL = DefaultURL
	}
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	cacheFile := filepath.Join(cacheDir, "modelsdev.json")
	return &Directory{
		cacheFile:  cacheFile,
		sourceURL:  sourceURL,
		httpClient: client,
	}
}

// Get returns the loaded index, fetching online or using the cache.
func (d *Directory) Get(ctx context.Context, mode Mode) (*Index, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if cached, ok := d.serveMemoryLocked(mode); ok {
		return cached, nil
	}
	nowMs := time.Now().UnixMilli()

	data, revalidated, fetchErr := d.fetchDatasetLocked(ctx, mode, nowMs)
	if revalidated {
		return d.index, nil
	}
	if len(data) == 0 {
		return d.staleDatasetLocked(data, fetchErr, nowMs)
	}
	return d.parseDatasetLocked(data, fetchErr, false, nowMs)
}

func (d *Directory) serveMemoryLocked(mode Mode) (*Index, bool) {
	// If already in memory and not forced and not stale, return it
	if d.index != nil && mode != ModeForce && !d.index.State.Stale {
		return d.index, true
	}

	// A live fetch that just failed is left alone until the backoff passes, so
	// a network that is down costs one attempt every few minutes and the copy
	// already held answers in the meantime.
	if mode != ModeForce && !d.failedAt.IsZero() && time.Since(d.failedAt) < fetchBackoff {
		if d.index != nil {
			return d.index, true
		}
		if cached, err := d.loadFileLocked(); err == nil && cached != nil {
			return cached, true
		}
	}
	return nil, false
}

func (d *Directory) fetchDatasetLocked(ctx context.Context, mode Mode, nowMs int64) ([]byte, bool, error) {
	req, err := d.newDatasetRequest(ctx, mode)
	if err != nil {
		return nil, false, err
	}
	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = resp.Body.Close() }()
	return d.receiveDataset(resp, nowMs)
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

func (d *Directory) receiveDataset(resp *http.Response, nowMs int64) ([]byte, bool, error) {
	if resp.StatusCode == http.StatusNotModified && d.index != nil {
		d.index.State.LastAttemptAtMs = nowMs
		d.index.State.LastError = ""
		d.failedAt = time.Time{}
		return nil, true, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("%w %d", ErrFetchStatus, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, false, err
	}
	d.failedAt = time.Time{}
	d.forgetFileLocked()
	d.etag = resp.Header.Get("ETag")
	d.lastModified = resp.Header.Get("Last-Modified")
	d.fetchedAtMs = nowMs
	_ = d.saveCache(body)
	return body, false, nil
}

func (d *Directory) staleDatasetLocked(data []byte, fetchErr error, nowMs int64) (*Index, error) {
	stale := false
	if len(data) == 0 {
		cachedData, err := os.ReadFile(d.cacheFile)
		if err == nil && len(cachedData) > 0 {
			data = cachedData
			stale = true
		}
	}

	if len(data) > 0 {
		return d.parseDatasetLocked(data, fetchErr, stale, nowMs)
	}
	errMsg := ""
	if fetchErr != nil {
		errMsg = fetchErr.Error()
	}
	if fetchErr != nil {
		d.failedAt = time.Now()
	}
	d.lastError = errMsg
	emptyIndex := d.emptyDatasetLocked(nowMs, errMsg)
	d.index = emptyIndex
	return emptyIndex, fetchErr
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

func (d *Directory) parseDatasetLocked(data []byte, fetchErr error, stale bool, nowMs int64) (*Index, error) {
	idx, err := parseIndex(data)
	if err != nil {
		d.lastError = err.Error()
		return nil, fmt.Errorf("parse models.dev data: %w", err)
	}

	errStr := ""
	if fetchErr != nil {
		errStr = fetchErr.Error()
	}

	idx.State = State{
		SourceURL:       d.sourceURL,
		FetchedAtMs:     d.fetchedAtMs,
		Stale:           stale,
		Available:       true,
		ETag:            d.etag,
		LastModified:    d.lastModified,
		ProvidersCount:  len(idx.Providers),
		ModelsCount:     idx.TotalModels(),
		LastAttemptAtMs: nowMs,
		LastError:       errStr,
	}

	d.index = idx
	d.lastError = errStr
	return idx, fetchErr
}

// CurrentState returns the current State.
func (d *Directory) CurrentState() State {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.index != nil {
		return d.index.State
	}
	return State{
		SourceURL: d.sourceURL,
		Available: false,
	}
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

// Cached returns the copy this process already holds or the one saved on
// disk, and never asks the network for it. A caller that has to answer now
// uses this; a caller that wants the newest data uses Get.
func (d *Directory) Cached() (*Index, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.index != nil && d.index.State.Available && !d.index.State.Stale {
		return d.index, nil
	}
	return d.loadFileLocked()
}

func (d *Directory) loadFileLocked() (*Index, error) {
	info, err := os.Stat(d.cacheFile)
	if err != nil {
		return nil, nil
	}
	if d.cached != nil && info.ModTime().Equal(d.cachedModTime) {
		return d.cached, nil
	}
	data, err := os.ReadFile(d.cacheFile)
	if err != nil || len(data) == 0 {
		return nil, nil
	}
	idx, err := parseIndex(data)
	if err != nil {
		return nil, fmt.Errorf("parse models.dev data: %w", err)
	}
	idx.State = State{
		SourceURL:       d.sourceURL,
		FetchedAtMs:     info.ModTime().UnixMilli(),
		Available:       true,
		ProvidersCount:  len(idx.Providers),
		ModelsCount:     idx.TotalModels(),
		LastAttemptAtMs: info.ModTime().UnixMilli(),
	}
	d.cached = idx
	d.cachedModTime = info.ModTime()
	return idx, nil
}

func (d *Directory) forgetFileLocked() {
	d.cached = nil
	d.cachedModTime = time.Time{}
}
